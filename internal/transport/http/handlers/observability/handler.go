package observability

import (
	"log/slog"

	"brokle/internal/config"
	obsServices "brokle/internal/core/services/observability"
)

// Handler contains all observability-related HTTP handlers
type Handler struct {
	config   *config.Config
	logger   *slog.Logger
	services *obsServices.ServiceRegistry
}

// CreateEventRequest represents telemetry event creation request
// @Description Request data for creating telemetry events
type CreateEventRequest struct {
	Timestamp   *int64                 `json:"timestamp,omitempty" example:"1677610602" description:"Unix timestamp (defaults to current time)"`
	SessionID   *string                `json:"session_id,omitempty" example:"sess_abc123" description:"Session identifier"`
	TraceID     *string                `json:"trace_id,omitempty" example:"trace_def456" description:"Distributed trace ID"`
	SpanID      *string                `json:"span_id,omitempty" example:"span_ghi789" description:"Span identifier"`
	UserID      *string                `json:"user_id,omitempty" example:"user_123" description:"User identifier"`
	Properties  map[string]any `json:"properties,omitempty" description:"Event-specific properties"`
	Metadata    map[string]any `json:"metadata,omitempty" description:"Additional metadata"`
	Environment *string                `json:"environment,omitempty" example:"production" description:"Environment name"`
	Version     *string                `json:"version,omitempty" example:"1.0.0" description:"Application version"`
	Source      *string                `json:"source,omitempty" example:"python-sdk" description:"Event source (sdk, api, etc.)"`
	EventType   string                 `json:"event_type" example:"ai.request.completed" description:"Type of telemetry event"`
	Tags        []string               `json:"tags,omitempty" description:"Event tags for categorization"`
}

// EventResponse represents telemetry event creation response
// @Description Telemetry event creation result
type EventResponse struct {
	EventID     string `json:"event_id" example:"evt_abc123" description:"Created event identifier"`
	Status      string `json:"status" example:"created" description:"Event creation status"`
	Message     string `json:"message" example:"Event created successfully" description:"Operation result message"`
	ProcessedAt int64  `json:"processed_at" example:"1677610602" description:"Unix timestamp when event was processed"`
}

// NewHandler creates a new observability handler
func NewHandler(
	cfg *config.Config,
	logger *slog.Logger,
	services *obsServices.ServiceRegistry,
) *Handler {
	return &Handler{
		config:   cfg,
		logger:   logger,
		services: services,
	}
}
