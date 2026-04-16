package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	appErrors "brokle/pkg/errors"
)

// permissionService implements auth.PermissionService interface
type permissionService struct {
	permissionRepo authDomain.PermissionRepository
	rolePermRepo   authDomain.RolePermissionRepository
}

// NewPermissionService creates a new permission service instance
func NewPermissionService(
	permissionRepo authDomain.PermissionRepository,
	rolePermRepo authDomain.RolePermissionRepository,
) authDomain.PermissionService {
	return &permissionService{
		permissionRepo: permissionRepo,
		rolePermRepo:   rolePermRepo,
	}
}

// CreatePermission creates a new permission
func (s *permissionService) CreatePermission(ctx context.Context, req *authDomain.CreatePermissionRequest) (*authDomain.Permission, error) {
	// Validate permission doesn't already exist
	existing, err := s.permissionRepo.GetByResourceAction(ctx, req.Resource, req.Action)
	if err == nil && existing != nil {
		return nil, appErrors.NewConflictError("Permission " + req.Resource + ":" + req.Action + " already exists")
	}

	// Create permission
	permission := authDomain.NewPermission(req.Resource, req.Action, req.Description)
	if err := s.permissionRepo.Create(ctx, permission); err != nil {
		return nil, appErrors.NewInternalError("Failed to create permission", err)
	}

	return permission, nil
}

// GetPermission retrieves a permission by ID
func (s *permissionService) GetPermission(ctx context.Context, permissionID uuid.UUID) (*authDomain.Permission, error) {
	return s.permissionRepo.GetByID(ctx, permissionID)
}

// GetPermissionByName retrieves a permission by legacy name
func (s *permissionService) GetPermissionByName(ctx context.Context, name string) (*authDomain.Permission, error) {
	return s.permissionRepo.GetByName(ctx, name)
}

// GetPermissionByResourceAction retrieves a permission by resource:action
func (s *permissionService) GetPermissionByResourceAction(ctx context.Context, resource, action string) (*authDomain.Permission, error) {
	return s.permissionRepo.GetByResourceAction(ctx, resource, action)
}

// UpdatePermission updates a permission
func (s *permissionService) UpdatePermission(ctx context.Context, permissionID uuid.UUID, req *authDomain.UpdatePermissionRequest) error {
	// Get existing permission
	permission, err := s.permissionRepo.GetByID(ctx, permissionID)
	if err != nil {
		return appErrors.NewNotFoundError("Permission not found")
	}

	// Update fields
	if req.Description != nil {
		permission.Description = *req.Description
	}

	return s.permissionRepo.Update(ctx, permission)
}

// DeletePermission deletes a permission
func (s *permissionService) DeletePermission(ctx context.Context, permissionID uuid.UUID) error {
	// Check if permission is in use
	// This would need to be implemented properly in a production system
	return s.permissionRepo.Delete(ctx, permissionID)
}

// ListPermissions lists permissions with pagination
func (s *permissionService) ListPermissions(ctx context.Context, limit, offset int) (*authDomain.PermissionListResponse, error) {
	permissions, err := s.permissionRepo.GetAllPermissions(ctx)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to list permissions", err)
	}

	totalCount := len(permissions)

	// Apply pagination
	start := offset
	end := offset + limit
	if start > len(permissions) {
		start = len(permissions)
	}
	if end > len(permissions) {
		end = len(permissions)
	}
	paginatedPermissions := permissions[start:end]

	return &authDomain.PermissionListResponse{
		Permissions: paginatedPermissions,
		TotalCount:  totalCount,
		Page:        offset/limit + 1,
		PageSize:    limit,
	}, nil
}

// GetAllPermissions returns all permissions
func (s *permissionService) GetAllPermissions(ctx context.Context) ([]*authDomain.Permission, error) {
	return s.permissionRepo.GetAllPermissions(ctx)
}

// GetPermissionsByResource returns all permissions for a resource
func (s *permissionService) GetPermissionsByResource(ctx context.Context, resource string) ([]*authDomain.Permission, error) {
	return s.permissionRepo.GetByResource(ctx, resource)
}

// GetPermissionsByNames returns permissions by legacy names
func (s *permissionService) GetPermissionsByNames(ctx context.Context, names []string) ([]*authDomain.Permission, error) {
	permissions := make([]*authDomain.Permission, 0, len(names))
	for _, name := range names {
		perm, err := s.permissionRepo.GetByName(ctx, name)
		if err != nil {
			return nil, appErrors.NewNotFoundError("Permission " + name + " not found")
		}
		permissions = append(permissions, perm)
	}
	return permissions, nil
}

