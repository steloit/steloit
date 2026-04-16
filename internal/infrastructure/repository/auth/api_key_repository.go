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

// apiKeyRepository implements authDomain.APIKeyRepository using GORM
type apiKeyRepository struct {
	db *gorm.DB
}

// NewAPIKeyRepository creates a new API key repository instance
func NewAPIKeyRepository(db *gorm.DB) authDomain.APIKeyRepository {
	return &apiKeyRepository{
		db: db,
	}
}

// Create creates a new API key
func (r *apiKeyRepository) Create(ctx context.Context, apiKey *authDomain.APIKey) error {
	return r.db.WithContext(ctx).Create(apiKey).Error
}

// GetByID retrieves an API key by ID
func (r *apiKeyRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.APIKey, error) {
	var apiKey authDomain.APIKey
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&apiKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get API key by ID %s: %w", id, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("database error getting API key by ID %s: %w", id, err)
	}
	return &apiKey, nil
}

// GetByKeyHash retrieves an API key by its SHA-256 hash (industry-standard direct lookup)
// This enables O(1) validation with unique index on key_hash column
// Note: We use SHA-256 (not bcrypt) because it's deterministic, enabling direct hash lookups
func (r *apiKeyRepository) GetByKeyHash(ctx context.Context, keyHash string) (*authDomain.APIKey, error) {
	var apiKey authDomain.APIKey
	err := r.db.WithContext(ctx).Where("key_hash = ? AND deleted_at IS NULL", keyHash).First(&apiKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get API key by hash: %w", authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("database error getting API key by hash: %w", err)
	}
	return &apiKey, nil
}

// Update updates an API key
func (r *apiKeyRepository) Update(ctx context.Context, apiKey *authDomain.APIKey) error {
	return r.db.WithContext(ctx).Save(apiKey).Error
}

// Delete soft deletes an API key
func (r *apiKeyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&authDomain.APIKey{}).Where("id = ?", id).Update("deleted_at", time.Now()).Error
}

// UpdateLastUsed updates the last used timestamp
func (r *apiKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&authDomain.APIKey{}).
		Where("id = ?", id).
		Update("last_used_at", time.Now()).Error
}

// GetByUserID retrieves API keys for a user
func (r *apiKeyRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.APIKey, error) {
	var apiKeys []*authDomain.APIKey
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Find(&apiKeys).Error
	return apiKeys, err
}

// GetByOrganizationID retrieves API keys for an organization (via project JOIN)
func (r *apiKeyRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*authDomain.APIKey, error) {
	var apiKeys []*authDomain.APIKey
	err := r.db.WithContext(ctx).
		Joins("JOIN projects ON api_keys.project_id = projects.id").
		Where("projects.organization_id = ? AND api_keys.deleted_at IS NULL", orgID).
		Order("api_keys.created_at DESC").
		Find(&apiKeys).Error
	return apiKeys, err
}

// GetByProjectID retrieves API keys for a project
func (r *apiKeyRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID) ([]*authDomain.APIKey, error) {
	var apiKeys []*authDomain.APIKey
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND deleted_at IS NULL", projectID).
		Order("created_at DESC").
		Find(&apiKeys).Error
	return apiKeys, err
}

// GetByFilters retrieves API keys based on filters
func (r *apiKeyRepository) GetByFilters(ctx context.Context, filters *authDomain.APIKeyFilters) ([]*authDomain.APIKey, error) {
	var apiKeys []*authDomain.APIKey
	query := r.db.WithContext(ctx).Where("deleted_at IS NULL")

	// Apply filters
	if filters.UserID != nil {
		query = query.Where("user_id = ?", *filters.UserID)
	}
	if filters.OrganizationID != nil {
		// Organization filter requires JOIN with projects table
		query = query.Joins("JOIN projects ON api_keys.project_id = projects.id").
			Where("projects.organization_id = ?", *filters.OrganizationID)
	}
	if filters.ProjectID != nil {
		query = query.Where("project_id = ?", *filters.ProjectID)
	}
	if filters.IsExpired != nil {
		if *filters.IsExpired {
			// Filter for expired keys
			query = query.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now())
		} else {
			// Filter for active (non-expired) keys
			query = query.Where("expires_at IS NULL OR expires_at > ?", time.Now())
		}
	}

	// Determine sort field and direction with validation
	allowedSortFields := []string{"created_at", "updated_at", "name", "expires_at", "last_used_at", "id"}
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
	if filters.Params.Limit > 0 {
		limit = filters.Params.Limit
	}
	offset := filters.Params.GetOffset()
	query = query.Limit(limit).Offset(offset)

	err := query.Find(&apiKeys).Error
	return apiKeys, err
}

// CountByFilters returns the total count of API keys matching the filters
func (r *apiKeyRepository) CountByFilters(ctx context.Context, filters *authDomain.APIKeyFilters) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&authDomain.APIKey{}).Where("deleted_at IS NULL")

	// Apply the same filters as GetByFilters
	if filters.UserID != nil {
		query = query.Where("user_id = ?", *filters.UserID)
	}
	if filters.OrganizationID != nil {
		// Organization filter requires JOIN with projects table
		query = query.Joins("JOIN projects ON api_keys.project_id = projects.id").
			Where("projects.organization_id = ?", *filters.OrganizationID)
	}
	if filters.ProjectID != nil {
		query = query.Where("project_id = ?", *filters.ProjectID)
	}
	if filters.IsExpired != nil {
		if *filters.IsExpired {
			// Count expired keys
			query = query.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now())
		} else {
			// Count active (non-expired) keys
			query = query.Where("expires_at IS NULL OR expires_at > ?", time.Now())
		}
	}

	err := query.Count(&count).Error
	return count, err
}

// CleanupExpiredAPIKeys removes expired API keys
func (r *apiKeyRepository) CleanupExpiredAPIKeys(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Delete(&authDomain.APIKey{}).Error
}

// GetAPIKeyCount returns the total count of API keys for a user
func (r *apiKeyRepository) GetAPIKeyCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&authDomain.APIKey{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&count).Error
	return int(count), err
}

// GetActiveAPIKeyCount returns the count of active (non-expired, non-deleted) API keys for a user
func (r *apiKeyRepository) GetActiveAPIKeyCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&authDomain.APIKey{}).
		Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?) AND deleted_at IS NULL", userID, time.Now()).
		Count(&count).Error
	return int(count), err
}
