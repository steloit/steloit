package evaluation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type SDKScoreHandler struct {
	logger             *slog.Logger
	scoreService       *obsServices.ScoreService
	scoreConfigService evaluationDomain.ScoreConfigService
}

func NewSDKScoreHandler(
	logger *slog.Logger,
	scoreService *obsServices.ScoreService,
	scoreConfigService evaluationDomain.ScoreConfigService,
) *SDKScoreHandler {
	return &SDKScoreHandler{
		logger:             logger,
		scoreService:       scoreService,
		scoreConfigService: scoreConfigService,
	}
}

// @Summary Create score
// @Description Creates a score for a trace/span. If a ScoreConfig exists for the score name, validates against it.
// @Tags SDK - Scores
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body CreateScoreRequest true "Score creation request"
// @Success 201 {object} ScoreResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /v1/scores [post]
func (h *SDKScoreHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()

	// Get project ID from SDK auth middleware
	projectIDPtr, exists := middleware.GetProjectID(c)
	if !exists || projectIDPtr == nil {
		h.logger.Error("Project ID not found in context")
		response.Unauthorized(c, "Authentication required")
		return
	}
	projectID := *projectIDPtr

	var req CreateScoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validate against ScoreConfig if one exists
	if err := h.validateAgainstConfig(ctx, projectID, req.Name, req.Type, req.Value, req.StringValue); err != nil {
		response.Error(c, err)
		return
	}

	// Build observability Score entity
	score := h.buildScore(projectID.String(), &req)

	// Create score using observability service
	if err := h.scoreService.CreateScore(ctx, score); err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("score created via SDK",
		"score_id", score.ID,
		"project_id", projectID,
		"trace_id", score.TraceID,
		"name", score.Name,
	)

	response.Created(c, h.toResponse(score))
}

// @Summary Batch create scores
// @Description Creates multiple scores in a single request. If ScoreConfigs exist, validates against them.
// @Tags SDK - Scores
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body BatchScoreRequest true "Batch score creation request"
// @Success 201 {object} BatchScoreResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /v1/scores/batch [post]
func (h *SDKScoreHandler) CreateBatch(c *gin.Context) {
	ctx := c.Request.Context()

	// Get project ID from SDK auth middleware
	projectIDPtr, exists := middleware.GetProjectID(c)
	if !exists || projectIDPtr == nil {
		h.logger.Error("Project ID not found in context")
		response.Unauthorized(c, "Authentication required")
		return
	}
	projectID := *projectIDPtr

	var req BatchScoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if len(req.Scores) == 0 {
		response.Error(c, appErrors.NewValidationError("Invalid request body", "scores array cannot be empty"))
		return
	}

	// Validate all scores against their configs and build entities
	scores := make([]*observability.Score, 0, len(req.Scores))
	for i, scoreReq := range req.Scores {
		// Validate against ScoreConfig if one exists
		if err := h.validateAgainstConfig(ctx, projectID, scoreReq.Name, scoreReq.Type, scoreReq.Value, scoreReq.StringValue); err != nil {
			// Include index in error for debugging
			h.logger.Warn("score validation failed",
				"index", i,
				"name", scoreReq.Name,
				"error", err.Error(),
			)
			response.Error(c, err)
			return
		}
		scores = append(scores, h.buildScore(projectID.String(), &scoreReq))
	}

	// Create scores using observability service
	if err := h.scoreService.CreateScoreBatch(ctx, scores); err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("batch scores created via SDK",
		"project_id", projectID,
		"count", len(scores),
	)

	response.Created(c, &BatchScoreResponse{
		Created: len(scores),
	})
}

