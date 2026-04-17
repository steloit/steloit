// Package auth provides the authentication and authorization domain model.
//
// The auth domain handles JWT tokens, sessions, API keys, roles, permissions,
// and role-based access control (RBAC) across the platform.
package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

// UserSession represents an active user session with secure token management.
// SECURITY: Access tokens are NOT stored - only session metadata and hashed refresh tokens.
type UserSession struct {
	ExpiresAt           time.Time   `json:"expires_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	CreatedAt           time.Time   `json:"created_at"`
	RefreshExpiresAt    time.Time   `json:"refresh_expires_at"`
	DeviceInfo          interface{} `json:"device_info,omitempty"`
	IPAddress           *string     `json:"ip_address,omitempty"`
	UserAgent           *string     `json:"user_agent,omitempty"`
	LastUsedAt          *time.Time  `json:"last_used_at,omitempty"`
	RevokedAt           *time.Time  `json:"revoked_at,omitempty"`
	CurrentJTI          string      `json:"-"`
	RefreshTokenHash    string      `json:"-"`
	RefreshTokenVersion int         `json:"refresh_token_version"`
	ID                  uuid.UUID   `json:"id"`
	UserID              uuid.UUID   `json:"user_id"`
	IsActive            bool        `json:"is_active"`
}

// BlacklistedToken represents a revoked access token for immediate revocation capability.
type BlacklistedToken struct {
	ExpiresAt          time.Time `json:"expires_at"`
	RevokedAt          time.Time `json:"revoked_at"`
	CreatedAt          time.Time `json:"created_at"`
	BlacklistTimestamp *int64    `json:"blacklist_timestamp,omitempty"`
	JTI                string    `json:"jti"`
	Reason             string    `json:"reason"`
	TokenType          string    `json:"token_type"`
	UserID             uuid.UUID `json:"user_id"`
}

// SessionStats represents session statistics and metrics
type SessionStats struct {
	ActiveSessions   int64 `json:"active_sessions"`
	ExpiredSessions  int64 `json:"expired_sessions"`
	TotalSessions    int64 `json:"total_sessions"`
	SessionsToday    int64 `json:"sessions_today"`
	SessionsThisWeek int64 `json:"sessions_this_week"`
	AvgSessionLength int64 `json:"avg_session_length_minutes"`
}

// External repository interfaces to avoid circular imports
// These will be implemented by the actual user and organization repositories

// UserRepository defines the interface for user data access needed by auth services
type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (interface{}, error)
	GetByEmail(ctx context.Context, email string) (interface{}, error)
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error
}

// OrganizationRepository defines the interface for organization data access needed by auth services
type OrganizationRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (interface{}, error)
	IsMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error)
}

// APIKey represents an industry-standard API key with secure hash storage.
// Format: bk_{40_char_random}
// Security: Full key is hashed with SHA-256 (deterministic, enables O(1) lookup)
// Organization is derived via projects.organization_id (no redundant storage)
// Status: Determined by deleted_at (soft delete) and expires_at (expiration)
type APIKey struct {
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	LastUsedAt *time.Time     `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
	KeyHash    string         `json:"-"`
	KeyPreview string         `json:"key_preview"`
	Name       string         `json:"name"`
	ID         uuid.UUID      `json:"id"`
	ProjectID  uuid.UUID      `json:"project_id"`
	UserID     uuid.UUID      `json:"user_id"`
}

// Role represents both system template roles and custom scoped roles
type Role struct {
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	ScopeID         *uuid.UUID       `json:"scope_id,omitempty"`
	Name            string           `json:"name"`
	ScopeType       string           `json:"scope_type"`
	Description     string           `json:"description"`
	Permissions     []Permission     `json:"permissions,omitempty"`
	RolePermissions []RolePermission `json:"role_permissions,omitempty"`
	ID              uuid.UUID        `json:"id"`
}

