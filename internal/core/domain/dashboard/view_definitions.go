package dashboard

// MeasureType defines the aggregation type for a measure
type MeasureType string

const (
	MeasureTypeCount     MeasureType = "count"
	MeasureTypeSum       MeasureType = "sum"
	MeasureTypeAvg       MeasureType = "avg"
	MeasureTypeMin       MeasureType = "min"
	MeasureTypeMax       MeasureType = "max"
	MeasureTypeP50       MeasureType = "p50"
	MeasureTypeP75       MeasureType = "p75"
	MeasureTypeP95       MeasureType = "p95"
	MeasureTypeP99       MeasureType = "p99"
	MeasureTypeDistinct  MeasureType = "distinct"
	MeasureTypeRate      MeasureType = "rate" // e.g., error_rate = errors / total
	MeasureTypeHistogram MeasureType = "histogram"
)

// UnitType defines the unit of measurement for display
type UnitType string

const (
	UnitTypeCount   UnitType = "count"
	UnitTypePercent UnitType = "percent"
	UnitTypeNano    UnitType = "ns"
	UnitTypeMicro   UnitType = "µs"
	UnitTypeMilli   UnitType = "ms"
	UnitTypeSeconds UnitType = "s"
	UnitTypeTokens  UnitType = "tokens"
	UnitTypeUSD     UnitType = "USD"
	UnitTypeBytes   UnitType = "bytes"
)

// MeasureConfig defines a measure that can be computed from a view
type MeasureConfig struct {
	ID              string      `json:"id"`
	Label           string      `json:"label"`
	Description     string      `json:"description"`
	SQL             string      `json:"sql"`              // ClickHouse aggregate expression (for GROUP BY queries)
	BaseColumn      string      `json:"base_column"`      // Raw column name (for non-aggregated queries like trace list)
	Type            MeasureType `json:"type"`             // aggregation type
	Unit            UnitType    `json:"unit"`             // display unit
	Format          string      `json:"format"`           // optional format string
	Dependencies    []string    `json:"dependencies"`     // other measures this depends on
	HistogramColumn string      `json:"histogram_column"` // column to use for histogram queries
}

// DimensionConfig defines a dimension for grouping results
type DimensionConfig struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	SQL         string `json:"sql"`         // ClickHouse expression
	ColumnType  string `json:"column_type"` // string, number, datetime, etc.
	Bucketable  bool   `json:"bucketable"`  // can be used for time bucketing
}

// ViewDefinition defines a data source view for widget queries
type ViewDefinition struct {
	Name        ViewType                   `json:"name"`
	Table       string                     `json:"table"`
	TimeColumn  string                     `json:"time_column"` // column for PREWHERE time filtering
	Description string                     `json:"description"`
	BaseFilter  string                     `json:"base_filter"` // always applied filter
	Measures    map[string]MeasureConfig   `json:"measures"`
	Dimensions  map[string]DimensionConfig `json:"dimensions"`
}

