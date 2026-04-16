package auth

import (
	"github.com/google/uuid"
)

// Note: ScopeLevel type and constants (ScopeLevelGlobal, ScopeLevelOrganization, ScopeLevelProject)
// are defined in auth.go to avoid import cycles and keep types together

// ScopeResolution represents the resolved scopes for a user in a specific context.
//
// Scope Hierarchy Rules:
// 1. Global scopes apply everywhere (system admin only, future)
// 2. Organization scopes apply to the org + ALL its projects
// 3. Project scopes apply ONLY to the specific project
// 4. Effective scopes = Global ∪ Organization ∪ Project
//
// Owner/Admin Shortcuts:
// - Owner at org level → gets ALL org + project scopes for that org
// - Admin at org level → gets ALL org + project scopes EXCEPT delete org/project
// - Developer at org level → gets project scopes + limited org scopes
// - Viewer at org level → gets read-only scopes only
//
// Examples:
//
//	// Org context only (no project)
//	resolution := GetUserScopes(userID, orgID=X, projectID=nil)
//	→ Returns: GlobalScopes + OrgScopes
//	→ ProjectScopes = [] (no project context)
//
//	// Project context (org + project)
//	resolution := GetUserScopes(userID, orgID=X, projectID=Y)
//	→ Returns: GlobalScopes + OrgScopes + ProjectScopes
//	→ All three levels combined
type ScopeResolution struct {
	OrganizationID     *uuid.UUID      `json:"organization_id,omitempty"`
	ProjectID          *uuid.UUID      `json:"project_id,omitempty"`
	EffectiveScopesSet map[string]bool `json:"-"`
	GlobalScopes       []string        `json:"global_scopes"`
	OrganizationScopes []string        `json:"organization_scopes"`
	ProjectScopes      []string        `json:"project_scopes"`
	EffectiveScopes    []string        `json:"effective_scopes"`
	UserID             uuid.UUID       `json:"user_id"`
}

// HasScope checks if a specific scope exists in the resolution (O(1) lookup)
func (sr *ScopeResolution) HasScope(scope string) bool {
	if sr.EffectiveScopesSet == nil {
		// Build set if not initialized
		sr.EffectiveScopesSet = make(map[string]bool)
		for _, s := range sr.EffectiveScopes {
			sr.EffectiveScopesSet[s] = true
		}
	}
	return sr.EffectiveScopesSet[scope]
}

// HasAnyScope checks if user has at least one of the specified scopes
func (sr *ScopeResolution) HasAnyScope(scopes []string) bool {
	for _, scope := range scopes {
		if sr.HasScope(scope) {
			return true
		}
	}
	return false
}

// HasAllScopes checks if user has all of the specified scopes
func (sr *ScopeResolution) HasAllScopes(scopes []string) bool {
	for _, scope := range scopes {
		if !sr.HasScope(scope) {
			return false
		}
	}
	return true
}

// GetScopesByLevel returns scopes filtered by level
func (sr *ScopeResolution) GetScopesByLevel(level ScopeLevel) []string {
	switch level {
	case ScopeLevelGlobal:
		return sr.GlobalScopes
	case ScopeLevelOrganization:
		return sr.OrganizationScopes
	case ScopeLevelProject:
		return sr.ProjectScopes
	default:
		return []string{}
	}
}

// ScopeCategory groups related scopes for UI display
type ScopeCategory struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Description string     `json:"description"`
	Level       ScopeLevel `json:"level"`
	Scopes      []string   `json:"scopes"`
}

// Standard scope categories for UI grouping
var ScopeCategories = []ScopeCategory{
	{
		Name:        "organization",
		DisplayName: "Organization Management",
		Description: "Manage organization settings and structure",
		Level:       ScopeLevelOrganization,
		Scopes:      []string{"organizations:read", "organizations:write", "organizations:delete", "organizations:admin"},
	},
	{
		Name:        "members",
		DisplayName: "Team Members",
		Description: "Manage organization members and roles",
		Level:       ScopeLevelOrganization,
		Scopes:      []string{"members:read", "members:invite", "members:update", "members:remove", "members:suspend"},
	},
	{
		Name:        "billing",
		DisplayName: "Billing & Subscriptions",
		Description: "Manage billing, invoices, and subscriptions",
		Level:       ScopeLevelOrganization,
		Scopes:      []string{"billing:read", "billing:manage", "billing:export", "billing:admin"},
	},
	{
		Name:        "settings",
		DisplayName: "Settings",
		Description: "Organization settings and configuration",
		Level:       ScopeLevelOrganization,
		Scopes:      []string{"settings:read", "settings:write", "settings:export", "settings:import", "settings:security", "settings:admin"},
	},
	{
		Name:        "projects",
		DisplayName: "Projects",
		Description: "Manage projects and environments",
		Level:       ScopeLevelOrganization,
		Scopes:      []string{"projects:read", "projects:write", "projects:delete", "projects:admin"},
	},
	{
		Name:        "observability",
		DisplayName: "Observability",
		Description: "Traces, analytics, and monitoring",
		Level:       ScopeLevelProject,
		Scopes:      []string{"traces:read", "traces:create", "traces:delete", "traces:export", "traces:share", "analytics:read", "analytics:export", "analytics:dashboards", "analytics:admin", "costs:read", "costs:export"},
	},
}

// GetScopeLevel determines the level of a scope from its name
// This is a helper for validation and UI purposes
func GetScopeLevel(scopeName string) ScopeLevel {
	// Project-level scope prefixes
	projectPrefixes := []string{"traces:", "analytics:", "costs:"}

	for _, prefix := range projectPrefixes {
		if len(scopeName) > len(prefix) && scopeName[:len(prefix)] == prefix {
			return ScopeLevelProject
		}
	}

	// Default to organization level
	return ScopeLevelOrganization
}

// IsValidScope checks if a scope name is valid (basic validation)
func IsValidScope(scopeName string) bool {
	// Must be in resource:action format
	if !IsValidResourceActionFormat(scopeName) {
		return false
	}

	// Additional validation can be added here
	return true
}

// IsValidResourceActionFormat checks if string is valid resource:action format
func IsValidResourceActionFormat(resourceAction string) bool {
	// Simple validation: must contain exactly one colon
	colonCount := 0
	for _, char := range resourceAction {
		if char == ':' {
			colonCount++
		}
	}
	return colonCount == 1 && len(resourceAction) > 2
}
