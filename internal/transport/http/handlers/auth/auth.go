package auth

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/auth"
	"brokle/internal/core/domain/user"
	authService "brokle/internal/core/services/auth"
	"brokle/internal/core/services/registration"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// Handler handles authentication endpoints
type Handler struct {
	config              *config.Config
	logger              *slog.Logger
	authService         auth.AuthService
	apiKeyService       auth.APIKeyService
	userService         user.UserService
	registrationService registration.RegistrationService
	oauthProvider       *authService.OAuthProviderService
}

// NewHandler creates a new auth handler
func NewHandler(
	config *config.Config,
	logger *slog.Logger,
	authService auth.AuthService,
	apiKeyService auth.APIKeyService,
	userService user.UserService,
	registrationService registration.RegistrationService,
	oauthProvider *authService.OAuthProviderService,
) *Handler {
	return &Handler{
		config:              config,
		logger:              logger,
		authService:         authService,
		apiKeyService:       apiKeyService,
		userService:         userService,
		registrationService: registrationService,
		oauthProvider:       oauthProvider,
	}
}

// LoginRequest represents the login request payload
// @Description User login credentials
type LoginRequest struct {
	DeviceInfo map[string]interface{} `json:"device_info,omitempty" description:"Device information for session tracking"`
	Email      string                 `json:"email" binding:"required,email" example:"user@example.com" description:"User email address"`
	Password   string                 `json:"password" binding:"required" example:"password123" description:"User password (minimum 8 characters)"`
}

// Login handles user login
// @Summary User login
// @Description Authenticate user. Sets httpOnly cookies: access_token (15min), refresh_token (7days), csrf_token (15min). Returns user data + expiry metadata (milliseconds).
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} response.SuccessResponse "Cookies set. Response: { user, expires_at(ms), expires_in(ms) }"
// @Header 200 {string} Set-Cookie "access_token=<jwt>; HttpOnly; Secure; SameSite=Lax; Max-Age=900"
// @Header 200 {string} Set-Cookie "refresh_token=<jwt>; HttpOnly; Secure; SameSite=Strict; Path=/api/v1/auth/refresh; Max-Age=604800"
// @Header 200 {string} Set-Cookie "csrf_token=<token>; Secure; SameSite=Lax; Max-Age=900"
// @Failure 400 {object} response.ErrorResponse "Invalid request payload"
// @Failure 401 {object} response.ErrorResponse "Invalid credentials"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Create auth login request
	authReq := &auth.LoginRequest{
		Email:      req.Email,
		Password:   req.Password,
		DeviceInfo: req.DeviceInfo,
	}

	// Attempt login
	loginResp, err := h.authService.Login(c.Request.Context(), authReq)
	if err != nil {
		h.logger.Error("Login failed", "error", err, "email", req.Email)
		response.Error(c, err)
		return
	}

	// Fetch user data BEFORE setting cookies (atomic authentication)
	userInterface, err := h.userService.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("Failed to get user data after login", "error", err, "email", req.Email)
		response.InternalServerError(c, "Failed to complete authentication")
		return
	}

	// Generate CSRF token with error handling
	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.Error("Failed to generate CSRF token", "error", err)
		response.InternalServerError(c, "Authentication setup failed")
		return
	}

	// All data fetched successfully - NOW set httpOnly cookies (atomic)
	setAuthCookies(c.Writer, loginResp.AccessToken, loginResp.RefreshToken, csrfToken, h.config.Server.CookieDomain)

	// Calculate expiry times in milliseconds (unified timestamp format)
	expiresAt := time.Now().Add(time.Duration(loginResp.ExpiresIn) * time.Second)
	expiresAtMs := expiresAt.UnixMilli()
	expiresInMs := loginResp.ExpiresIn * 1000

	// Return metadata only (tokens in httpOnly cookies)
	responseData := gin.H{
		"user":       userInterface, // Always present (atomic authentication)
		"expires_at": expiresAtMs,   // Milliseconds
		"expires_in": expiresInMs,   // Milliseconds
	}

	h.logger.Info("User logged in successfully", "email", req.Email)
	response.Success(c, responseData)
}

// RegisterRequest represents the registration request payload
// @Description User registration information
type RegisterRequest struct {
	OrganizationName *string `json:"organization_name,omitempty" example:"Acme Corp" description:"Organization name (required for fresh signup, omitted for invitation)"`
	ReferralSource   *string `json:"referral_source,omitempty" example:"search" description:"Where did you hear about us (optional)"`
	InvitationToken  *string `json:"invitation_token,omitempty" example:"01HX..." description:"Invitation token (for invitation-based signup)"`
	Email            string  `json:"email" binding:"required,email" example:"user@example.com" description:"User email address"`
	FirstName        string  `json:"first_name" binding:"required,min=1,max=100" example:"John" description:"User first name"`
	LastName         string  `json:"last_name" binding:"required,min=1,max=100" example:"Doe" description:"User last name"`
	Password         string  `json:"password" binding:"required,min=8" example:"password123" description:"User password (minimum 8 characters)"`
	Role             string  `json:"role" binding:"required,oneof=engineer product designer executive other" example:"engineer" description:"User role (engineer, product, designer, executive, other)"`
}

