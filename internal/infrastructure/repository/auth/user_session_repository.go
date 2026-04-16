package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
)

// userSessionRepository implements authDomain.UserSessionRepository using GORM
type userSessionRepository struct {
	db *gorm.DB
}

// NewUserSessionRepository creates a new user session repository instance
func NewUserSessionRepository(db *gorm.DB) authDomain.UserSessionRepository {
	return &userSessionRepository{
		db: db,
	}
}

// Create creates a new user session
func (r *userSessionRepository) Create(ctx context.Context, session *authDomain.UserSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// GetByID retrieves a user session by ID
func (r *userSessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.UserSession, error) {
	var session authDomain.UserSession
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get user session by ID %s: %w", id, authDomain.ErrSessionNotFound)
		}
		return nil, err
	}
	return &session, nil
}

// GetByJTI retrieves a user session by JWT ID (current access token JTI)
func (r *userSessionRepository) GetByJTI(ctx context.Context, jti string) (*authDomain.UserSession, error) {
	var session authDomain.UserSession
	err := r.db.WithContext(ctx).Where("current_jti = ? AND is_active = ? AND revoked_at IS NULL", jti, true).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get user session by JTI %s: %w", jti, authDomain.ErrSessionNotFound)
		}
		return nil, err
	}
	return &session, nil
}

// GetByRefreshTokenHash retrieves a user session by refresh token hash
func (r *userSessionRepository) GetByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (*authDomain.UserSession, error) {
	var session authDomain.UserSession
	err := r.db.WithContext(ctx).Where("refresh_token_hash = ? AND is_active = ? AND revoked_at IS NULL", refreshTokenHash, true).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get user session by refresh token hash: %w", authDomain.ErrSessionNotFound)
		}
		return nil, err
	}
	return &session, nil
}

// Update updates an existing user session
func (r *userSessionRepository) Update(ctx context.Context, session *authDomain.UserSession) error {
	return r.db.WithContext(ctx).Save(session).Error
}

// Delete deletes a user session by ID (hard delete)
func (r *userSessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&authDomain.UserSession{}, "id = ?", id).Error
}

// GetByUserID retrieves all sessions for a user
func (r *userSessionRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	var sessions []*authDomain.UserSession
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetActiveSessionsByUserID retrieves active sessions for a user
func (r *userSessionRepository) GetActiveSessionsByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	var sessions []*authDomain.UserSession
	err := r.db.WithContext(ctx).Where("user_id = ? AND is_active = ? AND revoked_at IS NULL AND expires_at > ?", userID, true, time.Now()).Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// DeactivateSession deactivates a session without revoking it
func (r *userSessionRepository) DeactivateSession(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&authDomain.UserSession{}).Where("id = ?", id).Update("is_active", false).Error
}

// DeactivateUserSessions deactivates all sessions for a user
func (r *userSessionRepository) DeactivateUserSessions(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&authDomain.UserSession{}).Where("user_id = ?", userID).Update("is_active", false).Error
}

// RevokeSession revokes a session (sets revoked_at timestamp)
func (r *userSessionRepository) RevokeSession(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&authDomain.UserSession{}).Where("id = ?", id).Updates(map[string]interface{}{
		"revoked_at": now,
		"is_active":  false,
		"updated_at": now,
	}).Error
}

// RevokeUserSessions revokes all sessions for a user
func (r *userSessionRepository) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&authDomain.UserSession{}).Where("user_id = ? AND revoked_at IS NULL", userID).Updates(map[string]interface{}{
		"revoked_at": now,
		"is_active":  false,
		"updated_at": now,
	}).Error
}

// CleanupExpiredSessions removes expired sessions
func (r *userSessionRepository) CleanupExpiredSessions(ctx context.Context) error {
	return r.db.WithContext(ctx).Delete(&authDomain.UserSession{}, "expires_at < ?", time.Now()).Error
}

// CleanupRevokedSessions removes revoked sessions older than 30 days
func (r *userSessionRepository) CleanupRevokedSessions(ctx context.Context) error {
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	return r.db.WithContext(ctx).Delete(&authDomain.UserSession{}, "revoked_at IS NOT NULL AND revoked_at < ?", thirtyDaysAgo).Error
}

// MarkAsUsed updates the last_used_at timestamp
func (r *userSessionRepository) MarkAsUsed(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&authDomain.UserSession{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_used_at": now,
		"updated_at":   now,
	}).Error
}

// GetByDeviceInfo retrieves sessions by user ID and device info
func (r *userSessionRepository) GetByDeviceInfo(ctx context.Context, userID uuid.UUID, deviceInfo interface{}) ([]*authDomain.UserSession, error) {
	var sessions []*authDomain.UserSession

	// Convert device info to JSON for comparison
	deviceJSON, err := json.Marshal(deviceInfo)
	if err != nil {
		return nil, err
	}

	err = r.db.WithContext(ctx).Where("user_id = ? AND device_info::jsonb = ?::jsonb", userID, string(deviceJSON)).Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetActiveSessionsCount returns the count of active sessions for a user
func (r *userSessionRepository) GetActiveSessionsCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&authDomain.UserSession{}).Where("user_id = ? AND is_active = ? AND revoked_at IS NULL AND expires_at > ?", userID, true, time.Now()).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
