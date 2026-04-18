package playground

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	playgroundDomain "brokle/internal/core/domain/playground"
	prompt "brokle/internal/core/domain/prompt"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// SDKPlaygroundHandler handles playground operations for SDK routes.
// Uses API key authentication instead of JWT.
type SDKPlaygroundHandler struct {
	logger            *slog.Logger
	playgroundService playgroundDomain.PlaygroundService
}

func NewSDKPlaygroundHandler(
	logger *slog.Logger,
	playgroundService playgroundDomain.PlaygroundService,
) *SDKPlaygroundHandler {
	return &SDKPlaygroundHandler{
		logger:            logger,
		playgroundService: playgroundService,
	}
}

// SDKExecuteRequest represents a playground execution request from SDK.
// Project ID is derived from API key authentication, not from request body.
type SDKExecuteRequest struct {
	Template        any         `json:"template" binding:"required"`
	PromptType      prompt.PromptType   `json:"prompt_type" binding:"required"`
	Variables       map[string]string   `json:"variables"`
	ConfigOverrides *prompt.ModelConfig `json:"config_overrides"`
}

// Execute handles POST /v1/playground/execute
// @Summary Execute a prompt via SDK
// @Description Executes a prompt template with variables using project credentials from API key.
// @Tags SDK - Playground
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body SDKExecuteRequest true "Execution request"
// @Success 200 {object} playground.ExecuteResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /v1/playground/execute [post]
func (h *SDKPlaygroundHandler) Execute(c *gin.Context) {
	projectID := middleware.MustGetProjectID(c)

	var req SDKExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &playgroundDomain.ExecuteRequest{
		ProjectID:       projectID,
		SessionID:       nil, // SDK doesn't use sessions
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

	h.logger.Info("prompt executed via SDK",
		"project_id", projectID,
		"prompt_type", req.PromptType,
	)

	response.Success(c, resp)
}
