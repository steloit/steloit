package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TimeRange represents the time range for overview queries
type TimeRange string

const (
	TimeRange15Minutes TimeRange = "15m"
	TimeRange30Minutes TimeRange = "30m"
	TimeRange1Hour     TimeRange = "1h"
	TimeRange3Hours    TimeRange = "3h"
	TimeRange6Hours    TimeRange = "6h"
	TimeRange12Hours   TimeRange = "12h"
	TimeRange24Hours   TimeRange = "24h"
	TimeRange7Days     TimeRange = "7d"
	TimeRange14Days    TimeRange = "14d"
	TimeRange30Days    TimeRange = "30d"
	TimeRange90Days    TimeRange = "90d"
	TimeRangeAll       TimeRange = "all" // All available data (capped at 365 days for safety)
)

// ParseTimeRange parses a string into a TimeRange, defaulting to 24h
func ParseTimeRange(s string) TimeRange {
	switch s {
	case "15m":
		return TimeRange15Minutes
	case "30m":
		return TimeRange30Minutes
	case "1h":
		return TimeRange1Hour
	case "3h":
		return TimeRange3Hours
	case "6h":
		return TimeRange6Hours
	case "12h":
		return TimeRange12Hours
	case "7d":
		return TimeRange7Days
	case "14d":
		return TimeRange14Days
	case "30d":
		return TimeRange30Days
	case "90d":
		return TimeRange90Days
	case "all":
		return TimeRangeAll
	default:
		return TimeRange24Hours
	}
}

// Duration returns the duration for the time range
func (tr TimeRange) Duration() time.Duration {
	switch tr {
	case TimeRange15Minutes:
		return 15 * time.Minute
	case TimeRange30Minutes:
		return 30 * time.Minute
	case TimeRange1Hour:
		return time.Hour
	case TimeRange3Hours:
		return 3 * time.Hour
	case TimeRange6Hours:
		return 6 * time.Hour
	case TimeRange12Hours:
		return 12 * time.Hour
	case TimeRange7Days:
		return 7 * 24 * time.Hour
	case TimeRange14Days:
		return 14 * 24 * time.Hour
	case TimeRange30Days:
		return 30 * 24 * time.Hour
	case TimeRange90Days:
		return 90 * 24 * time.Hour
	case TimeRangeAll:
		return 365 * 24 * time.Hour // Capped at 365 days for safety
	default:
		return 24 * time.Hour
	}
}

// OverviewStats contains the primary metrics for the stats row
type OverviewStats struct {
	TracesCount    int64   `json:"traces_count"`
	TracesTrend    float64 `json:"traces_trend"`     // Percentage change vs previous period
	TotalCost      float64 `json:"total_cost"`       // In dollars
	CostTrend      float64 `json:"cost_trend"`       // Percentage change vs previous period
	TotalTokens    int64   `json:"total_tokens"`     // Total tokens (input + output)
	TokensTrend    float64 `json:"tokens_trend"`     // Percentage change vs previous period
	AvgLatencyMs   float64 `json:"avg_latency_ms"`   // Average latency in milliseconds
	LatencyTrend   float64 `json:"latency_trend"`    // Percentage change vs previous period
	ErrorRate      float64 `json:"error_rate"`       // Percentage of traces with errors
	ErrorRateTrend float64 `json:"error_rate_trend"` // Percentage change vs previous period
}

// TimeSeriesPoint represents a single point in a time series
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// CostByModel represents cost breakdown by model
type CostByModel struct {
	Model  string  `json:"model"`
	Cost   float64 `json:"cost"`
	Tokens int64   `json:"tokens"` // Total tokens for this model
	Count  int64   `json:"count"`  // Number of spans using this model
}

// RecentTrace represents a trace summary for the recent traces table
type RecentTrace struct {
	TraceID   string    `json:"trace_id"`
	Name      string    `json:"name"`
	LatencyMs float64   `json:"latency_ms"`
	Status    string    `json:"status"` // "success", "error"
	Timestamp time.Time `json:"timestamp"`
}

