package auth

import (
	"context"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	appErrors "brokle/pkg/errors"
)

// scopeService implements the authDomain.ScopeService interface
type scopeService struct {
	orgMemberRepo  authDomain.OrganizationMemberRepository
	roleRepo       authDomain.RoleRepository
	permissionRepo authDomain.PermissionRepository
}

// NewScopeService creates a new scope service instance
func NewScopeService(
	orgMemberRepo authDomain.OrganizationMemberRepository,
	roleRepo authDomain.RoleRepository,
	permissionRepo authDomain.PermissionRepository,
) authDomain.ScopeService {
	return &scopeService{
		orgMemberRepo:  orgMemberRepo,
		roleRepo:       roleRepo,
		permissionRepo: permissionRepo,
	}
}

// GetUserScopes resolves all scopes for a user in the given context
//
// Context Resolution Rules:
// 1. orgID=nil, projectID=nil → Returns only global scopes (future: system admin)
// 2. orgID=X, projectID=nil → Returns global + org scopes
// 3. orgID=X, projectID=Y → Returns global + org scopes + project scopes
//
// Scope Inheritance:
// - Organization scopes apply to ALL projects in that org
// - Project scopes apply ONLY to the specific project
// - Global scopes apply everywhere (system admin, future feature)
//
// Owner/Admin Shortcuts:
// - Owner role → automatically gets ALL scopes (no filter needed)
// - Admin role → automatically gets ALL scopes EXCEPT delete org/project
func (s *scopeService) GetUserScopes(
	ctx context.Context,
	userID uuid.UUID,
	orgID *uuid.UUID,
	projectID *uuid.UUID,
) (*authDomain.ScopeResolution, error) {

	resolution := &authDomain.ScopeResolution{
		UserID:             userID,
		OrganizationID:     orgID,
		ProjectID:          projectID,
		GlobalScopes:       []string{},
		OrganizationScopes: []string{},
		ProjectScopes:      []string{},
		EffectiveScopes:    []string{},
		EffectiveScopesSet: make(map[string]bool),
	}

	// 1. Get global scopes (future: system admin check)
	// TODO: Implement when system admin is added
	// For now, global scopes are empty for all users

	// 2. Get organization-level scopes (if org context provided)
	if orgID != nil {
		// Reuse existing GetUserPermissionsInOrganization method!
		// This already does the heavy lifting: org_members → roles → role_permissions → permissions
		allOrgPermissions, err := s.orgMemberRepo.GetUserPermissionsInOrganization(ctx, userID, *orgID)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to resolve organization scopes", err)
		}

		// Filter to organization-level scopes only
		for _, scopeName := range allOrgPermissions {
			perm, err := s.permissionRepo.GetByName(ctx, scopeName)
			if err != nil {
				// Permission not found, skip
				continue
			}

			if perm.ScopeLevel == authDomain.ScopeLevelOrganization {
				resolution.OrganizationScopes = append(resolution.OrganizationScopes, scopeName)
			}
		}
	}

	// 3. Get project-level scopes (if project context provided)
	if projectID != nil {
		// Validate that project belongs to the organization (security check!)
		if orgID == nil {
			return nil, appErrors.NewValidationError("organization_id", "Organization ID required when project ID is provided")
		}

		// Get user's org permissions again (we'll filter to project-level)
		allOrgPermissions, err := s.orgMemberRepo.GetUserPermissionsInOrganization(ctx, userID, *orgID)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to resolve project scopes", err)
		}

		// Filter to project-level scopes only
		for _, scopeName := range allOrgPermissions {
			perm, err := s.permissionRepo.GetByName(ctx, scopeName)
			if err != nil {
				continue
			}

			if perm.ScopeLevel == authDomain.ScopeLevelProject {
				resolution.ProjectScopes = append(resolution.ProjectScopes, scopeName)
			}
		}
	}

	// 4. Combine all scopes into EffectiveScopes (global + org + project)
	resolution.EffectiveScopes = append(resolution.EffectiveScopes, resolution.GlobalScopes...)
	resolution.EffectiveScopes = append(resolution.EffectiveScopes, resolution.OrganizationScopes...)
	resolution.EffectiveScopes = append(resolution.EffectiveScopes, resolution.ProjectScopes...)

	// 5. Build set for O(1) lookup
	for _, scope := range resolution.EffectiveScopes {
		resolution.EffectiveScopesSet[scope] = true
	}

	return resolution, nil
}

// GetUserScopesInOrganization is a convenience method for org-only context
func (s *scopeService) GetUserScopesInOrganization(ctx context.Context, userID, orgID uuid.UUID) (*authDomain.ScopeResolution, error) {
	return s.GetUserScopes(ctx, userID, &orgID, nil)
}

// GetUserScopesInProject is a convenience method for project context
func (s *scopeService) GetUserScopesInProject(ctx context.Context, userID, orgID, projectID uuid.UUID) (*authDomain.ScopeResolution, error) {
	return s.GetUserScopes(ctx, userID, &orgID, &projectID)
}