// TracesViewDefinition returns the view definition for traces (root spans)
func TracesViewDefinition() *ViewDefinition {
	return &ViewDefinition{
		Name:        ViewTypeTraces,
		Table:       "otel_traces",
		TimeColumn:  "start_time",
		Description: "Root spans representing complete traces",
		BaseFilter:  "parent_span_id IS NULL AND deleted_at IS NULL",
		Measures: map[string]MeasureConfig{
			"count": {
				ID:          "count",
				Label:       "Count",
				Description: "Total number of traces",
				SQL:         "count()",
				Type:        MeasureTypeCount,
				Unit:        UnitTypeCount,
			},
			"avg_duration": {
				ID:              "avg_duration",
				Label:           "Avg Duration",
				Description:     "Average trace duration",
				SQL:             "avgOrNull(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeAvg,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"p50_duration": {
				ID:              "p50_duration",
				Label:           "P50 Duration",
				Description:     "50th percentile trace duration",
				SQL:             "quantileOrNull(0.5)(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeP50,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"p95_duration": {
				ID:              "p95_duration",
				Label:           "P95 Duration",
				Description:     "95th percentile trace duration",
				SQL:             "quantileOrNull(0.95)(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeP95,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"p99_duration": {
				ID:              "p99_duration",
				Label:           "P99 Duration",
				Description:     "99th percentile trace duration",
				SQL:             "quantileOrNull(0.99)(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeP99,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"total_cost": {
				ID:              "total_cost",
				Label:           "Total Cost",
				Description:     "Sum of all trace costs",
				SQL:             "sum(total_cost)",
				BaseColumn:      "total_cost",
				Type:            MeasureTypeSum,
				Unit:            UnitTypeUSD,
				HistogramColumn: "total_cost",
			},
			"avg_cost": {
				ID:              "avg_cost",
				Label:           "Avg Cost",
				Description:     "Average cost per trace",
				SQL:             "avgOrNull(total_cost)",
				BaseColumn:      "total_cost",
				Type:            MeasureTypeAvg,
				Unit:            UnitTypeUSD,
				HistogramColumn: "total_cost",
			},
			"total_input_tokens": {
				ID:              "total_input_tokens",
				Label:           "Total Input Tokens",
				Description:     "Sum of input tokens",
				SQL:             "sum(usage_details['input_tokens'])",
				Type:            MeasureTypeSum,
				Unit:            UnitTypeTokens,
				HistogramColumn: "usage_details['input_tokens']",
			},
			"total_output_tokens": {
				ID:              "total_output_tokens",
				Label:           "Total Output Tokens",
				Description:     "Sum of output tokens",
				SQL:             "sum(usage_details['output_tokens'])",
				Type:            MeasureTypeSum,
				Unit:            UnitTypeTokens,
				HistogramColumn: "usage_details['output_tokens']",
			},
			"total_tokens": {
				ID:              "total_tokens",
				Label:           "Total Tokens",
				Description:     "Sum of all tokens",
				SQL:             "sum(usage_details['input_tokens'] + usage_details['output_tokens'])",
				Type:            MeasureTypeSum,
				Unit:            UnitTypeTokens,
				HistogramColumn: "usage_details['input_tokens'] + usage_details['output_tokens']",
			},
			"error_count": {
				ID:          "error_count",
				Label:       "Error Count",
				Description: "Number of traces with errors",
				SQL:         "countIf(status_code = 2)",
				Type:        MeasureTypeCount,
				Unit:        UnitTypeCount,
			},
			"error_rate": {
				ID:          "error_rate",
				Label:       "Error Rate",
				Description: "Percentage of traces with errors",
				SQL:         "if(count() = 0, null, countIf(status_code = 2) * 100.0 / count())",
				Type:        MeasureTypeRate,
				Unit:        UnitTypePercent,
			},
			"unique_users": {
				ID:          "unique_users",
				Label:       "Unique Users",
				Description: "Number of unique users",
				SQL:         "uniq(span_attributes['user_id'])",
				Type:        MeasureTypeDistinct,
				Unit:        UnitTypeCount,
			},
			"unique_sessions": {
				ID:          "unique_sessions",
				Label:       "Unique Sessions",
				Description: "Number of unique sessions",
				SQL:         "uniq(span_attributes['session_id'])",
				Type:        MeasureTypeDistinct,
				Unit:        UnitTypeCount,
			},
		},
		Dimensions: map[string]DimensionConfig{
			"time": {
				ID:          "time",
				Label:       "Time",
				Description: "Trace start time",
				SQL:         "start_time",
				ColumnType:  "datetime",
				Bucketable:  true,
			},
			"model_name": {
				ID:          "model_name",
				Label:       "Model",
				Description: "AI model name",
				SQL:         "model_name",
				ColumnType:  "string",
			},
			"provider_name": {
				ID:          "provider_name",
				Label:       "Provider",
				Description: "AI provider",
				SQL:         "provider_name",
				ColumnType:  "string",
			},
			"service_name": {
				ID:          "service_name",
				Label:       "Service",
				Description: "Service name",
				SQL:         "service_name",
				ColumnType:  "string",
			},
			"status_code": {
				ID:          "status_code",
				Label:       "Status",
				Description: "Trace status code",
				SQL:         "status_code",
				ColumnType:  "number",
			},
			"span_name": {
				ID:          "span_name",
				Label:       "Name",
				Description: "Trace name",
				SQL:         "span_name",
				ColumnType:  "string",
			},
			"span_type": {
				ID:          "span_type",
				Label:       "Type",
				Description: "Span type",
				SQL:         "span_type",
				ColumnType:  "string",
			},
			"user_id": {
				ID:          "user_id",
				Label:       "User ID",
				Description: "User identifier",
				SQL:         "span_attributes['user_id']",
				ColumnType:  "string",
			},
			"session_id": {
				ID:          "session_id",
				Label:       "Session ID",
				Description: "Session identifier",
				SQL:         "span_attributes['session_id']",
				ColumnType:  "string",
			},
			"trace_id": {
				ID:          "trace_id",
				Label:       "Trace ID",
				Description: "Trace identifier",
				SQL:         "trace_id",
				ColumnType:  "string",
			},
			"duration_nano": {
				ID:          "duration_nano",
				Label:       "Duration",
				Description: "Trace duration in nanoseconds",
				SQL:         "duration_nano",
				ColumnType:  "number",
			},
		},
	}
}

