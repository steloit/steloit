package prompt

import (
	"strconv"

	"github.com/gin-gonic/gin"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// ListVersions handles GET /api/v1/projects/:projectId/prompts/:promptId/versions
// @Summary List prompt versions
// @Description Retrieve all versions of a prompt
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Success 200 {object} response.APIResponse{data=[]prompt.VersionResponse} "List of versions"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Prompt not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId}/versions [get]
func (h *Handler) ListVersions(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	promptID, err := ulid.Parse(c.Param("promptId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid prompt ID", "promptId must be a valid ULID"))
		return
	}

	versions, err := h.promptService.ListVersions(c.Request.Context(), projectID, promptID)
	if err != nil {
		h.logger.Error("Failed to list versions", "prompt_id", promptID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, versions)
}

// CreateVersion handles POST /api/v1/projects/:projectId/prompts/:promptId/versions
// @Summary Create a new version
// @Description Create a new version of a prompt
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Param request body prompt.CreateVersionRequest true "Create version request"
// @Success 201 {object} response.APIResponse{data=prompt.VersionResponse} "Created version"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Prompt not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId}/versions [post]
func (h *Handler) CreateVersion(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	promptID, err := ulid.Parse(c.Param("promptId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid prompt ID", "promptId must be a valid ULID"))
		return
	}

	var req promptDomain.CreateVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Template == nil {
		response.Error(c, appErrors.NewValidationError("Missing template", "template is required"))
		return
	}

	var userID *ulid.ULID
	if uid, ok := middleware.GetUserIDULID(c); ok {
		userID = &uid
	}

	version, labels, err := h.promptService.CreateVersion(c.Request.Context(), projectID, promptID, userID, &req)
	if err != nil {
		h.logger.Error("Failed to create version", "prompt_id", promptID, "error", err)
		response.Error(c, err)
		return
	}

	resp := buildVersionResponse(version, labels)
	response.Created(c, resp)
}

// GetVersion handles GET /api/v1/projects/:projectId/prompts/:promptId/versions/:versionId
// @Summary Get version by ID
// @Description Retrieve a specific version of a prompt
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Param versionId path string true "Version ID or version number"
// @Success 200 {object} response.APIResponse{data=prompt.VersionResponse} "Version details"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Version not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId}/versions/{versionId} [get]
func (h *Handler) GetVersion(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	promptID, err := ulid.Parse(c.Param("promptId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid prompt ID", "promptId must be a valid ULID"))
		return
	}

	versionParam := c.Param("versionId")

	if versionNum, err := strconv.Atoi(versionParam); err == nil {
		versionResp, err := h.promptService.GetVersion(c.Request.Context(), projectID, promptID, versionNum)
		if err != nil {
			h.logger.Error("Failed to get version", "prompt_id", promptID, "version", versionNum, "error", err)
			response.Error(c, err)
			return
		}
		response.Success(c, versionResp)
		return
	}

	versionID, err := ulid.Parse(versionParam)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid version ID", "version_id must be a valid ULID or version number"))
		return
	}

	resp, err := h.promptService.GetVersionByID(c.Request.Context(), projectID, promptID, versionID)
	if err != nil {
		h.logger.Error("Failed to get version by ID", "version_id", versionID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, resp)
}

// GetVersionDiff handles GET /api/v1/projects/:projectId/prompts/:promptId/diff
// @Summary Compare two versions
// @Description Get the diff between two versions of a prompt
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Param from query int true "From version number"
// @Param to query int true "To version number"
// @Success 200 {object} response.APIResponse{data=prompt.VersionDiffResponse} "Version diff"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Version not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId}/diff [get]
func (h *Handler) GetVersionDiff(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	promptID, err := ulid.Parse(c.Param("promptId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid prompt ID", "promptId must be a valid ULID"))
		return
	}

	fromStr := c.Query("from")
	toStr := c.Query("to")

	if fromStr == "" || toStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing version range", "from and to version numbers are required"))
		return
	}

	from, err := strconv.Atoi(fromStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid from version", "from must be an integer"))
		return
	}

	to, err := strconv.Atoi(toStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid to version", "to must be an integer"))
		return
	}

	diff, err := h.promptService.GetVersionDiff(c.Request.Context(), projectID, promptID, from, to)
	if err != nil {
		h.logger.Error("Failed to get diff", "prompt_id", promptID, "from", from, "to", to, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, diff)
}

func buildVersionResponse(version *promptDomain.Version, labels []string) *promptDomain.VersionResponse {
	// Ensure labels serializes to [] instead of null
	if labels == nil {
		labels = []string{}
	}
	resp := &promptDomain.VersionResponse{
		ID:            version.ID.String(),
		Version:       version.Version,
		Template:      version.Template,
		Config:        version.Config,
		Variables:     []string(version.Variables),
		CommitMessage: version.CommitMessage,
		Labels:        labels,
		CreatedAt:     version.CreatedAt,
	}

	if version.CreatedBy != nil {
		resp.CreatedBy = version.CreatedBy.String()
	}

	return resp
}
