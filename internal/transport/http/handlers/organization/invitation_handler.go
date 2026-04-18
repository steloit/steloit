package organization

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/core/domain/organization"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// InviteMemberRequestV2 represents the enhanced request to invite a member
type InviteMemberRequestV2 struct {
	Email   string    `json:"email" binding:"required,email" example:"john@acme.com" description:"Email address of user to invite"`
	RoleID  uuid.UUID `json:"role_id" binding:"required" example:"01HX..." description:"Role ID to assign"`
	Message *string   `json:"message,omitempty" binding:"omitempty,max=500" example:"Welcome to the team!" description:"Optional personal message (max 500 chars)"`
}

// InvitationResponse represents an invitation in API responses
type InvitationResponse struct {
	CreatedAt      time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt      time.Time  `json:"updated_at" example:"2024-01-01T00:00:00Z"`
	ExpiresAt      time.Time  `json:"expires_at" example:"2024-01-08T00:00:00Z"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	ResentAt       *time.Time `json:"resent_at,omitempty"`
	ID             uuid.UUID  `json:"id" example:"01HX..."`
	Email          string     `json:"email" example:"john@acme.com"`
	Status         string     `json:"status" example:"pending"`
	TokenPreview   string     `json:"token_preview,omitempty" example:"inv_AbCd..."`
	RoleID         uuid.UUID  `json:"role_id" example:"01HX..."`
	RoleName       string     `json:"role_name" example:"developer"`
	Message        *string    `json:"message,omitempty"`
	InvitedByID    uuid.UUID  `json:"invited_by_id" example:"01HX..."`
	InvitedByEmail string     `json:"invited_by_email" example:"admin@acme.com"`
	InvitedByName  string     `json:"invited_by_name" example:"Admin User"`
	ResentCount    int        `json:"resent_count" example:"0"`
}

// AcceptInvitationRequest represents the request to accept an invitation
type AcceptInvitationRequest struct {
	Token string `json:"token" binding:"required" example:"inv_..." description:"Invitation token"`
}

// PendingInvitationsResponse represents the response for listing pending invitations
type PendingInvitationsResponse struct {
	Invitations []InvitationResponse `json:"invitations"`
	Total       int                  `json:"total"`
}

// AcceptInvitationResponse represents the response after accepting an invitation
type AcceptInvitationResponse struct {
	OrganizationID   uuid.UUID `json:"organization_id" example:"01HX..."`
	OrganizationName string `json:"organization_name" example:"Acme Inc"`
	RoleName         string `json:"role_name" example:"developer"`
	Message          string `json:"message" example:"Successfully joined organization"`
}

// UserInvitationResponse represents an invitation from the user's perspective
// Includes organization details that InvitationResponse doesn't have
type UserInvitationResponse struct {
	CreatedAt        time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
	ExpiresAt        time.Time `json:"expires_at" example:"2024-01-08T00:00:00Z"`
	ID               uuid.UUID `json:"id" example:"01HX..."`
	Email            string    `json:"email" example:"john@acme.com"`
	Status           string    `json:"status" example:"pending"`
	RoleName         string    `json:"role_name" example:"developer"`
	Message          *string   `json:"message,omitempty"`
	OrganizationID   uuid.UUID `json:"organization_id" example:"01HX..."`
	OrganizationName string    `json:"organization_name" example:"Acme Inc"`
	InvitedByName    string    `json:"invited_by_name" example:"Admin User"`
}

// UserInvitationsResponse wraps the list response for user invitations
type UserInvitationsResponse struct {
	Invitations []UserInvitationResponse `json:"invitations"`
	Total       int                      `json:"total"`
}

// CreateInvitation handles POST /organizations/:orgId/invitations (enhanced version)
// @Summary Invite user to organization
// @Description Create and send an invitation for a user to join the organization
// @Tags Invitations
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param request body InviteMemberRequestV2 true "Invitation details"
// @Success 201 {object} InvitationResponse "Invitation created"
// @Failure 400 {object} response.ErrorResponse "Bad request"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden"
// @Failure 409 {object} response.ErrorResponse "Conflict - already member or pending invitation"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/invitations [post]
func (h *Handler) CreateInvitation(c *gin.Context) {
	orgIDStr := c.Param("orgId")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID format", err.Error()))
		return
	}

	var req InviteMemberRequestV2
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userID := middleware.MustGetUserID(c)

	// Create invitation request
	inviteReq := &organization.InviteUserRequest{
		Email:   strings.ToLower(strings.TrimSpace(req.Email)),
		RoleID:  req.RoleID,
		Message: req.Message,
	}

	// Send invitation
	invitation, err := h.invitationService.InviteUser(c.Request.Context(), orgID, userID, inviteReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Get role name for response
	roleName := "Member"
	if role, err := h.roleService.GetRoleByID(c.Request.Context(), invitation.RoleID); err == nil {
		roleName = role.Name
	}

	// Get inviter details for response
	inviterEmail := ""
	inviterName := ""
	if inviter, err := h.userService.GetUser(c.Request.Context(), userID); err == nil {
		inviterEmail = inviter.Email
		inviterName = inviter.GetFullName()
	}

	resp := InvitationResponse{
		ID:             invitation.ID,
		Email:          invitation.Email,
		Status:         string(invitation.Status),
		TokenPreview:   invitation.TokenPreview,
		RoleID:         invitation.RoleID,
		RoleName:       roleName,
		Message:        invitation.Message,
		InvitedByID:    invitation.InvitedByID,
		InvitedByEmail: inviterEmail,
		InvitedByName:  inviterName,
		ExpiresAt:      invitation.ExpiresAt,
		ResentCount:    invitation.ResentCount,
		CreatedAt:      invitation.CreatedAt,
		UpdatedAt:      invitation.UpdatedAt,
	}

	c.Status(201)
	response.Success(c, resp)
}

// GetPendingInvitations handles GET /organizations/:orgId/invitations
// @Summary List pending invitations
// @Description Get all pending invitations for an organization
// @Tags Invitations
// @Produce json
// @Param orgId path string true "Organization ID"
// @Success 200 {object} PendingInvitationsResponse "List of pending invitations"
// @Failure 400 {object} response.ErrorResponse "Bad request"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/invitations [get]
func (h *Handler) GetPendingInvitations(c *gin.Context) {
	orgIDStr := c.Param("orgId")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID format", err.Error()))
		return
	}

	userID := middleware.MustGetUserID(c)

	// Verify user is member of organization
	isMember, err := h.memberService.IsMember(c.Request.Context(), userID, orgID)
	if err != nil {
		response.InternalServerError(c, "Failed to verify permissions")
		return
	}
	if !isMember {
		response.Forbidden(c, "You are not authorized to view this organization's invitations")
		return
	}

	// Get pending invitations
	invitations, err := h.invitationService.GetPendingInvitations(c.Request.Context(), orgID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Convert to response format
	respInvitations := make([]InvitationResponse, 0, len(invitations))
	for _, inv := range invitations {
		// Get role name
		roleName := "Member"
		if role, err := h.roleService.GetRoleByID(c.Request.Context(), inv.RoleID); err == nil {
			roleName = role.Name
		}

		// Get inviter details
		inviterEmail := ""
		inviterName := ""
		if inviter, err := h.userService.GetUser(c.Request.Context(), inv.InvitedByID); err == nil {
			inviterEmail = inviter.Email
			inviterName = inviter.GetFullName()
		}

		respInvitations = append(respInvitations, InvitationResponse{
			ID:             inv.ID,
			Email:          inv.Email,
			Status:         string(inv.Status),
			TokenPreview:   inv.TokenPreview,
			RoleID:         inv.RoleID,
			RoleName:       roleName,
			Message:        inv.Message,
			InvitedByID:    inv.InvitedByID,
			InvitedByEmail: inviterEmail,
			InvitedByName:  inviterName,
			ExpiresAt:      inv.ExpiresAt,
			ResentCount:    inv.ResentCount,
			ResentAt:       inv.ResentAt,
			CreatedAt:      inv.CreatedAt,
			UpdatedAt:      inv.UpdatedAt,
		})
	}

	resp := PendingInvitationsResponse{
		Invitations: respInvitations,
		Total:       len(respInvitations),
	}

	response.Success(c, resp)
}

// ResendInvitation handles POST /organizations/:orgId/invitations/:invitationId/resend
// @Summary Resend invitation
// @Description Resend an invitation email with a new token
// @Tags Invitations
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param invitationId path string true "Invitation ID"
// @Success 200 {object} InvitationResponse "Invitation resent"
// @Failure 400 {object} response.ErrorResponse "Bad request"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden"
// @Failure 404 {object} response.ErrorResponse "Invitation not found"
// @Failure 429 {object} response.ErrorResponse "Too many resend attempts"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/invitations/{invitationId}/resend [post]
func (h *Handler) ResendInvitation(c *gin.Context) {
	invitationIDStr := c.Param("invitationId")
	invitationID, err := uuid.Parse(invitationIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid invitation ID format", err.Error()))
		return
	}

	userID := middleware.MustGetUserID(c)

	// Resend invitation - returns updated invitation directly
	invitation, err := h.invitationService.ResendInvitation(c.Request.Context(), invitationID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Get role name
	roleName := "Member"
	if role, err := h.roleService.GetRoleByID(c.Request.Context(), invitation.RoleID); err == nil {
		roleName = role.Name
	}

	// Get inviter details
	inviterEmail := ""
	inviterName := ""
	if inviter, err := h.userService.GetUser(c.Request.Context(), invitation.InvitedByID); err == nil {
		inviterEmail = inviter.Email
		inviterName = inviter.GetFullName()
	}

	resp := InvitationResponse{
		ID:             invitation.ID,
		Email:          invitation.Email,
		Status:         string(invitation.Status),
		TokenPreview:   invitation.TokenPreview,
		RoleID:         invitation.RoleID,
		RoleName:       roleName,
		Message:        invitation.Message,
		InvitedByID:    invitation.InvitedByID,
		InvitedByEmail: inviterEmail,
		InvitedByName:  inviterName,
		ExpiresAt:      invitation.ExpiresAt,
		ResentCount:    invitation.ResentCount,
		ResentAt:       invitation.ResentAt,
		CreatedAt:      invitation.CreatedAt,
		UpdatedAt:      invitation.UpdatedAt,
	}

	response.Success(c, resp)
}

// RevokeInvitation handles DELETE /organizations/:orgId/invitations/:invitationId
// @Summary Revoke invitation
// @Description Revoke a pending invitation
// @Tags Invitations
// @Param orgId path string true "Organization ID"
// @Param invitationId path string true "Invitation ID"
// @Success 204 "Invitation revoked"
// @Failure 400 {object} response.ErrorResponse "Bad request"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden"
// @Failure 404 {object} response.ErrorResponse "Invitation not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/invitations/{invitationId} [delete]
func (h *Handler) RevokeInvitation(c *gin.Context) {
	invitationIDStr := c.Param("invitationId")
	invitationID, err := uuid.Parse(invitationIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid invitation ID format", err.Error()))
		return
	}

	userID := middleware.MustGetUserID(c)

	// Revoke invitation
	err = h.invitationService.RevokeInvitation(c.Request.Context(), invitationID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Status(204)
}

// AcceptInvitation handles POST /invitations/accept
// @Summary Accept invitation
// @Description Accept an invitation to join an organization
// @Tags Invitations
// @Accept json
// @Produce json
// @Param request body AcceptInvitationRequest true "Invitation token"
// @Success 200 {object} AcceptInvitationResponse "Invitation accepted"
// @Failure 400 {object} response.ErrorResponse "Bad request"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - email mismatch"
// @Failure 404 {object} response.ErrorResponse "Invitation not found"
// @Failure 410 {object} response.ErrorResponse "Invitation expired"
// @Failure 409 {object} response.ErrorResponse "Already a member"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/invitations/accept [post]
func (h *Handler) AcceptInvitation(c *gin.Context) {
	var req AcceptInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userID := middleware.MustGetUserID(c)

	// Accept invitation - returns org details directly (no extra DB query needed)
	result, err := h.invitationService.AcceptInvitation(c.Request.Context(), req.Token, userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, AcceptInvitationResponse{
		OrganizationID:   result.OrganizationID,
		OrganizationName: result.OrganizationName,
		RoleName:         result.RoleName,
		Message:          "Successfully joined organization",
	})
}

// DeclineInvitation handles POST /invitations/decline
// @Summary Decline an invitation
// @Description Decline an invitation to join an organization (no authentication required, uses token)
// @Tags Invitations
// @Accept json
// @Produce json
// @Param request body AcceptInvitationRequest true "Invitation token"
// @Success 200 {object} map[string]string "Invitation declined"
// @Failure 400 {object} response.ErrorResponse "Bad request"
// @Failure 404 {object} response.ErrorResponse "Invitation not found"
// @Failure 409 {object} response.ErrorResponse "Invitation not pending"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/invitations/decline [post]
func (h *Handler) DeclineInvitation(c *gin.Context) {
	var req AcceptInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	err := h.invitationService.DeclineInvitation(c.Request.Context(), req.Token)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, map[string]string{"message": "Invitation declined"})
}

// GetUserInvitations handles GET /invitations
// @Summary List user's invitations
// @Description Get all pending invitations for the authenticated user
// @Tags Invitations
// @Produce json
// @Success 200 {object} UserInvitationsResponse "List of pending invitations"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/invitations [get]
func (h *Handler) GetUserInvitations(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	// Get user email
	user, err := h.userService.GetUser(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Get invitations for user's email
	invitations, err := h.invitationService.GetUserInvitations(c.Request.Context(), user.Email)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Filter to only pending invitations
	respInvitations := make([]UserInvitationResponse, 0)
	for _, inv := range invitations {
		if inv.Status != organization.InvitationStatusPending {
			continue
		}

		// Skip expired invitations
		if time.Now().After(inv.ExpiresAt) {
			continue
		}

		// Get organization name
		orgName := ""
		if org, err := h.organizationService.GetOrganization(c.Request.Context(), inv.OrganizationID); err == nil {
			orgName = org.Name
		}

		// Get role name
		roleName := "Member"
		if role, err := h.roleService.GetRoleByID(c.Request.Context(), inv.RoleID); err == nil {
			roleName = role.Name
		}

		// Get inviter name
		inviterName := ""
		if inviter, err := h.userService.GetUser(c.Request.Context(), inv.InvitedByID); err == nil {
			inviterName = inviter.GetFullName()
		}

		respInvitations = append(respInvitations, UserInvitationResponse{
			ID:               inv.ID,
			Email:            inv.Email,
			Status:           string(inv.Status),
			RoleName:         roleName,
			Message:          inv.Message,
			OrganizationID:   inv.OrganizationID,
			OrganizationName: orgName,
			InvitedByName:    inviterName,
			ExpiresAt:        inv.ExpiresAt,
			CreatedAt:        inv.CreatedAt,
		})
	}

	resp := UserInvitationsResponse{
		Invitations: respInvitations,
		Total:       len(respInvitations),
	}

	response.Success(c, resp)
}
