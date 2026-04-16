package observability

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	logspb "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/google/uuid"

	"brokle/internal/core/domain/observability"
	"brokle/pkg/uid"
)

// OTLPEventsConverterService handles conversion of OTLP GenAI events to Brokle domain entities
// GenAI events are sent via OTLP Logs API as structured log records with specific event names
// OTEL Specification: https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-events/
type OTLPEventsConverterService struct {
	logger *slog.Logger
}

// NewOTLPEventsConverterService creates a new OTLP events converter service
func NewOTLPEventsConverterService(logger *slog.Logger) *OTLPEventsConverterService {
	return &OTLPEventsConverterService{
		logger: logger,
	}
}

// ConvertGenAIEventsRequest converts OTLP LogsData to GenAI event domain entities
// Filters for GenAI-specific event names and extracts structured attributes
// Returns typed domain entities for type safety
func (s *OTLPEventsConverterService) ConvertGenAIEventsRequest(
	ctx context.Context,
	logsData *logspb.LogsData,
	projectID uuid.UUID,
) ([]*observability.TelemetryEventRequest, error) {
	var events []*observability.TelemetryEventRequest

	// Process each ResourceLogs
	for _, resourceLogs := range logsData.GetResourceLogs() {
		// Extract resource attributes once per resource
		resourceAttrs := extractResourceAttributes(resourceLogs.GetResource())

		// Process each ScopeLogs within this resource
		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			// Process each LogRecord within this scope
			for _, logRecord := range scopeLogs.GetLogRecords() {
				// Extract event name from log attributes
				logAttributes := convertKeyValuesToMap(logRecord.GetAttributes())
				eventName := logAttributes["event.name"]

				// Filter for GenAI events only
				if !isGenAIEvent(eventName) {
					continue
				}

				// Convert to GenAI event entity
				genAIEvent := s.convertGenAIEventRecord(
					logRecord,
					eventName,
					resourceAttrs,
					logAttributes,
					projectID,
				)

				timestamp := genAIEvent.Timestamp
				events = append(events, &observability.TelemetryEventRequest{
					EventType: observability.TelemetryEventTypeGenAIEvent,
					EventID:   uid.New(),
					Timestamp: &timestamp,
					TraceID:   genAIEvent.TraceID,
					SpanID:    genAIEvent.SpanID,
					Payload:   convertEntityToPayload(genAIEvent),
				})
			}
		}
	}

	return events, nil
}

