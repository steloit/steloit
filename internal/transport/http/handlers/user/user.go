package user

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/organization"
	"brokle/internal/core/domain/user"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/utils"
)

// Handler handles user endpoints
type Handler struct {
	config              *config.Config
	logger              *slog.Logger
	userService         user.UserService
	profileService      user.ProfileService
	organizationService organization.OrganizationService
}

// NewHandler creates a new user handler
func NewHandler(config *config.Config, logger *slog.Logger, userService user.UserService, profileService user.ProfileService, organizationService organization.OrganizationService) *Handler {
	return &Handler{
		config:              config,
		logger:              logger,
		userService:         userService,
		profileService:      profileService,
		organizationService: organizationService,
	}
}

// UserProfileResponse represents the complete user profile response
// @Description Complete user profile information including basic info and extended profile
type UserProfileResponse struct {
	CreatedAt             time.Time        `json:"created_at" example:"2025-01-01T00:00:00Z" description:"Account creation timestamp"`
	Profile               *UserProfileData `json:"profile,omitempty" description:"Extended profile information"`
	DefaultOrganizationID *uuid.UUID       `json:"default_organization_id,omitempty" example:"01K4FHGHT3XX9WFM293QPZ5G9V" description:"Default organization ID" swaggertype:"string"`
	LastLoginAt           *time.Time       `json:"last_login_at,omitempty" example:"2025-01-02T10:30:00Z" description:"Last login timestamp"`
	FirstName             string           `json:"first_name" example:"John" description:"User first name"`
	AvatarURL             string           `json:"avatar_url" example:"https://example.com/avatar.jpg" description:"Profile avatar URL"`
	LastName              string           `json:"last_name" example:"Doe" description:"User last name"`
	Name                  string           `json:"name" example:"John Doe" description:"User full name"`
	Email                 string           `json:"email" example:"user@example.com" description:"User email address"`
	Completeness          int              `json:"completeness" example:"85" description:"Profile completeness percentage"`
	ID                    uuid.UUID        `json:"id" example:"01K4FHGHT3XX9WFM293QPZ5G9V" description:"User unique identifier" swaggertype:"string"`
	IsEmailVerified       bool             `json:"is_email_verified" example:"true" description:"Email verification status"`
	IsActive              bool             `json:"is_active" example:"true" description:"Account active status"`
}

// UserProfileData represents extended profile information
// @Description Extended user profile data including bio, location, and social links
type UserProfileData struct {
	Bio         *string `json:"bio,omitempty" example:"Software engineer passionate about AI" description:"User biography"`
	Location    *string `json:"location,omitempty" example:"San Francisco, CA" description:"User location"`
	Website     *string `json:"website,omitempty" example:"https://johndoe.com" description:"Personal website URL"`
	TwitterURL  *string `json:"twitter_url,omitempty" example:"https://twitter.com/johndoe" description:"Twitter profile URL"`
	LinkedInURL *string `json:"linkedin_url,omitempty" example:"https://linkedin.com/in/johndoe" description:"LinkedIn profile URL"`
	GitHubURL   *string `json:"github_url,omitempty" example:"https://github.com/johndoe" description:"GitHub profile URL"`
	Timezone    string  `json:"timezone" example:"UTC" description:"User timezone preference"`
	Language    string  `json:"language" example:"en" description:"User language preference"`
	Theme       string  `json:"theme" example:"dark" description:"UI theme preference"`
}

// OrganizationWithProjects represents an organization with its nested projects
type OrganizationWithProjects struct {
	ID            uuid.UUID        `json:"id"`
	Name          string           `json:"name"`
	CompositeSlug string           `json:"composite_slug"`
	Plan          string           `json:"plan"`
	Role          string           `json:"role"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	Projects      []ProjectSummary `json:"projects"`
}

// ProjectSummary represents a summary of a project
type ProjectSummary struct {
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Description    *string   `json:"description,omitempty"`
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	CompositeSlug  string    `json:"composite_slug"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Status         string    `json:"status"`
}

// EnhancedUserProfileResponse extends UserProfileResponse with organizations hierarchy
type EnhancedUserProfileResponse struct {
	*UserProfileResponse                            // Embed pointer to preserve all existing fields
	Organizations        []OrganizationWithProjects `json:"organizations"`
}

