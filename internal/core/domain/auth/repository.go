package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/pagination"
)

// UserSessionRepository defines the interface for user session data access.
type UserSessionRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, session *UserSession) error
	GetByID(ctx context.Context, id uuid.UUID) (*UserSession, error)
	GetByJTI(ctx context.Context, jti string) (*UserSession, error)                           // Get session by JWT ID (current access token)
	GetByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (*UserSession, error) // Get session by refresh token hash
	Update(ctx context.Context, session *UserSession) error
	Delete(ctx context.Context, id uuid.UUID) error

	// User sessions
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*UserSession, error)
	GetActiveSessionsByUserID(ctx context.Context, userID uuid.UUID) ([]*UserSession, error)

	// Session management
	DeactivateSession(ctx context.Context, id uuid.UUID) error
	DeactivateUserSessions(ctx context.Context, userID uuid.UUID) error
	RevokeSession(ctx context.Context, id uuid.UUID) error
	RevokeUserSessions(ctx context.Context, userID uuid.UUID) error
	CleanupExpiredSessions(ctx context.Context) error
	CleanupRevokedSessions(ctx context.Context) error
	MarkAsUsed(ctx context.Context, id uuid.UUID) error

	// Device-specific queries
	GetByDeviceInfo(ctx context.Context, userID uuid.UUID, deviceInfo any) ([]*UserSession, error)
	GetActiveSessionsCount(ctx context.Context, userID uuid.UUID) (int, error)
}

// BlacklistedTokenRepository defines the interface for blacklisted token data access.
type BlacklistedTokenRepository interface {
	// Basic operations
	Create(ctx context.Context, blacklistedToken *BlacklistedToken) error
	GetByJTI(ctx context.Context, jti string) (*BlacklistedToken, error)
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)

	// User-wide timestamp blacklisting (GDPR/SOC2 compliance)
	CreateUserTimestampBlacklist(ctx context.Context, userID uuid.UUID, blacklistTimestamp int64, reason string) error
	IsUserBlacklistedAfterTimestamp(ctx context.Context, userID uuid.UUID, tokenIssuedAt int64) (bool, error)
	GetUserBlacklistTimestamp(ctx context.Context, userID uuid.UUID) (*int64, error)

	// Cleanup operations
	CleanupExpiredTokens(ctx context.Context) error
	CleanupTokensOlderThan(ctx context.Context, olderThan time.Time) error

	// Bulk operations
	BlacklistUserTokens(ctx context.Context, userID uuid.UUID, reason string) error
	GetBlacklistedTokensByUser(ctx context.Context, filters *BlacklistedTokenFilter) ([]*BlacklistedToken, error)

	// Statistics
	GetBlacklistedTokensCount(ctx context.Context) (int64, error)
	GetBlacklistedTokensByReason(ctx context.Context, reason string) ([]*BlacklistedToken, error)
}

// BlacklistedTokenFilter represents filters for blacklisted token queries
type BlacklistedTokenFilter struct {
	// Domain filters
	UserID *uuid.UUID
	Reason *string

	// Pagination (embedded for DRY)
	pagination.Params
}

// APIKeyRepository defines the interface for API key data access.
type APIKeyRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, apiKey *APIKey) error
	GetByID(ctx context.Context, id uuid.UUID) (*APIKey, error)
	GetByKeyHash(ctx context.Context, keyHash string) (*APIKey, error) // Industry-standard direct hash lookup
	Update(ctx context.Context, apiKey *APIKey) error
	Delete(ctx context.Context, id uuid.UUID) error

	// User and organization scoped
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*APIKey, error)
	GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*APIKey, error)
	GetByProjectID(ctx context.Context, projectID uuid.UUID) ([]*APIKey, error)

	// API key management
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error // For async last used updates
	CleanupExpiredAPIKeys(ctx context.Context) error

	// Filtered queries with pagination
	GetByFilters(ctx context.Context, filters *APIKeyFilters) ([]*APIKey, error)
	CountByFilters(ctx context.Context, filters *APIKeyFilters) (int64, error)

	// Statistics
	GetAPIKeyCount(ctx context.Context, userID uuid.UUID) (int, error)
	GetActiveAPIKeyCount(ctx context.Context, userID uuid.UUID) (int, error)
}

