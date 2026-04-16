package auth

import (
	"context"

	"gorm.io/gorm"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/shared"
)

// roleRepository implements authDomain.RoleRepository using GORM for normalized template roles
type roleRepository struct {
	db *gorm.DB
}

// NewRoleRepository creates a new template role repository instance
func NewRoleRepository(db *gorm.DB) authDomain.RoleRepository {
	return &roleRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *roleRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Core CRUD operations

func (r *roleRepository) Create(ctx context.Context, role *authDomain.Role) error {
	return r.getDB(ctx).WithContext(ctx).Create(role).Error
}

func (r *roleRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.Role, error) {
	var role authDomain.Role
	err := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id).
		Preload("Permissions").
		First(&role).Error

	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) GetByNameAndScope(ctx context.Context, name, scopeType string) (*authDomain.Role, error) {
	var role authDomain.Role
	err := r.getDB(ctx).WithContext(ctx).
		Where("name = ? AND scope_type = ? AND scope_id IS NULL", name, scopeType).
		Preload("Permissions").
		First(&role).Error

	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) Update(ctx context.Context, role *authDomain.Role) error {
	return r.getDB(ctx).WithContext(ctx).Save(role).Error
}

func (r *roleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Delete(&authDomain.Role{}, id).Error
}

// Template role queries

func (r *roleRepository) GetByScopeType(ctx context.Context, scopeType string) ([]*authDomain.Role, error) {
	var roles []*authDomain.Role
	err := r.getDB(ctx).WithContext(ctx).
		Where("scope_type = ?", scopeType).
		Preload("Permissions").
		Find(&roles).Error

	return roles, err
}

func (r *roleRepository) GetAllRoles(ctx context.Context) ([]*authDomain.Role, error) {
	var roles []*authDomain.Role
	err := r.getDB(ctx).WithContext(ctx).
		Preload("Permissions").
		Find(&roles).Error

	return roles, err
}

// Permission management for roles

func (r *roleRepository) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]*authDomain.Permission, error) {
	var permissions []*authDomain.Permission
	err := r.getDB(ctx).WithContext(ctx).
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ?", roleID).
		Find(&permissions).Error

	return permissions, err
}

func (r *roleRepository) AssignRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	var rolePermissions []authDomain.RolePermission
	for _, permissionID := range permissionIDs {
		rolePermissions = append(rolePermissions, authDomain.RolePermission{
			RoleID:       roleID,
			PermissionID: permissionID,
			GrantedBy:    grantedBy,
		})
	}
	return r.getDB(ctx).WithContext(ctx).Create(&rolePermissions).Error
}

func (r *roleRepository) RevokeRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Where("role_id = ? AND permission_id IN ?", roleID, permissionIDs).
		Delete(&authDomain.RolePermission{}).Error
}

func (r *roleRepository) UpdateRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Remove all existing permissions
		if err := tx.Where("role_id = ?", roleID).Delete(&authDomain.RolePermission{}).Error; err != nil {
			return err
		}

		// Add new permissions
		if len(permissionIDs) > 0 {
			var rolePermissions []authDomain.RolePermission
			for _, permissionID := range permissionIDs {
				rolePermissions = append(rolePermissions, authDomain.RolePermission{
					RoleID:       roleID,
					PermissionID: permissionID,
					GrantedBy:    grantedBy,
				})
			}
			return tx.Create(&rolePermissions).Error
		}

		return nil
	})
}

// Statistics

func (r *roleRepository) GetRoleStatistics(ctx context.Context) (*authDomain.RoleStatistics, error) {
	var stats authDomain.RoleStatistics

	// Get total role count
	var totalCount int64
	if err := r.getDB(ctx).WithContext(ctx).Model(&authDomain.Role{}).Count(&totalCount).Error; err != nil {
		return nil, err
	}
	stats.TotalRoles = int(totalCount)

	// Get scope distribution
	var scopeCounts []struct {
		ScopeType string
		Count     int64
	}
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.Role{}).
		Select("scope_type, COUNT(*) as count").
		Group("scope_type").
		Find(&scopeCounts).Error; err != nil {
		return nil, err
	}

	stats.ScopeDistribution = make(map[string]int)
	for _, sc := range scopeCounts {
		stats.ScopeDistribution[sc.ScopeType] = int(sc.Count)

		switch sc.ScopeType {
		case authDomain.ScopeOrganization:
			stats.OrganizationRoles = int(sc.Count)
		case authDomain.ScopeProject:
			stats.ProjectRoles = int(sc.Count)
		}
	}

	// Get role usage distribution (how many members have each role)
	var roleUsage []struct {
		RoleName string
		Count    int64
	}
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.OrganizationMember{}).
		Select("r.name as role_name, COUNT(*) as count").
		Joins("JOIN roles r ON organization_members.role_id = r.id").
		Group("r.name").
		Find(&roleUsage).Error; err != nil {
		return nil, err
	}

	stats.RoleDistribution = make(map[string]int)
	for _, ru := range roleUsage {
		stats.RoleDistribution[ru.RoleName] = int(ru.Count)
	}

	return &stats, nil
}

// System template role methods

func (r *roleRepository) GetSystemRoles(ctx context.Context) ([]*authDomain.Role, error) {
	var roles []*authDomain.Role
	err := r.getDB(ctx).WithContext(ctx).
		Where("scope_type = ? AND scope_id IS NULL", authDomain.ScopeSystem).
		Preload("Permissions").
		Order("name ASC").
		Find(&roles).Error
	return roles, err
}

// Custom scoped role methods

func (r *roleRepository) GetCustomRolesByScopeID(ctx context.Context, scopeType string, scopeID uuid.UUID) ([]*authDomain.Role, error) {
	var roles []*authDomain.Role
	err := r.getDB(ctx).WithContext(ctx).
		Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).
		Preload("Permissions").
		Order("name ASC").
		Find(&roles).Error
	return roles, err
}

func (r *roleRepository) GetByNameScopeAndID(ctx context.Context, name, scopeType string, scopeID *uuid.UUID) (*authDomain.Role, error) {
	var role authDomain.Role
	query := r.getDB(ctx).WithContext(ctx).
		Where("name = ? AND scope_type = ?", name, scopeType)

	if scopeID == nil {
		query = query.Where("scope_id IS NULL")
	} else {
		query = query.Where("scope_id = ?", *scopeID)
	}

	err := query.Preload("Permissions").First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) GetCustomRolesByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*authDomain.Role, error) {
	var roles []*authDomain.Role
	err := r.getDB(ctx).WithContext(ctx).
		Where("scope_type = ? AND scope_id = ?", authDomain.ScopeOrganization, organizationID).
		Preload("Permissions").
		Order("name ASC").
		Find(&roles).Error
	return roles, err
}

// Bulk operations

func (r *roleRepository) BulkCreate(ctx context.Context, roles []*authDomain.Role) error {
	return r.getDB(ctx).WithContext(ctx).Create(&roles).Error
}