// Signup handles user registration
// @Summary User registration
// @Description Register new user with organization or invitation. Sets httpOnly cookies: access_token (15min), refresh_token (7days), csrf_token (15min). Returns user data + expiry metadata (milliseconds).
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Registration information"
// @Success 200 {object} response.SuccessResponse "Cookies set. Response: { user, expires_at(ms), expires_in(ms) }"
// @Header 200 {string} Set-Cookie "access_token=<jwt>; HttpOnly; Secure; SameSite=Lax; Max-Age=900"
// @Header 200 {string} Set-Cookie "refresh_token=<jwt>; HttpOnly; Secure; SameSite=Strict; Path=/api/v1/auth/refresh; Max-Age=604800"
// @Header 200 {string} Set-Cookie "csrf_token=<token>; Secure; SameSite=Lax; Max-Age=900"
// @Failure 400 {object} response.ErrorResponse "Invalid request payload"
// @Failure 409 {object} response.ErrorResponse "Email already exists"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/signup [post]
func (h *Handler) Signup(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validation: must have either org name or invitation token
	if req.InvitationToken == nil && req.OrganizationName == nil {
		h.logger.Error("Registration requires either organization_name or invitation_token")
		response.Error(c, appErrors.NewValidationError("organization_name required for fresh signups", ""))
		return
	}

	// Create registration request
	regReq := &registration.RegisterRequest{
		Email:            req.Email,
		Password:         req.Password,
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		Role:             req.Role,
		ReferralSource:   req.ReferralSource,
		OrganizationName: req.OrganizationName,
		InvitationToken:  req.InvitationToken,
		IsOAuthUser:      false, // Email/password signup
	}

	// Route to appropriate registration method
	var regResp *registration.RegistrationResponse
	var err error

	if req.InvitationToken != nil {
		// Invitation-based signup
		regResp, err = h.registrationService.RegisterWithInvitation(c.Request.Context(), regReq)
	} else {
		// Fresh signup with organization
		regResp, err = h.registrationService.RegisterWithOrganization(c.Request.Context(), regReq)
	}

	if err != nil {
		h.logger.Error("Registration failed", "error", err, "email", req.Email)
		response.Error(c, err)
		return
	}

	// Fetch user data BEFORE setting cookies (atomic authentication)
	userInterface, err := h.userService.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("Failed to get user data after signup", "error", err, "email", req.Email)
		response.InternalServerError(c, "Failed to complete authentication")
		return
	}

	// Generate CSRF token with error handling
	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.Error("Failed to generate CSRF token during signup", "error", err)
		response.InternalServerError(c, "Authentication setup failed")
		return
	}

	// All data fetched successfully - NOW set httpOnly cookies (atomic)
	setAuthCookies(c.Writer, regResp.LoginTokens.AccessToken, regResp.LoginTokens.RefreshToken, csrfToken, h.config.Server.CookieDomain)

	// Calculate expiry times in milliseconds (unified timestamp format)
	expiresAt := time.Now().Add(time.Duration(regResp.LoginTokens.ExpiresIn) * time.Second)
	expiresAtMs := expiresAt.UnixMilli()
	expiresInMs := regResp.LoginTokens.ExpiresIn * 1000

	// Return metadata only (tokens in httpOnly cookies)
	responseData := gin.H{
		"user":       userInterface, // Always present (atomic authentication)
		"expires_at": expiresAtMs,   // Milliseconds
		"expires_in": expiresInMs,   // Milliseconds
	}

	h.logger.Info("User registered successfully", "email", req.Email, "org_id", regResp.Organization.ID)

	response.Success(c, responseData)
}

// CompleteOAuthSignupRequest represents the OAuth signup completion request
// @Description Complete OAuth signup with additional user information
type CompleteOAuthSignupRequest struct {
	OrganizationName *string `json:"organization_name,omitempty" example:"Acme Corp" description:"Organization name (required for fresh signup)"`
	ReferralSource   *string `json:"referral_source,omitempty" example:"search" description:"Where did you hear about us"`
	SessionID        string  `json:"session_id" binding:"required" example:"01HX..." description:"OAuth session ID from redirect"`
	Role             string  `json:"role" binding:"required" example:"engineer" description:"User role"`
}

