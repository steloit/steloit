// Package auth is the dashboard-plane authentication handler domain.
// It exposes login / logout / password-management / session-management
// operations as Huma v2 operations mounted on the apiAdmin surface.
//
// The package is the second vertical slice of the gin → huma handler
// conversion. It establishes the cookie-set / cookie-read / cookie-
// clear patterns every other dashboard domain reuses when it needs
// to touch auth cookies (which is rare — most domains just read the
// authenticated user via httpctx.MustGetUserID(ctx)).
//
// Operations currently registered (follow-up sessions add the rest):
//
//   - POST /api/v1/auth/login             — issue cookies, return user
//   - POST /api/v1/auth/logout            — blacklist JWT, clear cookies (authed)
//   - POST /api/v1/auth/refresh           — rotate cookies via refresh_token cookie
//   - POST /api/v1/auth/forgot-password   — email a reset token (user enumeration-safe)
//   - POST /api/v1/auth/reset-password    — exchange reset token for new password
//   - POST /api/v1/auth/change-password   — authenticated password change
//
// Not yet converted (follow-up sessions):
//
//   - signup / complete-oauth-signup / exchange-session
//   - /me (get current user) / profile get / profile update
//   - sessions list/get/revoke/revoke-all
//   - OAuth initiate / callback (Google, GitHub)
//   - POST /v1/auth/validate-key (SDK plane)
package auth

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"brokle/internal/config"
	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/core/domain/user"
	authService "brokle/internal/core/services/auth"
	"brokle/internal/core/services/registration"
	"brokle/internal/transport/http/httpctx"
	appErrors "brokle/pkg/errors"
)

// handler bundles every service an auth operation needs. Package-
// private; the only public surface is RegisterPublicRoutes /
// RegisterProtectedRoutes (dashboard plane). SDK-plane operations
// have their own lighter-weight sdkHandler in sdk.go.
type handler struct {
	authSvc       authDomain.AuthService
	userSvc       user.UserService
	profileSvc    user.ProfileService
	regSvc        registration.RegistrationService
	sessionSvc    authDomain.SessionService
	oauthProvider *authService.OAuthProviderService
	cfg           *config.Config
	logger        *slog.Logger
}

// ----- login ---------------------------------------------------------

// LoginInput is the POST /api/v1/auth/login request.
type LoginInput struct {
	Body loginBody
}

type loginBody struct {
	Email      string         `json:"email" format:"email" doc:"User email address"`
	Password   string         `json:"password" minLength:"1" doc:"User password"`
	DeviceInfo map[string]any `json:"device_info,omitempty" doc:"Optional device metadata for session tracking"`
}

// LoginOutput emits three Set-Cookie headers alongside the JSON
// body. Huma serialises the SetCookie slice into multiple
// Set-Cookie header lines.
type LoginOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      loginResponse
}

// loginResponse is the on-the-wire success shape. Tokens are NOT
// in the body — they ride the Set-Cookie headers. The dashboard
// uses expires_at / expires_in to schedule its own proactive
// refresh call so it doesn't wait for the next request to bounce
// off a 401.
type loginResponse struct {
	User      any   `json:"user" doc:"Authenticated user object"`
	ExpiresAt int64 `json:"expires_at" doc:"Access-token expiry as Unix milliseconds"`
	ExpiresIn int64 `json:"expires_in" doc:"Access-token TTL in milliseconds"`
}

