package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/core/services/registration"
	appErrors "brokle/pkg/errors"
)

// OAuth initiate + callback handlers for Google and GitHub. The two
// providers share identical flow shapes — this file centralises them
// so the provider-specific paths are just a string dispatched to the
// shared logic, not a copy-paste duplication.
//
// Flow overview:
//
//  1. initiate*OAuth → generate state, store in Redis, 302 to provider
//  2. user authenticates on provider, provider redirects back to
//     /api/v1/auth/{google,github}/callback?code=…&state=…
//  3. callback validates state, exchanges code for a profile, and
//     either (a) redirects to the frontend with a login session id
//     for existing OAuth users, (b) redirects to the frontend signup
//     page with an OAuth session id for new users, or (c) redirects
//     to the sign-in page with an error query param on any failure.
//
// IMPORTANT: every callback exit path RETURNS A REDIRECT — never an
// error. Huma has no "return an error AND emit 302" primitive and
// client browsers don't handle JSON error bodies from a URL they
// were redirected to. Errors become query params on the signin
// redirect; the frontend renders the message.

// ----- initiate-google-oauth ---------------------------------------

type InitiateOAuthInput struct {
	InvitationToken string `query:"invitation_token" required:"false" doc:"Optional invitation token. When present, the OAuth signup that follows joins an existing organization instead of creating one."`
}

type InitiateOAuthOutput struct {
	Status   int    `json:"-"`
	Location string `header:"Location"`
}

func (h *handler) initiateGoogleOAuth(ctx context.Context, in *InitiateOAuthInput) (*InitiateOAuthOutput, error) {
	return h.initiateOAuth(ctx, "google", in.InvitationToken)
}

// ----- initiate-github-oauth ---------------------------------------

func (h *handler) initiateGithubOAuth(ctx context.Context, in *InitiateOAuthInput) (*InitiateOAuthOutput, error) {
	return h.initiateOAuth(ctx, "github", in.InvitationToken)
}

// initiateOAuth is the shared path both provider-specific
// initiators dispatch to. State generation + authorization-URL
// construction + the 302 redirect have no provider-dependent
// surface beyond the provider name.
func (h *handler) initiateOAuth(ctx context.Context, provider, invitationToken string) (*InitiateOAuthOutput, error) {
	var invitePtr *string
	if invitationToken != "" {
		invitePtr = &invitationToken
	}

	state, err := h.oauthProvider.GenerateState(ctx, invitePtr)
	if err != nil {
		h.logger.ErrorContext(ctx, "oauth initiate: state generation failed", "provider", provider, "error", err)
		return nil, err
	}

	authURL, err := h.oauthProvider.GetAuthorizationURL(provider, state)
	if err != nil {
		h.logger.ErrorContext(ctx, "oauth initiate: authorization URL build failed", "provider", provider, "error", err)
		return nil, err
	}

	return &InitiateOAuthOutput{Status: http.StatusTemporaryRedirect, Location: authURL}, nil
}

// ----- oauth callback shared types --------------------------------

type OAuthCallbackInput struct {
	Code  string `query:"code" doc:"Authorization code returned by the OAuth provider"`
	State string `query:"state" doc:"CSRF state token echoed back by the provider"`
}

// OAuthCallbackOutput is the redirect-only shape every callback exit
// path emits. Huma's header:"Location" tag turns the Location field
// into the Set-Location header; Status is wired via DefaultStatus in
// the operation registration and the runtime override below.
type OAuthCallbackOutput struct {
	Status   int    `json:"-"`
	Location string `header:"Location"`
}

// ----- google-oauth-callback -------------------------------------

func (h *handler) googleOAuthCallback(ctx context.Context, in *OAuthCallbackInput) (*OAuthCallbackOutput, error) {
	return h.oauthCallback(ctx, "google", in)
}

// ----- github-oauth-callback -------------------------------------

func (h *handler) githubOAuthCallback(ctx context.Context, in *OAuthCallbackInput) (*OAuthCallbackOutput, error) {
	return h.oauthCallback(ctx, "github", in)
}

