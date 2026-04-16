package dashboard

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	dashboardDomain "brokle/internal/core/domain/dashboard"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// ExecuteQueryRequest represents the request body for query execution
type ExecuteQueryRequest struct {
	TimeRange      *TimeRangeRequest      `json:"time_range,omitempty"`
	ForceRefresh   bool                   `json:"force_refresh,omitempty"`
	VariableValues map[string]interface{} `json:"variable_values,omitempty"`
}

// TimeRangeRequest represents time range parameters
type TimeRangeRequest struct {
	From     *time.Time `json:"from,omitempty"`
	To       *time.Time `json:"to,omitempty"`
	Relative string     `json:"relative,omitempty"` // "1h", "24h", "7d", "30d"
}

// QueryResult represents the result of a widget query (swagger doc type)
// This mirrors dashboardDomain.QueryResult for swagger documentation purposes
type QueryResult struct {
	WidgetID string                   `json:"widget_id"`
	Data     []map[string]interface{} `json:"data" swaggertype:"array,object"`
	Metadata *QueryMetadata           `json:"metadata,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

// QueryMetadata contains metadata about the query execution (swagger doc type)
type QueryMetadata struct {
	ExecutedAt     time.Time  `json:"executed_at"`
	DurationMs     int64      `json:"duration_ms"`
	RowCount       int        `json:"row_count"`
	Cached         bool       `json:"cached"`
	CacheExpiresAt *time.Time `json:"cache_expires_at,omitempty"`
}

// ExecuteDashboardQueries handles POST /api/v1/projects/:projectId/dashboards/:dashboardId/execute
// @Summary Execute all widget queries for a dashboard
// @Description Execute queries for all widgets in a dashboard and return results
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Param body body ExecuteQueryRequest false "Query execution parameters"
// @Success 200 {object} response.APIResponse{data=dashboard.DashboardQueryResults} "Query results"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId}/execute [post]
func (h *Handler) ExecuteDashboardQueries(c *gin.Context) {
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

	var req ExecuteQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Convert request to domain type
	execReq := &dashboardDomain.QueryExecutionRequest{
		ProjectID:      projectID,
		DashboardID:    dashboardID,
		ForceRefresh:   req.ForceRefresh,
		VariableValues: req.VariableValues,
	}

	if req.TimeRange != nil {
		execReq.TimeRange = &dashboardDomain.TimeRange{
			From:     req.TimeRange.From,
			To:       req.TimeRange.To,
			Relative: req.TimeRange.Relative,
		}
	}

	results, err := h.queryService.ExecuteDashboardQueries(c.Request.Context(), execReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, results)
}

// ExecuteWidgetQuery handles POST /api/v1/projects/:projectId/dashboards/:dashboardId/widgets/:widgetId/execute
// @Summary Execute query for a single widget
// @Description Execute the query for a specific widget and return results
// @Tags Dashboards
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param dashboardId path string true "Dashboard ID"
// @Param widgetId path string true "Widget ID"
// @Param body body ExecuteQueryRequest false "Query execution parameters"
// @Success 200 {object} response.APIResponse{data=dashboard.QueryResult} "Query result"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Dashboard or widget not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/{dashboardId}/widgets/{widgetId}/execute [post]
func (h *Handler) ExecuteWidgetQuery(c *gin.Context) {
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

	widgetID := c.Param("widgetId")
	if widgetID == "" {
		response.Error(c, appErrors.NewValidationError("Invalid widget ID", "widget_id is required"))
		return
	}

	var req ExecuteQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Convert request to domain type
	execReq := &dashboardDomain.QueryExecutionRequest{
		ProjectID:    projectID,
		DashboardID:  dashboardID,
		WidgetID:     &widgetID,
		ForceRefresh: req.ForceRefresh,
	}

	if req.TimeRange != nil {
		execReq.TimeRange = &dashboardDomain.TimeRange{
			From:     req.TimeRange.From,
			To:       req.TimeRange.To,
			Relative: req.TimeRange.Relative,
		}
	}

	results, err := h.queryService.ExecuteDashboardQueries(c.Request.Context(), execReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Return just the single widget result
	if result, ok := results.Results[widgetID]; ok {
		response.Success(c, result)
		return
	}

	response.NotFound(c, "widget")
}

// GetViewDefinitions handles GET /api/v1/dashboards/view-definitions
// @Summary Get available view definitions
// @Description Get available views, measures, and dimensions for the query builder
// @Tags Dashboards
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=dashboard.ViewDefinitionResponse} "View definitions"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/dashboards/view-definitions [get]
func (h *Handler) GetViewDefinitions(c *gin.Context) {
	definitions, err := h.queryService.GetViewDefinitions(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, definitions)
}

// GetVariableOptions handles GET /api/v1/projects/:projectId/dashboards/variable-options
// @Summary Get variable options
// @Description Get distinct values for a dimension to populate variable dropdowns
// @Tags Dashboards
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param view query string true "View type (traces, spans, scores)"
// @Param dimension query string true "Dimension field name"
// @Param limit query int false "Maximum number of options" default(100)
// @Success 200 {object} response.APIResponse{data=dashboard.VariableOptionsResponse} "Variable options"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/variable-options [get]
func (h *Handler) GetVariableOptions(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	view := c.Query("view")
	if view == "" {
		response.Error(c, appErrors.NewValidationError("View required", "view query parameter is required"))
		return
	}

	dimension := c.Query("dimension")
	if dimension == "" {
		response.Error(c, appErrors.NewValidationError("Dimension required", "dimension query parameter is required"))
		return
	}

	// Parse limit with default
	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, parseErr := strconv.Atoi(limitStr); parseErr == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	req := &dashboardDomain.VariableOptionsRequest{
		ProjectID: projectID,
		View:      dashboardDomain.ViewType(view),
		Dimension: dimension,
		Limit:     limit,
	}

	result, err := h.queryService.GetVariableOptions(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}
