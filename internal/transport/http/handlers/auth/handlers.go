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
	"brokle/internal/transport/http/httpctx"
	appErrors "brokle/pkg/errors"
)

// handler bundles the auth service + user service + config + logger
// so the operation methods don't repeat them on every signature.
// Package-private; the only public surface is
// RegisterPublicRoutes / RegisterProtectedRoutes.
type handler struct {
	authSvc authDomain.AuthService
	userSvc user.UserService
	cfg     *config.Config
	logger  *slog.Logger
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
