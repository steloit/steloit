package observability

import (
	"context"
	"errors"
	"time"

	"brokle/pkg/ulid"
)

type TraceService interface {
	IngestSpan(ctx context.Context, span *Span) error
	IngestSpanBatch(ctx context.Context, spans []*Span) error

	GetSpan(ctx context.Context, spanID string) (*Span, error)
	GetSpanByProject(ctx context.Context, spanID string, projectID string) (*Span, error)
	GetSpansByFilter(ctx context.Context, filter *SpanFilter) ([]*Span, error)
	CountSpans(ctx context.Context, filter *SpanFilter) (int64, error)

	GetTrace(ctx context.Context, traceID string) (*TraceSummary, error)
	GetTraceSpans(ctx context.Context, traceID string) ([]*Span, error)
	GetTraceTree(ctx context.Context, traceID string) ([]*Span, error)
	GetRootSpan(ctx context.Context, traceID string) (*Span, error)
	ListTraces(ctx context.Context, filter *TraceFilter) ([]*TraceSummary, error)
	CountTraces(ctx context.Context, filter *TraceFilter) (int64, error)

	GetTracesBySession(ctx context.Context, sessionID string) ([]*TraceSummary, error)
	GetTracesByUser(ctx context.Context, userID string, filter *TraceFilter) ([]*TraceSummary, error)
	CalculateTraceCost(ctx context.Context, traceID string) (float64, error)
	CalculateTraceTokens(ctx context.Context, traceID string) (uint64, error)

	DeleteSpan(ctx context.Context, spanID string) error
	DeleteTrace(ctx context.Context, traceID string) error
	UpdateTraceTags(ctx context.Context, projectID, traceID string, tags []string) ([]string, error)
	UpdateTraceBookmark(ctx context.Context, projectID, traceID string, bookmarked bool) error
}

// ScoreService used by both workers (CreateScore) and handlers (GetScoresByTraceID, etc.)
type ScoreService interface {
	CreateScore(ctx context.Context, score *Score) error
	CreateScoreBatch(ctx context.Context, scores []*Score) error

	GetScoreByID(ctx context.Context, id string) (*Score, error)
	GetScoresByTraceID(ctx context.Context, traceID string) ([]*Score, error)
	GetScoresBySpanID(ctx context.Context, spanID string) ([]*Score, error)
	GetScoresByFilter(ctx context.Context, filter *ScoreFilter) ([]*Score, error)

	UpdateScore(ctx context.Context, score *Score) error

	DeleteScore(ctx context.Context, id string) error

	CountScores(ctx context.Context, filter *ScoreFilter) (int64, error)
}

type MetricsService interface {
	CreateMetricSumBatch(ctx context.Context, metricsSums []*MetricSum) error
	CreateMetricGaugeBatch(ctx context.Context, metricsGauges []*MetricGauge) error
	CreateMetricHistogramBatch(ctx context.Context, metricsHistograms []*MetricHistogram) error
	CreateMetricExponentialHistogramBatch(ctx context.Context, metricsExpHistograms []*MetricExponentialHistogram) error
}

type LogsService interface {
	CreateLogBatch(ctx context.Context, logs []*Log) error
}

type GenAIEventsService interface {
	CreateGenAIEventBatch(ctx context.Context, events []*GenAIEvent) error
}