// oauthCallback is the shared path both provider-specific callbacks
// dispatch to. The security-check cascade is identical for Google
// and GitHub; only the provider name varies.
//
// All exit paths return a redirect response — no errors escape this
// function. When a logged error would have been returned in the gin
// version (which would have produced a JSON body on a URL the
// browser followed), we redirect to the signin page with an error
// query param instead.
func (h *handler) oauthCallback(ctx context.Context, provider string, in *OAuthCallbackInput) (*OAuthCallbackOutput, error) {
	frontend := h.cfg.Server.AppURL

	if in.Code == "" || in.State == "" {
		h.logger.WarnContext(ctx, "oauth callback: missing code or state", "provider", provider)
		return redirectFrontend(frontend+"/auth/signin?error=oauth_failed"), nil
	}

	// Validate state → returns any invitation token the initiator
	// tucked away in Redis alongside the state (so invitation-
	// based OAuth signups survive the round-trip).
	invitationToken, err := h.oauthProvider.ValidateState(ctx, in.State)
	if err != nil {
		h.logger.WarnContext(ctx, "oauth callback: invalid state", "provider", provider, "error", err)
		return redirectFrontend(frontend+"/auth/signin?error=invalid_state"), nil
	}

	// Exchange the code for an OAuth access token.
	token, err := h.oauthProvider.ExchangeCode(ctx, provider, in.Code)
	if err != nil {
		h.logger.ErrorContext(ctx, "oauth callback: code exchange failed", "provider", provider, "error", err)
		return redirectFrontend(frontend+"/auth/signin?error=token_exchange_failed"), nil
	}

	// Fetch provider user profile (email, provider ID, etc.).
	userProfile, err := h.oauthProvider.GetUserProfile(ctx, provider, token)
	if err != nil {
		h.logger.ErrorContext(ctx, "oauth callback: profile fetch failed", "provider", provider, "error", err)
		return redirectFrontend(frontend+"/auth/signin?error=profile_fetch_failed"), nil
	}

	// Does the email already have an account?
	existingUser, lookupErr := h.userSvc.GetUserByEmail(ctx, userProfile.Email)
	if lookupErr == nil && existingUser != nil {
		// Security check 1: must be an OAuth-auth account.
		if existingUser.AuthMethod != "oauth" {
			h.logger.WarnContext(ctx, "oauth callback: password account attempted OAuth login", "email", userProfile.Email, "auth_method", existingUser.AuthMethod)
			return redirectFrontend(frontend + "/auth/signin?error=account_exists_use_password"), nil
		}

		// Security check 2: OAuth provider must match.
		if existingUser.OAuthProvider == nil || *existingUser.OAuthProvider != provider {
			storedProvider := "a_different_provider"
			if existingUser.OAuthProvider != nil {
				storedProvider = *existingUser.OAuthProvider
			}
			h.logger.WarnContext(ctx, "oauth callback: wrong provider", "email", userProfile.Email, "stored", storedProvider, "attempted", provider)
			return redirectFrontend(fmt.Sprintf("%s/auth/signin?error=use_%s", frontend, storedProvider)), nil
		}

		// Security check 3: provider-assigned subject must match.
		if existingUser.OAuthProviderID != nil && *existingUser.OAuthProviderID != userProfile.ProviderID {
			h.logger.ErrorContext(ctx, "oauth callback: provider ID mismatch (possible account takeover)", "email", userProfile.Email, "provider", provider)
			return redirectFrontend(frontend + "/auth/signin?error=authentication_failed"), nil
		}

		// All checks passed. Generate tokens + one-time login session.
		loginTokens, tokErr := h.authSvc.GenerateTokensForUser(ctx, existingUser.ID)
		if tokErr != nil {
			h.logger.ErrorContext(ctx, "oauth callback: token generation failed", "error", tokErr)
			return redirectFrontend(frontend + "/auth/signin?error=login_failed"), nil
		}

		sessionID, sessErr := h.authSvc.CreateLoginTokenSession(ctx, loginTokens.AccessToken, loginTokens.RefreshToken, loginTokens.ExpiresIn, existingUser.ID)
		if sessErr != nil {
			h.logger.ErrorContext(ctx, "oauth callback: login-session creation failed", "error", sessErr)
			return redirectFrontend(frontend + "/auth/signin?error=session_failed"), nil
		}

		h.logger.InfoContext(ctx, "oauth callback: existing user login", "email", userProfile.Email, "provider", provider)
		return redirectFrontend(fmt.Sprintf("%s/auth/callback?session=%s&type=login", frontend, sessionID)), nil
	}

	// New user: store the profile in an OAuth session and redirect
	// to the signup-completion page so the user can pick a role +
	// organization name.
	session := &authDomain.OAuthSession{
		Email:           userProfile.Email,
		FirstName:       userProfile.FirstName,
		LastName:        userProfile.LastName,
		Provider:        userProfile.Provider,
		ProviderID:      userProfile.ProviderID,
		InvitationToken: invitationToken,
	}
	sessionID, err := h.authSvc.CreateOAuthSession(ctx, session)
	if err != nil {
		h.logger.ErrorContext(ctx, "oauth callback: OAuth session creation failed", "error", err)
		return redirectFrontend(frontend + "/auth/signin?error=session_creation_failed"), nil
	}

	h.logger.InfoContext(ctx, "oauth callback: new user signup session created", "email", userProfile.Email, "provider", provider)
	return redirectFrontend(fmt.Sprintf("%s/auth/signup?session=%s", frontend, sessionID)), nil
}