func (h *handler) login(ctx context.Context, in *LoginInput) (*LoginOutput, error) {
	loginResp, err := h.authSvc.Login(ctx, &authDomain.LoginRequest{
		Email:      in.Body.Email,
		Password:   in.Body.Password,
		DeviceInfo: in.Body.DeviceInfo,
	})
	if err != nil {
		h.logger.WarnContext(ctx, "login failed", "email", in.Body.Email, "error", err)
		return nil, err
	}

	// Fetch user data BEFORE building cookies so a user-fetch
	// failure aborts authentication — avoids a half-authenticated
	// session where the client has valid cookies but no user.
	u, err := h.userSvc.GetUserByEmail(ctx, in.Body.Email)
	if err != nil {
		h.logger.ErrorContext(ctx, "login: user fetch failed after successful credentials", "email", in.Body.Email, "error", err)
		return nil, appErrors.NewInternalError("Failed to complete authentication", err)
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.ErrorContext(ctx, "login: CSRF token generation failed", "error", err)
		return nil, appErrors.NewInternalError("Authentication setup failed", err)
	}

	cookies := buildAuthCookies(
		loginResp.AccessToken,
		loginResp.RefreshToken,
		csrfToken,
		h.cfg.Server.CookieDomain,
	)

	h.logger.InfoContext(ctx, "login successful", "email", in.Body.Email)

	return &LoginOutput{
		SetCookie: cookies,
		Body:      expiryResponse(loginResp, u),
	}, nil
}

// ----- signup --------------------------------------------------------
//
// Fresh signup (creates a new organization) and invitation-based
// signup (joins an existing org via token) share this one endpoint.
// The service layer routes on the presence of InvitationToken. On
// success the handler issues the same three cookies as login so the
// user is authenticated immediately — no "check your email" step for
// the password flow.

type SignupInput struct {
	Body signupBody
}

// signupBody validates the "at least one of org-name / invitation-
// token must be provided" rule in the handler because Huma's
// declarative constraints don't express mutual-or-either
// requirements. The gin handler had the same check at auth.go:173.
type signupBody struct {
	Email            string  `json:"email" format:"email" doc:"User email address"`
	Password         string  `json:"password" minLength:"8" doc:"Password (minimum 8 characters)"`
	FirstName        string  `json:"first_name" minLength:"1" maxLength:"100" doc:"User first name"`
	LastName         string  `json:"last_name" minLength:"1" maxLength:"100" doc:"User last name"`
	Role             string  `json:"role" enum:"engineer,product,designer,executive,other" doc:"Self-declared role"`
	OrganizationName *string `json:"organization_name,omitempty" doc:"Organization name — required for fresh signup, omit when InvitationToken is present"`
	InvitationToken  *string `json:"invitation_token,omitempty" doc:"Invitation token — when present, joins the existing organization instead of creating one"`
	ReferralSource   *string `json:"referral_source,omitempty" doc:"Optional referral source for product analytics"`
}

type SignupOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      loginResponse
}

func (h *handler) signup(ctx context.Context, in *SignupInput) (*SignupOutput, error) {
	if in.Body.InvitationToken == nil && in.Body.OrganizationName == nil {
		return nil, appErrors.NewValidationError(
			"Signup requires either organization_name or invitation_token",
			"Provide organization_name for a fresh signup, or invitation_token to join an existing organization",
		)
	}

	regReq := &registration.RegisterRequest{
		Email:            in.Body.Email,
		Password:         in.Body.Password,
		FirstName:        in.Body.FirstName,
		LastName:         in.Body.LastName,
		Role:             in.Body.Role,
		ReferralSource:   in.Body.ReferralSource,
		OrganizationName: in.Body.OrganizationName,
		InvitationToken:  in.Body.InvitationToken,
		IsOAuthUser:      false,
	}

	var (
		regResp *registration.RegistrationResponse
		err     error
	)
	if in.Body.InvitationToken != nil {
		regResp, err = h.regSvc.RegisterWithInvitation(ctx, regReq)
	} else {
		regResp, err = h.regSvc.RegisterWithOrganization(ctx, regReq)
	}
	if err != nil {
		h.logger.WarnContext(ctx, "signup failed", "email", in.Body.Email, "error", err)
		return nil, err
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.ErrorContext(ctx, "signup: CSRF token generation failed", "error", err)
		return nil, appErrors.NewInternalError("Authentication setup failed", err)
	}

	cookies := buildAuthCookies(
		regResp.LoginTokens.AccessToken,
		regResp.LoginTokens.RefreshToken,
		csrfToken,
		h.cfg.Server.CookieDomain,
	)

	h.logger.InfoContext(ctx, "signup successful", "email", in.Body.Email, "user_id", regResp.User.ID)

	return &SignupOutput{
		SetCookie: cookies,
		Body:      expiryResponse(regResp.LoginTokens, regResp.User),
	}, nil
}

