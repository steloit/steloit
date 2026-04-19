package organization

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	orgDomain "brokle/internal/core/domain/organization"
	userDomain "brokle/internal/core/domain/user"
	orgRepo "brokle/internal/infrastructure/repository/organization"
	"brokle/pkg/email"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/token"
)

const (
	// InvitationExpiryDays is the default expiration for invitations
	InvitationExpiryDays = 7
	// MaxResendAttempts is the maximum number of resends allowed
	MaxResendAttempts = 5
	// ResendCooldownHours is the minimum hours between resends
	ResendCooldownHours = 1
)

// invitationService implements the orgDomain.InvitationService interface
type invitationService struct {
	inviteRepo  orgDomain.InvitationRepository
	orgRepo     orgDomain.OrganizationRepository
	memberRepo  orgDomain.MemberRepository
	userRepo    userDomain.Repository
	roleService authDomain.RoleService
	emailSender email.EmailSender
	appURL      string // Base URL for accept links
	logger      *slog.Logger
}

// InvitationServiceConfig contains configuration for the invitation service
type InvitationServiceConfig struct {
	AppURL string // Base application URL for generating accept links
}

// NewInvitationService creates a new invitation service instance
func NewInvitationService(
	inviteRepo orgDomain.InvitationRepository,
	orgRepo orgDomain.OrganizationRepository,
	memberRepo orgDomain.MemberRepository,
	userRepo userDomain.Repository,
	roleService authDomain.RoleService,
	emailSender email.EmailSender,
	cfg InvitationServiceConfig,
	logger *slog.Logger,
) orgDomain.InvitationService {
	return &invitationService{
		inviteRepo:  inviteRepo,
		orgRepo:     orgRepo,
		memberRepo:  memberRepo,
		userRepo:    userRepo,
		roleService: roleService,
		emailSender: emailSender,
		appURL:      cfg.AppURL,
		logger:      logger,
	}
}

// InviteUser creates an invitation for a user to join an organization
func (s *invitationService) InviteUser(ctx context.Context, orgID uuid.UUID, inviterID uuid.UUID, req *orgDomain.InviteUserRequest) (*orgDomain.Invitation, error) {
	// Normalize email
	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))

	// Verify organization exists
	org, err := s.orgRepo.GetByID(ctx, orgID)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Organization not found")
	}

	// Verify role exists
	targetRole, err := s.roleService.GetRoleByID(ctx, req.RoleID)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Role not found")
	}

	// Verify inviter is a member of the organization
	inviterRoleID, err := s.memberRepo.GetMemberRole(ctx, inviterID, orgID)
	if err != nil {
		return nil, appErrors.NewForbiddenError("You must be a member to invite others")
	}

	// Only owners can assign the owner role
	if targetRole.Name == "owner" {
		inviterRoleInfo, err := s.roleService.GetRoleByID(ctx, inviterRoleID)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to get inviter role info", err)
		}
		if inviterRoleInfo.Name != "owner" {
			return nil, appErrors.NewForbiddenError("Only owners can assign the owner role")
		}
	}

	// Check if email is already a member
	user, _ := s.userRepo.GetByEmail(ctx, normalizedEmail)
	if user != nil {
		isMember, _ := s.memberRepo.IsMember(ctx, user.ID, orgID)
		if isMember {
			return nil, appErrors.NewConflictError("User is already a member of this organization")
		}
	}

	// Check for existing pending invitation
	existing, _ := s.inviteRepo.GetPendingByEmail(ctx, orgID, normalizedEmail)
	if existing != nil {
		return nil, appErrors.NewConflictError("A pending invitation already exists for this email")
	}

	// Generate secure token
	tokenData, err := token.GenerateInviteToken()
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate invitation token", err)
	}

	// Create invitation with hash (never store plaintext)
	expiresAt := time.Now().Add(InvitationExpiryDays * 24 * time.Hour)
	invitation := orgDomain.NewInvitationWithMessage(
		orgID,
		req.RoleID,
		inviterID,
		normalizedEmail,
		tokenData.Hash,
		tokenData.Preview,
		req.Message,
		expiresAt,
	)

	err = s.inviteRepo.Create(ctx, invitation)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create invitation", err)
	}

	// Create audit event
	s.createAuditEvent(ctx, invitation.ID, orgDomain.AuditEventCreated, &inviterID, nil)

	// Send invitation email (async - don't fail if email fails)
	// Use detached context with timeout since request context will be canceled when handler returns
	go func() {
		emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.sendInvitationEmail(emailCtx, invitation, org, tokenData.Token, &inviterID)
	}()

	s.logger.Info("invitation created",
		"invitation_id", invitation.ID,
		"organization_id", orgID,
		"inviter_id", inviterID,
		"invitee_email", normalizedEmail,
		"role_id", req.RoleID,
	)

	return invitation, nil
}

