package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"

	"brokle/internal/config"
	authDomain "brokle/internal/core/domain/auth"
	userDomain "brokle/internal/core/domain/user"
	appErrors "brokle/pkg/errors"
)

// authService implements the authDomain.AuthService interface
type authService struct {
	authConfig        *config.AuthConfig
	userRepo          userDomain.Repository
	sessionRepo       authDomain.UserSessionRepository
	jwtService        authDomain.JWTService
	roleService       authDomain.RoleService
	passwordResetRepo authDomain.PasswordResetTokenRepository
	blacklistedTokens authDomain.BlacklistedTokenService
	redis             *redis.Client // For OAuth session storage
}

// NewAuthService creates a new auth service instance
func NewAuthService(
	authConfig *config.AuthConfig,
	userRepo userDomain.Repository,
	sessionRepo authDomain.UserSessionRepository,
	jwtService authDomain.JWTService,
	roleService authDomain.RoleService,
	passwordResetRepo authDomain.PasswordResetTokenRepository,
	blacklistedTokens authDomain.BlacklistedTokenService,
	redisClient *redis.Client,
) authDomain.AuthService {
	return &authService{
		authConfig:        authConfig,
		userRepo:          userRepo,
		sessionRepo:       sessionRepo,
		jwtService:        jwtService,
		roleService:       roleService,
		passwordResetRepo: passwordResetRepo,
		blacklistedTokens: blacklistedTokens,
		redis:             redisClient,
	}
}

// Login authenticates a user and returns a login response
func (s *authService) Login(ctx context.Context, req *authDomain.LoginRequest) (*authDomain.LoginResponse, error) {
	// Get user with password
	user, err := s.userRepo.GetByEmailWithPassword(ctx, req.Email)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return nil, appErrors.NewUnauthorizedError("Invalid email or password")
		}
		return nil, appErrors.NewInternalError("Authentication service unavailable", err)
	}

	// Check if user is active
	if !user.IsActive {
		return nil, appErrors.NewForbiddenError("Account is inactive")
	}

	// Block OAuth users from password login
	if user.AuthMethod == "oauth" {
		providerName := "OAuth"
		if user.OAuthProvider != nil {
			providerName = *user.OAuthProvider
		}
		return nil, appErrors.NewUnauthorizedError("This account uses " + providerName + " login - please sign in with " + providerName)
	}

	// Verify password (only for password-based accounts)
	if user.AuthMethod == "password" {
		if user.Password == "" {
			return nil, appErrors.NewUnauthorizedError("Invalid email or password")
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
		if err != nil {
			return nil, appErrors.NewUnauthorizedError("Invalid email or password")
		}
	}

	// Get user effective permissions across all scopes
	// Note: Permissions are now handled by OrganizationMemberService
	permissions := []string{}

	// Generate access token with JTI for session tracking
	accessToken, jti, err := s.jwtService.GenerateAccessTokenWithJTI(ctx, user.ID, map[string]any{
		"email":           user.Email,
		"organization_id": user.DefaultOrganizationID,
		"permissions":     permissions,
	})
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate access token", err)
	}

	refreshToken, err := s.jwtService.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate refresh token", err)
	}

	// Use configurable token TTLs from AuthConfig
	expiresAt := time.Now().Add(s.authConfig.AccessTokenTTL)
	refreshExpiresAt := time.Now().Add(s.authConfig.RefreshTokenTTL)

	// Hash the refresh token for secure storage
	refreshTokenHash := s.hashToken(refreshToken)

	// Extract IP address and user agent from request context (if available)
	var ipAddress, userAgent *string
	// TODO: Extract from request context when available

	// Create secure session (NO ACCESS TOKEN STORED)
	session := authDomain.NewUserSession(user.ID, refreshTokenHash, jti, expiresAt, refreshExpiresAt, ipAddress, userAgent, req.DeviceInfo)
	err = s.sessionRepo.Create(ctx, session)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create session", err)
	}

	// Update last login
	err = s.userRepo.UpdateLastLogin(ctx, user.ID)
	if err != nil {
		// Non-critical error, continue with login
	}

	return &authDomain.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.authConfig.AccessTokenTTL.Seconds()),
	}, nil
}

