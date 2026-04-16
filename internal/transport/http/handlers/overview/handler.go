package overview

import (
	"log/slog"

	"brokle/internal/config"
	"brokle/internal/core/domain/analytics"
	"brokle/internal/transport/http/handlers/shared"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for project overview
type Handler struct {
	config          *config.Config
	logger          *slog.Logger
	overviewService analytics.OverviewService
}

// NewHandler creates a new overview handler
func NewHandler(
	config *config.Config,
	logger *slog.Logger,
	overviewService analytics.OverviewService,
) *Handler {
	return &Handler{
		config:          config,
		logger:          logger,
		overviewService: overviewService,
	}
}

// OverviewRequest represents the query parameters for the overview endpoint
type OverviewRequest struct {
	TimeRange string `form:"time_range" binding:"omitempty,oneof=15m 30m 1h 3h 6h 12h 24h 7d 14d 30d all"`
	From      string `form:"from" binding:"omitempty"` // ISO 8601 (RFC3339) for custom range start
	To        string `form:"to" binding:"omitempty"`   // ISO 8601 (RFC3339) for custom range end
}

// GetOverview handles GET /api/v1/projects/:projectId/overview
// @Summary Get project overview
// @Description Get comprehensive overview data for a project including stats, charts, and onboarding status
// @Tags Projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param time_range query string false "Relative time range preset" default("24h") Enums(15m,30m,1h,3h,6h,12h,24h,7d,14d,30d)
// @Param from query string false "Custom range start (ISO 8601/RFC3339, e.g., 2024-01-01T00:00:00Z)"
// @Param to query string false "Custom range end (ISO 8601/RFC3339, e.g., 2024-01-02T00:00:00Z)"
// @Success 200 {object} response.SuccessResponse{data=analytics.OverviewResponse} "Project overview data"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid parameters"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - no access to project"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId}/overview [get]
func (h *Handler) GetOverview(c *gin.Context) {
	// Parse project ID from path parameter
	projectIDStr := c.Param("projectId")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("project_id is required", "projectId path parameter is missing"))
		return
	}

	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	// Parse query parameters
	var req OverviewRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid query parameters", err.Error()))
		return
	}

	// Parse time range from request
	fromTime, toTime, err := shared.ParseTimeRange(req.From, req.To, req.TimeRange, analytics.TimeRange24Hours)
	if err != nil {
		response.Error(c, err)
		return
	}

	filter := &analytics.OverviewFilter{
		ProjectID: projectID,
		StartTime: fromTime,
		EndTime:   toTime,
	}

	// Get overview data from service
	overview, err := h.overviewService.GetOverview(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, appErrors.NewInternalError("Failed to get project overview", err))
		return
	}

	response.Success(c, overview)
}