// AcceptInvitation accepts an invitation and adds the user to the organization.
// Returns AcceptInvitationResult with org details to avoid extra DB query in handler.
func (s *invitationService) AcceptInvitation(ctx context.Context, tokenStr string, userID uuid.UUID) (*orgDomain.AcceptInvitationResult, error) {
	// Validate token format
	if !token.ValidateTokenFormat(tokenStr) {
		return nil, appErrors.NewValidationError("token", "Invalid invitation token format")
	}

	// Hash the token and lookup
	tokenHash := token.HashToken(tokenStr)
	invitation, err := s.inviteRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Invitation not found or invalid")
	}

	if invitation.Status != orgDomain.InvitationStatusPending {
		return nil, appErrors.NewValidationError("status", "Invitation is no longer pending")
	}

	if invitation.ExpiresAt.Before(time.Now()) {
		// Mark as expired
		s.inviteRepo.MarkExpired(ctx, invitation.ID)
		s.createAuditEvent(ctx, invitation.ID, orgDomain.AuditEventExpired, nil, nil)
		return nil, appErrors.NewValidationError("expiry", "Invitation has expired")
	}

	// Verify email matches (for security)
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, appErrors.NewNotFoundError("User not found")
	}

	if strings.ToLower(user.Email) != strings.ToLower(invitation.Email) {
		return nil, appErrors.NewForbiddenError("Invitation was sent to a different email address")
	}

	// Check if user is already a member
	isMember, err := s.memberRepo.IsMember(ctx, userID, invitation.OrganizationID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to check membership", err)
	}
	if isMember {
		return nil, appErrors.NewConflictError("You are already a member of this organization")
	}

	// Get organization details (we already need this for the result)
	org, err := s.orgRepo.GetByID(ctx, invitation.OrganizationID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to get organization", err)
	}

	// Get role name
	role, err := s.roleService.GetRoleByID(ctx, invitation.RoleID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to get role", err)
	}

	// Add user as member
	member := orgDomain.NewMember(invitation.OrganizationID, userID, invitation.RoleID)
	member.InvitedBy = invitation.InvitedByID
	err = s.memberRepo.Create(ctx, member)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to add member", err)
	}

	// Mark invitation as accepted
	err = s.inviteRepo.MarkAccepted(ctx, invitation.ID, userID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to update invitation", err)
	}

	// Create audit event
	s.createAuditEvent(ctx, invitation.ID, orgDomain.AuditEventAccepted, &userID, nil)

	// Set as default organization if user doesn't have one
	if user.DefaultOrganizationID == nil {
		s.userRepo.SetDefaultOrganization(ctx, userID, invitation.OrganizationID)
	}

	s.logger.Info("invitation accepted",
		"invitation_id", invitation.ID,
		"organization_id", invitation.OrganizationID,
		"user_id", userID,
	)

	return &orgDomain.AcceptInvitationResult{
		OrganizationID:   org.ID,
		OrganizationName: org.Name,
		RoleName:         role.Name,
	}, nil
}

// DeclineInvitation declines an invitation
func (s *invitationService) DeclineInvitation(ctx context.Context, tokenStr string) error {
	// Validate and hash token
	if !token.ValidateTokenFormat(tokenStr) {
		return appErrors.NewValidationError("token", "Invalid invitation token format")
	}

	tokenHash := token.HashToken(tokenStr)
	invitation, err := s.inviteRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return appErrors.NewNotFoundError("Invitation not found")
	}

	if invitation.Status != orgDomain.InvitationStatusPending {
		return appErrors.NewValidationError("status", "Invitation is not pending")
	}

	// Mark invitation as declined (using revoked status)
	invitation.Status = orgDomain.InvitationStatusRevoked
	invitation.UpdatedAt = time.Now()
	err = s.inviteRepo.Update(ctx, invitation)
	if err != nil {
		return appErrors.NewInternalError("Failed to update invitation", err)
	}

	// Create audit event
	s.createAuditEvent(ctx, invitation.ID, orgDomain.AuditEventDeclined, nil, nil)

	return nil
}

// RevokeInvitation revokes a pending invitation
func (s *invitationService) RevokeInvitation(ctx context.Context, invitationID uuid.UUID, revokedByID uuid.UUID) error {
	// Get invitation
	invitation, err := s.inviteRepo.GetByID(ctx, invitationID)
	if err != nil {
		return appErrors.NewNotFoundError("Invitation not found")
	}

	if invitation.Status != orgDomain.InvitationStatusPending {
		return appErrors.NewValidationError("status", "Invitation is not pending")
	}

	// Verify revoker has permission (is member of the org)
	isMember, err := s.memberRepo.IsMember(ctx, revokedByID, invitation.OrganizationID)
	if err != nil || !isMember {
		return appErrors.NewForbiddenError("You are not authorized to revoke this invitation")
	}

	// Revoke the invitation
	err = s.inviteRepo.RevokeInvitation(ctx, invitationID, revokedByID)
	if err != nil {
		return appErrors.NewInternalError("Failed to revoke invitation", err)
	}

	// Create audit event
	s.createAuditEvent(ctx, invitationID, orgDomain.AuditEventRevoked, &revokedByID, nil)

	s.logger.Info("invitation revoked",
		"invitation_id", invitationID,
		"revoked_by_id", revokedByID,
	)

	return nil
}

