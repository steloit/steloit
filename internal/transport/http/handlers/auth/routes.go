package auth

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"brokle/internal/config"
	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/core/domain/user"
)

// RegisterPublicRoutes registers every unauthenticated auth
// operation on the supplied huma.API. Mount against apiAdmin (the
// dashboard OpenAPI surface) — these endpoints live under
// /api/v1/auth/* and carry no SDK semantics.
//
// The handler struct is built once per call; RegisterPublicRoutes
// is called exactly once at server startup (see
// internal/server/routes.go).
func RegisterPublicRoutes(
	api huma.API,
	authSvc authDomain.AuthService,
	userSvc user.UserService,
	cfg *config.Config,
	logger *slog.Logger,
) {
	h := &handler{authSvc: authSvc, userSvc: userSvc, cfg: cfg, logger: logger}

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
}

// RegisterProtectedRoutes registers every auth operation that
// requires a valid session. The caller is expected to mount this
// inside a chi.Router group that already has RequireAuth applied —
// the Security declaration on each operation is documentation only,
// enforcement lives in middleware.
func RegisterProtectedRoutes(
	api huma.API,
	authSvc authDomain.AuthService,
	userSvc user.UserService,
	cfg *config.Config,
	logger *slog.Logger,
) {
	h := &handler{authSvc: authSvc, userSvc: userSvc, cfg: cfg, logger: logger}

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
}
