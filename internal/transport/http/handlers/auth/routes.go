package auth

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"brokle/internal/config"
	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/core/domain/user"
	authService "brokle/internal/core/services/auth"
	"brokle/internal/core/services/registration"
)

// PublicDeps bundles every dependency the public dashboard auth
// operations need. Grouping them in a struct keeps the
// RegisterPublicRoutes signature stable as we add more operations
// that need additional services — and keeps the call site in
// server/routes.go readable.
type PublicDeps struct {
	Auth          authDomain.AuthService
	User          user.UserService
	Registration  registration.RegistrationService
	Session       authDomain.SessionService
	OAuthProvider *authService.OAuthProviderService
	Config        *config.Config
	Logger        *slog.Logger
}

// ProtectedDeps mirrors PublicDeps for the authenticated routes.
// Carries the profile service in addition to the common set;
// public operations never touch profile data.
type ProtectedDeps struct {
	Auth          authDomain.AuthService
	User          user.UserService
	Profile       user.ProfileService
	Registration  registration.RegistrationService
	Session       authDomain.SessionService
	OAuthProvider *authService.OAuthProviderService
	Config        *config.Config
	Logger        *slog.Logger
}

// RegisterPublicRoutes registers every unauthenticated auth
// operation on the supplied huma.API. Mount against apiAdmin (the
// dashboard OpenAPI surface) — these endpoints live under
// /api/v1/auth/* and carry no SDK semantics.
//
// The handler struct is built once per call; RegisterPublicRoutes
// is called exactly once at server startup (see
// internal/server/routes.go).
func RegisterPublicRoutes(api huma.API, d PublicDeps) {
	h := &handler{
		authSvc:       d.Auth,
		userSvc:       d.User,
		regSvc:        d.Registration,
		sessionSvc:    d.Session,
		oauthProvider: d.OAuthProvider,
		cfg:           d.Config,
		logger:        d.Logger,
	}

	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/login",
		Tags:        []string{"auth"},
		Summary:     "Authenticate a user and set session cookies",
		Description: "Exchanges email + password for three httpOnly cookies " +
			"(access_token, refresh_token, csrf_token) and returns the user " +
			"object plus expiry metadata in milliseconds. The tokens do NOT " +
			"appear in the JSON body — they ride the Set-Cookie header.",
	}, h.login)

	huma.Register(api, huma.Operation{
		OperationID:   "signup",
		Method:        http.MethodPost,
		Path:          "/api/v1/auth/signup",
		Tags:          []string{"auth"},
		Summary:       "Register a new user and set session cookies",
		Description:   "Creates a new user with either a new organization (when organization_name is set) or by joining an existing organization via invitation_token. Sets the same three cookies as login on success.",
		DefaultStatus: http.StatusCreated,
	}, h.signup)

	huma.Register(api, huma.Operation{
		OperationID: "refresh-tokens",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/refresh",
		Tags:        []string{"auth"},
		Summary:     "Rotate session cookies via the refresh_token cookie",
		Description: "Reads the refresh_token httpOnly cookie, validates it, " +
			"issues a new access/refresh/csrf triplet, and returns the new " +
			"expiry metadata. Old cookies are cleared on failure so the " +
			"client stops retrying against stale state.",
	}, h.refresh)

	huma.Register(api, huma.Operation{
		OperationID: "forgot-password",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/forgot-password",
		Tags:        []string{"auth"},
		Summary:     "Initiate a password-reset email",
		Description: "Always returns the same success response whether or not " +
			"the email is registered — prevents account enumeration via the " +
			"reset form.",
	}, h.forgotPassword)

	huma.Register(api, huma.Operation{
		OperationID: "reset-password",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/reset-password",
		Tags:        []string{"auth"},
		Summary:     "Complete a password reset with the token from email",
		Description: "Currently returns 501 — the service-layer token-exchange " +
			"flow has not yet been implemented (the gin handler had the same " +
			"gap). Tracked as a follow-up for the auth service.",
	}, h.resetPassword)

	huma.Register(api, huma.Operation{
		OperationID:   "initiate-google-oauth",
		Method:        http.MethodGet,
		Path:          "/api/v1/auth/google",
		Tags:          []string{"auth", "oauth"},
		Summary:       "Begin Google OAuth flow",
		Description:   "Generates a CSRF state token and redirects the browser to Google's consent screen. Optional invitation_token threads an invitation through the post-consent callback so OAuth signup can join an existing organization.",
		DefaultStatus: http.StatusTemporaryRedirect,
	}, h.initiateGoogleOAuth)

	huma.Register(api, huma.Operation{
		OperationID:   "google-oauth-callback",
		Method:        http.MethodGet,
		Path:          "/api/v1/auth/google/callback",
		Tags:          []string{"auth", "oauth"},
		Summary:       "Google OAuth callback",
		Description:   "Validates state, exchanges code for a profile, and redirects to the frontend — to /auth/callback?session=… for existing OAuth users, to /auth/signup?session=… for new users, or to /auth/signin?error=… on any failure. All exit paths are 302 redirects; errors become query parameters so the browser-landed page can render them.",
		DefaultStatus: http.StatusFound,
	}, h.googleOAuthCallback)

	huma.Register(api, huma.Operation{
		OperationID:   "initiate-github-oauth",
		Method:        http.MethodGet,
		Path:          "/api/v1/auth/github",
		Tags:          []string{"auth", "oauth"},
		Summary:       "Begin GitHub OAuth flow",
		Description:   "Mirrors initiate-google-oauth but for GitHub. See that endpoint for flow details.",
		DefaultStatus: http.StatusTemporaryRedirect,
	}, h.initiateGithubOAuth)

	huma.Register(api, huma.Operation{
		OperationID:   "github-oauth-callback",
		Method:        http.MethodGet,
		Path:          "/api/v1/auth/github/callback",
		Tags:          []string{"auth", "oauth"},
		Summary:       "GitHub OAuth callback",
		Description:   "Mirrors google-oauth-callback but for GitHub. Same three exit redirects (callback / signup / signin?error).",
		DefaultStatus: http.StatusFound,
	}, h.githubOAuthCallback)

	huma.Register(api, huma.Operation{
		OperationID:   "complete-oauth-signup",
		Method:        http.MethodPost,
		Path:          "/api/v1/auth/complete-oauth-signup",
		Tags:          []string{"auth", "oauth"},
		Summary:       "Complete OAuth signup with organization + role",
		Description:   "Second step of OAuth signup for new users. Reads the OAuth session created by the callback, combines it with the client-supplied organization_name (or honours the session's invitation_token), and issues auth cookies on success.",
		DefaultStatus: http.StatusCreated,
	}, h.completeOAuthSignup)

	huma.Register(api, huma.Operation{
		OperationID: "exchange-login-session",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/exchange-session/{session_id}",
		Tags:        []string{"auth", "oauth"},
		Summary:     "Exchange an OAuth-login session for auth cookies",
		Description: "Frontend's post-OAuth-login step for existing users. Exchanges a one-time session id (issued by the OAuth callback) for the three auth cookies. Sessions are deleted on read to prevent replay.",
	}, h.exchangeLoginSession)
}