// ----- get-current-user (/me) ---------------------------------------
//
// Returns the authenticated user object plus access-token expiry
// metadata so the dashboard can schedule a proactive refresh before
// the next request bounces off a 401. This is the most-called
// dashboard endpoint.

type GetCurrentUserOutput struct {
	Body loginResponse
}

func (h *handler) getCurrentUser(ctx context.Context, _ *struct{}) (*GetCurrentUserOutput, error) {
	userID := httpctx.MustGetUserID(ctx)
	claims := httpctx.MustGetTokenClaims(ctx)

	u, err := h.userSvc.GetUser(ctx, userID)
	if err != nil {
		h.logger.WarnContext(ctx, "get-current-user: user fetch failed", "user_id", userID, "error", err)
		return nil, err
	}

	// Expiry math: JWT exp is Unix seconds; the response uses
	// milliseconds to match the format login/refresh emit, so the
	// dashboard's schedule code has one consistent timestamp type
	// to consume.
	expiresAtMs := claims.ExpiresAt * 1000
	expiresInMs := expiresAtMs - time.Now().UnixMilli()

	return &GetCurrentUserOutput{
		Body: loginResponse{
			User:      u,
			ExpiresAt: expiresAtMs,
			ExpiresIn: expiresInMs,
		},
	}, nil
}

// ----- get-profile ---------------------------------------------------
//
// Returns the authenticated user record. Distinct from
// get-current-user (/me) because /me includes access-token expiry
// metadata the dashboard uses for refresh scheduling, while
// /profile is the "user record for rendering a profile page"
// endpoint — no session metadata, just the persistent fields.

type GetProfileOutput struct {
	Body any `json:"user" doc:"Authenticated user profile record"`
}

func (h *handler) getProfile(ctx context.Context, _ *struct{}) (*GetProfileOutput, error) {
	userID := httpctx.MustGetUserID(ctx)
	u, err := h.userSvc.GetUser(ctx, userID)
	if err != nil {
		h.logger.WarnContext(ctx, "get-profile: user fetch failed", "user_id", userID, "error", err)
		return nil, err
	}
	return &GetProfileOutput{Body: u}, nil
}

// ----- update-profile -----------------------------------------------

type UpdateProfileInput struct {
	Body updateProfileBody
}

// updateProfileBody mirrors user.UpdateProfileRequest. All fields
// are pointer-typed because a missing field means "leave this
// field alone" rather than "clear this field" — only fields the
// client explicitly sets are updated. The body covers profile-
// record fields (bio, location, social URLs, preferences); name/
// email changes flow through a separate user-update endpoint
// (not yet converted).
type updateProfileBody struct {
	Bio         *string `json:"bio,omitempty" maxLength:"500" doc:"Free-form profile bio"`
	Location    *string `json:"location,omitempty" maxLength:"100" doc:"City / country string"`
	Website     *string `json:"website,omitempty" format:"uri" doc:"Personal website URL"`
	TwitterURL  *string `json:"twitter_url,omitempty" format:"uri" doc:"Twitter / X profile URL"`
	LinkedInURL *string `json:"linkedin_url,omitempty" format:"uri" doc:"LinkedIn profile URL"`
	GitHubURL   *string `json:"github_url,omitempty" format:"uri" doc:"GitHub profile URL"`
	AvatarURL   *string `json:"avatar_url,omitempty" format:"uri" doc:"Avatar image URL"`
	Phone       *string `json:"phone,omitempty" maxLength:"50" doc:"Phone number"`
	Timezone    *string `json:"timezone,omitempty" doc:"IANA timezone identifier (e.g. 'America/New_York')"`
	Language    *string `json:"language,omitempty" minLength:"2" maxLength:"2" doc:"ISO 639-1 two-letter language code"`
	Theme       *string `json:"theme,omitempty" enum:"light,dark,auto" doc:"Dashboard colour theme preference"`

	EmailNotifications    *bool `json:"email_notifications,omitempty" doc:"Receive email notifications"`
	PushNotifications     *bool `json:"push_notifications,omitempty" doc:"Receive push notifications"`
	MarketingEmails       *bool `json:"marketing_emails,omitempty" doc:"Receive product-marketing emails"`
	WeeklyReports         *bool `json:"weekly_reports,omitempty" doc:"Receive weekly usage reports"`
	MonthlyReports        *bool `json:"monthly_reports,omitempty" doc:"Receive monthly usage reports"`
	SecurityAlerts        *bool `json:"security_alerts,omitempty" doc:"Receive security alerts"`
	BillingAlerts         *bool `json:"billing_alerts,omitempty" doc:"Receive billing alerts"`
	UsageThresholdPercent *int  `json:"usage_threshold_percent,omitempty" minimum:"0" maximum:"100" doc:"Percent-of-quota threshold that triggers a usage alert"`
}

