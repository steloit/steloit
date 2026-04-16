package comment

import (
	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	commentDomain "brokle/internal/core/domain/comment"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

type Handler struct {
	service commentDomain.Service
}

func NewHandler(service commentDomain.Service) *Handler {
	return &Handler{
		service: service,
	}
}

// @Summary Create comment
// @Description Creates a new comment on a trace.
// @Tags Comments
// @Accept json
// @Produce json
// @Param id path string true "Trace ID"
// @Param project_id query string true "Project ID"
// @Param request body comment.CreateCommentRequest true "Comment content"
// @Success 201 {object} comment.CommentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Trace not found"
// @Router /api/v1/traces/{id}/comments [post]
func (h *Handler) CreateComment(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Error(c, appErrors.NewUnauthorizedError("authentication required"))
		return
	}

	var req commentDomain.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	createdComment, err := h.service.CreateComment(c.Request.Context(), projectID, traceID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, createdComment)
}

// @Summary List comments
// @Description Returns all comments for a trace with user information.
// @Tags Comments
// @Produce json
// @Param id path string true "Trace ID"
// @Param project_id query string true "Project ID"
// @Success 200 {object} comment.ListCommentsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/traces/{id}/comments [get]
func (h *Handler) ListComments(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	// Get current user ID if available (for reaction hasUser flag)
	var currentUserID *uuid.UUID
	if userID, exists := middleware.GetUserIDFromContext(c); exists {
		currentUserID = &userID
	}

	result, err := h.service.ListComments(c.Request.Context(), projectID, traceID, currentUserID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// @Summary Get comment count
// @Description Returns the count of comments for a trace.
// @Tags Comments
// @Produce json
// @Param id path string true "Trace ID"
// @Param project_id query string true "Project ID"
// @Success 200 {object} comment.CommentCountResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/traces/{id}/comments/count [get]
func (h *Handler) GetCommentCount(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	result, err := h.service.GetCommentCount(c.Request.Context(), projectID, traceID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// @Summary Update comment
// @Description Updates an existing comment. Only the owner can update their comment.
// @Tags Comments
// @Accept json
// @Produce json
// @Param id path string true "Trace ID"
// @Param comment_id path string true "Comment ID"
// @Param project_id query string true "Project ID"
// @Param request body comment.UpdateCommentRequest true "Updated content"
// @Success 200 {object} comment.CommentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse "Not the comment owner"
// @Failure 404 {object} response.ErrorResponse "Comment not found"
// @Router /api/v1/traces/{id}/comments/{comment_id} [put]
func (h *Handler) UpdateComment(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	commentID, err := uuid.Parse(c.Param("comment_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid comment ID", "comment_id must be a valid UUID"))
		return
	}

	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Error(c, appErrors.NewUnauthorizedError("authentication required"))
		return
	}

	var req commentDomain.UpdateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	comment, err := h.service.UpdateComment(c.Request.Context(), projectID, traceID, commentID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, comment)
}

// @Summary Delete comment
// @Description Deletes a comment. Only the owner can delete their comment.
// @Tags Comments
// @Produce json
// @Param id path string true "Trace ID"
// @Param comment_id path string true "Comment ID"
// @Param project_id query string true "Project ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse "Not the comment owner"
// @Failure 404 {object} response.ErrorResponse "Comment not found"
// @Router /api/v1/traces/{id}/comments/{comment_id} [delete]
func (h *Handler) DeleteComment(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	commentID, err := uuid.Parse(c.Param("comment_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid comment ID", "comment_id must be a valid UUID"))
		return
	}

	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Error(c, appErrors.NewUnauthorizedError("authentication required"))
		return
	}

	if err := h.service.DeleteComment(c.Request.Context(), projectID, traceID, commentID, userID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Toggle reaction
// @Description Adds a reaction if it doesn't exist, removes it if it does.
// @Tags Comments
// @Accept json
// @Produce json
// @Param id path string true "Trace ID"
// @Param comment_id path string true "Comment ID"
// @Param project_id query string true "Project ID"
// @Param request body comment.ToggleReactionRequest true "Reaction emoji"
// @Success 200 {array} comment.ReactionSummary
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Comment not found"
// @Router /api/v1/traces/{id}/comments/{comment_id}/reactions [post]
func (h *Handler) ToggleReaction(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	commentID, err := uuid.Parse(c.Param("comment_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid comment ID", "comment_id must be a valid UUID"))
		return
	}

	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Error(c, appErrors.NewUnauthorizedError("authentication required"))
		return
	}

	var req commentDomain.ToggleReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	reactions, err := h.service.ToggleReaction(c.Request.Context(), projectID, traceID, commentID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, reactions)
}

// @Summary Create reply
// @Description Creates a reply to an existing top-level comment. Replies cannot have replies (one level deep only).
// @Tags Comments
// @Accept json
// @Produce json
// @Param id path string true "Trace ID"
// @Param comment_id path string true "Parent Comment ID"
// @Param project_id query string true "Project ID"
// @Param request body comment.CreateCommentRequest true "Reply content"
// @Success 201 {object} comment.CommentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Comment or Trace not found"
// @Router /api/v1/traces/{id}/comments/{comment_id}/replies [post]
func (h *Handler) CreateReply(c *gin.Context) {
	traceID := c.Param("id")
	if traceID == "" {
		response.Error(c, appErrors.NewValidationError("Missing trace ID", "id is required"))
		return
	}

	parentID, err := uuid.Parse(c.Param("comment_id"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid comment ID", "comment_id must be a valid UUID"))
		return
	}

	projectIDStr := c.Query("project_id")
	if projectIDStr == "" {
		response.Error(c, appErrors.NewValidationError("Missing project ID", "project_id is required"))
		return
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Error(c, appErrors.NewUnauthorizedError("authentication required"))
		return
	}

	var req commentDomain.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	reply, err := h.service.CreateReply(c.Request.Context(), projectID, traceID, parentID, userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, reply)
}