// CompleteOAuthSignup handles OAuth signup completion (Step 2)
// @Summary Complete OAuth signup
// @Description Complete OAuth-based registration with additional user information
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body CompleteOAuthSignupRequest true "OAuth signup completion data"
// @Success 200 {object} response.SuccessResponse "OAuth signup completed successfully"
// @Failure 400 {object} response.ErrorResponse "Invalid request payload"
// @Failure 404 {object} response.ErrorResponse "Session not found or expired"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/complete-oauth-signup [post]
func (h *Handler) CompleteOAuthSignup(c *gin.Context) {
	var req CompleteOAuthSignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Get OAuth session from Redis
	session, err := h.authService.GetOAuthSession(c.Request.Context(), req.SessionID)
	if err != nil {
		h.logger.Error("Failed to get OAuth session", "error", err)
		response.Error(c, err)
		return
	}

	// Validate required fields
	if session.Email == "" {
		h.logger.Error("Missing email in OAuth session")
		response.Error(c, appErrors.NewValidationError("Invalid session: missing email", ""))
		return
	}
	if session.FirstName == "" {
		h.logger.Error("Missing first name in OAuth session")
		response.Error(c, appErrors.NewValidationError("Invalid session: missing first name", ""))
		return
	}
	if session.LastName == "" {
		h.logger.Error("Missing last name in OAuth session")
		response.Error(c, appErrors.NewValidationError("Invalid session: missing last name", ""))
		return
	}
	if session.Provider == "" {
		h.logger.Error("Missing provider in OAuth session")
		response.Error(c, appErrors.NewValidationError("Invalid session: missing provider", ""))
		return
	}
	if session.ProviderID == "" {
		h.logger.Error("Missing provider ID in OAuth session")
		response.Error(c, appErrors.NewValidationError("Invalid session: missing provider ID", ""))
		return
	}

	// Create OAuth registration request with validated session data
	oauthReq := &registration.OAuthRegistrationRequest{
		Email:            session.Email,
		FirstName:        session.FirstName,
		LastName:         session.LastName,
		Role:             req.Role,
		Provider:         session.Provider,
		ProviderID:       session.ProviderID,
		ReferralSource:   req.ReferralSource,
		OrganizationName: req.OrganizationName,
		InvitationToken:  session.InvitationToken,
	}

	// Complete OAuth registration
	regResp, err := h.registrationService.CompleteOAuthRegistration(c.Request.Context(), oauthReq)
	if err != nil {
		h.logger.Error("OAuth registration failed", "error", err, "email", session.Email)
		response.Error(c, err)
		return
	}

	// Delete OAuth session (cleanup)
	_ = h.authService.DeleteOAuthSession(c.Request.Context(), req.SessionID)

	// Fetch user data BEFORE setting cookies (atomic authentication)
	userInterface, err := h.userService.GetUserByEmail(c.Request.Context(), session.Email)
	if err != nil {
		h.logger.Error("Failed to get user data after OAuth signup", "error", err, "email", session.Email)
		response.InternalServerError(c, "Failed to complete authentication")
		return
	}

	// Generate CSRF token with error handling
	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.Error("Failed to generate CSRF token during OAuth signup", "error", err)
		response.InternalServerError(c, "Authentication setup failed")
		return
	}

	// All data fetched successfully - NOW set httpOnly cookies (atomic)
	setAuthCookies(c.Writer, regResp.LoginTokens.AccessToken, regResp.LoginTokens.RefreshToken, csrfToken, h.config.Server.CookieDomain)

	// Calculate expiry times in milliseconds (unified timestamp format)
	expiresAt := time.Now().Add(time.Duration(regResp.LoginTokens.ExpiresIn) * time.Second)
	expiresAtMs := expiresAt.UnixMilli()
	expiresInMs := regResp.LoginTokens.ExpiresIn * 1000

	// Return metadata with user and organization (tokens in httpOnly cookies)
	responseData := gin.H{
		"user":         userInterface,        // Always present (atomic authentication)
		"organization": regResp.Organization, // Always present from registration
		"expires_at":   expiresAtMs,          // Milliseconds
		"expires_in":   expiresInMs,          // Milliseconds
	}

	h.logger.Info("OAuth registration completed successfully", "email", session.Email, "provider", session.Provider, "org_id", regResp.Organization.ID)

	response.Success(c, responseData)
}

