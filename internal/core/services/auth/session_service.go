package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"

	"brokle/internal/config"
	authDomain "brokle/internal/core/domain/auth"
	userDomain "brokle/internal/core/domain/user"
	appErrors "brokle/pkg/errors"
)

// sessionService implements the auth.SessionService interface
type sessionService struct {
	authConfig  *config.AuthConfig
	sessionRepo authDomain.UserSessionRepository
	userRepo    userDomain.Repository
	jwtService  authDomain.JWTService
}

// NewSessionService creates a new session service instance
func NewSessionService(
	authConfig *config.AuthConfig,
	sessionRepo authDomain.UserSessionRepository,
	userRepo userDomain.Repository,
	jwtService authDomain.JWTService,
) authDomain.SessionService {
	return &sessionService{
		authConfig:  authConfig,
		sessionRepo: sessionRepo,
		userRepo:    userRepo,
		jwtService:  jwtService,
	}
}

// GetSession retrieves a session by ID
func (s *sessionService) GetSession(ctx context.Context, sessionID uuid.UUID) (*authDomain.UserSession, error) {
	return s.sessionRepo.GetByID(ctx, sessionID)
}

// RevokeSession revokes a specific session
func (s *sessionService) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return appErrors.NewNotFoundError("Session not found")
	}

	err = s.sessionRepo.RevokeSession(ctx, sessionID)
	if err != nil {
		return appErrors.NewInternalError("Failed to revoke session", err)
	}

	return nil
}

// GetUserSessions retrieves all sessions for a user
func (s *sessionService) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	return s.sessionRepo.GetByUserID(ctx, userID)
}

// RevokeUserSessions revokes all sessions for a user
func (s *sessionService) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	err := s.sessionRepo.RevokeUserSessions(ctx, userID)
	if err != nil {
		return appErrors.NewInternalError("Failed to revoke user sessions", err)
	}

	return nil
}

// CleanupExpiredSessions removes expired sessions from the database
func (s *sessionService) CleanupExpiredSessions(ctx context.Context) error {
	return s.sessionRepo.CleanupExpiredSessions(ctx)
}

// GetActiveSessions retrieves only active sessions for a user
func (s *sessionService) GetActiveSessions(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	return s.sessionRepo.GetActiveSessionsByUserID(ctx, userID)
}

// hashToken creates a SHA-256 hash of a token for secure storage
func (s *sessionService) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