// GenerateTokensForUser generates login tokens for a user without password validation.
// Used for OAuth signup, email verification, trusted authentication flows, and existing OAuth user login.
func (s *authService) GenerateTokensForUser(ctx context.Context, userID uuid.UUID) (*authDomain.LoginResponse, error) {
	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return nil, appErrors.NewNotFoundError("User not found")
		}
		return nil, appErrors.NewInternalError("User lookup failed", err)
	}

	// Check if user is active
	if !user.IsActive {
		return nil, appErrors.NewForbiddenError("Account is inactive")
	}

	// Get user effective permissions
	permissions := []string{}

	// Generate access token with JTI for session tracking
	accessToken, jti, err := s.jwtService.GenerateAccessTokenWithJTI(ctx, user.ID, map[string]any{
		"email":           user.Email,
		"organization_id": user.DefaultOrganizationID,
		"permissions":     permissions,
	})
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate access token", err)
	}

	refreshToken, err := s.jwtService.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate refresh token", err)
	}

	// Use configurable token TTLs from AuthConfig
	expiresAt := time.Now().Add(s.authConfig.AccessTokenTTL)
	refreshExpiresAt := time.Now().Add(s.authConfig.RefreshTokenTTL)

	// Hash the refresh token for secure storage
	refreshTokenHash := s.hashToken(refreshToken)

	// Create secure session
	session := authDomain.NewUserSession(user.ID, refreshTokenHash, jti, expiresAt, refreshExpiresAt, nil, nil, nil)
	err = s.sessionRepo.Create(ctx, session)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create session", err)
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID)

	return &authDomain.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.authConfig.AccessTokenTTL.Seconds()),
	}, nil
}

// Logout invalidates a user access token via JTI blacklisting
func (s *authService) Logout(ctx context.Context, jti string, userID uuid.UUID) error {
	// Blacklist the current access token immediately
	expiry := time.Now().Add(s.authConfig.AccessTokenTTL) // Blacklist until token would expire
	err := s.blacklistedTokens.BlacklistToken(ctx, jti, userID, expiry, "user_logout")
	if err != nil {
		return appErrors.NewInternalError("Failed to blacklist token", err)
	}

	return nil
}

// RefreshToken generates new access token using refresh token
func (s *authService) RefreshToken(ctx context.Context, req *authDomain.RefreshTokenRequest) (*authDomain.LoginResponse, error) {
	// Validate refresh token
	claims, err := s.jwtService.ValidateRefreshToken(ctx, req.RefreshToken)
	if err != nil {
		if errors.Is(err, authDomain.ErrTokenExpired) {
			return nil, appErrors.NewUnauthorizedError("Refresh token expired")
		}
		if errors.Is(err, authDomain.ErrTokenInvalid) {
			return nil, appErrors.NewUnauthorizedError("Invalid refresh token")
		}
		return nil, appErrors.NewInternalError("Token validation failed", err)
	}

	// Get session by refresh token hash
	refreshTokenHash := s.hashToken(req.RefreshToken)
	session, err := s.sessionRepo.GetByRefreshTokenHash(ctx, refreshTokenHash)
	if err != nil {
		if errors.Is(err, authDomain.ErrSessionNotFound) {
			return nil, appErrors.NewUnauthorizedError("Session not found")
		}
		return nil, appErrors.NewInternalError("Session lookup failed", err)
	}

	if !session.IsActive {
		return nil, appErrors.NewUnauthorizedError("Session is inactive")
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return nil, appErrors.NewUnauthorizedError("User not found")
		}
		return nil, appErrors.NewInternalError("User lookup failed", err)
	}

	if !user.IsActive {
		return nil, appErrors.NewForbiddenError("User is inactive")
	}

	// Get user effective permissions across all scopes
	// Note: Permissions are now handled by OrganizationMemberService
	permissions := []string{}

	// Generate new access token with JTI for session tracking
	accessToken, jti, err := s.jwtService.GenerateAccessTokenWithJTI(ctx, user.ID, map[string]any{
		"email":           user.Email,
		"organization_id": user.DefaultOrganizationID,
		"permissions":     permissions,
	})
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate access token", err)
	}

	// Implement token rotation if enabled
	var newRefreshToken string
	if s.authConfig.TokenRotationEnabled {
		// Generate new refresh token
		newRefreshToken, err = s.jwtService.GenerateRefreshToken(ctx, user.ID)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to generate new refresh token", err)
		}

		// Blacklist the old refresh token to prevent reuse
		oldRefreshClaims, err := s.jwtService.ValidateRefreshToken(ctx, req.RefreshToken)
		if err == nil && oldRefreshClaims.JWTID != "" {
			// Add old refresh token to blacklist
			err = s.blacklistedTokens.BlacklistToken(
				ctx,
				oldRefreshClaims.JWTID,
				user.ID,
				time.Now().Add(s.authConfig.RefreshTokenTTL), // Keep in blacklist until natural expiry
				"token_rotation",
			)
			if err != nil {
				// Non-critical error, continue with token rotation
			}
		}

		// Update session with new refresh token hash
		session.RefreshTokenHash = s.hashToken(newRefreshToken)
	} else {
		newRefreshToken = req.RefreshToken // Keep same refresh token
	}

	// Update session with new JTI and expiry (NO ACCESS TOKEN STORED)
	session.CurrentJTI = jti
	session.ExpiresAt = time.Now().Add(s.authConfig.AccessTokenTTL)
	session.UpdatedAt = time.Now()
	session.MarkAsUsed() // Update last used timestamp

	err = s.sessionRepo.Update(ctx, session)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to update session", err)
	}

	return &authDomain.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.authConfig.AccessTokenTTL.Seconds()),
	}, nil
}