// GetProfile handles GET /users/me
// @Summary Get current user profile
// @Description Get the profile information of the currently authenticated user
// @Tags User
// @Produce json
// @Security BearerAuth
// @Success 200 {object} UserProfileResponse "User profile retrieved successfully"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 404 {object} response.ErrorResponse "User not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/users/me [get]
func (h *Handler) GetProfile(c *gin.Context) {
	// Get user ID from middleware (set by auth middleware)
	userID := middleware.MustGetUserID(c)

	// Get basic user information
	userData, err := h.userService.GetUser(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user", "error", err, "user_id", userID)
		response.NotFound(c, "User not found")
		return
	}

	// Get extended profile information (this might not exist for all users)
	profileData, err := h.profileService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		// Profile might not exist yet, which is okay
		h.logger.Debug("Profile not found, using defaults", "error", err, "user_id", userID)
		profileData = nil
	}

	// Get profile completeness
	completeness, err := h.profileService.GetProfileCompleteness(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get profile completeness", "error", err, "user_id", userID)
		// Continue with 0% completeness
	}

	// Build response
	profileResponse := &UserProfileResponse{
		ID:                    userData.ID,
		Email:                 userData.Email,
		Name:                  userData.GetFullName(),
		FirstName:             userData.FirstName,
		LastName:              userData.LastName,
		AvatarURL:             "", // Now stored in profile
		IsEmailVerified:       userData.IsEmailVerified,
		IsActive:              userData.IsActive,
		CreatedAt:             userData.CreatedAt,
		LastLoginAt:           userData.LastLoginAt,
		DefaultOrganizationID: userData.DefaultOrganizationID,
		Completeness:          0, // Default
	}

	// Add extended profile data if available
	if profileData != nil {
		profileResponse.Profile = &UserProfileData{
			Bio:         profileData.Bio,
			Location:    profileData.Location,
			Website:     profileData.Website,
			TwitterURL:  profileData.TwitterURL,
			LinkedInURL: profileData.LinkedInURL,
			GitHubURL:   profileData.GitHubURL,
			Timezone:    profileData.Timezone,
			Language:    profileData.Language,
			Theme:       profileData.Theme,
		}
	}

	// Add completeness percentage if available
	if completeness != nil {
		profileResponse.Completeness = completeness.OverallScore
	}

	// Fetch user's organizations with nested projects
	userOrgsWithProjects, err := h.organizationService.GetUserOrganizationsWithProjects(
		c.Request.Context(),
		userID,
	)
	if err != nil {
		h.logger.Warn("Failed to load organizations hierarchy, returning empty array", "error", err)
		userOrgsWithProjects = []*organization.OrganizationWithProjectsAndRole{}
	}

	// Map to response format with composite slugs
	organizationsWithProjects := make([]OrganizationWithProjects, 0, len(userOrgsWithProjects))
	for _, orgData := range userOrgsWithProjects {
		// Generate composite slug for organization
		orgCompositeSlug := utils.GenerateCompositeSlug(orgData.Organization.Name, orgData.Organization.ID)

		// Map projects with composite slugs
		projectSummaries := make([]ProjectSummary, 0, len(orgData.Projects))
		for _, proj := range orgData.Projects {
			projectCompositeSlug := utils.GenerateCompositeSlug(proj.Name, proj.ID)

			projectSummaries = append(projectSummaries, ProjectSummary{
				ID:             proj.ID,
				Name:           proj.Name,
				CompositeSlug:  projectCompositeSlug,
				Description:    proj.Description,
				OrganizationID: proj.OrganizationID,
				Status:         proj.Status,
				CreatedAt:      proj.CreatedAt,
				UpdatedAt:      proj.UpdatedAt,
			})
		}

		organizationsWithProjects = append(organizationsWithProjects, OrganizationWithProjects{
			ID:            orgData.Organization.ID,
			Name:          orgData.Organization.Name,
			CompositeSlug: orgCompositeSlug,
			Plan:          orgData.Organization.Plan,
			Role:          orgData.RoleName,
			CreatedAt:     orgData.Organization.CreatedAt,
			UpdatedAt:     orgData.Organization.UpdatedAt,
			Projects:      projectSummaries,
		})
	}

	// Build enhanced response
	enhancedResponse := &EnhancedUserProfileResponse{
		UserProfileResponse: profileResponse,
		Organizations:       organizationsWithProjects,
	}

	h.logger.Info("User profile retrieved successfully", "user_id", userID)
	response.Success(c, enhancedResponse)
}