type UpdateProfileOutput struct {
	Body any `json:"profile" doc:"Updated user profile record"`
}

func (h *handler) updateProfile(ctx context.Context, in *UpdateProfileInput) (*UpdateProfileOutput, error) {
	userID := httpctx.MustGetUserID(ctx)

	req := &user.UpdateProfileRequest{
		Bio:         in.Body.Bio,
		Location:    in.Body.Location,
		Website:     in.Body.Website,
		TwitterURL:  in.Body.TwitterURL,
		LinkedInURL: in.Body.LinkedInURL,
		GitHubURL:   in.Body.GitHubURL,
		AvatarURL:   in.Body.AvatarURL,
		Phone:       in.Body.Phone,
		Timezone:    in.Body.Timezone,
		Language:    in.Body.Language,
		Theme:       in.Body.Theme,

		EmailNotifications:    in.Body.EmailNotifications,
		PushNotifications:     in.Body.PushNotifications,
		MarketingEmails:       in.Body.MarketingEmails,
		WeeklyReports:         in.Body.WeeklyReports,
		MonthlyReports:        in.Body.MonthlyReports,
		SecurityAlerts:        in.Body.SecurityAlerts,
		BillingAlerts:         in.Body.BillingAlerts,
		UsageThresholdPercent: in.Body.UsageThresholdPercent,
	}

	profile, err := h.profileSvc.UpdateProfile(ctx, userID, req)
	if err != nil {
		h.logger.WarnContext(ctx, "update-profile failed", "user_id", userID, "error", err)
		return nil, err
	}

	return &UpdateProfileOutput{Body: profile}, nil
}

// ----- sessions list -------------------------------------------------

type ListSessionsOutput struct {
	Body listSessionsResponse
}

type listSessionsResponse struct {
	Sessions []*authDomain.UserSession `json:"sessions" doc:"All sessions owned by the authenticated user"`
}

func (h *handler) listSessions(ctx context.Context, _ *struct{}) (*ListSessionsOutput, error) {
	userID := httpctx.MustGetUserID(ctx)
	sessions, err := h.sessionSvc.GetUserSessions(ctx, userID)
	if err != nil {
		h.logger.WarnContext(ctx, "list-sessions: fetch failed", "user_id", userID, "error", err)
		return nil, err
	}
	return &ListSessionsOutput{Body: listSessionsResponse{Sessions: sessions}}, nil
}

// ----- session get ---------------------------------------------------

type GetSessionInput struct {
	SessionID uuid.UUID `path:"session_id" doc:"Session identifier"`
}

type GetSessionOutput struct {
	Body *authDomain.UserSession
}

