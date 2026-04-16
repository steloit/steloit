package observability

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
)

// SpanEvent represents an OTLP span event (Nested type for ClickHouse)
type SpanEvent struct {
	Timestamp              time.Time         `json:"timestamp" db:"timestamp"`
	Name                   string            `json:"name" db:"name"`
	Attributes             map[string]string `json:"attributes" db:"attributes"`
	DroppedAttributesCount uint32            `json:"dropped_attributes_count" db:"dropped_attributes_count"`
}

// SpanLink represents an OTLP span link (Nested type for ClickHouse)
type SpanLink struct {
	TraceID                string            `json:"trace_id" db:"trace_id"`
	SpanID                 string            `json:"span_id" db:"span_id"`
	TraceState             string            `json:"trace_state" db:"trace_state"`
	Attributes             map[string]string `json:"attributes" db:"attributes"`
	DroppedAttributesCount uint32            `json:"dropped_attributes_count" db:"dropped_attributes_count"`
}

// TraceSummary represents on-demand aggregated trace-level metrics
// Computed from spans via GROUP BY queries (OTEL-native approach)
// Note: Traces are virtual in OTLP - they are derived from root spans (parent_span_id IS NULL)
type TraceSummary struct {
	TraceID        string          `json:"trace_id" db:"trace_id"`
	RootSpanID     string          `json:"root_span_id" db:"root_span_id"`
	ProjectID      string          `json:"project_id" db:"project_id"`
	Name           string          `json:"name" db:"name"` // Root span's span_name
	StartTime      time.Time       `json:"start_time" db:"start_time"`
	EndTime        *time.Time      `json:"end_time,omitempty" db:"end_time"` // Nullable for in-flight traces
	Duration       *uint64         `json:"duration,omitempty" db:"duration"` // Nullable for in-flight traces (nanoseconds)
	TotalCost      decimal.Decimal `json:"total_cost" db:"total_cost"`
	InputTokens    uint64          `json:"input_tokens" db:"input_tokens"`
	OutputTokens   uint64          `json:"output_tokens" db:"output_tokens"`
	TotalTokens    uint64          `json:"total_tokens" db:"total_tokens"`
	SpanCount      int64           `json:"span_count" db:"span_count"`
	ErrorSpanCount int64           `json:"error_span_count" db:"error_span_count"`
	HasError       bool            `json:"has_error" db:"has_error"`
	StatusCode     *uint8          `json:"status_code,omitempty" db:"status_code"` // Root span's status code
	ServiceName    *string         `json:"service_name,omitempty" db:"service_name"`
	ModelName      *string         `json:"model_name,omitempty" db:"model_name"`
	ProviderName   *string         `json:"provider_name,omitempty" db:"provider_name"`
	UserID         *string         `json:"user_id,omitempty" db:"user_id"`
	SessionID      *string         `json:"session_id,omitempty" db:"session_id"`
	Tags           []string        `json:"tags,omitempty" db:"tags"`   // User-managed tags for organization
	Bookmarked     bool            `json:"bookmarked" db:"bookmarked"` // User-managed bookmark status
}