// GetPermissionsByResourceActions returns permissions by resource:action format
func (s *permissionService) GetPermissionsByResourceActions(ctx context.Context, resourceActions []string) ([]*authDomain.Permission, error) {
	permissions := make([]*authDomain.Permission, 0, len(resourceActions))
	for _, resourceAction := range resourceActions {
		resource, action, err := s.ParseResourceAction(resourceAction)
		if err != nil {
			return nil, appErrors.NewValidationError("resource_action", "Invalid resource:action format: "+resourceAction)
		}

		perm, err := s.permissionRepo.GetByResourceAction(ctx, resource, action)
		if err != nil {
			return nil, appErrors.NewNotFoundError("Permission " + resourceAction + " not found")
		}
		permissions = append(permissions, perm)
	}
	return permissions, nil
}

// SearchPermissions searches permissions with pagination
func (s *permissionService) SearchPermissions(ctx context.Context, query string, limit, offset int) (*authDomain.PermissionListResponse, error) {
	// Basic search implementation - in production this would be done at DB level
	allPermissions, err := s.permissionRepo.GetAllPermissions(ctx)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to search permissions", err)
	}

	// Filter permissions by query
	filteredPermissions := make([]*authDomain.Permission, 0)
	for _, perm := range allPermissions {
		if strings.Contains(strings.ToLower(perm.Name), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(perm.Description), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(perm.Resource), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(perm.Action), strings.ToLower(query)) {
			filteredPermissions = append(filteredPermissions, perm)
		}
	}

	// Apply pagination
	totalCount := len(filteredPermissions)
	start := offset
	end := offset + limit
	if start > len(filteredPermissions) {
		start = len(filteredPermissions)
	}
	if end > len(filteredPermissions) {
		end = len(filteredPermissions)
	}
	paginatedPermissions := filteredPermissions[start:end]

	return &authDomain.PermissionListResponse{
		Permissions: paginatedPermissions,
		TotalCount:  totalCount,
		Page:        offset/limit + 1,
		PageSize:    limit,
	}, nil
}

// GetAvailableResources returns all distinct resources
func (s *permissionService) GetAvailableResources(ctx context.Context) ([]string, error) {
	return s.permissionRepo.GetAvailableResources(ctx)
}

// GetActionsForResource returns all actions for a resource
func (s *permissionService) GetActionsForResource(ctx context.Context, resource string) ([]string, error) {
	return s.permissionRepo.GetActionsForResource(ctx, resource)
}

// ValidatePermissionName validates legacy permission name
func (s *permissionService) ValidatePermissionName(ctx context.Context, name string) error {
	if !strings.Contains(name, ".") {
		return appErrors.NewValidationError("name", "Invalid permission name format: "+name+" (must contain dot)")
	}
	return nil
}

// ValidateResourceAction validates resource:action format
func (s *permissionService) ValidateResourceAction(ctx context.Context, resource, action string) error {
	if resource == "" || action == "" {
		return appErrors.NewValidationError("resource_action", "Resource and action cannot be empty")
	}
	return nil
}

// PermissionExists checks if a resource:action permission exists
func (s *permissionService) PermissionExists(ctx context.Context, resource, action string) (bool, error) {
	_, err := s.permissionRepo.GetByResourceAction(ctx, resource, action)
	if err != nil {
		if errors.Is(err, authDomain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// BulkPermissionExists checks if multiple resource:action permissions exist
func (s *permissionService) BulkPermissionExists(ctx context.Context, resourceActions []string) (map[string]bool, error) {
	results := make(map[string]bool)
	for _, resourceAction := range resourceActions {
		resource, action, err := s.ParseResourceAction(resourceAction)
		if err != nil {
			results[resourceAction] = false
			continue
		}

		exists, err := s.PermissionExists(ctx, resource, action)
		if err != nil {
			results[resourceAction] = false
			continue
		}
		results[resourceAction] = exists
	}
	return results, nil
}

// ParseResourceAction parses resource:action format
func (s *permissionService) ParseResourceAction(resourceAction string) (resource, action string, err error) {
	parts := strings.Split(resourceAction, ":")
	if len(parts) != 2 {
		return "", "", appErrors.NewValidationError("resource_action", "Invalid resource:action format: "+resourceAction)
	}
	return parts[0], parts[1], nil
}

// FormatResourceAction formats resource and action into resource:action
func (s *permissionService) FormatResourceAction(resource, action string) string {
	return fmt.Sprintf("%s:%s", resource, action)
}

// IsValidResourceActionFormat checks if string is valid resource:action format
func (s *permissionService) IsValidResourceActionFormat(resourceAction string) bool {
	parts := strings.Split(resourceAction, ":")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
