package playground

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	playgroundDomain "brokle/internal/core/domain/playground"
	prompt "brokle/internal/core/domain/prompt"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// StreamRequest represents a streaming execution request.
type StreamRequest struct {
	Template        interface{}         `json:"template" binding:"required"`
	PromptType      prompt.PromptType   `json:"prompt_type" binding:"required"`
	Variables       map[string]string   `json:"variables"`
	ConfigOverrides *prompt.ModelConfig `json:"config_overrides"`
	SessionID       *string             `json:"session_id,omitempty"` // Optional: updates session's last_run
	ProjectID       *string             `json:"project_id"`           // Required: for session access validation
}

// StreamChunk represents a streaming response chunk.
type StreamChunk struct {
	Type         string          `json:"type"` // "start", "content", "end", "error", "metrics"
	Content      string          `json:"content,omitempty"`
	Error        string          `json:"error,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
	Metrics      *StreamMetrics  `json:"metrics,omitempty"`
}

// StreamMetrics contains final execution metrics
type StreamMetrics struct {
	Model            string   `json:"model,omitempty"`
	PromptTokens     int      `json:"prompt_tokens,omitempty"`
	CompletionTokens int      `json:"completion_tokens,omitempty"`
	TotalTokens      int      `json:"total_tokens,omitempty"`
	Cost             *float64 `json:"cost,omitempty"`
	TTFTMs           *float64 `json:"ttft_ms,omitempty"`
	TotalDuration    int64    `json:"total_duration_ms,omitempty"`
}

// Stream handles POST /api/v1/playground/stream
// @Summary Execute a prompt with streaming
// @Description Streams prompt execution results via SSE. Optionally updates session's last_run if session_id provided.
// @Tags Playground
// @Accept json
// @Produce text/event-stream
// @Param request body StreamRequest true "Streaming execution request"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/playground/stream [post]
func (h *Handler) Stream(c *gin.Context) {
	var req StreamRequest
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

	projectID, err := ulid.Parse(*req.ProjectID)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid ULID"))
		return
	}

	// Derive organization ID from project (don't trust client-provided org ID)
	project, err := h.projectService.GetProject(c.Request.Context(), projectID)
	if err != nil {
		response.Error(c, err)
		return
	}
	organizationID := project.OrganizationID

	var sessionID *ulid.ULID
	if req.SessionID != nil {
		sid, err := ulid.Parse(*req.SessionID)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid session ID", "session_id must be a valid ULID"))
			return
		}
		sessionID = &sid
	}

	domainReq := &playgroundDomain.StreamRequest{
		ProjectID:       projectID,
		OrganizationID:  organizationID,
		SessionID:       sessionID,
		Template:        req.Template,
		PromptType:      req.PromptType,
		Variables:       req.Variables,
		ConfigOverrides: req.ConfigOverrides,
	}

	streamResp, err := h.playgroundService.StreamPrompt(c.Request.Context(), domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	ctx := c.Request.Context()

	for event := range streamResp.EventChan {
		select {
		case <-ctx.Done():
			return // Client disconnected
		default:
			chunk := StreamChunk{
				Type:         string(event.Type),
				Content:      event.Content,
				Error:        event.Error,
				FinishReason: event.FinishReason,
			}
			h.sendChunk(c, chunk)
			c.Writer.Flush()
		}
	}

	if result := <-streamResp.ResultChan; result != nil {
		var metrics *StreamMetrics
		if result.Usage != nil {
			metrics = &StreamMetrics{
				Model:            result.Model,
				PromptTokens:     result.Usage.PromptTokens,
				CompletionTokens: result.Usage.CompletionTokens,
				TotalTokens:      result.Usage.TotalTokens,
				Cost:             result.Cost,
				TTFTMs:           result.TTFTMs,
				TotalDuration:    result.TotalDuration,
			}
		}

		metricsChunk := StreamChunk{
			Type:    "metrics",
			Metrics: metrics,
		}
		h.sendChunk(c, metricsChunk)
		c.Writer.Flush()
	}
}

func (h *Handler) sendChunk(c *gin.Context, chunk StreamChunk) {
	data, _ := json.Marshal(chunk)
	c.SSEvent("message", string(data))
}

func (h *Handler) sendErrorChunk(c *gin.Context, errMsg string) {
	h.sendChunk(c, StreamChunk{Type: "error", Error: errMsg})
}