// Span represents an OTEL span with Gen AI semantic conventions and Brokle extensions
type Span struct {
	StartTime     time.Time  `json:"start_time" db:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty" db:"end_time"`
	Duration      *uint64    `json:"duration,omitempty" db:"duration"` // Nanoseconds (OTLP spec)
	StatusMessage *string    `json:"status_message,omitempty" db:"status_message"`
	ParentSpanID  *string    `json:"parent_span_id,omitempty" db:"parent_span_id"`

	TraceState     *string `json:"trace_state,omitempty" db:"trace_state"`
	Input          *string `json:"input,omitempty" db:"input"`
	Output         *string `json:"output,omitempty" db:"output"`
	TraceID        string  `json:"trace_id" db:"trace_id"`
	SpanName       string  `json:"span_name" db:"span_name"`
	SpanID         string  `json:"span_id" db:"span_id"`
	ProjectID      string  `json:"project_id" db:"project_id"`
	OrganizationID string  `json:"organization_id" db:"organization_id"`

	Events []SpanEvent `json:"events,omitempty" db:"events"`
	Links  []SpanLink  `json:"links,omitempty" db:"links"`

	ResourceAttributes map[string]string          `json:"resource_attributes,omitempty" db:"resource_attributes"`
	SpanAttributes     map[string]string          `json:"span_attributes,omitempty" db:"span_attributes"`
	ScopeName          *string                    `json:"scope_name,omitempty" db:"scope_name"`
	ScopeVersion       *string                    `json:"scope_version,omitempty" db:"scope_version"`
	ScopeAttributes    map[string]string          `json:"scope_attributes,omitempty" db:"scope_attributes"`
	ResourceSchemaURL  *string                    `json:"resource_schema_url,omitempty" db:"resource_schema_url"`
	ScopeSchemaURL     *string                    `json:"scope_schema_url,omitempty" db:"scope_schema_url"`
	UsageDetails       map[string]uint64          `json:"usage_details,omitempty" db:"usage_details"`
	CostDetails        map[string]decimal.Decimal `json:"cost_details,omitempty" db:"cost_details"`
	PricingSnapshot    map[string]decimal.Decimal `json:"pricing_snapshot,omitempty" db:"pricing_snapshot"`
	TotalCost          *decimal.Decimal           `json:"total_cost,omitempty" db:"total_cost"`

	Version             *string    `json:"version,omitempty" db:"version"`
	CompletionStartTime *time.Time `json:"completion_start_time,omitempty" db:"completion_start_time"`

	ModelName    *string `json:"model_name,omitempty" db:"-"`
	ProviderName *string `json:"provider_name,omitempty" db:"-"`
	SpanType     *string `json:"span_type,omitempty" db:"-"`
	Level        *string `json:"level,omitempty" db:"-"`

	ServiceName *string    `json:"service_name,omitempty" db:"service_name"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`

	Scores     []*Score `json:"scores,omitempty" db:"-"`
	ChildSpans []*Span  `json:"child_spans,omitempty" db:"-"`
	StatusCode uint8    `json:"status_code" db:"status_code"`
	HasError   bool     `json:"has_error" db:"has_error"`
	SpanKind   uint8    `json:"span_kind" db:"span_kind"`
	Tags       []string `json:"tags,omitempty" db:"tags"`
	Bookmarked bool     `json:"bookmarked,omitempty" db:"bookmarked"`
}

// Score represents a quality evaluation score linked to traces and spans
type Score struct {
	// Identity
	ID             string `json:"id" db:"score_id"`
	ProjectID      string `json:"project_id" db:"project_id"`
	OrganizationID string `json:"organization_id" db:"organization_id"`

	// Links (optional - experiment-only scores may not have trace/span)
	TraceID *string `json:"trace_id,omitempty" db:"trace_id"`
	SpanID  *string `json:"span_id,omitempty" db:"span_id"`

	// Score data
	Name        string   `json:"name" db:"name"`
	Value       *float64 `json:"value,omitempty" db:"value"`
	StringValue *string  `json:"string_value,omitempty" db:"string_value"`
	Type        string   `json:"type" db:"type"`
	Source      string   `json:"source" db:"source"`

	// Additional fields
	Reason   *string         `json:"reason,omitempty" db:"reason"`
	Metadata json.RawMessage `json:"metadata" db:"metadata"`

	// Experiment tracking
	ExperimentID     *string `json:"experiment_id,omitempty" db:"experiment_id"`
	ExperimentItemID *string `json:"experiment_item_id,omitempty" db:"experiment_item_id"`

	// Audit trail (for human annotations)
	CreatedBy *string `json:"created_by,omitempty" db:"created_by"`

	// Timestamp
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
}

