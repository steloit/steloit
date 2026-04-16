package credentials

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	credentialsDomain "brokle/internal/core/domain/credentials"
)

type providerCredentialRepository struct {
	db *gorm.DB
}

func NewProviderCredentialRepository(db *gorm.DB) credentialsDomain.ProviderCredentialRepository {
	return &providerCredentialRepository{
		db: db,
	}
}

func (r *providerCredentialRepository) Create(ctx context.Context, credential *credentialsDomain.ProviderCredential) error {
	err := r.db.WithContext(ctx).Create(credential).Error
	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return fmt.Errorf("create credential: %w", credentialsDomain.ErrCredentialExists)
		}
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

// GetByID retrieves a credential by its ID within a specific organization.
// Returns ErrCredentialNotFound if not found or belongs to different organization.
func (r *providerCredentialRepository) GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*credentialsDomain.ProviderCredential, error) {
	var credential credentialsDomain.ProviderCredential
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, orgID).
		First(&credential).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get credential by ID %s: %w", id, credentialsDomain.ErrCredentialNotFound)
		}
		return nil, fmt.Errorf("get credential by ID: %w", err)
	}
	return &credential, nil
}

// GetByOrgAndName retrieves the credential for a specific organization and name.
// Returns nil if not found.
func (r *providerCredentialRepository) GetByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (*credentialsDomain.ProviderCredential, error) {
	var credential credentialsDomain.ProviderCredential
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND name = ?", orgID, name).
		First(&credential).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil for uniqueness checks (not found = available)
		}
		return nil, fmt.Errorf("get credential by name: %w", err)
	}
	return &credential, nil
}

// GetByOrgAndAdapter retrieves all credentials for a specific organization and adapter type.
// Returns empty slice if none found.
func (r *providerCredentialRepository) GetByOrgAndAdapter(ctx context.Context, orgID uuid.UUID, adapter credentialsDomain.Provider) ([]*credentialsDomain.ProviderCredential, error) {
	var credentials []*credentialsDomain.ProviderCredential
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND adapter = ?", orgID, adapter).
		Order("created_at DESC").
		Find(&credentials).Error
	if err != nil {
		return nil, fmt.Errorf("get credentials by adapter: %w", err)
	}
	return credentials, nil
}

func (r *providerCredentialRepository) ListByOrganization(ctx context.Context, orgID uuid.UUID) ([]*credentialsDomain.ProviderCredential, error) {
	var credentials []*credentialsDomain.ProviderCredential
	err := r.db.WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&credentials).Error
	if err != nil {
		return nil, fmt.Errorf("list credentials for organization %s: %w", orgID, err)
	}
	return credentials, nil
}

// Update updates an existing credential within a specific organization.
// Returns ErrCredentialNotFound if not found or belongs to different organization.
func (r *providerCredentialRepository) Update(ctx context.Context, credential *credentialsDomain.ProviderCredential, orgID uuid.UUID) error {
	credential.UpdatedAt = time.Now()

	// Use organization-scoped update to prevent cross-organization modification
	result := r.db.WithContext(ctx).
		Model(&credentialsDomain.ProviderCredential{}).
		Where("id = ? AND organization_id = ?", credential.ID, orgID).
		Updates(map[string]interface{}{
			"name":          credential.Name,
			"encrypted_key": credential.EncryptedKey,
			"key_preview":   credential.KeyPreview,
			"base_url":      credential.BaseURL,
			"config":        credential.Config,
			"custom_models": credential.CustomModels,
			"headers":       credential.Headers,
			"updated_at":    credential.UpdatedAt,
		})

	if result.Error != nil {
		// Check for unique constraint violation (name conflict)
		if isUniqueViolation(result.Error) {
			return fmt.Errorf("update credential: %w", credentialsDomain.ErrCredentialExists)
		}
		return fmt.Errorf("update credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update credential: %w", credentialsDomain.ErrCredentialNotFound)
	}
	return nil
}

// Delete removes a credential by ID within a specific organization.
// Returns ErrCredentialNotFound if not found or belongs to different organization.
func (r *providerCredentialRepository) Delete(ctx context.Context, id uuid.UUID, orgID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, orgID).
		Delete(&credentialsDomain.ProviderCredential{})
	if result.Error != nil {
		return fmt.Errorf("delete credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("delete credential %s: %w", id, credentialsDomain.ErrCredentialNotFound)
	}
	return nil
}

func (r *providerCredentialRepository) ExistsByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&credentialsDomain.ProviderCredential{}).
		Where("organization_id = ? AND name = ?", orgID, name).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check credential exists: %w", err)
	}
	return count > 0, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL unique violation error code: 23505
	errStr := err.Error()
	return strings.Contains(errStr, "23505") || strings.Contains(errStr, "unique constraint") || strings.Contains(errStr, "duplicate key")
}