// UpdateProfileRequest represents the update profile request payload
// @Description User profile update information
type UpdateProfileRequest struct {
	FirstName *string `json:"first_name,omitempty" example:"John" description:"User first name"`
	LastName  *string `json:"last_name,omitempty" example:"Doe" description:"User last name"`
	AvatarURL *string `json:"avatar_url,omitempty" example:"https://example.com/avatar.jpg" description:"Profile avatar URL"`
	Phone     *string `json:"phone,omitempty" example:"+1234567890" description:"User phone number"`
	Timezone  *string `json:"timezone,omitempty" example:"UTC" description:"User timezone"`
	Language  *string `json:"language,omitempty" example:"en" description:"User language preference (ISO 639-1 code)"`
}

// UpdateProfile handles PATCH /users/me
// @Summary Update current user profile
// @Description Update the profile information of the currently authenticated user
// @Tags User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateProfileRequest true "Profile update information"
// @Success 200 {object} UserProfileResponse "Profile updated successfully"
// @Failure 400 {object} response.ErrorResponse "Invalid request payload"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/users/me [patch]
func (h *Handler) UpdateProfile(c *gin.Context) {
	// Get user ID from middleware (set by auth middleware)
	userID := middleware.MustGetUserID(c)

	// Parse request body
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Update basic user information (name) via user service
	if req.FirstName != nil || req.LastName != nil {
		userUpdateReq := &user.UpdateUserRequest{
			FirstName: req.FirstName,
			LastName:  req.LastName,
		}

		_, err := h.userService.UpdateUser(c.Request.Context(), userID, userUpdateReq)
		if err != nil {
			h.logger.Error("Failed to update user", "error", err, "user_id", userID)
			response.InternalServerError(c, "Failed to update user information")
			return
		}
	}

	// Update profile-specific information (timezone, language) via profile service
	if req.Timezone != nil || req.Language != nil {
		profileUpdateReq := &user.UpdateProfileRequest{
			Timezone: req.Timezone,
			Language: req.Language,
		}

		_, err := h.profileService.UpdateProfile(c.Request.Context(), userID, profileUpdateReq)
		if err != nil {
			// Profile might not exist yet - log but don't fail the entire operation
			h.logger.Debug("Profile update failed, profile may not exist yet", "error", err, "user_id", userID)
			// For now, we'll skip profile updates if the profile doesn't exist
			// In a future iteration, we could create the profile automatically
		}
	}

	// Return updated profile (call GetProfile internally to get consistent response)
	h.logger.Info("Profile updated successfully", "user_id", userID)

	// Re-fetch and return updated profile
	h.GetProfile(c)
}

// SetDefaultOrgRequest represents the request to set default organization
type SetDefaultOrgRequest struct {
	OrganizationID string `json:"organization_id" binding:"required"`
}

// SetDefaultOrganization handles PUT /users/me/default-organization
// @Summary Set user's default organization
// @Description Set the default organization for the authenticated user
// @Tags User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SetDefaultOrgRequest true "Organization ID to set as default"
// @Success 200 {object} map[string]string "Default organization updated successfully"
// @Failure 400 {object} response.ErrorResponse "Invalid request body or organization ID"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "User is not a member of the organization"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Router /api/v1/users/me/default-organization [put]
func (h *Handler) SetDefaultOrganization(c *gin.Context) {
	// Get user ID from middleware
	userID := middleware.MustGetUserID(c)

	// Parse request body
	var req SetDefaultOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Parse organization ID
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		h.logger.Error("Invalid organization ID format", "error", err, "organization_id", req.OrganizationID)
		response.Error(c, appErrors.NewValidationError("Invalid organization ID format", err.Error()))
		return
	}

	// Validate that user is a member of the organization
	isMember, err := h.userService.ValidateUserOrgMembership(c.Request.Context(), userID, orgID)
	if err != nil {
		h.logger.Error("Failed to validate organization membership", "error", err, "user_id", userID, "organization_id", orgID)
		response.InternalServerError(c, "Failed to validate organization membership")
		return
	}

	if !isMember {
		h.logger.Warn("User attempted to set default organization they are not a member of", "user_id", userID, "organization_id", orgID)
		response.Forbidden(c, "You are not a member of this organization")
		return
	}

	// Set default organization
	if err := h.userService.SetDefaultOrganization(c.Request.Context(), userID, orgID); err != nil {
		h.logger.Error("Failed to update default organization", "error", err, "user_id", userID, "organization_id", orgID)
		response.InternalServerError(c, "Failed to update default organization")
		return
	}

	h.logger.Info("Default organization updated successfully", "user_id", userID, "organization_id", orgID)

	response.Success(c, map[string]string{
		"message": "Default organization updated successfully",
	})
}