// ExchangeLoginSession exchanges a one-time session ID for login tokens
// @Summary Exchange login session
// @Description Exchange OAuth login session for access tokens (existing user OAuth login)
// @Tags Authentication
// @Param session_id path string true "Login session ID"
// @Success 200 {object} response.SuccessResponse "Login tokens"
// @Failure 404 {object} response.ErrorResponse "Session not found or expired"
// @Failure 400 {object} response.ErrorResponse "Invalid session data"
// @Router /api/v1/auth/exchange-session/{session_id} [post]
func (h *Handler) ExchangeLoginSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	// Get login tokens from session (one-time use)
	sessionData, err := h.authService.GetLoginTokenSession(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Error("Failed to get login session", "error", err)
		response.Error(c, err)
		return
	}

	accessToken := sessionData.AccessToken
	if accessToken == "" {
		h.logger.Error("Missing access_token in login session")
		response.Error(c, appErrors.NewValidationError("Invalid session data: missing access token", ""))
		return
	}

	refreshToken := sessionData.RefreshToken
	if refreshToken == "" {
		h.logger.Error("Missing refresh_token in login session")
		response.Error(c, appErrors.NewValidationError("Invalid session data: missing refresh token", ""))
		return
	}

	expiresIn := sessionData.ExpiresIn
	if expiresIn <= 0 {
		h.logger.Error("Missing or invalid expires_in in login session")
		response.Error(c, appErrors.NewValidationError("Invalid session data: missing expiration", ""))
		return
	}

	userID := sessionData.UserID
	if userID == uuid.Nil {
		h.logger.Error("Missing user_id in login session")
		response.Error(c, appErrors.NewValidationError("Invalid session data: missing user ID", ""))
		return
	}

	// Fetch user data BEFORE setting cookies (atomic authentication)
	userInterface, err := h.userService.GetUser(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user data during session exchange", "error", err, "user_id", userID)
		response.InternalServerError(c, "Failed to complete authentication")
		return
	}

	// Generate CSRF token with error handling
	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.Error("Failed to generate CSRF token during session exchange", "error", err)
		response.InternalServerError(c, "Authentication setup failed")
		return
	}

	// All data fetched successfully - NOW set httpOnly cookies (atomic)
	setAuthCookies(c.Writer, accessToken, refreshToken, csrfToken, h.config.Server.CookieDomain)

	// Calculate expiry times in milliseconds (unified timestamp format)
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	expiresAtMs := expiresAt.UnixMilli()
	expiresInMs := expiresIn * 1000

	// Return metadata only (tokens in httpOnly cookies)
	responseData := gin.H{
		"user":       userInterface, // Always present (atomic authentication)
		"expires_at": expiresAtMs,   // Milliseconds
		"expires_in": expiresInMs,   // Milliseconds
	}

	h.logger.Info("Login session exchanged successfully")
	response.Success(c, responseData)
}

// RefreshTokenRequest represents the refresh token request payload (deprecated - now using cookies)
// @Description Refresh token credentials
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." description:"Valid refresh token"`
}

// RefreshToken handles token refresh via httpOnly cookies
// @Summary Refresh access token
// @Description Refresh tokens using refresh_token from httpOnly cookie. Returns new cookies with rotated tokens + expiry metadata (milliseconds).
// @Tags Authentication
// @Accept json
// @Produce json
// @Success 200 {object} response.SuccessResponse "New cookies set. Response: { expires_at(ms), expires_in(ms) }"
// @Header 200 {string} Set-Cookie "access_token=<jwt>; HttpOnly; Secure; SameSite=Lax; Max-Age=900"
// @Header 200 {string} Set-Cookie "refresh_token=<jwt>; HttpOnly; Secure; SameSite=Strict; Path=/api/v1/auth/refresh; Max-Age=604800"
// @Header 200 {string} Set-Cookie "csrf_token=<token>; Secure; SameSite=Lax; Max-Age=900"
// @Failure 401 {object} response.ErrorResponse "Refresh token missing, invalid, or expired. Cookies cleared."
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/refresh [post]
func (h *Handler) RefreshToken(c *gin.Context) {
	// Read refresh token from httpOnly cookie
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		h.logger.Error("Refresh token cookie not found", "error", err)
		clearAuthCookies(c.Writer, h.config.Server.CookieDomain)
		response.ErrorWithStatus(c, http.StatusUnauthorized, "REFRESH_EXPIRED", "Refresh token not found", "")
		return
	}

	// Create auth refresh request
	authReq := &auth.RefreshTokenRequest{
		RefreshToken: refreshToken,
	}

	// Refresh token
	loginResp, err := h.authService.RefreshToken(c.Request.Context(), authReq)
	if err != nil {
		h.logger.Error("Token refresh failed", "error", err)
		clearAuthCookies(c.Writer, h.config.Server.CookieDomain)
		response.ErrorWithStatus(c, http.StatusUnauthorized, "REFRESH_EXPIRED", "Refresh token invalid or expired", err.Error())
		return
	}

	// Generate new CSRF token with error handling
	csrfToken, err := generateCSRFToken()
	if err != nil {
		h.logger.Error("Failed to generate CSRF token during refresh", "error", err)
		clearAuthCookies(c.Writer, h.config.Server.CookieDomain)
		response.InternalServerError(c, "Token refresh setup failed")
		return
	}

	// Set new httpOnly cookies
	setAuthCookies(c.Writer, loginResp.AccessToken, loginResp.RefreshToken, csrfToken, h.config.Server.CookieDomain)

	// Get user ID from the refresh token JWT claims to fetch user data
	// Note: We could extract user ID from the new access token, but for now we'll skip user data
	// The frontend can call /me endpoint if it needs user data

	// Calculate expiry times in milliseconds (unified timestamp format)
	expiresAt := time.Now().Add(time.Duration(loginResp.ExpiresIn) * time.Second)
	expiresAtMs := expiresAt.UnixMilli()
	expiresInMs := loginResp.ExpiresIn * 1000

	// Return metadata only (tokens in httpOnly cookies)
	responseData := gin.H{
		"expires_at": expiresAtMs, // Milliseconds
		"expires_in": expiresInMs, // Milliseconds
	}

	h.logger.Info("Token refreshed successfully")
	response.Success(c, responseData)
}

