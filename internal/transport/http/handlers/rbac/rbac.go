package rbac

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/auth"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// Handler handles RBAC-related HTTP requests (roles and permissions) - Clean version
type Handler struct {
	config                    *config.Config
	logger                    *slog.Logger
	roleService               auth.RoleService
	permissionService         auth.PermissionService
	organizationMemberService auth.OrganizationMemberService
	scopeService              auth.ScopeService
}

// NewHandler creates a new clean RBAC handler
func NewHandler(
	config *config.Config,
	logger *slog.Logger,
	roleService auth.RoleService,
	permissionService auth.PermissionService,
	organizationMemberService auth.OrganizationMemberService,
	scopeService auth.ScopeService,
) *Handler {
	return &Handler{
		config:                    config,
		logger:                    logger,
		roleService:               roleService,
		permissionService:         permissionService,
		organizationMemberService: organizationMemberService,
		scopeService:              scopeService,
	}
}

// =============================================================================
// CLEAN ROLE MANAGEMENT ENDPOINTS
// =============================================================================

// CreateRole handles POST /rbac/roles
func (h *Handler) CreateRole(c *gin.Context) {
	var req auth.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	role, err := h.roleService.CreateRole(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create role", "error", err, "scope_type", req.ScopeType, "role_name", req.Name)
		response.InternalServerError(c, "Failed to create role")
		return
	}

	h.logger.Info("Role created successfully", "role_id", role.ID, "role_name", role.Name, "scope_type", role.ScopeType)
	response.Created(c, role)
}

// GetRole handles GET /rbac/roles/{roleId}
func (h *Handler) GetRole(c *gin.Context) {
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid role ID", "roleId must be a valid UUID"))
		return
	}

	role, err := h.roleService.GetRoleByID(c.Request.Context(), roleID)
	if err != nil {
		h.logger.Error("Failed to get role", "error", err, "role_id", roleID)
		response.NotFound(c, "Role not found")
		return
	}

	h.logger.Info("Role retrieved successfully", "role_id", roleID)
	response.Success(c, role)
}

// UpdateRole handles PUT /rbac/roles/{roleId}
func (h *Handler) UpdateRole(c *gin.Context) {
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid role ID", "roleId must be a valid UUID"))
		return
	}

	var req auth.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	updatedRole, err := h.roleService.UpdateRole(c.Request.Context(), roleID, &req)
	if err != nil {
		h.logger.Error("Failed to update role", "error", err, "role_id", roleID)
		response.InternalServerError(c, "Failed to update role")
		return
	}

	h.logger.Info("Role updated successfully", "role_id", roleID)
	response.Success(c, updatedRole)
}

// DeleteRole handles DELETE /rbac/roles/{roleId}
func (h *Handler) DeleteRole(c *gin.Context) {
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid role ID", "roleId must be a valid UUID"))
		return
	}

	// Get role first to check if it's a system role
	role, err := h.roleService.GetRoleByID(c.Request.Context(), roleID)
	if err != nil {
		h.logger.Error("Failed to get role", "error", err, "role_id", roleID)
		response.NotFound(c, "Role not found")
		return
	}

	// Check if it's a built-in role (cannot be deleted)
	builtinRoles := map[string]bool{
		"owner":     true,
		"admin":     true,
		"developer": true,
		"viewer":    true,
	}

	if builtinRoles[role.Name] {
		h.logger.Warn("Attempted to delete built-in role", "role_id", roleID)
		response.Forbidden(c, "Cannot delete built-in role")
		return
	}

	// Delete role
	err = h.roleService.DeleteRole(c.Request.Context(), roleID)
	if err != nil {
		h.logger.Error("Failed to delete role", "error", err, "role_id", roleID)
		response.InternalServerError(c, "Failed to delete role")
		return
	}

	h.logger.Info("Role deleted successfully", "role_id", roleID)
	response.NoContent(c)
}

