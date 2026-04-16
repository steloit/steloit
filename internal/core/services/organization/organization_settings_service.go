package organization

import (
	"context"

	"github.com/google/uuid"

	orgDomain "brokle/internal/core/domain/organization"
	appErrors "brokle/pkg/errors"
)

// organizationSettingsService implements orgDomain.OrganizationSettingsService
type organizationSettingsService struct {
	settingsRepo orgDomain.OrganizationSettingsRepository
	memberRepo   orgDomain.MemberRepository
}

// NewOrganizationSettingsService creates a new organization settings service instance
func NewOrganizationSettingsService(
	settingsRepo orgDomain.OrganizationSettingsRepository,
	memberRepo orgDomain.MemberRepository,
) orgDomain.OrganizationSettingsService {
	return &organizationSettingsService{
		settingsRepo: settingsRepo,
		memberRepo:   memberRepo,
	}
}

// CreateSetting creates a new organization setting
func (s *organizationSettingsService) CreateSetting(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, req *orgDomain.CreateOrganizationSettingRequest) (*orgDomain.OrganizationSettings, error) {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "create"); err != nil {
		return nil, err
	}

	// Check if setting already exists
	existing, err := s.settingsRepo.GetByKey(ctx, orgID, req.Key)
	if err != nil && err.Error() != "organization setting not found" {
		return nil, appErrors.NewInternalError("Failed to check existing setting", err)
	}
	if existing != nil {
		return nil, appErrors.NewConflictError("Setting with this key already exists")
	}

	// Create new setting
	setting, err := orgDomain.NewOrganizationSettings(orgID, req.Key, req.Value)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create setting", err)
	}

	if err := s.settingsRepo.Create(ctx, setting); err != nil {
		return nil, appErrors.NewInternalError("Failed to save setting", err)
	}

	return setting, nil
}

// GetSetting retrieves a specific organization setting
func (s *organizationSettingsService) GetSetting(ctx context.Context, orgID uuid.UUID, key string) (*orgDomain.OrganizationSettings, error) {
	return s.settingsRepo.GetByKey(ctx, orgID, key)
}

// GetAllSettings retrieves all settings for an organization as a map
func (s *organizationSettingsService) GetAllSettings(ctx context.Context, orgID uuid.UUID) (map[string]interface{}, error) {
	return s.settingsRepo.GetSettingsMap(ctx, orgID)
}

// UpdateSetting updates an existing organization setting
func (s *organizationSettingsService) UpdateSetting(ctx context.Context, orgID uuid.UUID, key string, userID uuid.UUID, req *orgDomain.UpdateOrganizationSettingRequest) (*orgDomain.OrganizationSettings, error) {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "update"); err != nil {
		return nil, err
	}

	// Get existing setting
	setting, err := s.settingsRepo.GetByKey(ctx, orgID, key)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Setting not found")
	}

	// Update setting value
	if err := setting.SetValue(req.Value); err != nil {
		return nil, appErrors.NewInternalError("Failed to set value", err)
	}

	if err := s.settingsRepo.Update(ctx, setting); err != nil {
		return nil, appErrors.NewInternalError("Failed to update setting", err)
	}

	return setting, nil
}

// DeleteSetting deletes an organization setting
func (s *organizationSettingsService) DeleteSetting(ctx context.Context, orgID uuid.UUID, key string, userID uuid.UUID) error {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "delete"); err != nil {
		return err
	}

	// Verify setting exists before deletion
	_, err := s.settingsRepo.GetByKey(ctx, orgID, key)
	if err != nil {
		return appErrors.NewNotFoundError("Setting not found")
	}

	if err := s.settingsRepo.DeleteByKey(ctx, orgID, key); err != nil {
		return appErrors.NewInternalError("Failed to delete setting", err)
	}

	return nil
}

// UpsertSetting creates or updates a setting
func (s *organizationSettingsService) UpsertSetting(ctx context.Context, orgID uuid.UUID, key string, value interface{}, userID uuid.UUID) (*orgDomain.OrganizationSettings, error) {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "upsert"); err != nil {
		return nil, err
	}

	setting, err := s.settingsRepo.UpsertSetting(ctx, orgID, key, value)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to upsert setting", err)
	}

	return setting, nil
}

