package evaluation

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

type ExperimentHandler struct {
	logger      *slog.Logger
	service     evaluationDomain.ExperimentService
	itemService evaluationDomain.ExperimentItemService
}

func NewExperimentHandler(
	logger *slog.Logger,
	service evaluationDomain.ExperimentService,
	itemService evaluationDomain.ExperimentItemService,
) *ExperimentHandler {
	return &ExperimentHandler{
		logger:      logger,
		service:     service,
		itemService: itemService,
	}
}

// @Summary Create experiment
// @Description Creates a new experiment for the project. Works for both SDK and Dashboard routes.
// @Tags Experiments, SDK - Experiments
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param request body evaluation.CreateExperimentRequest true "Experiment request"
// @Success 201 {object} evaluation.ExperimentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Dataset not found"
// @Router /api/v1/projects/{projectId}/experiments [post]
// @Router /v1/experiments [post]
func (h *ExperimentHandler) Create(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req evaluationDomain.CreateExperimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	experiment, err := h.service.Create(c.Request.Context(), projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("experiment created",
		"experiment_id", experiment.ID,
		"project_id", projectID,
		"name", experiment.Name,
	)

	response.Created(c, experiment.ToResponse())
}

// @Summary List experiments
// @Description Returns all experiments for the project with pagination. Works for both SDK and Dashboard routes.
// @Tags Experiments, SDK - Experiments
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 50)"
// @Param dataset_id query string false "Filter by dataset ID"
// @Param status query string false "Filter by status (pending, running, completed, failed)"
// @Param search query string false "Search by name or description"
// @Param ids query string false "Filter by specific experiment IDs (comma-separated UUIDs)"
// @Success 200 {object} response.ListResponse{data=[]evaluation.ExperimentResponse}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/experiments [get]
// @Router /v1/experiments [get]
func (h *ExperimentHandler) List(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	params := response.ParsePaginationParams(c.Query("page"), c.Query("limit"), "", "")

	var filter *evaluationDomain.ExperimentFilter

	if datasetIDStr := c.Query("dataset_id"); datasetIDStr != "" {
		datasetID, err := uuid.Parse(datasetIDStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("dataset_id", "must be a valid UUID"))
			return
		}
		if filter == nil {
			filter = &evaluationDomain.ExperimentFilter{}
		}
		filter.DatasetID = &datasetID
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status := evaluationDomain.ExperimentStatus(statusStr)
		switch status {
		case evaluationDomain.ExperimentStatusPending,
			evaluationDomain.ExperimentStatusRunning,
			evaluationDomain.ExperimentStatusCompleted,
			evaluationDomain.ExperimentStatusFailed,
			evaluationDomain.ExperimentStatusPartial,
			evaluationDomain.ExperimentStatusCancelled:
			if filter == nil {
				filter = &evaluationDomain.ExperimentFilter{}
			}
			filter.Status = &status
		default:
			response.Error(c, appErrors.NewValidationError("status", "must be pending, running, completed, failed, partial, or cancelled"))
			return
		}
	}

	if search := c.Query("search"); search != "" {
		if filter == nil {
			filter = &evaluationDomain.ExperimentFilter{}
		}
		filter.Search = &search
	}

	// Parse ids filter (comma-separated)
	if idsStr := c.Query("ids"); idsStr != "" {
		var ids []uuid.UUID
		idParts := strings.Split(idsStr, ",")
		for _, idStr := range idParts {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			id, err := uuid.Parse(idStr)
			if err != nil {
				response.Error(c, appErrors.NewValidationError("ids", "invalid UUID: "+idStr))
				return
			}
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			if filter == nil {
				filter = &evaluationDomain.ExperimentFilter{}
			}
			filter.IDs = ids
		}
	}

	experiments, total, err := h.service.List(c.Request.Context(), projectID, filter, params.Page, params.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*evaluationDomain.ExperimentResponse, len(experiments))
	for i, exp := range experiments {
		responses[i] = exp.ToResponse()
	}

	pag := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, pag)
}

// @Summary Get experiment
// @Description Returns the experiment for a specific ID. Works for both SDK and Dashboard routes.
// @Tags Experiments, SDK - Experiments
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param experimentId path string true "Experiment ID"
// @Success 200 {object} evaluation.ExperimentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/experiments/{experimentId} [get]
// @Router /v1/experiments/{experimentId} [get]
func (h *ExperimentHandler) Get(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid UUID"))
		return
	}

	experiment, err := h.service.GetByID(c.Request.Context(), experimentID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, experiment.ToResponse())
}

// @Summary Update experiment
// @Description Updates an existing experiment by ID. Works for both SDK and Dashboard routes.
// @Tags Experiments, SDK - Experiments
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param experimentId path string true "Experiment ID"
// @Param request body evaluation.UpdateExperimentRequest true "Update request"
// @Success 200 {object} evaluation.ExperimentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/experiments/{experimentId} [put]
// @Router /v1/experiments/{experimentId} [patch]
func (h *ExperimentHandler) Update(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid UUID"))
		return
	}

	var req evaluationDomain.UpdateExperimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	experiment, err := h.service.Update(c.Request.Context(), experimentID, projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("experiment updated",
		"experiment_id", experimentID,
		"project_id", projectID,
		"status", experiment.Status,
	)

	response.Success(c, experiment.ToResponse())
}

