package observability

// OTLP data structures for OpenTelemetry Protocol trace ingestion
// These types support both JSON and Protobuf formats

// OTLPRequest represents an OTLP trace export request
type OTLPRequest struct {
	ResourceSpans []ResourceSpan `json:"resourceSpans"`
}

// ResourceSpan represents a collection of spans from a single resource
type ResourceSpan struct {
	Resource   *Resource   `json:"resource,omitempty"`
	ScopeSpans []ScopeSpan `json:"scopeSpans"`
}

// Resource represents OTEL resource attributes
type Resource struct {
	Attributes []KeyValue `json:"attributes"`
	SchemaUrl  string     `json:"schemaUrl,omitempty"`
}

// ScopeSpan represents a collection of spans from a single instrumentation scope
type ScopeSpan struct {
	Scope *Scope     `json:"scope,omitempty"`
	Spans []OTLPSpan `json:"spans"`
}

// Scope represents an instrumentation scope
type Scope struct {
	Name       string     `json:"name"`
	Version    string     `json:"version,omitempty"`
	Attributes []KeyValue `json:"attributes,omitempty"`
	SchemaUrl  string     `json:"schemaUrl,omitempty"`
}

// OTLPSpan represents an OTLP span (wire format)
type OTLPSpan struct {
	TraceID           any `json:"traceId"`
	SpanID            any `json:"spanId"`
	ParentSpanID      any `json:"parentSpanId,omitempty"`
	StartTimeUnixNano any `json:"startTimeUnixNano"`
	EndTimeUnixNano   any `json:"endTimeUnixNano,omitempty"`
	Status            *Status     `json:"status,omitempty"`
	Name              string      `json:"name"`
	Attributes        []KeyValue  `json:"attributes,omitempty"`
	Events            []Event     `json:"events,omitempty"`
	Links             []Link      `json:"links,omitempty"`
	Kind              int         `json:"kind,omitempty"`
}

// KeyValue represents an OTLP attribute key-value pair
type KeyValue struct {
	Value any `json:"value"`
	Key   string      `json:"key"`
}

// Event represents an OTLP span event (timestamped annotation within a span)
type Event struct {
	TimeUnixNano           any `json:"timeUnixNano"`
	Name                   string      `json:"name"`
	Attributes             []KeyValue  `json:"attributes,omitempty"`
	DroppedAttributesCount uint32      `json:"droppedAttributesCount,omitempty"` // Number of dropped attributes
}

// Link represents an OTLP span link (reference to span in another trace)
type Link struct {
	TraceID                any `json:"traceId"`                          // Linked trace ID (Buffer or hex string)
	SpanID                 any `json:"spanId"`                           // Linked span ID (Buffer or hex string)
	TraceState             any `json:"traceState,omitempty"`             // W3C TraceState for linked span
	Attributes             []KeyValue  `json:"attributes,omitempty"`             // Link metadata
	DroppedAttributesCount uint32      `json:"droppedAttributesCount,omitempty"` // Number of dropped attributes
}

// Status represents OTLP status
type Status struct {
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

// ============================================================================
// OTLP Logs Types (for swagger documentation)
// ============================================================================

// OTLPLogsRequest represents an OTLP logs export request
type OTLPLogsRequest struct {
	ResourceLogs []ResourceLogs `json:"resourceLogs"`
}

// ResourceLogs represents a collection of logs from a single resource
type ResourceLogs struct {
	Resource  *Resource   `json:"resource,omitempty"`
	ScopeLogs []ScopeLogs `json:"scopeLogs"`
}

// ScopeLogs represents a collection of logs from a single instrumentation scope
type ScopeLogs struct {
	Scope      *Scope      `json:"scope,omitempty"`
	LogRecords []LogRecord `json:"logRecords"`
}

// LogRecord represents an OTLP log record
type LogRecord struct {
	TimeUnixNano         any `json:"timeUnixNano"`
	ObservedTimeUnixNano any `json:"observedTimeUnixNano,omitempty"`
	SeverityNumber       int         `json:"severityNumber,omitempty"`
	SeverityText         string      `json:"severityText,omitempty"`
	Body                 any `json:"body,omitempty"`
	Attributes           []KeyValue  `json:"attributes,omitempty"`
	TraceID              any `json:"traceId,omitempty"`
	SpanID               any `json:"spanId,omitempty"`
}

// ============================================================================
// OTLP Metrics Types (for swagger documentation)
// ============================================================================

// OTLPMetricsRequest represents an OTLP metrics export request
type OTLPMetricsRequest struct {
	ResourceMetrics []ResourceMetrics `json:"resourceMetrics"`
}

// ResourceMetrics represents a collection of metrics from a single resource
type ResourceMetrics struct {
	Resource     *Resource      `json:"resource,omitempty"`
	ScopeMetrics []ScopeMetrics `json:"scopeMetrics"`
}

// ScopeMetrics represents a collection of metrics from a single instrumentation scope
type ScopeMetrics struct {
	Scope   *Scope   `json:"scope,omitempty"`
	Metrics []Metric `json:"metrics"`
}

// Metric represents an OTLP metric
type Metric struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Unit        string      `json:"unit,omitempty"`
	Data        any `json:"data,omitempty"` // Can be Gauge, Sum, Histogram, etc.
}