type TelemetryDeduplicationService interface {
	ClaimEvents(ctx context.Context, projectID ulid.ULID, batchID ulid.ULID, dedupIDs []string, ttl time.Duration) (claimedIDs, duplicateIDs []string, err error)
	ReleaseEvents(ctx context.Context, dedupIDs []string) error

	// Deprecated: Use ClaimEvents instead.
	CheckDuplicate(ctx context.Context, dedupID string) (bool, error)
	CheckBatchDuplicates(ctx context.Context, dedupIDs []string) ([]string, error)
	RegisterEvent(ctx context.Context, dedupID string, batchID ulid.ULID, projectID ulid.ULID, ttl time.Duration) error
	RegisterProcessedEventsBatch(ctx context.Context, projectID ulid.ULID, batchID ulid.ULID, dedupIDs []string) error

	// TTL management (string-based for composite IDs)
	CalculateOptimalTTL(ctx context.Context, dedupID string, defaultTTL time.Duration) (time.Duration, error)
	GetExpirationTime(dedupID string, baseTTL time.Duration) time.Time

	// Cleanup operations
	CleanupExpired(ctx context.Context) (int64, error)
	CleanupByProject(ctx context.Context, projectID ulid.ULID, olderThan time.Time) (int64, error)
	BatchCleanup(ctx context.Context, olderThan time.Time, batchSize int) (int64, error)

	// Redis fallback management
	SyncToRedis(ctx context.Context, entries []*TelemetryEventDeduplication) error
	ValidateRedisHealth(ctx context.Context) (*RedisHealthStatus, error)
	GetDeduplicationStats(ctx context.Context, projectID ulid.ULID) (*DeduplicationStats, error)

	// Performance monitoring
	GetCacheHitRate(ctx context.Context, timeWindow time.Duration) (float64, error)
	GetFallbackRate(ctx context.Context, timeWindow time.Duration) (float64, error)
}

type TelemetryService interface {
	Deduplication() TelemetryDeduplicationService

	GetHealth(ctx context.Context) (*TelemetryHealthStatus, error)
	GetMetrics(ctx context.Context) (*TelemetryMetrics, error)
	GetPerformanceStats(ctx context.Context, timeWindow time.Duration) (*TelemetryPerformanceStats, error)
}

type QualityEvaluator interface {
	Name() string
	Version() string
	Description() string
	SupportedTypes() []string // Span types: span, generation, event, tool, etc.
	Evaluate(ctx context.Context, input *EvaluationInput) (*Score, error)
	ValidateInput(input *EvaluationInput) error
}