// ChangePassword changes a user's password
func (s *authService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	// Verify current password
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword))
	if err != nil {
		return appErrors.NewUnauthorizedError("Current password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return appErrors.NewInternalError("Failed to hash new password", err)
	}

	// Update password
	err = s.userRepo.UpdatePassword(ctx, userID, string(hashedPassword))
	if err != nil {
		return appErrors.NewInternalError("Failed to update password", err)
	}

	// Revoke all user sessions (force re-login)
	err = s.sessionRepo.RevokeUserSessions(ctx, userID)
	if err != nil {
		// Non-critical error, password change succeeded
	}

	return nil
}

// ResetPassword initiates password reset process
func (s *authService) ResetPassword(ctx context.Context, email string) error {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Don't reveal if user exists or not
		return nil
	}

	// Invalidate any existing password reset tokens for this user
	err = s.passwordResetRepo.InvalidateAllUserTokens(ctx, user.ID)
	if err != nil {
		// Non-critical error, continue with reset process
	}

	// Generate secure reset token
	tokenBytes := make([]byte, 32)
	_, err = rand.Read(tokenBytes)
	if err != nil {
		return appErrors.NewInternalError("Failed to generate reset token", err)
	}
	tokenString := hex.EncodeToString(tokenBytes)

	// Create password reset token (expires in 1 hour)
	resetToken := authDomain.NewPasswordResetToken(user.ID, tokenString, time.Now().Add(1*time.Hour))
	err = s.passwordResetRepo.Create(ctx, resetToken)
	if err != nil {
		return appErrors.NewInternalError("Failed to create password reset token", err)
	}

	// TODO: Send email with reset link containing tokenString
	// The email would contain a link like: https://app.brokle.com/reset-password?token=tokenString

	return nil
}

// ConfirmPasswordReset completes password reset process
func (s *authService) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	// Find and validate password reset token
	resetToken, err := s.passwordResetRepo.GetByToken(ctx, token)
	if err != nil {
		return appErrors.NewUnauthorizedError("Invalid or expired password reset token")
	}

	// Check if token is valid (not used and not expired)
	isValid, err := s.passwordResetRepo.IsValid(ctx, resetToken.ID)
	if err != nil {
		return appErrors.NewInternalError("Failed to validate password reset token", err)
	}
	if !isValid {
		return appErrors.NewUnauthorizedError("Password reset token is invalid or expired")
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, resetToken.UserID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	if !user.IsActive {
		return appErrors.NewForbiddenError("User account is inactive")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return appErrors.NewInternalError("Failed to hash new password", err)
	}

	// Update password
	err = s.userRepo.UpdatePassword(ctx, user.ID, string(hashedPassword))
	if err != nil {
		return appErrors.NewInternalError("Failed to update password", err)
	}

	// Mark token as used
	err = s.passwordResetRepo.MarkAsUsed(ctx, resetToken.ID)
	if err != nil {
		// Non-critical error - password was already updated
	}

	// Revoke all user sessions (force re-login with new password)
	err = s.sessionRepo.RevokeUserSessions(ctx, user.ID)
	if err != nil {
		// Non-critical error, password reset succeeded
	}

	return nil
}

