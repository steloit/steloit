package observability

import (
	"time"

	"github.com/google/uuid"
)

type CreateSpanRequest struct {
	StartTime        time.Time              `json:"start_time" binding:"required"`
	Input            map[string]interface{} `json:"input,omitempty"`
	QualityScore     *float64               `json:"quality_score,omitempty"`
	LatencyMs        *int                   `json:"latency_ms,omitempty"`
	TotalCost        *float64               `json:"total_cost,omitempty"`
	OutputCost       *float64               `json:"output_cost,omitempty"`
	EndTime          *time.Time             `json:"end_time,omitempty"`
	InputCost        *float64               `json:"input_cost,omitempty"`
	ModelParameters  map[string]interface{} `json:"model_parameters,omitempty"`
	Output           map[string]interface{} `json:"output,omitempty"`
	Model            string                 `json:"model,omitempty"`
	Provider         string                 `json:"provider,omitempty"`
	TraceID          string                 `json:"trace_id" binding:"required"`
	Version          string                 `json:"version,omitempty"`
	StatusMessage    string                 `json:"status_message,omitempty"`
	Level            string                 `json:"level,omitempty"`
	Name             string                 `json:"name" binding:"required"`
	Type             string                 `json:"type" binding:"required"`
	ParentSpanID     string                 `json:"parent_span_id,omitempty"`
	ExternalSpanID   string                 `json:"external_span_id" binding:"required"`
	PromptTokens     int                    `json:"prompt_tokens,omitempty"`
	CompletionTokens int                    `json:"completion_tokens,omitempty"`
	TotalTokens      int                    `json:"total_tokens,omitempty"`
}

// OTEL spans are immutable - create or delete only, never update