// CreateMultipleSettings creates multiple settings in bulk
func (s *organizationSettingsService) CreateMultipleSettings(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, settings map[string]interface{}) error {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "bulk_create"); err != nil {
		return err
	}

	settingEntities := make([]*orgDomain.OrganizationSettings, 0, len(settings))
	for key, value := range settings {
		setting, err := orgDomain.NewOrganizationSettings(orgID, key, value)
		if err != nil {
			return appErrors.NewInternalError("Failed to create setting for key "+key, err)
		}
		settingEntities = append(settingEntities, setting)
	}

	if err := s.settingsRepo.CreateMultiple(ctx, settingEntities); err != nil {
		return appErrors.NewInternalError("Failed to create multiple settings", err)
	}

	return nil
}

// GetSettingsByKeys retrieves specific settings by keys
func (s *organizationSettingsService) GetSettingsByKeys(ctx context.Context, orgID uuid.UUID, keys []string) (map[string]interface{}, error) {
	settings, err := s.settingsRepo.GetByKeys(ctx, orgID, keys)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to get settings by keys", err)
	}

	result := make(map[string]interface{})
	for _, setting := range settings {
		value, err := setting.GetValue()
		if err != nil {
			// If unmarshaling fails, store as string
			value = setting.Value
		}
		result[setting.Key] = value
	}

	return result, nil
}

// DeleteMultipleSettings deletes multiple settings by keys
func (s *organizationSettingsService) DeleteMultipleSettings(ctx context.Context, orgID uuid.UUID, keys []string, userID uuid.UUID) error {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "bulk_delete"); err != nil {
		return err
	}

	if err := s.settingsRepo.DeleteMultiple(ctx, orgID, keys); err != nil {
		return appErrors.NewInternalError("Failed to delete multiple settings", err)
	}

	return nil
}

// ValidateSettingsAccess validates if user can perform settings operations
func (s *organizationSettingsService) ValidateSettingsAccess(ctx context.Context, userID, orgID uuid.UUID, operation string) error {
	// Check if user is a member of the organization
	isMember, err := s.memberRepo.IsMember(ctx, userID, orgID)
	if err != nil {
		return appErrors.NewInternalError("Failed to check membership", err)
	}
	if !isMember {
		return appErrors.NewForbiddenError("User is not a member of this organization")
	}

	// For now, allow any member to manage settings
	// This could be enhanced with role-based permissions
	return nil
}

// CanUserManageSettings checks if user can manage organization settings
func (s *organizationSettingsService) CanUserManageSettings(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	err := s.ValidateSettingsAccess(ctx, userID, orgID, "manage")
	return err == nil, nil
}

// ResetToDefaults resets organization settings to default values
func (s *organizationSettingsService) ResetToDefaults(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) error {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "reset"); err != nil {
		return err
	}

	// Get all current settings
	currentSettings, err := s.settingsRepo.GetAllByOrganizationID(ctx, orgID)
	if err != nil {
		return appErrors.NewInternalError("Failed to get current settings", err)
	}

	// Delete all current settings
	if len(currentSettings) > 0 {
		keys := make([]string, len(currentSettings))
		for i, setting := range currentSettings {
			keys[i] = setting.Key
		}
		if err := s.settingsRepo.DeleteMultiple(ctx, orgID, keys); err != nil {
			return appErrors.NewInternalError("Failed to clear current settings", err)
		}
	}

	return nil
}

// ExportSettings exports all organization settings
func (s *organizationSettingsService) ExportSettings(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) (map[string]interface{}, error) {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "export"); err != nil {
		return nil, err
	}

	settings, err := s.settingsRepo.GetSettingsMap(ctx, orgID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to export settings", err)
	}

	return settings, nil
}

// ImportSettings imports organization settings
func (s *organizationSettingsService) ImportSettings(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, settings map[string]interface{}) error {
	// Validate user access
	if err := s.ValidateSettingsAccess(ctx, userID, orgID, "import"); err != nil {
		return err
	}

	// Create or update each setting
	for key, value := range settings {
		_, err := s.settingsRepo.UpsertSetting(ctx, orgID, key, value)
		if err != nil {
			return appErrors.NewInternalError("Failed to import setting "+key, err)
		}
	}

	return nil
}
