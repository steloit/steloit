package playground

import (
	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	playgroundDomain "brokle/internal/core/domain/playground"
	prompt "brokle/internal/core/domain/prompt"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// ExecuteRequest represents a playground execution request.
type ExecuteRequest struct {
	Template        interface{}         `json:"template" binding:"required"`
	PromptType      prompt.PromptType   `json:"prompt_type" binding:"required"`
	Variables       map[string]string   `json:"variables"`
	ConfigOverrides *prompt.ModelConfig `json:"config_overrides"`
	SessionID       *string             `json:"session_id,omitempty"` // Optional: updates session's last_run
	ProjectID       *string             `json:"project_id"`           // Required: for session access validation
}

// Execute handles POST /api/v1/playground/execute
// @Summary Execute a prompt in playground
// @Description Executes a prompt template with variables. Optionally updates session's last_run if session_id provided.
// @Tags Playground
// @Accept json
// @Produce json
// @Param request body ExecuteRequest true "Execution request"
// @Success 200 {object} playground.ExecuteResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/playground/execute [post]
func (h *Handler) Execute(c *gin.Context) {
	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validate project access before using project credentials
	if err := h.validateProjectAccess(c, req.ProjectID); err != nil {
		response.Error(c, err)
		return
	}

	if err := h.validateSessionAccess(c, req.SessionID); err != nil {
		response.Error(c, err)
		return
	}

	projectID, err := uuid.Parse(*req.ProjectID)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
		return
	}

	// Derive organization ID from project (don't trust client-provided org ID)
	project, err := h.projectService.GetProject(c.Request.Context(), projectID)
	if err != nil {
		response.Error(c, err)
		return
	}
	organizationID := project.OrganizationID

	var sessionID *uuid.UUID
	if req.SessionID != nil {
		sid, err := uuid.Parse(*req.SessionID)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid session ID", "session_id must be a valid UUID"))
			return
		}
		sessionID = &sid
	}

	domainReq := &playgroundDomain.ExecuteRequest{
		ProjectID:       projectID,
		OrganizationID:  organizationID,
		SessionID:       sessionID,
		Template:        req.Template,
		PromptType:      req.PromptType,
		Variables:       req.Variables,
		ConfigOverrides: req.ConfigOverrides,
	}

	resp, err := h.playgroundService.ExecutePrompt(c.Request.Context(), domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, resp)
}
