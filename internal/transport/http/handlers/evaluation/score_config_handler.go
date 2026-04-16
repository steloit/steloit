// Package evaluation provides HTTP handlers for evaluation domain operations
// including score configuration management and SDK score ingestion.
package evaluation

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

type ScoreConfigHandler struct {
	logger  *slog.Logger
	service evaluationDomain.ScoreConfigService
}

func NewScoreConfigHandler(
	logger *slog.Logger,
	service evaluationDomain.ScoreConfigService,
) *ScoreConfigHandler {
	return &ScoreConfigHandler{
		logger:  logger,
		service: service,
	}
}

// @Summary Create score config
// @Description Creates a new score configuration for the project. Score configs define validation rules for scores.
// @Tags Score Configs
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param request body CreateRequest true "Score config request"
// @Success 201 {object} evaluation.ScoreConfigResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/score-configs [post]
func (h *ScoreConfigHandler) Create(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &evaluationDomain.CreateScoreConfigRequest{
		Name:        req.Name,
		Description: req.Description,
		Type:        evaluationDomain.ScoreType(req.Type),
		MinValue:    req.MinValue,
		MaxValue:    req.MaxValue,
		Categories:  req.Categories,
		Metadata:    req.Metadata,
	}

	config, err := h.service.Create(c.Request.Context(), projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, config.ToResponse())
}

// @Summary List score configs
// @Description Returns all score configurations for the project with pagination.
// @Tags Score Configs
// @Produce json
// @Param projectId path string true "Project ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 50)"
// @Success 200 {object} response.ListResponse{data=[]evaluation.ScoreConfigResponse}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/score-configs [get]
func (h *ScoreConfigHandler) List(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	params := response.ParsePaginationParams(c.Query("page"), c.Query("limit"), "", "")

	configs, total, err := h.service.List(c.Request.Context(), projectID, params.Page, params.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*evaluationDomain.ScoreConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = config.ToResponse()
	}

	pag := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, pag)
}

// @Summary Get score config
// @Description Returns the score configuration for a specific config ID.
// @Tags Score Configs
// @Produce json
// @Param projectId path string true "Project ID"
// @Param configId path string true "Score Config ID"
// @Success 200 {object} evaluation.ScoreConfigResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/score-configs/{configId} [get]
func (h *ScoreConfigHandler) Get(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	configID, err := uuid.Parse(c.Param("configId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid score config ID", "configId must be a valid UUID"))
		return
	}

	config, err := h.service.GetByID(c.Request.Context(), configID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, config.ToResponse())
}

// @Summary Update score config
// @Description Updates an existing score configuration by ID.
// @Tags Score Configs
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param configId path string true "Score Config ID"
// @Param request body UpdateRequest true "Update request"
// @Success 200 {object} evaluation.ScoreConfigResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/score-configs/{configId} [put]
func (h *ScoreConfigHandler) Update(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	configID, err := uuid.Parse(c.Param("configId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid score config ID", "configId must be a valid UUID"))
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var scoreType *evaluationDomain.ScoreType
	if req.Type != nil {
		st := evaluationDomain.ScoreType(*req.Type)
		scoreType = &st
	}

	domainReq := &evaluationDomain.UpdateScoreConfigRequest{
		Name:        req.Name,
		Description: req.Description,
		Type:        scoreType,
		MinValue:    req.MinValue,
		MaxValue:    req.MaxValue,
		Categories:  req.Categories,
		Metadata:    req.Metadata,
	}

	config, err := h.service.Update(c.Request.Context(), configID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, config.ToResponse())
}

// @Summary Delete score config
// @Description Removes a score configuration by its ID.
// @Tags Score Configs
// @Produce json
// @Param projectId path string true "Project ID"
// @Param configId path string true "Score Config ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/score-configs/{configId} [delete]
func (h *ScoreConfigHandler) Delete(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	configID, err := uuid.Parse(c.Param("configId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid score config ID", "configId must be a valid UUID"))
		return
	}

	if err := h.service.Delete(c.Request.Context(), configID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}
