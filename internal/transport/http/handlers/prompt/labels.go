package prompt

import (
	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// SetLabels handles PATCH /api/v1/projects/:projectId/prompts/:promptId/versions/:versionId/labels
// @Summary Set labels on a version
// @Description Add or update labels pointing to a specific version
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param promptId path string true "Prompt ID"
// @Param versionId path string true "Version ID"
// @Param request body prompt.SetLabelsRequest true "Set labels request"
// @Success 200 {object} response.APIResponse "Labels set successfully"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 403 {object} response.APIResponse{error=response.APIError} "Protected label modification forbidden"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Version not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/{promptId}/versions/{versionId}/labels [patch]
func (h *Handler) SetLabels(c *gin.Context) {
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

	versionID, err := uuid.Parse(c.Param("versionId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid version ID", "versionId must be a valid UUID"))
		return
	}

	var req promptDomain.SetLabelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var userID *uuid.UUID
	if uid, ok := middleware.GetUserIDFromContext(c); ok {
		userID = &uid
	}

	labels, err := h.promptService.SetLabels(c.Request.Context(), projectID, promptID, versionID, userID, req.Labels)
	if err != nil {
		h.logger.Error("Failed to set labels", "version_id", versionID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"labels": labels})
}

// GetProtectedLabels handles GET /api/v1/projects/:projectId/prompts/settings/protected-labels
// @Summary Get protected labels
// @Description Retrieve the list of protected labels for a project
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Success 200 {object} response.APIResponse{data=[]string} "List of protected labels"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/settings/protected-labels [get]
func (h *Handler) GetProtectedLabels(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	labels, err := h.promptService.GetProtectedLabels(c.Request.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get protected labels", "project_id", projectID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"protected_labels": labels})
}

// SetProtectedLabels handles PUT /api/v1/projects/:projectId/prompts/settings/protected-labels
// @Summary Set protected labels
// @Description Update the list of protected labels for a project
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body prompt.ProtectedLabelsRequest true "Protected labels request"
// @Success 200 {object} response.APIResponse "Protected labels updated"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/settings/protected-labels [put]
func (h *Handler) SetProtectedLabels(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req promptDomain.ProtectedLabelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var userID *uuid.UUID
	if uid, ok := middleware.GetUserIDFromContext(c); ok {
		userID = &uid
	}

	labels, err := h.promptService.SetProtectedLabels(c.Request.Context(), projectID, userID, req.ProtectedLabels)
	if err != nil {
		h.logger.Error("Failed to set protected labels", "project_id", projectID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"protected_labels": labels})
}
