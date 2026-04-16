package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

// OAuthSession stores incomplete user data during OAuth flow
type OAuthSession struct {
	ExpiresAt       time.Time `json:"expires_at"`
	InvitationToken *string   `json:"invitation_token,omitempty"`
	Email           string    `json:"email"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	Provider        string    `json:"provider"`
	ProviderID      string    `json:"provider_id"`
}

// CreateOAuthSession stores a temporary OAuth session in Redis
func (s *authService) CreateOAuthSession(ctx context.Context, session interface{}) (string, error) {
	oauthSession, ok := session.(*OAuthSession)
	if !ok {
		return "", appErrors.NewInternalError("Invalid session type", nil)
	}
	sessionID := uid.New().String()
	key := "oauth:session:" + sessionID

	// Set expiration
	oauthSession.ExpiresAt = time.Now().Add(15 * time.Minute)

	data, err := json.Marshal(oauthSession)
	if err != nil {
		return "", appErrors.NewInternalError("Failed to marshal OAuth session", err)
	}

	err = s.redis.Set(ctx, key, data, 15*time.Minute).Err()
	if err != nil {
		return "", appErrors.NewInternalError("Failed to store OAuth session", err)
	}

	return sessionID, nil
}

// GetOAuthSession retrieves an OAuth session from Redis
func (s *authService) GetOAuthSession(ctx context.Context, sessionID string) (interface{}, error) {
	key := "oauth:session:" + sessionID

	data, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, appErrors.NewNotFoundError("OAuth session expired or invalid")
	}

	var session OAuthSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, appErrors.NewInternalError("Failed to unmarshal OAuth session", err)
	}

	// Check expiration
	if time.Now().After(session.ExpiresAt) {
		return nil, appErrors.NewNotFoundError("OAuth session has expired")
	}

	return &session, nil
}

// GetOAuthSessionTyped is a helper to get the session with proper type
func (s *authService) GetOAuthSessionTyped(ctx context.Context, sessionID string) (*OAuthSession, error) {
	sessionInterface, err := s.GetOAuthSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	session, ok := sessionInterface.(*OAuthSession)
	if !ok {
		return nil, appErrors.NewInternalError("Invalid session type", nil)
	}

	return session, nil
}

// DeleteOAuthSession removes an OAuth session from Redis
func (s *authService) DeleteOAuthSession(ctx context.Context, sessionID string) error {
	key := "oauth:session:" + sessionID
	return s.redis.Del(ctx, key).Err()
}

// CreateLoginTokenSession stores login tokens temporarily for OAuth callback redirect.
// Returns a one-time session ID that frontend can exchange for tokens.
// Used when existing OAuth users login (not signup).
func (s *authService) CreateLoginTokenSession(ctx context.Context, accessToken, refreshToken string, expiresIn int64, userID uuid.UUID) (string, error) {
	sessionID := uid.New().String()
	key := "oauth:login:" + sessionID

	sessionData := map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    expiresIn,
		"user_id":       userID.String(),
		"created_at":    time.Now().Unix(),
	}

	data, err := json.Marshal(sessionData)
	if err != nil {
		return "", appErrors.NewInternalError("Failed to marshal login session", err)
	}

	// Store with 5-minute expiration (one-time use)
	err = s.redis.Set(ctx, key, data, 5*time.Minute).Err()
	if err != nil {
		return "", appErrors.NewInternalError("Failed to store login session", err)
	}

	return sessionID, nil
}

// GetLoginTokenSession retrieves login tokens from Redis (one-time use).
// After retrieval, the session is deleted.
func (s *authService) GetLoginTokenSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	key := "oauth:login:" + sessionID

	data, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, appErrors.NewNotFoundError("Login session expired or invalid")
	}

	// Delete session immediately (one-time use for security)
	s.redis.Del(ctx, key)

	var sessionData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &sessionData); err != nil {
		return nil, appErrors.NewInternalError("Failed to unmarshal login session", err)
	}

	return sessionData, nil
}