// validateAgainstConfig validates the score against a ScoreConfig if one exists for the given name.
// Returns nil if no config exists (scores without configs are allowed).
// Returns error if config lookup fails (to prevent bypassing validation during outages).
func (h *SDKScoreHandler) validateAgainstConfig(
	ctx context.Context,
	projectID ulid.ULID,
	name string,
	scoreType string,
	value *float64,
	stringValue *string,
) error {
	config, err := h.scoreConfigService.GetByName(ctx, projectID, name)
	if err != nil {
		// If config not found, skip validation (scores without configs are allowed)
		if appErrors.IsNotFound(err) {
			return nil
		}
		// Any other error (DB failure, etc.) - fail the request to prevent
		// bypassing validation during repository outages
		return err
	}

	// Validate type matches
	if string(config.Type) != scoreType {
		return appErrors.NewValidationError("type",
			"must match score config (expected: "+string(config.Type)+")")
	}

	// Validate based on type
	switch config.Type {
	case evaluationDomain.ScoreTypeNumeric:
		if value == nil {
			return appErrors.NewValidationError("value", "required for NUMERIC type")
		}
		if config.MinValue != nil && *value < *config.MinValue {
			return appErrors.NewValidationError("value", "below minimum configured value")
		}
		if config.MaxValue != nil && *value > *config.MaxValue {
			return appErrors.NewValidationError("value", "above maximum configured value")
		}

	case evaluationDomain.ScoreTypeCategorical:
		if stringValue == nil {
			return appErrors.NewValidationError("string_value", "required for CATEGORICAL type")
		}
		// Check if value is in allowed categories
		if !contains(config.Categories, *stringValue) {
			return appErrors.NewValidationError("string_value", "not in allowed categories")
		}

	case evaluationDomain.ScoreTypeBoolean:
		// For boolean, we expect a numeric value of 0 or 1, or a string value of "true"/"false"
		if value == nil && stringValue == nil {
			return appErrors.NewValidationError("value", "required for BOOLEAN type (0 or 1)")
		}
		if value != nil && (*value != 0 && *value != 1) {
			return appErrors.NewValidationError("value", "must be 0 or 1 for BOOLEAN type")
		}
	}

	return nil
}

func (h *SDKScoreHandler) buildScore(projectID string, req *CreateScoreRequest) *observability.Score {
	metadata := json.RawMessage("{}")
	if req.Metadata != nil {
		if jsonBytes, err := json.Marshal(req.Metadata); err == nil {
			metadata = jsonBytes
		}
	}

	score := &observability.Score{
		ID:               ulid.New().String(),
		ProjectID:        projectID,
		TraceID:          req.TraceID,
		Name:             req.Name,
		Type:             req.Type,
		Value:            req.Value,
		StringValue:      req.StringValue,
		Source:           observability.ScoreSourceAPI, // SDK scores are programmatic
		Reason:           req.Reason,
		Metadata:         metadata,
		ExperimentID:     req.ExperimentID,
		ExperimentItemID: req.ExperimentItemID,
		Timestamp:        time.Now(),
	}

	// Set SpanID: use provided value, or default to TraceID for trace-linked scores
	if req.SpanID != nil {
		score.SpanID = req.SpanID
	} else if req.TraceID != nil {
		// Default span_id to trace_id for trace-linked scores
		score.SpanID = req.TraceID
	}
	// For experiment-only scores, SpanID remains nil

	return score
}

func (h *SDKScoreHandler) toResponse(score *observability.Score) *ScoreResponse {
	// Ensure metadata is safe for JSON serialization.
	// Valid JSON passes through as-is. Malformed bytes (from legacy writes)
	// are escaped as a JSON string so the raw content is preserved losslessly
	// rather than silently dropped or breaking Gin's c.JSON() marshaling.
	metadata := score.Metadata
	if len(metadata) > 0 && !json.Valid(metadata) {
		metadata, _ = json.Marshal(string(metadata))
	}

	return &ScoreResponse{
		ID:               score.ID,
		ProjectID:        score.ProjectID,
		TraceID:          score.TraceID,
		SpanID:           score.SpanID,
		Name:             score.Name,
		Value:            score.Value,
		StringValue:      score.StringValue,
		Type:             score.Type,
		Source:           score.Source,
		Reason:           score.Reason,
		Metadata:         metadata,
		ExperimentID:     score.ExperimentID,
		ExperimentItemID: score.ExperimentItemID,
		Timestamp:        score.Timestamp,
	}
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
