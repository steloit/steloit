package observability

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/google/uuid"

	"brokle/internal/core/domain/observability"
	"brokle/pkg/uid"
)

// OTLPLogsConverterService handles conversion of OTLP logs to Brokle domain entities
// Follows the proven traces/metrics converter pattern with type-safe conversions
type OTLPLogsConverterService struct {
	logger *slog.Logger
}

// NewOTLPLogsConverterService creates a new OTLP logs converter service
func NewOTLPLogsConverterService(logger *slog.Logger) *OTLPLogsConverterService {
	return &OTLPLogsConverterService{
		logger: logger,
	}
}

// ConvertLogsRequest converts OTLP LogsData to Brokle telemetry events
// Returns typed domain entities (NOT map[string]interface{}) for type safety
func (s *OTLPLogsConverterService) ConvertLogsRequest(
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
			scopeName := scopeLogs.GetScope().GetName()
			scopeAttrs := extractScopeAttributes(scopeLogs.GetScope())

			// Process each LogRecord within this scope
			for _, logRecord := range scopeLogs.GetLogRecords() {
				logEntity := s.convertLogRecord(
					logRecord,
					resourceAttrs,
					scopeName,
					scopeAttrs,
					projectID,
				)

				timestamp := logEntity.Timestamp
				events = append(events, &observability.TelemetryEventRequest{
					EventType: observability.TelemetryEventTypeLog,
					EventID:   uid.New(),
					Timestamp: &timestamp,
					TraceID:   logEntity.TraceID, // May be empty if no trace correlation
					SpanID:    logEntity.SpanID,  // May be empty if no span correlation
					Payload:   convertEntityToPayload(logEntity),
				})
			}
		}
	}

	return events, nil
}

// convertLogRecord converts OTLP LogRecord to Log domain entity
func (s *OTLPLogsConverterService) convertLogRecord(
	logRecord *logspb.LogRecord,
	resourceAttrs map[string]string,
	scopeName string,
	scopeAttrs map[string]string,
	projectID uuid.UUID,
) *observability.Log {
	// Extract timestamps (nanosecond precision)
	timestamp := time.Unix(0, int64(logRecord.GetTimeUnixNano()))
	observedTimestamp := time.Unix(0, int64(logRecord.GetObservedTimeUnixNano()))

	// Extract trace/span IDs for correlation (hex strings, auto-padded)
	traceID := hex.EncodeToString(logRecord.GetTraceId()) // 32 hex chars
	spanID := hex.EncodeToString(logRecord.GetSpanId())   // 16 hex chars

	// Extract severity (number and text)
	severityNumber := logRecord.GetSeverityNumber()
	severityText := logRecord.GetSeverityText()

	// If severity text is empty, derive from severity number
	if severityText == "" {
		severityText = observability.ConvertSeverityNumberToText(int32(severityNumber))
	}

	// Extract log body (safe extraction with all AnyValue types)
	body := s.extractLogBody(logRecord.GetBody())

	// Extract log-specific attributes
	logAttributes := convertKeyValuesToMap(logRecord.GetAttributes())

	return &observability.Log{
		Timestamp:          timestamp,
		ObservedTimestamp:  observedTimestamp,
		TraceID:            traceID,
		SpanID:             spanID,
		TraceFlags:         logRecord.GetFlags(),
		SeverityText:       severityText,
		SeverityNumber:     int32(severityNumber),
		Body:               body,
		ResourceAttributes: resourceAttrs,
		ServiceName:        resourceAttrs["service.name"], // MATERIALIZED column
		ScopeName:          scopeName,
		ScopeAttributes:    scopeAttrs,
		LogAttributes:      logAttributes,
		ProjectID:          projectID.String(),
	}
}

// extractLogBody safely extracts log body from AnyValue
// Handles nil, all AnyValue types (string, int, double, bool, bytes, array, kvlist)
// JSON-encodes complex types (arrays, maps) to string
func (s *OTLPLogsConverterService) extractLogBody(body *commonpb.AnyValue) string {
	if body == nil {
		return ""
	}

	switch v := body.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return v.StringValue

	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", v.IntValue)

	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%f", v.DoubleValue)

	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", v.BoolValue)

	case *commonpb.AnyValue_BytesValue:
		// Hex-encode bytes for readability
		return hex.EncodeToString(v.BytesValue)

	case *commonpb.AnyValue_ArrayValue:
		// JSON-encode arrays
		array := anyValueArrayToInterface(v.ArrayValue)
		jsonBytes, err := json.Marshal(array)
		if err != nil {
			s.logger.Warn("failed to marshal array body, using empty string", "error", err)
			return ""
		}
		return string(jsonBytes)

	case *commonpb.AnyValue_KvlistValue:
		// JSON-encode key-value lists (convert to map first)
		kvMap := convertKeyValuesToMap(v.KvlistValue.GetValues())
		jsonBytes, err := json.Marshal(kvMap)
		if err != nil {
			s.logger.Warn("failed to marshal kvlist body, using empty string", "error", err)
			return ""
		}
		return string(jsonBytes)

	default:
		// Unknown type - return empty string
		return ""
	}
}

// anyValueArrayToInterface converts OTLP ArrayValue to Go []interface{}
// Used for JSON marshaling of array bodies
func anyValueArrayToInterface(arrayValue *commonpb.ArrayValue) []interface{} {
	if arrayValue == nil {
		return []interface{}{}
	}

	values := arrayValue.GetValues()
	result := make([]interface{}, len(values))
	for i, v := range values {
		result[i] = anyValueToInterface(v)
	}
	return result
}

// anyValueToInterface converts OTLP AnyValue to Go interface{}
// Preserves type information for JSON marshaling
func anyValueToInterface(value *commonpb.AnyValue) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return v.StringValue
	case *commonpb.AnyValue_IntValue:
		return v.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return v.DoubleValue
	case *commonpb.AnyValue_BoolValue:
		return v.BoolValue
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(v.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		return anyValueArrayToInterface(v.ArrayValue)
	case *commonpb.AnyValue_KvlistValue:
		return convertKeyValuesToMap(v.KvlistValue.GetValues())
	default:
		return nil
	}
}

// Note: Helper functions (convertKeyValuesToMap, attributeValueToString, convertEntityToPayload,
// extractResourceAttributes, extractScopeAttributes) are now in otlp_helpers.go