// ListRoles handles GET /rbac/roles
func (h *Handler) ListRoles(c *gin.Context) {
	scopeType := c.Query("scope_type")
	if scopeType == "" {
		response.Error(c, appErrors.NewValidationError("Scope type is required", "scope_type parameter cannot be empty"))
		return
	}

	roles, err := h.roleService.GetRolesByScopeType(c.Request.Context(), scopeType)
	if err != nil {
		h.logger.Error("Failed to list roles", "error", err, "scope_type", scopeType)
		response.InternalServerError(c, "Failed to list roles")
		return
	}

	h.logger.Info("Roles listed successfully", "scope_type", scopeType, "roles_count", len(roles))
	response.Success(c, roles)
}

// =============================================================================
// USER ROLE MANAGEMENT
// =============================================================================

// GetUserRoles handles GET /rbac/users/{userId}/roles
func (h *Handler) GetUserRoles(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	userMemberships, err := h.organizationMemberService.GetUserMemberships(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user memberships", "error", err, "user_id", userID)
		response.NotFound(c, "User memberships not found")
		return
	}

	h.logger.Info("User memberships retrieved successfully", "user_id", userID, "memberships_count", len(userMemberships))
	response.Success(c, userMemberships)
}

// GetUserPermissions handles GET /rbac/users/{userId}/permissions
func (h *Handler) GetUserPermissions(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	permissions, err := h.organizationMemberService.GetUserEffectivePermissions(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user permissions", "error", err, "user_id", userID)
		response.NotFound(c, "User permissions not found")
		return
	}

	h.logger.Info("User permissions retrieved successfully", "user_id", userID, "permissions_count", len(permissions))
	response.Success(c, permissions)
}

// AssignOrganizationRole handles POST /rbac/users/{userId}/organizations/{orgId}/roles
func (h *Handler) AssignOrganizationRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "orgId must be a valid UUID"))
		return
	}

	var req auth.AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Create organization membership with role
	member, err := h.organizationMemberService.AddMember(c.Request.Context(), userID, orgID, req.RoleID, nil)
	if err != nil {
		h.logger.Error("Failed to assign role to user in organization", "error", err, "user_id", userID, "org_id", orgID, "role_id", req.RoleID)
		response.InternalServerError(c, "Failed to assign role to user in organization")
		return
	}

	h.logger.Info("Role assigned to user in organization successfully", "user_id", userID, "org_id", orgID, "role_id", req.RoleID)
	response.Created(c, member)
}

// RemoveOrganizationMember handles DELETE /rbac/users/{userId}/organizations/{orgId}
func (h *Handler) RemoveOrganizationMember(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "orgId must be a valid UUID"))
		return
	}

	if err := h.organizationMemberService.RemoveMember(c.Request.Context(), userID, orgID); err != nil {
		h.logger.Error("Failed to remove user from organization", "error", err, "user_id", userID, "org_id", orgID)
		response.InternalServerError(c, "Failed to remove user from organization")
		return
	}

	h.logger.Info("User removed from organization successfully", "user_id", userID, "org_id", orgID)
	response.NoContent(c)
}

// CheckUserPermissions handles POST /rbac/users/{userId}/permissions/check
func (h *Handler) CheckUserPermissions(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	var req auth.CheckPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	result, err := h.organizationMemberService.CheckUserPermissions(c.Request.Context(), userID, req.ResourceActions)
	if err != nil {
		h.logger.Error("Failed to check user permissions", "error", err, "user_id", userID, "permissions_count", len(req.ResourceActions))
		response.InternalServerError(c, "Failed to check permissions")
		return
	}

	h.logger.Info("User permissions checked successfully", "user_id", userID, "permissions_count", len(req.ResourceActions))
	response.Success(c, result)
}

// GetRoleStatistics handles GET /rbac/roles/statistics
func (h *Handler) GetRoleStatistics(c *gin.Context) {
	stats, err := h.roleService.GetRoleStatistics(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get role statistics", "error", err)
		response.InternalServerError(c, "Failed to get role statistics")
		return
	}

	h.logger.Info("Role statistics retrieved successfully")
	response.Success(c, stats)
}

