package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/pkg/pagination"
)

// blacklistedTokenRepository implements authDomain.BlacklistedTokenRepository using GORM
type blacklistedTokenRepository struct {
	db *gorm.DB
}

// NewBlacklistedTokenRepository creates a new blacklisted token repository instance
func NewBlacklistedTokenRepository(db *gorm.DB) authDomain.BlacklistedTokenRepository {
	return &blacklistedTokenRepository{
		db: db,
	}
}

// Create adds a new token to the blacklist
func (r *blacklistedTokenRepository) Create(ctx context.Context, blacklistedToken *authDomain.BlacklistedToken) error {
	return r.db.WithContext(ctx).Create(blacklistedToken).Error
}

// GetByJTI retrieves a blacklisted token by JWT ID
func (r *blacklistedTokenRepository) GetByJTI(ctx context.Context, jti string) (*authDomain.BlacklistedToken, error) {
	var token authDomain.BlacklistedToken
	err := r.db.WithContext(ctx).Where("jti = ?", jti).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get blacklisted token by JTI %s: %w", jti, authDomain.ErrNotFound)
		}
		return nil, err
	}
	return &token, nil
}

// IsTokenBlacklisted checks if a token is blacklisted (optimized for fast lookup)
func (r *blacklistedTokenRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&authDomain.BlacklistedToken{}).
		Where("jti = ? AND expires_at > ?", jti, time.Now()).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// CleanupExpiredTokens removes tokens that have naturally expired
func (r *blacklistedTokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Delete(&authDomain.BlacklistedToken{}, "expires_at <= ?", time.Now()).
		Error
}

// CleanupTokensOlderThan removes tokens older than specified time
func (r *blacklistedTokenRepository) CleanupTokensOlderThan(ctx context.Context, olderThan time.Time) error {
	return r.db.WithContext(ctx).
		Delete(&authDomain.BlacklistedToken{}, "created_at < ?", olderThan).
		Error
}

// CreateUserTimestampBlacklist creates a user-wide timestamp blacklist entry for GDPR/SOC2 compliance
func (r *blacklistedTokenRepository) CreateUserTimestampBlacklist(ctx context.Context, userID uuid.UUID, blacklistTimestamp int64, reason string) error {
	blacklistedToken := authDomain.NewUserTimestampBlacklistedToken(userID, blacklistTimestamp, reason)
	return r.db.WithContext(ctx).Create(blacklistedToken).Error
}

// IsUserBlacklistedAfterTimestamp checks if a user is blacklisted after a specific timestamp
func (r *blacklistedTokenRepository) IsUserBlacklistedAfterTimestamp(ctx context.Context, userID uuid.UUID, tokenIssuedAt int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&authDomain.BlacklistedToken{}).
		Where("user_id = ? AND token_type = ? AND blacklist_timestamp IS NOT NULL AND ? < blacklist_timestamp AND expires_at > ?",
			userID, authDomain.TokenTypeUserTimestamp, tokenIssuedAt, time.Now()).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetUserBlacklistTimestamp gets the latest blacklist timestamp for a user
func (r *blacklistedTokenRepository) GetUserBlacklistTimestamp(ctx context.Context, userID uuid.UUID) (*int64, error) {
	var token authDomain.BlacklistedToken
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND token_type = ? AND blacklist_timestamp IS NOT NULL AND expires_at > ?",
			userID, authDomain.TokenTypeUserTimestamp, time.Now()).
		Order("blacklist_timestamp DESC").
		First(&token).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No user-wide blacklist found
		}
		return nil, err
	}

	return token.BlacklistTimestamp, nil
}

// BlacklistUserTokens adds all active tokens for a user to blacklist (legacy method, now replaced by timestamp approach)
func (r *blacklistedTokenRepository) BlacklistUserTokens(ctx context.Context, userID uuid.UUID, reason string) error {
	// DEPRECATED: This method is now replaced by CreateUserTimestampBlacklist for GDPR/SOC2 compliance
	// Create a user-wide timestamp blacklist instead of trying to track individual JTIs
	blacklistTimestamp := time.Now().Unix()
	return r.CreateUserTimestampBlacklist(ctx, userID, blacklistTimestamp, reason)
}

// GetBlacklistedTokensByUser retrieves blacklisted tokens with cursor pagination
func (r *blacklistedTokenRepository) GetBlacklistedTokensByUser(ctx context.Context, filters *authDomain.BlacklistedTokenFilter) ([]*authDomain.BlacklistedToken, error) {
	var tokens []*authDomain.BlacklistedToken

	query := r.db.WithContext(ctx)

	// Apply filters
	if filters != nil {
		if filters.UserID != nil {
			query = query.Where("user_id = ?", *filters.UserID)
		}
		if filters.Reason != nil {
			query = query.Where("reason = ?", *filters.Reason)
		}
	}

	// Determine sort field and direction with validation
	allowedSortFields := []string{"created_at", "expires_at", "reason", "id"}
	sortField := "created_at" // default
	sortDir := "DESC"

	if filters != nil {
		// Validate sort field against whitelist
		if filters.Params.SortBy != "" {
			validated, err := pagination.ValidateSortField(filters.Params.SortBy, allowedSortFields)
			if err != nil {
				return nil, err
			}
			if validated != "" {
				sortField = validated
			}
		}
		if filters.Params.SortDir == "asc" {
			sortDir = "ASC"
		}
	}

	// Apply sorting with secondary sort on id for stable ordering
	query = query.Order(fmt.Sprintf("%s %s, id %s", sortField, sortDir, sortDir))

	// Apply limit and offset for pagination
	limit := pagination.DefaultPageSize
	offset := 0
	if filters != nil {
		if filters.Params.Limit > 0 {
			limit = filters.Params.Limit
		}
		offset = filters.Params.GetOffset()
	}
	query = query.Limit(limit).Offset(offset)

	err := query.Find(&tokens).Error
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetBlacklistedTokensCount returns the total count of blacklisted tokens
func (r *blacklistedTokenRepository) GetBlacklistedTokensCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&authDomain.BlacklistedToken{}).Count(&count).Error
	return count, err
}

// GetBlacklistedTokensByReason retrieves tokens blacklisted for a specific reason
func (r *blacklistedTokenRepository) GetBlacklistedTokensByReason(ctx context.Context, reason string) ([]*authDomain.BlacklistedToken, error) {
	var tokens []*authDomain.BlacklistedToken
	err := r.db.WithContext(ctx).Where("reason = ?", reason).
		Order("created_at DESC").Find(&tokens).Error

	if err != nil {
		return nil, err
	}

	return tokens, nil
}