// ResendInvitation resends a pending invitation and returns the updated invitation
func (s *invitationService) ResendInvitation(ctx context.Context, invitationID uuid.UUID, resentByID uuid.UUID) (*orgDomain.Invitation, error) {
	// Get invitation
	invitation, err := s.inviteRepo.GetByID(ctx, invitationID)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Invitation not found")
	}

	if invitation.Status != orgDomain.InvitationStatusPending {
		return nil, appErrors.NewValidationError("status", "Invitation is not pending")
	}

	// Verify resender has permission (is member of the org)
	isMember, err := s.memberRepo.IsMember(ctx, resentByID, invitation.OrganizationID)
	if err != nil || !isMember {
		return nil, appErrors.NewForbiddenError("You are not authorized to resend this invitation")
	}

	// Get organization for email
	org, err := s.orgRepo.GetByID(ctx, invitation.OrganizationID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to get organization", err)
	}

	// Atomically check limits and mark as resent FIRST (race-condition safe)
	// Must happen before token rotation to avoid breaking invitation if limits exceeded
	newExpiresAt := time.Now().Add(InvitationExpiryDays * 24 * time.Hour)
	err = s.inviteRepo.MarkResent(ctx, invitationID, newExpiresAt, MaxResendAttempts, ResendCooldownHours*time.Hour)
	if err != nil {
		if errors.Is(err, orgRepo.ErrResendLimitReached) {
			return nil, appErrors.NewValidationError("resend", "Maximum resend attempts reached (5 max)")
		}
		if errors.Is(err, orgRepo.ErrResendCooldown) {
			return nil, appErrors.NewValidationError("resend", "Please wait 1 hour before resending")
		}
		return nil, appErrors.NewInternalError("Failed to mark invitation as resent", err)
	}

	// Generate a new token only AFTER limits check passes
	tokenData, err := token.GenerateInviteToken()
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate new token", err)
	}

	// Update token hash
	err = s.inviteRepo.UpdateTokenHash(ctx, invitationID, tokenData.Hash, tokenData.Preview)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to update invitation token", err)
	}

	// Create audit event
	s.createAuditEvent(ctx, invitationID, orgDomain.AuditEventResent, &resentByID, nil)

	// Fetch fresh invitation data after all mutations
	updatedInvitation, err := s.inviteRepo.GetByID(ctx, invitationID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to get updated invitation", err)
	}

	// Send invitation email with new token using updated invitation data
	// Use detached context with timeout since request context will be canceled when handler returns
	go func() {
		emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.sendInvitationEmail(emailCtx, updatedInvitation, org, tokenData.Token, updatedInvitation.InvitedByID)
	}()

	s.logger.Info("invitation resent",
		"invitation_id", invitationID,
		"resent_by_id", resentByID,
	)

	return updatedInvitation, nil
}

// GetInvitation retrieves an invitation by ID
func (s *invitationService) GetInvitation(ctx context.Context, invitationID uuid.UUID) (*orgDomain.Invitation, error) {
	return s.inviteRepo.GetByID(ctx, invitationID)
}

// GetInvitationByToken retrieves an invitation by token (for validation)
func (s *invitationService) GetInvitationByToken(ctx context.Context, tokenStr string) (*orgDomain.Invitation, error) {
	if !token.ValidateTokenFormat(tokenStr) {
		return nil, appErrors.NewValidationError("token", "Invalid invitation token format")
	}

	tokenHash := token.HashToken(tokenStr)
	return s.inviteRepo.GetByTokenHash(ctx, tokenHash)
}

// GetPendingInvitations retrieves all pending invitations for an organization
func (s *invitationService) GetPendingInvitations(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Invitation, error) {
	return s.inviteRepo.GetPendingInvitations(ctx, orgID)
}

// GetUserInvitations retrieves all invitations for a user by email
func (s *invitationService) GetUserInvitations(ctx context.Context, email string) ([]*orgDomain.Invitation, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	return s.inviteRepo.GetByEmail(ctx, normalizedEmail)
}

