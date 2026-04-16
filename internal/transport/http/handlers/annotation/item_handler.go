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

// ItemHandler handles annotation queue item HTTP endpoints.
type ItemHandler struct {
	logger            *slog.Logger
	service           annotationDomain.ItemService
	assignmentService annotationDomain.AssignmentService
}

// NewItemHandler creates a new ItemHandler.
func NewItemHandler(
	logger *slog.Logger,
	service annotationDomain.ItemService,
	assignmentService annotationDomain.AssignmentService,
) *ItemHandler {
	return &ItemHandler{
		logger:            logger,
		service:           service,
		assignmentService: assignmentService,
	}
}

// @Summary Add items to annotation queue
// @Description Adds items (traces or spans) to an annotation queue for human review.
// @Tags Annotation Queue Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param request body AddItemsBatchRequest true "Items to add"
// @Success 201 {object} BatchAddItemsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Queue not found"
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/items [post]
func (h *ItemHandler) AddItems(c *gin.Context) {
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

	var req AddItemsBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &annotationDomain.AddItemsBatchRequest{
		Items: make([]annotationDomain.AddItemRequest, len(req.Items)),
	}
	for i, item := range req.Items {
		domainReq.Items[i] = annotationDomain.AddItemRequest{
			ObjectID:   item.ObjectID,
			ObjectType: annotationDomain.ObjectType(item.ObjectType),
			Priority:   item.Priority,
			Metadata:   item.Metadata,
		}
	}

	count, err := h.service.AddItems(c.Request.Context(), queueID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("items added to annotation queue",
		"queue_id", queueID,
		"project_id", projectID,
		"count", count,
	)

	response.Created(c, &BatchAddItemsResponse{Created: count})
}

// @Summary List annotation queue items
// @Description Returns items in an annotation queue with optional filtering.
// @Tags Annotation Queue Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 50)"
// @Param status query string false "Filter by status (pending, completed, skipped)"
// @Success 200 {object} response.ListResponse{data=[]ItemResponse}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/items [get]
func (h *ItemHandler) ListItems(c *gin.Context) {
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

	params := response.ParsePaginationParams(c.Query("page"), c.Query("limit"), "", "")

	filter := &annotationDomain.ItemFilter{
		Limit:  params.Limit,
		Offset: (params.Page - 1) * params.Limit,
	}

	if status := c.Query("status"); status != "" {
		itemStatus := annotationDomain.ItemStatus(status)
		filter.Status = &itemStatus
	}

	items, total, err := h.service.ListItems(c.Request.Context(), queueID, projectID, filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*ItemResponse, len(items))
	for i, item := range items {
		responses[i] = toItemResponse(item)
	}

	pag := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, pag)
}

// @Summary Claim next annotation item
// @Description Claims the next available item for annotation. Uses 5-minute lock lease.
// @Tags Annotation Queue Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param request body ClaimNextRequest false "Optional seen item IDs to exclude"
// @Success 200 {object} ItemResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Queue not found or no items available"
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/items/claim [post]
func (h *ItemHandler) ClaimNext(c *gin.Context) {
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

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Unauthorized(c, "User authentication required")
		return
	}

	// Check user has at least annotator role on this queue
	if err := h.assignmentService.CheckAccess(
		c.Request.Context(),
		queueID,
		userID,
		annotationDomain.RoleAnnotator,
	); err != nil {
		response.Error(c, err)
		return
	}

	var req ClaimNextRequest
	_ = c.ShouldBindJSON(&req) // Ignore errors - body is optional

	var seenItemIDs []uuid.UUID
	if len(req.SeenItemIDs) > 0 {
		seenItemIDs = make([]uuid.UUID, 0, len(req.SeenItemIDs))
		for _, idStr := range req.SeenItemIDs {
			if id, err := uuid.Parse(idStr); err == nil {
				seenItemIDs = append(seenItemIDs, id)
			}
		}
	}

	item, err := h.service.ClaimNext(c.Request.Context(), queueID, projectID, userID, seenItemIDs)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, toItemResponse(item))
}