func (h *handler) getSession(ctx context.Context, in *GetSessionInput) (*GetSessionOutput, error) {
	// The session service returns the session regardless of owner;
	// enforce owner-match at the handler layer so a compromised or
	// predicted UUID can't read another user's session.
	userID := httpctx.MustGetUserID(ctx)
	sess, err := h.sessionSvc.GetSession(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	if sess.UserID != userID {
		h.logger.WarnContext(ctx, "get-session: cross-user access attempt", "actor", userID, "session_id", in.SessionID, "owner", sess.UserID)
		return nil, appErrors.NewNotFoundError("Session")
	}
	return &GetSessionOutput{Body: sess}, nil
}

// ----- session revoke -----------------------------------------------
//
// Revokes a single session by ID. Owner-enforcement mirrors
// get-session — a user can only revoke their own sessions.

type RevokeSessionInput struct {
	SessionID uuid.UUID `path:"session_id" doc:"Session identifier to revoke"`
}

type RevokeSessionOutput struct {
	Body messageResponse
}

func (h *handler) revokeSession(ctx context.Context, in *RevokeSessionInput) (*RevokeSessionOutput, error) {
	userID := httpctx.MustGetUserID(ctx)
	sess, err := h.sessionSvc.GetSession(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	if sess.UserID != userID {
		h.logger.WarnContext(ctx, "revoke-session: cross-user access attempt", "actor", userID, "session_id", in.SessionID, "owner", sess.UserID)
		return nil, appErrors.NewNotFoundError("Session")
	}
	if err := h.sessionSvc.RevokeSession(ctx, in.SessionID); err != nil {
		h.logger.WarnContext(ctx, "revoke-session: failed", "user_id", userID, "session_id", in.SessionID, "error", err)
		return nil, err
	}
	h.logger.InfoContext(ctx, "session revoked", "user_id", userID, "session_id", in.SessionID)
	return &RevokeSessionOutput{Body: messageResponse{Message: "Session revoked successfully"}}, nil
}

// ----- sessions revoke-all ------------------------------------------
//
// Bulk revokes every session the authenticated user owns AND
// clears the caller's own auth cookies so the current browser
// drops its client-side session state alongside the server-side
// invalidation. GDPR/SOC2 "log me out everywhere" flow — the
// backend writes a user-wide timestamp blacklist internally so all
// tokens issued before this call are rejected even if they haven't
// hit the per-JTI blacklist yet.

type RevokeAllSessionsOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      messageResponse
}

func (h *handler) revokeAllSessions(ctx context.Context, _ *struct{}) (*RevokeAllSessionsOutput, error) {
	userID := httpctx.MustGetUserID(ctx)
	if err := h.authSvc.RevokeAllSessions(ctx, userID); err != nil {
		h.logger.WarnContext(ctx, "revoke-all-sessions: failed", "user_id", userID, "error", err)
		return nil, err
	}
	h.logger.InfoContext(ctx, "all sessions revoked", "user_id", userID)
	return &RevokeAllSessionsOutput{
		SetCookie: buildClearAuthCookies(h.cfg.Server.CookieDomain),
		Body:      messageResponse{Message: "All sessions revoked successfully"},
	}, nil
}

// ----- logout --------------------------------------------------------

// LogoutOutput returns three cleared cookies + a confirmation body.
type LogoutOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      messageResponse
}

func (h *handler) logout(ctx context.Context, _ *struct{}) (*LogoutOutput, error) {
	// Must* — the route registers RequireAuth upstream, so a
	// misconfiguration panics into the recoverer rather than
	// silently returning 401 (CLAUDE.md gotcha #6).
	claims := httpctx.MustGetTokenClaims(ctx)

	if err := h.authSvc.Logout(ctx, claims.JWTID, claims.UserID); err != nil {
		h.logger.ErrorContext(ctx, "logout: blacklist failed", "jti", claims.JWTID, "user_id", claims.UserID, "error", err)
		return nil, err
	}

	h.logger.InfoContext(ctx, "logout successful", "user_id", claims.UserID)

	return &LogoutOutput{
		SetCookie: buildClearAuthCookies(h.cfg.Server.CookieDomain),
		Body:      messageResponse{Message: "Logged out successfully"},
	}, nil
}

// ----- refresh-tokens -----------------------------------------------

// RefreshInput reads the refresh_token cookie Huma sees as a
// request header. `cookie:"refresh_token"` is Huma's supported
// input tag for single-cookie extraction.
type RefreshInput struct {
	RefreshToken string `cookie:"refresh_token" doc:"httpOnly refresh token issued by a prior login/signup/refresh"`
}

// RefreshOutput carries the rotated cookies + expiry metadata.
// User data is NOT returned — the dashboard keeps the user object
// from login/signup and only needs the new expiry values to
// schedule the next refresh. The /me endpoint (not yet converted)
// re-fetches the user when needed.
type RefreshOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      refreshResponse
}