// SpansViewDefinition returns the view definition for spans (all spans)
func SpansViewDefinition() *ViewDefinition {
	return &ViewDefinition{
		Name:        ViewTypeSpans,
		Table:       "otel_traces",
		TimeColumn:  "start_time",
		Description: "All spans including nested spans",
		BaseFilter:  "deleted_at IS NULL",
		Measures: map[string]MeasureConfig{
			"count": {
				ID:          "count",
				Label:       "Count",
				Description: "Total number of spans",
				SQL:         "count()",
				Type:        MeasureTypeCount,
				Unit:        UnitTypeCount,
			},
			"avg_duration": {
				ID:              "avg_duration",
				Label:           "Avg Duration",
				Description:     "Average span duration",
				SQL:             "avgOrNull(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeAvg,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"p50_duration": {
				ID:              "p50_duration",
				Label:           "P50 Duration",
				Description:     "50th percentile span duration",
				SQL:             "quantileOrNull(0.5)(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeP50,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"p95_duration": {
				ID:              "p95_duration",
				Label:           "P95 Duration",
				Description:     "95th percentile span duration",
				SQL:             "quantileOrNull(0.95)(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeP95,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"p99_duration": {
				ID:              "p99_duration",
				Label:           "P99 Duration",
				Description:     "99th percentile span duration",
				SQL:             "quantileOrNull(0.99)(duration_nano)",
				BaseColumn:      "duration_nano",
				Type:            MeasureTypeP99,
				Unit:            UnitTypeNano,
				HistogramColumn: "duration_nano",
			},
			"total_cost": {
				ID:              "total_cost",
				Label:           "Total Cost",
				Description:     "Sum of all span costs",
				SQL:             "sum(total_cost)",
				BaseColumn:      "total_cost",
				Type:            MeasureTypeSum,
				Unit:            UnitTypeUSD,
				HistogramColumn: "total_cost",
			},
			"avg_cost": {
				ID:              "avg_cost",
				Label:           "Avg Cost",
				Description:     "Average cost per span",
				SQL:             "avgOrNull(total_cost)",
				BaseColumn:      "total_cost",
				Type:            MeasureTypeAvg,
				Unit:            UnitTypeUSD,
				HistogramColumn: "total_cost",
			},
			"total_tokens": {
				ID:              "total_tokens",
				Label:           "Total Tokens",
				Description:     "Sum of all tokens",
				SQL:             "sum(usage_details['input_tokens'] + usage_details['output_tokens'])",
				Type:            MeasureTypeSum,
				Unit:            UnitTypeTokens,
				HistogramColumn: "usage_details['input_tokens'] + usage_details['output_tokens']",
			},
			"error_count": {
				ID:          "error_count",
				Label:       "Error Count",
				Description: "Number of spans with errors",
				SQL:         "countIf(status_code = 2)",
				Type:        MeasureTypeCount,
				Unit:        UnitTypeCount,
			},
			"error_rate": {
				ID:          "error_rate",
				Label:       "Error Rate",
				Description: "Percentage of spans with errors",
				SQL:         "if(count() = 0, null, countIf(status_code = 2) * 100.0 / count())",
				Type:        MeasureTypeRate,
				Unit:        UnitTypePercent,
			},
		},
		Dimensions: map[string]DimensionConfig{
			"time": {
				ID:          "time",
				Label:       "Time",
				Description: "Span start time",
				SQL:         "start_time",
				ColumnType:  "datetime",
				Bucketable:  true,
			},
			"model_name": {
				ID:          "model_name",
				Label:       "Model",
				Description: "AI model name",
				SQL:         "model_name",
				ColumnType:  "string",
			},
			"provider_name": {
				ID:          "provider_name",
				Label:       "Provider",
				Description: "AI provider",
				SQL:         "provider_name",
				ColumnType:  "string",
			},
			"service_name": {
				ID:          "service_name",
				Label:       "Service",
				Description: "Service name",
				SQL:         "service_name",
				ColumnType:  "string",
			},
			"span_name": {
				ID:          "span_name",
				Label:       "Name",
				Description: "Span name",
				SQL:         "span_name",
				ColumnType:  "string",
			},
			"span_type": {
				ID:          "span_type",
				Label:       "Type",
				Description: "Span type",
				SQL:         "span_type",
				ColumnType:  "string",
			},
			"span_level": {
				ID:          "span_level",
				Label:       "Level",
				Description: "Span nesting level",
				SQL:         "span_level",
				ColumnType:  "string",
			},
			"status_code": {
				ID:          "status_code",
				Label:       "Status",
				Description: "Span status code",
				SQL:         "status_code",
				ColumnType:  "number",
			},
		},
	}
}