// TopError represents an error summary for the top errors table
type TopError struct {
	Message  string    `json:"message"`
	Count    int64     `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// ScoreSummary represents a score overview for the conditional score section
type ScoreSummary struct {
	Name      string            `json:"name"`
	AvgValue  float64           `json:"avg_value"`
	Trend     float64           `json:"trend"` // Percentage change vs previous period
	Sparkline []TimeSeriesPoint `json:"sparkline"`
}

// ChecklistStatus represents the onboarding checklist state
type ChecklistStatus struct {
	HasProject     bool `json:"has_project"`     // Always true (they're viewing the page)
	HasTraces      bool `json:"has_traces"`      // Has sent at least one trace
	HasAIProvider  bool `json:"has_ai_provider"` // Has configured an AI provider
	HasEvaluations bool `json:"has_evaluations"` // Has set up evaluation scores
}

// OverviewResponse is the complete response for the overview endpoint
type OverviewResponse struct {
	Stats           OverviewStats     `json:"stats"`
	TraceVolume     []TimeSeriesPoint `json:"trace_volume"`
	CostTimeSeries  []TimeSeriesPoint `json:"cost_time_series"`  // Cost over time
	TokenTimeSeries []TimeSeriesPoint `json:"token_time_series"` // Tokens over time
	ErrorTimeSeries []TimeSeriesPoint `json:"error_time_series"` // Error count over time
	CostByModel     []CostByModel     `json:"cost_by_model"`
	RecentTraces    []RecentTrace     `json:"recent_traces"`
	TopErrors       []TopError        `json:"top_errors"`
	ScoresSummary   []ScoreSummary    `json:"scores_summary,omitempty"` // Only if scores exist
	ChecklistStatus ChecklistStatus   `json:"checklist_status"`
}

// OverviewFilter contains the filter parameters for overview queries
type OverviewFilter struct {
	ProjectID uuid.UUID
	TimeRange TimeRange
	StartTime time.Time
	EndTime   time.Time
}

// NewOverviewFilter creates a new OverviewFilter with calculated time boundaries
func NewOverviewFilter(projectID uuid.UUID, timeRange TimeRange) *OverviewFilter {
	endTime := time.Now().UTC()
	startTime := endTime.Add(-timeRange.Duration())

	return &OverviewFilter{
		ProjectID: projectID,
		TimeRange: timeRange,
		StartTime: startTime,
		EndTime:   endTime,
	}
}

// PreviousPeriodStart returns the start time for the comparison period
func (f *OverviewFilter) PreviousPeriodStart() time.Time {
	var duration time.Duration
	if f.TimeRange != "" {
		// Preset time range - use its duration
		duration = f.TimeRange.Duration()
	} else {
		// Custom range - calculate from actual start/end times
		duration = f.EndTime.Sub(f.StartTime)
	}
	return f.StartTime.Add(-duration)
}

// OverviewRepository defines the data access interface for overview data
type OverviewRepository interface {
	// GetStats retrieves the primary metrics for the stats row
	GetStats(ctx context.Context, filter *OverviewFilter) (*OverviewStats, error)

	// GetTraceVolume retrieves the time series data for trace volume
	GetTraceVolume(ctx context.Context, filter *OverviewFilter) ([]TimeSeriesPoint, error)

	// GetCostTimeSeries retrieves the time series data for cost
	GetCostTimeSeries(ctx context.Context, filter *OverviewFilter) ([]TimeSeriesPoint, error)

	// GetTokenTimeSeries retrieves the time series data for tokens
	GetTokenTimeSeries(ctx context.Context, filter *OverviewFilter) ([]TimeSeriesPoint, error)

	// GetErrorTimeSeries retrieves the time series data for error count
	GetErrorTimeSeries(ctx context.Context, filter *OverviewFilter) ([]TimeSeriesPoint, error)

	// GetCostByModel retrieves the cost breakdown by model (top 5)
	GetCostByModel(ctx context.Context, filter *OverviewFilter) ([]CostByModel, error)

	// GetRecentTraces retrieves the most recent traces (top 5)
	GetRecentTraces(ctx context.Context, filter *OverviewFilter, limit int) ([]RecentTrace, error)

	// GetTopErrors retrieves the most frequent errors (top 5)
	GetTopErrors(ctx context.Context, filter *OverviewFilter, limit int) ([]TopError, error)

	// GetScoresSummary retrieves score overview data (top 3 scores)
	GetScoresSummary(ctx context.Context, filter *OverviewFilter, limit int) ([]ScoreSummary, error)

	// HasTraces checks if the project has any traces
	HasTraces(ctx context.Context, projectID uuid.UUID) (bool, error)

	// HasScores checks if the project has any scores
	HasScores(ctx context.Context, projectID uuid.UUID) (bool, error)
}

// OverviewService defines the business logic interface for overview
type OverviewService interface {
	// GetOverview retrieves the complete overview data for a project
	// Filter must contain ProjectID and either TimeRange (for presets) or StartTime/EndTime (for custom ranges)
	GetOverview(ctx context.Context, filter *OverviewFilter) (*OverviewResponse, error)
}
