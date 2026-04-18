package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// ListSessions handles GET /api/v1/projects/:projectId/sessions
// @Summary List sessions for a project
// @Description Retrieve paginated list of sessions aggregated from traces
// @Tags Sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param search query string false "Filter by session ID (substring match)"
// @Param user_id query string false "Filter by user ID"
// @Param start_time query int64 false "Start time (Unix timestamp)"
// @Param end_time query int64 false "End time (Unix timestamp)"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Limit (default 20, max 100)"
// @Param sort_by query string false "Sort field: last_trace, first_trace, trace_count, total_tokens, total_cost (default: last_trace)"
// @Param sort_dir query string false "Sort direction: asc, desc (default: desc)"
// @Success 200 {object} response.APIResponse{data=[]observability.SessionSummary} "List of sessions with pagination"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/sessions [get]
func (h *Handler) ListSessions(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	filter := &observability.SessionFilter{
		ProjectID: projectID,
	}

	// Parse search filter
	if search := c.Query("search"); search != "" {
		filter.Search = &search
	}

	// Parse user filter
	if userID := c.Query("user_id"); userID != "" {
		filter.UserID = &userID
	}

	// Parse time range
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		startTimeInt, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid start_time", "start_time must be a Unix timestamp"))
			return
		}
		startTime := time.Unix(startTimeInt, 0)
		filter.StartTime = &startTime
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		endTimeInt, err := strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid end_time", "end_time must be a Unix timestamp"))
			return
		}
		endTime := time.Unix(endTimeInt, 0)
		filter.EndTime = &endTime
	}

	// Parse pagination and sorting
	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)
	filter.Params = params

	// Get sessions from service
	sessions, err := h.services.GetTraceService().ListSessions(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to list sessions", "error", err, "project_id", projectID)
		response.Error(c, err)
		return
	}

	// Get total count for pagination
	totalCount, err := h.services.GetTraceService().CountSessions(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to count sessions", "error", err, "project_id", projectID)
		response.Error(c, err)
		return
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, totalCount)

	// Return sessions array directly with standard pagination
	response.SuccessWithPagination(c, sessions, paginationMeta)
}
