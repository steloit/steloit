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

// permissionRepository implements authDomain.PermissionRepository using GORM
type permissionRepository struct {
	db *gorm.DB
}

// NewPermissionRepository creates a new permission repository instance
func NewPermissionRepository(db *gorm.DB) authDomain.PermissionRepository {
	return &permissionRepository{
		db: db,
	}
}

// Create creates a new permission
func (r *permissionRepository) Create(ctx context.Context, permission *authDomain.Permission) error {
	return r.db.WithContext(ctx).Create(permission).Error
}

// GetByID retrieves a permission by ID
func (r *permissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.Permission, error) {
	var permission authDomain.Permission
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&permission).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get permission by ID %s: %w", id, authDomain.ErrNotFound)
		}
		return nil, err
	}
	return &permission, nil
}

// GetByName retrieves a permission by name
func (r *permissionRepository) GetByName(ctx context.Context, name string) (*authDomain.Permission, error) {
	var permission authDomain.Permission
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&permission).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get permission by name %s: %w", name, authDomain.ErrNotFound)
		}
		return nil, err
	}
	return &permission, nil
}

// GetAll retrieves all permissions
func (r *permissionRepository) GetAll(ctx context.Context) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).Order("category ASC, name ASC").Find(&permissions).Error
	return permissions, err
}

// GetAllPermissions retrieves all permissions (interface method)
func (r *permissionRepository) GetAllPermissions(ctx context.Context) ([]*authDomain.Permission, error) {
	return r.GetAll(ctx)
}

// GetByNames retrieves permissions by names
func (r *permissionRepository) GetByNames(ctx context.Context, names []string) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Where("name IN ?", names).
		Order("name ASC").
		Find(&permissions).Error
	return permissions, err
}

// Update updates a permission
func (r *permissionRepository) Update(ctx context.Context, permission *authDomain.Permission) error {
	return r.db.WithContext(ctx).Save(permission).Error
}

// Delete soft deletes a permission
func (r *permissionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&authDomain.Permission{}).Where("id = ?", id).Update("deleted_at", time.Now()).Error
}

// GetPermissionsByRoleID retrieves permissions for a specific role
func (r *permissionRepository) GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Table("permissions").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ?", roleID).
		Order("permissions.category ASC, permissions.name ASC").
		Find(&permissions).Error
	return permissions, err
}

// GetUserPermissions retrieves permissions for a user in an organization
func (r *permissionRepository) GetUserPermissions(ctx context.Context, userID, orgID uuid.UUID) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Table("permissions").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Joins("JOIN roles ON role_permissions.role_id = roles.id").
		Joins("JOIN organization_members ON roles.id = organization_members.role_id").
		Where("organization_members.user_id = ? AND organization_members.organization_id = ?", userID, orgID).
		Order("permissions.category ASC, permissions.resource ASC, permissions.action ASC").
		Find(&permissions).Error
	return permissions, err
}

// GetUserPermissionsByAPIKey retrieves permissions for a user through API key
func (r *permissionRepository) GetUserPermissionsByAPIKey(ctx context.Context, apiKeyID uuid.UUID) ([]string, error) {
	var permissionNames []string
	err := r.db.WithContext(ctx).
		Table("permissions").
		Select("permissions.name").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Joins("JOIN roles ON role_permissions.role_id = roles.id").
		Joins("JOIN organization_members ON roles.id = organization_members.role_id").
		Joins("JOIN api_keys ON organization_members.user_id = api_keys.user_id AND organization_members.organization_id = api_keys.organization_id").
		Where("api_keys.id = ?", apiKeyID).
		Pluck("permissions.name", &permissionNames).Error
	return permissionNames, err
}

// GetByResourceAction retrieves permission by resource and action
func (r *permissionRepository) GetByResourceAction(ctx context.Context, resource, action string) (*authDomain.Permission, error) {
	var permission authDomain.Permission
	err := r.db.WithContext(ctx).Where("resource = ? AND action = ?", resource, action).First(&permission).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get permission by resource %s action %s: %w", resource, action, authDomain.ErrNotFound)
		}
		return nil, err
	}
	return &permission, nil
}