// redirectFrontend builds a 302 redirect output pointing at the
// supplied absolute URL. Temporary redirect (307 in the gin
// handler) would force GET → GET but browsers treat 302 identically
// for the OAuth callback flow; use 302 to match the broader web
// convention for post-callback redirects.
func redirectFrontend(url string) *OAuthCallbackOutput {
	return &OAuthCallbackOutput{Status: http.StatusFound, Location: url}
}

// ----- complete-oauth-signup ---------------------------------------

type CompleteOAuthSignupInput struct {
	Body completeOAuthSignupBody
}

type completeOAuthSignupBody struct {
	SessionID        string  `json:"session_id" minLength:"1" doc:"OAuth session ID returned by the callback redirect"`
	Role             string  `json:"role" enum:"engineer,product,designer,executive,other" doc:"Self-declared role"`
	OrganizationName *string `json:"organization_name,omitempty" doc:"Organization name — required for fresh signup, omitted when the session's invitation_token is present"`
	ReferralSource   *string `json:"referral_source,omitempty" doc:"Optional referral source for product analytics"`
}

type CompleteOAuthSignupOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      completeOAuthSignupResponse
}

// completeOAuthSignupResponse is the login-with-org shape. Matches
// the gin handler's responseData: user + organization + expiry in
// milliseconds.
type completeOAuthSignupResponse struct {
	User         any   `json:"user" doc:"Authenticated user object"`
	Organization any   `json:"organization" doc:"Organization created (fresh signup) or joined (invitation signup)"`
	ExpiresAt    int64 `json:"expires_at" doc:"Access-token expiry as Unix milliseconds"`
	ExpiresIn    int64 `json:"expires_in" doc:"Access-token TTL in milliseconds"`
}

