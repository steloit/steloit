package observability

import "time"

// ==================================
// OTLP Metrics Entities
// ==================================
// All structs match ClickHouse schemas EXACTLY (column order, types, nullability)
// Verified via: docker compose exec -T clickhouse clickhouse-client -q "DESCRIBE TABLE otel_metrics_*"

// MetricSum represents a sum metric (Counter or UpDownCounter)
// Schema: otel_metrics_sum (18 columns)
// Supports: DELTA and CUMULATIVE aggregation temporality
type MetricSum struct {
	ResourceAttributes          map[string]string   `ch:"resource_attributes"`
	ServiceName                 string              `ch:"service_name"` // MATERIALIZED from resource_attributes["service.name"]
	ScopeName                   string              `ch:"scope_name"`
	ScopeVersion                string              `ch:"scope_version"`
	ScopeAttributes             map[string]string   `ch:"scope_attributes"`
	MetricName                  string              `ch:"metric_name"`
	MetricDescription           string              `ch:"metric_description"`
	MetricUnit                  string              `ch:"metric_unit"`
	Attributes                  map[string]string   `ch:"attributes"`
	StartTimeUnix               time.Time           `ch:"start_time_unix"` // DateTime64(9) nanosecond precision
	TimeUnix                    time.Time           `ch:"time_unix"`       // DateTime64(9) nanosecond precision
	Value                       float64             `ch:"value"`
	AggregationTemporality      string              `ch:"aggregation_temporality"` // Enum8: "UNSPECIFIED", "DELTA", "CUMULATIVE"
	IsMonotonic                 bool                `ch:"is_monotonic"`
	ResourceSchemaURL           *string             `ch:"resource_schema_url"`           // Nullable(String) - OTLP versioning
	ScopeSchemaURL              *string             `ch:"scope_schema_url"`              // Nullable(String) - OTLP versioning
	ExemplarsTimestamp          []time.Time         `ch:"exemplars_timestamp"`           // Array(DateTime64(9))
	ExemplarsValue              []float64           `ch:"exemplars_value"`               // Array(Float64)
	ExemplarsFilteredAttributes []map[string]string `ch:"exemplars_filtered_attributes"` // Array(Map(LowCardinality(String), String))
	ExemplarsTraceID            []string            `ch:"exemplars_trace_id"`
	ExemplarsSpanID             []string            `ch:"exemplars_span_id"`
	ProjectID                   string              `ch:"project_id"`
}

// MetricGauge represents a gauge metric (instantaneous value)
// Schema: otel_metrics_gauge (15 columns)
// Note: Includes start_time_unix (differs from OTLP spec where Gauge has no start time)
type MetricGauge struct {
	ResourceAttributes          map[string]string   `ch:"resource_attributes"`
	ServiceName                 string              `ch:"service_name"` // MATERIALIZED from resource_attributes["service.name"]
	ScopeName                   string              `ch:"scope_name"`
	ScopeVersion                string              `ch:"scope_version"`
	ScopeAttributes             map[string]string   `ch:"scope_attributes"`
	MetricName                  string              `ch:"metric_name"`
	MetricDescription           string              `ch:"metric_description"`
	MetricUnit                  string              `ch:"metric_unit"`
	Attributes                  map[string]string   `ch:"attributes"`
	StartTimeUnix               time.Time           `ch:"start_time_unix"` // DateTime64(9) - Gauge HAS start_time in our schema
	TimeUnix                    time.Time           `ch:"time_unix"`       // DateTime64(9) nanosecond precision
	Value                       float64             `ch:"value"`
	ResourceSchemaURL           *string             `ch:"resource_schema_url"`           // Nullable(String) - OTLP versioning
	ScopeSchemaURL              *string             `ch:"scope_schema_url"`              // Nullable(String) - OTLP versioning
	ExemplarsTimestamp          []time.Time         `ch:"exemplars_timestamp"`           // Array(DateTime64(9))
	ExemplarsValue              []float64           `ch:"exemplars_value"`               // Array(Float64)
	ExemplarsFilteredAttributes []map[string]string `ch:"exemplars_filtered_attributes"` // Array(Map(LowCardinality(String), String))
	ExemplarsTraceID            []string            `ch:"exemplars_trace_id"`
	ExemplarsSpanID             []string            `ch:"exemplars_span_id"`
	ProjectID                   string              `ch:"project_id"`
}