// GetCurrentUser returns current authenticated user with token expiry metadata
// @Summary Get current user
// @Description Get current user with token expiry metadata. Requires access_token cookie for authentication.
// @Tags Authentication
// @Accept json
// @Produce json
// @Security CookieAuth
// @Success 200 {object} response.SuccessResponse "Response: { user, expires_at(ms), expires_in(ms) }"
// @Failure 401 {object} response.ErrorResponse "Unauthorized - cookie invalid or missing"
// @Failure 404 {object} response.ErrorResponse "User not found"
// @Router /api/v1/auth/me [get]
func (h *Handler) GetCurrentUser(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	claims := middleware.MustGetTokenClaims(c)

	// Get user data
	user, err := h.userService.GetUser(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user", "error", err, "user_id", userID)
		response.NotFound(c, "User")
		return
	}

	// Calculate expiry times in milliseconds from JWT exp claim
	// JWT exp is in seconds (Unix timestamp), convert to milliseconds
	expiresAtMs := claims.ExpiresAt * 1000
	expiresInMs := expiresAtMs - time.Now().UnixMilli()

	// Return user with expiry metadata
	responseData := gin.H{
		"user":       user,
		"expires_at": expiresAtMs, // Milliseconds
		"expires_in": expiresInMs, // Milliseconds
	}

	response.Success(c, responseData)
}

// ForgotPasswordRequest represents the forgot password request payload
// @Description Email for password reset
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email" example:"user@example.com" description:"Email address for password reset"`
}

// ForgotPassword handles forgot password
// @Summary Request password reset
// @Description Initiate password reset process by sending reset email
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body ForgotPasswordRequest true "Email for password reset"
// @Success 200 {object} response.MessageResponse "Reset email sent if account exists"
// @Failure 400 {object} response.ErrorResponse "Invalid request payload"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/forgot-password [post]
func (h *Handler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Initiate password reset
	err := h.authService.ResetPassword(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("Password reset initiation failed", "error", err, "email", req.Email)
		// Don't reveal if email exists or not
	}

	h.logger.Info("Password reset initiated", "email", req.Email)
	response.Success(c, gin.H{
		"message": "If the email exists, a password reset link has been sent",
	})
}

// ResetPasswordRequest represents the reset password request payload
// @Description Reset password with token
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required" example:"reset_token_123" description:"Password reset token from email"`
	NewPassword string `json:"new_password" binding:"required,min=8" example:"newpassword123" description:"New password (minimum 8 characters)"`
}

// ResetPassword handles reset password
// @Summary Reset password
// @Description Complete password reset using token from email
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body ResetPasswordRequest true "Reset password information"
// @Success 200 {object} response.MessageResponse "Password reset successful"
// @Failure 400 {object} response.ErrorResponse "Invalid request payload or expired token"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/reset-password [post]
func (h *Handler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Confirm password reset
	err := h.authService.ConfirmPasswordReset(c.Request.Context(), req.Token, req.NewPassword)
	if err != nil {
		h.logger.Error("Password reset failed", "error", err)
		response.Error(c, appErrors.NewValidationError("Password reset failed", err.Error()))
		return
	}

	h.logger.Info("Password reset completed successfully")
	response.Success(c, gin.H{
		"message": "Password reset successfully",
	})
}

// Logout handles user logout
// @Summary User logout
// @Description Logout user, invalidate session, clear httpOnly cookies. REQUIRES X-CSRF-Token header matching csrf_token cookie value (double-submit CSRF protection).
// @Tags Authentication
// @Accept json
// @Produce json
// @Security CookieAuth && CSRFToken
// @Param X-CSRF-Token header string true "CSRF token from csrf_token cookie"
// @Success 200 {object} response.MessageResponse "Logout successful. All cookies cleared."
// @Header 200 {string} Set-Cookie "access_token=; Max-Age=-1"
// @Header 200 {string} Set-Cookie "refresh_token=; Max-Age=-1"
// @Header 200 {string} Set-Cookie "csrf_token=; Max-Age=-1"
// @Failure 401 {object} response.ErrorResponse "Invalid session or cookie missing"
// @Failure 403 {object} response.ErrorResponse "CSRF validation failed - token missing or mismatch"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	claims := middleware.MustGetTokenClaims(c)

	// Logout user by blacklisting current access token JTI
	err := h.authService.Logout(c.Request.Context(), claims.JWTID, claims.UserID)
	if err != nil {
		h.logger.Error("Logout failed", "error", err, "jti", claims.JWTID, "user_id", claims.UserID)
		// Clear cookies even if logout fails (best effort)
		clearAuthCookies(c.Writer, h.config.Server.CookieDomain)
		response.ErrorWithStatus(c, http.StatusInternalServerError, "logout_failed", "Logout failed", err.Error())
		return
	}

	// Clear httpOnly cookies
	clearAuthCookies(c.Writer, h.config.Server.CookieDomain)

	h.logger.Info("User logged out successfully", "jti", claims.JWTID, "user_id", claims.UserID)
	response.Success(c, gin.H{
		"message": "Logged out successfully",
	})
}