// @Summary Delete experiment
// @Description Removes an experiment by its ID. Also deletes all items in the experiment.
// @Tags Experiments
// @Produce json
// @Param projectId path string true "Project ID"
// @Param experimentId path string true "Experiment ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/experiments/{experimentId} [delete]
func (h *ExperimentHandler) Delete(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid UUID"))
		return
	}

	if err := h.service.Delete(c.Request.Context(), experimentID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Compare experiments
// @Description Compares score metrics across multiple experiments. Optionally specify a baseline for diff calculations.
// @Tags Experiments, SDK - Experiments
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param request body CompareExperimentsRequest true "Compare experiments request"
// @Success 200 {object} CompareExperimentsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Experiment not found"
// @Router /api/v1/projects/{projectId}/experiments/compare [post]
// @Router /v1/experiments/compare [post]
func (h *ExperimentHandler) CompareExperiments(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req CompareExperimentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	experimentIDs := make([]uuid.UUID, len(req.ExperimentIDs))
	for i, idStr := range req.ExperimentIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("experiment_ids", "invalid UUID at index "+strconv.Itoa(i)))
			return
		}
		experimentIDs[i] = id
	}

	var baselineID *uuid.UUID
	if req.BaselineID != nil {
		id, err := uuid.Parse(*req.BaselineID)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("baseline_id", "must be a valid UUID"))
			return
		}
		baselineID = &id
	}

	result, err := h.service.CompareExperiments(c.Request.Context(), projectID, experimentIDs, baselineID)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := &CompareExperimentsResponse{
		Experiments: make(map[string]*ExperimentSummaryResponse),
		Scores:      make(map[string]map[string]*ScoreAggregationResponse),
	}

	for id, exp := range result.Experiments {
		resp.Experiments[id] = &ExperimentSummaryResponse{
			Name:   exp.Name,
			Status: exp.Status,
		}
	}

	for scoreName, expScores := range result.Scores {
		resp.Scores[scoreName] = make(map[string]*ScoreAggregationResponse)
		for expID, agg := range expScores {
			resp.Scores[scoreName][expID] = &ScoreAggregationResponse{
				Mean:   agg.Mean,
				StdDev: agg.StdDev,
				Min:    agg.Min,
				Max:    agg.Max,
				Count:  agg.Count,
			}
		}
	}

	if result.Diffs != nil {
		resp.Diffs = make(map[string]map[string]*ScoreDiffResponse)
		for scoreName, expDiffs := range result.Diffs {
			resp.Diffs[scoreName] = make(map[string]*ScoreDiffResponse)
			for expID, diff := range expDiffs {
				if diff != nil {
					resp.Diffs[scoreName][expID] = &ScoreDiffResponse{
						Type:       string(diff.Type),
						Difference: diff.Difference,
						Direction:  diff.Direction,
					}
				}
			}
		}
	}

	response.Success(c, resp)
}

// @Summary Batch create experiment items via SDK
// @Description Creates multiple items for an experiment using API key authentication.
// @Tags SDK - Experiments
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param experimentId path string true "Experiment ID"
// @Param request body evaluation.CreateExperimentItemsBatchRequest true "Batch items request"
// @Success 201 {object} SDKBatchCreateExperimentItemsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /v1/experiments/{experimentId}/items [post]
func (h *ExperimentHandler) CreateItems(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid UUID"))
		return
	}

	var req evaluationDomain.CreateExperimentItemsBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	count, err := h.itemService.CreateBatch(c.Request.Context(), experimentID, projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("experiment items created",
		"experiment_id", experimentID,
		"project_id", projectID,
		"count", count,
	)

	response.Created(c, &SDKBatchCreateExperimentItemsResponse{Created: count})
}

// @Summary Re-run experiment
// @Description Creates a new experiment based on an existing one, using the same dataset.
// @Description The new experiment starts in pending status, ready for the SDK to run with a new task function.
// @Tags Experiments, SDK - Experiments
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param experimentId path string true "Source Experiment ID"
// @Param request body evaluation.RerunExperimentRequest false "Optional overrides for name, description, metadata"
// @Success 201 {object} evaluation.ExperimentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Source experiment not found"
// @Router /api/v1/projects/{projectId}/experiments/{experimentId}/rerun [post]
// @Router /v1/experiments/{experimentId}/rerun [post]
func (h *ExperimentHandler) Rerun(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid UUID"))
		return
	}

	var req evaluationDomain.RerunExperimentRequest
	// Allow empty body - all fields are optional
	_ = c.ShouldBindJSON(&req)

	experiment, err := h.service.Rerun(c.Request.Context(), experimentID, projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("experiment rerun created",
		"experiment_id", experiment.ID,
		"source_experiment_id", experimentID,
		"project_id", projectID,
		"name", experiment.Name,
	)

	response.Created(c, experiment.ToResponse())
}

// @Summary Get experiment progress
// @Description Returns current progress for an experiment including item counts and ETA.
// @Tags Experiments
// @Produce json
// @Param projectId path string true "Project ID"
// @Param experimentId path string true "Experiment ID"
// @Success 200 {object} evaluation.ExperimentProgressResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/experiments/{experimentId}/progress [get]
func (h *ExperimentHandler) GetProgress(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid UUID"))
		return
	}

	progress, err := h.service.GetProgress(c.Request.Context(), experimentID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, progress)
}

// @Summary Get experiment metrics
// @Description Returns comprehensive metrics for an experiment including progress, performance, and score aggregations.
// @Tags Experiments
// @Produce json
// @Param projectId path string true "Project ID"
// @Param experimentId path string true "Experiment ID"
// @Success 200 {object} evaluation.ExperimentMetricsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/experiments/{experimentId}/metrics [get]
func (h *ExperimentHandler) GetMetrics(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid UUID"))
		return
	}

	metrics, err := h.service.GetMetrics(c.Request.Context(), projectID, experimentID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, metrics)
}
