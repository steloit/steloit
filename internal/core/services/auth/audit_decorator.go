package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	appErrors "brokle/pkg/errors"
)

// auditDecorator wraps an AuthService to provide automatic audit logging
type auditDecorator struct {
	authService authDomain.AuthService
	auditRepo   authDomain.AuditLogRepository
	logger      *slog.Logger
}

// NewAuditDecorator creates a new audit decorator that wraps the auth service
func NewAuditDecorator(authService authDomain.AuthService, auditRepo authDomain.AuditLogRepository, logger *slog.Logger) authDomain.AuthService {
	return &auditDecorator{
		authService: authService,
		auditRepo:   auditRepo,
		logger:      logger,
	}
}

// Login handles user login with audit logging
func (a *auditDecorator) Login(ctx context.Context, req *authDomain.LoginRequest) (*authDomain.LoginResponse, error) {
	resp, err := a.authService.Login(ctx, req)

	// Audit based on result
	if err != nil {
		var reason string
		var userID *uuid.UUID

		// Determine reason based on error type
		if appErr, ok := appErrors.IsAppError(err); ok {
			switch appErr.Type {
			case appErrors.UnauthorizedError:
				reason = "invalid_credentials"
			case appErrors.ForbiddenError:
				reason = "account_inactive"
			default:
				reason = "system_error"
			}
		} else {
			reason = "system_error"
		}

		// For user_not_found, we don't have a userID, for others we might need to look it up
		auditLog := authDomain.NewAuditLog(userID, nil, "auth.login.failed", "user", "",
			fmt.Sprintf(`{"email": "%s", "reason": "%s"}`, req.Email, reason), "", "")
		if createErr := a.auditRepo.Create(ctx, auditLog); createErr != nil {
			a.logger.Error("Failed to create login failure audit log", "error", createErr)
		}
	} else {
		// Success audit - we can get user ID from the response context
		// For now, we'll need to look up the user or modify the service to return user info
		auditLog := authDomain.NewAuditLog(nil, nil, "auth.login.success", "user", "",
			fmt.Sprintf(`{"email": "%s"}`, req.Email), "", "")
		if createErr := a.auditRepo.Create(ctx, auditLog); createErr != nil {
			a.logger.Error("Failed to create login success audit log", "error", createErr)
		}
	}

	return resp, err
}

// RefreshToken handles token refresh with audit logging
func (a *auditDecorator) RefreshToken(ctx context.Context, req *authDomain.RefreshTokenRequest) (*authDomain.LoginResponse, error) {
	resp, err := a.authService.RefreshToken(ctx, req)

	// Audit based on result
	if err != nil {
		var reason string
		if appErr, ok := appErrors.IsAppError(err); ok {
			switch appErr.Type {
			case appErrors.UnauthorizedError:
				reason = "invalid_token"
			default:
				reason = "system_error"
			}
		} else {
			reason = "system_error"
		}

		auditLog := authDomain.NewAuditLog(nil, nil, "auth.refresh_token.failed", "token", "",
			fmt.Sprintf(`{"reason": "%s"}`, reason), "", "")
		if createErr := a.auditRepo.Create(ctx, auditLog); createErr != nil {
			a.logger.Error("Failed to create refresh token failure audit log", "error", createErr)
		}
	} else {
		// Success audit
		auditLog := authDomain.NewAuditLog(nil, nil, "auth.refresh_token.success", "token", "", `{}`, "", "")
		if createErr := a.auditRepo.Create(ctx, auditLog); createErr != nil {
			a.logger.Error("Failed to create refresh token success audit log", "error", createErr)
		}
	}

	return resp, err
}