// =============================================================================
// PERMISSION MANAGEMENT ENDPOINTS
// =============================================================================

// ListPermissions handles GET /rbac/permissions
func (h *Handler) ListPermissions(c *gin.Context) {
	limit := parseQueryInt(c, "limit", 50)
	if limit > 100 {
		limit = 100
	}
	offset := parseQueryInt(c, "offset", 0)

	result, err := h.permissionService.ListPermissions(c.Request.Context(), limit, offset)
	if err != nil {
		h.logger.Error("Failed to list permissions", "error", err)
		response.InternalServerError(c, "Failed to list permissions")
		return
	}

	h.logger.Info("Permissions listed successfully", "permissions_count", result.TotalCount)
	response.Success(c, result)
}

// CreatePermission handles POST /rbac/permissions
func (h *Handler) CreatePermission(c *gin.Context) {
	var req auth.CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	permission, err := h.permissionService.CreatePermission(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create permission", "error", err, "resource", req.Resource, "action", req.Action)
		response.InternalServerError(c, "Failed to create permission")
		return
	}

	h.logger.Info("Permission created successfully", "permission_id", permission.ID, "resource", permission.Resource, "action", permission.Action)
	response.Created(c, permission)
}

// GetPermission handles GET /rbac/permissions/{permissionId}
func (h *Handler) GetPermission(c *gin.Context) {
	permissionID, err := uuid.Parse(c.Param("permissionId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid permission ID", "permissionId must be a valid UUID"))
		return
	}

	permission, err := h.permissionService.GetPermission(c.Request.Context(), permissionID)
	if err != nil {
		h.logger.Error("Failed to get permission", "error", err, "permission_id", permissionID)
		response.NotFound(c, "Permission not found")
		return
	}

	h.logger.Info("Permission retrieved successfully", "permission_id", permissionID)
	response.Success(c, permission)
}

// GetAvailableResources handles GET /rbac/permissions/resources
func (h *Handler) GetAvailableResources(c *gin.Context) {
	resources, err := h.permissionService.GetAvailableResources(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get available resources", "error", err)
		response.InternalServerError(c, "Failed to get available resources")
		return
	}

	h.logger.Info("Available resources retrieved successfully", "resources_count", len(resources))
	response.Success(c, resources)
}

// GetActionsForResource handles GET /rbac/permissions/resources/{resource}/actions
func (h *Handler) GetActionsForResource(c *gin.Context) {
	resource := c.Param("resource")
	if resource == "" {
		response.Error(c, appErrors.NewValidationError("Resource parameter is required", "resource parameter cannot be empty"))
		return
	}

	actions, err := h.permissionService.GetActionsForResource(c.Request.Context(), resource)
	if err != nil {
		h.logger.Error("Failed to get actions for resource", "error", err, "resource", resource)
		response.InternalServerError(c, "Failed to get actions for resource")
		return
	}

	h.logger.Info("Actions for resource retrieved successfully", "resource", resource, "actions_count", len(actions))
	response.Success(c, actions)
}

// Legacy method for backward compatibility
func (h *Handler) GetUserRole(c *gin.Context) {
	// This is a legacy endpoint that should redirect to the new GetUserRoles endpoint
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	// Get all user memberships instead of direct roles
	userMemberships, err := h.organizationMemberService.GetUserMemberships(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user memberships", "error", err, "user_id", userID)
		response.NotFound(c, "User memberships not found")
		return
	}

	// Return first membership for backward compatibility
	if len(userMemberships) > 0 {
		response.Success(c, userMemberships[0])
	} else {
		response.NotFound(c, "User has no organization memberships")
	}
}

// Custom Role Management Handlers