// RegisterProtectedRoutes registers every auth operation that
// requires a valid session. The caller is expected to mount this
// inside a chi.Router group that already has RequireAuth applied —
// the Security declaration on each operation is documentation only,
// enforcement lives in middleware.
func RegisterProtectedRoutes(api huma.API, d ProtectedDeps) {
	h := &handler{
		authSvc:       d.Auth,
		userSvc:       d.User,
		profileSvc:    d.Profile,
		regSvc:        d.Registration,
		sessionSvc:    d.Session,
		oauthProvider: d.OAuthProvider,
		cfg:           d.Config,
		logger:        d.Logger,
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-current-user",
		Method:      http.MethodGet,
		Path:        "/api/v1/auth/me",
		Tags:        []string{"auth"},
		Summary:     "Get the currently authenticated user",
		Description: "Returns the authenticated user object plus access-token expiry metadata in milliseconds. The dashboard uses the expiry values to schedule a proactive refresh before the next request bounces off a 401.",
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.getCurrentUser)

	huma.Register(api, huma.Operation{
		OperationID: "logout",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/logout",
		Tags:        []string{"auth"},
		Summary:     "Invalidate the current session and clear cookies",
		Description: "Blacklists the JWT jti, clears the three auth cookies, " +
			"and returns a confirmation message. CSRF double-submit is " +
			"enforced by the stdlib CrossOriginProtection middleware on the " +
			"/api/v1/* route group.",
		Security: []map[string][]string{{"bearerAuth": {}}},
	}, h.logout)

	huma.Register(api, huma.Operation{
		OperationID: "change-password",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/change-password",
		Tags:        []string{"auth"},
		Summary:     "Change the current user's password",
		Description: "Requires the current password as a re-auth step. The " +
			"new password must be at least 8 characters. Session cookies " +
			"are NOT rotated — the new password takes effect on next login.",
		Security: []map[string][]string{{"bearerAuth": {}}},
	}, h.changePassword)

	huma.Register(api, huma.Operation{
		OperationID: "get-profile",
		Method:      http.MethodGet,
		Path:        "/api/v1/auth/profile",
		Tags:        []string{"auth"},
		Summary:     "Get the authenticated user's profile",
		Description: "Returns the persistent user record without session metadata. For the refresh-aware /me variant, use get-current-user.",
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.getProfile)

	huma.Register(api, huma.Operation{
		OperationID: "update-profile",
		Method:      http.MethodPatch,
		Path:        "/api/v1/auth/profile",
		Tags:        []string{"auth"},
		Summary:     "Update the authenticated user's profile",
		Description: "Partial update — every field is optional. Missing fields keep their current value; explicitly null would clear them but the JSON schema marks all fields as optional so 'missing' is the idiomatic 'don't touch'. Name and email changes flow through a separate user-update endpoint, not yet converted.",
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.updateProfile)

	huma.Register(api, huma.Operation{
		OperationID: "list-sessions",
		Method:      http.MethodGet,
		Path:        "/api/v1/auth/sessions",
		Tags:        []string{"auth"},
		Summary:     "List all sessions owned by the authenticated user",
		Description: "Returns every UserSession row for the current user. The dashboard renders this list in the security settings page so the user can review and revoke individual sessions.",
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.listSessions)

	huma.Register(api, huma.Operation{
		OperationID: "get-session",
		Method:      http.MethodGet,
		Path:        "/api/v1/auth/sessions/{session_id}",
		Tags:        []string{"auth"},
		Summary:     "Get one session by ID",
		Description: "Owner-enforced — returns 404 when the session belongs to a different user so a predicted UUID can't reveal another user's session metadata.",
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.getSession)

	huma.Register(api, huma.Operation{
		OperationID: "revoke-session",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/sessions/{session_id}/revoke",
		Tags:        []string{"auth"},
		Summary:     "Revoke a single session by ID",
		Description: "Owner-enforced. Revoking the caller's current session via this endpoint leaves the caller's cookies intact — use logout or revoke-all-sessions for a self-logout flow.",
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.revokeSession)

	huma.Register(api, huma.Operation{
		OperationID: "revoke-all-sessions",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/sessions/revoke-all",
		Tags:        []string{"auth"},
		Summary:     "Revoke every session the caller owns and clear their cookies",
		Description: "GDPR/SOC2 'log me out everywhere' flow. Writes a user-wide timestamp blacklist on the server so tokens issued before the call are rejected even if their jti hasn't been per-token-blacklisted yet. Clears the caller's three auth cookies in the response.",
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.revokeAllSessions)
}