// GetProfile returns current user profile
func (h *Handler) GetProfile(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	// Get current user
	user, err := h.userService.GetUser(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user profile", "error", err, "user_id", userID)
		response.ErrorWithStatus(c, http.StatusNotFound, "user_not_found", "User not found", err.Error())
		return
	}

	response.Success(c, user)
}

// UpdateProfileRequest represents the update profile request payload
// @Description User profile update information
type UpdateProfileRequest struct {
	FirstName *string `json:"first_name,omitempty" validate:"omitempty,min=1,max=100" example:"John" description:"User first name"`
	LastName  *string `json:"last_name,omitempty" validate:"omitempty,min=1,max=100" example:"Doe" description:"User last name"`
	AvatarURL *string `json:"avatar_url,omitempty" validate:"omitempty,url" example:"https://example.com/avatar.jpg" description:"Profile avatar URL"`
	Phone     *string `json:"phone,omitempty" validate:"omitempty,max=50" example:"+1234567890" description:"User phone number"`
	Timezone  *string `json:"timezone,omitempty" example:"UTC" description:"User timezone"`
	Language  *string `json:"language,omitempty" validate:"omitempty,len=2" example:"en" description:"User language preference (ISO 639-1 code)"`
}

// UpdateProfile updates user profile
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userID := middleware.MustGetUserID(c)

	// Create user update request
	updateReq := &user.UpdateUserRequest{
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Timezone:  req.Timezone,
		Language:  req.Language,
	}

	// Update profile
	updatedUser, err := h.userService.UpdateUser(c.Request.Context(), userID, updateReq)
	if err != nil {
		h.logger.Error("Profile update failed", "error", err, "user_id", userID)
		response.Error(c, err)
		return
	}

	h.logger.Info("Profile updated successfully", "user_id", userID)
	response.Success(c, updatedUser)
}

// ChangePasswordRequest represents the change password request payload
// @Description Password change information
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required" example:"currentpass123" description:"Current password"`
	NewPassword     string `json:"new_password" binding:"required,min=8" example:"newpass123" description:"New password (minimum 8 characters)"`
}

// ChangePassword changes user password
// @Summary Change password
// @Description Change user password with current password verification. REQUIRES X-CSRF-Token header matching csrf_token cookie value (double-submit CSRF protection).
// @Tags Authentication
// @Accept json
// @Produce json
// @Security CookieAuth && CSRFToken
// @Param X-CSRF-Token header string true "CSRF token from csrf_token cookie"
// @Param request body ChangePasswordRequest true "Password change information"
// @Success 200 {object} response.MessageResponse "Password changed successfully"
// @Failure 400 {object} response.ErrorResponse "Invalid request or wrong current password"
// @Failure 401 {object} response.ErrorResponse "Unauthorized - cookie invalid or missing"
// @Failure 403 {object} response.ErrorResponse "CSRF validation failed - token missing or mismatch"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/change-password [post]
func (h *Handler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userID := middleware.MustGetUserID(c)

	// Change password
	err := h.userService.ChangePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		h.logger.Error("Password change failed", "error", err, "user_id", userID)
		response.ErrorWithStatus(c, http.StatusBadRequest, "password_change_failed", "Password change failed", err.Error())
		return
	}

	h.logger.Info("Password changed successfully", "user_id", userID)
	response.Success(c, gin.H{
		"message": "Password changed successfully",
	})
}

// ValidateToken validates JWT tokens (for middleware)
func (h *Handler) ValidateToken(token string) (*auth.AuthContext, error) {
	return h.authService.ValidateAuthToken(context.Background(), token)
}

// ValidateAPIKey validates API keys (for middleware)
func (h *Handler) ValidateAPIKey(apiKey string) (*auth.AuthContext, error) {
	ctx := context.Background()

	// Validate the API key using the APIKeyService
	key, err := h.apiKeyService.ValidateAPIKey(ctx, apiKey)
	if err != nil {
		// Log validation failure without exposing the full key (security best practice)
		h.logger.Warn("API key validation failed", "error", err)
		return nil, err
	}

	// Create AuthContext from the validated key
	authContext := key.AuthContext

	// Log successful validation (without the actual key)
	h.logger.Debug("API key validation successful",
		"user_id", key.AuthContext.UserID,
		"api_key_id", key.AuthContext.APIKeyID,
		"project_id", key.ProjectID,
	)

	return authContext, nil
}