// SendEmailVerification sends email verification
func (s *authService) SendEmailVerification(ctx context.Context, userID uuid.UUID) error {
	// TODO: Generate verification token and send email

	return nil
}

// VerifyEmail verifies user's email
func (s *authService) VerifyEmail(ctx context.Context, token string) error {
	// TODO: Implement email verification
	return appErrors.NewNotImplementedError("Email verification not implemented")
}

// GetCurrentUser returns current user information
func (s *authService) GetCurrentUser(ctx context.Context, userID uuid.UUID) (*userDomain.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}

// UpdateProfile updates user profile
func (s *authService) UpdateProfile(ctx context.Context, userID uuid.UUID, req *authDomain.UpdateProfileRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	// Update fields if provided
	if req.FirstName != nil && *req.FirstName != "" {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil && *req.LastName != "" {
		user.LastName = *req.LastName
	}
	if req.Timezone != nil {
		user.Timezone = *req.Timezone
	}
	if req.Language != nil {
		user.Language = *req.Language
	}

	user.UpdatedAt = time.Now()

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return appErrors.NewInternalError("Failed to update user", err)
	}

	return nil
}

// GetUserSessions returns user's active sessions
func (s *authService) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	return s.sessionRepo.GetActiveSessionsByUserID(ctx, userID)
}

// RevokeSession revokes a specific user session
func (s *authService) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	// Verify session belongs to user
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, authDomain.ErrSessionNotFound) {
			return appErrors.NewNotFoundError("Session not found")
		}
		return appErrors.NewInternalError("Session lookup failed", err)
	}

	if session.UserID != userID {
		return appErrors.NewForbiddenError("Session does not belong to user")
	}

	return s.sessionRepo.RevokeSession(ctx, sessionID)
}

// RevokeAllSessions revokes all user sessions
func (s *authService) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	return s.sessionRepo.RevokeUserSessions(ctx, userID)
}

// GetAuthContext returns authentication context from token
func (s *authService) GetAuthContext(ctx context.Context, token string) (*authDomain.AuthContext, error) {
	// Validate JWT token
	claims, err := s.jwtService.ValidateAccessToken(ctx, token)
	if err != nil {
		if errors.Is(err, authDomain.ErrTokenExpired) {
			return nil, appErrors.NewUnauthorizedError("Token expired")
		}
		if errors.Is(err, authDomain.ErrTokenInvalid) {
			return nil, appErrors.NewUnauthorizedError("Invalid token")
		}
		return nil, appErrors.NewInternalError("Token validation failed", err)
	}

	// Return clean auth context - permissions resolved dynamically when needed
	return claims.GetUserContext(), nil
}

// ValidateAuthToken validates token and returns auth context
func (s *authService) ValidateAuthToken(ctx context.Context, token string) (*authDomain.AuthContext, error) {
	return s.GetAuthContext(ctx, token)
}

// RevokeAccessToken immediately revokes an access token by adding it to blacklist
func (s *authService) RevokeAccessToken(ctx context.Context, jti string, userID uuid.UUID, reason string) error {
	// Parse JTI to get token expiration time
	// We need the expiration time to know when to cleanup the blacklisted token
	// For now, we'll use a default expiration time based on config
	expiresAt := time.Now().Add(s.authConfig.AccessTokenTTL)

	// Add token to blacklist
	err := s.blacklistedTokens.BlacklistToken(ctx, jti, userID, expiresAt, reason)
	if err != nil {
		return appErrors.NewInternalError("Failed to revoke access token", err)
	}

	return nil
}

// RevokeUserAccessTokens revokes all active access tokens for a user
func (s *authService) RevokeUserAccessTokens(ctx context.Context, userID uuid.UUID, reason string) error {
	// Blacklist all user tokens
	err := s.blacklistedTokens.BlacklistUserTokens(ctx, userID, reason)
	if err != nil {
		return appErrors.NewInternalError("Failed to revoke user access tokens", err)
	}

	return nil
}

// IsTokenRevoked checks if an access token has been revoked
func (s *authService) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	return s.blacklistedTokens.IsTokenBlacklisted(ctx, jti)
}

// hashToken creates a SHA-256 hash of a token for secure storage
func (s *authService) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
