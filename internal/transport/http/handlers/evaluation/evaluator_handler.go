package evaluation

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"brokle/internal/core/domain/analytics"
	evaluationDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/transport/http/handlers/shared"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/pagination"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type EvaluatorHandler struct {
	service evaluationDomain.EvaluatorService
}

func NewEvaluatorHandler(
	service evaluationDomain.EvaluatorService,
) *EvaluatorHandler {
	return &EvaluatorHandler{
		service: service,
	}
}

// @Summary Create evaluator
// @Description Creates a new evaluator for automated span scoring.
// @Tags Evaluators
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param request body evaluation.CreateEvaluatorRequest true "Evaluator request"
// @Success 201 {object} evaluation.EvaluatorResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/evaluators [post]
func (h *EvaluatorHandler) Create(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	var req evaluationDomain.CreateEvaluatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Get user ID if available (for created_by tracking)
	var userID *ulid.ULID
	if uid, ok := middleware.GetUserIDULID(c); ok {
		userID = &uid
	}

	evaluator, err := h.service.Create(c.Request.Context(), projectID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, evaluator.ToResponse())
}

// @Summary List evaluators
// @Description Returns all evaluators for the project with optional filtering and pagination.
// @Tags Evaluators
// @Produce json
// @Param projectId path string true "Project ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 50)"
// @Param status query string false "Filter by status (active, inactive, paused)"
// @Param scorer_type query string false "Filter by scorer type (llm, builtin, regex)"
// @Param search query string false "Search by name"
// @Success 200 {object} response.ListResponse{data=[]evaluation.EvaluatorResponse}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators [get]
func (h *EvaluatorHandler) List(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	// Parse pagination params
	params := pagination.Params{}
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed >= 1 {
			params.Page = parsed
		}
	}
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			params.Limit = parsed
		}
	}

	// Parse and validate sort params
	allowedSortFields := []string{"name", "status", "sampling_rate", "created_at", "updated_at"}
	if sortBy := c.Query("sort_by"); sortBy != "" {
		validatedField, err := pagination.ValidateSortField(sortBy, allowedSortFields)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("sort_by", err.Error()))
			return
		}
		params.SortBy = validatedField
	}
	if sortDir := c.Query("sort_dir"); sortDir != "" {
		if sortDir != "asc" && sortDir != "desc" {
			response.Error(c, appErrors.NewValidationError("sort_dir", "must be 'asc' or 'desc'"))
			return
		}
		params.SortDir = sortDir
	}

	params.SetDefaults("created_at")

	// Parse filter params
	var filter evaluationDomain.EvaluatorFilter
	if status := c.Query("status"); status != "" {
		s := evaluationDomain.EvaluatorStatus(status)
		filter.Status = &s
	}
	if scorerType := c.Query("scorer_type"); scorerType != "" {
		st := evaluationDomain.ScorerType(scorerType)
		filter.ScorerType = &st
	}
	if search := c.Query("search"); search != "" {
		filter.Search = &search
	}

	evaluators, total, err := h.service.List(c.Request.Context(), projectID, &filter, params)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*evaluationDomain.EvaluatorResponse, len(evaluators))
	for i, evaluator := range evaluators {
		responses[i] = evaluator.ToResponse()
	}

	pag := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, pag)
}

// @Summary Get evaluator
// @Description Returns the evaluator for a specific ID.
// @Tags Evaluators
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Success 200 {object} evaluation.EvaluatorResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId} [get]
func (h *EvaluatorHandler) Get(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	evaluator, err := h.service.GetByID(c.Request.Context(), evaluatorID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, evaluator.ToResponse())
}

// @Summary Update evaluator
// @Description Updates an existing evaluator by ID.
// @Tags Evaluators
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Param request body evaluation.UpdateEvaluatorRequest true "Update request"
// @Success 200 {object} evaluation.EvaluatorResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId} [put]
func (h *EvaluatorHandler) Update(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	var req evaluationDomain.UpdateEvaluatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	evaluator, err := h.service.Update(c.Request.Context(), evaluatorID, projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, evaluator.ToResponse())
}