// ValidateAPIKeyHandler validates self-contained API keys (industry standard)
// @Summary Validate API key
// @Description Validates a self-contained API key and extracts project information automatically
// @Tags SDK - Authentication
// @Accept json
// @Produce json
// @Param X-API-Key header string false "API key (format: bk_{40_char_random})"
// @Param Authorization header string false "Bearer token format: Bearer {api_key}"
// @Success 200 {object} response.SuccessResponse "API key validation successful"
// @Failure 400 {object} response.ErrorResponse "Invalid request"
// @Failure 401 {object} response.ErrorResponse "Invalid, inactive, or expired API key"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /v1/auth/validate-key [post]
func (h *Handler) ValidateAPIKeyHandler(c *gin.Context) {
	// Extract API key from X-API-Key header or Authorization Bearer
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		// Fallback to Authorization header with Bearer format
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if apiKey == "" {
		h.logger.Warn("API key validation request missing API key")
		response.Error(c, appErrors.NewValidationError("Missing API key", "API key must be provided via X-API-Key header or Authorization Bearer token"))
		return
	}

	// Validate the industry-standard API key (bk_{random})
	resp, err := h.apiKeyService.ValidateAPIKey(c.Request.Context(), apiKey)
	if err != nil {
		// Log validation failure without exposing the full key (security best practice)
		h.logger.Warn("API key validation failed", "error", err)
		response.Error(c, err) // Properly propagate AppError status codes (401, etc.)
		return
	}

	// Log successful validation (without the actual key)
	h.logger.Info("API key validation successful", "user_id", resp.AuthContext.UserID, "api_key_id", resp.AuthContext.APIKeyID, "project_id", resp.ProjectID)

	response.Success(c, resp)
}

// ListSessionsRequest represents request for listing user sessions
type ListSessionsRequest struct {
	Page     int  `form:"page,default=1" example:"1" description:"Page number for pagination"`
	PageSize int  `form:"page_size,default=10" example:"10" description:"Number of sessions per page"`
	Active   bool `form:"active,default=false" example:"false" description:"Filter for active sessions only"`
}

// ListSessions lists all user sessions
// @Summary List user sessions
// @Description Get paginated list of user sessions with device info
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param cursor query string false "Pagination cursor" example("eyJjcmVhdGVkX2F0IjoiMjAyNC0wMS0wMVQxMjowMDowMFoiLCJpZCI6IjAxSDJYM1k0WjUifQ==")
// @Param page_size query int false "Items per page" Enums(10,20,30,40,50) default(50)
// @Param sort_by query string false "Sort field" Enums(created_at) default("created_at")
// @Param sort_dir query string false "Sort direction" Enums(asc,desc) default("desc")
// @Param active query bool false "Active sessions only" default(false)
// @Success 200 {object} response.SuccessResponse "Sessions retrieved successfully"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/sessions [get]
func (h *Handler) ListSessions(c *gin.Context) {
	var req ListSessionsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.logger.Error("Invalid list sessions request", "error", err)
		response.Error(c, appErrors.NewValidationError("Invalid request parameters", err.Error()))
		return
	}

	// Get user ID from context
	userID := middleware.MustGetUserID(c)

	// Get user sessions (using GetUserSessions method)
	sessions, err := h.authService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to list sessions", "error", err, "user_id", userID)
		response.InternalServerError(c, "Failed to retrieve sessions")
		return
	}

	// Parse offset pagination parameters
	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)

	// Filter active sessions if requested
	var filteredSessions []*auth.UserSession
	if req.Active {
		for _, session := range sessions {
			if session.IsValid() {
				filteredSessions = append(filteredSessions, session)
			}
		}
	} else {
		filteredSessions = sessions
	}

	total := int64(len(filteredSessions))

	// Sort sessions for stable ordering
	sort.Slice(filteredSessions, func(i, j int) bool {
		if params.SortDir == "asc" {
			return filteredSessions[i].CreatedAt.Before(filteredSessions[j].CreatedAt)
		}
		return filteredSessions[i].CreatedAt.After(filteredSessions[j].CreatedAt)
	})

	// Apply offset pagination
	offset := params.GetOffset()
	limit := params.Limit

	// Calculate end index for slicing
	end := offset + limit
	if end > len(filteredSessions) {
		end = len(filteredSessions)
	}

	// Apply pagination slice
	if offset < len(filteredSessions) {
		filteredSessions = filteredSessions[offset:end]
	} else {
		filteredSessions = []*auth.UserSession{}
	}

	// Create offset pagination
	pag := response.NewPagination(params.Page, params.Limit, total)

	h.logger.Info("Sessions listed successfully", "user_id", userID, "count", len(filteredSessions), "total", total)

	response.SuccessWithPagination(c, filteredSessions, pag)
}

// GetSessionRequest represents request for getting session by ID
type GetSessionRequest struct {
	SessionID uuid.UUID `uri:"session_id" binding:"required" example:"01FXYZ123456789ABCDEFGHIJK0" description:"Session ID" swaggertype:"string"`
}

