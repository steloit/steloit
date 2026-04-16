package auth

import (
	"context"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	appErrors "brokle/pkg/errors"
)

// roleService implements clean auth.RoleService interface (template roles only)
type roleService struct {
	roleRepo     authDomain.RoleRepository
	rolePermRepo authDomain.RolePermissionRepository
}

// NewRoleService creates a new clean role service instance
func NewRoleService(
	roleRepo authDomain.RoleRepository,
	rolePermRepo authDomain.RolePermissionRepository,
) authDomain.RoleService {
	return &roleService{
		roleRepo:     roleRepo,
		rolePermRepo: rolePermRepo,
	}
}

// CreateRole creates a new template role
func (s *roleService) CreateRole(ctx context.Context, req *authDomain.CreateRoleRequest) (*authDomain.Role, error) {
	// Validate request
	if req.Name == "" {
		return nil, appErrors.NewValidationError("name", "Role name is required")
	}
	if req.ScopeType == "" {
		return nil, appErrors.NewValidationError("scope_type", "Scope type is required")
	}

	// Check if role already exists with this name and scope
	existing, err := s.roleRepo.GetByNameAndScope(ctx, req.Name, req.ScopeType)
	if err == nil && existing != nil {
		return nil, appErrors.NewConflictError("Role with name " + req.Name + " and scope " + req.ScopeType + " already exists")
	}

	// Create new role
	role := authDomain.NewRole(req.Name, req.ScopeType, req.Description)

	err = s.roleRepo.Create(ctx, role)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create role", err)
	}

	return role, nil
}

// GetRoleByID gets a role by ID
func (s *roleService) GetRoleByID(ctx context.Context, roleID uuid.UUID) (*authDomain.Role, error) {
	return s.roleRepo.GetByID(ctx, roleID)
}

// GetRoleByNameAndScope gets a role by name and scope type
func (s *roleService) GetRoleByNameAndScope(ctx context.Context, name, scopeType string) (*authDomain.Role, error) {
	return s.roleRepo.GetByNameAndScope(ctx, name, scopeType)
}

// UpdateRole updates a role
func (s *roleService) UpdateRole(ctx context.Context, roleID uuid.UUID, req *authDomain.UpdateRoleRequest) (*authDomain.Role, error) {
	// Get existing role
	role, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Role not found")
	}

	// Update fields
	if req.Description != nil {
		role.Description = *req.Description
	}

	// Save changes
	err = s.roleRepo.Update(ctx, role)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to update role", err)
	}

	return role, nil
}

// DeleteRole deletes a role
func (s *roleService) DeleteRole(ctx context.Context, roleID uuid.UUID) error {
	// Get role to check if it exists
	role, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return appErrors.NewNotFoundError("Role not found")
	}

	// Built-in role names that cannot be deleted
	builtinRoles := map[string]bool{
		"owner":     true,
		"admin":     true,
		"developer": true,
		"viewer":    true,
	}

	if builtinRoles[role.Name] {
		return appErrors.NewForbiddenError("Cannot delete built-in role: " + role.Name)
	}

	return s.roleRepo.Delete(ctx, roleID)
}

// GetRolesByScopeType gets all roles for a specific scope type
func (s *roleService) GetRolesByScopeType(ctx context.Context, scopeType string) ([]*authDomain.Role, error) {
	return s.roleRepo.GetByScopeType(ctx, scopeType)
}

// GetAllRoles gets all template roles
func (s *roleService) GetAllRoles(ctx context.Context) ([]*authDomain.Role, error) {
	return s.roleRepo.GetAllRoles(ctx)
}

// GetRolePermissions gets all permissions assigned to a role
func (s *roleService) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]*authDomain.Permission, error) {
	return s.roleRepo.GetRolePermissions(ctx, roleID)
}

// AssignRolePermissions assigns permissions to a role
func (s *roleService) AssignRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	// Verify role exists
	_, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return appErrors.NewNotFoundError("Role not found")
	}

	return s.roleRepo.AssignRolePermissions(ctx, roleID, permissionIDs, grantedBy)
}

// RevokeRolePermissions revokes permissions from a role
func (s *roleService) RevokeRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	// Verify role exists
	_, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return appErrors.NewNotFoundError("Role not found")
	}

	return s.roleRepo.RevokeRolePermissions(ctx, roleID, permissionIDs)
}

// GetRoleStatistics gets role usage statistics
func (s *roleService) GetRoleStatistics(ctx context.Context) (*authDomain.RoleStatistics, error) {
	return s.roleRepo.GetRoleStatistics(ctx)
}

// System template role methods

func (s *roleService) GetSystemRoles(ctx context.Context) ([]*authDomain.Role, error) {
	return s.roleRepo.GetSystemRoles(ctx)
}

// Custom scoped role management

func (s *roleService) CreateCustomRole(ctx context.Context, scopeType string, scopeID uuid.UUID, req *authDomain.CreateRoleRequest) (*authDomain.Role, error) {
	// Validate request
	if req.Name == "" {
		return nil, appErrors.NewValidationError("name", "Role name is required")
	}
	if scopeType == "" {
		return nil, appErrors.NewValidationError("scope_type", "Scope type is required")
	}

	// Check if custom role already exists with this name and scope
	existing, err := s.roleRepo.GetByNameScopeAndID(ctx, req.Name, scopeType, &scopeID)
	if err == nil && existing != nil {
		return nil, appErrors.NewConflictError("Custom role with name " + req.Name + " already exists in this scope")
	}

	// Create new custom role
	role := authDomain.NewCustomRole(req.Name, scopeType, req.Description, scopeID)

	err = s.roleRepo.Create(ctx, role)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create custom role", err)
	}

	// Assign permissions if provided
	if len(req.PermissionIDs) > 0 {
		err = s.roleRepo.AssignRolePermissions(ctx, role.ID, req.PermissionIDs, nil)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to assign permissions to custom role", err)
		}
	}

	return role, nil
}

func (s *roleService) GetCustomRolesByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*authDomain.Role, error) {
	return s.roleRepo.GetCustomRolesByOrganization(ctx, organizationID)
}

func (s *roleService) UpdateCustomRole(ctx context.Context, roleID uuid.UUID, req *authDomain.UpdateRoleRequest) (*authDomain.Role, error) {
	// Get existing role and verify it's a custom role
	role, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Custom role not found")
	}

	if role.IsSystemRole() {
		return nil, appErrors.NewForbiddenError("Cannot update system role")
	}

	// Update fields
	if req.Description != nil {
		role.Description = *req.Description
	}

	// Save changes
	err = s.roleRepo.Update(ctx, role)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to update custom role", err)
	}

	// Update permissions if provided
	if req.PermissionIDs != nil {
		err = s.roleRepo.UpdateRolePermissions(ctx, role.ID, req.PermissionIDs, nil)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to update role permissions", err)
		}
	}

	return role, nil
}

func (s *roleService) DeleteCustomRole(ctx context.Context, roleID uuid.UUID) error {
	// Get role to check if it exists and is a custom role
	role, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return appErrors.NewNotFoundError("Custom role not found")
	}

	if role.IsSystemRole() {
		return appErrors.NewForbiddenError("Cannot delete system role")
	}

	// TODO: Add check if role is in use by organization members
	// This would require checking the organization_members table

	return s.roleRepo.Delete(ctx, roleID)
}