// @Summary Complete annotation item
// @Description Marks an item as completed and optionally submits scores.
// @Tags Annotation Queue Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param itemId path string true "Item ID"
// @Param request body CompleteItemRequest false "Optional scores to submit"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse "Item locked by another user"
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/items/{itemId}/complete [post]
func (h *ItemHandler) Complete(c *gin.Context) {
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

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid item ID", "itemId must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Unauthorized(c, "User authentication required")
		return
	}

	// Check user has at least annotator role on this queue
	if err := h.assignmentService.CheckAccess(
		c.Request.Context(),
		queueID,
		userID,
		annotationDomain.RoleAnnotator,
	); err != nil {
		response.Error(c, err)
		return
	}

	var req CompleteItemRequest
	_ = c.ShouldBindJSON(&req) // Ignore errors - body is optional

	domainReq := &annotationDomain.CompleteItemRequest{
		Scores: make([]annotationDomain.ScoreSubmission, len(req.Scores)),
	}
	for i, score := range req.Scores {
		domainReq.Scores[i] = annotationDomain.ScoreSubmission{
			ScoreConfigID: score.ScoreConfigID,
			Value:         score.Value,
			Comment:       score.Comment,
		}
	}

	if err := h.service.Complete(c.Request.Context(), itemID, queueID, projectID, userID, domainReq); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Skip annotation item
// @Description Marks an item as skipped with an optional reason.
// @Tags Annotation Queue Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param itemId path string true "Item ID"
// @Param request body SkipItemRequest false "Optional skip reason"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse "Item locked by another user"
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/items/{itemId}/skip [post]
func (h *ItemHandler) Skip(c *gin.Context) {
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

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid item ID", "itemId must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Unauthorized(c, "User authentication required")
		return
	}

	// Check user has at least annotator role on this queue
	if err := h.assignmentService.CheckAccess(
		c.Request.Context(),
		queueID,
		userID,
		annotationDomain.RoleAnnotator,
	); err != nil {
		response.Error(c, err)
		return
	}

	var req SkipItemRequest
	_ = c.ShouldBindJSON(&req) // Ignore errors - body is optional

	domainReq := &annotationDomain.SkipItemRequest{
		Reason: req.Reason,
	}

	if err := h.service.Skip(c.Request.Context(), itemID, queueID, projectID, userID, domainReq); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Release item lock