// ValidateInvitationToken validates an invitation token and returns the invitation
func (s *invitationService) ValidateInvitationToken(ctx context.Context, tokenStr string) (*orgDomain.Invitation, error) {
	if !token.ValidateTokenFormat(tokenStr) {
		return nil, appErrors.NewValidationError("token", "Invalid invitation token format")
	}

	tokenHash := token.HashToken(tokenStr)
	invitation, err := s.inviteRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, appErrors.NewNotFoundError("Invitation not found or invalid")
	}

	if invitation.Status != orgDomain.InvitationStatusPending {
		return nil, appErrors.NewValidationError("status", "Invitation is no longer pending")
	}

	if invitation.ExpiresAt.Before(time.Now()) {
		return nil, appErrors.NewValidationError("expiry", "Invitation has expired")
	}

	return invitation, nil
}

// IsEmailAlreadyInvited checks if an email already has a pending invitation for an organization
func (s *invitationService) IsEmailAlreadyInvited(ctx context.Context, email string, orgID uuid.UUID) (bool, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	return s.inviteRepo.IsEmailAlreadyInvited(ctx, normalizedEmail, orgID)
}

// CleanupExpiredInvitations removes expired invitations
func (s *invitationService) CleanupExpiredInvitations(ctx context.Context) error {
	return s.inviteRepo.CleanupExpiredInvitations(ctx)
}

// createAuditEvent creates an audit event for an invitation action
func (s *invitationService) createAuditEvent(ctx context.Context, invitationID uuid.UUID, eventType orgDomain.InvitationAuditEventType, actorID *uuid.UUID, metadata map[string]any) {
	actorType := orgDomain.ActorTypeSystem
	if actorID != nil {
		actorType = orgDomain.ActorTypeUser
	}

	event := orgDomain.NewInvitationAuditEvent(invitationID, eventType, actorID, actorType)
	if metadata != nil {
		event.WithMetadata(metadata)
	}

	if err := s.inviteRepo.CreateAuditEvent(ctx, event); err != nil {
		s.logger.Error("failed to create audit event",
			"error", err,
			"invitation_id", invitationID,
			"event_type", eventType,
		)
	}
}

// sendInvitationEmail sends an invitation email asynchronously. A nil
// inviterID skips the email — the invitation still exists, but the
// inviter row has been deleted so we can't render the "invited by" line.
func (s *invitationService) sendInvitationEmail(ctx context.Context, invitation *orgDomain.Invitation, org *orgDomain.Organization, plainTextToken string, inviterID *uuid.UUID) {
	if s.emailSender == nil {
		s.logger.Debug("email sender not configured, skipping invitation email",
			"invitation_id", invitation.ID,
		)
		return
	}
	if inviterID == nil {
		s.logger.Warn("invitation has no inviter — skipping email",
			"invitation_id", invitation.ID,
		)
		return
	}

	// Get inviter info
	inviter, err := s.userRepo.GetByID(ctx, *inviterID)
	if err != nil {
		s.logger.Error("failed to get inviter for email",
			"error", err,
			"inviter_id", inviterID,
		)
		return
	}

	// Get role name
	role, err := s.roleService.GetRoleByID(ctx, invitation.RoleID)
	if err != nil {
		s.logger.Error("failed to get role for email",
			"error", err,
			"role_id", invitation.RoleID,
		)
		return
	}

	// Build accept URL
	acceptURL := s.appURL + "/accept-invite?token=" + plainTextToken

	// Build email params
	params := email.InvitationEmailParams{
		InviteeEmail:     invitation.Email,
		OrganizationName: org.Name,
		InviterName:      inviter.GetFullName(),
		InviterEmail:     inviter.Email,
		RoleName:         role.Name,
		AcceptURL:        acceptURL,
		ExpiresIn:        "7 days",
		AppName:          "Brokle",
		AppURL:           s.appURL,
	}

	if invitation.Message != nil {
		params.PersonalMessage = *invitation.Message
	}

	// Generate email content
	htmlContent, textContent, err := email.BuildInvitationEmail(params)
	if err != nil {
		s.logger.Error("failed to build invitation email",
			"error", err,
			"invitation_id", invitation.ID,
		)
		return
	}

	// Send email
	err = s.emailSender.Send(ctx, email.SendEmailParams{
		To:      []string{invitation.Email},
		Subject: "You're invited to join " + org.Name + " on Brokle",
		HTML:    htmlContent,
		Text:    textContent,
		Tags: map[string]string{
			"type":          "invitation",
			"organization":  org.ID.String(),
			"invitation_id": invitation.ID.String(),
		},
	})

	if err != nil {
		s.logger.Error("failed to send invitation email",
			"error", err,
			"invitation_id", invitation.ID,
			"email", invitation.Email,
		)
		return
	}

	s.logger.Info("invitation email sent",
		"invitation_id", invitation.ID,
		"email", invitation.Email,
	)
}
