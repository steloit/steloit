package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

// CreateOAuthSession stores a temporary OAuth session in Redis.
func (s *authService) CreateOAuthSession(ctx context.Context, session *authDomain.OAuthSession) (string, error) {
	if session == nil {
		return "", appErrors.NewInternalError("nil OAuth session", nil)
	}

	sessionID := uid.New().String()
	key := "oauth:session:" + sessionID

	session.ExpiresAt = time.Now().Add(15 * time.Minute)

	data, err := json.Marshal(session)
	if err != nil {
		return "", appErrors.NewInternalError("Failed to marshal OAuth session", err)
	}
	if err := s.redis.Set(ctx, key, data, 15*time.Minute).Err(); err != nil {
		return "", appErrors.NewInternalError("Failed to store OAuth session", err)
	}
	return sessionID, nil
}

// GetOAuthSession retrieves an OAuth session from Redis. Returns a
// NotFound error when the session is missing, expired, or unreadable.
func (s *authService) GetOAuthSession(ctx context.Context, sessionID string) (*authDomain.OAuthSession, error) {
	key := "oauth:session:" + sessionID

	data, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, appErrors.NewNotFoundError("OAuth session expired or invalid")
	}

	var session authDomain.OAuthSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, appErrors.NewInternalError("Failed to unmarshal OAuth session", err)
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, appErrors.NewNotFoundError("OAuth session has expired")
	}
	return &session, nil
}

// DeleteOAuthSession removes an OAuth session from Redis.
func (s *authService) DeleteOAuthSession(ctx context.Context, sessionID string) error {
	key := "oauth:session:" + sessionID
	return s.redis.Del(ctx, key).Err()
}

// CreateLoginTokenSession stores login tokens temporarily for OAuth
// callback redirect. Returns a one-time session ID that the frontend can
// exchange for tokens. Used when existing OAuth users log in (not sign up).
func (s *authService) CreateLoginTokenSession(ctx context.Context, accessToken, refreshToken string, expiresIn int64, userID uuid.UUID) (string, error) {
	sessionID := uid.New().String()
	key := "oauth:login:" + sessionID

	payload := authDomain.LoginTokenSession{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
		UserID:       userID,
		CreatedAt:    time.Now(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", appErrors.NewInternalError("Failed to marshal login session", err)
	}
	if err := s.redis.Set(ctx, key, data, 5*time.Minute).Err(); err != nil {
		return "", appErrors.NewInternalError("Failed to store login session", err)
	}
	return sessionID, nil
}

// GetLoginTokenSession retrieves login tokens from Redis and deletes the
// session immediately (one-time use). Returns NotFound when the session is
// missing, expired, or unreadable.
func (s *authService) GetLoginTokenSession(ctx context.Context, sessionID string) (*authDomain.LoginTokenSession, error) {
	key := "oauth:login:" + sessionID

	data, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, appErrors.NewNotFoundError("Login session expired or invalid")
	}

	// One-time use — delete immediately regardless of subsequent decode.
	s.redis.Del(ctx, key)

	var out authDomain.LoginTokenSession
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return nil, appErrors.NewInternalError("Failed to unmarshal login session", err)
	}
	return &out, nil
}