// @Description Releases the lock on an item, returning it to the pending pool.
// @Tags Annotation Queue Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param itemId path string true "Item ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse "Item locked by another user"
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/items/{itemId}/release [post]
func (h *ItemHandler) ReleaseLock(c *gin.Context) {
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

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid item ID", "itemId must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Unauthorized(c, "User authentication required")
		return
	}

	// Check user has at least annotator role on this queue
	if err := h.assignmentService.CheckAccess(
		c.Request.Context(),
		queueID,
		userID,
		annotationDomain.RoleAnnotator,
	); err != nil {
		response.Error(c, err)
		return
	}

	if err := h.service.ReleaseLock(c.Request.Context(), itemID, queueID, projectID, userID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Delete annotation item
// @Description Removes an item from the annotation queue.
// @Tags Annotation Queue Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param itemId path string true "Item ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/items/{itemId} [delete]
func (h *ItemHandler) DeleteItem(c *gin.Context) {
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

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid item ID", "itemId must be a valid UUID"))
		return
	}

	if err := h.service.DeleteItem(c.Request.Context(), itemID, queueID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Get queue statistics
// @Description Returns statistics for an annotation queue.
// @Tags Annotation Queue Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Success 200 {object} StatsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/stats [get]
func (h *ItemHandler) GetStats(c *gin.Context) {
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

	stats, err := h.service.GetStats(c.Request.Context(), queueID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, toStatsResponse(stats))
}

// ============================================================================
// SDK Handlers (API Key Auth - projectId from auth context)
// ============================================================================

// @Summary Add items to annotation queue (SDK)
// @Description Adds items (traces or spans) to an annotation queue for human review. Project determined by API key.
// @Tags SDK - Annotation Queues
// @Accept json
// @Produce json
// @Param queueId path string true "Queue ID"
// @Param request body AddItemsBatchRequest true "Items to add"
// @Security ApiKeyAuth
// @Success 201 {object} BatchAddItemsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Queue not found"
// @Router /v1/annotation-queues/{queueId}/items [post]
func (h *ItemHandler) AddItemsSDK(c *gin.Context) {
	projectIDPtr, exists := middleware.GetProjectID(c)
	if !exists || projectIDPtr == nil {
		response.Unauthorized(c, "API key authentication required")
		return
	}
	projectID := *projectIDPtr

	queueID, err := uuid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid UUID"))
		return
	}

	var req AddItemsBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &annotationDomain.AddItemsBatchRequest{
		Items: make([]annotationDomain.AddItemRequest, len(req.Items)),
	}
	for i, item := range req.Items {
		domainReq.Items[i] = annotationDomain.AddItemRequest{
			ObjectID:   item.ObjectID,
			ObjectType: annotationDomain.ObjectType(item.ObjectType),
			Priority:   item.Priority,
			Metadata:   item.Metadata,
		}
	}

	count, err := h.service.AddItems(c.Request.Context(), queueID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("items added to annotation queue via SDK",
		"queue_id", queueID,
		"project_id", projectID,
		"count", count,
	)

	response.Created(c, &BatchAddItemsResponse{Created: count})
}

// @Summary List annotation queue items (SDK)
// @Description Returns items in an annotation queue with optional filtering. Project determined by API key.
// @Tags SDK - Annotation Queues
// @Produce json
// @Param queueId path string true "Queue ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 50)"
// @Param status query string false "Filter by status (pending, completed, skipped)"
// @Security ApiKeyAuth
// @Success 200 {object} response.ListResponse{data=[]ItemResponse}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /v1/annotation-queues/{queueId}/items [get]
func (h *ItemHandler) ListItemsSDK(c *gin.Context) {
	projectIDPtr, exists := middleware.GetProjectID(c)
	if !exists || projectIDPtr == nil {
		response.Unauthorized(c, "API key authentication required")
		return
	}
	projectID := *projectIDPtr

	queueID, err := uuid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid UUID"))
		return
	}

	params := response.ParsePaginationParams(c.Query("page"), c.Query("limit"), "", "")

	filter := &annotationDomain.ItemFilter{
		Limit:  params.Limit,
		Offset: (params.Page - 1) * params.Limit,
	}

	if status := c.Query("status"); status != "" {
		itemStatus := annotationDomain.ItemStatus(status)
		filter.Status = &itemStatus
	}

	items, total, err := h.service.ListItems(c.Request.Context(), queueID, projectID, filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*ItemResponse, len(items))
	for i, item := range items {
		responses[i] = toItemResponse(item)
	}

	pag := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, pag)
}

// Helper function to convert domain item to response

func toItemResponse(item *annotationDomain.QueueItem) *ItemResponse {
	resp := &ItemResponse{
		ID:         item.ID.String(),
		QueueID:    item.QueueID.String(),
		ObjectID:   item.ObjectID,
		ObjectType: string(item.ObjectType),
		Status:     string(item.Status),
		Priority:   item.Priority,
		LockedAt:   item.LockedAt,
		Metadata:   item.Metadata,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
	if item.LockedByUserID != nil {
		lockedBy := item.LockedByUserID.String()
		resp.LockedByUserID = &lockedBy
	}
	if item.AnnotatorUserID != nil {
		annotator := item.AnnotatorUserID.String()
		resp.AnnotatorUserID = &annotator
	}
	if item.CompletedAt != nil {
		resp.CompletedAt = item.CompletedAt
	}
	return resp
}
