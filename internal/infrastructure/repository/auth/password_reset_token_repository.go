package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
)

// passwordResetTokenRepository implements authDomain.PasswordResetTokenRepository using GORM
type passwordResetTokenRepository struct {
	db *gorm.DB
}

// NewPasswordResetTokenRepository creates a new password reset token repository instance
func NewPasswordResetTokenRepository(db *gorm.DB) authDomain.PasswordResetTokenRepository {
	return &passwordResetTokenRepository{
		db: db,
	}
}

// Create creates a new password reset token
func (r *passwordResetTokenRepository) Create(ctx context.Context, token *authDomain.PasswordResetToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// GetByID retrieves a password reset token by ID
func (r *passwordResetTokenRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.PasswordResetToken, error) {
	var token authDomain.PasswordResetToken
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get password reset token: %w", authDomain.ErrNotFound)
		}
		return nil, err
	}
	return &token, nil
}

// GetByToken retrieves a password reset token by token string
func (r *passwordResetTokenRepository) GetByToken(ctx context.Context, tokenStr string) (*authDomain.PasswordResetToken, error) {
	var token authDomain.PasswordResetToken
	err := r.db.WithContext(ctx).Where("token = ?", tokenStr).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get password reset token: %w", authDomain.ErrNotFound)
		}
		return nil, err
	}
	return &token, nil
}

// GetByUserID retrieves all password reset tokens for a user
func (r *passwordResetTokenRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.PasswordResetToken, error) {
	var tokens []*authDomain.PasswordResetToken
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// Update updates an existing password reset token
func (r *passwordResetTokenRepository) Update(ctx context.Context, token *authDomain.PasswordResetToken) error {
	return r.db.WithContext(ctx).Save(token).Error
}

// Delete deletes a password reset token by ID
func (r *passwordResetTokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&authDomain.PasswordResetToken{}, "id = ?", id).Error
}

// MarkAsUsed marks a password reset token as used by setting the used_at timestamp
func (r *passwordResetTokenRepository) MarkAsUsed(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&authDomain.PasswordResetToken{}).Where("id = ?", id).Updates(map[string]interface{}{
		"used_at":    now,
		"updated_at": now,
	}).Error
}

// IsUsed checks if a password reset token is used
func (r *passwordResetTokenRepository) IsUsed(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&authDomain.PasswordResetToken{}).Where("id = ? AND used_at IS NOT NULL", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsValid checks if a password reset token is valid (not used and not expired)
func (r *passwordResetTokenRepository) IsValid(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&authDomain.PasswordResetToken{}).Where("id = ? AND used_at IS NULL AND expires_at > ?", id, time.Now()).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetValidTokenByUserID retrieves the most recent valid password reset token for a user
func (r *passwordResetTokenRepository) GetValidTokenByUserID(ctx context.Context, userID uuid.UUID) (*authDomain.PasswordResetToken, error) {
	var token authDomain.PasswordResetToken
	err := r.db.WithContext(ctx).Where("user_id = ? AND used_at IS NULL AND expires_at > ?", userID, time.Now()).Order("created_at DESC").First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get valid password reset token for user %s: %w", userID, authDomain.ErrNotFound)
		}
		return nil, err
	}
	return &token, nil
}

// CleanupExpiredTokens removes expired password reset tokens
func (r *passwordResetTokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	return r.db.WithContext(ctx).Delete(&authDomain.PasswordResetToken{}, "expires_at < ?", time.Now()).Error
}

// CleanupUsedTokens removes used password reset tokens older than the specified time
func (r *passwordResetTokenRepository) CleanupUsedTokens(ctx context.Context, olderThan time.Time) error {
	return r.db.WithContext(ctx).Delete(&authDomain.PasswordResetToken{}, "used_at IS NOT NULL AND used_at < ?", olderThan).Error
}

// InvalidateAllUserTokens marks all existing tokens for a user as used (invalidates them)
func (r *passwordResetTokenRepository) InvalidateAllUserTokens(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&authDomain.PasswordResetToken{}).Where("user_id = ? AND used_at IS NULL", userID).Updates(map[string]interface{}{
		"used_at":    now,
		"updated_at": now,
	}).Error
}