// GetByResource retrieves permissions for a specific resource
func (r *permissionRepository) GetByResource(ctx context.Context, resource string) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Where("resource = ?", resource).
		Order("action ASC").
		Find(&permissions).Error
	return permissions, err
}

// GetByResourceActions retrieves permissions by resource:action format
func (r *permissionRepository) GetByResourceActions(ctx context.Context, resourceActions []string) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Where("resource_action IN ?", resourceActions).
		Order("resource ASC, action ASC").
		Find(&permissions).Error
	return permissions, err
}

// ListPermissions returns paginated list of permissions with total count
func (r *permissionRepository) ListPermissions(ctx context.Context, limit, offset int) ([]*authDomain.Permission, int, error) {
	// Get total count
	var total int64
	if err := r.db.WithContext(ctx).Model(&authDomain.Permission{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get permissions with cursor pagination (fetch +1 to detect has_next)
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Order("category ASC, resource ASC, action ASC, id ASC").
		Limit(limit + 1). // Fetch extra for has_next detection
		Find(&permissions).Error

	return permissions, int(total), err
}

// SearchPermissions searches permissions with pagination
func (r *permissionRepository) SearchPermissions(ctx context.Context, query string, limit, offset int) ([]*authDomain.Permission, int, error) {
	searchPattern := "%" + query + "%"
	dbQuery := r.db.WithContext(ctx).Where(
		"(name ILIKE ? OR description ILIKE ? OR resource ILIKE ? OR action ILIKE ? OR category ILIKE ?)",
		searchPattern, searchPattern, searchPattern, searchPattern, searchPattern,
	)

	// Get total count
	var total int64
	if err := dbQuery.Model(&authDomain.Permission{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get permissions with cursor pagination (fetch +1 to detect has_next)
	var permissions []*authDomain.Permission
	err := dbQuery.
		Order("category ASC, resource ASC, action ASC, id ASC").
		Limit(limit + 1). // Fetch extra for has_next detection
		Find(&permissions).Error

	return permissions, int(total), err
}

// GetAvailableResources returns all distinct resources
func (r *permissionRepository) GetAvailableResources(ctx context.Context) ([]string, error) {
	var resources []string
	err := r.db.WithContext(ctx).
		Model(&authDomain.Permission{}).
		Distinct("resource").
		Where("resource IS NOT NULL AND resource != ''").
		Order("resource ASC").
		Pluck("resource", &resources).Error
	return resources, err
}

// GetActionsForResource returns all actions for a specific resource
func (r *permissionRepository) GetActionsForResource(ctx context.Context, resource string) ([]string, error) {
	var actions []string
	err := r.db.WithContext(ctx).
		Model(&authDomain.Permission{}).
		Where("resource = ?", resource).
		Order("action ASC").
		Pluck("action", &actions).Error
	return actions, err
}

// GetPermissionCategories returns all distinct categories
func (r *permissionRepository) GetPermissionCategories(ctx context.Context) ([]string, error) {
	var categories []string
	err := r.db.WithContext(ctx).
		Model(&authDomain.Permission{}).
		Distinct("category").
		Where("category IS NOT NULL AND category != ''").
		Order("category ASC").
		Pluck("category", &categories).Error
	return categories, err
}

// GetRolePermissionMap returns a map of resource:action -> true for a role
func (r *permissionRepository) GetRolePermissionMap(ctx context.Context, roleID uuid.UUID) (map[string]bool, error) {
	var resourceActions []string
	err := r.db.WithContext(ctx).
		Table("permissions").
		Select("permissions.resource_action").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ?", roleID).
		Pluck("permissions.resource_action", &resourceActions).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, resourceAction := range resourceActions {
		result[resourceAction] = true
	}
	return result, nil
}

// GetUserPermissionStrings returns user permissions as resource:action strings
func (r *permissionRepository) GetUserPermissionStrings(ctx context.Context, userID, orgID uuid.UUID) ([]string, error) {
	var resourceActions []string
	err := r.db.WithContext(ctx).
		Table("permissions").
		Select("permissions.resource_action").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Joins("JOIN roles ON role_permissions.role_id = roles.id").
		Joins("JOIN organization_members ON roles.id = organization_members.role_id").
		Where("organization_members.user_id = ? AND organization_members.organization_id = ?", userID, orgID).
		Pluck("permissions.resource_action", &resourceActions).Error
	return resourceActions, err
}

// GetUserEffectivePermissions returns effective permissions as map
func (r *permissionRepository) GetUserEffectivePermissions(ctx context.Context, userID, orgID uuid.UUID) (map[string]bool, error) {
	resourceActions, err := r.GetUserPermissionStrings(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, resourceAction := range resourceActions {
		result[resourceAction] = true
	}
	return result, nil
}

// ValidateResourceAction validates resource:action format
func (r *permissionRepository) ValidateResourceAction(ctx context.Context, resource, action string) error {
	if resource == "" {
		return fmt.Errorf("validate permission resource: %w", authDomain.ErrInvalidCredentials)
	}
	if action == "" {
		return fmt.Errorf("validate permission action: %w", authDomain.ErrInvalidCredentials)
	}
	return nil
}

// PermissionExists checks if a permission exists by resource:action
func (r *permissionRepository) PermissionExists(ctx context.Context, resource, action string) (bool, error) {
	resourceAction := resource + ":" + action
	var count int64
	err := r.db.WithContext(ctx).
		Model(&authDomain.Permission{}).
		Where("resource_action = ?", resourceAction).
		Count(&count).Error
	return count > 0, err
}

// BulkPermissionExists checks multiple permissions at once
func (r *permissionRepository) BulkPermissionExists(ctx context.Context, resourceActions []string) (map[string]bool, error) {
	var existingActions []string
	err := r.db.WithContext(ctx).
		Model(&authDomain.Permission{}).
		Where("resource_action IN ?", resourceActions).
		Pluck("resource_action", &existingActions).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	existingSet := make(map[string]bool)
	for _, action := range existingActions {
		existingSet[action] = true
	}

	for _, resourceAction := range resourceActions {
		result[resourceAction] = existingSet[resourceAction]
	}

	return result, nil
}

// BulkCreate creates multiple permissions
func (r *permissionRepository) BulkCreate(ctx context.Context, permissions []*authDomain.Permission) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, permission := range permissions {
			if err := tx.Create(permission).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// BulkUpdate updates multiple permissions
func (r *permissionRepository) BulkUpdate(ctx context.Context, permissions []*authDomain.Permission) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, permission := range permissions {
			if err := tx.Save(permission).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// BulkDelete deletes multiple permissions
func (r *permissionRepository) BulkDelete(ctx context.Context, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range permissionIDs {
			if err := tx.Model(&authDomain.Permission{}).Where("id = ?", id).Update("deleted_at", time.Now()).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ========================================
// NEW: Scope-Level Query Methods
// ========================================

// GetByScopeLevel retrieves all permissions for a specific scope level
func (r *permissionRepository) GetByScopeLevel(ctx context.Context, scopeLevel authDomain.ScopeLevel) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Where("scope_level = ?", scopeLevel).
		Order("category ASC, resource ASC, action ASC").
		Find(&permissions).Error
	return permissions, err
}

// GetByCategory retrieves all permissions for a specific category
func (r *permissionRepository) GetByCategory(ctx context.Context, category string) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Where("category = ?", category).
		Order("resource ASC, action ASC").
		Find(&permissions).Error
	return permissions, err
}

// GetByScopeLevelAndCategory retrieves permissions filtered by both scope level and category
func (r *permissionRepository) GetByScopeLevelAndCategory(ctx context.Context, scopeLevel authDomain.ScopeLevel, category string) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.db.WithContext(ctx).
		Where("scope_level = ? AND category = ?", scopeLevel, category).
		Order("resource ASC, action ASC").
		Find(&permissions).Error
	return permissions, err
}

// GetAvailableCategories returns all distinct categories
func (r *permissionRepository) GetAvailableCategories(ctx context.Context) ([]string, error) {
	var categories []string
	err := r.db.WithContext(ctx).
		Model(&authDomain.Permission{}).
		Distinct("category").
		Where("category IS NOT NULL AND category != ''").
		Order("category ASC").
		Pluck("category", &categories).Error
	return categories, err
}