type SpanCompletion struct {
	EndTime        time.Time        `json:"end_time"`
	Output         map[string]any   `json:"output,omitempty"`
	Usage          *TokenUsage      `json:"usage,omitempty"`
	Cost           *CostCalculation `json:"cost,omitempty"`
	QualityScore   *float64         `json:"quality_score,omitempty"`
	StatusMessage  *string          `json:"status_message,omitempty"`
	AdditionalData map[string]any   `json:"additional_data,omitempty"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type CostCalculation struct {
	Currency   string  `json:"currency"`
	Provider   string  `json:"provider"`
	Model      string  `json:"model"`
	InputCost  float64 `json:"input_cost"`
	OutputCost float64 `json:"output_cost"`
	TotalCost  float64 `json:"total_cost"`
}

// Uses Span as traces are virtual in OTLP (root spans = traces)
type BatchIngestRequest struct {
	Spans     []*Span   `json:"spans"`
	ProjectID ulid.ULID `json:"project_id"`
	Async     bool      `json:"async"`
}

type BatchIngestResult struct {
	JobID          *string               `json:"job_id,omitempty"`
	Errors         []BatchIngestionError `json:"errors,omitempty"`
	ProcessedCount int                   `json:"processed_count"`
	FailedCount    int                   `json:"failed_count"`
	Duration       time.Duration         `json:"duration"`
}

type TelemetryBatchRequest struct {
	Metadata  map[string]any           `json:"metadata"`
	Events    []*TelemetryEventRequest `json:"events"`
	ProjectID ulid.ULID                `json:"project_id"`
	Async     bool                     `json:"async"`
}

type TelemetryEventRequest struct {
	Payload   map[string]any     `json:"payload"`
	Timestamp *time.Time         `json:"timestamp,omitempty"`
	SpanID    string             `json:"span_id"`
	TraceID   string             `json:"trace_id"`
	EventType TelemetryEventType `json:"event_type"`
	EventID   ulid.ULID          `json:"event_id"`
}

func (e *TelemetryEventRequest) Validate() error {
	if e.EventType == TelemetryEventTypeSpan && e.SpanID == "" {
		return errors.New("span events must have non-empty span_id")
	}
	if e.TraceID == "" {
		return errors.New("trace_id is required for all events")
	}
	return nil
}

type TelemetryBatchResponse struct {
	JobID             *string               `json:"job_id,omitempty"`
	Errors            []TelemetryEventError `json:"errors,omitempty"`
	DuplicateEventIDs []ulid.ULID           `json:"duplicate_event_ids,omitempty"`
	ProcessedEvents   int                   `json:"processed_events"`
	DuplicateEvents   int                   `json:"duplicate_events"`
	FailedEvents      int                   `json:"failed_events"`
	ProcessingTimeMs  int                   `json:"processing_time_ms"`
	BatchID           ulid.ULID             `json:"batch_id"`
}

type TelemetryEventError struct {
	EventType    TelemetryEventType `json:"event_type"`
	ErrorCode    string             `json:"error_code"`
	ErrorMessage string             `json:"error_message"`
	EventID      ulid.ULID          `json:"event_id"`
	Retryable    bool               `json:"retryable"`
}

type BatchProcessingResult struct {
	Errors           []TelemetryEventError `json:"errors,omitempty"`
	TotalEvents      int                   `json:"total_events"`
	ProcessedEvents  int                   `json:"processed_events"`
	FailedEvents     int                   `json:"failed_events"`
	SkippedEvents    int                   `json:"skipped_events"`
	ProcessingTimeMs int                   `json:"processing_time_ms"`
	ThroughputPerSec float64               `json:"throughput_per_sec"`
	SuccessRate      float64               `json:"success_rate"`
	BatchID          ulid.ULID             `json:"batch_id"`
}

type EventProcessingResult struct {
	ProcessedEventIDs []ulid.ULID           `json:"processed_event_ids"`
	NotProcessedIDs   []ulid.ULID           `json:"not_processed_ids"`
	Errors            []TelemetryEventError `json:"errors,omitempty"`
	ProcessedCount    int                   `json:"processed_count"`
	FailedCount       int                   `json:"failed_count"`
	NotProcessedCount int                   `json:"not_processed_count"`
	RetryCount        int                   `json:"retry_count"`
	ProcessingTimeMs  int                   `json:"processing_time_ms"`
	SuccessRate       float64               `json:"success_rate"`
}

type RedisHealthStatus struct {
	LastError   *string       `json:"last_error,omitempty"`
	LatencyMs   float64       `json:"latency_ms"`
	MemoryUsage int64         `json:"memory_usage_bytes"`
	Connections int           `json:"connections"`
	Uptime      time.Duration `json:"uptime"`
	Available   bool          `json:"available"`
}

type DeduplicationStats struct {
	ProjectID         ulid.ULID `json:"project_id"`
	TotalChecks       int64     `json:"total_checks"`
	CacheHits         int64     `json:"cache_hits"`
	CacheMisses       int64     `json:"cache_misses"`
	DatabaseFallbacks int64     `json:"database_fallbacks"`
	DuplicatesFound   int64     `json:"duplicates_found"`
	CacheHitRate      float64   `json:"cache_hit_rate"`
	FallbackRate      float64   `json:"fallback_rate"`
	AverageLatencyMs  float64   `json:"average_latency_ms"`
}

type TelemetryHealthStatus struct {
	Database              *DatabaseHealth    `json:"database"`
	Redis                 *RedisHealthStatus `json:"redis"`
	ProcessingQueue       *QueueHealth       `json:"processing_queue"`
	ActiveWorkers         int                `json:"active_workers"`
	AverageProcessingTime float64            `json:"average_processing_time_ms"`
	ThroughputPerMinute   float64            `json:"throughput_per_minute"`
	ErrorRate             float64            `json:"error_rate"`
	Healthy               bool               `json:"healthy"`
}

type DatabaseHealth struct {
	Connected         bool    `json:"connected"`
	LatencyMs         float64 `json:"latency_ms"`
	ActiveConnections int     `json:"active_connections"`
	MaxConnections    int     `json:"max_connections"`
}

type QueueHealth struct {
	Size             int64   `json:"size"`
	ProcessingRate   float64 `json:"processing_rate"`
	AverageWaitTime  float64 `json:"average_wait_time_ms"`
	OldestMessageAge float64 `json:"oldest_message_age_ms"`
}

type TelemetryMetrics struct {
	TotalBatches          int64   `json:"total_batches"`
	CompletedBatches      int64   `json:"completed_batches"`
	FailedBatches         int64   `json:"failed_batches"`
	ProcessingBatches     int64   `json:"processing_batches"`
	TotalEvents           int64   `json:"total_events"`
	ProcessedEvents       int64   `json:"processed_events"`
	FailedEvents          int64   `json:"failed_events"`
	DuplicateEvents       int64   `json:"duplicate_events"`
	AverageEventsPerBatch float64 `json:"average_events_per_batch"`
	ThroughputPerSecond   float64 `json:"throughput_per_second"`
	SuccessRate           float64 `json:"success_rate"`
	DeduplicationRate     float64 `json:"deduplication_rate"`
}

type TelemetryPerformanceStats struct {
	TimeWindow           time.Duration `json:"time_window"`
	TotalRequests        int64         `json:"total_requests"`
	SuccessfulRequests   int64         `json:"successful_requests"`
	AverageLatencyMs     float64       `json:"average_latency_ms"`
	P95LatencyMs         float64       `json:"p95_latency_ms"`
	P99LatencyMs         float64       `json:"p99_latency_ms"`
	ThroughputPerSecond  float64       `json:"throughput_per_second"`
	PeakThroughput       float64       `json:"peak_throughput"`
	CacheHitRate         float64       `json:"cache_hit_rate"`
	DatabaseFallbackRate float64       `json:"database_fallback_rate"`
	ErrorRate            float64       `json:"error_rate"`
	RetryRate            float64       `json:"retry_rate"`
}

type SpanBatchRequest struct {
	Spans     []*Span   `json:"spans"`
	ProjectID ulid.ULID `json:"project_id"`
	Async     bool      `json:"async"`
}

type QualityScoreBatchRequest struct {
	QualityScores []*Score  `json:"quality_scores"`
	ProjectID     ulid.ULID `json:"project_id"`
	Async         bool      `json:"async"`
}

type BatchIngestionError struct {
	Details any    `json:"details,omitempty"`
	Error   string `json:"error"`
	Index   int    `json:"index"`
}

type AnalyticsFilter struct {
	ProjectID *string    `json:"project_id,omitempty"`
	UserID    *string    `json:"user_id,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Provider  *string    `json:"provider,omitempty"`
	Model     *string    `json:"model,omitempty"`
	SpanType  *string    `json:"span_type,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
}

type BulkEvaluationRequest struct {
	Filter         *AnalyticsFilter `json:"filter,omitempty"`
	TraceIDs       []ulid.ULID      `json:"trace_ids,omitempty"`
	SpanIDs        []ulid.ULID      `json:"span_ids,omitempty"`
	EvaluatorNames []string         `json:"evaluator_names"`
	Async          bool             `json:"async"`
}

type BulkEvaluationResult struct {
	JobID          *string               `json:"job_id,omitempty"`
	Scores         []*Score              `json:"scores,omitempty"`
	Errors         []BulkEvaluationError `json:"errors,omitempty"`
	ProcessedCount int                   `json:"processed_count"`
	FailedCount    int                   `json:"failed_count"`
}

type BulkEvaluationError struct {
	Details any       `json:"details,omitempty"`
	Error   string    `json:"error"`
	ItemID  ulid.ULID `json:"item_id"`
}

// Uses TraceSummary as traces are virtual in OTLP.
type EvaluationInput struct {
	TraceID      *ulid.ULID    `json:"trace_id,omitempty"`
	SpanID       *ulid.ULID    `json:"span_id,omitempty"`
	TraceSummary *TraceSummary `json:"trace_summary,omitempty"`
	Span         *Span         `json:"span,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
}