// RoleRepository defines the interface for both system template and custom scoped role data access.
type RoleRepository interface {
	// Core CRUD operations
	Create(ctx context.Context, role *Role) error
	GetByID(ctx context.Context, id uuid.UUID) (*Role, error)
	GetByNameAndScope(ctx context.Context, name, scopeType string) (*Role, error)
	Update(ctx context.Context, role *Role) error
	Delete(ctx context.Context, id uuid.UUID) error

	// System template role queries
	GetByScopeType(ctx context.Context, scopeType string) ([]*Role, error)
	GetAllRoles(ctx context.Context) ([]*Role, error)
	GetSystemRoles(ctx context.Context) ([]*Role, error)

	// Custom scoped role queries
	GetCustomRolesByScopeID(ctx context.Context, scopeType string, scopeID uuid.UUID) ([]*Role, error)
	GetByNameScopeAndID(ctx context.Context, name, scopeType string, scopeID *uuid.UUID) (*Role, error)
	GetCustomRolesByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*Role, error)

	// Permission management for roles
	GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]*Permission, error)
	AssignRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error
	RevokeRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	UpdateRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error

	// Statistics
	GetRoleStatistics(ctx context.Context) (*RoleStatistics, error)

	// Bulk operations
	BulkCreate(ctx context.Context, roles []*Role) error
}

// RolePermissionAssignment represents a bulk role permission assignment
type RolePermissionAssignment struct {
	RoleID       uuid.UUID `json:"role_id"`
	PermissionID uuid.UUID `json:"permission_id"`
}

// RolePermissionRevocation represents a bulk role permission revocation
type RolePermissionRevocation struct {
	RoleID       uuid.UUID `json:"role_id"`
	PermissionID uuid.UUID `json:"permission_id"`
}

// PermissionRepository defines the interface for normalized permission data access.
type PermissionRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, permission *Permission) error
	GetByID(ctx context.Context, id uuid.UUID) (*Permission, error)
	GetByName(ctx context.Context, name string) (*Permission, error)
	GetByResourceAction(ctx context.Context, resource, action string) (*Permission, error)
	Update(ctx context.Context, permission *Permission) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Permission queries
	GetAllPermissions(ctx context.Context) ([]*Permission, error)
	GetByResource(ctx context.Context, resource string) ([]*Permission, error)
	GetByNames(ctx context.Context, names []string) ([]*Permission, error)
	GetByResourceActions(ctx context.Context, resourceActions []string) ([]*Permission, error)
	ListPermissions(ctx context.Context, limit, offset int) ([]*Permission, int, error)
	SearchPermissions(ctx context.Context, query string, limit, offset int) ([]*Permission, int, error)

	// Scope-level queries (NEW)
	GetByScopeLevel(ctx context.Context, scopeLevel ScopeLevel) ([]*Permission, error)
	GetByCategory(ctx context.Context, category string) ([]*Permission, error)
	GetByScopeLevelAndCategory(ctx context.Context, scopeLevel ScopeLevel, category string) ([]*Permission, error)

	// Resource and action queries
	GetAvailableResources(ctx context.Context) ([]string, error)
	GetActionsForResource(ctx context.Context, resource string) ([]string, error)
	GetAvailableCategories(ctx context.Context) ([]string, error)

	// Role permissions
	GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]*Permission, error)

	// Permission validation
	PermissionExists(ctx context.Context, resource, action string) (bool, error)
	BulkPermissionExists(ctx context.Context, resourceActions []string) (map[string]bool, error)

	// Bulk operations
	BulkCreate(ctx context.Context, permissions []*Permission) error
}

// OrganizationMemberRepository defines the interface for organization membership management.
type OrganizationMemberRepository interface {
	// Core CRUD operations
	Create(ctx context.Context, member *OrganizationMember) error
	GetByUserAndOrganization(ctx context.Context, userID, orgID uuid.UUID) (*OrganizationMember, error)
	Update(ctx context.Context, member *OrganizationMember) error
	Delete(ctx context.Context, userID, orgID uuid.UUID) error

	// Membership queries
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*OrganizationMember, error)
	GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*OrganizationMember, error)
	GetByRole(ctx context.Context, roleID uuid.UUID) ([]*OrganizationMember, error)
	Exists(ctx context.Context, userID, orgID uuid.UUID) (bool, error)

	// Permission queries
	GetUserEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
	HasUserPermission(ctx context.Context, userID uuid.UUID, permission string) (bool, error)
	CheckUserPermissions(ctx context.Context, userID uuid.UUID, permissions []string) (map[string]bool, error)
	GetUserPermissionsInOrganization(ctx context.Context, userID, orgID uuid.UUID) ([]string, error)

	// Status management
	ActivateMember(ctx context.Context, userID, orgID uuid.UUID) error
	SuspendMember(ctx context.Context, userID, orgID uuid.UUID) error
	GetActiveMembers(ctx context.Context, orgID uuid.UUID) ([]*OrganizationMember, error)

	// Role management
	UpdateMemberRole(ctx context.Context, userID, orgID, roleID uuid.UUID) error

	// Bulk operations
	BulkCreate(ctx context.Context, members []*OrganizationMember) error
	BulkUpdateRoles(ctx context.Context, updates []MemberRoleUpdate) error

	// Statistics
	GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error)
	GetMembersByRole(ctx context.Context, orgID uuid.UUID) (map[string]int, error)
}