// GetSession gets a specific user session by ID
// @Summary Get user session
// @Description Get details of a specific user session
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param session_id path string true "Session ID"
// @Success 200 {object} response.SuccessResponse "Session retrieved successfully"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 404 {object} response.ErrorResponse "Session not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/sessions/{session_id} [get]
func (h *Handler) GetSession(c *gin.Context) {
	var req GetSessionRequest
	if err := c.ShouldBindUri(&req); err != nil {
		h.logger.Error("Invalid get session request", "error", err)
		response.Error(c, appErrors.NewValidationError("Invalid session ID", err.Error()))
		return
	}

	// Get user ID from context
	userID := middleware.MustGetUserID(c)

	// Get all user sessions first, then filter by session ID
	sessions, err := h.authService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get sessions", "error", err, "user_id", userID, "session_id", req.SessionID)
		response.InternalServerError(c, "Failed to retrieve session")
		return
	}

	// Find the specific session
	var session *auth.UserSession
	for _, s := range sessions {
		if s.ID == req.SessionID {
			session = s
			break
		}
	}

	if session == nil {
		h.logger.Warn("Session not found", "user_id", userID, "session_id", req.SessionID)
		response.NotFound(c, "Session")
		return
	}

	h.logger.Info("Session retrieved successfully", "user_id", userID, "session_id", req.SessionID)

	response.Success(c, session)
}

// RevokeSessionRequest represents request for revoking a session
type RevokeSessionRequest struct {
	SessionID uuid.UUID `uri:"session_id" binding:"required" example:"01FXYZ123456789ABCDEFGHIJK0" description:"Session ID to revoke" swaggertype:"string"`
}

// RevokeSession revokes a specific user session
// @Summary Revoke user session
// @Description Revoke a specific user session (logout from specific device)
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param session_id path string true "Session ID"
// @Success 200 {object} response.MessageResponse "Session revoked successfully"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 404 {object} response.ErrorResponse "Session not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/sessions/{session_id}/revoke [post]
func (h *Handler) RevokeSession(c *gin.Context) {
	var req RevokeSessionRequest
	if err := c.ShouldBindUri(&req); err != nil {
		h.logger.Error("Invalid revoke session request", "error", err)
		response.Error(c, appErrors.NewValidationError("Invalid session ID", err.Error()))
		return
	}

	// Get user ID from context
	userID := middleware.MustGetUserID(c)

	// Revoke session
	err := h.authService.RevokeSession(c.Request.Context(), userID, req.SessionID)
	if err != nil {
		h.logger.Error("Failed to revoke session", "error", err, "user_id", userID, "session_id", req.SessionID)
		response.NotFound(c, "Session")
		return
	}

	h.logger.Info("Session revoked successfully", "user_id", userID, "session_id", req.SessionID)

	response.Success(c, gin.H{
		"message": "Session revoked successfully",
	})
}

// RevokeAllSessionsRequest represents request for revoking all user sessions
type RevokeAllSessionsRequest struct {
	// Note: This struct is kept for future extensibility but currently has no fields
}

// RevokeAllSessions revokes all user sessions
// @Summary Revoke all user sessions
// @Description Revoke all user sessions (logout from all devices). This will invalidate ALL active sessions for the user.
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body RevokeAllSessionsRequest false "Request body (currently unused but kept for future extensibility)"
// @Success 200 {object} response.MessageResponse "All sessions revoked successfully"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/auth/sessions/revoke-all [post]
func (h *Handler) RevokeAllSessions(c *gin.Context) {
	var req RevokeAllSessionsRequest
	// Don't require body, use defaults
	c.ShouldBindJSON(&req)

	// Get user ID from context
	userID := middleware.MustGetUserID(c)

	// Get current sessions count before revoking
	sessions, err := h.authService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get sessions for count", "error", err, "user_id", userID)
		response.InternalServerError(c, "Failed to revoke sessions")
		return
	}

	count := 0
	for _, session := range sessions {
		if session.IsValid() {
			count++
		}
	}

	// Revoke all sessions
	err = h.authService.RevokeAllSessions(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to revoke all sessions", "error", err, "user_id", userID)
		response.InternalServerError(c, "Failed to revoke sessions")
		return
	}

	// GDPR/SOC2 Compliance: Create user-wide timestamp blacklist to immediately block ALL tokens
	// This ensures complete compliance - any token issued before this timestamp is immediately invalid
	err = h.authService.RevokeUserAccessTokens(c.Request.Context(), userID, "user_requested_revoke_all_sessions")
	if err != nil {
		h.logger.Error("Failed to create user-wide token blacklist", "error", err, "user_id", userID)
		// Log error but don't fail the request since sessions were already revoked
		// This maintains partial security even if timestamp blacklisting fails
	}

	h.logger.Info("All sessions and access tokens revoked successfully", "user_id", userID, "revoked_count", count)

	response.Success(c, gin.H{
		"message": "All sessions revoked successfully",
		"count":   count,
	})
}
