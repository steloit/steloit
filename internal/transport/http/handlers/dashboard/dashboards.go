package dashboard

import (
	"strconv"

	"github.com/gin-gonic/gin"

	dashboardDomain "brokle/internal/core/domain/dashboard"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// ListDashboards handles GET /api/v1/projects/:projectId/dashboards
// @Summary List dashboards for a project
// @Description Retrieve a paginated list of dashboards with optional filtering
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param name query string false "Filter by name (partial match)"
// @Param limit query int false "Items per page (default: 50)"
// @Param offset query int false "Offset for pagination"
// @Success 200 {object} response.APIResponse{data=dashboard.DashboardListResponse} "List of dashboards"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards [get]
func (h *Handler) ListDashboards(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	filter := &dashboardDomain.DashboardFilter{}

	if name := c.Query("name"); name != "" {
		filter.Name = name
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			response.Error(c, appErrors.NewValidationError("Invalid limit", "limit must be a positive integer"))
			return
		}
		filter.Limit = limit
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			response.Error(c, appErrors.NewValidationError("Invalid offset", "offset must be a non-negative integer"))
			return
		}
		filter.Offset = offset
	}

	resp, err := h.service.ListDashboards(c.Request.Context(), projectID, filter)
	if err != nil {
		h.logger.Error("Failed to list dashboards", "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, resp)
}

// CreateDashboard handles POST /api/v1/projects/:projectId/dashboards
// @Summary Create a new dashboard
// @Description Create a new dashboard for a project
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body dashboard.CreateDashboardRequest true "Create dashboard request"
// @Success 201 {object} response.APIResponse{data=dashboard.Dashboard} "Created dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 409 {object} response.APIResponse{error=response.APIError} "Dashboard name already exists"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards [post]
func (h *Handler) CreateDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	var req dashboardDomain.CreateDashboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Name == "" {
		response.Error(c, appErrors.NewValidationError("Name is required", "dashboard name is required"))
		return
	}

	var userID *ulid.ULID
	if uid, ok := middleware.GetUserIDULID(c); ok {
		userID = &uid
	}

	dashboard, err := h.service.CreateDashboard(c.Request.Context(), projectID, userID, &req)
	if err != nil {
		h.logger.Error("Failed to create dashboard", "name", req.Name, "error", err)
		response.Error(c, err)
		return
	}

	response.Created(c, dashboard)
}

// GetDashboard handles GET /api/v1/projects/:projectId/dashboards/:dashboardId
// @Summary Get dashboard by ID
// @Description Retrieve dashboard details
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Success 200 {object} response.APIResponse{data=dashboard.Dashboard} "Dashboard details"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId} [get]
func (h *Handler) GetDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	dashboardID, err := ulid.Parse(c.Param("dashboardId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dashboard ID", "dashboardId must be a valid ULID"))
		return
	}

	dashboard, err := h.service.GetDashboardByProject(c.Request.Context(), projectID, dashboardID)
	if err != nil {
		h.logger.Error("Failed to get dashboard", "dashboard_id", dashboardID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, dashboard)
}

// UpdateDashboard handles PUT /api/v1/projects/:projectId/dashboards/:dashboardId
// @Summary Update a dashboard
// @Description Update dashboard details, configuration, or layout
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Param request body dashboard.UpdateDashboardRequest true "Update dashboard request"
// @Success 200 {object} response.APIResponse{data=dashboard.Dashboard} "Updated dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 409 {object} response.APIResponse{error=response.APIError} "Dashboard name already exists"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId} [put]
func (h *Handler) UpdateDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	dashboardID, err := ulid.Parse(c.Param("dashboardId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dashboard ID", "dashboardId must be a valid ULID"))
		return
	}

	var req dashboardDomain.UpdateDashboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	dashboard, err := h.service.UpdateDashboard(c.Request.Context(), projectID, dashboardID, &req)
	if err != nil {
		h.logger.Error("Failed to update dashboard", "dashboard_id", dashboardID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, dashboard)
}

