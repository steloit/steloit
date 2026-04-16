package observability

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"brokle/internal/core/domain/observability"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// CreateFilterPreset creates a new filter preset.
// @Summary Create filter preset
// @Description Create a new saved filter preset for traces or spans
// @Tags filter-presets
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param request body observability.CreateFilterPresetRequest true "Create filter preset request"
// @Success 201 {object} observability.FilterPreset
// @Failure 400 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/filter-presets [post]
func (h *Handler) CreateFilterPreset(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req observability.CreateFilterPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	preset, err := h.services.FilterPresetService.Create(c.Request.Context(), projectID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, preset)
}

// GetFilterPreset retrieves a filter preset by ID.
// @Summary Get filter preset
// @Description Retrieve a filter preset by its ID
// @Tags filter-presets
// @Produce json
// @Param projectId path string true "Project ID"
// @Param id path string true "Filter Preset ID"
// @Success 200 {object} observability.FilterPreset
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/filter-presets/{id} [get]
func (h *Handler) GetFilterPreset(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	presetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid filter preset ID", "id must be a valid UUID"))
		return
	}

	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	preset, err := h.services.FilterPresetService.GetByID(c.Request.Context(), projectID, presetID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, preset)
}

// UpdateFilterPreset updates a filter preset.
// @Summary Update filter preset
// @Description Update an existing filter preset
// @Tags filter-presets
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param id path string true "Filter Preset ID"
// @Param request body observability.UpdateFilterPresetRequest true "Update filter preset request"
// @Success 200 {object} observability.FilterPreset
// @Failure 400 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/filter-presets/{id} [patch]
func (h *Handler) UpdateFilterPreset(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	presetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid filter preset ID", "id must be a valid UUID"))
		return
	}

	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req observability.UpdateFilterPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	preset, err := h.services.FilterPresetService.Update(c.Request.Context(), projectID, presetID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, preset)
}

// DeleteFilterPreset deletes a filter preset.
// @Summary Delete filter preset
// @Description Delete a filter preset by its ID
// @Tags filter-presets
// @Param projectId path string true "Project ID"
// @Param id path string true "Filter Preset ID"
// @Success 204 "No Content"
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/filter-presets/{id} [delete]
func (h *Handler) DeleteFilterPreset(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	presetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid filter preset ID", "id must be a valid UUID"))
		return
	}

	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	if err := h.services.FilterPresetService.Delete(c.Request.Context(), projectID, presetID, userID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListFilterPresets lists filter presets for a project.
// @Summary List filter presets
// @Description List filter presets for a project, including user's own and public presets
// @Tags filter-presets
// @Produce json
// @Param projectId path string true "Project ID"
// @Param table_name query string false "Filter by table name (traces or spans)"
// @Param include_public query bool false "Include public presets (default: true)"
// @Success 200 {array} observability.FilterPreset
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/filter-presets [get]
func (h *Handler) ListFilterPresets(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var tableName *string
	if tn := c.Query("table_name"); tn != "" {
		tableName = &tn
	}

	includePublic := true
	if ip := c.Query("include_public"); ip == "false" {
		includePublic = false
	}

	presets, err := h.services.FilterPresetService.List(c.Request.Context(), projectID, userID, tableName, includePublic)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, presets)
}
