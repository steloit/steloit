package playground

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"

	playgroundDomain "brokle/internal/core/domain/playground"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// CreateSessionRequest represents the request body for creating a session.
// All sessions are saved to the database.
type CreateSessionRequest struct {
	Name        string          `json:"name" binding:"required,max=200"`
	Description *string         `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty" binding:"omitempty,max=10,dive,max=50"`
	Variables   json.RawMessage `json:"variables,omitempty" swaggertype:"object"`
	Config      json.RawMessage `json:"config,omitempty" swaggertype:"object"`
	Windows     json.RawMessage `json:"windows" binding:"required" swaggertype:"object"`
}

// UpdateSessionRequest represents the request body for updating a session.
type UpdateSessionRequest struct {
	Name        *string         `json:"name,omitempty" binding:"omitempty,max=200"`
	Description *string         `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty" binding:"omitempty,max=10,dive,max=50"`
	Variables   json.RawMessage `json:"variables,omitempty" swaggertype:"object"`
	Config      json.RawMessage `json:"config,omitempty" swaggertype:"object"`
	Windows     json.RawMessage `json:"windows,omitempty" swaggertype:"object"`
}

// CreateSession creates a new playground session.
// @Summary Create playground session
// @Description Creates a new playground session with name and windows.
// @Tags Playground
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param request body CreateSessionRequest true "Session configuration"
// @Success 201 {object} playground.SessionResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/playground/sessions [post]
func (h *Handler) CreateSession(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userID, exists := middleware.GetUserIDULID(c)
	var userIDPtr *ulid.ULID
	if exists {
		userIDPtr = &userID
	}

	domainReq := &playgroundDomain.CreateSessionRequest{
		ProjectID:   projectID,
		CreatedBy:   userIDPtr,
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Variables:   req.Variables,
		Config:      req.Config,
		Windows:     req.Windows,
	}

	session, err := h.playgroundService.CreateSession(c.Request.Context(), domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, session)
}

// GetSession retrieves a session by ID.
// @Summary Get playground session
// @Description Retrieves a playground session by its ID.
// @Tags Playground
// @Produce json
// @Param projectId path string true "Project ID"
// @Param sessionId path string true "Session ID"
// @Success 200 {object} playground.SessionResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/playground/sessions/{sessionId} [get]
func (h *Handler) GetSession(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	sessionID, err := ulid.Parse(c.Param("sessionId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid session ID", "sessionId must be a valid ULID"))
		return
	}

	// Validate project access
	if err := h.playgroundService.ValidateProjectAccess(c.Request.Context(), sessionID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	session, err := h.playgroundService.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, session)
}

// ListSessions lists sessions for a project.
// @Summary List playground sessions
// @Description Lists all playground sessions for the sidebar.
// @Tags Playground
// @Produce json
// @Param projectId path string true "Project ID"
// @Param limit query int false "Maximum number of sessions to return (default 20, max 100)"
// @Param tags query []string false "Filter by tags (any match)"
// @Success 200 {array} playground.SessionSummary
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/playground/sessions [get]
func (h *Handler) ListSessions(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	tags := c.QueryArray("tags")

	domainReq := &playgroundDomain.ListSessionsRequest{
		ProjectID: projectID,
		Limit:     limit,
		Tags:      tags,
	}

	sessions, err := h.playgroundService.ListSessions(c.Request.Context(), domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, sessions)
}

// UpdateSession updates a session's content and metadata.
// @Summary Update playground session
// @Description Updates a session's name, description, tags, variables, config, or windows.
// @Tags Playground
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param sessionId path string true "Session ID"
// @Param request body UpdateSessionRequest true "Update fields"
// @Success 200 {object} playground.SessionResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/playground/sessions/{sessionId} [put]
func (h *Handler) UpdateSession(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	sessionID, err := ulid.Parse(c.Param("sessionId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid session ID", "sessionId must be a valid ULID"))
		return
	}

	// Validate project access
	if err := h.playgroundService.ValidateProjectAccess(c.Request.Context(), sessionID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	var req UpdateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &playgroundDomain.UpdateSessionRequest{
		SessionID:   sessionID,
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Variables:   req.Variables,
		Config:      req.Config,
		Windows:     req.Windows,
	}

	session, err := h.playgroundService.UpdateSession(c.Request.Context(), domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, session)
}

// DeleteSession removes a session.
// @Summary Delete playground session
// @Description Permanently deletes a playground session.
// @Tags Playground
// @Produce json
// @Param projectId path string true "Project ID"
// @Param sessionId path string true "Session ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/playground/sessions/{sessionId} [delete]
func (h *Handler) DeleteSession(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	sessionID, err := ulid.Parse(c.Param("sessionId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid session ID", "sessionId must be a valid ULID"))
		return
	}

	// Validate project access
	if err := h.playgroundService.ValidateProjectAccess(c.Request.Context(), sessionID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	if err := h.playgroundService.DeleteSession(c.Request.Context(), sessionID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}