// DeleteDashboard handles DELETE /api/v1/projects/:projectId/dashboards/:dashboardId
// @Summary Delete a dashboard
// @Description Delete a dashboard
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Success 204 "Dashboard deleted"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId} [delete]
func (h *Handler) DeleteDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	dashboardID, err := ulid.Parse(c.Param("dashboardId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dashboard ID", "dashboardId must be a valid ULID"))
		return
	}

	if err := h.service.DeleteDashboard(c.Request.Context(), projectID, dashboardID); err != nil {
		h.logger.Error("Failed to delete dashboard", "dashboard_id", dashboardID, "error", err)
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// DuplicateDashboard handles POST /api/v1/projects/:projectId/dashboards/:dashboardId/duplicate
// @Summary Duplicate a dashboard
// @Description Create a copy of an existing dashboard with a new name
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Param request body dashboard.DuplicateDashboardRequest true "Duplicate dashboard request"
// @Success 201 {object} response.APIResponse{data=dashboard.Dashboard} "Duplicated dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 409 {object} response.APIResponse{error=response.APIError} "Dashboard name already exists"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId}/duplicate [post]
func (h *Handler) DuplicateDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	dashboardID, err := ulid.Parse(c.Param("dashboardId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dashboard ID", "dashboardId must be a valid ULID"))
		return
	}

	var req dashboardDomain.DuplicateDashboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Name == "" {
		response.Error(c, appErrors.NewValidationError("Name is required", "dashboard name is required"))
		return
	}

	dashboard, err := h.service.DuplicateDashboard(c.Request.Context(), projectID, dashboardID, &req)
	if err != nil {
		h.logger.Error("Failed to duplicate dashboard", "dashboard_id", dashboardID, "error", err)
		response.Error(c, err)
		return
	}

	response.Created(c, dashboard)
}

// LockDashboard handles POST /api/v1/projects/:projectId/dashboards/:dashboardId/lock
// @Summary Lock a dashboard
// @Description Lock a dashboard to prevent modifications
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Success 200 {object} response.APIResponse{data=dashboard.Dashboard} "Locked dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId}/lock [post]
func (h *Handler) LockDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	dashboardID, err := ulid.Parse(c.Param("dashboardId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dashboard ID", "dashboardId must be a valid ULID"))
		return
	}

	dashboard, err := h.service.LockDashboard(c.Request.Context(), projectID, dashboardID)
	if err != nil {
		h.logger.Error("Failed to lock dashboard", "dashboard_id", dashboardID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, dashboard)
}

// UnlockDashboard handles POST /api/v1/projects/:projectId/dashboards/:dashboardId/unlock
// @Summary Unlock a dashboard
// @Description Unlock a dashboard to allow modifications
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Success 200 {object} response.APIResponse{data=dashboard.Dashboard} "Unlocked dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId}/unlock [post]
func (h *Handler) UnlockDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	dashboardID, err := ulid.Parse(c.Param("dashboardId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dashboard ID", "dashboardId must be a valid ULID"))
		return
	}

	dashboard, err := h.service.UnlockDashboard(c.Request.Context(), projectID, dashboardID)
	if err != nil {
		h.logger.Error("Failed to unlock dashboard", "dashboard_id", dashboardID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, dashboard)
}

// ExportDashboard handles GET /api/v1/projects/:projectId/dashboards/:dashboardId/export
// @Summary Export a dashboard
// @Description Export a dashboard configuration as JSON for sharing or backup
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Success 200 {object} response.APIResponse{data=dashboard.DashboardExport} "Exported dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId}/export [get]
func (h *Handler) ExportDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	dashboardID, err := ulid.Parse(c.Param("dashboardId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dashboard ID", "dashboardId must be a valid ULID"))
		return
	}

	export, err := h.service.ExportDashboard(c.Request.Context(), projectID, dashboardID)
	if err != nil {
		h.logger.Error("Failed to export dashboard", "dashboard_id", dashboardID, "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, export)
}

// ImportDashboard handles POST /api/v1/projects/:projectId/dashboards/import
// @Summary Import a dashboard
// @Description Import a dashboard from an exported JSON configuration
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body dashboard.DashboardImportRequest true "Import dashboard request"
// @Success 201 {object} response.APIResponse{data=dashboard.Dashboard} "Imported dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 409 {object} response.APIResponse{error=response.APIError} "Dashboard name already exists"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/import [post]
func (h *Handler) ImportDashboard(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	var req dashboardDomain.DashboardImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var userID *ulid.ULID
	if uid, ok := middleware.GetUserIDULID(c); ok {
		userID = &uid
	}

	dashboard, err := h.service.ImportDashboard(c.Request.Context(), projectID, userID, &req)
	if err != nil {
		h.logger.Error("Failed to import dashboard", "error", err)
		response.Error(c, err)
		return
	}

	response.Created(c, dashboard)
}
