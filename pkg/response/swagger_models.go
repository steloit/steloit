package response

// This file contains Swagger-specific models for common API structures

// SuccessResponse represents a successful API response
// @Description Standard successful response
type SuccessResponse struct {
	Data    any `json:"data" description:"Response data payload"`
	Meta    *Meta       `json:"meta,omitempty" description:"Response metadata"`
	Success bool        `json:"success" example:"true" description:"Always true for successful responses"`
}

// ErrorResponse represents an error API response
// @Description Standard error response
type ErrorResponse struct {
	Error   *APIError `json:"error" description:"Error details"`
	Meta    *Meta     `json:"meta,omitempty" description:"Response metadata"`
	Success bool      `json:"success" example:"false" description:"Always false for error responses"`
}

// MessageResponse represents a simple message response
// @Description Simple message response for actions
type MessageResponse struct {
	Message string `json:"message" example:"Operation completed successfully" description:"Response message"`
}

// IDResponse represents a response with a single ID
// @Description Response containing an ID reference
type IDResponse struct {
	ID string `json:"id" example:"018f6b6a-1234-7abc-8def-0123456789ab" description:"Resource identifier"`
}

// ListResponse represents a cursor-paginated list response
// @Description Cursor-paginated list response wrapper
type ListResponse struct {
	Data    any `json:"data" description:"Array of items"`
	Meta    *Meta       `json:"meta" description:"Response metadata with cursor pagination"`
	Success bool        `json:"success" example:"true" description:"Request success status"`
}

// HealthResponse represents health check response
// @Description Health check response
type HealthResponse struct {
	Status    string            `json:"status" example:"healthy" description:"Overall health status"`
	Services  map[string]string `json:"services" description:"Individual service health status"`
	Timestamp string            `json:"timestamp" example:"2023-12-01T10:30:00Z" description:"Health check timestamp"`
	Version   string            `json:"version" example:"1.0.0" description:"Application version"`
}

// =============================================================================
// RBAC/Auth Swagger Models (Clean models without GORM fields)
// =============================================================================

// RoleResponse represents a role in API responses
// @Description Role information for API responses
type RoleResponse struct {
	OrganizationID *string              `json:"organization_id,omitempty" example:"01ARZ3NDEKTSV4RRFFQ69G5FAV" description:"Organization ID (null for global system roles)"`
	ID             string               `json:"id" example:"01ARZ3NDEKTSV4RRFFQ69G5FAV" description:"Role unique identifier"`
	Name           string               `json:"name" example:"admin" description:"Role name"`
	DisplayName    string               `json:"display_name" example:"Administrator" description:"Human-readable role name"`
	Description    string               `json:"description" example:"Full administrative access" description:"Role description"`
	CreatedAt      string               `json:"created_at" example:"2023-01-01T00:00:00Z" description:"Role creation timestamp"`
	UpdatedAt      string               `json:"updated_at" example:"2023-01-01T00:00:00Z" description:"Role last update timestamp"`
	Permissions    []PermissionResponse `json:"permissions,omitempty" description:"Role permissions"`
	IsSystemRole   bool                 `json:"is_system_role" example:"false" description:"Whether this is a system-defined role"`
}

// PermissionResponse represents a permission in API responses
// @Description Permission information for API responses
type PermissionResponse struct {
	ID          string `json:"id" example:"01ARZ3NDEKTSV4RRFFQ69G5FAV" description:"Permission unique identifier"`
	Name        string `json:"name" example:"users.read" description:"Legacy permission name (dot notation)"`
	Resource    string `json:"resource" example:"users" description:"Resource name"`
	Action      string `json:"action" example:"read" description:"Action name"`
	DisplayName string `json:"display_name" example:"Read Users" description:"Human-readable permission name"`
	Description string `json:"description" example:"Permission to read user information" description:"Permission description"`
	Category    string `json:"category" example:"users" description:"Permission category"`
	CreatedAt   string `json:"created_at" example:"2023-01-01T00:00:00Z" description:"Permission creation timestamp"`
	UpdatedAt   string `json:"updated_at" example:"2023-01-01T00:00:00Z" description:"Permission last update timestamp"`
}

// RoleListResponse represents a paginated list of roles
// @Description Paginated list of roles
type RoleListResponse struct {
	Roles      []RoleResponse `json:"roles" description:"Array of roles"`
	TotalCount int            `json:"total_count" example:"25" description:"Total number of roles"`
	Page       int            `json:"page,omitempty" example:"1" description:"Current page number"`
	PageSize   int            `json:"page_size,omitempty" example:"10" description:"Number of items per page"`
}

// PermissionListResponse represents a paginated list of permissions
// @Description Paginated list of permissions
type PermissionListResponse struct {
	Permissions []PermissionResponse `json:"permissions" description:"Array of permissions"`
	TotalCount  int                  `json:"total_count" example:"50" description:"Total number of permissions"`
	Page        int                  `json:"page,omitempty" example:"1" description:"Current page number"`
	PageSize    int                  `json:"page_size,omitempty" example:"10" description:"Number of items per page"`
}

// UserPermissionsResponse represents a user's effective permissions in an organization
// @Description User's effective permissions with role information
type UserPermissionsResponse struct {
	UserID          string               `json:"user_id" example:"01ARZ3NDEKTSV4RRFFQ69G5FAV" description:"User unique identifier"`
	OrganizationID  string               `json:"organization_id" example:"01ARZ3NDEKTSV4RRFFQ69G5FAV" description:"Organization unique identifier"`
	Role            *RoleResponse        `json:"role,omitempty" description:"User's assigned role"`
	Permissions     []PermissionResponse `json:"permissions" description:"User's effective permissions"`
	ResourceActions []string             `json:"resource_actions" example:"[\"users:read\", \"projects:write\"]" description:"Permission strings in resource:action format"`
}

// CheckPermissionsResponse represents the result of checking multiple permissions
// @Description Result of checking multiple permissions for a user
type CheckPermissionsResponse struct {
	Results map[string]bool `json:"results" description:"Permission check results (resource:action -> has_permission)"`
}

// RoleStatistics represents statistics about roles in an organization
// @Description Statistics about roles in an organization
type RoleStatistics struct {
	RoleDistribution map[string]int `json:"role_distribution" description:"Distribution of members by role"`
	OrganizationID   string         `json:"organization_id" example:"01ARZ3NDEKTSV4RRFFQ69G5FAV" description:"Organization unique identifier"`
	LastUpdated      string         `json:"last_updated" example:"2023-01-01T00:00:00Z" description:"Last update timestamp"`
	TotalRoles       int            `json:"total_roles" example:"10" description:"Total number of roles"`
	SystemRoles      int            `json:"system_roles" example:"4" description:"Number of system roles"`
	CustomRoles      int            `json:"custom_roles" example:"6" description:"Number of custom roles"`
	TotalMembers     int            `json:"total_members" example:"25" description:"Total organization members"`
	PermissionCount  int            `json:"permission_count" example:"45" description:"Total number of available permissions"`
}
