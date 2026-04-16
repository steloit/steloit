package project

import (
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"brokle/internal/config"
	"brokle/internal/core/domain/organization"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type Handler struct {
	config              *config.Config
	logger              *slog.Logger
	projectService      organization.ProjectService
	organizationService organization.OrganizationService
	memberService       organization.MemberService
}

func NewHandler(
	config *config.Config,
	logger *slog.Logger,
	projectService organization.ProjectService,
	organizationService organization.OrganizationService,
	memberService organization.MemberService,
) *Handler {
	return &Handler{
		config:              config,
		logger:              logger,
		projectService:      projectService,
		organizationService: organizationService,
		memberService:       memberService,
	}
}

// Request/Response Models

// ListRequest represents the request parameters for listing projects
type ListRequest struct {
	OrganizationID string `form:"organization_id" binding:"omitempty" example:"org_1234567890" description:"Optional filter by organization ID"`
	Status         string `form:"status" binding:"omitempty,oneof=active paused archived" example:"active" description:"Filter by project status"`
	Search         string `form:"search" binding:"omitempty" example:"chatbot" description:"Search projects by name or slug"`
	Page           int    `form:"page" binding:"omitempty,min=1" example:"1" description:"Page number (default: 1)"`
	Limit          int    `form:"limit" binding:"omitempty,min=1,max=100" example:"20" description:"Items per page (default: 20, max: 100)"`
}

// Project represents a project entity
type Project struct {
	CreatedAt      time.Time `json:"created_at" example:"2024-01-01T00:00:00Z" description:"Creation timestamp"`
	UpdatedAt      time.Time `json:"updated_at" example:"2024-01-01T00:00:00Z" description:"Last update timestamp"`
	ID             string    `json:"id" example:"proj_1234567890" description:"Unique project identifier"`
	Name           string    `json:"name" example:"AI Chatbot" description:"Project name"`
	Description    string    `json:"description,omitempty" example:"Customer support AI chatbot" description:"Optional project description"`
	OrganizationID string    `json:"organization_id" example:"org_1234567890" description:"Organization ID this project belongs to"`
	Status         string    `json:"status" example:"active" description:"Project status (active, archived)"`
}

// CreateProjectRequest represents the request to create a project
type CreateProjectRequest struct {
	Name           string `json:"name" binding:"required,min=2,max=100" example:"AI Chatbot" description:"Project name (2-100 characters)"`
	Description    string `json:"description,omitempty" binding:"omitempty,max=500" example:"Customer support AI chatbot" description:"Optional description (max 500 characters)"`
	OrganizationID string `json:"organization_id" binding:"required" example:"org_1234567890" description:"Organization ID this project belongs to"`
}

// UpdateProjectRequest represents the request to update a project
type UpdateProjectRequest struct {
	Name        string `json:"name,omitempty" binding:"omitempty,min=2,max=100" example:"AI Chatbot" description:"Project name (2-100 characters)"`
	Description string `json:"description,omitempty" binding:"omitempty,max=500" example:"Customer support AI chatbot" description:"Description (max 500 characters)"`
	// Status removed - use Archive/Unarchive endpoints instead
}

// List handles GET /api/v1/projects
// @Summary List projects
// @Description Get a paginated list of projects accessible to the authenticated user. Optionally filter by organization.
// @Tags Projects
// @Accept json
// @Produce json
// @Param organization_id query string false "Filter by organization ID" example("org_1234567890")
// @Param status query string false "Filter by project status" Enums(active,paused,archived)
// @Param cursor query string false "Cursor for pagination" example("eyJjcmVhdGVkX2F0IjoiMjAyNC0wMS0wMVQxMjowMDowMFoiLCJpZCI6IjAxSDJYM1k0WjUifQ==")
// @Param page_size query int false "Items per page" Enums(10,20,30,40,50) default(50)
// @Param sort_by query string false "Sort field" Enums(created_at,name) default("created_at")
// @Param sort_dir query string false "Sort direction" Enums(asc,desc) default("desc")
// @Param search query string false "Search projects by name or slug"
// @Success 200 {object} response.APIResponse{data=[]Project,meta=response.Meta{pagination=response.Pagination}} "List of projects with cursor pagination"
// @Failure 400 {object} response.ErrorResponse "Bad request"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects [get]
func (h *Handler) List(c *gin.Context) {
	// Extract user ID from JWT
	userID, exists := c.Get("user_id")
	if !exists {
		h.logger.Error("User not authenticated", "endpoint", "List")
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userULID, ok := userID.(ulid.ULID)
	if !ok {
		h.logger.Error("Invalid user ID type", "endpoint", "List", "user_id", userID)
		response.InternalServerError(c, "Authentication error")
		return
	}

	// Bind and validate query parameters
	var req ListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.logger.Error("Invalid request parameters", "endpoint", "List", "user_id", userULID.String(), "error", err.Error())
		response.Error(c, appErrors.NewValidationError("Invalid request parameters", ""))
		return
	}

	// Parse offset pagination parameters
	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)

	ctx := c.Request.Context()
	var projects []*organization.Project
	var total int

	// If organization_id is provided, filter by organization and validate access
	if req.OrganizationID != "" {
		orgULID, err := ulid.Parse(req.OrganizationID)
		if err != nil {
			h.logger.Error("Invalid organization ID format", "endpoint", "List", "user_id", userULID.String(), "organization_id", req.OrganizationID, "error", err.Error())
			response.Error(c, appErrors.NewValidationError("Invalid organization ID", ""))
			return
		}

		// Validate user is member of organization
		isMember, err := h.memberService.IsMember(ctx, userULID, orgULID)
		if err != nil {
			h.logger.Error("Failed to check organization membership", "endpoint", "List", "user_id", userULID.String(), "organization_id", req.OrganizationID, "error", err.Error())
			response.InternalServerError(c, "Failed to verify organization access")
			return
		}

		if !isMember {
			h.logger.Warn("User attempted to access organization projects without membership", "endpoint", "List", "user_id", userULID.String(), "organization_id", req.OrganizationID)
			response.Forbidden(c, "You don't have access to this organization")
			return
		}

		// Get projects for the specific organization
		orgProjects, err := h.projectService.GetProjectsByOrganization(ctx, orgULID)
		if err != nil {
			h.logger.Error("Failed to get organization projects", "endpoint", "List", "user_id", userULID.String(), "organization_id", req.OrganizationID, "error", err.Error())
			response.InternalServerError(c, "Failed to retrieve projects")
			return
		}

		projects = orgProjects
		total = len(projects)
	} else {
		// Get projects from all user's organizations
		userOrgs, err := h.organizationService.GetUserOrganizations(ctx, userULID)
		if err != nil {
			h.logger.Error("Failed to get user organizations", "endpoint", "List", "user_id", userULID.String(), "error", err.Error())
			response.InternalServerError(c, "Failed to retrieve projects")
			return
		}

		// Collect projects from all organizations
		var allProjects []*organization.Project
		for _, org := range userOrgs {
			orgProjects, err := h.projectService.GetProjectsByOrganization(ctx, org.ID)
			if err != nil {
				h.logger.Error("Failed to get projects for organization", "endpoint", "List", "user_id", userULID.String(), "organization_id", org.ID.String(), "error", err.Error())
				continue // Skip this organization but continue with others
			}
			allProjects = append(allProjects, orgProjects...)
		}

		projects = allProjects
		total = len(projects)
	}

	// Apply filtering
	var filteredProjects []*organization.Project
	for _, project := range projects {
		// Status filter
		if req.Status != "" && req.Status != "active" {
			// For now, all projects are considered "active" - extend this when status field is added
			continue
		}

		// Search filter
		if req.Search != "" {
			searchLower := strings.ToLower(req.Search)
			if !strings.Contains(strings.ToLower(project.Name), searchLower) {
				continue
			}
		}

		filteredProjects = append(filteredProjects, project)
	}

	// Update total after filtering
	total = len(filteredProjects)

	// Apply in-memory offset pagination
	// Sort filtered projects for stable ordering
	sort.Slice(filteredProjects, func(i, j int) bool {
		if params.SortDir == "asc" {
			return filteredProjects[i].CreatedAt.Before(filteredProjects[j].CreatedAt)
		}
		return filteredProjects[i].CreatedAt.After(filteredProjects[j].CreatedAt)
	})

	// Apply offset pagination
	offset := params.GetOffset()
	limit := params.Limit

	// Calculate end index for slicing
	end := offset + limit
	if end > len(filteredProjects) {
		end = len(filteredProjects)
	}

	// Apply pagination slice
	if offset < len(filteredProjects) {
		filteredProjects = filteredProjects[offset:end]
	} else {
		filteredProjects = []*organization.Project{}
	}

	// Convert to response format
	responseProjects := make([]Project, len(filteredProjects))
	for i, proj := range filteredProjects {
		responseProjects[i] = Project{
			ID:             proj.ID.String(),
			Name:           proj.Name,
			Description:    proj.Description,
			OrganizationID: proj.OrganizationID.String(),
			Status:         proj.Status,
			CreatedAt:      proj.CreatedAt,
			UpdatedAt:      proj.UpdatedAt,
		}
	}

	// Create offset pagination
	pag := response.NewPagination(params.Page, params.Limit, int64(total))

	response.SuccessWithPagination(c, responseProjects, pag)

	h.logger.Info("Projects listed successfully", "endpoint", "List", "user_id", userULID.String(), "organization_id", req.OrganizationID, "total_projects", total, "returned", len(responseProjects), "page", req.Page, "limit", req.Limit)
}

