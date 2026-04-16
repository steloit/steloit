package organization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/google/uuid"

	orgDomain "brokle/internal/core/domain/organization"
)

// organizationSettingsRepository implements orgDomain.OrganizationSettingsRepository using GORM
type organizationSettingsRepository struct {
	db *gorm.DB
}

// NewOrganizationSettingsRepository creates a new organization settings repository instance
func NewOrganizationSettingsRepository(db *gorm.DB) orgDomain.OrganizationSettingsRepository {
	return &organizationSettingsRepository{
		db: db,
	}
}

// Create creates a new organization setting
func (r *organizationSettingsRepository) Create(ctx context.Context, setting *orgDomain.OrganizationSettings) error {
	return r.db.WithContext(ctx).Create(setting).Error
}

// GetByID retrieves an organization setting by ID
func (r *organizationSettingsRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.OrganizationSettings, error) {
	var setting orgDomain.OrganizationSettings
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get organization setting by ID %s: %w", id, orgDomain.ErrSettingsNotFound)
		}
		return nil, err
	}
	return &setting, nil
}

// GetByKey retrieves an organization setting by organization ID and key
func (r *organizationSettingsRepository) GetByKey(ctx context.Context, orgID uuid.UUID, key string) (*orgDomain.OrganizationSettings, error) {
	var setting orgDomain.OrganizationSettings
	err := r.db.WithContext(ctx).Where("organization_id = ? AND key = ?", orgID, key).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get organization setting by org %s and key %s: %w", orgID, key, orgDomain.ErrSettingsNotFound)
		}
		return nil, err
	}
	return &setting, nil
}

// Update updates an existing organization setting
func (r *organizationSettingsRepository) Update(ctx context.Context, setting *orgDomain.OrganizationSettings) error {
	return r.db.WithContext(ctx).Save(setting).Error
}

// Delete deletes an organization setting by ID
func (r *organizationSettingsRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&orgDomain.OrganizationSettings{}, "id = ?", id).Error
}

// GetAllByOrganizationID retrieves all settings for an organization
func (r *organizationSettingsRepository) GetAllByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.OrganizationSettings, error) {
	var settings []*orgDomain.OrganizationSettings
	err := r.db.WithContext(ctx).Where("organization_id = ?", orgID).Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// GetSettingsMap retrieves all settings for an organization as a map[string]interface{}
func (r *organizationSettingsRepository) GetSettingsMap(ctx context.Context, orgID uuid.UUID) (map[string]interface{}, error) {
	settings, err := r.GetAllByOrganizationID(ctx, orgID)
	if err != nil {
		return nil, err
	}

	settingsMap := make(map[string]interface{})
	for _, setting := range settings {
		var value interface{}
		if err := json.Unmarshal([]byte(setting.Value), &value); err != nil {
			// If unmarshaling fails, store as string
			value = setting.Value
		}
		settingsMap[setting.Key] = value
	}

	return settingsMap, nil
}

// DeleteByKey deletes an organization setting by organization ID and key
func (r *organizationSettingsRepository) DeleteByKey(ctx context.Context, orgID uuid.UUID, key string) error {
	return r.db.WithContext(ctx).Delete(&orgDomain.OrganizationSettings{}, "organization_id = ? AND key = ?", orgID, key).Error
}

// UpsertSetting creates or updates a setting by organization ID and key
func (r *organizationSettingsRepository) UpsertSetting(ctx context.Context, orgID uuid.UUID, key string, value interface{}) (*orgDomain.OrganizationSettings, error) {
	// Try to get existing setting
	existing, err := r.GetByKey(ctx, orgID, key)
	if err != nil && !errors.Is(err, orgDomain.ErrSettingsNotFound) {
		return nil, err
	}

	// Convert value to JSON for validation
	_, err = json.Marshal(value)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// Update existing
		if err := existing.SetValue(value); err != nil {
			return nil, err
		}
		if err := r.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	} else {
		// Create new
		setting, err := orgDomain.NewOrganizationSettings(orgID, key, value)
		if err != nil {
			return nil, err
		}
		if err := r.Create(ctx, setting); err != nil {
			return nil, err
		}
		return setting, nil
	}
}

// CreateMultiple creates multiple organization settings in a transaction
func (r *organizationSettingsRepository) CreateMultiple(ctx context.Context, settings []*orgDomain.OrganizationSettings) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, setting := range settings {
			if err := tx.Create(setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetByKeys retrieves settings by organization ID and multiple keys
func (r *organizationSettingsRepository) GetByKeys(ctx context.Context, orgID uuid.UUID, keys []string) ([]*orgDomain.OrganizationSettings, error) {
	var settings []*orgDomain.OrganizationSettings
	err := r.db.WithContext(ctx).Where("organization_id = ? AND key IN ?", orgID, keys).Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// DeleteMultiple deletes multiple settings by organization ID and keys
func (r *organizationSettingsRepository) DeleteMultiple(ctx context.Context, orgID uuid.UUID, keys []string) error {
	return r.db.WithContext(ctx).Delete(&orgDomain.OrganizationSettings{}, "organization_id = ? AND key IN ?", orgID, keys).Error
}