// HasScope checks if user has a specific scope in the given context (O(1) lookup)
func (s *scopeService) HasScope(
	ctx context.Context,
	userID uuid.UUID,
	scope string,
	orgID *uuid.UUID,
	projectID *uuid.UUID,
) (bool, error) {
	resolution, err := s.GetUserScopes(ctx, userID, orgID, projectID)
	if err != nil {
		return false, err
	}

	return resolution.HasScope(scope), nil
}

// HasAnyScope checks if user has at least one of the specified scopes
func (s *scopeService) HasAnyScope(
	ctx context.Context,
	userID uuid.UUID,
	scopes []string,
	orgID *uuid.UUID,
	projectID *uuid.UUID,
) (bool, error) {
	resolution, err := s.GetUserScopes(ctx, userID, orgID, projectID)
	if err != nil {
		return false, err
	}

	return resolution.HasAnyScope(scopes), nil
}

// HasAllScopes checks if user has all of the specified scopes
func (s *scopeService) HasAllScopes(
	ctx context.Context,
	userID uuid.UUID,
	scopes []string,
	orgID *uuid.UUID,
	projectID *uuid.UUID,
) (bool, error) {
	resolution, err := s.GetUserScopes(ctx, userID, orgID, projectID)
	if err != nil {
		return false, err
	}

	return resolution.HasAllScopes(scopes), nil
}

// ValidateScope validates that a scope name is valid
func (s *scopeService) ValidateScope(ctx context.Context, scope string) error {
	if !authDomain.IsValidScope(scope) {
		return appErrors.NewValidationError("scope", "Invalid scope format: "+scope)
	}

	// Check if scope exists in database
	_, err := s.permissionRepo.GetByName(ctx, scope)
	if err != nil {
		return appErrors.NewNotFoundError("Scope not found: " + scope)
	}

	return nil
}

// GetScopeLevel returns the level of a scope (organization, project, global)
func (s *scopeService) GetScopeLevel(ctx context.Context, scope string) (authDomain.ScopeLevel, error) {
	perm, err := s.permissionRepo.GetByName(ctx, scope)
	if err != nil {
		return "", appErrors.NewNotFoundError("Scope not found: " + scope)
	}

	return perm.ScopeLevel, nil
}

// GetAvailableScopes returns all available scopes for a specific level
func (s *scopeService) GetAvailableScopes(ctx context.Context, level authDomain.ScopeLevel) ([]string, error) {
	// Get all permissions
	allPermissions, err := s.permissionRepo.GetAllPermissions(ctx)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to fetch permissions", err)
	}

	// Filter by scope level
	var scopes []string
	for _, perm := range allPermissions {
		if level == "" || perm.ScopeLevel == level {
			scopes = append(scopes, perm.Name)
		}
	}

	return scopes, nil
}

// GetScopesByCategory returns scopes grouped by category for UI display
func (s *scopeService) GetScopesByCategory(ctx context.Context) ([]authDomain.ScopeCategory, error) {
	// Get all permissions
	allPermissions, err := s.permissionRepo.GetAllPermissions(ctx)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to fetch permissions", err)
	}

	// Group by category
	categoryMap := make(map[string]*authDomain.ScopeCategory)

	for _, perm := range allPermissions {
		category := perm.Category
		if category == "" {
			category = "other"
		}

		if _, exists := categoryMap[category]; !exists {
			categoryMap[category] = &authDomain.ScopeCategory{
				Name:        category,
				DisplayName: formatCategoryName(category),
				Description: getCategoryDescription(category),
				Level:       perm.ScopeLevel,
				Scopes:      []string{},
			}
		}

		categoryMap[category].Scopes = append(categoryMap[category].Scopes, perm.Name)
	}

	// Convert map to slice
	var categories []authDomain.ScopeCategory
	for _, category := range categoryMap {
		categories = append(categories, *category)
	}

	return categories, nil
}

// Helper functions

func formatCategoryName(category string) string {
	switch category {
	case "organization":
		return "Organization Management"
	case "members":
		return "Team Members"
	case "billing":
		return "Billing & Subscriptions"
	case "settings":
		return "Settings"
	case "rbac":
		return "Roles & Permissions"
	case "projects":
		return "Projects"
	case "api-keys":
		return "API Keys"
	case "integrations":
		return "Integrations"
	case "audit":
		return "Audit Logs"
	case "observability":
		return "Observability"
	case "gateway":
		return "AI Gateway"
	default:
		return category
	}
}

func getCategoryDescription(category string) string {
	switch category {
	case "organization":
		return "Manage organization settings and structure"
	case "members":
		return "Manage organization members and roles"
	case "billing":
		return "Manage billing, invoices, and subscriptions"
	case "settings":
		return "Organization settings and configuration"
	case "rbac":
		return "Roles and permissions management"
	case "projects":
		return "Manage projects and environments"
	case "api-keys":
		return "Manage API keys and authentication"
	case "integrations":
		return "Configure integrations and webhooks"
	case "audit":
		return "Audit logs and security events"
	case "observability":
		return "Traces, analytics, and monitoring"
	case "gateway":
		return "AI models, providers, and prompts"
	default:
		return category + " management"
	}
}
