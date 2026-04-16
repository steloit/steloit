package auth

import (
	"context"

	"gorm.io/gorm"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
)

// rolePermissionRepository implements authDomain.RolePermissionRepository using GORM
type rolePermissionRepository struct {
	db *gorm.DB
}

// NewRolePermissionRepository creates a new role permission repository instance
func NewRolePermissionRepository(db *gorm.DB) authDomain.RolePermissionRepository {
	return &rolePermissionRepository{
		db: db,
	}
}

// Create creates a new role permission association
func (r *rolePermissionRepository) Create(ctx context.Context, rolePermission *authDomain.RolePermission) error {
	return r.db.WithContext(ctx).Create(rolePermission).Error
}

// GetByRoleID retrieves all role permissions for a role
func (r *rolePermissionRepository) GetByRoleID(ctx context.Context, roleID uuid.UUID) ([]*authDomain.RolePermission, error) {
	var rolePermissions []*authDomain.RolePermission
	err := r.db.WithContext(ctx).
		Where("role_id = ?", roleID).
		Find(&rolePermissions).Error
	return rolePermissions, err
}

// GetByPermissionID retrieves all role permissions for a permission
func (r *rolePermissionRepository) GetByPermissionID(ctx context.Context, permissionID uuid.UUID) ([]*authDomain.RolePermission, error) {
	var rolePermissions []*authDomain.RolePermission
	err := r.db.WithContext(ctx).
		Where("permission_id = ?", permissionID).
		Find(&rolePermissions).Error
	return rolePermissions, err
}

// Delete removes a role permission association
func (r *rolePermissionRepository) Delete(ctx context.Context, roleID, permissionID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).
		Delete(&authDomain.RolePermission{}).Error
}

// DeleteByRoleID removes all permissions for a role
func (r *rolePermissionRepository) DeleteByRoleID(ctx context.Context, roleID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("role_id = ?", roleID).
		Delete(&authDomain.RolePermission{}).Error
}

// DeleteByPermissionID removes all roles for a permission
func (r *rolePermissionRepository) DeleteByPermissionID(ctx context.Context, permissionID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("permission_id = ?", permissionID).
		Delete(&authDomain.RolePermission{}).Error
}

// Exists checks if a role permission association exists
func (r *rolePermissionRepository) Exists(ctx context.Context, roleID, permissionID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&authDomain.RolePermission{}).
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).
		Count(&count).Error
	return count > 0, err
}

// AssignPermissions assigns permissions to a role (bulk operation)
func (r *rolePermissionRepository) AssignPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	// First, remove existing permissions for this role
	err := r.DeleteByRoleID(ctx, roleID)
	if err != nil {
		return err
	}

	// Add new permissions
	for _, permissionID := range permissionIDs {
		rolePermission := &authDomain.RolePermission{
			RoleID:       roleID,
			PermissionID: permissionID,
			GrantedBy:    grantedBy,
		}
		if err := r.Create(ctx, rolePermission); err != nil {
			return err
		}
	}
	return nil
}

// RevokePermissions removes specific permissions from a role
func (r *rolePermissionRepository) RevokePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("role_id = ? AND permission_id IN ?", roleID, permissionIDs).
		Delete(&authDomain.RolePermission{}).Error
}

// RevokeAllPermissions removes all permissions from a role
func (r *rolePermissionRepository) RevokeAllPermissions(ctx context.Context, roleID uuid.UUID) error {
	return r.DeleteByRoleID(ctx, roleID)
}

// HasPermission checks if a role has a specific permission
func (r *rolePermissionRepository) HasPermission(ctx context.Context, roleID, permissionID uuid.UUID) (bool, error) {
	return r.Exists(ctx, roleID, permissionID)
}

// ReplaceAllPermissions replaces all permissions for a role
func (r *rolePermissionRepository) ReplaceAllPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Remove all existing permissions
		if err := tx.Where("role_id = ?", roleID).Delete(&authDomain.RolePermission{}).Error; err != nil {
			return err
		}

		// Add new permissions
		for _, permissionID := range permissionIDs {
			rolePermission := &authDomain.RolePermission{
				RoleID:       roleID,
				PermissionID: permissionID,
				GrantedBy:    grantedBy,
			}
			if err := tx.Create(rolePermission).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// HasResourceAction checks if a role has specific resource:action permission
func (r *rolePermissionRepository) HasResourceAction(ctx context.Context, roleID uuid.UUID, resource, action string) (bool, error) {
	resourceAction := resource + ":" + action
	var count int64
	err := r.db.WithContext(ctx).
		Table("role_permissions").
		Joins("JOIN permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ? AND permissions.resource_action = ?", roleID, resourceAction).
		Count(&count).Error
	return count > 0, err
}

// CheckResourceActions checks multiple resource:action permissions at once
func (r *rolePermissionRepository) CheckResourceActions(ctx context.Context, roleID uuid.UUID, resourceActions []string) (map[string]bool, error) {
	result := make(map[string]bool)

	if len(resourceActions) == 0 {
		return result, nil
	}

	// Query for all permissions the role has
	var permissions []string
	err := r.db.WithContext(ctx).
		Table("role_permissions").
		Select("permissions.resource_action").
		Joins("JOIN permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ? AND permissions.resource_action IN ?", roleID, resourceActions).
		Pluck("permissions.resource_action", &permissions).Error
	if err != nil {
		return nil, err
	}

	// Build result map
	permissionSet := make(map[string]bool)
	for _, perm := range permissions {
		permissionSet[perm] = true
	}

	for _, resourceAction := range resourceActions {
		result[resourceAction] = permissionSet[resourceAction]
	}

	return result, nil
}

// BulkAssign assigns permissions to multiple roles in bulk
func (r *rolePermissionRepository) BulkAssign(ctx context.Context, assignments []authDomain.RolePermissionAssignment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, assignment := range assignments {
			rolePermission := &authDomain.RolePermission{
				RoleID:       assignment.RoleID,
				PermissionID: assignment.PermissionID,
			}
			if err := tx.Create(rolePermission).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// BulkRevoke revokes permissions from multiple roles in bulk
func (r *rolePermissionRepository) BulkRevoke(ctx context.Context, revocations []authDomain.RolePermissionRevocation) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, revocation := range revocations {
			if err := tx.Where("role_id = ? AND permission_id = ?", revocation.RoleID, revocation.PermissionID).
				Delete(&authDomain.RolePermission{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetRolePermissionCount returns the number of permissions assigned to a role
func (r *rolePermissionRepository) GetRolePermissionCount(ctx context.Context, roleID uuid.UUID) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&authDomain.RolePermission{}).
		Where("role_id = ?", roleID).
		Count(&count).Error
	return int(count), err
}

// GetPermissionRoleCount returns the number of roles assigned to a permission
func (r *rolePermissionRepository) GetPermissionRoleCount(ctx context.Context, permissionID uuid.UUID) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&authDomain.RolePermission{}).
		Where("permission_id = ?", permissionID).
		Count(&count).Error
	return int(count), err
}