// convertGenAIEventRecord converts OTLP LogRecord to GenAIEvent domain entity
func (s *OTLPEventsConverterService) convertGenAIEventRecord(
	logRecord *logspb.LogRecord,
	eventName string,
	resourceAttrs map[string]string,
	logAttributes map[string]string,
	projectID uuid.UUID,
) *observability.GenAIEvent {
	// Extract timestamps (nanosecond precision)
	timestamp := time.Unix(0, int64(logRecord.GetTimeUnixNano()))

	// Extract trace/span IDs for correlation (hex strings)
	traceID := hex.EncodeToString(logRecord.GetTraceId())
	spanID := hex.EncodeToString(logRecord.GetSpanId())

	// Extract inference operation attributes
	operationName := logAttributes["gen_ai.operation.name"]
	modelName := logAttributes["gen_ai.request.model"]
	providerName := logAttributes["gen_ai.system"] // OTEL uses gen_ai.system for provider

	// Extract input/output messages (JSON strings)
	inputMessages := s.extractMessagesJSON(logAttributes["gen_ai.prompt"])
	outputMessages := s.extractMessagesJSON(logAttributes["gen_ai.completion"])

	// Extract token counts
	inputTokens := s.extractUInt32(logAttributes["gen_ai.usage.input_tokens"])
	outputTokens := s.extractUInt32(logAttributes["gen_ai.usage.output_tokens"])

	// Extract request parameters (nullable)
	temperature := s.extractFloat32Ptr(logAttributes["gen_ai.request.temperature"])
	topP := s.extractFloat32Ptr(logAttributes["gen_ai.request.top_p"])
	maxTokens := s.extractUInt32Ptr(logAttributes["gen_ai.request.max_tokens"])

	// Extract finish reasons (array)
	finishReasons := s.extractStringArray(logAttributes["gen_ai.response.finish_reasons"])

	// Extract response ID
	responseID := logAttributes["gen_ai.response.id"]

	// Extract evaluation attributes (nullable - only for evaluation events)
	var evaluationName, evaluationLabel *string
	var evaluationScore *float32
	var evaluationExplanation *string

	if isEvaluationEvent(eventName) {
		if val := logAttributes["gen_ai.evaluation.name"]; val != "" {
			evaluationName = &val
		}
		if val := logAttributes["gen_ai.evaluation.label"]; val != "" {
			evaluationLabel = &val
		}
		evaluationScore = s.extractFloat32Ptr(logAttributes["gen_ai.evaluation.score"])
		if val := logAttributes["gen_ai.evaluation.explanation"]; val != "" {
			evaluationExplanation = &val
		}
	}

	// Extract Brokle-specific attributes
	userID := logAttributes["brokle.user_id"]
	if userID == "" {
		userID = resourceAttrs["user.id"] // Fallback to resource attributes
	}

	sessionID := logAttributes["brokle.session_id"]
	if sessionID == "" {
		sessionID = logAttributes["session.id"] // Fallback to OTEL session.id
	}

	return &observability.GenAIEvent{
		Timestamp:             timestamp,
		EventName:             eventName,
		TraceID:               traceID,
		SpanID:                spanID,
		OperationName:         operationName,
		ModelName:             modelName,
		ProviderName:          providerName,
		InputMessages:         inputMessages,
		OutputMessages:        outputMessages,
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		Temperature:           temperature,
		TopP:                  topP,
		MaxTokens:             maxTokens,
		FinishReasons:         finishReasons,
		ResponseID:            responseID,
		EvaluationName:        evaluationName,
		EvaluationScore:       evaluationScore,
		EvaluationLabel:       evaluationLabel,
		EvaluationExplanation: evaluationExplanation,
		ProjectID:             projectID.String(),
		UserID:                userID,
		SessionID:             sessionID,
	}
}

// ===== Helper Functions =====

// isGenAIEvent checks if event name is a recognized GenAI event
func isGenAIEvent(eventName string) bool {
	switch eventName {
	case observability.EventGenAIInferenceDetails,
		observability.EventGenAIEvaluationResult:
		return true
	default:
		return false
	}
}

// isEvaluationEvent checks if event is an evaluation event
func isEvaluationEvent(eventName string) bool {
	return eventName == observability.EventGenAIEvaluationResult
}

// extractMessagesJSON extracts and validates JSON string from attribute
// Returns empty string if invalid or missing
func (s *OTLPEventsConverterService) extractMessagesJSON(value string) string {
	if value == "" {
		return ""
	}

	// Validate JSON (basic check)
	var test interface{}
	if err := json.Unmarshal([]byte(value), &test); err != nil {
		s.logger.Warn("invalid JSON in messages attribute, storing as-is", "error", err)
	}

	return value
}

// extractUInt32 safely extracts uint32 from string attribute
func (s *OTLPEventsConverterService) extractUInt32(value string) uint32 {
	if value == "" {
		return 0
	}

	var result uint32
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		s.logger.Warn("failed to parse uint32 attribute", "error", err)
		return 0
	}

	return result
}

// extractUInt32Ptr safely extracts *uint32 from string attribute (nullable)
func (s *OTLPEventsConverterService) extractUInt32Ptr(value string) *uint32 {
	if value == "" {
		return nil
	}

	var result uint32
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return nil
	}

	return &result
}

// extractFloat32Ptr safely extracts *float32 from string attribute (nullable)
func (s *OTLPEventsConverterService) extractFloat32Ptr(value string) *float32 {
	if value == "" {
		return nil
	}

	var result float32
	if _, err := fmt.Sscanf(value, "%f", &result); err != nil {
		return nil
	}

	return &result
}

// extractStringArray safely extracts string array from JSON attribute
func (s *OTLPEventsConverterService) extractStringArray(value string) []string {
	if value == "" {
		return []string{}
	}

	var result []string
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		s.logger.Warn("failed to parse string array attribute", "error", err)
		return []string{}
	}

	return result
}