// OrganizationMember represents user membership in an organization with a single role
type OrganizationMember struct {
	JoinedAt       time.Time  `json:"joined_at"`
	InvitedBy      *uuid.UUID `json:"invited_by,omitempty"`
	Role           *Role      `json:"role,omitempty"`
	Status         string     `json:"status"`
	UserID         uuid.UUID  `json:"user_id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	RoleID         uuid.UUID  `json:"role_id"`
}

// ProjectMember represents user membership in a project with a single role (future)
type ProjectMember struct {
	JoinedAt  time.Time `json:"joined_at"`
	Role      *Role     `json:"role,omitempty"`
	Status    string    `json:"status"`
	UserID    uuid.UUID `json:"user_id"`
	ProjectID uuid.UUID `json:"project_id"`
	RoleID    uuid.UUID `json:"role_id"`
}

// Scope constants for roles
const (
	ScopeSystem       = "system"       // System template roles
	ScopeOrganization = "organization" // Organization-specific roles
	ScopeProject      = "project"      // Project-specific roles
)

// Membership status constants
const (
	MemberStatusActive    = "active"
	MemberStatusInvited   = "invited"
	MemberStatusSuspended = "suspended"
)

// Helper methods for scoped roles
func (r *Role) IsSystemRole() bool {
	return r.ScopeType == ScopeSystem && r.ScopeID == nil
}

func (r *Role) IsCustomRole() bool {
	return r.ScopeType != ScopeSystem && r.ScopeID != nil
}

func (r *Role) IsOrganizationRole() bool {
	return r.ScopeType == ScopeOrganization
}

func (r *Role) IsProjectRole() bool {
	return r.ScopeType == ScopeProject
}

func (r *Role) GetScopeDisplay() string {
	switch r.ScopeType {
	case ScopeSystem:
		return "System"
	case ScopeOrganization:
		if r.ScopeID == nil {
			return "Organization Template"
		}
		return "Organization Custom"
	case ScopeProject:
		return "Project"
	default:
		return "Unknown"
	}
}

// Helper methods for organization membership
func (m *OrganizationMember) IsActive() bool {
	return m.Status == MemberStatusActive
}

func (m *OrganizationMember) IsInvited() bool {
	return m.Status == MemberStatusInvited
}

func (m *OrganizationMember) IsSuspended() bool {
	return m.Status == MemberStatusSuspended
}

func (m *OrganizationMember) Activate() {
	m.Status = MemberStatusActive
}

func (m *OrganizationMember) Suspend() {
	m.Status = MemberStatusSuspended
}

// Helper methods for project membership
func (m *ProjectMember) IsActive() bool {
	return m.Status == MemberStatusActive
}

func (m *ProjectMember) Activate() {
	m.Status = MemberStatusActive
}

// ScopeLevel defines where a scope applies in the hierarchy
type ScopeLevel string

const (
	// ScopeLevelGlobal: Platform-wide scopes (system admin only, future feature)
	ScopeLevelGlobal ScopeLevel = "global"

	// ScopeLevelOrganization: Organization-wide scopes (apply to org + all its projects)
	ScopeLevelOrganization ScopeLevel = "organization"

	// ScopeLevelProject: Project-specific scopes (apply only within a specific project)
	ScopeLevelProject ScopeLevel = "project"
)

// Permission represents a normalized permission using resource:action format
type Permission struct {
	CreatedAt   time.Time  `json:"created_at"`
	Name        string     `json:"name"`
	Resource    string     `json:"resource"`
	Action      string     `json:"action"`
	Description string     `json:"description"`
	ScopeLevel  ScopeLevel `json:"scope_level"`
	Category    string     `json:"category"`
	Roles       []Role     `json:"roles,omitempty"`
	ID          uuid.UUID  `json:"id"`
}

// GetResourceAction returns the resource:action format string
func (p *Permission) GetResourceAction() string {
	return fmt.Sprintf("%s:%s", p.Resource, p.Action)
}

// IsWildcardPermission returns true if this is a wildcard permission (*:* or resource:*)
func (p *Permission) IsWildcardPermission() bool {
	return p.Resource == "*" || p.Action == "*"
}

// MatchesResourceAction checks if this permission matches the given resource:action
func (p *Permission) MatchesResourceAction(resource, action string) bool {
	// Exact match
	if p.Resource == resource && p.Action == action {
		return true
	}
	// Wildcard resource match
	if p.Resource == "*" && p.Action == action {
		return true
	}
	// Wildcard action match
	if p.Resource == resource && p.Action == "*" {
		return true
	}
	// Full wildcard match
	if p.Resource == "*" && p.Action == "*" {
		return true
	}
	return false
}

// ParseResourceAction parses a resource:action string into resource and action components
func ParseResourceAction(resourceAction string) (resource, action string, err error) {
	parts := strings.Split(resourceAction, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid resource:action format: %s", resourceAction)
	}
	return parts[0], parts[1], nil
}

// ValidateResourceAction validates a resource:action string format
func ValidateResourceAction(resourceAction string) error {
	_, _, err := ParseResourceAction(resourceAction)
	return err
}

// RolePermission represents the many-to-many relationship between template roles and permissions
type RolePermission struct {
	GrantedAt    time.Time  `json:"granted_at"`
	GrantedBy    *uuid.UUID `json:"granted_by,omitempty"`
	Role         Role       `json:"role,omitempty"`
	Permission   Permission `json:"permission,omitempty"`
	RoleID       uuid.UUID  `json:"role_id"`
	PermissionID uuid.UUID  `json:"permission_id"`
}

// AuditLog represents an audit log entry for compliance.
type AuditLog struct {
	CreatedAt      time.Time  `json:"created_at"`
	UserID         *uuid.UUID `json:"user_id,omitempty"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"`
	Action         string     `json:"action"`
	Resource       string     `json:"resource"`
	ResourceID     string     `json:"resource_id"`
	Metadata       string     `json:"metadata"`
	IPAddress      string     `json:"ip_address"`
	UserAgent      string     `json:"user_agent"`
	ID             uuid.UUID  `json:"id"`
}

