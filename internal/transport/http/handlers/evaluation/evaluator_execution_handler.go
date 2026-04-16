package evaluation

import (
	"strconv"

	"github.com/gin-gonic/gin"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/pagination"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type EvaluatorExecutionHandler struct {
	service evaluationDomain.EvaluatorExecutionService
}

func NewEvaluatorExecutionHandler(
	service evaluationDomain.EvaluatorExecutionService,
) *EvaluatorExecutionHandler {
	return &EvaluatorExecutionHandler{
		service: service,
	}
}

// ExecutionListResponse wraps the list response with pagination metadata.
type ExecutionListResponse struct {
	Executions []*evaluationDomain.EvaluatorExecutionResponse `json:"executions" swaggertype:"array,object"`
	Total      int64                                          `json:"total"`
	Page       int                                            `json:"page"`
	Limit      int                                            `json:"limit"`
}

// @Summary List evaluator executions
// @Description Returns execution history for an evaluator with optional filtering and pagination.
// @Tags Evaluator Executions
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 25)"
// @Param status query string false "Filter by status (pending, running, completed, failed, cancelled)"
// @Param trigger_type query string false "Filter by trigger type (automatic, manual)"
// @Success 200 {object} ExecutionListResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/executions [get]
func (h *EvaluatorExecutionHandler) List(c *gin.Context) {
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
	params.SetDefaults("created_at")

	var filter evaluationDomain.ExecutionFilter
	if status := c.Query("status"); status != "" {
		s := evaluationDomain.ExecutionStatus(status)
		filter.Status = &s
	}
	if triggerType := c.Query("trigger_type"); triggerType != "" {
		t := evaluationDomain.TriggerType(triggerType)
		filter.TriggerType = &t
	}

	executions, total, err := h.service.ListByEvaluatorID(c.Request.Context(), evaluatorID, projectID, &filter, params)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*evaluationDomain.EvaluatorExecutionResponse, len(executions))
	for i, execution := range executions {
		responses[i] = execution.ToResponse()
	}

	response.Success(c, &ExecutionListResponse{
		Executions: responses,
		Total:      total,
		Page:       params.Page,
		Limit:      params.Limit,
	})
}

// @Summary Get evaluator execution
// @Description Returns a specific evaluator execution by ID.
// @Tags Evaluator Executions
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Param executionId path string true "Execution ID"
// @Success 200 {object} evaluation.EvaluatorExecutionResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/executions/{executionId} [get]
func (h *EvaluatorExecutionHandler) Get(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	// Parse evaluatorId for validation (we don't use it in the query since execution ID is unique)
	_, err = ulid.Parse(c.Param("evaluatorId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid evaluator ID", "evaluatorId must be a valid ULID"))
		return
	}

	executionID, err := ulid.Parse(c.Param("executionId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid execution ID", "executionId must be a valid ULID"))
		return
	}

	execution, err := h.service.GetByID(c.Request.Context(), executionID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, execution.ToResponse())
}

// @Summary Get latest evaluator execution
// @Description Returns the most recent execution for an evaluator.
// @Tags Evaluator Executions
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Success 200 {object} evaluation.EvaluatorExecutionResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "No executions found"
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/executions/latest [get]
func (h *EvaluatorExecutionHandler) GetLatest(c *gin.Context) {
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

	execution, err := h.service.GetLatestByEvaluatorID(c.Request.Context(), evaluatorID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if execution == nil {
		response.Error(c, appErrors.NewNotFoundError("no executions found for this evaluator"))
		return
	}

	response.Success(c, execution.ToResponse())
}

// @Summary Get execution detail
// @Description Returns detailed execution information including span-level results for debugging.
// @Tags Evaluator Executions
// @Produce json
// @Param projectId path string true "Project ID"
// @Param evaluatorId path string true "Evaluator ID"
// @Param executionId path string true "Execution ID"
// @Success 200 {object} evaluation.EvaluatorExecutionDetailFlat
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/evaluators/{evaluatorId}/executions/{executionId}/detail [get]
func (h *EvaluatorExecutionHandler) GetDetail(c *gin.Context) {
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

	executionID, err := ulid.Parse(c.Param("executionId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid execution ID", "executionId must be a valid ULID"))
		return
	}

	detail, err := h.service.GetExecutionDetail(c.Request.Context(), executionID, projectID, evaluatorID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Flatten the response for frontend compatibility
	response.Success(c, detail.ToFlat())
}