// Create handles POST /api/v1/projects
// @Summary Create project
// @Description Create a new project within an organization. User must have appropriate permissions in the organization.
// @Tags Projects
// @Accept json
// @Produce json
// @Param request body CreateProjectRequest true "Project details (includes organization_id)"
// @Success 201 {object} response.SuccessResponse{data=Project} "Project created successfully"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid input or validation errors"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions in organization"
// @Failure 404 {object} response.ErrorResponse "Organization not found"
// @Failure 409 {object} response.ErrorResponse "Conflict - project slug already exists in organization"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects [post]
func (h *Handler) Create(c *gin.Context) {
	// Extract user ID from JWT
	userID, exists := c.Get("user_id")
	if !exists {
		h.logger.Error("User not authenticated", "endpoint", "Create")
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userULID, ok := userID.(ulid.ULID)
	if !ok {
		h.logger.Error("Invalid user ID type", "endpoint", "Create", "user_id", userID)
		response.InternalServerError(c, "Authentication error")
		return
	}

	// Bind and validate request body
	var req CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	ctx := c.Request.Context()

	// Parse and validate organization ID from request body
	orgULID, err := ulid.Parse(req.OrganizationID)
	if err != nil {
		h.logger.Error("Invalid organization ID format", "endpoint", "Create", "user_id", userULID.String(), "organization_id", req.OrganizationID, "error", err.Error())
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", ""))
		return
	}

	// Validate user is member of organization
	isMember, err := h.memberService.IsMember(ctx, userULID, orgULID)
	if err != nil {
		h.logger.Error("Failed to check organization membership", "endpoint", "Create", "user_id", userULID.String(), "organization_id", req.OrganizationID, "error", err.Error())
		response.InternalServerError(c, "Failed to verify organization access")
		return
	}

	if !isMember {
		h.logger.Warn("User attempted to create project in organization without membership", "endpoint", "Create", "user_id", userULID.String(), "organization_id", req.OrganizationID)
		response.Forbidden(c, "You don't have permission to create projects in this organization")
		return
	}

	// Create project via service (no slug needed)
	createReq := &organization.CreateProjectRequest{
		Name:        req.Name,
		Description: req.Description,
	}

	project, err := h.projectService.CreateProject(ctx, orgULID, createReq)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			h.logger.Warn("Project name already exists", "endpoint", "Create", "user_id", userULID.String(), "organization_id", req.OrganizationID, "name", req.Name, "error", err.Error())
			response.Conflict(c, "Project with this slug already exists in organization")
			return
		}

		if strings.Contains(err.Error(), "not found") {
			h.logger.Error("Organization not found", "endpoint", "Create", "user_id", userULID.String(), "organization_id", req.OrganizationID, "error", err.Error())
			response.NotFound(c, "Organization")
			return
		}

		h.logger.Error("Failed to create project", "endpoint", "Create", "user_id", userULID.String(), "organization_id", req.OrganizationID, "project_name", req.Name, "error", err.Error())
		response.InternalServerError(c, "Failed to create project")
		return
	}

	// Environments are now tags, not entities

	// Convert to response format
	responseProject := Project{
		ID:             project.ID.String(),
		Name:           project.Name,
		Description:    project.Description,
		OrganizationID: project.OrganizationID.String(),
		Status:         project.Status,
		CreatedAt:      project.CreatedAt,
		UpdatedAt:      project.UpdatedAt,
	}

	response.Created(c, responseProject)

	h.logger.Info("Project created successfully", "endpoint", "Create", "user_id", userULID.String(), "organization_id", req.OrganizationID, "project_id", project.ID.String(), "project_name", project.Name)
}

