package observability

import (
	"time"

	"github.com/google/uuid"
)

// ==================================
// OTLP GenAI Events Entity
// ==================================
// GenAI events capture detailed inference operations and evaluation results
// OTEL Specification: https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-events/
// Schema: otel_genai_events (23 columns)

// GenAIEvent represents a GenAI-specific event (inference operation or evaluation)
// Events are sent via OTLP Logs API with structured attributes
// Events survive trace sampling and provide detailed inference capture
type GenAIEvent struct {
	Timestamp             time.Time `ch:"timestamp"`              // DateTime64(9) nanosecond precision
	EventName             string    `ch:"event_name"`             // event.gen_ai.client.inference.operation.details or event.gen_ai.evaluation.result
	TraceID               string    `ch:"trace_id"`               // Associated trace ID (hex string)
	SpanID                string    `ch:"span_id"`                // Associated span ID (hex string)
	OperationName         string    `ch:"operation_name"`         // chat, completion, embedding, etc.
	ModelName             string    `ch:"model_name"`             // gen_ai.request.model
	ProviderName          string    `ch:"provider_name"`          // gen_ai.provider.name
	InputMessages         string    `ch:"input_messages"`         // JSON array of input messages (ZSTD compressed)
	OutputMessages        string    `ch:"output_messages"`        // JSON array of output messages (ZSTD compressed)
	InputTokens           uint32    `ch:"input_tokens"`           // gen_ai.usage.input_tokens
	OutputTokens          uint32    `ch:"output_tokens"`          // gen_ai.usage.output_tokens
	Temperature           *float32  `ch:"temperature"`            // Nullable - gen_ai.request.temperature
	TopP                  *float32  `ch:"top_p"`                  // Nullable - gen_ai.request.top_p
	MaxTokens             *uint32   `ch:"max_tokens"`             // Nullable - gen_ai.request.max_tokens
	FinishReasons         []string  `ch:"finish_reasons"`         // Array - gen_ai.response.finish_reasons
	ResponseID            string    `ch:"response_id"`            // gen_ai.response.id
	EvaluationName        *string   `ch:"evaluation_name"`        // Nullable - for evaluation events
	EvaluationScore       *float32  `ch:"evaluation_score"`       // Nullable - quality score (0.0-1.0)
	EvaluationLabel       *string   `ch:"evaluation_label"`       // Nullable - pass/fail/warning
	EvaluationExplanation *string   `ch:"evaluation_explanation"` // Nullable - ZSTD compressed
	ProjectID             uuid.UUID `ch:"project_id"`             // Multi-tenancy
	UserID                string    `ch:"user_id"`                // CH LowCardinality(String) — free-form from OTLP attrs
	SessionID             string    `ch:"session_id"`             // CH LowCardinality(String) — free-form session id
}

// ==================================
// Event Name Constants
// ==================================
// OTEL GenAI event names per specification
const (
	// Inference operation events (detailed input/output capture)
	EventGenAIInferenceDetails = "event.gen_ai.client.inference.operation.details"

	// Evaluation result events (quality scores)
	EventGenAIEvaluationResult = "event.gen_ai.evaluation.result"
)