// MetricHistogram represents a histogram metric (distribution)
// Schema: otel_metrics_histogram (21 columns)
// Note: sum, min, max are Nullable(Float64) - use pointer types
type MetricHistogram struct {
	ResourceAttributes          map[string]string   `ch:"resource_attributes"`
	ServiceName                 string              `ch:"service_name"` // MATERIALIZED from resource_attributes["service.name"]
	ScopeName                   string              `ch:"scope_name"`
	ScopeVersion                string              `ch:"scope_version"`
	ScopeAttributes             map[string]string   `ch:"scope_attributes"`
	MetricName                  string              `ch:"metric_name"`
	MetricDescription           string              `ch:"metric_description"`
	MetricUnit                  string              `ch:"metric_unit"`
	Attributes                  map[string]string   `ch:"attributes"`
	StartTimeUnix               time.Time           `ch:"start_time_unix"` // DateTime64(9) nanosecond precision
	TimeUnix                    time.Time           `ch:"time_unix"`       // DateTime64(9) nanosecond precision
	Count                       uint64              `ch:"count"`
	Sum                         *float64            `ch:"sum"`                           // Nullable(Float64)
	Min                         *float64            `ch:"min"`                           // Nullable(Float64)
	Max                         *float64            `ch:"max"`                           // Nullable(Float64)
	BucketCounts                []uint64            `ch:"bucket_counts"`                 // Array(UInt64)
	ExplicitBounds              []float64           `ch:"explicit_bounds"`               // Array(Float64)
	AggregationTemporality      string              `ch:"aggregation_temporality"`       // Enum8: "UNSPECIFIED", "DELTA", "CUMULATIVE"
	ResourceSchemaURL           *string             `ch:"resource_schema_url"`           // Nullable(String) - OTLP versioning
	ScopeSchemaURL              *string             `ch:"scope_schema_url"`              // Nullable(String) - OTLP versioning
	ExemplarsTimestamp          []time.Time         `ch:"exemplars_timestamp"`           // Array(DateTime64(9))
	ExemplarsValue              []float64           `ch:"exemplars_value"`               // Array(Float64)
	ExemplarsFilteredAttributes []map[string]string `ch:"exemplars_filtered_attributes"` // Array(Map(LowCardinality(String), String))
	ExemplarsTraceID            []string            `ch:"exemplars_trace_id"`
	ExemplarsSpanID             []string            `ch:"exemplars_span_id"`
	ProjectID                   string              `ch:"project_id"`
}

// MetricExponentialHistogram represents an exponential histogram metric (memory-efficient distribution)
// Schema: otel_metrics_exponential_histogram (24 columns)
// OTLP 1.38+: Modern histogram using exponential bucketing (base-2 by default)
// Note: sum, min, max are Nullable(Float64) - use pointer types
type MetricExponentialHistogram struct {
	ResourceAttributes          map[string]string   `ch:"resource_attributes"`
	ServiceName                 string              `ch:"service_name"` // MATERIALIZED from resource_attributes["service.name"]
	ScopeName                   string              `ch:"scope_name"`
	ScopeVersion                string              `ch:"scope_version"`
	ScopeAttributes             map[string]string   `ch:"scope_attributes"`
	MetricName                  string              `ch:"metric_name"`
	MetricDescription           string              `ch:"metric_description"`
	MetricUnit                  string              `ch:"metric_unit"`
	Attributes                  map[string]string   `ch:"attributes"`
	StartTimeUnix               time.Time           `ch:"start_time_unix"` // DateTime64(9) nanosecond precision
	TimeUnix                    time.Time           `ch:"time_unix"`       // DateTime64(9) nanosecond precision
	Count                       uint64              `ch:"count"`
	Sum                         *float64            `ch:"sum"`                           // Nullable(Float64)
	Scale                       int32               `ch:"scale"`                         // Exponential scale factor
	ZeroCount                   uint64              `ch:"zero_count"`                    // Count of exact zero values
	PositiveOffset              int32               `ch:"positive_offset"`               // Offset for positive buckets
	PositiveBucketCounts        []uint64            `ch:"positive_bucket_counts"`        // Array(UInt64)
	NegativeOffset              int32               `ch:"negative_offset"`               // Offset for negative buckets
	NegativeBucketCounts        []uint64            `ch:"negative_bucket_counts"`        // Array(UInt64)
	Min                         *float64            `ch:"min"`                           // Nullable(Float64)
	Max                         *float64            `ch:"max"`                           // Nullable(Float64)
	AggregationTemporality      string              `ch:"aggregation_temporality"`       // Enum8: "UNSPECIFIED", "DELTA", "CUMULATIVE"
	ResourceSchemaURL           *string             `ch:"resource_schema_url"`           // Nullable(String) - OTLP versioning
	ScopeSchemaURL              *string             `ch:"scope_schema_url"`              // Nullable(String) - OTLP versioning
	ExemplarsTimestamp          []time.Time         `ch:"exemplars_timestamp"`           // Array(DateTime64(9))
	ExemplarsValue              []float64           `ch:"exemplars_value"`               // Array(Float64)
	ExemplarsFilteredAttributes []map[string]string `ch:"exemplars_filtered_attributes"` // Array(Map(LowCardinality(String), String))
	ExemplarsTraceID            []string            `ch:"exemplars_trace_id"`
	ExemplarsSpanID             []string            `ch:"exemplars_span_id"`
	ProjectID                   string              `ch:"project_id"`
}

// ==================================
// Aggregation Temporality Constants
// ==================================
// OTLP spec: AggregationTemporality enum for Sum and Histogram
// ClickHouse: Enum8('UNSPECIFIED'=0, 'DELTA'=1, 'CUMULATIVE'=2)
// Driver accepts enum names as strings
const (
	AggregationTemporalityUnspecified = "UNSPECIFIED" // Unknown or not applicable
	AggregationTemporalityDelta       = "DELTA"       // Value since last report (resets)
	AggregationTemporalityCumulative  = "CUMULATIVE"  // Value since process start (monotonic)
)

// ConvertAggregationTemporality converts OTLP protobuf enum to string
// OTLP spec: 0=UNSPECIFIED, 1=DELTA, 2=CUMULATIVE
func ConvertAggregationTemporality(temporality int32) string {
	switch temporality {
	case 1:
		return AggregationTemporalityDelta
	case 2:
		return AggregationTemporalityCumulative
	default:
		return AggregationTemporalityUnspecified
	}
}