// Request/Response DTOs
type LoginRequest struct {
	DeviceInfo map[string]interface{} `json:"device_info,omitempty"`
	Email      string                 `json:"email" validate:"required,email"`
	Password   string                 `json:"password" validate:"required"`
	Remember   bool                   `json:"remember"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"` // Always "Bearer"
	ExpiresIn    int64  `json:"expires_in"` // Seconds until expiration
}

type AuthUser struct {
	AvatarURL             *string    `json:"avatar_url,omitempty"`
	DefaultOrganizationID *uuid.UUID `json:"default_organization_id,omitempty"`
	Email                 string     `json:"email"`
	Name                  string     `json:"name"`
	ID                    uuid.UUID  `json:"id"`
	IsEmailVerified       bool       `json:"is_email_verified"`
}

type CreateAPIKeyRequest struct {
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Name      string     `json:"name" validate:"required,min=1,max=100"`
	ProjectID uuid.UUID  `json:"project_id" validate:"required"`
}

type CreateAPIKeyResponse struct {
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	ID         string     `json:"id" example:"key_01234567890123456789012345"`
	Name       string     `json:"name" example:"Production API Key"`
	Key        string     `json:"key" example:"bk_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCd"`
	KeyPreview string     `json:"key_preview" example:"bk_AbCd...AbCd"`
	ProjectID  string     `json:"project_id" example:"proj_01234567890123456789012345"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// AuthContext represents the authenticated context for a request.
// AuthContext represents clean user identity context (permissions resolved dynamically)
type AuthContext struct {
	APIKeyID  *uuid.UUID `json:"api_key_id,omitempty"`
	SessionID *uuid.UUID `json:"session_id,omitempty"`
	UserID    uuid.UUID  `json:"user_id"`
}

// Deprecated: StandardPermissions removed
// Use scope-based permissions instead (see seeds/dev.yaml for full list)
// Organization-level: organizations:*, members:*, billing:*, settings:*, etc.
// Project-level: traces:*, analytics:*, models:*, providers:*, etc.

// Blacklisted token types
const (
	TokenTypeIndividual    = "individual"          // Individual JTI-based blacklisting (default)
	TokenTypeUserTimestamp = "user_wide_timestamp" // User-wide timestamp blacklisting (GDPR/SOC2)
)

// Deprecated: SystemRoles removed
// Use scope-based role mappings instead (see seeds/dev.yaml)
// Roles are now seeded with proper scope assignments:
//   - owner: 63 scopes (full access)
//   - admin: 61 scopes (no delete org/projects)
//   - developer: 30 scopes (project workflows)
//   - viewer: 15 scopes (read-only)

// Constructor functions
func NewUserSession(userID uuid.UUID, refreshTokenHash string, currentJTI string, expiresAt, refreshExpiresAt time.Time, ipAddress, userAgent *string, deviceInfo interface{}) *UserSession {
	return &UserSession{
		ID:                  uid.New(),
		UserID:              userID,
		RefreshTokenHash:    refreshTokenHash,
		RefreshTokenVersion: 1,
		CurrentJTI:          currentJTI,
		ExpiresAt:           expiresAt,
		RefreshExpiresAt:    refreshExpiresAt,
		IPAddress:           ipAddress,
		UserAgent:           userAgent,
		DeviceInfo:          deviceInfo,
		IsActive:            true,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
}

func NewBlacklistedToken(jti string, userID uuid.UUID, expiresAt time.Time, reason string) *BlacklistedToken {
	return &BlacklistedToken{
		JTI:       jti,
		UserID:    userID,
		ExpiresAt: expiresAt,
		RevokedAt: time.Now(),
		Reason:    reason,
		TokenType: TokenTypeIndividual, // Default to individual JTI blacklisting
		CreatedAt: time.Now(),
	}
}

// NewUserTimestampBlacklistedToken creates a user-wide timestamp blacklist entry for GDPR/SOC2 compliance
func NewUserTimestampBlacklistedToken(userID uuid.UUID, blacklistTimestamp int64, reason string) *BlacklistedToken {
	// Generate a proper UUID for this user-wide blacklist entry
	userWideJTI := uid.New()

	// Set expiry far in the future to cover all possible access token lifetimes
	// We use the blacklist timestamp + reasonable buffer (24 hours) to ensure cleanup
	farFutureExpiry := time.Unix(blacklistTimestamp, 0).Add(24 * time.Hour)

	return &BlacklistedToken{
		JTI:                userWideJTI.String(),
		UserID:             userID,
		ExpiresAt:          farFutureExpiry,
		RevokedAt:          time.Now(),
		Reason:             reason,
		TokenType:          TokenTypeUserTimestamp,
		BlacklistTimestamp: &blacklistTimestamp,
		CreatedAt:          time.Now(),
	}
}

func NewAPIKey(userID, projectID uuid.UUID, name, keyHash, keyPreview string, expiresAt *time.Time) *APIKey {
	return &APIKey{
		ID:         uid.New(),
		KeyHash:    keyHash,
		KeyPreview: keyPreview,
		ProjectID:  projectID,
		UserID:     userID,
		Name:       name,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

func NewRole(name, scopeType, description string) *Role {
	return &Role{
		ID:          uid.New(),
		Name:        name,
		ScopeType:   scopeType,
		ScopeID:     nil, // System/template role
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewCustomRole creates a custom role scoped to a specific organization/project/environment
func NewCustomRole(name, scopeType, description string, scopeID uuid.UUID) *Role {
	return &Role{
		ID:          uid.New(),
		Name:        name,
		ScopeType:   scopeType,
		ScopeID:     &scopeID,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func NewOrganizationMember(userID, organizationID, roleID uuid.UUID, invitedBy *uuid.UUID) *OrganizationMember {
	return &OrganizationMember{
		UserID:         userID,
		OrganizationID: organizationID,
		RoleID:         roleID,
		Status:         MemberStatusActive,
		JoinedAt:       time.Now(),
		InvitedBy:      invitedBy,
	}
}

func NewProjectMember(userID, projectID, roleID uuid.UUID) *ProjectMember {
	return &ProjectMember{
		UserID:    userID,
		ProjectID: projectID,
		RoleID:    roleID,
		Status:    MemberStatusActive,
		JoinedAt:  time.Now(),
	}
}

func NewPermission(resource, action, description string) *Permission {
	name := fmt.Sprintf("%s:%s", resource, action)

	return &Permission{
		ID:          uid.New(),
		Name:        name,
		Resource:    resource,
		Action:      action,
		Description: description,
		ScopeLevel:  ScopeLevelOrganization, // Default to organization level
		Category:    resource,               // Default category to resource name
		CreatedAt:   time.Now(),
	}
}

// NewPermissionWithScope creates a permission with explicit scope level and category
func NewPermissionWithScope(resource, action, description string, scopeLevel ScopeLevel, category string) *Permission {
	name := fmt.Sprintf("%s:%s", resource, action)

	return &Permission{
		ID:          uid.New(),
		Name:        name,
		Resource:    resource,
		Action:      action,
		Description: description,
		ScopeLevel:  scopeLevel,
		Category:    category,
		CreatedAt:   time.Now(),
	}
}

// PasswordResetToken represents a password reset token
type PasswordResetToken struct {
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	Token     string     `json:"-"`
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
}

func NewAuditLog(userID, orgID *uuid.UUID, action, resource, resourceID, metadata, ipAddress, userAgent string) *AuditLog {
	return &AuditLog{
		ID:             uid.New(),
		UserID:         userID,
		OrganizationID: orgID,
		Action:         action,
		Resource:       resource,
		ResourceID:     resourceID,
		Metadata:       metadata,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		CreatedAt:      time.Now(),
	}
}

func NewPasswordResetToken(userID uuid.UUID, token string, expiresAt time.Time) *PasswordResetToken {
	return &PasswordResetToken{
		ID:        uid.New(),
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// ValidateAPIKeyResponse represents the response from API key validation
type ValidateAPIKeyResponse struct {
	APIKey         *APIKey      `json:"api_key"`
	AuthContext    *AuthContext `json:"auth_context,omitempty"`
	ProjectID      uuid.UUID    `json:"project_id"`
	OrganizationID uuid.UUID    `json:"organization_id"`
	Valid          bool         `json:"valid"`
}

// Utility methods
func (s *UserSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSession) IsRefreshExpired() bool {
	return time.Now().After(s.RefreshExpiresAt)
}

func (s *UserSession) IsValid() bool {
	return s.IsActive && !s.IsExpired() && s.RevokedAt == nil
}

func (s *UserSession) MarkAsUsed() {
	now := time.Now()
	s.LastUsedAt = &now
	s.UpdatedAt = now
}

func (s *UserSession) Revoke() {
	now := time.Now()
	s.RevokedAt = &now
	s.IsActive = false
	s.UpdatedAt = now
}

func (s *UserSession) Deactivate() {
	s.IsActive = false
	s.UpdatedAt = time.Now()
}

func (k *APIKey) IsExpired() bool {
	return k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt)
}

func (k *APIKey) IsValid() bool {
	// Deleted keys are filtered by GORM soft delete
	// Only check if not expired
	return !k.IsExpired()
}

func (k *APIKey) MarkAsUsed() {
	now := time.Now()
	k.LastUsedAt = &now
	k.UpdatedAt = now
}

func (r *Role) AddPermission(permissionID uuid.UUID, grantedBy *uuid.UUID) *RolePermission {
	return &RolePermission{
		RoleID:       r.ID,
		PermissionID: permissionID,
		GrantedAt:    time.Now(),
		GrantedBy:    grantedBy,
	}
}

// RBAC Request/Response DTOs

// CreateRoleRequest represents a request to create a new role
type CreateRoleRequest struct {
	ScopeType     string      `json:"scope_type" validate:"required,oneof=system organization project environment"`
	Name          string      `json:"name" validate:"required,min=1,max=100"`
	Description   string      `json:"description,omitempty"`
	PermissionIDs []uuid.UUID `json:"permission_ids,omitempty"`
}

// UpdateRoleRequest represents a request to update an existing role
type UpdateRoleRequest struct {
	Description   *string     `json:"description,omitempty"`
	PermissionIDs []uuid.UUID `json:"permission_ids,omitempty"`
}

// CreatePermissionRequest represents a request to create a new permission
type CreatePermissionRequest struct {
	Resource    string `json:"resource" validate:"required,min=1,max=50"`
	Action      string `json:"action" validate:"required,min=1,max=50"`
	Description string `json:"description,omitempty"`
}

// UpdatePermissionRequest represents a request to update an existing permission
type UpdatePermissionRequest struct {
	Description *string `json:"description,omitempty"`
}

// AssignRoleRequest represents a request to assign a role to a user
type AssignRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" validate:"required"`
}