type SpanResponse struct {
	UpdatedAt        time.Time              `json:"updated_at"`
	CreatedAt        time.Time              `json:"created_at"`
	StartTime        time.Time              `json:"start_time"`
	Input            map[string]interface{} `json:"input,omitempty"`
	QualityScore     *float64               `json:"quality_score,omitempty"`
	LatencyMs        *int                   `json:"latency_ms,omitempty"`
	TotalCost        *float64               `json:"total_cost,omitempty"`
	EndTime          *time.Time             `json:"end_time,omitempty"`
	OutputCost       *float64               `json:"output_cost,omitempty"`
	InputCost        *float64               `json:"input_cost,omitempty"`
	ModelParameters  map[string]interface{} `json:"model_parameters,omitempty"`
	Output           map[string]interface{} `json:"output,omitempty"`
	Provider         string                 `json:"provider,omitempty"`
	Type             string                 `json:"type"`
	Model            string                 `json:"model,omitempty"`
	Version          string                 `json:"version,omitempty"`
	TraceID          string                 `json:"trace_id"`
	ExternalSpanID   string                 `json:"external_span_id"`
	ParentSpanID     string                 `json:"parent_span_id,omitempty"`
	StatusMessage    string                 `json:"status_message,omitempty"`
	Level            string                 `json:"level"`
	Name             string                 `json:"name"`
	ID               string                 `json:"id"`
	TotalTokens      int                    `json:"total_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	PromptTokens     int                    `json:"prompt_tokens"`
}

type BatchCreateSpansRequest struct {
	Spans []CreateSpanRequest `json:"spans" binding:"required"`
}

type BatchCreateSpansResponse struct {
	Spans          []SpanResponse `json:"spans"`
	ProcessedCount int            `json:"processed_count"`
}

type CreateQualityScoreRequest struct {
	TraceID          string   `json:"trace_id" binding:"required"`
	SpanID           string   `json:"span_id,omitempty"`
	ScoreName        string   `json:"score_name" binding:"required"`
	ScoreValue       *float64 `json:"score_value,omitempty"`
	StringValue      *string  `json:"string_value,omitempty"`
	Type             string   `json:"type" binding:"required"`
	Source           string   `json:"source,omitempty"`
	EvaluatorName    string   `json:"evaluator_name,omitempty"`
	EvaluatorVersion string   `json:"evaluator_version,omitempty"`
	Comment          string   `json:"comment,omitempty"`
	AuthorUserID     string   `json:"author_user_id,omitempty"`
}

type UpdateQualityScoreRequest struct {
	ScoreValue       *float64 `json:"score_value,omitempty"`
	StringValue      *string  `json:"string_value,omitempty"`
	Comment          string   `json:"comment,omitempty"`
	EvaluatorName    string   `json:"evaluator_name,omitempty"`
	EvaluatorVersion string   `json:"evaluator_version,omitempty"`
}

type QualityScoreResponse struct {
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedAt        time.Time `json:"created_at"`
	ScoreValue       *float64  `json:"score_value,omitempty"`
	StringValue      *string   `json:"string_value,omitempty"`
	Type             string    `json:"type"`
	ScoreName        string    `json:"score_name"`
	ID               string    `json:"id"`
	Source           string    `json:"source"`
	EvaluatorName    string    `json:"evaluator_name,omitempty"`
	EvaluatorVersion string    `json:"evaluator_version,omitempty"`
	Comment          string    `json:"comment,omitempty"`
	AuthorUserID     string    `json:"author_user_id,omitempty"`
	SpanID           string    `json:"span_id,omitempty"`
	TraceID          string    `json:"trace_id"`
}

type EvaluateRequest struct {
	EvaluatorName string   `json:"evaluator_name" binding:"required"`
	TraceIDs      []string `json:"trace_ids,omitempty"`
	SpanIDs       []string `json:"span_ids,omitempty"`
}

type EvaluateResponse struct {
	QualityScores  []QualityScoreResponse `json:"quality_scores"`
	Errors         []EvaluationError      `json:"errors,omitempty"`
	ProcessedCount int                    `json:"processed_count"`
	FailedCount    int                    `json:"failed_count"`
}

type EvaluationError struct {
	ItemID  string `json:"item_id"`
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

type AnalyticsFilter struct {
	ProjectID string     `json:"project_id,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Provider  string     `json:"provider,omitempty"`
	Model     string     `json:"model,omitempty"`
	SpanType  string     `json:"span_type,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
}

type DashboardOverviewResponse struct {
	TopProviders   []ProviderSummaryResponse `json:"top_providers"`
	RecentActivity []ActivityItemResponse    `json:"recent_activity"`
	CostTrend      []TimeSeriesPointResponse `json:"cost_trend"`
	LatencyTrend   []TimeSeriesPointResponse `json:"latency_trend"`
	QualityTrend   []TimeSeriesPointResponse `json:"quality_trend"`
	TotalTraces    int64                     `json:"total_traces"`
	TotalCost      float64                   `json:"total_cost"`
	AverageLatency float64                   `json:"average_latency"`
	ErrorRate      float64                   `json:"error_rate"`
}

type ProviderSummaryResponse struct {
	Provider       string  `json:"provider"`
	RequestCount   int64   `json:"request_count"`
	TotalCost      float64 `json:"total_cost"`
	AverageLatency float64 `json:"average_latency"`
	ErrorRate      float64 `json:"error_rate"`
}

type ActivityItemResponse struct {
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
}

type TimeSeriesPointResponse struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type ErrorResponse struct {
	Details interface{} `json:"details,omitempty"`
	Error   string      `json:"error"`
	Message string      `json:"message"`
	Code    string      `json:"code,omitempty"`
}

type ValidationErrorResponse struct {
	FieldErrors map[string]string `json:"field_errors,omitempty"`
	Error       string            `json:"error"`
	Message     string            `json:"message"`
}

type SortInfo struct {
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// CreateAnnotationRequest represents a request to create a human annotation score on a trace
type CreateAnnotationRequest struct {
	Name        string   `json:"name" binding:"required"`
	Value       *float64 `json:"value,omitempty"`
	StringValue *string  `json:"string_value,omitempty"`
	DataType    string   `json:"type" binding:"required,oneof=NUMERIC CATEGORICAL BOOLEAN"`
	Reason      *string  `json:"reason,omitempty"`
}

// AnnotationResponse represents a score returned from the annotation API
type AnnotationResponse struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	TraceID     *string   `json:"trace_id,omitempty"` // W3C hex
	SpanID      *string   `json:"span_id,omitempty"`  // W3C hex
	Name        string    `json:"name"`
	Value       *float64  `json:"value,omitempty"`
	StringValue *string   `json:"string_value,omitempty"`
	DataType    string    `json:"type"`
	Source      string    `json:"source"`
	Reason      *string   `json:"reason,omitempty"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	Timestamp   string    `json:"timestamp"`
}
