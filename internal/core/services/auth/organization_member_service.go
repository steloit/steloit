package auth

import (
	"context"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	appErrors "brokle/pkg/errors"
)

// organizationMemberService implements the auth.OrganizationMemberService interface
type organizationMemberService struct {
	orgMemberRepo authDomain.OrganizationMemberRepository
	roleRepo      authDomain.RoleRepository
}

// NewOrganizationMemberService creates a new organization member service instance
func NewOrganizationMemberService(
	orgMemberRepo authDomain.OrganizationMemberRepository,
	roleRepo authDomain.RoleRepository,
) authDomain.OrganizationMemberService {
	return &organizationMemberService{
		orgMemberRepo: orgMemberRepo,
		roleRepo:      roleRepo,
	}
}

// AddMember adds a user to an organization with specified role
func (s *organizationMemberService) AddMember(ctx context.Context, userID, orgID, roleID uuid.UUID, invitedBy *uuid.UUID) (*authDomain.OrganizationMember, error) {
	// Verify role exists
	_, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Role not found")
	}

	// Check if user is already a member
	exists, err := s.orgMemberRepo.Exists(ctx, userID, orgID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to check membership", err)
	}
	if exists {
		return nil, appErrors.NewConflictError("User is already a member of this organization")
	}

	// Create new membership
	member := authDomain.NewOrganizationMember(userID, orgID, roleID, invitedBy)

	err = s.orgMemberRepo.Create(ctx, member)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create membership", err)
	}

	return member, nil
}

// RemoveMember removes a user from an organization
func (s *organizationMemberService) RemoveMember(ctx context.Context, userID, orgID uuid.UUID) error {
	// Check if user is a member
	exists, err := s.orgMemberRepo.Exists(ctx, userID, orgID)
	if err != nil {
		return appErrors.NewInternalError("Failed to check membership", err)
	}
	if !exists {
		return appErrors.NewNotFoundError("User is not a member of this organization")
	}

	return s.orgMemberRepo.Delete(ctx, userID, orgID)
}

// UpdateMemberRole updates a member's role in an organization
func (s *organizationMemberService) UpdateMemberRole(ctx context.Context, userID, orgID, roleID uuid.UUID) error {
	// Verify role exists
	_, err := s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return appErrors.NewNotFoundError("Role not found")
	}

	// Check if user is a member
	exists, err := s.orgMemberRepo.Exists(ctx, userID, orgID)
	if err != nil {
		return appErrors.NewInternalError("Failed to check membership", err)
	}
	if !exists {
		return appErrors.NewNotFoundError("User is not a member of this organization")
	}

	return s.orgMemberRepo.UpdateMemberRole(ctx, userID, orgID, roleID)
}

// GetMember gets a specific organization membership
func (s *organizationMemberService) GetMember(ctx context.Context, userID, orgID uuid.UUID) (*authDomain.OrganizationMember, error) {
	return s.orgMemberRepo.GetByUserAndOrganization(ctx, userID, orgID)
}

// GetUserMemberships gets all organization memberships for a user
func (s *organizationMemberService) GetUserMemberships(ctx context.Context, userID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	return s.orgMemberRepo.GetByUserID(ctx, userID)
}

// GetOrganizationMembers gets all members of an organization
func (s *organizationMemberService) GetOrganizationMembers(ctx context.Context, orgID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	return s.orgMemberRepo.GetByOrganizationID(ctx, orgID)
}

// GetMembersByRole gets all members with a specific role
func (s *organizationMemberService) GetMembersByRole(ctx context.Context, roleID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	return s.orgMemberRepo.GetByRole(ctx, roleID)
}

// IsMember checks if a user is a member of an organization
func (s *organizationMemberService) IsMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	return s.orgMemberRepo.Exists(ctx, userID, orgID)
}

// GetUserEffectivePermissions gets all effective permissions for a user across all organizations
func (s *organizationMemberService) GetUserEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return s.orgMemberRepo.GetUserEffectivePermissions(ctx, userID)
}

// GetUserPermissionsInOrganization gets user permissions within a specific organization
func (s *organizationMemberService) GetUserPermissionsInOrganization(ctx context.Context, userID, orgID uuid.UUID) ([]string, error) {
	return s.orgMemberRepo.GetUserPermissionsInOrganization(ctx, userID, orgID)
}

// CheckUserPermission checks if a user has a specific permission
func (s *organizationMemberService) CheckUserPermission(ctx context.Context, userID uuid.UUID, permission string) (bool, error) {
	return s.orgMemberRepo.HasUserPermission(ctx, userID, permission)
}

// CheckUserPermissions checks multiple permissions for a user
func (s *organizationMemberService) CheckUserPermissions(ctx context.Context, userID uuid.UUID, permissions []string) (map[string]bool, error) {
	return s.orgMemberRepo.CheckUserPermissions(ctx, userID, permissions)
}

// ActivateMember activates a member in an organization
func (s *organizationMemberService) ActivateMember(ctx context.Context, userID, orgID uuid.UUID) error {
	return s.orgMemberRepo.ActivateMember(ctx, userID, orgID)
}

// SuspendMember suspends a member in an organization
func (s *organizationMemberService) SuspendMember(ctx context.Context, userID, orgID uuid.UUID) error {
	return s.orgMemberRepo.SuspendMember(ctx, userID, orgID)
}

// GetActiveMembers gets all active members of an organization
func (s *organizationMemberService) GetActiveMembers(ctx context.Context, orgID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	return s.orgMemberRepo.GetActiveMembers(ctx, orgID)
}

// GetMemberCount gets the count of members in an organization
func (s *organizationMemberService) GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	return s.orgMemberRepo.GetMemberCount(ctx, orgID)
}

// GetMembersByRoleCount gets member counts by role in an organization
func (s *organizationMemberService) GetMembersByRoleCount(ctx context.Context, orgID uuid.UUID) (map[string]int, error) {
	return s.orgMemberRepo.GetMembersByRole(ctx, orgID)
}
