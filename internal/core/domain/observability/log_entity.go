package observability

import (
	"time"

	"github.com/google/uuid"
)

// ==================================
// OTLP Logs Entity
// ==================================
// Struct matches ClickHouse schema EXACTLY (column order, types)
// Verified via: docker compose exec -T clickhouse clickhouse-client -q "DESCRIBE TABLE otel_logs"

// Log represents an OTLP log record with trace correlation
// Schema: otel_logs (15 columns)
// Note: trace_id and span_id are hex strings (NOT arrays)
type Log struct {
	Timestamp          time.Time         `ch:"timestamp"`           // DateTime64(9) nanosecond precision
	ObservedTimestamp  time.Time         `ch:"observed_timestamp"`  // DateTime64(9) nanosecond precision
	TraceID            string            `ch:"trace_id"`            // Hex string (32 chars) for trace correlation
	SpanID             string            `ch:"span_id"`             // Hex string (16 chars) for span correlation
	TraceFlags         uint32            `ch:"trace_flags"`         // W3C trace flags
	SeverityText       string            `ch:"severity_text"`       // Human-readable severity (e.g., "INFO", "ERROR")
	SeverityNumber     int32             `ch:"severity_number"`     // OTLP severity number (1-24)
	Body               string            `ch:"body"`                // Log message body (AnyValue → string)
	ResourceAttributes map[string]string `ch:"resource_attributes"` // OTLP resource attributes
	ServiceName        string            `ch:"service_name"`        // MATERIALIZED from resource_attributes["service.name"]
	ScopeName          string            `ch:"scope_name"`          // Instrumentation scope name
	ScopeAttributes    map[string]string `ch:"scope_attributes"`    // Instrumentation scope attributes
	LogAttributes      map[string]string `ch:"log_attributes"`      // Log-specific attributes
	ProjectID          uuid.UUID         `ch:"project_id"`
}

// ==================================
// OTLP Severity Number Constants
// ==================================
// OTLP spec: SeverityNumber enum (1-24)
// Reference: https://opentelemetry.io/docs/specs/otel/logs/data-model/#field-severitynumber
const (
	SeverityNumberUnspecified = 0  // Unspecified
	SeverityNumberTrace       = 1  // TRACE
	SeverityNumberTrace2      = 2  // TRACE2
	SeverityNumberTrace3      = 3  // TRACE3
	SeverityNumberTrace4      = 4  // TRACE4
	SeverityNumberDebug       = 5  // DEBUG
	SeverityNumberDebug2      = 6  // DEBUG2
	SeverityNumberDebug3      = 7  // DEBUG3
	SeverityNumberDebug4      = 8  // DEBUG4
	SeverityNumberInfo        = 9  // INFO
	SeverityNumberInfo2       = 10 // INFO2
	SeverityNumberInfo3       = 11 // INFO3
	SeverityNumberInfo4       = 12 // INFO4
	SeverityNumberWarn        = 13 // WARN
	SeverityNumberWarn2       = 14 // WARN2
	SeverityNumberWarn3       = 15 // WARN3
	SeverityNumberWarn4       = 16 // WARN4
	SeverityNumberError       = 17 // ERROR
	SeverityNumberError2      = 18 // ERROR2
	SeverityNumberError3      = 19 // ERROR3
	SeverityNumberError4      = 20 // ERROR4
	SeverityNumberFatal       = 21 // FATAL
	SeverityNumberFatal2      = 22 // FATAL2
	SeverityNumberFatal3      = 23 // FATAL3
	SeverityNumberFatal4      = 24 // FATAL4
)

// SeverityText string constants for common severity levels
const (
	SeverityTextTrace = "TRACE"
	SeverityTextDebug = "DEBUG"
	SeverityTextInfo  = "INFO"
	SeverityTextWarn  = "WARN"
	SeverityTextError = "ERROR"
	SeverityTextFatal = "FATAL"
)

// ConvertSeverityNumberToText converts OTLP severity number to text
// Returns empty string for unspecified or invalid values
func ConvertSeverityNumberToText(severityNumber int32) string {
	switch {
	case severityNumber >= SeverityNumberFatal && severityNumber <= SeverityNumberFatal4:
		return SeverityTextFatal
	case severityNumber >= SeverityNumberError && severityNumber <= SeverityNumberError4:
		return SeverityTextError
	case severityNumber >= SeverityNumberWarn && severityNumber <= SeverityNumberWarn4:
		return SeverityTextWarn
	case severityNumber >= SeverityNumberInfo && severityNumber <= SeverityNumberInfo4:
		return SeverityTextInfo
	case severityNumber >= SeverityNumberDebug && severityNumber <= SeverityNumberDebug4:
		return SeverityTextDebug
	case severityNumber >= SeverityNumberTrace && severityNumber <= SeverityNumberTrace4:
		return SeverityTextTrace
	default:
		return ""
	}
}
