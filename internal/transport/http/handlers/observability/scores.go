package observability

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"brokle/internal/core/domain/observability"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// ScoreResponse is a DTO for score API responses.
// Metadata is json.RawMessage to preserve the stored JSON exactly as-is,
// avoiding lossy map[string]any conversion that silently drops non-object values.
type ScoreResponse struct {
	ID               string           `json:"id"`
	ProjectID        string           `json:"project_id"`
	TraceID          *string          `json:"trace_id,omitempty"`
	SpanID           *string          `json:"span_id,omitempty"`
	Name             string           `json:"name"`
	Value            *float64         `json:"value,omitempty"`
	StringValue      *string          `json:"string_value,omitempty"`
	Type             string           `json:"type"`
	Source           string           `json:"source"`
	Reason           *string          `json:"reason,omitempty"`
	Metadata         json.RawMessage  `json:"metadata,omitempty"`
	ExperimentID     *string          `json:"experiment_id,omitempty"`
	ExperimentItemID *string          `json:"experiment_item_id,omitempty"`
	CreatedBy        *string          `json:"created_by,omitempty"`
	Timestamp        time.Time        `json:"timestamp"`
}

func toScoreResponse(s *observability.Score) *ScoreResponse {
	// Ensure metadata is safe for JSON serialization.
	// Valid JSON passes through as-is. Malformed bytes (from legacy writes)
	// are escaped as a JSON string so the raw content is preserved losslessly
	// rather than silently dropped or breaking Gin's c.JSON() marshaling.
	metadata := s.Metadata
	if len(metadata) > 0 && !json.Valid(metadata) {
		metadata, _ = json.Marshal(string(metadata))
	}

	return &ScoreResponse{
		ID:               s.ID,
		ProjectID:        s.ProjectID,
		TraceID:          s.TraceID,
		SpanID:           s.SpanID,
		Name:             s.Name,
		Value:            s.Value,
		StringValue:      s.StringValue,
		Type:             s.Type,
		Source:           s.Source,
		Reason:           s.Reason,
		Metadata:         metadata,
		ExperimentID:     s.ExperimentID,
		ExperimentItemID: s.ExperimentItemID,
		CreatedBy:        s.CreatedBy,
		Timestamp:        s.Timestamp,
	}
}

func toScoreResponses(scores []*observability.Score) []*ScoreResponse {
	result := make([]*ScoreResponse, 0, len(scores))
	for _, s := range scores {
		result = append(result, toScoreResponse(s))
	}
	return result
}

// Quality Score Handlers for Dashboard (JWT-authenticated, read + update operations)

// ListProjectScores handles GET /api/v1/projects/:projectId/scores
// @Summary List quality scores for a project
// @Description Retrieve paginated list of quality scores scoped to a project
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param trace_id query string false "Filter by trace ID"
// @Param span_id query string false "Filter by span ID"
// @Param name query string false "Filter by score name"
// @Param source query string false "Filter by source (API, AUTO, HUMAN, EVAL)"
// @Param type query string false "Filter by type (NUMERIC, CATEGORICAL, BOOLEAN)"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Page size (default 50, max 1000)"
// @Success 200 {object} response.APIResponse{data=[]ScoreResponse} "List of scores"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/scores [get]
func (h *Handler) ListProjectScores(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "projectId is required"))
		return
	}

	filter := &observability.ScoreFilter{
		ProjectID: projectID,
	}

	if traceID := c.Query("trace_id"); traceID != "" {
		filter.TraceID = &traceID
	}
	if spanID := c.Query("span_id"); spanID != "" {
		filter.SpanID = &spanID
	}
	if name := c.Query("name"); name != "" {
		filter.Name = &name
	}
	if source := c.Query("source"); source != "" {
		filter.Source = &source
	}
	if scoreType := c.Query("type"); scoreType != "" {
		filter.Type = &scoreType
	}

	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)
	filter.Params = params

	scores, err := h.services.GetScoreService().GetScoresByFilter(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	totalCount, err := h.services.GetScoreService().CountScores(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, totalCount)
	response.SuccessWithPagination(c, toScoreResponses(scores), paginationMeta)
}

