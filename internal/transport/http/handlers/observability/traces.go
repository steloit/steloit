package observability

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"brokle/internal/core/domain/observability"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

func parseFloat64(value string) *float64 {
	if value == "" {
		return nil
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseInt64(value string) *int64 {
	if value == "" {
		return nil
	}
	i, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &i
}

func parseBool(value string) *bool {
	if value == "" {
		return nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return nil
	}
	return &b
}

func filterEmptyStrings(slice []string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// ListTraces handles GET /api/v1/traces
// @Summary List traces for a project
// @Description Retrieve paginated list of traces with optional filtering
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param project_id query string true "Project ID"
// @Param session_id query string false "Filter by session ID"
// @Param user_id query string false "Filter by user ID"
// @Param service_name query string false "Filter by service name (OTLP resource attribute)"
// @Param model_name query string false "Filter by AI model name (e.g., gpt-4, claude-3-opus)"
// @Param provider_name query string false "Filter by provider name (e.g., openai, anthropic)"
// @Param min_cost query number false "Minimum total cost filter"
// @Param max_cost query number false "Maximum total cost filter"
// @Param min_tokens query int64 false "Minimum total tokens filter"
// @Param max_tokens query int64 false "Maximum total tokens filter"
// @Param min_duration query int64 false "Minimum duration filter (nanoseconds)"
// @Param max_duration query int64 false "Maximum duration filter (nanoseconds)"
// @Param has_error query boolean false "Filter traces with errors only"
// @Param start_time query int64 false "Start time (Unix timestamp)"
// @Param end_time query int64 false "End time (Unix timestamp)"
// @Param limit query int false "Limit (default 50, max 1000)"
// @Param offset query int false "Offset (default 0)"
// @Success 200 {object} response.APIResponse{data=[]observability.TraceSummary} "List of trace summaries"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces [get]
func (h *Handler) ListTraces(c *gin.Context) {
	// Project scoping + membership check is enforced by RequireProjectAccess
	// middleware; the resolved projectID is pinned to the context.
	filter := &observability.TraceFilter{
		ProjectID: middleware.MustGetProjectID(c),
	}

	if sessionID := c.Query("session_id"); sessionID != "" {
		filter.SessionID = &sessionID
	}
	if userID := c.Query("user_id"); userID != "" {
		filter.UserID = &userID
	}
	if serviceName := c.Query("service_name"); serviceName != "" {
		filter.ServiceName = &serviceName
	}

	if modelName := c.Query("model_name"); modelName != "" {
		filter.ModelName = &modelName
	}
	if providerName := c.Query("provider_name"); providerName != "" {
		filter.ProviderName = &providerName
	}
	filter.MinCost = parseFloat64(c.Query("min_cost"))
	filter.MaxCost = parseFloat64(c.Query("max_cost"))
	filter.MinTokens = parseInt64(c.Query("min_tokens"))
	filter.MaxTokens = parseInt64(c.Query("max_tokens"))
	filter.MinDuration = parseInt64(c.Query("min_duration"))
	filter.MaxDuration = parseInt64(c.Query("max_duration"))
	filter.HasError = parseBool(c.Query("has_error"))

	if search := c.Query("search"); search != "" {
		filter.Search = &search
	}
	if searchType := c.Query("search_type"); searchType != "" {
		filter.SearchType = &searchType
	}

	// Status filter (comma-separated: "ok,error,unset")
	if statusStr := c.Query("status"); statusStr != "" {
		filter.Statuses = filterEmptyStrings(strings.Split(statusStr, ","))
		for _, s := range filter.Statuses {
			if s != "ok" && s != "error" && s != "unset" {
				response.Error(c, appErrors.NewValidationError("Invalid status value", "status must be one of: ok, error, unset (got: "+s+")"))
				return
			}
		}
	}
	// Status exclusion filter (comma-separated: "ok,error,unset")
	if statusNotStr := c.Query("status_not"); statusNotStr != "" {
		filter.StatusesNot = filterEmptyStrings(strings.Split(statusNotStr, ","))
		for _, s := range filter.StatusesNot {
			if s != "ok" && s != "error" && s != "unset" {
				response.Error(c, appErrors.NewValidationError("Invalid status_not value", "status_not must be one of: ok, error, unset (got: "+s+")"))
				return
			}
		}
	}

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

	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)
	filter.Params = params

	traces, err := h.services.GetTraceService().ListTraces(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to list traces", "error", err)
		response.Error(c, err)
		return
	}

	totalCount, err := h.services.GetTraceService().CountTraces(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to count traces", "error", err)
		response.Error(c, err)
		return
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, totalCount)

	response.SuccessWithPagination(c, traces, paginationMeta)
}

// GetTrace handles GET /api/v1/traces/:id
// @Summary Get trace by ID
// @Description Retrieve detailed trace information (aggregated from spans)
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Success 200 {object} response.APIResponse{data=observability.TraceSummary} "Trace summary"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Trace not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id} [get]
func (h *Handler) GetTrace(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	traceSummary, err := h.services.GetTraceService().GetTrace(c.Request.Context(), traceID)
	if err != nil {
		h.logger.Error("Failed to get trace summary", "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, traceSummary)
}

// GetTraceWithSpans handles GET /api/v1/traces/:id/spans
// @Summary Get trace with spans tree
// @Description Retrieve trace with all spans in hierarchical structure
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Success 200 {object} response.APIResponse{data=[]observability.Span} "Spans for trace"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Trace not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id}/spans [get]
func (h *Handler) GetTraceWithSpans(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	spans, err := h.services.GetTraceService().GetTraceSpans(c.Request.Context(), traceID)
	if err != nil {
		h.logger.Error("Failed to get spans for trace", "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, spans)
}

// GetTraceWithScores handles GET /api/v1/traces/:id/scores
// @Summary Get trace with quality scores
// @Description Retrieve trace with all associated quality scores
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Success 200 {object} response.APIResponse{data=[]ScoreResponse} "Scores for trace"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Trace not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id}/scores [get]
func (h *Handler) GetTraceWithScores(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	scores, err := h.services.GetScoreService().GetScoresByTraceID(c.Request.Context(), traceID)
	if err != nil {
		h.logger.Error("Failed to get scores for trace", "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, toScoreResponses(scores))
}

// DeleteTrace handles DELETE /api/v1/traces/:id
// @Summary Delete a trace
// @Description Delete all spans belonging to a trace
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Success 204 "No Content"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Trace not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id} [delete]
func (h *Handler) DeleteTrace(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	if err := h.services.GetTraceService().DeleteTrace(c.Request.Context(), traceID); err != nil {
		h.logger.Error("Failed to delete trace", "error", err)
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// UpdateTraceTags handles PUT /api/v1/traces/:id/tags
// @Summary Update trace tags
// @Description Update user-managed tags for a trace (replaces existing tags)
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Param project_id query string true "Project ID"
// @Param body body observability.UpdateTraceTagsRequest true "Tags to set"
// @Success 200 {object} response.APIResponse{data=map[string]interface{}} "Tags updated"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Trace not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id}/tags [put]
func (h *Handler) UpdateTraceTags(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}
	projectID, err := uuid.Parse(c.Query("project_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	var req observability.UpdateTraceTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validate tags
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		response.Error(c, appErrors.NewValidationError("Validation failed", validationErrors[0].Message))
		return
	}

	tags, err := h.services.GetTraceService().UpdateTraceTags(c.Request.Context(), projectID, traceID, req.Tags)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"tags": tags})
}

// UpdateTraceBookmarkRequest represents the request to update trace bookmark
type UpdateTraceBookmarkRequest struct {
	Bookmarked bool `json:"bookmarked"`
}

// UpdateTraceBookmark handles PUT /api/v1/traces/:id/bookmark
// @Summary Update trace bookmark status
// @Description Update the bookmark status for a trace
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Param project_id query string true "Project ID"
// @Param body body UpdateTraceBookmarkRequest true "Bookmark status"
// @Success 200 {object} response.APIResponse{data=map[string]interface{}} "Bookmark updated"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Trace not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id}/bookmark [put]
func (h *Handler) UpdateTraceBookmark(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}
	projectID, err := uuid.Parse(c.Query("project_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	var req UpdateTraceBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if err := h.services.GetTraceService().UpdateTraceBookmark(c.Request.Context(), projectID, traceID, req.Bookmarked); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"bookmarked": req.Bookmarked})
}

// GetTraceFilterOptions handles GET /api/v1/traces/filter-options
// @Summary Get available filter options for traces
// @Description Retrieve available filter values from actual trace data for populating filter UI
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param project_id query string true "Project ID"
// @Success 200 {object} response.APIResponse{data=observability.TraceFilterOptions} "Filter options"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/filter-options [get]
func (h *Handler) GetTraceFilterOptions(c *gin.Context) {
	projectID, err := uuid.Parse(c.Query("project_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	options, err := h.services.GetTraceService().GetFilterOptions(c.Request.Context(), projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, options)
}

// DiscoverAttributes handles GET /api/v1/traces/attributes
// @Summary Discover attribute keys from trace data
// @Description Extract unique attribute keys from span_attributes and resource_attributes for filter autocomplete
// @Tags Traces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param project_id query string true "Project ID"
// @Param prefix query string false "Filter attribute keys by prefix"
// @Param source query string false "Filter by source (span_attributes, resource_attributes)"
// @Param limit query int false "Maximum number of attributes to return (default 100, max 500)"
// @Success 200 {object} response.APIResponse{data=observability.AttributeDiscoveryResponse} "Discovered attributes"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/attributes [get]
func (h *Handler) DiscoverAttributes(c *gin.Context) {
	projectID, err := uuid.Parse(c.Query("project_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	req := &observability.AttributeDiscoveryRequest{
		ProjectID: projectID,
		Prefix:    c.Query("prefix"),
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	if source := c.Query("source"); source != "" {
		switch source {
		case "span_attributes":
			req.Sources = []observability.AttributeSource{observability.AttributeSourceSpan}
		case "resource_attributes":
			req.Sources = []observability.AttributeSource{observability.AttributeSourceResource}
		default:
			response.Error(c, appErrors.NewValidationError("Invalid source", "source must be 'span_attributes' or 'resource_attributes'"))
			return
		}
	}

	attributes, err := h.services.GetTraceService().DiscoverAttributes(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, attributes)
}
