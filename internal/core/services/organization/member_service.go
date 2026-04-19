package organization

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	orgDomain "brokle/internal/core/domain/organization"
	userDomain "brokle/internal/core/domain/user"
	appErrors "brokle/pkg/errors"
)

// memberService implements the orgDomain.MemberService interface
type memberService struct {
	memberRepo  orgDomain.MemberRepository
	orgRepo     orgDomain.OrganizationRepository
	userRepo    userDomain.Repository
	roleService authDomain.RoleService
}

// NewMemberService creates a new member service instance
func NewMemberService(
	memberRepo orgDomain.MemberRepository,
	orgRepo orgDomain.OrganizationRepository,
	userRepo userDomain.Repository,
	roleService authDomain.RoleService,
) orgDomain.MemberService {
	return &memberService{
		memberRepo:  memberRepo,
		orgRepo:     orgRepo,
		userRepo:    userRepo,
		roleService: roleService,
	}
}

// AddMember adds a user to an organization with specified role
func (s *memberService) AddMember(ctx context.Context, orgID, userID, roleID uuid.UUID, addedByID uuid.UUID) error {
	// Verify organization exists
	_, err := s.orgRepo.GetByID(ctx, orgID)
	if err != nil {
		return appErrors.NewNotFoundError("Organization not found")
	}

	// Verify user exists
	_, err = s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return appErrors.NewNotFoundError("User not found")
	}

	// Verify role exists
	_, err = s.roleService.GetRoleByID(ctx, roleID)
	if err != nil {
		return appErrors.NewNotFoundError("Role not found")
	}

	// Check if user is already a member
	isMember, err := s.memberRepo.IsMember(ctx, userID, orgID)
	if err != nil {
		return appErrors.NewInternalError("Failed to check membership", err)
	}
	if isMember {
		return appErrors.NewConflictError("User is already a member of this organization")
	}

	// Create member
	member := orgDomain.NewMember(orgID, userID, roleID)
	err = s.memberRepo.Create(ctx, member)
	if err != nil {
		return appErrors.NewInternalError("Failed to add member", err)
	}

	return nil
}

// RemoveMember removes a user from an organization
func (s *memberService) RemoveMember(ctx context.Context, orgID, userID uuid.UUID, removedByID uuid.UUID) error {
	// Verify membership exists
	member, err := s.memberRepo.GetByUserAndOrganization(ctx, userID, orgID)
	if err != nil {
		return appErrors.NewNotFoundError("Member not found")
	}

	// Check if this is the only owner
	ownerRole, err := s.roleService.GetRoleByNameAndScope(ctx, "owner", authDomain.ScopeOrganization)
	if err != nil {
		return appErrors.NewInternalError("Failed to get owner role", err)
	}

	if member.RoleID == ownerRole.ID {
		ownerCount, err := s.memberRepo.CountByOrganizationAndRole(ctx, orgID, ownerRole.ID)
		if err != nil {
			return appErrors.NewInternalError("Failed to count owners", err)
		}
		if ownerCount <= 1 {
			return appErrors.NewForbiddenError("Cannot remove the last owner of the organization")
		}
	}

	// Remove member (Member has composite primary key: OrganizationID + UserID)
	err = s.memberRepo.DeleteByUserAndOrg(ctx, member.OrganizationID, member.UserID)
	if err != nil {
		return appErrors.NewInternalError("Failed to remove member", err)
	}

	// If this was their default organization, clear it
	user, err := s.userRepo.GetByID(ctx, userID)
	if err == nil && user.DefaultOrganizationID != nil && *user.DefaultOrganizationID == orgID {
		err = s.userRepo.SetDefaultOrganization(ctx, userID, uuid.UUID{})
		if err != nil {
			slog.Error("failed to clear default organization",
				"user_id", userID,
				"organization_id", orgID,
				"error", err)
		}
	}

	return nil
}

// UpdateMemberRole updates a member's role in an organization
func (s *memberService) UpdateMemberRole(ctx context.Context, orgID, userID, newRoleID uuid.UUID, updatedByID uuid.UUID) error {
	// Verify membership exists
	member, err := s.memberRepo.GetByUserAndOrganization(ctx, userID, orgID)
	if err != nil {
		return appErrors.NewNotFoundError("Member not found")
	}

	// Verify new role exists
	_, err = s.roleService.GetRoleByID(ctx, newRoleID)
	if err != nil {
		return appErrors.NewNotFoundError("Role not found")
	}

	// Check if demoting the last owner
	ownerRole, err := s.roleService.GetRoleByNameAndScope(ctx, "owner", authDomain.ScopeOrganization)
	if err != nil {
		return appErrors.NewInternalError("Failed to get owner role", err)
	}

	if member.RoleID == ownerRole.ID && newRoleID != ownerRole.ID {
		ownerCount, err := s.memberRepo.CountByOrganizationAndRole(ctx, orgID, ownerRole.ID)
		if err != nil {
			return appErrors.NewInternalError("Failed to count owners", err)
		}
		if ownerCount <= 1 {
			return appErrors.NewForbiddenError("Cannot demote the last owner of the organization")
		}
	}

	// Update role
	member.RoleID = newRoleID
	member.UpdatedAt = time.Now()
	err = s.memberRepo.Update(ctx, member)
	if err != nil {
		return appErrors.NewInternalError("Failed to update member role", err)
	}

	return nil
}

// GetMember retrieves a specific member
func (s *memberService) GetMember(ctx context.Context, orgID, userID uuid.UUID) (*orgDomain.Member, error) {
	return s.memberRepo.GetByUserAndOrganization(ctx, userID, orgID)
}

// GetMembers retrieves all members of an organization
func (s *memberService) GetMembers(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Member, error) {
	return s.memberRepo.GetByOrganizationID(ctx, orgID)
}

// IsMember checks if a user is a member of an organization
func (s *memberService) IsMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	return s.memberRepo.IsMember(ctx, userID, orgID)
}

// CanUserAccessOrganization checks if user can access organization
func (s *memberService) CanUserAccessOrganization(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	return s.memberRepo.IsMember(ctx, userID, orgID)
}

// GetUserRole returns a user's role ID in an organization
func (s *memberService) GetUserRole(ctx context.Context, userID, orgID uuid.UUID) (uuid.UUID, error) {
	member, err := s.memberRepo.GetByUserAndOrganization(ctx, userID, orgID)
	if err != nil {
		return uuid.UUID{}, appErrors.NewNotFoundError("Member not found")
	}

	return member.RoleID, nil
}

// GetMemberCount returns the number of members in an organization
func (s *memberService) GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	members, err := s.memberRepo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return 0, appErrors.NewInternalError("Failed to get members", err)
	}
	return len(members), nil
}

// GetMembersByRole returns all members with a specific role
func (s *memberService) GetMembersByRole(ctx context.Context, orgID, roleID uuid.UUID) ([]*orgDomain.Member, error) {
	allMembers, err := s.memberRepo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to get members", err)
	}

	var membersWithRole []*orgDomain.Member
	for _, member := range allMembers {
		if member.RoleID == roleID {
			membersWithRole = append(membersWithRole, member)
		}
	}

	return membersWithRole, nil
}