// Model represents an LLM/API model with pricing information (PostgreSQL)
// Used for cost calculation via internal_model_id lookup
type Model struct {
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	StartDate               *time.Time `json:"start_date,omitempty" db:"start_date"`
	ProjectID               *string    `json:"project_id,omitempty" db:"project_id"`
	TokenizerConfig         *string    `json:"tokenizer_config,omitempty" db:"tokenizer_config"`
	InputPrice              *float64   `json:"input_price,omitempty" db:"input_price"`
	OutputPrice             *float64   `json:"output_price,omitempty" db:"output_price"`
	TotalPrice              *float64   `json:"total_price,omitempty" db:"total_price"`
	TokenizerID             *string    `json:"tokenizer_id,omitempty" db:"tokenizer_id"`
	EndDate                 *time.Time `json:"end_date,omitempty" db:"end_date"`
	Provider                string     `json:"provider" db:"provider"`
	Unit                    string     `json:"unit" db:"unit"`
	ID                      string     `json:"id" db:"id"`
	MatchPattern            string     `json:"match_pattern" db:"match_pattern"`
	ModelName               string     `json:"model_name" db:"model_name"`
	BatchDiscountPercentage float64    `json:"batch_discount_percentage" db:"batch_discount_percentage"`
	CacheReadMultiplier     float64    `json:"cache_read_multiplier" db:"cache_read_multiplier"`
	CacheWriteMultiplier    float64    `json:"cache_write_multiplier" db:"cache_write_multiplier"`
	IsDeprecated            bool       `json:"is_deprecated" db:"is_deprecated"`
}

func (m *Model) IsActive() bool {
	if m.IsDeprecated {
		return false
	}
	now := time.Now()
	if m.StartDate != nil && now.Before(*m.StartDate) {
		return false
	}
	if m.EndDate != nil && now.After(*m.EndDate) {
		return false
	}
	return true
}

func (m *Model) IsGlobalPricing() bool {
	return m.ProjectID == nil
}

func (m *Model) CalculateInputCost(inputTokens int32, cacheHit bool) float64 {
	if m.InputPrice == nil {
		return 0.0
	}
	cost := (float64(inputTokens) / 1_000_000.0) * *m.InputPrice
	if cacheHit && m.CacheReadMultiplier > 0 {
		cost *= m.CacheReadMultiplier
	}
	return cost
}

func (m *Model) CalculateOutputCost(outputTokens int32) float64 {
	if m.OutputPrice == nil {
		return 0.0
	}
	return (float64(outputTokens) / 1_000_000.0) * *m.OutputPrice
}

func (m *Model) CalculateTotalCost(inputTokens, outputTokens int32, cacheHit, batchMode bool) float64 {
	inputCost := m.CalculateInputCost(inputTokens, cacheHit)
	outputCost := m.CalculateOutputCost(outputTokens)
	totalCost := inputCost + outputCost
	if batchMode && m.BatchDiscountPercentage > 0 {
		totalCost *= (1.0 - m.BatchDiscountPercentage/100.0)
	}
	return totalCost
}

func (m *Model) Validate() []ValidationError {
	var errors []ValidationError

	if m.ModelName == "" {
		errors = append(errors, ValidationError{Field: "model_name", Message: "model name is required"})
	}
	if m.MatchPattern == "" {
		errors = append(errors, ValidationError{Field: "match_pattern", Message: "match pattern is required"})
	}
	if m.Provider == "" {
		errors = append(errors, ValidationError{Field: "provider", Message: "provider is required"})
	}
	if m.Unit == "" {
		errors = append(errors, ValidationError{Field: "unit", Message: "pricing unit is required"})
	}
	if m.InputPrice == nil && m.OutputPrice == nil && m.TotalPrice == nil {
		errors = append(errors, ValidationError{Field: "pricing", Message: "at least one price (input/output/total) is required"})
	}
	if m.InputPrice != nil && *m.InputPrice < 0 {
		errors = append(errors, ValidationError{Field: "input_price", Message: "must be non-negative"})
	}
	if m.OutputPrice != nil && *m.OutputPrice < 0 {
		errors = append(errors, ValidationError{Field: "output_price", Message: "must be non-negative"})
	}
	if m.TotalPrice != nil && *m.TotalPrice < 0 {
		errors = append(errors, ValidationError{Field: "total_price", Message: "must be non-negative"})
	}

	// ReDoS protection for regex patterns
	if m.MatchPattern != "" {
		if _, err := regexp.Compile(m.MatchPattern); err != nil {
			errors = append(errors, ValidationError{Field: "match_pattern", Message: fmt.Sprintf("invalid regex pattern: %v", err)})
		}
		if len(m.MatchPattern) > 200 {
			errors = append(errors, ValidationError{Field: "match_pattern", Message: "pattern too long (max 200 characters)"})
		}
		if strings.Count(m.MatchPattern, "*") > 10 {
			errors = append(errors, ValidationError{Field: "match_pattern", Message: "pattern too complex (max 10 wildcards)"})
		}
	}

	if m.StartDate != nil && m.EndDate != nil && !m.EndDate.After(*m.StartDate) {
		errors = append(errors, ValidationError{Field: "end_date", Message: "end date must be after start date"})
	}
	if m.CacheWriteMultiplier < 0 {
		errors = append(errors, ValidationError{Field: "cache_write_multiplier", Message: "must be non-negative"})
	}
	if m.CacheReadMultiplier < 0 || m.CacheReadMultiplier > 1.0 {
		errors = append(errors, ValidationError{Field: "cache_read_multiplier", Message: "must be between 0 and 1"})
	}
	if m.BatchDiscountPercentage < 0 || m.BatchDiscountPercentage > 100 {
		errors = append(errors, ValidationError{Field: "batch_discount_percentage", Message: "must be between 0 and 100"})
	}

	return errors
}