type refreshResponse struct {
	ExpiresAt int64 `json:"expires_at" doc:"New access-token expiry as Unix milliseconds"`
	ExpiresIn int64 `json:"expires_in" doc:"New access-token TTL in milliseconds"`
}

func (h *handler) refresh(ctx context.Context, in *RefreshInput) (*RefreshOutput, error) {
	if in.RefreshToken == "" {
		// No cookie — surface as 401 and clear any stale cookies
		// the client might still hold. Huma doesn't support "return
		// an error AND set Set-Cookie", so the clear path lives on
		// the response-shaping side via the wrapped error type
		// below.
		h.logger.WarnContext(ctx, "refresh: missing refresh_token cookie")
		return nil, newAuthClearError(h.cfg.Server.CookieDomain,
			appErrors.NewUnauthorizedError("Refresh token not found"))
	}

	loginResp, err := h.authSvc.RefreshToken(ctx, &authDomain.RefreshTokenRequest{
		RefreshToken: in.RefreshToken,
	})
	if err != nil {
		h.logger.WarnContext(ctx, "refresh: token validation failed", "error", err)
		return nil, newAuthClearError(h.cfg.Server.CookieDomain,
			appErrors.NewUnauthorizedError("Refresh token invalid or expired"))
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.ErrorContext(ctx, "refresh: CSRF token generation failed", "error", err)
		return nil, newAuthClearError(h.cfg.Server.CookieDomain,
			appErrors.NewInternalError("Token refresh setup failed", err))
	}

	cookies := buildAuthCookies(
		loginResp.AccessToken,
		loginResp.RefreshToken,
		csrfToken,
		h.cfg.Server.CookieDomain,
	)

	h.logger.InfoContext(ctx, "refresh successful")

	expiresAt := time.Now().Add(time.Duration(loginResp.ExpiresIn) * time.Second)
	return &RefreshOutput{
		SetCookie: cookies,
		Body: refreshResponse{
			ExpiresAt: expiresAt.UnixMilli(),
			ExpiresIn: loginResp.ExpiresIn * 1000,
		},
	}, nil
}

// ----- forgot-password ----------------------------------------------

type ForgotPasswordInput struct {
	Body forgotPasswordBody
}

type forgotPasswordBody struct {
	Email string `json:"email" format:"email" doc:"Email address for password reset"`
}

type ForgotPasswordOutput struct {
	Body messageResponse
}

func (h *handler) forgotPassword(ctx context.Context, in *ForgotPasswordInput) (*ForgotPasswordOutput, error) {
	// Intentionally ignore the error — we return the same response
	// whether the email is registered or not to prevent account
	// enumeration via the forgot-password form. The authService
	// logs internally.
	if err := h.authSvc.ResetPassword(ctx, in.Body.Email); err != nil {
		h.logger.WarnContext(ctx, "forgot-password: reset initiation failed (swallowed)", "email", in.Body.Email, "error", err)
	}

	h.logger.InfoContext(ctx, "forgot-password requested", "email", in.Body.Email)

	return &ForgotPasswordOutput{
		Body: messageResponse{Message: "If the email exists, a password reset link has been sent"},
	}, nil
}

// ----- reset-password -----------------------------------------------
//
// Two-step flow: (1) forgot-password emails a token, (2) this
// endpoint exchanges the token + new password for an updated user
// credential row. No cookies set here — the user logs in again
// after reset so device-info / session tracking is preserved.

type ResetPasswordInput struct {
	Body resetPasswordBody
}

type resetPasswordBody struct {
	Token       string `json:"token" minLength:"1" doc:"Password-reset token delivered via email"`
	NewPassword string `json:"new_password" minLength:"8" doc:"New password (minimum 8 characters)"`
}

type ResetPasswordOutput struct {
	Body messageResponse
}