type QualityEvaluatorInfo struct {
	Configuration  map[string]any `json:"configuration,omitempty"`
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Description    string         `json:"description"`
	SupportedTypes []string       `json:"supported_types"`
	IsBuiltIn      bool           `json:"is_built_in"`
}

type DashboardOverview struct {
	TopProviders   []*ProviderSummary `json:"top_providers"`
	RecentActivity []*ActivityItem    `json:"recent_activity"`
	CostTrend      []*TimeSeriesPoint `json:"cost_trend"`
	LatencyTrend   []*TimeSeriesPoint `json:"latency_trend"`
	QualityTrend   []*TimeSeriesPoint `json:"quality_trend"`
	TotalTraces    int64              `json:"total_traces"`
	TotalCost      float64            `json:"total_cost"`
	AverageLatency float64            `json:"average_latency"`
	ErrorRate      float64            `json:"error_rate"`
}

type ProviderSummary struct {
	Provider       string  `json:"provider"`
	RequestCount   int64   `json:"request_count"`
	TotalCost      float64 `json:"total_cost"`
	AverageLatency float64 `json:"average_latency"`
	ErrorRate      float64 `json:"error_rate"`
}

type ActivityItem struct {
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type OptimizationSuggestion struct {
	Metadata         map[string]any `json:"metadata,omitempty"`
	Type             string         `json:"type"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	ActionItems      []string       `json:"action_items"`
	PotentialSavings float64        `json:"potential_savings"`
	Confidence       float64        `json:"confidence"`
}

type LatencyHeatmap struct {
	Data     [][]float64 `json:"data"`
	XLabels  []string    `json:"x_labels"`
	YLabels  []string    `json:"y_labels"`
	MinValue float64     `json:"min_value"`
	MaxValue float64     `json:"max_value"`
}

type ThroughputMetrics struct {
	TimeSeries        []*TimeSeriesPoint `json:"time_series"`
	RequestsPerSecond float64            `json:"requests_per_second"`
	RequestsPerMinute float64            `json:"requests_per_minute"`
	RequestsPerHour   float64            `json:"requests_per_hour"`
	PeakThroughput    float64            `json:"peak_throughput"`
}

type QueueStatus struct {
	TraceQueue   *QueueInfo `json:"trace_queue"`
	SpanQueue    *QueueInfo `json:"span_queue"`
	QualityQueue *QueueInfo `json:"quality_queue"`
	TotalPending int64      `json:"total_pending"`
	IsHealthy    bool       `json:"is_healthy"`
}

type QueueInfo struct {
	LastUpdated time.Time `json:"last_updated"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	Processing  int64     `json:"processing"`
	Failed      int64     `json:"failed"`
}

type IngestionHealth struct {
	LastCheck      time.Time      `json:"last_check"`
	Details        map[string]any `json:"details,omitempty"`
	Status         string         `json:"status"`
	Bottlenecks    []string       `json:"bottlenecks,omitempty"`
	IngestionRate  float64        `json:"ingestion_rate"`
	ProcessingRate float64        `json:"processing_rate"`
	ErrorRate      float64        `json:"error_rate"`
}

type IngestionMetrics struct {
	Errors         []*IngestionError `json:"recent_errors,omitempty"`
	TotalIngested  int64             `json:"total_ingested"`
	IngestedToday  int64             `json:"ingested_today"`
	ProcessingRate float64           `json:"processing_rate"`
	AverageLatency float64           `json:"average_latency"`
	QueueBacklog   int64             `json:"queue_backlog"`
	WorkerCount    int               `json:"worker_count"`
}

type IngestionError struct {
	Timestamp time.Time      `json:"timestamp"`
	Details   map[string]any `json:"details,omitempty"`
	Type      string         `json:"type"`
	Message   string         `json:"message"`
	Count     int64          `json:"count"`
}

type ExportFormat string

const (
	ExportFormatJSON    ExportFormat = "json"
	ExportFormatCSV     ExportFormat = "csv"
	ExportFormatParquet ExportFormat = "parquet"
)

type ExportResult struct {
	ExpiresAt   time.Time    `json:"expires_at"`
	DownloadURL string       `json:"download_url"`
	Format      ExportFormat `json:"format"`
	Status      string       `json:"status"`
	RecordCount int64        `json:"record_count"`
	FileSize    int64        `json:"file_size"`
}

type ReportType string

const (
	ReportTypeCost        ReportType = "cost"
	ReportTypePerformance ReportType = "performance"
	ReportTypeQuality     ReportType = "quality"
	ReportTypeUsage       ReportType = "usage"
)

type Report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Data        map[string]any `json:"data"`
	Type        ReportType     `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Format      ExportFormat   `json:"format"`
	DownloadURL string         `json:"download_url,omitempty"`
	ID          ulid.ULID      `json:"id"`
}

// Note: Analytics types (TimeSeriesPoint, ScoreStatistics, etc.) are defined in repository.go