// CreateCustomRole creates a custom role for an organization
func (h *Handler) CreateCustomRole(c *gin.Context) {
	// Get organization ID from URL
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "orgId must be a valid UUID"))
		return
	}

	var req auth.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validate scope type for custom roles
	if req.ScopeType != auth.ScopeOrganization {
		response.Error(c, appErrors.NewValidationError("Custom roles must have organization scope type", "scope_type must be 'organization' for custom roles"))
		return
	}

	role, err := h.roleService.CreateCustomRole(c.Request.Context(), auth.ScopeOrganization, orgID, &req)
	if err != nil {
		h.logger.Error("Failed to create custom role", "error", err, "org_id", orgID, "role_name", req.Name, "scope_type", req.ScopeType)

		if err.Error() == "custom role with name "+req.Name+" already exists in this scope" {
			response.Conflict(c, err.Error())
			return
		}

		response.InternalServerError(c, "Failed to create custom role")
		return
	}

	h.logger.Info("Custom role created successfully", "role_id", role.ID, "role_name", role.Name, "org_id", orgID)

	response.Success(c, role)
}

// GetCustomRoles lists all custom roles for an organization
func (h *Handler) GetCustomRoles(c *gin.Context) {
	// Get organization ID from URL
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "orgId must be a valid UUID"))
		return
	}

	roles, err := h.roleService.GetCustomRolesByOrganization(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("Failed to get custom roles", "error", err, "org_id", orgID)
		response.InternalServerError(c, "Failed to retrieve custom roles")
		return
	}

	response.Success(c, gin.H{
		"roles":       roles,
		"total_count": len(roles),
	})
}

// GetCustomRole retrieves a specific custom role
func (h *Handler) GetCustomRole(c *gin.Context) {
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid role ID", "roleId must be a valid UUID"))
		return
	}

	role, err := h.roleService.GetRoleByID(c.Request.Context(), roleID)
	if err != nil {
		h.logger.Error("Failed to get custom role", "error", err, "role_id", roleID)
		response.NotFound(c, "Custom role not found")
		return
	}

	// Verify it's a custom role (not system role)
	if role.IsSystemRole() {
		response.Error(c, appErrors.NewValidationError("Cannot access system role through custom role endpoint", "use /rbac/roles endpoint for system roles"))
		return
	}

	response.Success(c, role)
}

// UpdateCustomRole updates a custom role
func (h *Handler) UpdateCustomRole(c *gin.Context) {
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid role ID", "roleId must be a valid UUID"))
		return
	}

	var req auth.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	role, err := h.roleService.UpdateCustomRole(c.Request.Context(), roleID, &req)
	if err != nil {
		h.logger.Error("Failed to update custom role", "error", err, "role_id", roleID)

		if err.Error() == "cannot update system role" {
			response.Forbidden(c, err.Error())
			return
		}

		response.InternalServerError(c, "Failed to update custom role")
		return
	}

	h.logger.Info("Custom role updated successfully", "role_id", role.ID, "role_name", role.Name)

	response.Success(c, role)
}

// DeleteCustomRole deletes a custom role
func (h *Handler) DeleteCustomRole(c *gin.Context) {
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid role ID", "roleId must be a valid UUID"))
		return
	}

	err = h.roleService.DeleteCustomRole(c.Request.Context(), roleID)
	if err != nil {
		h.logger.Error("Failed to delete custom role", "error", err, "role_id", roleID)

		if err.Error() == "cannot delete system role" {
			response.Forbidden(c, err.Error())
			return
		}

		response.InternalServerError(c, "Failed to delete custom role")
		return
	}

	h.logger.Info("Custom role deleted successfully", "role_id", roleID)
	response.NoContent(c)
}

// ========================================
// NEW: SCOPE-BASED AUTHORIZATION ENDPOINTS
// ========================================

// CheckUserScopesRequest represents a request to check user scopes
type CheckUserScopesRequest struct {
	OrganizationID *string  `json:"organization_id"`
	ProjectID      *string  `json:"project_id"`
	Scopes         []string `json:"scopes" binding:"required,min=1"`
}