// Logout handles user logout with audit logging
func (a *auditDecorator) Logout(ctx context.Context, jti string, userID uuid.UUID) error {
	err := a.authService.Logout(ctx, jti, userID)

	// Audit based on result
	if err != nil {
		auditLog := authDomain.NewAuditLog(&userID, nil, "auth.logout.failed", "user", userID.String(),
			fmt.Sprintf(`{"jti": "%s"}`, jti), "", "")
		if createErr := a.auditRepo.Create(ctx, auditLog); createErr != nil {
			a.logger.Error("Failed to create logout failure audit log", "error", createErr)
		}
	} else {
		// Success audit
		auditLog := authDomain.NewAuditLog(&userID, nil, "auth.logout.success", "user", userID.String(),
			fmt.Sprintf(`{"jti": "%s"}`, jti), "", "")
		if createErr := a.auditRepo.Create(ctx, auditLog); createErr != nil {
			a.logger.Error("Failed to create logout success audit log", "error", createErr)
		}
	}

	return err
}

// Delegate all other methods to the wrapped service without audit (for now)

func (a *auditDecorator) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	return a.authService.ChangePassword(ctx, userID, currentPassword, newPassword)
}

func (a *auditDecorator) ResetPassword(ctx context.Context, email string) error {
	return a.authService.ResetPassword(ctx, email)
}

func (a *auditDecorator) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	return a.authService.ConfirmPasswordReset(ctx, token, newPassword)
}

func (a *auditDecorator) SendEmailVerification(ctx context.Context, userID uuid.UUID) error {
	return a.authService.SendEmailVerification(ctx, userID)
}

func (a *auditDecorator) VerifyEmail(ctx context.Context, token string) error {
	return a.authService.VerifyEmail(ctx, token)
}

func (a *auditDecorator) ValidateAuthToken(ctx context.Context, token string) (*authDomain.AuthContext, error) {
	return a.authService.ValidateAuthToken(ctx, token)
}

func (a *auditDecorator) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	return a.authService.GetUserSessions(ctx, userID)
}

func (a *auditDecorator) RevokeSession(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID) error {
	return a.authService.RevokeSession(ctx, userID, sessionID)
}

func (a *auditDecorator) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	return a.authService.RevokeAllSessions(ctx, userID)
}

func (a *auditDecorator) RevokeUserAccessTokens(ctx context.Context, userID uuid.UUID, reason string) error {
	return a.authService.RevokeUserAccessTokens(ctx, userID, reason)
}

func (a *auditDecorator) RevokeAccessToken(ctx context.Context, jti string, userID uuid.UUID, reason string) error {
	return a.authService.RevokeAccessToken(ctx, jti, userID, reason)
}

func (a *auditDecorator) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	return a.authService.IsTokenRevoked(ctx, jti)
}

func (a *auditDecorator) GetAuthContext(ctx context.Context, token string) (*authDomain.AuthContext, error) {
	return a.authService.GetAuthContext(ctx, token)
}

// OAuth session methods - delegate without audit
func (a *auditDecorator) CreateOAuthSession(ctx context.Context, session interface{}) (string, error) {
	return a.authService.CreateOAuthSession(ctx, session)
}

func (a *auditDecorator) GetOAuthSession(ctx context.Context, sessionID string) (interface{}, error) {
	return a.authService.GetOAuthSession(ctx, sessionID)
}

func (a *auditDecorator) DeleteOAuthSession(ctx context.Context, sessionID string) error {
	return a.authService.DeleteOAuthSession(ctx, sessionID)
}

// GenerateTokensForUser delegates to wrapped service (no audit - same as Login success)
func (a *auditDecorator) GenerateTokensForUser(ctx context.Context, userID uuid.UUID) (*authDomain.LoginResponse, error) {
	return a.authService.GenerateTokensForUser(ctx, userID)
}

// OAuth login token session methods - delegate without audit
func (a *auditDecorator) CreateLoginTokenSession(ctx context.Context, accessToken, refreshToken string, expiresIn int64, userID uuid.UUID) (string, error) {
	return a.authService.CreateLoginTokenSession(ctx, accessToken, refreshToken, expiresIn, userID)
}

func (a *auditDecorator) GetLoginTokenSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return a.authService.GetLoginTokenSession(ctx, sessionID)
}