// Get handles GET /api/v1/projects/:projectId
// @Summary Get project details
// @Description Get detailed information about a specific project
// @Tags Projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_1234567890")
// @Success 200 {object} response.SuccessResponse{data=Project} "Project details"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid project ID"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId} [get]
func (h *Handler) Get(c *gin.Context) {
	// Extract user ID from JWT
	userID, exists := c.Get("user_id")
	if !exists {
		h.logger.Error("User not authenticated", "endpoint", "Get")
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userULID, ok := userID.(ulid.ULID)
	if !ok {
		h.logger.Error("Invalid user ID type", "endpoint", "Get", "user_id", userID)
		response.InternalServerError(c, "Authentication error")
		return
	}

	// Parse and validate project ID from path parameter
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	ctx := c.Request.Context()

	// Validate user can access this project (checks org membership via project)
	err = h.projectService.ValidateProjectAccess(ctx, userULID, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("Project not found", "endpoint", "Get", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.NotFound(c, "Project")
			return
		}

		if strings.Contains(err.Error(), "access") {
			h.logger.Warn("User attempted to access project without permission", "endpoint", "Get", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.Forbidden(c, "You don't have access to this project")
			return
		}

		h.logger.Error("Failed to validate project access", "endpoint", "Get", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to validate project access")
		return
	}

	// Get project details
	project, err := h.projectService.GetProject(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to get project", "endpoint", "Get", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to retrieve project")
		return
	}

	// Environments are now tags, not entities

	// Convert to response format
	responseProject := Project{
		ID:             project.ID.String(),
		Name:           project.Name,
		Description:    project.Description,
		OrganizationID: project.OrganizationID.String(),
		Status:         project.Status,
		CreatedAt:      project.CreatedAt,
		UpdatedAt:      project.UpdatedAt,
	}

	response.Success(c, responseProject)

	h.logger.Info("Project retrieved successfully", "endpoint", "Get", "user_id", userULID.String(), "project_id", project.ID.String(), "project_name", project.Name)
}

// Update handles PUT /api/v1/projects/:projectId
// @Summary Update project
// @Description Update project details. Requires appropriate permissions within the project organization.
// @Tags Projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_1234567890")
// @Param request body UpdateProjectRequest true "Updated project details"
// @Success 200 {object} response.SuccessResponse{data=Project} "Project updated successfully"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid input or validation errors"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId} [put]
func (h *Handler) Update(c *gin.Context) {
	// Extract user ID from JWT
	userID, exists := c.Get("user_id")
	if !exists {
		h.logger.Error("User not authenticated", "endpoint", "Update")
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userULID, ok := userID.(ulid.ULID)
	if !ok {
		h.logger.Error("Invalid user ID type", "endpoint", "Update", "user_id", userID)
		response.InternalServerError(c, "Authentication error")
		return
	}

	// Parse and validate project ID from path parameter
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	// Bind and validate request body
	var req UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	ctx := c.Request.Context()

	// Validate user can access this project (checks org membership via project)
	err = h.projectService.ValidateProjectAccess(ctx, userULID, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("Project not found", "endpoint", "Update", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.NotFound(c, "Project")
			return
		}

		if strings.Contains(err.Error(), "access") {
			h.logger.Warn("User attempted to update project without permission", "endpoint", "Update", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.Forbidden(c, "You don't have permission to update this project")
			return
		}

		h.logger.Error("Failed to validate project access", "endpoint", "Update", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to validate project access")
		return
	}

	// Update project via service
	updateReq := &organization.UpdateProjectRequest{}
	if req.Name != "" {
		updateReq.Name = &req.Name
	}
	if req.Description != "" {
		updateReq.Description = &req.Description
	}

	err = h.projectService.UpdateProject(ctx, projectID, updateReq)
	if err != nil {
		h.logger.Error("Failed to update project", "endpoint", "Update", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.Error(c, err)
		return
	}

	// Get updated project details
	project, err := h.projectService.GetProject(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to get updated project", "endpoint", "Update", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to retrieve updated project")
		return
	}

	// Environments are now tags, not entities

	// Convert to response format
	responseProject := Project{
		ID:             project.ID.String(),
		Name:           project.Name,
		Description:    project.Description,
		OrganizationID: project.OrganizationID.String(),
		Status:         project.Status,
		CreatedAt:      project.CreatedAt,
		UpdatedAt:      project.UpdatedAt,
	}

	response.Success(c, responseProject)

	h.logger.Info("Project updated successfully", "endpoint", "Update", "user_id", userULID.String(), "project_id", project.ID.String(), "project_name", project.Name)
}

// Delete handles DELETE /api/v1/projects/:projectId
// @Summary Delete project
// @Description Permanently delete a project and all associated environments and data. This action cannot be undone.
// @Tags Projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_1234567890")
// @Success 204 "Project deleted successfully"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid project ID"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions (requires admin or owner role)"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 409 {object} response.ErrorResponse "Conflict - cannot delete project with active environments or API usage"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId} [delete]
func (h *Handler) Delete(c *gin.Context) {
	// Extract user ID from JWT
	userID, exists := c.Get("user_id")
	if !exists {
		h.logger.Error("User not authenticated", "endpoint", "Delete")
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userULID, ok := userID.(ulid.ULID)
	if !ok {
		h.logger.Error("Invalid user ID type", "endpoint", "Delete", "user_id", userID)
		response.InternalServerError(c, "Authentication error")
		return
	}

	// Parse and validate project ID from path parameter
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	ctx := c.Request.Context()

	// Validate user can access this project (checks org membership via project)
	err = h.projectService.ValidateProjectAccess(ctx, userULID, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("Project not found", "endpoint", "Delete", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.NotFound(c, "Project")
			return
		}

		if strings.Contains(err.Error(), "access") {
			h.logger.Warn("User attempted to delete project without permission", "endpoint", "Delete", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.Forbidden(c, "You don't have permission to delete this project")
			return
		}

		h.logger.Error("Failed to validate project access", "endpoint", "Delete", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to validate project access")
		return
	}

	// Get project details before deletion for logging
	project, err := h.projectService.GetProject(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to get project for deletion", "endpoint", "Delete", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to get project")
		return
	}

	// TODO: Add additional validation for admin/owner permissions
	// For now, we allow any organization member to delete projects

	// TODO: Check if project has active API keys or usage data
	// For now, we allow deletion regardless of active resources

	// Delete project via service (soft delete)
	err = h.projectService.DeleteProject(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to delete project", "endpoint", "Delete", "user_id", userULID.String(), "project_id", projectID.String(), "project_name", project.Name, "error", err.Error())
		response.InternalServerError(c, "Failed to delete project")
		return
	}

	response.NoContent(c)

	h.logger.Info("Project deleted successfully", "endpoint", "Delete", "user_id", userULID.String(), "project_id", project.ID.String(), "project_name", project.Name, "organization_id", project.OrganizationID.String())
}

// Archive handles POST /api/v1/projects/:projectId/archive
// @Summary Archive project
// @Description Archive a project (sets status to archived, read-only, reversible)
// @Tags Projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_1234567890")
// @Success 204 "Project archived successfully"
// @Failure 400 {object} response.ErrorResponse "Bad request - project already archived"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId}/archive [post]
func (h *Handler) Archive(c *gin.Context) {
	// Extract user ID from JWT
	userID, exists := c.Get("user_id")
	if !exists {
		h.logger.Error("User not authenticated", "endpoint", "Archive")
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userULID, ok := userID.(ulid.ULID)
	if !ok {
		h.logger.Error("Invalid user ID type", "endpoint", "Archive", "user_id", userID)
		response.InternalServerError(c, "Authentication error")
		return
	}

	// Parse and validate project ID from path parameter
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	ctx := c.Request.Context()

	// Validate user can access this project
	err = h.projectService.ValidateProjectAccess(ctx, userULID, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("Project not found", "endpoint", "Archive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.NotFound(c, "Project")
			return
		}

		if strings.Contains(err.Error(), "access") {
			h.logger.Warn("User attempted to archive project without permission", "endpoint", "Archive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.Forbidden(c, "You don't have permission to archive this project")
			return
		}

		h.logger.Error("Failed to validate project access", "endpoint", "Archive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to validate project access")
		return
	}

	// Archive project via service
	err = h.projectService.ArchiveProject(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to archive project", "endpoint", "Archive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.Error(c, err)
		return
	}

	response.NoContent(c)

	h.logger.Info("Project archived successfully", "endpoint", "Archive", "user_id", userULID.String(), "project_id", projectID.String())
}

// Unarchive handles POST /api/v1/projects/:projectId/unarchive
// @Summary Unarchive project
// @Description Unarchive a project (sets status back to active)
// @Tags Projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_1234567890")
// @Success 204 "Project unarchived successfully"
// @Failure 400 {object} response.ErrorResponse "Bad request - project already active"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId}/unarchive [post]
func (h *Handler) Unarchive(c *gin.Context) {
	// Extract user ID from JWT
	userID, exists := c.Get("user_id")
	if !exists {
		h.logger.Error("User not authenticated", "endpoint", "Unarchive")
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userULID, ok := userID.(ulid.ULID)
	if !ok {
		h.logger.Error("Invalid user ID type", "endpoint", "Unarchive", "user_id", userID)
		response.InternalServerError(c, "Authentication error")
		return
	}

	// Parse and validate project ID from path parameter
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	ctx := c.Request.Context()

	// Validate user can access this project
	err = h.projectService.ValidateProjectAccess(ctx, userULID, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("Project not found", "endpoint", "Unarchive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.NotFound(c, "Project")
			return
		}

		if strings.Contains(err.Error(), "access") {
			h.logger.Warn("User attempted to unarchive project without permission", "endpoint", "Unarchive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
			response.Forbidden(c, "You don't have permission to unarchive this project")
			return
		}

		h.logger.Error("Failed to validate project access", "endpoint", "Unarchive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.InternalServerError(c, "Failed to validate project access")
		return
	}

	// Unarchive project via service
	err = h.projectService.UnarchiveProject(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to unarchive project", "endpoint", "Unarchive", "user_id", userULID.String(), "project_id", projectID.String(), "error", err.Error())
		response.Error(c, err)
		return
	}

	response.NoContent(c)

	h.logger.Info("Project unarchived successfully", "endpoint", "Unarchive", "user_id", userULID.String(), "project_id", projectID.String())
}