// CheckUserScopes handles POST /rbac/users/{userId}/scopes/check
func (h *Handler) CheckUserScopes(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	var req CheckUserScopesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Parse organization ID
	var orgID *uuid.UUID
	if req.OrganizationID != nil && *req.OrganizationID != "" {
		parsedOrgID, err := uuid.Parse(*req.OrganizationID)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid organization ID", err.Error()))
			return
		}
		orgID = &parsedOrgID
	}

	// Parse project ID
	var projectID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		parsedProjectID, err := uuid.Parse(*req.ProjectID)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid project ID", err.Error()))
			return
		}
		projectID = &parsedProjectID
	}

	// Check each scope and build result map
	result := make(map[string]bool)
	for _, scope := range req.Scopes {
		hasScope, err := h.scopeService.HasScope(c.Request.Context(), userID, scope, orgID, projectID)
		if err != nil {
			h.logger.Error("Failed to check scope", "error", err, "user_id", userID, "scope", scope)
			// On error, mark as false (safe default)
			result[scope] = false
			continue
		}
		result[scope] = hasScope
	}

	h.logger.Info("User scopes checked successfully", "user_id", userID, "org_id", orgID, "project_id", projectID, "scopes_count", len(req.Scopes))

	response.Success(c, result)
}

// GetUserScopes handles GET /rbac/users/{userId}/scopes
func (h *Handler) GetUserScopes(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid UUID"))
		return
	}

	// Parse organization ID from query
	var orgID *uuid.UUID
	if orgIDStr := c.Query("organization_id"); orgIDStr != "" {
		parsedOrgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid organization ID", err.Error()))
			return
		}
		orgID = &parsedOrgID
	}

	// Parse project ID from query
	var projectID *uuid.UUID
	if projectIDStr := c.Query("project_id"); projectIDStr != "" {
		parsedProjectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid project ID", err.Error()))
			return
		}
		projectID = &parsedProjectID
	}

	// Get user scopes
	scopeResolution, err := h.scopeService.GetUserScopes(c.Request.Context(), userID, orgID, projectID)
	if err != nil {
		h.logger.Error("Failed to get user scopes", "error", err, "user_id", userID, "org_id", orgID, "project_id", projectID)
		response.InternalServerError(c, "Failed to get user scopes")
		return
	}

	h.logger.Info("User scopes retrieved successfully", "user_id", userID, "org_id", orgID, "project_id", projectID, "effective_count", len(scopeResolution.EffectiveScopes))

	response.Success(c, scopeResolution)
}

// GetScopeCategories handles GET /rbac/scopes/categories
func (h *Handler) GetScopeCategories(c *gin.Context) {
	categories, err := h.scopeService.GetScopesByCategory(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get scope categories", "error", err)
		response.InternalServerError(c, "Failed to get scope categories")
		return
	}

	h.logger.Info("Scope categories retrieved successfully", "categories_count", len(categories))
	response.Success(c, categories)
}

// GetAvailableScopes handles GET /rbac/scopes?level=organization
func (h *Handler) GetAvailableScopes(c *gin.Context) {
	levelStr := c.Query("level")
	var level auth.ScopeLevel

	if levelStr != "" {
		level = auth.ScopeLevel(levelStr)
		// Validate scope level
		if level != auth.ScopeLevelOrganization && level != auth.ScopeLevelProject && level != auth.ScopeLevelGlobal {
			response.Error(c, appErrors.NewValidationError("Invalid scope level", "level must be 'organization', 'project', or 'global'"))
			return
		}
	}

	scopes, err := h.scopeService.GetAvailableScopes(c.Request.Context(), level)
	if err != nil {
		h.logger.Error("Failed to get available scopes", "error", err, "level", level)
		response.InternalServerError(c, "Failed to get available scopes")
		return
	}

	h.logger.Info("Available scopes retrieved successfully", "level", level, "scopes_count", len(scopes))

	response.Success(c, scopes)
}

// parseQueryInt parses an integer query parameter with a default value
func parseQueryInt(c *gin.Context, key string, defaultValue int) int {
	if value := c.Query(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
