package dashboard

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// QueryExecutionRequest represents a request to execute widget queries
type QueryExecutionRequest struct {
	ProjectID      uuid.UUID              `json:"project_id"`
	DashboardID    uuid.UUID              `json:"dashboard_id"`
	WidgetID       *string                `json:"widget_id,omitempty"` // nil = all widgets
	TimeRange      *TimeRange             `json:"time_range,omitempty"`
	ForceRefresh   bool                   `json:"force_refresh,omitempty"`
	VariableValues map[string]interface{} `json:"variable_values,omitempty" swaggertype:"object"`
}

// VariableOptionsRequest represents a request to get variable options
type VariableOptionsRequest struct {
	ProjectID uuid.UUID `json:"project_id"`
	View      ViewType  `json:"view"`
	Dimension string    `json:"dimension"`
	Limit     int       `json:"limit,omitempty"`
}

// VariableOptionsResponse represents the response for variable options
type VariableOptionsResponse struct {
	Values []string `json:"values"`
}

// QueryResult represents the result of a widget query
type QueryResult struct {
	WidgetID string                   `json:"widget_id"`
	Data     []map[string]interface{} `json:"data" swaggertype:"array,object"`
	Metadata *QueryMetadata           `json:"metadata,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

// QueryMetadata contains metadata about the query execution
type QueryMetadata struct {
	ExecutedAt     time.Time  `json:"executed_at"`
	DurationMs     int64      `json:"duration_ms"`
	RowCount       int        `json:"row_count"`
	Cached         bool       `json:"cached"`
	CacheExpiresAt *time.Time `json:"cache_expires_at,omitempty"`
}

// DashboardQueryResults contains results for all widgets in a dashboard
type DashboardQueryResults struct {
	DashboardID uuid.UUID               `json:"dashboard_id"`
	Results     map[string]*QueryResult `json:"results"` // keyed by widget_id
	ExecutedAt  time.Time               `json:"executed_at"`
}

// TraceListItem represents a single trace in trace_list widget
type TraceListItem struct {
	TraceID      string    `json:"trace_id"`
	Name         string    `json:"name"`
	StartTime    time.Time `json:"start_time"`
	DurationNano int64     `json:"duration_nano"`
	StatusCode   int       `json:"status_code"`
	TotalCost    *float64  `json:"total_cost,omitempty"`
	ModelName    *string   `json:"model_name,omitempty"`
	ProviderName *string   `json:"provider_name,omitempty"`
	ServiceName  *string   `json:"service_name,omitempty"`
}

// HistogramBucket represents a single bucket in a histogram
type HistogramBucket struct {
	LowerBound float64 `json:"lower_bound"`
	UpperBound float64 `json:"upper_bound"`
	Count      int64   `json:"count"`
}

// HistogramData represents histogram query results
type HistogramData struct {
	Buckets []HistogramBucket `json:"buckets"`
	Stats   *HistogramStats   `json:"stats,omitempty"`
}

// HistogramStats contains optional statistics for histogram
type HistogramStats struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
}

// ViewDefinitionResponse is the API response for view definitions
type ViewDefinitionResponse struct {
	Views map[ViewType]*ViewDefinitionPublic `json:"views"`
}

// ViewDefinitionPublic is the public representation of a view definition
type ViewDefinitionPublic struct {
	Name        ViewType          `json:"name"`
	Description string            `json:"description"`
	Measures    []MeasurePublic   `json:"measures"`
	Dimensions  []DimensionPublic `json:"dimensions"`
}

// MeasurePublic is the public representation of a measure
type MeasurePublic struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Unit        string `json:"unit"`
}

// DimensionPublic is the public representation of a dimension
type DimensionPublic struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	ColumnType  string `json:"column_type"`
	Bucketable  bool   `json:"bucketable"`
}

// WidgetQueryService defines the interface for executing widget queries
type WidgetQueryService interface {
	// ExecuteWidgetQuery executes a single widget query
	ExecuteWidgetQuery(ctx context.Context, projectID uuid.UUID, widget *Widget, timeRange *TimeRange) (*QueryResult, error)

	// ExecuteDashboardQueries executes all widget queries for a dashboard
	ExecuteDashboardQueries(ctx context.Context, req *QueryExecutionRequest) (*DashboardQueryResults, error)

	// GetViewDefinitions returns available view definitions for the query builder
	GetViewDefinitions(ctx context.Context) (*ViewDefinitionResponse, error)

	// GetVariableOptions returns distinct values for a dimension to populate variable dropdowns
	GetVariableOptions(ctx context.Context, req *VariableOptionsRequest) (*VariableOptionsResponse, error)
}

// WidgetQueryRepository defines the interface for widget query data access
type WidgetQueryRepository interface {
	// ExecuteQuery executes a raw query and returns results
	ExecuteQuery(ctx context.Context, query string, args []interface{}) ([]map[string]interface{}, error)

	// ExecuteTraceListQuery executes a trace list query
	ExecuteTraceListQuery(ctx context.Context, query string, args []interface{}) ([]*TraceListItem, error)

	// ExecuteHistogramQuery executes a histogram query
	ExecuteHistogramQuery(ctx context.Context, query string, args []interface{}) (*HistogramData, error)
}
