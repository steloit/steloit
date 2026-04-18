package annotation

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	annotationDomain "brokle/internal/core/domain/annotation"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// QueueHandler handles annotation queue HTTP endpoints.
type QueueHandler struct {
	logger  *slog.Logger
	service annotationDomain.QueueService
}

// NewQueueHandler creates a new QueueHandler.
func NewQueueHandler(logger *slog.Logger, service annotationDomain.QueueService) *QueueHandler {
	return &QueueHandler{
		logger:  logger,
		service: service,
	}
}

// @Summary Create annotation queue
// @Description Creates a new annotation queue for the project.
// @Tags Annotation Queues
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param request body CreateQueueRequest true "Queue request"
// @Success 201 {object} QueueResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/annotation-queues [post]
func (h *QueueHandler) Create(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req CreateQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	var userIDPtr *uuid.UUID
	if exists {
		userIDPtr = &userID
	}

	domainReq := &annotationDomain.CreateQueueRequest{
		Name:           req.Name,
		Description:    req.Description,
		Instructions:   req.Instructions,
		ScoreConfigIDs: req.ScoreConfigIDs,
	}
	if req.Settings != nil {
		domainReq.Settings = &annotationDomain.QueueSettings{
			LockTimeoutSeconds: req.Settings.LockTimeoutSeconds,
			AutoAssignment:     req.Settings.AutoAssignment,
		}
	}

	queue, err := h.service.Create(c.Request.Context(), projectID, userIDPtr, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("annotation queue created",
		"queue_id", queue.ID,
		"project_id", projectID,
		"name", queue.Name,
	)

	response.Created(c, toQueueResponse(queue))
}

// @Summary List annotation queues
// @Description Returns all annotation queues for the project with their statistics.
// @Tags Annotation Queues
// @Produce json
// @Param projectId path string true "Project ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 50)"
// @Param status query string false "Filter by status (active, paused, archived)"
// @Param search query string false "Search by name or description"
// @Success 200 {object} response.ListResponse{data=[]QueueWithStatsResponse}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues [get]
func (h *QueueHandler) List(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	params := response.ParsePaginationParams(c.Query("page"), c.Query("limit"), "", "")

	filter := &annotationDomain.QueueFilter{}
	if status := c.Query("status"); status != "" {
		queueStatus := annotationDomain.QueueStatus(status)
		if queueStatus.IsValid() {
			filter.Status = &queueStatus
		}
	}

	if search := c.Query("search"); search != "" {
		filter.Search = &search
	}

	queues, stats, total, err := h.service.ListWithStats(c.Request.Context(), projectID, filter, params.Page, params.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*QueueWithStatsResponse, len(queues))
	for i, queue := range queues {
		responses[i] = &QueueWithStatsResponse{
			Queue: toQueueResponse(queue),
			Stats: toStatsResponse(stats[i]),
		}
	}

	pag := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, pag)
}

// @Summary Get annotation queue
// @Description Returns the annotation queue for a specific ID.
// @Tags Annotation Queues
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Success 200 {object} QueueResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId} [get]
func (h *QueueHandler) Get(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	queueID, err := uuid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid UUID"))
		return
	}

	queue, err := h.service.GetByID(c.Request.Context(), queueID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, toQueueResponse(queue))
}

// @Summary Get annotation queue with stats
// @Description Returns the annotation queue with its statistics.
// @Tags Annotation Queues
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Success 200 {object} QueueWithStatsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/stats [get]
func (h *QueueHandler) GetWithStats(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	queueID, err := uuid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid UUID"))
		return
	}

	queue, stats, err := h.service.GetWithStats(c.Request.Context(), queueID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, &QueueWithStatsResponse{
		Queue: toQueueResponse(queue),
		Stats: toStatsResponse(stats),
	})
}

// @Summary Update annotation queue
// @Description Updates an existing annotation queue by ID.
// @Tags Annotation Queues
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param request body UpdateQueueRequest true "Update request"
// @Success 200 {object} QueueResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId} [put]
func (h *QueueHandler) Update(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	queueID, err := uuid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid UUID"))
		return
	}

	var req UpdateQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &annotationDomain.UpdateQueueRequest{
		Name:           req.Name,
		Description:    req.Description,
		Instructions:   req.Instructions,
		ScoreConfigIDs: req.ScoreConfigIDs,
	}
	if req.Status != nil {
		status := annotationDomain.QueueStatus(*req.Status)
		domainReq.Status = &status
	}
	if req.Settings != nil {
		domainReq.Settings = &annotationDomain.QueueSettings{
			LockTimeoutSeconds: req.Settings.LockTimeoutSeconds,
			AutoAssignment:     req.Settings.AutoAssignment,
		}
	}

	queue, err := h.service.Update(c.Request.Context(), queueID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, toQueueResponse(queue))
}

// @Summary Delete annotation queue
// @Description Removes an annotation queue by its ID. Also deletes all items and assignments.
// @Tags Annotation Queues
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId} [delete]
func (h *QueueHandler) Delete(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	queueID, err := uuid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid UUID"))
		return
	}

	if err := h.service.Delete(c.Request.Context(), queueID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// Helper functions for converting domain types to response types

func toQueueResponse(queue *annotationDomain.AnnotationQueue) *QueueResponse {
	return &QueueResponse{
		ID:             queue.ID,
		ProjectID:      queue.ProjectID,
		Name:           queue.Name,
		Description:    queue.Description,
		Instructions:   queue.Instructions,
		ScoreConfigIDs: queue.ScoreConfigIDs,
		Status:         string(queue.Status),
		Settings: &QueueSettings{
			LockTimeoutSeconds: queue.Settings.LockTimeoutSeconds,
			AutoAssignment:     queue.Settings.AutoAssignment,
		},
		CreatedBy: queue.CreatedBy,
		CreatedAt: queue.CreatedAt,
		UpdatedAt: queue.UpdatedAt,
	}
}

func toStatsResponse(stats *annotationDomain.QueueStats) *StatsResponse {
	return &StatsResponse{
		TotalItems:      stats.TotalItems,
		PendingItems:    stats.PendingItems,
		InProgressItems: stats.InProgressItems,
		CompletedItems:  stats.CompletedItems,
		SkippedItems:    stats.SkippedItems,
	}
}