// CostBreakdown represents detailed cost calculation result
type CostBreakdown struct {
	CacheSavings *float64 `json:"cache_savings,omitempty"`
	BatchSavings *float64 `json:"batch_savings,omitempty"`
	InputCost    string   `json:"input_cost"`
	OutputCost   string   `json:"output_cost"`
	TotalCost    string   `json:"total_cost"`
	Currency     string   `json:"currency"`
	ModelName    string   `json:"model_name"`
	Provider     string   `json:"provider"`
	InputTokens  int32    `json:"input_tokens"`
	OutputTokens int32    `json:"output_tokens"`
	CacheHit     bool     `json:"cache_hit"`
	BatchMode    bool     `json:"batch_mode"`
}

// Tag constraints for user-managed trace tags
const (
	MaxTagsPerTrace = 50  // Maximum number of tags allowed per trace
	MaxTagLength    = 100 // Maximum character length per tag
)

// UpdateTraceTagsRequest represents a request to update trace tags
type UpdateTraceTagsRequest struct {
	Tags []string `json:"tags" binding:"required"`
}

// Validate validates the UpdateTraceTagsRequest
func (r *UpdateTraceTagsRequest) Validate() []ValidationError {
	var errors []ValidationError

	if len(r.Tags) > MaxTagsPerTrace {
		errors = append(errors, ValidationError{
			Field:   "tags",
			Message: fmt.Sprintf("maximum %d tags allowed, got %d", MaxTagsPerTrace, len(r.Tags)),
		})
	}

	for i, tag := range r.Tags {
		trimmed := strings.TrimSpace(tag)
		if len(trimmed) == 0 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("tags[%d]", i),
				Message: "empty tags not allowed",
			})
			continue
		}
		if len(tag) > MaxTagLength {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("tags[%d]", i),
				Message: fmt.Sprintf("tag exceeds max length of %d characters", MaxTagLength),
			})
		}
	}

	return errors
}

// NormalizeTags normalizes tags by lowercasing, trimming, removing duplicates, and sorting
func NormalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}

	seen := make(map[string]bool)
	normalized := make([]string, 0, len(tags))

	for _, tag := range tags {
		t := strings.ToLower(strings.TrimSpace(tag))
		if t != "" && !seen[t] {
			seen[t] = true
			normalized = append(normalized, t)
		}
	}

	// Sort for consistent ordering
	for i := 0; i < len(normalized)-1; i++ {
		for j := i + 1; j < len(normalized); j++ {
			if normalized[i] > normalized[j] {
				normalized[i], normalized[j] = normalized[j], normalized[i]
			}
		}
	}

	return normalized
}

// OTEL SpanKind enum values (UInt8 in ClickHouse)
const (
	SpanKindUnspecified uint8 = 0 // SPAN_KIND_UNSPECIFIED
	SpanKindInternal    uint8 = 1 // SPAN_KIND_INTERNAL
	SpanKindServer      uint8 = 2 // SPAN_KIND_SERVER
	SpanKindClient      uint8 = 3 // SPAN_KIND_CLIENT
	SpanKindProducer    uint8 = 4 // SPAN_KIND_PRODUCER
	SpanKindConsumer    uint8 = 5 // SPAN_KIND_CONSUMER
)