// ScoresViewDefinition returns the view definition for quality scores
func ScoresViewDefinition() *ViewDefinition {
	return &ViewDefinition{
		Name:        ViewTypeScores,
		Table:       "scores",
		TimeColumn:  "timestamp",
		Description: "Quality scores for traces and spans",
		BaseFilter:  "",
		Measures: map[string]MeasureConfig{
			"count": {
				ID:          "count",
				Label:       "Count",
				Description: "Total number of scores",
				SQL:         "count()",
				Type:        MeasureTypeCount,
				Unit:        UnitTypeCount,
			},
			"avg_score": {
				ID:              "avg_score",
				Label:           "Avg Score",
				Description:     "Average score value",
				SQL:             "avgOrNull(value)",
				BaseColumn:      "value",
				Type:            MeasureTypeAvg,
				Unit:            UnitTypeCount,
				HistogramColumn: "value",
			},
			"min_score": {
				ID:              "min_score",
				Label:           "Min Score",
				Description:     "Minimum score value",
				SQL:             "min(value)",
				BaseColumn:      "value",
				Type:            MeasureTypeMin,
				Unit:            UnitTypeCount,
				HistogramColumn: "value",
			},
			"max_score": {
				ID:              "max_score",
				Label:           "Max Score",
				Description:     "Maximum score value",
				SQL:             "max(value)",
				BaseColumn:      "value",
				Type:            MeasureTypeMax,
				Unit:            UnitTypeCount,
				HistogramColumn: "value",
			},
			"p50_score": {
				ID:              "p50_score",
				Label:           "P50 Score",
				Description:     "50th percentile score",
				SQL:             "quantileOrNull(0.5)(value)",
				BaseColumn:      "value",
				Type:            MeasureTypeP50,
				Unit:            UnitTypeCount,
				HistogramColumn: "value",
			},
			"p95_score": {
				ID:              "p95_score",
				Label:           "P95 Score",
				Description:     "95th percentile score",
				SQL:             "quantileOrNull(0.95)(value)",
				BaseColumn:      "value",
				Type:            MeasureTypeP95,
				Unit:            UnitTypeCount,
				HistogramColumn: "value",
			},
			"passing_rate": {
				ID:          "passing_rate",
				Label:       "Passing Rate",
				Description: "Percentage of scores above threshold (value >= 0.5)",
				SQL:         "if(count() = 0, null, countIf(value >= 0.5) * 100.0 / count())",
				Type:        MeasureTypeRate,
				Unit:        UnitTypePercent,
			},
			"unique_traces": {
				ID:          "unique_traces",
				Label:       "Unique Traces",
				Description: "Number of unique traces with scores",
				SQL:         "uniq(trace_id)",
				Type:        MeasureTypeDistinct,
				Unit:        UnitTypeCount,
			},
		},
		Dimensions: map[string]DimensionConfig{
			"time": {
				ID:          "time",
				Label:       "Time",
				Description: "Score creation time",
				SQL:         "timestamp",
				ColumnType:  "datetime",
				Bucketable:  true,
			},
			"name": {
				ID:          "name",
				Label:       "Name",
				Description: "Score name",
				SQL:         "name",
				ColumnType:  "string",
			},
			"source": {
				ID:          "source",
				Label:       "Source",
				Description: "Score source",
				SQL:         "source",
				ColumnType:  "string",
			},
			"type": {
				ID:          "type",
				Label:       "Type",
				Description: "Score type",
				SQL:         "type",
				ColumnType:  "string",
			},
			"trace_id": {
				ID:          "trace_id",
				Label:       "Trace ID",
				Description: "Associated trace identifier",
				SQL:         "trace_id",
				ColumnType:  "string",
			},
		},
	}
}

// GetViewDefinition returns the view definition for a given view type
func GetViewDefinition(viewType ViewType) *ViewDefinition {
	switch viewType {
	case ViewTypeTraces:
		return TracesViewDefinition()
	case ViewTypeSpans:
		return SpansViewDefinition()
	case ViewTypeScores:
		return ScoresViewDefinition()
	default:
		return nil
	}
}

// GetAllViewDefinitions returns all available view definitions
func GetAllViewDefinitions() map[ViewType]*ViewDefinition {
	return map[ViewType]*ViewDefinition{
		ViewTypeTraces: TracesViewDefinition(),
		ViewTypeSpans:  SpansViewDefinition(),
		ViewTypeScores: ScoresViewDefinition(),
	}
}

// ValidMeasures returns the list of valid measure IDs for a view type
func ValidMeasures(viewType ViewType) []string {
	def := GetViewDefinition(viewType)
	if def == nil {
		return nil
	}
	measures := make([]string, 0, len(def.Measures))
	for id := range def.Measures {
		measures = append(measures, id)
	}
	return measures
}

// ValidDimensions returns the list of valid dimension IDs for a view type
func ValidDimensions(viewType ViewType) []string {
	def := GetViewDefinition(viewType)
	if def == nil {
		return nil
	}
	dimensions := make([]string, 0, len(def.Dimensions))
	for id := range def.Dimensions {
		dimensions = append(dimensions, id)
	}
	return dimensions
}