// @Summary Delete evaluator
// @Description Removes an evaluator by its ID.
// @Tags Evaluators
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId} [delete]
func (h *EvaluatorHandler) Delete(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	if err := h.service.Delete(c.Request.Context(), evaluatorID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Activate evaluator
// @Description Activates an evaluator, enabling automatic span evaluation.
// @Tags Evaluators
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Success 200 {object} response.SuccessResponse "Evaluator activated"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/activate [post]
func (h *EvaluatorHandler) Activate(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	if err := h.service.Activate(c.Request.Context(), evaluatorID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, map[string]string{"message": "evaluator activated"})
}

// @Summary Deactivate evaluator
// @Description Deactivates an evaluator, stopping automatic span evaluation.
// @Tags Evaluators
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Success 200 {object} response.SuccessResponse "Evaluator deactivated"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/deactivate [post]
func (h *EvaluatorHandler) Deactivate(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	if err := h.service.Deactivate(c.Request.Context(), evaluatorID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, map[string]string{"message": "evaluator deactivated"})
}

// @Summary Trigger evaluator
// @Description Manually triggers an evaluator against matching spans. Returns 202 Accepted with execution ID for async processing.
// @Tags Evaluators
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Param request body evaluation.TriggerOptions false "Optional trigger options"
// @Success 202 {object} evaluation.TriggerResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/trigger [post]
func (h *EvaluatorHandler) Trigger(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	var opts *evaluationDomain.TriggerOptions
	if c.Request.ContentLength > 0 {
		var reqOpts evaluationDomain.TriggerOptions
		if err := c.ShouldBindJSON(&reqOpts); err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
			return
		}
		opts = &reqOpts
	}

	result, err := h.service.TriggerEvaluator(c.Request.Context(), evaluatorID, projectID, opts)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Return 202 Accepted for async processing (Opik pattern)
	response.Accepted(c, result)
}

// @Summary Test evaluator
// @Description Tests an evaluator against sample spans without persisting scores. Useful for validating evaluator configuration before activation.
// @Tags Evaluators
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Param request body evaluation.TestEvaluatorRequest false "Optional test parameters"
// @Success 200 {object} evaluation.TestEvaluatorResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/test [post]
func (h *EvaluatorHandler) Test(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	var req *evaluationDomain.TestEvaluatorRequest
	if c.Request.ContentLength > 0 {
		var reqBody evaluationDomain.TestEvaluatorRequest
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
			return
		}
		req = &reqBody
	}

	result, err := h.service.TestEvaluator(c.Request.Context(), evaluatorID, projectID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// @Summary Get evaluator analytics
// @Description Returns performance analytics for an evaluator over a specified time period.
// @Tags Evaluators
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Param period query string false "Time period: 24h, 7d, 30d (default: 7d)"
// @Param from_timestamp query string false "Custom start time (RFC3339 format)"
// @Param to_timestamp query string false "Custom end time (RFC3339 format)"
// @Success 200 {object} evaluation.EvaluatorAnalyticsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/analytics [get]
func (h *EvaluatorHandler) GetAnalytics(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	evaluatorID, err := ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	// Parse time range using shared utility
	fromTimestamp := c.Query("from_timestamp")
	toTimestamp := c.Query("to_timestamp")
	period := c.DefaultQuery("period", "7d")

	fromTime, toTime, err := shared.ParseTimeRange(
		fromTimestamp,
		toTimestamp,
		period,
		analytics.TimeRange7Days,
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	params := &evaluationDomain.EvaluatorAnalyticsParams{
		ProjectID:   projectID,
		EvaluatorID: evaluatorID,
		Period:      period,
		From:        &fromTime,
		To:          &toTime,
	}

	result, err := h.service.GetAnalytics(c.Request.Context(), evaluatorID, projectID, params)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}