// MemberRoleUpdate represents a bulk role update for a member
type MemberRoleUpdate struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	RoleID         uuid.UUID `json:"role_id"`
}

// RolePermissionRepository defines the interface for role-permission relationships.
type RolePermissionRepository interface {
	// Core operations
	Create(ctx context.Context, rolePermission *RolePermission) error
	Delete(ctx context.Context, roleID, permissionID uuid.UUID) error
	GetByRoleID(ctx context.Context, roleID uuid.UUID) ([]*RolePermission, error)
	HasPermission(ctx context.Context, roleID, permissionID uuid.UUID) (bool, error)

	// Batch operations
	AssignPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error
	RevokePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	ReplaceAllPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error

	// Bulk operations
	BulkAssign(ctx context.Context, assignments []RolePermissionAssignment) error
	BulkRevoke(ctx context.Context, revocations []RolePermissionRevocation) error
}

// AuditLogRepository defines the interface for audit log data access.
type AuditLogRepository interface {
	// Basic operations
	Create(ctx context.Context, auditLog *AuditLog) error
	GetByID(ctx context.Context, id uuid.UUID) (*AuditLog, error)

	// Audit log queries
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*AuditLog, error)
	GetByOrganizationID(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*AuditLog, error)
	GetByResource(ctx context.Context, resource, resourceID string, limit, offset int) ([]*AuditLog, error)
	GetByAction(ctx context.Context, action string, limit, offset int) ([]*AuditLog, error)
	GetByDateRange(ctx context.Context, startDate, endDate time.Time, limit, offset int) ([]*AuditLog, error)

	// Advanced queries
	Search(ctx context.Context, filters *AuditLogFilters) ([]*AuditLog, int, error)

	// Statistics
	GetAuditLogStats(ctx context.Context) (*AuditLogStats, error)
	GetUserAuditLogStats(ctx context.Context, userID uuid.UUID) (*AuditLogStats, error)
	GetOrganizationAuditLogStats(ctx context.Context, orgID uuid.UUID) (*AuditLogStats, error)

	// Cleanup
	CleanupOldLogs(ctx context.Context, olderThan time.Time) error
}

// AuditLogFilters represents filters for audit log queries.
type AuditLogFilters struct {
	// Domain filters
	UserID         *uuid.UUID `json:"user_id,omitempty"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"`
	Action         *string    `json:"action,omitempty"`
	Resource       *string    `json:"resource,omitempty"`
	ResourceID     *string    `json:"resource_id,omitempty"`
	IPAddress      *string    `json:"ip_address,omitempty"`
	StartDate      *time.Time `json:"start_date,omitempty"`
	EndDate        *time.Time `json:"end_date,omitempty"`

	// Pagination (embedded for DRY)
	pagination.Params `json:",inline"`
}

// AuditLogStats represents audit log statistics
type AuditLogStats struct {
	LogsByAction   map[string]int64 `json:"logs_by_action"`
	LogsByResource map[string]int64 `json:"logs_by_resource"`
	LastLogTime    *time.Time       `json:"last_log_time,omitempty"`
	TotalLogs      int64            `json:"total_logs"`
}

// PasswordResetTokenRepository defines the interface for password reset token data access.
type PasswordResetTokenRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, token *PasswordResetToken) error
	GetByID(ctx context.Context, id uuid.UUID) (*PasswordResetToken, error)
	GetByToken(ctx context.Context, token string) (*PasswordResetToken, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*PasswordResetToken, error)
	Update(ctx context.Context, token *PasswordResetToken) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Token management
	MarkAsUsed(ctx context.Context, id uuid.UUID) error
	IsUsed(ctx context.Context, id uuid.UUID) (bool, error)
	IsValid(ctx context.Context, id uuid.UUID) (bool, error)
	GetValidTokenByUserID(ctx context.Context, userID uuid.UUID) (*PasswordResetToken, error)

	// Cleanup operations
	CleanupExpiredTokens(ctx context.Context) error
	CleanupUsedTokens(ctx context.Context, olderThan time.Time) error
	InvalidateAllUserTokens(ctx context.Context, userID uuid.UUID) error
}

// Repository aggregates all auth-related repositories (normalized version).
type Repository interface {
	UserSessions() UserSessionRepository
	BlacklistedTokens() BlacklistedTokenRepository
	APIKeys() APIKeyRepository
	Roles() RoleRepository
	OrganizationMembers() OrganizationMemberRepository
	Permissions() PermissionRepository
	RolePermissions() RolePermissionRepository
	AuditLogs() AuditLogRepository
	PasswordResetTokens() PasswordResetTokenRepository
}
