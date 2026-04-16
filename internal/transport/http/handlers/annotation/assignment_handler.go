package annotation

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	annotationDomain "brokle/internal/core/domain/annotation"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// AssignmentHandler handles annotation queue assignment HTTP endpoints.
type AssignmentHandler struct {
	logger  *slog.Logger
	service annotationDomain.AssignmentService
}

// NewAssignmentHandler creates a new AssignmentHandler.
func NewAssignmentHandler(logger *slog.Logger, service annotationDomain.AssignmentService) *AssignmentHandler {
	return &AssignmentHandler{
		logger:  logger,
		service: service,
	}
}

// @Summary Assign user to annotation queue
// @Description Assigns a user to an annotation queue with the specified role.
// @Tags Annotation Queue Assignments
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param request body AssignUserRequest true "Assignment request"
// @Success 201 {object} AssignmentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Queue not found"
// @Failure 409 {object} response.ErrorResponse "User already assigned"
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/assignments [post]
func (h *AssignmentHandler) Assign(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	queueID, err := ulid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid ULID"))
		return
	}

	var req AssignUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userIDToAssign, err := ulid.Parse(req.UserID)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("user_id", "must be a valid ULID"))
		return
	}

	// Get assigner's user ID
	assignedByID, exists := middleware.GetUserIDULID(c)
	var assignedByPtr *ulid.ULID
	if exists {
		assignedByPtr = &assignedByID
	}

	role := annotationDomain.AssignmentRole(req.Role)

	assignment, err := h.service.Assign(c.Request.Context(), queueID, projectID, userIDToAssign, role, assignedByPtr)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("user assigned to annotation queue",
		"assignment_id", assignment.ID,
		"queue_id", queueID,
		"user_id", userIDToAssign,
		"role", role,
	)

	response.Created(c, toAssignmentResponse(assignment))
}

// @Summary List queue assignments
// @Description Returns all user assignments for an annotation queue.
// @Tags Annotation Queue Assignments
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Success 200 {array} AssignmentResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/assignments [get]
func (h *AssignmentHandler) List(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	queueID, err := ulid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid ULID"))
		return
	}

	assignments, err := h.service.ListAssignments(c.Request.Context(), queueID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*AssignmentResponse, len(assignments))
	for i, assignment := range assignments {
		responses[i] = toAssignmentResponse(assignment)
	}

	response.Success(c, responses)
}

// @Summary Remove user from annotation queue
// @Description Removes a user's assignment from an annotation queue.
// @Tags Annotation Queue Assignments
// @Produce json
// @Param projectId path string true "Project ID"
// @Param queueId path string true "Queue ID"
// @Param userId path string true "User ID to unassign"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse "Queue or assignment not found"
// @Router /api/v1/projects/{projectId}/annotation-queues/{queueId}/assignments/{userId} [delete]
func (h *AssignmentHandler) Unassign(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	queueID, err := ulid.Parse(c.Param("queueId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid queue ID", "queueId must be a valid ULID"))
		return
	}

	userID, err := ulid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid user ID", "userId must be a valid ULID"))
		return
	}

	if err := h.service.Unassign(c.Request.Context(), queueID, projectID, userID); err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("user unassigned from annotation queue",
		"queue_id", queueID,
		"user_id", userID,
	)

	response.NoContent(c)
}

// @Summary Get user's queue assignments
// @Description Returns all annotation queues a user is assigned to.
// @Tags Annotation Queue Assignments
// @Produce json
// @Success 200 {array} AssignmentResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/annotation-queues/my-assignments [get]
func (h *AssignmentHandler) GetMyAssignments(c *gin.Context) {
	userID, exists := middleware.GetUserIDULID(c)
	if !exists {
		response.Unauthorized(c, "User authentication required")
		return
	}

	assignments, err := h.service.GetUserQueues(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*AssignmentResponse, len(assignments))
	for i, assignment := range assignments {
		responses[i] = toAssignmentResponse(assignment)
	}

	response.Success(c, responses)
}

// Helper function to convert domain assignment to response

func toAssignmentResponse(assignment *annotationDomain.QueueAssignment) *AssignmentResponse {
	resp := &AssignmentResponse{
		ID:         assignment.ID.String(),
		QueueID:    assignment.QueueID.String(),
		UserID:     assignment.UserID.String(),
		Role:       string(assignment.Role),
		AssignedAt: assignment.AssignedAt,
	}
	if assignment.AssignedBy != nil {
		assignedBy := assignment.AssignedBy.String()
		resp.AssignedBy = &assignedBy
	}
	return resp
}
