package prompt

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// ListPrompts handles GET /api/v1/projects/:projectId/prompts
// @Summary List prompts for a project
// @Description Retrieve a paginated list of prompts with optional filtering
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param type query string false "Filter by prompt type (text, chat)"
// @Param tags query string false "Filter by tags (comma-separated)"
// @Param search query string false "Search in name and description"
// @Param page query int false "Page number (default: 1)"
// @Param limit query int false "Items per page (default: 50, max: 100)"
// @Success 200 {object} response.APIResponse{data=[]prompt.PromptListItem} "List of prompts"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts [get]
func (h *Handler) ListPrompts(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	filters := &promptDomain.PromptFilters{}

	if typeStr := c.Query("type"); typeStr != "" {
		promptType := promptDomain.PromptType(typeStr)
		if promptType != promptDomain.PromptTypeText && promptType != promptDomain.PromptTypeChat {
			response.Error(c, appErrors.NewValidationError("Invalid type", "type must be 'text' or 'chat'"))
			return
		}
		filters.Type = &promptType
	}

	if tagsStr := c.Query("tags"); tagsStr != "" {
		filters.Tags = strings.Split(tagsStr, ",")
	}

	if search := c.Query("search"); search != "" {
		filters.Search = &search
	}

	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)
	filters.Params = params

	prompts, total, err := h.promptService.ListPrompts(c.Request.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list prompts", "error", err)
		response.Error(c, err)
		return
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, prompts, paginationMeta)
}

// CreatePrompt handles POST /api/v1/projects/:projectId/prompts
// @Summary Create a new prompt
// @Description Create a new prompt with initial version
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body prompt.CreatePromptRequest true "Create prompt request"
// @Success 201 {object} response.APIResponse{data=prompt.PromptResponse} "Created prompt"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 409 {object} response.APIResponse{error=response.APIError} "Prompt name already exists"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts [post]
func (h *Handler) CreatePrompt(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req promptDomain.CreatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Name == "" {
		response.Error(c, appErrors.NewValidationError("Missing name", "prompt name is required"))
		return
	}
	if req.Template == nil {
		response.Error(c, appErrors.NewValidationError("Missing template", "prompt template is required"))
		return
	}

	var userID *uuid.UUID
	if uid, ok := middleware.GetUserIDFromContext(c); ok {
		userID = &uid
	}

	prompt, version, labels, err := h.promptService.CreatePrompt(c.Request.Context(), projectID, userID, &req)
	if err != nil {
		h.logger.Error("Failed to create prompt", "name", req.Name, "error", err)
		response.Error(c, err)
		return
	}

	resp := buildPromptResponse(prompt, version, labels)
	response.Created(c, resp)
}

// GetPrompt handles GET /api/v1/projects/:projectId/prompts/:promptId
// @Summary Get prompt by ID
// @Description Retrieve prompt details with latest version
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Success 200 {object} response.APIResponse{data=prompt.PromptResponse} "Prompt details"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Prompt not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId} [get]
func (h *Handler) GetPrompt(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	promptID, err := uuid.Parse(c.Param("promptId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid prompt ID", "promptId must be a valid UUID"))
		return
	}

	prompt, err := h.promptService.GetPromptByID(c.Request.Context(), projectID, promptID)
	if err != nil {
		h.logger.Error("Failed to get prompt", "prompt_id", promptID, "error", err)
		response.Error(c, err)
		return
	}

	opts := &promptDomain.GetPromptOptions{Label: "latest"}
	fullPrompt, err := h.promptService.GetPrompt(c.Request.Context(), prompt.ProjectID, prompt.Name, opts)
	if err != nil {
		h.logger.Error("Failed to get prompt with version", "prompt_id", promptID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, fullPrompt)
}

// UpdatePrompt handles PUT /api/v1/projects/:projectId/prompts/:promptId
// @Summary Update prompt metadata
// @Description Update prompt name, description, or tags (not template)
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Param request body prompt.UpdatePromptRequest true "Update prompt request"
// @Success 200 {object} response.APIResponse "Prompt updated"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Prompt not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId} [put]
func (h *Handler) UpdatePrompt(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	promptID, err := uuid.Parse(c.Param("promptId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid prompt ID", "promptId must be a valid UUID"))
		return
	}

	var req promptDomain.UpdatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	prompt, err := h.promptService.UpdatePrompt(c.Request.Context(), projectID, promptID, &req)
	if err != nil {
		h.logger.Error("Failed to update prompt", "prompt_id", promptID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, prompt)
}