func (h *handler) completeOAuthSignup(ctx context.Context, in *CompleteOAuthSignupInput) (*CompleteOAuthSignupOutput, error) {
	session, err := h.authSvc.GetOAuthSession(ctx, in.Body.SessionID)
	if err != nil {
		return nil, err
	}

	// Validate the required fields on the session; any missing
	// field indicates tampering or a protocol bug upstream.
	if session.Email == "" || session.FirstName == "" || session.LastName == "" ||
		session.Provider == "" || session.ProviderID == "" {
		h.logger.ErrorContext(ctx, "complete-oauth-signup: OAuth session missing required fields", "session_id", in.Body.SessionID)
		return nil, appErrors.NewValidationError("Invalid OAuth session", "session missing one or more required profile fields")
	}

	oauthReq := &registration.OAuthRegistrationRequest{
		Email:            session.Email,
		FirstName:        session.FirstName,
		LastName:         session.LastName,
		Role:             in.Body.Role,
		Provider:         session.Provider,
		ProviderID:       session.ProviderID,
		ReferralSource:   in.Body.ReferralSource,
		OrganizationName: in.Body.OrganizationName,
		InvitationToken:  session.InvitationToken,
	}

	regResp, err := h.regSvc.CompleteOAuthRegistration(ctx, oauthReq)
	if err != nil {
		h.logger.WarnContext(ctx, "complete-oauth-signup: registration failed", "email", session.Email, "error", err)
		return nil, err
	}

	// Clean up the OAuth session — best-effort, we don't fail the
	// signup if deletion errors.
	_ = h.authSvc.DeleteOAuthSession(ctx, in.Body.SessionID)

	u, err := h.userSvc.GetUserByEmail(ctx, session.Email)
	if err != nil {
		h.logger.ErrorContext(ctx, "complete-oauth-signup: user fetch failed after registration", "email", session.Email, "error", err)
		return nil, appErrors.NewInternalError("Failed to complete authentication", err)
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.ErrorContext(ctx, "complete-oauth-signup: CSRF token generation failed", "error", err)
		return nil, appErrors.NewInternalError("Authentication setup failed", err)
	}

	cookies := buildAuthCookies(
		regResp.LoginTokens.AccessToken,
		regResp.LoginTokens.RefreshToken,
		csrfToken,
		h.cfg.Server.CookieDomain,
	)

	expiresAt := time.Now().Add(time.Duration(regResp.LoginTokens.ExpiresIn) * time.Second)

	h.logger.InfoContext(ctx, "complete-oauth-signup successful", "email", session.Email, "provider", session.Provider)

	return &CompleteOAuthSignupOutput{
		SetCookie: cookies,
		Body: completeOAuthSignupResponse{
			User:         u,
			Organization: regResp.Organization,
			ExpiresAt:    expiresAt.UnixMilli(),
			ExpiresIn:    regResp.LoginTokens.ExpiresIn * 1000,
		},
	}, nil
}

// ----- exchange-login-session ---------------------------------------
//
// One-time exchange of a login-session ID (issued by the OAuth
// callback for an existing OAuth user) for the actual three
// httpOnly cookies. Reads the session from Redis and the service
// deletes it immediately (one-time use) to prevent replay.

type ExchangeLoginSessionInput struct {
	SessionID string `path:"session_id" doc:"One-time login session ID from the OAuth callback redirect"`
}

type ExchangeLoginSessionOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      loginResponse
}

func (h *handler) exchangeLoginSession(ctx context.Context, in *ExchangeLoginSessionInput) (*ExchangeLoginSessionOutput, error) {
	sessionData, err := h.authSvc.GetLoginTokenSession(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}

	if sessionData.AccessToken == "" || sessionData.RefreshToken == "" ||
		sessionData.ExpiresIn <= 0 || sessionData.UserID == uuid.Nil {
		h.logger.ErrorContext(ctx, "exchange-login-session: session missing fields", "session_id", in.SessionID)
		return nil, appErrors.NewValidationError("Invalid session data", "login session missing required fields")
	}

	u, err := h.userSvc.GetUser(ctx, sessionData.UserID)
	if err != nil {
		h.logger.ErrorContext(ctx, "exchange-login-session: user fetch failed", "user_id", sessionData.UserID, "error", err)
		return nil, appErrors.NewInternalError("Failed to complete authentication", err)
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.ErrorContext(ctx, "exchange-login-session: CSRF token generation failed", "error", err)
		return nil, appErrors.NewInternalError("Authentication setup failed", err)
	}

	cookies := buildAuthCookies(
		sessionData.AccessToken,
		sessionData.RefreshToken,
		csrfToken,
		h.cfg.Server.CookieDomain,
	)

	expiresAt := time.Now().Add(time.Duration(sessionData.ExpiresIn) * time.Second)

	h.logger.InfoContext(ctx, "exchange-login-session successful", "user_id", sessionData.UserID)

	return &ExchangeLoginSessionOutput{
		SetCookie: cookies,
		Body: loginResponse{
			User:      u,
			ExpiresAt: expiresAt.UnixMilli(),
			ExpiresIn: sessionData.ExpiresIn * 1000,
		},
	}, nil
}
