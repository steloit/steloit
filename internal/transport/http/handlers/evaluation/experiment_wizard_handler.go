package evaluation

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type ExperimentWizardHandler struct {
	logger  *slog.Logger
	service evaluationDomain.ExperimentWizardService
}

func NewExperimentWizardHandler(
	logger *slog.Logger,
	service evaluationDomain.ExperimentWizardService,
) *ExperimentWizardHandler {
	return &ExperimentWizardHandler{
		logger:  logger,
		service: service,
	}
}

// @Summary Create experiment from wizard
// @Description Creates a new experiment from the dashboard wizard configuration
// @Tags Experiments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body evaluation.CreateExperimentFromWizardRequest true "Wizard configuration"
// @Success 201 {object} evaluation.ExperimentResponse
// @Failure 400 {object} response.ErrorResponse "Validation error"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 404 {object} response.ErrorResponse "Prompt, dataset, or version not found"
// @Router /api/v1/projects/{projectId}/experiments/wizard [post]
func (h *ExperimentWizardHandler) CreateFromWizard(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Get optional user ID for audit purposes
	var userID *ulid.ULID
	if uid, exists := middleware.GetUserIDULID(c); exists {
		userID = &uid
	}

	var req evaluationDomain.CreateExperimentFromWizardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	experiment, err := h.service.CreateFromWizard(c.Request.Context(), projectID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, experiment.ToResponse())
}

// @Summary Validate wizard step
// @Description Validates a specific step of the experiment wizard
// @Tags Experiments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body evaluation.ValidateStepRequest true "Step data to validate"
// @Success 200 {object} evaluation.ValidateStepResponse
// @Failure 400 {object} response.ErrorResponse "Validation error"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Router /api/v1/projects/{projectId}/experiments/wizard/validate [post]
func (h *ExperimentWizardHandler) ValidateStep(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req evaluationDomain.ValidateStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	result, err := h.service.ValidateStep(c.Request.Context(), projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// @Summary Estimate experiment cost
// @Description Estimates the cost of running an experiment with the given configuration
// @Tags Experiments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body evaluation.EstimateCostRequest true "Cost estimation parameters"
// @Success 200 {object} evaluation.EstimateCostResponse
// @Failure 400 {object} response.ErrorResponse "Validation error"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 404 {object} response.ErrorResponse "Dataset not found"
// @Router /api/v1/projects/{projectId}/experiments/wizard/estimate [post]
func (h *ExperimentWizardHandler) EstimateCost(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req evaluationDomain.EstimateCostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	result, err := h.service.EstimateCost(c.Request.Context(), projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// @Summary Get dataset fields
// @Description Returns the schema of dataset fields for variable mapping in the wizard
// @Tags Experiments, Datasets
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Success 200 {object} evaluation.DatasetFieldsResponse
// @Failure 400 {object} response.ErrorResponse "Validation error"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 404 {object} response.ErrorResponse "Dataset not found"
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/fields [get]
func (h *ExperimentWizardHandler) GetDatasetFields(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	result, err := h.service.GetDatasetFields(c.Request.Context(), projectID, datasetID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// @Summary Get experiment config
// @Description Returns the wizard configuration for an experiment created via the dashboard
// @Tags Experiments
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param experimentId path string true "Experiment ID"
// @Success 200 {object} evaluation.ExperimentConfigResponse
// @Failure 400 {object} response.ErrorResponse "Validation error"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 404 {object} response.ErrorResponse "Experiment or config not found"
// @Router /api/v1/projects/{projectId}/experiments/{experimentId}/config [get]
func (h *ExperimentWizardHandler) GetExperimentConfig(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	experimentID, err := ulid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid ULID"))
		return
	}

	config, err := h.service.GetExperimentConfig(c.Request.Context(), experimentID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, config.ToResponse())
}