// DeletePrompt handles DELETE /api/v1/projects/:projectId/prompts/:promptId
// @Summary Delete a prompt
// @Description Soft delete a prompt and all its versions
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Prompt not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId} [delete]
func (h *Handler) DeletePrompt(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	promptID, err := uuid.Parse(c.Param("promptId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid prompt ID", "promptId must be a valid UUID"))
		return
	}

	if err := h.promptService.DeletePrompt(c.Request.Context(), projectID, promptID); err != nil {
		h.logger.Error("Failed to delete prompt", "prompt_id", promptID, "error", err)
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// UpsertPrompt handles POST /v1/prompts (SDK)
// @Summary Create or update a prompt
// @Description Creates a new prompt or adds a new version if prompt already exists
// @Tags SDK Prompts
// @Accept json
// @Produce json
// @Security APIKeyAuth
// @Param request body prompt.UpsertPromptRequest true "Prompt upsert request"
// @Success 200 {object} response.APIResponse{data=prompt.UpsertResponse} "Upsert result"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /v1/prompts [post]
func (h *Handler) UpsertPrompt(c *gin.Context) {
	projectID := middleware.MustGetProjectID(c)

	var req promptDomain.UpsertPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Name == "" {
		response.Error(c, appErrors.NewValidationError("Missing name", "prompt name is required"))
		return
	}
	if req.Template == nil {
		response.Error(c, appErrors.NewValidationError("Missing template", "prompt template is required"))
		return
	}

	result, err := h.promptService.UpsertPrompt(c.Request.Context(), projectID, nil, &req)
	if err != nil {
		h.logger.Error("Failed to upsert prompt", "name", req.Name, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// ListPromptsSDK handles GET /v1/prompts (SDK)
// @Summary List prompts
// @Description Retrieve a list of prompts for the project
// @Tags SDK Prompts
// @Accept json
// @Produce json
// @Security APIKeyAuth
// @Param type query string false "Filter by prompt type (text, chat)"
// @Param tags query string false "Filter by tags (comma-separated)"
// @Param search query string false "Search in name and description"
// @Param page query int false "Page number (default: 1)"
// @Param limit query int false "Items per page (default: 50, max: 100)"
// @Success 200 {object} response.APIResponse{data=[]prompt.PromptListItem} "List of prompts"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /v1/prompts [get]
func (h *Handler) ListPromptsSDK(c *gin.Context) {
	projectID := middleware.MustGetProjectID(c)

	filters := &promptDomain.PromptFilters{}

	if typeStr := c.Query("type"); typeStr != "" {
		promptType := promptDomain.PromptType(typeStr)
		if promptType != promptDomain.PromptTypeText && promptType != promptDomain.PromptTypeChat {
			response.Error(c, appErrors.NewValidationError("Invalid type", "type must be 'text' or 'chat'"))
			return
		}
		filters.Type = &promptType
	}

	if tagsStr := c.Query("tags"); tagsStr != "" {
		filters.Tags = strings.Split(tagsStr, ",")
	}

	if search := c.Query("search"); search != "" {
		filters.Search = &search
	}

	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)
	filters.Params = params

	prompts, total, err := h.promptService.ListPrompts(c.Request.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list prompts", "error", err)
		response.Error(c, err)
		return
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, prompts, paginationMeta)
}

// GetPromptByName handles GET /v1/prompts/:name (SDK)
// @Summary Get prompt by name
// @Description Retrieve a prompt by name with optional label or version specification
// @Tags SDK Prompts
// @Accept json
// @Produce json
// @Security APIKeyAuth
// @Param name path string true "Prompt name"
// @Param label query string false "Label to resolve (default: latest)"
// @Param version query int false "Specific version number (takes precedence over label)"
// @Param cache_ttl query int false "Cache TTL in seconds (default: 60)"
// @Success 200 {object} response.APIResponse{data=prompt.PromptResponse} "Prompt data"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Prompt not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /v1/prompts/{name} [get]
func (h *Handler) GetPromptByName(c *gin.Context) {
	projectID := middleware.MustGetProjectID(c)

	name := c.Param("name")
	if name == "" {
		response.Error(c, appErrors.NewValidationError("Missing name", "prompt name path parameter is required"))
		return
	}

	opts := &promptDomain.GetPromptOptions{
		Label: c.DefaultQuery("label", "latest"),
	}

	if versionStr := c.Query("version"); versionStr != "" {
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid version", "version must be an integer"))
			return
		}
		opts.Version = &version
		opts.Label = ""
	}

	if cacheTTLStr := c.Query("cache_ttl"); cacheTTLStr != "" {
		cacheTTL, err := strconv.Atoi(cacheTTLStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid cache TTL", "cache_ttl must be an integer"))
			return
		}
		opts.CacheTTL = &cacheTTL
	}

	prompt, err := h.promptService.GetPrompt(c.Request.Context(), projectID, name, opts)
	if err != nil {
		h.logger.Error("Failed to get prompt", "name", name, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, prompt)
}

func buildPromptResponse(prompt *promptDomain.Prompt, version *promptDomain.Version, labels []string) *promptDomain.PromptResponse {
	resp := &promptDomain.PromptResponse{
		ID:            prompt.ID.String(),
		Name:          prompt.Name,
		Type:          prompt.Type,
		Description:   prompt.Description,
		Tags:          []string(prompt.Tags),
		Version:       version.Version,
		VersionID:     version.ID.String(),
		Labels:        labels,
		Template:      version.Template,
		Config:        version.Config,
		Variables:     []string(version.Variables),
		CommitMessage: version.CommitMessage,
		CreatedAt:     version.CreatedAt,
	}

	if version.CreatedBy != nil {
		resp.CreatedBy = version.CreatedBy.String()
	}

	return resp
}