// ListScores handles GET /api/v1/scores
// @Summary List quality scores with filtering
// @Description Retrieve paginated list of quality scores
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param trace_id query string false "Filter by trace ID"
// @Param span_id query string false "Filter by span ID"
// @Param session_id query string false "Filter by session ID"
// @Param name query string false "Filter by score name"
// @Param source query string false "Filter by source (API, AUTO, HUMAN, EVAL)"
// @Param type query string false "Filter by type (NUMERIC, CATEGORICAL, BOOLEAN)"
// @Param limit query int false "Limit (default 50, max 1000)"
// @Param offset query int false "Offset (default 0)"
// @Success 200 {object} response.APIResponse{data=[]ScoreResponse} "List of scores"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/scores [get]
func (h *Handler) ListScores(c *gin.Context) {
	filter := &observability.ScoreFilter{}

	if traceID := c.Query("trace_id"); traceID != "" {
		filter.TraceID = &traceID
	}
	if spanID := c.Query("span_id"); spanID != "" {
		filter.SpanID = &spanID
	}
	if name := c.Query("name"); name != "" {
		filter.Name = &name
	}
	if source := c.Query("source"); source != "" {
		filter.Source = &source
	}
	if scoreType := c.Query("type"); scoreType != "" {
		filter.Type = &scoreType
	}

	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)
	filter.Params = params

	scores, err := h.services.GetScoreService().GetScoresByFilter(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	totalCount, err := h.services.GetScoreService().CountScores(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, totalCount)

	response.SuccessWithPagination(c, toScoreResponses(scores), paginationMeta)
}

// GetScore handles GET /api/v1/scores/:id
// @Summary Get quality score by ID
// @Description Retrieve detailed score information
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Score ID"
// @Success 200 {object} response.APIResponse{data=ScoreResponse} "Score details"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Score not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/scores/{id} [get]
func (h *Handler) GetScore(c *gin.Context) {
	scoreID := c.Param("id")
	if scoreID == "" {
		response.Error(c, appErrors.NewValidationError("Missing score ID", "id is required"))
		return
	}

	score, err := h.services.GetScoreService().GetScoreByID(c.Request.Context(), scoreID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, toScoreResponse(score))
}

