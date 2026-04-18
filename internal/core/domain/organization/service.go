package organization

import (
	"context"

	"github.com/google/uuid"
)

// OrganizationService defines the organization management service interface.
type OrganizationService interface {
	// Organization CRUD operations
	CreateOrganization(ctx context.Context, userID uuid.UUID, req *CreateOrganizationRequest) (*Organization, error)
	GetOrganization(ctx context.Context, orgID uuid.UUID) (*Organization, error)
	GetOrganizationBySlug(ctx context.Context, slug string) (*Organization, error)
	UpdateOrganization(ctx context.Context, orgID uuid.UUID, req *UpdateOrganizationRequest) error
	DeleteOrganization(ctx context.Context, orgID uuid.UUID) error
	ListOrganizations(ctx context.Context, filters *OrganizationFilters) ([]*Organization, error)

	// User organization context
	GetUserOrganizations(ctx context.Context, userID uuid.UUID) ([]*Organization, error)
	GetUserDefaultOrganization(ctx context.Context, userID uuid.UUID) (*Organization, error)
	SetUserDefaultOrganization(ctx context.Context, userID, orgID uuid.UUID) error

	// Workspace hierarchy
	GetUserOrganizationsWithProjects(ctx context.Context, userID uuid.UUID) ([]*OrganizationWithProjectsAndRole, error)
}

// MemberService defines the organization member management service interface.
type MemberService interface {
	// Member management
	AddMember(ctx context.Context, orgID, userID, roleID uuid.UUID, addedByID uuid.UUID) error
	RemoveMember(ctx context.Context, orgID, userID uuid.UUID, removedByID uuid.UUID) error
	UpdateMemberRole(ctx context.Context, orgID, userID, roleID uuid.UUID, updatedByID uuid.UUID) error
	GetMember(ctx context.Context, orgID, userID uuid.UUID) (*Member, error)
	GetMembers(ctx context.Context, orgID uuid.UUID) ([]*Member, error)

	// Member validation
	IsMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error)
	CanUserAccessOrganization(ctx context.Context, userID, orgID uuid.UUID) (bool, error)
	GetUserRole(ctx context.Context, userID, orgID uuid.UUID) (uuid.UUID, error)

	// Member statistics
	GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error)
	GetMembersByRole(ctx context.Context, orgID, roleID uuid.UUID) ([]*Member, error)
}

// ProjectService defines the project management service interface.
type ProjectService interface {
	// Project CRUD operations
	CreateProject(ctx context.Context, orgID uuid.UUID, req *CreateProjectRequest) (*Project, error)
	GetProject(ctx context.Context, projectID uuid.UUID) (*Project, error)
	GetProjectBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Project, error)
	UpdateProject(ctx context.Context, projectID uuid.UUID, req *UpdateProjectRequest) error
	DeleteProject(ctx context.Context, projectID uuid.UUID) error

	// Archive lifecycle
	ArchiveProject(ctx context.Context, projectID uuid.UUID) error
	UnarchiveProject(ctx context.Context, projectID uuid.UUID) error

	// Organization projects
	GetProjectsByOrganization(ctx context.Context, orgID uuid.UUID) ([]*Project, error)
	GetProjectCount(ctx context.Context, orgID uuid.UUID) (int, error)

	// Access validation
	CanUserAccessProject(ctx context.Context, userID, projectID uuid.UUID) (bool, error)
	ValidateProjectAccess(ctx context.Context, userID, projectID uuid.UUID) error
}

// AcceptInvitationResult contains details returned after accepting an invitation.
// This eliminates the need for an extra DB query to get org details after accept.
type AcceptInvitationResult struct {
	OrganizationID   uuid.UUID
	OrganizationName string
	RoleName         string
}

// InvitationService defines the user invitation service interface.
type InvitationService interface {
	// Invitation management
	InviteUser(ctx context.Context, orgID uuid.UUID, inviterID uuid.UUID, req *InviteUserRequest) (*Invitation, error)
	AcceptInvitation(ctx context.Context, token string, userID uuid.UUID) (*AcceptInvitationResult, error)
	DeclineInvitation(ctx context.Context, token string) error
	RevokeInvitation(ctx context.Context, invitationID uuid.UUID, revokedByID uuid.UUID) error
	ResendInvitation(ctx context.Context, invitationID uuid.UUID, resentByID uuid.UUID) (*Invitation, error)

	// Invitation queries
	GetInvitation(ctx context.Context, invitationID uuid.UUID) (*Invitation, error)
	GetInvitationByToken(ctx context.Context, token string) (*Invitation, error)
	GetPendingInvitations(ctx context.Context, orgID uuid.UUID) ([]*Invitation, error)
	GetUserInvitations(ctx context.Context, email string) ([]*Invitation, error)

	// Invitation validation
	ValidateInvitationToken(ctx context.Context, token string) (*Invitation, error)
	IsEmailAlreadyInvited(ctx context.Context, email string, orgID uuid.UUID) (bool, error)

	// Cleanup
	CleanupExpiredInvitations(ctx context.Context) error
}

// OrganizationSettingsService defines the organization settings management service interface.
type OrganizationSettingsService interface {
	// Settings CRUD operations
	CreateSetting(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, req *CreateOrganizationSettingRequest) (*OrganizationSettings, error)
	GetSetting(ctx context.Context, orgID uuid.UUID, key string) (*OrganizationSettings, error)
	GetAllSettings(ctx context.Context, orgID uuid.UUID) (map[string]any, error)
	UpdateSetting(ctx context.Context, orgID uuid.UUID, key string, userID uuid.UUID, req *UpdateOrganizationSettingRequest) (*OrganizationSettings, error)
	DeleteSetting(ctx context.Context, orgID uuid.UUID, key string, userID uuid.UUID) error

	// Bulk operations
	UpsertSetting(ctx context.Context, orgID uuid.UUID, key string, value any, userID uuid.UUID) (*OrganizationSettings, error)
	CreateMultipleSettings(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, settings map[string]any) error
	GetSettingsByKeys(ctx context.Context, orgID uuid.UUID, keys []string) (map[string]any, error)
	DeleteMultipleSettings(ctx context.Context, orgID uuid.UUID, keys []string, userID uuid.UUID) error

	// Access validation
	ValidateSettingsAccess(ctx context.Context, userID, orgID uuid.UUID, operation string) error
	CanUserManageSettings(ctx context.Context, userID, orgID uuid.UUID) (bool, error)

	// Settings management
	ResetToDefaults(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) error
	ExportSettings(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) (map[string]any, error)
	ImportSettings(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, settings map[string]any) error
}