// SpanKind string constants for backwards compatibility
const (
	SpanKindUnspecifiedStr = "UNSPECIFIED"
	SpanKindInternalStr    = "INTERNAL"
	SpanKindServerStr      = "SERVER"
	SpanKindClientStr      = "CLIENT"
	SpanKindProducerStr    = "PRODUCER"
	SpanKindConsumerStr    = "CONSUMER"
)

// Brokle span type constants (stored in attributes as brokle.span.type)
const (
	SpanTypeSpan       = "span"
	SpanTypeGeneration = "generation"
	SpanTypeEvent      = "event"
	SpanTypeTool       = "tool"
	SpanTypeAgent      = "agent"
	SpanTypeChain      = "chain"
	SpanTypeRetrieval  = "retrieval"
	SpanTypeEmbedding  = "embedding"
)

// OTEL StatusCode enum values (UInt8 in ClickHouse)
const (
	StatusCodeUnset uint8 = 0 // STATUS_CODE_UNSET
	StatusCodeOK    uint8 = 1 // STATUS_CODE_OK
	StatusCodeError uint8 = 2 // STATUS_CODE_ERROR
)

// StatusCode string constants for backwards compatibility
const (
	StatusCodeUnsetStr = "UNSET"
	StatusCodeOKStr    = "OK"
	StatusCodeErrorStr = "ERROR"
)

// Span level constants
const (
	SpanLevelDebug   = "DEBUG"
	SpanLevelInfo    = "INFO"
	SpanLevelWarning = "WARNING"
	SpanLevelError   = "ERROR"
	SpanLevelDefault = "DEFAULT"
)

// Score type constants
const (
	ScoreTypeNumeric     = "NUMERIC"
	ScoreTypeCategorical = "CATEGORICAL"
	ScoreTypeBoolean     = "BOOLEAN"
)

// Score source constants (matches ClickHouse Enum8)
const (
	ScoreSourceAPI        = "api"        // SDK/programmatic scores
	ScoreSourceEval       = "eval"       // Automated evaluations (LLM, code, rules)
	ScoreSourceAnnotation = "annotation" // Human annotations
)

// UnmarshalJSON handles input/output fields that may be strings, objects, or arrays from SDK
func (s *Span) UnmarshalJSON(data []byte) error {
	type Alias Span
	aux := &struct {
		*Alias
		Input  json.RawMessage `json:"input,omitempty"`
		Output json.RawMessage `json:"output,omitempty"`
	}{Alias: (*Alias)(s)}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Input) > 0 {
		s.Input = normalizeJSONField(aux.Input)
	}
	if len(aux.Output) > 0 {
		s.Output = normalizeJSONField(aux.Output)
	}
	return nil
}

func normalizeJSONField(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return &str
	}
	jsonStr := string(raw)
	return &jsonStr
}

func (s *Span) IsCompleted() bool { return s.EndTime != nil }
func (s *Span) HasParent() bool   { return s.ParentSpanID != nil && *s.ParentSpanID != "" }
func (s *Span) IsRootSpan() bool  { return s.ParentSpanID == nil || *s.ParentSpanID == "" }

func (s *Span) CalculateDuration() {
	if s.EndTime != nil && s.Duration == nil {
		duration := uint64(s.EndTime.Sub(s.StartTime).Nanoseconds())
		s.Duration = &duration
	}
}

func (s *Span) GetTotalCost() decimal.Decimal {
	if s.TotalCost != nil {
		return *s.TotalCost
	}
	total := decimal.Zero
	if s.CostDetails != nil {
		for _, cost := range s.CostDetails {
			total = total.Add(cost)
		}
	}
	return total
}

func (s *Span) GetTotalTokens() uint64 {
	if s.UsageDetails != nil {
		if total, ok := s.UsageDetails["total"]; ok {
			return total
		}
		var sum uint64
		if input, ok := s.UsageDetails["input"]; ok {
			sum += input
		}
		if output, ok := s.UsageDetails["output"]; ok {
			sum += output
		}
		return sum
	}
	return 0
}

func (s *Score) GetScoreLevel() string {
	switch s.Type {
	case ScoreTypeNumeric, ScoreTypeBoolean:
		if s.Value != nil {
			if *s.Value >= 0.8 {
				return "excellent"
			} else if *s.Value >= 0.6 {
				return "good"
			} else if *s.Value >= 0.4 {
				return "fair"
			}
			return "poor"
		}
	case ScoreTypeCategorical:
		if s.StringValue != nil {
			return *s.StringValue
		}
	}
	return "unknown"
}