// UpdateScoreRequest is a DTO for score updates, preventing direct binding to domain entity.
// Metadata is json.RawMessage so the update API accepts the same JSON shapes the read API
// returns (objects, arrays, strings, numbers) — enabling lossless round-trips.
type UpdateScoreRequest struct {
	Name        string          `json:"name,omitempty"`
	Value       *float64        `json:"value,omitempty"`
	StringValue *string         `json:"string_value,omitempty"`
	Type        string          `json:"type,omitempty"`
	Source      string          `json:"source,omitempty"`
	Reason      *string         `json:"reason,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// UpdateScore handles PUT /api/v1/scores/:id
// @Summary Update quality score by ID
// @Description Update an existing score (for corrections/enrichment after initial creation)
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Score ID"
// @Param score body UpdateScoreRequest true "Updated score data"
// @Success 200 {object} response.APIResponse{data=ScoreResponse} "Updated score"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Score not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/scores/{id} [put]
func (h *Handler) UpdateScore(c *gin.Context) {
	scoreID := c.Param("id")
	if scoreID == "" {
		response.Error(c, appErrors.NewValidationError("Missing score ID", "id is required"))
		return
	}

	var req UpdateScoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Build domain Score from DTO
	score := observability.Score{
		ID:          scoreID,
		Name:        req.Name,
		Value:       req.Value,
		StringValue: req.StringValue,
		Type:        req.Type,
		Source:      req.Source,
		Reason:      req.Reason,
	}

	// Validate and assign raw JSON metadata directly (no marshaling needed)
	if len(req.Metadata) > 0 {
		if !json.Valid(req.Metadata) {
			response.Error(c, appErrors.NewValidationError("Invalid metadata", "metadata must be valid JSON"))
			return
		}
		score.Metadata = req.Metadata
	}

	if err := h.services.GetScoreService().UpdateScore(c.Request.Context(), &score); err != nil {
		response.Error(c, err)
		return
	}

	updated, err := h.services.GetScoreService().GetScoreByID(c.Request.Context(), scoreID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, toScoreResponse(updated))
}

// GetScoreAnalytics handles GET /api/v1/projects/:projectId/scores/analytics
// @Summary Get score analytics for a project
// @Description Retrieve comprehensive analytics for a score including statistics, time series, and distribution
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param score_name query string true "Score name to analyze"
// @Param compare_score_name query string false "Optional second score for comparison"
// @Param from_timestamp query string false "Start of time range (RFC3339)"
// @Param to_timestamp query string false "End of time range (RFC3339)"
// @Param interval query string false "Aggregation interval (hour, day, week)"
// @Success 200 {object} response.APIResponse{data=observability.ScoreAnalyticsResponse} "Analytics data"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/scores/analytics [get]
func (h *Handler) GetScoreAnalytics(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "projectId is required"))
		return
	}

	scoreName := c.Query("score_name")
	if scoreName == "" {
		response.Error(c, appErrors.NewValidationError("Missing score name", "score_name is required"))
		return
	}

	filter := &observability.ScoreAnalyticsFilter{
		ProjectID: projectID,
		ScoreName: scoreName,
		Interval:  c.DefaultQuery("interval", "day"),
	}

	if compareScoreName := c.Query("compare_score_name"); compareScoreName != "" {
		filter.CompareScoreName = &compareScoreName
	}

	if fromStr := c.Query("from_timestamp"); fromStr != "" {
		fromTime, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid from_timestamp", "must be RFC3339 format (e.g., 2024-01-15T00:00:00Z)"))
			return
		}
		filter.FromTimestamp = &fromTime
	}
	if toStr := c.Query("to_timestamp"); toStr != "" {
		toTime, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid to_timestamp", "must be RFC3339 format (e.g., 2024-01-15T23:59:59Z)"))
			return
		}
		filter.ToTimestamp = &toTime
	}

	analytics, err := h.services.GetScoreAnalyticsService().GetAnalytics(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, analytics)
}

// GetScoreNames handles GET /api/v1/projects/:projectId/scores/names
// @Summary Get distinct score names for a project
// @Description Retrieve all unique score names available in a project (for dropdown selection)
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Success 200 {object} response.APIResponse{data=[]string} "List of score names"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/scores/names [get]
func (h *Handler) GetScoreNames(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "projectId is required"))
		return
	}

	names, err := h.services.GetScoreAnalyticsService().GetDistinctScoreNames(c.Request.Context(), projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, names)
}

// CreateTraceScore handles POST /api/v1/traces/:id/scores
// @Summary Create annotation score for a trace
// @Description Creates a human annotation score for a trace (JWT-authenticated dashboard endpoint)
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Param project_id query string true "Project ID"
// @Param request body CreateAnnotationRequest true "Annotation data"
// @Success 201 {object} response.APIResponse{data=AnnotationResponse} "Created annotation"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Authentication required"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Trace not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id}/scores [post]
func (h *Handler) CreateTraceScore(c *gin.Context) {
	ctx := c.Request.Context()

	// Get trace ID from path
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	// Get project ID from query
	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}

	// Get user ID from JWT (for audit trail)
	userID, exists := middleware.GetUserIDULID(c)
	if !exists {
		response.Error(c, appErrors.NewUnauthorizedError("authentication required"))
		return
	}

	// Parse request body
	var req CreateAnnotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Get root span to retrieve organization ID
	rootSpan, err := h.services.GetTraceService().GetRootSpan(ctx, traceID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Validate that the trace belongs to the requested project
	if rootSpan.ProjectID != projectIDStr {
		response.Error(c, appErrors.NewValidationError("project_id", "does not match trace's project"))
		return
	}

	// Build the score entity
	userIDStr := userID.String()
	score := &observability.Score{
		ID:             ulid.New().String(),
		ProjectID:      projectIDStr,
		OrganizationID: rootSpan.OrganizationID,
		TraceID:        &traceID,
		SpanID:         &rootSpan.SpanID, // Use the actual root span's ID
		Name:           req.Name,
		Value:          req.Value,
		StringValue:    req.StringValue,
		Type:           req.DataType,
		Source:         observability.ScoreSourceAnnotation,
		Reason:         req.Reason,
		Metadata:       json.RawMessage("{}"),
		CreatedBy:      &userIDStr,
		Timestamp:      time.Now(),
	}

	// Create the score
	if err := h.services.GetScoreService().CreateScore(ctx, score); err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("annotation created",
		"score_id", score.ID,
		"project_id", projectIDStr,
		"trace_id", traceID,
		"user_id", userID,
		"name", req.Name,
	)

	// Return response
	response.Created(c, &AnnotationResponse{
		ID:          score.ID,
		ProjectID:   score.ProjectID,
		TraceID:     score.TraceID,
		SpanID:      score.SpanID,
		Name:        score.Name,
		Value:       score.Value,
		StringValue: score.StringValue,
		DataType:    score.Type,
		Source:      score.Source,
		Reason:      score.Reason,
		CreatedBy:   score.CreatedBy,
		Timestamp:   score.Timestamp.Format(time.RFC3339),
	})
}

// GetTraceScores handles GET /api/v1/traces/:id/scores
// @Summary List scores for a trace
// @Description Retrieve all scores (annotations and automated) for a specific trace
// @Tags Scores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Param project_id query string true "Project ID"
// @Success 200 {object} response.APIResponse{data=[]AnnotationResponse} "List of scores"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id}/scores [get]
func (h *Handler) GetTraceScores(c *gin.Context) {
	ctx := c.Request.Context()

	// Get trace ID from path
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	// Get project ID from query (for authorization)
	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	_ = projectIDStr // used for authorization context

	// Get scores for the trace
	scores, err := h.services.GetScoreService().GetScoresByTraceID(ctx, traceID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Convert to response format
	responses := make([]*AnnotationResponse, 0, len(scores))
	for _, score := range scores {
		responses = append(responses, &AnnotationResponse{
			ID:          score.ID,
			ProjectID:   score.ProjectID,
			TraceID:     score.TraceID,
			SpanID:      score.SpanID,
			Name:        score.Name,
			Value:       score.Value,
			StringValue: score.StringValue,
			DataType:    score.Type,
			Source:      score.Source,
			Reason:      score.Reason,
			CreatedBy:   score.CreatedBy,
			Timestamp:   score.Timestamp.Format(time.RFC3339),
		})
	}

	response.Success(c, responses)
}

// DeleteTraceScore handles DELETE /api/v1/traces/:id/scores/:score_id
// @Summary Delete annotation score
// @Description Deletes a human annotation score. Only the creator can delete their annotation.
// @Tags Scores
// @Produce json
// @Security BearerAuth
// @Param id path string true "Trace ID"
// @Param score_id path string true "Score ID"
// @Param project_id query string true "Project ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Authentication required"
// @Failure 403 {object} response.APIResponse{error=response.APIError} "Not the annotation owner"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Score not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/traces/{id}/scores/{score_id} [delete]
func (h *Handler) DeleteTraceScore(c *gin.Context) {
	ctx := c.Request.Context()

	// Get trace ID from path
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	// Get score ID from path
	scoreID := c.Param("score_id")
	if scoreID == "" {
		response.Error(c, appErrors.NewValidationError("Missing score ID", "score_id is required"))
		return
	}

	// Get project ID from query
	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	_ = projectIDStr // used for authorization context

	// Get user ID from JWT
	userID, exists := middleware.GetUserIDULID(c)
	if !exists {
		response.Error(c, appErrors.NewUnauthorizedError("authentication required"))
		return
	}

	// Get the score to verify ownership
	score, err := h.services.GetScoreService().GetScoreByID(ctx, scoreID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Verify the score belongs to the trace
	if score.TraceID == nil || *score.TraceID != traceID {
		response.Error(c, appErrors.NewNotFoundError("score"))
		return
	}

	// Only allow deletion of annotation scores by their creator
	if score.Source != observability.ScoreSourceAnnotation {
		response.Error(c, appErrors.NewForbiddenError("only annotation scores can be deleted"))
		return
	}

	if score.CreatedBy == nil || *score.CreatedBy != userID.String() {
		response.Error(c, appErrors.NewForbiddenError("only the creator can delete this annotation"))
		return
	}

	// Delete the score
	if err := h.services.GetScoreService().DeleteScore(ctx, scoreID); err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("annotation deleted",
		"score_id", scoreID,
		"project_id", projectIDStr,
		"trace_id", traceID,
		"user_id", userID,
	)

	response.NoContent(c)
}