func (h *handler) resetPassword(ctx context.Context, in *ResetPasswordInput) (*ResetPasswordOutput, error) {
	// The current auth.Service interface exposes ResetPassword as
	// a single-argument (email) "send email" flow; the actual
	// token-exchange happens inside the service layer's password
	// reset completion routine. For the chi+huma migration we
	// surface it as NotImplemented until the service layer grows
	// the two-step API explicitly — the gin handler had the same
	// gap (it called h.authService.ResetPassword(ctx, req.Email)
	// with `req.Email` NEVER set — effectively a no-op). Log and
	// return 501 so the dashboard surfaces the gap instead of
	// silently "succeeding".
	h.logger.WarnContext(ctx, "reset-password endpoint hit — service-layer completion not yet implemented", "token_prefix", in.Body.Token[:min(len(in.Body.Token), 6)])
	return nil, appErrors.NewNotImplementedError("Password reset completion is not yet implemented in the service layer")
}

// ----- change-password ----------------------------------------------

type ChangePasswordInput struct {
	Body changePasswordBody
}

type changePasswordBody struct {
	CurrentPassword string `json:"current_password" minLength:"1" doc:"Current password — re-auth step"`
	NewPassword     string `json:"new_password" minLength:"8" doc:"New password (minimum 8 characters)"`
}

type ChangePasswordOutput struct {
	Body messageResponse
}

func (h *handler) changePassword(ctx context.Context, in *ChangePasswordInput) (*ChangePasswordOutput, error) {
	userID := httpctx.MustGetUserID(ctx)

	if err := h.userSvc.ChangePassword(ctx, userID, in.Body.CurrentPassword, in.Body.NewPassword); err != nil {
		h.logger.WarnContext(ctx, "change-password failed", "user_id", userID, "error", err)
		return nil, err
	}

	h.logger.InfoContext(ctx, "change-password successful", "user_id", userID)

	return &ChangePasswordOutput{
		Body: messageResponse{Message: "Password changed successfully"},
	}, nil
}

// ----- shared helpers ------------------------------------------------

// messageResponse is the flat {"message": "..."} shape every
// action-confirmation endpoint returns. Kept private — every
// Output type embeds it explicitly rather than exposing it as a
// shared response envelope, because Huma's OpenAPI emitter
// generates a cleaner schema per-operation when the types are
// literal.
type messageResponse struct {
	Message string `json:"message" doc:"Human-readable outcome"`
}

// expiryResponse converts an authService.LoginResponse + user
// object into the loginResponse body shape. Extracted so login
// and any future endpoint that returns the same body (e.g.
// complete-oauth-signup when it lands) reuse one expiry-math
// site — drift between the two has bitten the gin handler
// twice (see git blame for the old expires_at / expires_in
// field tweaks).
func expiryResponse(loginResp *authDomain.LoginResponse, u any) loginResponse {
	expiresAt := time.Now().Add(time.Duration(loginResp.ExpiresIn) * time.Second)
	return loginResponse{
		User:      u,
		ExpiresAt: expiresAt.UnixMilli(),
		ExpiresIn: loginResp.ExpiresIn * 1000,
	}
}

// authClearError wraps an AppError with a "clear cookies on the
// response" intent. Huma's default error pipeline only emits the
// JSON envelope — it doesn't know about our cookie-clear path.
// The huma.NewError override in internal/server/api_error.go
// detects *authClearError and adds the Set-Cookie clear headers
// to the outgoing response before writing the body.
//
// (The override lives in api_error.go rather than here so the
// server package owns the full error-rendering pipeline. This
// type is defined here because only the auth package raises it.)
type authClearError struct {
	err    *appErrors.AppError
	domain string
}

func newAuthClearError(domain string, err *appErrors.AppError) *authClearError {
	return &authClearError{err: err, domain: domain}
}

func (e *authClearError) Error() string    { return e.err.Error() }
func (e *authClearError) Unwrap() error    { return e.err }
func (e *authClearError) GetStatus() int   { return e.err.HTTPStatus() }
func (e *authClearError) AppError() *appErrors.AppError { return e.err }
func (e *authClearError) Cookies() []http.Cookie {
	return buildClearAuthCookies(e.domain)
}

// Silence unused-import warnings for uuid — left in for the
// follow-up operations (session management, etc.) that will need
// it.
var _ = uuid.Nil