// RoleListResponse represents a list of roles with metadata
type RoleListResponse struct {
	Roles      []*Role `json:"roles"`
	TotalCount int     `json:"total_count"`
	Page       int     `json:"page,omitempty"`
	PageSize   int     `json:"page_size,omitempty"`
}

// PermissionListResponse represents a list of permissions with metadata
type PermissionListResponse struct {
	Permissions []*Permission `json:"permissions"`
	TotalCount  int           `json:"total_count"`
	Page        int           `json:"page,omitempty"`
	PageSize    int           `json:"page_size,omitempty"`
}

// UserPermissionsResponse represents a user's effective permissions across all scopes
type UserPermissionsResponse struct {
	Roles           []*Role       `json:"roles"`
	Permissions     []*Permission `json:"permissions"`
	ResourceActions []string      `json:"resource_actions"`
	UserID          uuid.UUID     `json:"user_id"`
}

// CheckPermissionsRequest represents a request to check multiple permissions
type CheckPermissionsRequest struct {
	ResourceActions []string `json:"resource_actions" validate:"required,min=1"`
}

// CheckPermissionsResponse represents the result of checking multiple permissions
type CheckPermissionsResponse struct {
	Results map[string]bool `json:"results"` // resource:action -> has_permission
}

// RoleStatistics represents statistics about roles across all scopes
type RoleStatistics struct {
	LastUpdated       time.Time      `json:"last_updated"`
	ScopeDistribution map[string]int `json:"scope_distribution"`
	RoleDistribution  map[string]int `json:"role_distribution"`
	TotalRoles        int            `json:"total_roles"`
	SystemRoles       int            `json:"system_roles"`
	OrganizationRoles int            `json:"organization_roles"`
	ProjectRoles      int            `json:"project_roles"`
	PermissionCount   int            `json:"permission_count"`
}