func (s *Score) IsNumeric() bool     { return s.Type == ScoreTypeNumeric }
func (s *Score) IsCategorical() bool { return s.Type == ScoreTypeCategorical }
func (s *Score) IsBoolean() bool     { return s.Type == ScoreTypeBoolean }

func ConvertStatusCodeToEnum(statusStr string) uint8 {
	switch statusStr {
	case StatusCodeOKStr:
		return StatusCodeOK
	case StatusCodeErrorStr:
		return StatusCodeError
	case StatusCodeUnsetStr, "":
		return StatusCodeUnset
	default:
		return StatusCodeUnset
	}
}

func ConvertStatusCodeToString(statusCode uint8) string {
	switch statusCode {
	case StatusCodeOK:
		return StatusCodeOKStr
	case StatusCodeError:
		return StatusCodeErrorStr
	case StatusCodeUnset:
		return StatusCodeUnsetStr
	default:
		return StatusCodeUnsetStr
	}
}

func ConvertSpanKindToEnum(kindStr string) uint8 {
	switch kindStr {
	case SpanKindInternalStr:
		return SpanKindInternal
	case SpanKindServerStr:
		return SpanKindServer
	case SpanKindClientStr:
		return SpanKindClient
	case SpanKindProducerStr:
		return SpanKindProducer
	case SpanKindConsumerStr:
		return SpanKindConsumer
	case SpanKindUnspecifiedStr, "":
		return SpanKindUnspecified
	default:
		return SpanKindUnspecified
	}
}

func ConvertSpanKindToString(spanKind uint8) string {
	switch spanKind {
	case SpanKindInternal:
		return SpanKindInternalStr
	case SpanKindServer:
		return SpanKindServerStr
	case SpanKindClient:
		return SpanKindClientStr
	case SpanKindProducer:
		return SpanKindProducerStr
	case SpanKindConsumer:
		return SpanKindConsumerStr
	case SpanKindUnspecified:
		return SpanKindUnspecifiedStr
	default:
		return SpanKindUnspecifiedStr
	}
}

type TelemetryEventType string

const (
	TelemetryEventTypeTrace                      TelemetryEventType = "trace"
	TelemetryEventTypeSession                    TelemetryEventType = "session"
	TelemetryEventTypeSpan                       TelemetryEventType = "span"
	TelemetryEventTypeQualityScore               TelemetryEventType = "quality_score"
	TelemetryEventTypeMetricSum                  TelemetryEventType = "metric_sum"
	TelemetryEventTypeMetricGauge                TelemetryEventType = "metric_gauge"
	TelemetryEventTypeMetricHistogram            TelemetryEventType = "metric_histogram"
	TelemetryEventTypeMetricExponentialHistogram TelemetryEventType = "metric_exponential_histogram"
	TelemetryEventTypeLog                        TelemetryEventType = "log"
	TelemetryEventTypeGenAIEvent                 TelemetryEventType = "genai_event"
)

type TelemetryEventDeduplication struct {
	FirstSeenAt time.Time `json:"first_seen_at" db:"first_seen_at"`
	ExpiresAt   time.Time `json:"expires_at" db:"expires_at"`
	EventID     string    `json:"event_id" db:"event_id"`
	BatchID     uuid.UUID `json:"batch_id" db:"batch_id"`
	ProjectID   uuid.UUID `json:"project_id" db:"project_id"`
}

func (d *TelemetryEventDeduplication) IsExpired() bool                { return time.Now().After(d.ExpiresAt) }
func (d *TelemetryEventDeduplication) TimeUntilExpiry() time.Duration { return time.Until(d.ExpiresAt) }

func (d *TelemetryEventDeduplication) Validate() []ValidationError {
	var errors []ValidationError
	if d.EventID == "" {
		errors = append(errors, ValidationError{Field: "event_id", Message: "event_id is required"})
	}
	if d.BatchID == uuid.Nil {
		errors = append(errors, ValidationError{Field: "batch_id", Message: "batch_id is required"})
	}
	if d.ProjectID == uuid.Nil {
		errors = append(errors, ValidationError{Field: "project_id", Message: "project_id is required"})
	}
	return errors
}
