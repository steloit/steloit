package organization

import (
	"context"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/pagination"
)

// OrganizationRepository defines the interface for organization data access.
type OrganizationRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id uuid.UUID) (*Organization, error)
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filters *OrganizationFilters) ([]*Organization, error)

	// User context
	GetOrganizationsByUserID(ctx context.Context, userID uuid.UUID) ([]*Organization, error)

	// Batch operations for workspace context
	GetUserOrganizationsWithProjectsBatch(ctx context.Context, userID uuid.UUID) ([]*OrganizationWithProjectsAndRole, error)
}

// MemberRepository defines the interface for organization member data access.
type MemberRepository interface {
	// Member management
	Create(ctx context.Context, member *Member) error
	GetByID(ctx context.Context, id uuid.UUID) (*Member, error)
	GetByUserAndOrg(ctx context.Context, userID, orgID uuid.UUID) (*Member, error)
	GetByUserAndOrganization(ctx context.Context, userID, orgID uuid.UUID) (*Member, error) // Alias for GetByUserAndOrg
	Update(ctx context.Context, member *Member) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByUserAndOrg(ctx context.Context, orgID, userID uuid.UUID) error

	// Organization members
	GetMembersByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*Member, error)
	GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*Member, error) // Alias for GetMembersByOrganizationID
	GetMembersByUserID(ctx context.Context, userID uuid.UUID) ([]*Member, error)

	// Role operations
	UpdateMemberRole(ctx context.Context, orgID, userID, roleID uuid.UUID) error
	GetMemberRole(ctx context.Context, userID, orgID uuid.UUID) (uuid.UUID, error)
	CountByOrganizationAndRole(ctx context.Context, orgID, roleID uuid.UUID) (int, error)

	// Membership validation
	IsMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error)
	GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error)
}

// ProjectRepository defines the interface for project data access.
type ProjectRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, id uuid.UUID) (*Project, error)
	GetBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Project, error)
	Update(ctx context.Context, project *Project) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Organization scoped
	GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*Project, error)
	GetProjectCount(ctx context.Context, orgID uuid.UUID) (int, error)

	// Access validation
	CanUserAccessProject(ctx context.Context, userID, projectID uuid.UUID) (bool, error)
}

// InvitationRepository defines the interface for user invitation data access.
type InvitationRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, invitation *Invitation) error
	GetByID(ctx context.Context, id uuid.UUID) (*Invitation, error)
	GetByToken(ctx context.Context, token string) (*Invitation, error)         // Deprecated: use GetByTokenHash
	GetByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error) // Secure token lookup
	Update(ctx context.Context, invitation *Invitation) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Organization invitations
	GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*Invitation, error)
	GetByEmail(ctx context.Context, email string) ([]*Invitation, error)
	GetPendingByEmail(ctx context.Context, orgID uuid.UUID, email string) (*Invitation, error)
	GetPendingInvitations(ctx context.Context, orgID uuid.UUID) ([]*Invitation, error)

	// Invitation management
	MarkAccepted(ctx context.Context, id uuid.UUID, acceptedByID uuid.UUID) error
	MarkExpired(ctx context.Context, id uuid.UUID) error
	RevokeInvitation(ctx context.Context, id uuid.UUID, revokedByID uuid.UUID) error
	// MarkResent atomically increments resent_count if within limits.
	// Returns ErrResendLimitReached or ErrResendCooldown if constraints not met.
	MarkResent(ctx context.Context, id uuid.UUID, newExpiresAt time.Time, maxAttempts int, cooldown time.Duration) error
	CleanupExpiredInvitations(ctx context.Context) error

	// UpdateTokenHash updates only the token hash and preview fields.
	// Use this instead of Update when you need to preserve other field changes.
	UpdateTokenHash(ctx context.Context, id uuid.UUID, tokenHash, tokenPreview string) error

	// Validation
	IsEmailAlreadyInvited(ctx context.Context, email string, orgID uuid.UUID) (bool, error)

	// Audit logging
	CreateAuditEvent(ctx context.Context, event *InvitationAuditEvent) error
	GetAuditEventsByInvitationID(ctx context.Context, invitationID uuid.UUID) ([]*InvitationAuditEvent, error)
}

// OrganizationFilters represents filters for organization queries.
type OrganizationFilters struct {
	// Domain filters
	Name   *string
	Plan   *string
	Status *string

	// Pagination (embedded for DRY)
	pagination.Params
}

// MemberFilters represents filters for member queries.
type MemberFilters struct {
	// Domain filters
	OrganizationID *uuid.UUID
	UserID         *uuid.UUID
	RoleID         *uuid.UUID

	// Pagination (embedded for DRY)
	pagination.Params
}

// ProjectFilters represents filters for project queries.
type ProjectFilters struct {
	// Domain filters
	OrganizationID *uuid.UUID
	Name           *string

	// Pagination (embedded for DRY)
	pagination.Params
}

// InvitationFilters represents filters for invitation queries.
type InvitationFilters struct {
	// Domain filters
	OrganizationID *uuid.UUID
	Status         *string
	Email          *string

	// Pagination (embedded for DRY)
	pagination.Params
}

// OrganizationSettingsRepository defines the interface for organization settings data access.
type OrganizationSettingsRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, setting *OrganizationSettings) error
	GetByID(ctx context.Context, id uuid.UUID) (*OrganizationSettings, error)
	GetByKey(ctx context.Context, orgID uuid.UUID, key string) (*OrganizationSettings, error)
	Update(ctx context.Context, setting *OrganizationSettings) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Organization scoped operations
	GetAllByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*OrganizationSettings, error)
	GetSettingsMap(ctx context.Context, orgID uuid.UUID) (map[string]interface{}, error)
	DeleteByKey(ctx context.Context, orgID uuid.UUID, key string) error
	UpsertSetting(ctx context.Context, orgID uuid.UUID, key string, value interface{}) (*OrganizationSettings, error)

	// Bulk operations
	CreateMultiple(ctx context.Context, settings []*OrganizationSettings) error
	GetByKeys(ctx context.Context, orgID uuid.UUID, keys []string) ([]*OrganizationSettings, error)
	DeleteMultiple(ctx context.Context, orgID uuid.UUID, keys []string) error
}

// Repository aggregates all organization-related repositories.
type Repository interface {
	Organizations() OrganizationRepository
	Members() MemberRepository
	Projects() ProjectRepository
	Invitations() InvitationRepository
	Settings() OrganizationSettingsRepository
}
