package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/analytics"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type overviewRepository struct {
	db driver.Conn
}

// NewOverviewRepository creates a new overview repository instance
func NewOverviewRepository(db driver.Conn) analytics.OverviewRepository {
	return &overviewRepository{db: db}
}

// getBucketSeconds determines the appropriate bucket size for time series based on duration
func getBucketSeconds(filter *analytics.OverviewFilter) int64 {
	duration := filter.EndTime.Sub(filter.StartTime)

	switch {
	case filter.TimeRange == analytics.TimeRangeAll || duration >= 90*24*time.Hour:
		return 7 * 24 * 3600 // Weekly buckets for 90+ days
	case filter.TimeRange == analytics.TimeRange30Days || duration >= 14*24*time.Hour:
		return 24 * 3600 // Daily buckets for 14+ days
	case filter.TimeRange == analytics.TimeRange7Days || filter.TimeRange == analytics.TimeRange14Days || duration >= 3*24*time.Hour:
		return 6 * 3600 // 6-hour buckets for 3-14 days
	default:
		return 3600 // Hourly buckets for < 3 days
	}
}

// GetStats retrieves the primary metrics for the stats row with trend calculation
func (r *overviewRepository) GetStats(ctx context.Context, filter *analytics.OverviewFilter) (*analytics.OverviewStats, error) {
	// Query for current period - includes total_tokens
	currentQuery := `
		SELECT
			count(DISTINCT trace_id) as trace_count,
			toFloat64(sum(total_cost)) as total_cost,
			sum(usage_details['total']) as total_tokens,
			avg(duration_nano) / 1000000.0 as avg_latency_ms,
			countIf(status_code = 2) as error_count
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
	`

	var currentTraceCount uint64
	var currentTotalCost float64
	var currentTotalTokens uint64
	var currentAvgLatencyMs float64
	var currentErrorCount uint64

	row := r.db.QueryRow(ctx, currentQuery,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
	)
	if err := row.Scan(&currentTraceCount, &currentTotalCost, &currentTotalTokens, &currentAvgLatencyMs, &currentErrorCount); err != nil {
		return nil, fmt.Errorf("query current stats: %w", err)
	}

	// Query for previous period (for trend calculation)
	previousQuery := `
		SELECT
			count(DISTINCT trace_id) as trace_count,
			toFloat64(sum(total_cost)) as total_cost,
			sum(usage_details['total']) as total_tokens,
			avg(duration_nano) / 1000000.0 as avg_latency_ms,
			countIf(status_code = 2) as error_count
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
	`

	var prevTraceCount uint64
	var prevTotalCost float64
	var prevTotalTokens uint64
	var prevAvgLatencyMs float64
	var prevErrorCount uint64

	prevRow := r.db.QueryRow(ctx, previousQuery,
		filter.ProjectID.String(),
		filter.PreviousPeriodStart(),
		filter.StartTime,
	)
	if err := prevRow.Scan(&prevTraceCount, &prevTotalCost, &prevTotalTokens, &prevAvgLatencyMs, &prevErrorCount); err != nil {
		return nil, fmt.Errorf("query previous stats: %w", err)
	}

	// Calculate trends (percentage change)
	calcTrend := func(current, previous float64) float64 {
		if previous == 0 {
			if current > 0 {
				return 100.0 // 100% increase from zero
			}
			return 0.0
		}
		return ((current - previous) / previous) * 100.0
	}

	// Calculate error rates
	var currentErrorRate, prevErrorRate float64
	if currentTraceCount > 0 {
		currentErrorRate = (float64(currentErrorCount) / float64(currentTraceCount)) * 100.0
	}
	if prevTraceCount > 0 {
		prevErrorRate = (float64(prevErrorCount) / float64(prevTraceCount)) * 100.0
	}

	return &analytics.OverviewStats{
		TracesCount:    int64(currentTraceCount),
		TracesTrend:    calcTrend(float64(currentTraceCount), float64(prevTraceCount)),
		TotalCost:      currentTotalCost,
		CostTrend:      calcTrend(currentTotalCost, prevTotalCost),
		TotalTokens:    int64(currentTotalTokens),
		TokensTrend:    calcTrend(float64(currentTotalTokens), float64(prevTotalTokens)),
		AvgLatencyMs:   currentAvgLatencyMs,
		LatencyTrend:   calcTrend(currentAvgLatencyMs, prevAvgLatencyMs),
		ErrorRate:      currentErrorRate,
		ErrorRateTrend: calcTrend(currentErrorRate, prevErrorRate),
	}, nil
}

// GetTraceVolume retrieves trace counts for the time series chart
func (r *overviewRepository) GetTraceVolume(ctx context.Context, filter *analytics.OverviewFilter) ([]analytics.TimeSeriesPoint, error) {
	bucketSeconds := getBucketSeconds(filter)

	query := fmt.Sprintf(`
		SELECT
			toStartOfInterval(start_time, INTERVAL %d SECOND) as bucket,
			count(DISTINCT trace_id) as trace_count
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSeconds)

	rows, err := r.db.Query(ctx, query,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
	)
	if err != nil {
		return nil, fmt.Errorf("query trace volume: %w", err)
	}
	defer rows.Close()

	var result []analytics.TimeSeriesPoint
	for rows.Next() {
		var ts time.Time
		var count uint64
		if err := rows.Scan(&ts, &count); err != nil {
			return nil, fmt.Errorf("scan trace volume row: %w", err)
		}
		result = append(result, analytics.TimeSeriesPoint{
			Timestamp: ts,
			Value:     float64(count),
		})
	}

	return result, nil
}

// GetCostTimeSeries retrieves cost over time for the time series chart
func (r *overviewRepository) GetCostTimeSeries(ctx context.Context, filter *analytics.OverviewFilter) ([]analytics.TimeSeriesPoint, error) {
	bucketSeconds := getBucketSeconds(filter)

	query := fmt.Sprintf(`
		SELECT
			toStartOfInterval(start_time, INTERVAL %d SECOND) as bucket,
			toFloat64(sum(total_cost)) as total_cost
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSeconds)

	rows, err := r.db.Query(ctx, query,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
	)
	if err != nil {
		return nil, fmt.Errorf("query cost time series: %w", err)
	}
	defer rows.Close()

	var result []analytics.TimeSeriesPoint
	for rows.Next() {
		var ts time.Time
		var cost float64
		if err := rows.Scan(&ts, &cost); err != nil {
			return nil, fmt.Errorf("scan cost time series row: %w", err)
		}
		result = append(result, analytics.TimeSeriesPoint{
			Timestamp: ts,
			Value:     cost,
		})
	}

	return result, nil
}

// GetTokenTimeSeries retrieves token usage over time for the time series chart
func (r *overviewRepository) GetTokenTimeSeries(ctx context.Context, filter *analytics.OverviewFilter) ([]analytics.TimeSeriesPoint, error) {
	bucketSeconds := getBucketSeconds(filter)

	query := fmt.Sprintf(`
		SELECT
			toStartOfInterval(start_time, INTERVAL %d SECOND) as bucket,
			sum(usage_details['total']) as total_tokens
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSeconds)

	rows, err := r.db.Query(ctx, query,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
	)
	if err != nil {
		return nil, fmt.Errorf("query token time series: %w", err)
	}
	defer rows.Close()

	var result []analytics.TimeSeriesPoint
	for rows.Next() {
		var ts time.Time
		var tokens uint64
		if err := rows.Scan(&ts, &tokens); err != nil {
			return nil, fmt.Errorf("scan token time series row: %w", err)
		}
		result = append(result, analytics.TimeSeriesPoint{
			Timestamp: ts,
			Value:     float64(tokens),
		})
	}

	return result, nil
}

// GetErrorTimeSeries retrieves error count over time for the time series chart
func (r *overviewRepository) GetErrorTimeSeries(ctx context.Context, filter *analytics.OverviewFilter) ([]analytics.TimeSeriesPoint, error) {
	bucketSeconds := getBucketSeconds(filter)

	query := fmt.Sprintf(`
		SELECT
			toStartOfInterval(start_time, INTERVAL %d SECOND) as bucket,
			countIf(status_code = 2) as error_count
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSeconds)

	rows, err := r.db.Query(ctx, query,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
	)
	if err != nil {
		return nil, fmt.Errorf("query error time series: %w", err)
	}
	defer rows.Close()

	var result []analytics.TimeSeriesPoint
	for rows.Next() {
		var ts time.Time
		var errCount uint64
		if err := rows.Scan(&ts, &errCount); err != nil {
			return nil, fmt.Errorf("scan error time series row: %w", err)
		}
		result = append(result, analytics.TimeSeriesPoint{
			Timestamp: ts,
			Value:     float64(errCount),
		})
	}

	return result, nil
}

// GetCostByModel retrieves cost breakdown by model (top 5) including token counts
func (r *overviewRepository) GetCostByModel(ctx context.Context, filter *analytics.OverviewFilter) ([]analytics.CostByModel, error) {
	query := `
		SELECT
			if(model_name = '', 'unknown', model_name) as model,
			toFloat64(sum(total_cost)) as cost,
			sum(usage_details['total']) as tokens,
			count(*) as span_count
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND (total_cost > 0 OR usage_details['total'] > 0)
			AND deleted_at IS NULL
		GROUP BY model
		ORDER BY cost DESC
		LIMIT 5
	`

	rows, err := r.db.Query(ctx, query,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
	)
	if err != nil {
		return nil, fmt.Errorf("query cost by model: %w", err)
	}
	defer rows.Close()

	var result []analytics.CostByModel
	for rows.Next() {
		var model string
		var cost float64
		var tokens uint64
		var count uint64
		if err := rows.Scan(&model, &cost, &tokens, &count); err != nil {
			return nil, fmt.Errorf("scan cost by model row: %w", err)
		}
		result = append(result, analytics.CostByModel{
			Model:  model,
			Cost:   cost,
			Tokens: int64(tokens),
			Count:  int64(count),
		})
	}

	return result, nil
}

// GetRecentTraces retrieves the most recent traces within the time range
func (r *overviewRepository) GetRecentTraces(ctx context.Context, filter *analytics.OverviewFilter, limit int) ([]analytics.RecentTrace, error) {
	query := `
		SELECT
			trace_id,
			span_name,
			duration_nano / 1000000.0 as latency_ms,
			status_code,
			start_time
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
		ORDER BY start_time DESC
		LIMIT ?
	`

	rows, err := r.db.Query(ctx, query,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent traces: %w", err)
	}
	defer rows.Close()

	var result []analytics.RecentTrace
	for rows.Next() {
		var traceID, name string
		var latencyMs float64
		var statusCode uint8
		var timestamp time.Time

		if err := rows.Scan(&traceID, &name, &latencyMs, &statusCode, &timestamp); err != nil {
			return nil, fmt.Errorf("scan recent trace row: %w", err)
		}

		status := "success"
		if statusCode == 2 {
			status = "error"
		}

		result = append(result, analytics.RecentTrace{
			TraceID:   traceID,
			Name:      name,
			LatencyMs: latencyMs,
			Status:    status,
			Timestamp: timestamp,
		})
	}

	return result, nil
}

// GetTopErrors retrieves the most frequent errors
func (r *overviewRepository) GetTopErrors(ctx context.Context, filter *analytics.OverviewFilter, limit int) ([]analytics.TopError, error) {
	query := `
		SELECT
			if(status_message = '', 'Unknown error', status_message) as error_message,
			count(*) as error_count,
			max(start_time) as last_seen
		FROM otel_traces
		WHERE project_id = ?
			AND start_time >= ?
			AND start_time < ?
			AND status_code = 2
			AND deleted_at IS NULL
		GROUP BY error_message
		ORDER BY error_count DESC
		LIMIT ?
	`

	rows, err := r.db.Query(ctx, query,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query top errors: %w", err)
	}
	defer rows.Close()

	var result []analytics.TopError
	for rows.Next() {
		var message string
		var count uint64
		var lastSeen time.Time

		if err := rows.Scan(&message, &count, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan top error row: %w", err)
		}

		// Truncate long error messages
		if len(message) > 200 {
			message = message[:197] + "..."
		}

		result = append(result, analytics.TopError{
			Message:  message,
			Count:    int64(count),
			LastSeen: lastSeen,
		})
	}

	return result, nil
}

// GetScoresSummary retrieves score overview data for the top scores
func (r *overviewRepository) GetScoresSummary(ctx context.Context, filter *analytics.OverviewFilter, limit int) ([]analytics.ScoreSummary, error) {
	// First, get the top scores by count
	topScoresQuery := `
		SELECT
			name,
			avg(value) as avg_value,
			count(*) as score_count
		FROM scores
		WHERE project_id = ?
			AND timestamp >= ?
			AND timestamp < ?
		GROUP BY name
		ORDER BY score_count DESC
		LIMIT ?
	`

	rows, err := r.db.Query(ctx, topScoresQuery,
		filter.ProjectID.String(),
		filter.StartTime,
		filter.EndTime,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query top scores: %w", err)
	}
	defer rows.Close()

	type scoreInfo struct {
		name     string
		avgValue float64
	}
	var topScores []scoreInfo

	for rows.Next() {
		var name string
		var avgValue float64
		var count uint64
		if err := rows.Scan(&name, &avgValue, &count); err != nil {
			return nil, fmt.Errorf("scan top score row: %w", err)
		}
		topScores = append(topScores, scoreInfo{name: name, avgValue: avgValue})
	}

	if len(topScores) == 0 {
		return nil, nil
	}

	// Get previous period averages for trend calculation
	// Build placeholders for IN clause (ClickHouse driver doesn't expand slices)
	placeholders := make([]string, len(topScores))
	for i := range topScores {
		placeholders[i] = "?"
	}

	prevQuery := fmt.Sprintf(`
		SELECT
			name,
			avg(value) as avg_value
		FROM scores
		WHERE project_id = ?
			AND timestamp >= ?
			AND timestamp < ?
			AND name IN (%s)
		GROUP BY name
	`, strings.Join(placeholders, ", "))

	// Build args with expanded score names
	args := []any{
		filter.ProjectID.String(),
		filter.PreviousPeriodStart(),
		filter.StartTime,
	}
	for _, s := range topScores {
		args = append(args, s.name)
	}

	prevRows, err := r.db.Query(ctx, prevQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query previous scores: %w", err)
	}
	defer prevRows.Close()

	prevAvgs := make(map[string]float64)
	for prevRows.Next() {
		var name string
		var avgValue float64
		if err := prevRows.Scan(&name, &avgValue); err != nil {
			return nil, fmt.Errorf("scan previous score row: %w", err)
		}
		prevAvgs[name] = avgValue
	}

	// Get sparkline data for each score
	var result []analytics.ScoreSummary
	for _, score := range topScores {
		sparklineQuery := `
			SELECT
				toStartOfHour(timestamp) as bucket,
				avg(value) as avg_value
			FROM scores
			WHERE project_id = ?
				AND name = ?
				AND timestamp >= ?
				AND timestamp < ?
			GROUP BY bucket
			ORDER BY bucket ASC
		`

		sparklineRows, err := r.db.Query(ctx, sparklineQuery,
			filter.ProjectID.String(),
			score.name,
			filter.StartTime,
			filter.EndTime,
		)
		if err != nil {
			return nil, fmt.Errorf("query sparkline for %s: %w", score.name, err)
		}

		var sparkline []analytics.TimeSeriesPoint
		for sparklineRows.Next() {
			var ts time.Time
			var value float64
			if err := sparklineRows.Scan(&ts, &value); err != nil {
				sparklineRows.Close()
				return nil, fmt.Errorf("scan sparkline row: %w", err)
			}
			sparkline = append(sparkline, analytics.TimeSeriesPoint{
				Timestamp: ts,
				Value:     value,
			})
		}
		sparklineRows.Close()

		// Calculate trend
		var trend float64
		if prevAvg, ok := prevAvgs[score.name]; ok && prevAvg != 0 {
			trend = ((score.avgValue - prevAvg) / prevAvg) * 100.0
		}

		result = append(result, analytics.ScoreSummary{
			Name:      score.name,
			AvgValue:  score.avgValue,
			Trend:     trend,
			Sparkline: sparkline,
		})
	}

	return result, nil
}

// HasTraces checks if the project has any traces
func (r *overviewRepository) HasTraces(ctx context.Context, projectID uuid.UUID) (bool, error) {
	query := `
		SELECT 1
		FROM otel_traces
		WHERE project_id = ?
			AND deleted_at IS NULL
		LIMIT 1
	`

	row := r.db.QueryRow(ctx, query, projectID.String())
	var exists int
	if err := row.Scan(&exists); err != nil {
		// No rows means no traces
		return false, nil
	}
	return true, nil
}

// HasScores checks if the project has any scores
func (r *overviewRepository) HasScores(ctx context.Context, projectID uuid.UUID) (bool, error) {
	query := `
		SELECT 1
		FROM scores
		WHERE project_id = ?
		LIMIT 1
	`

	row := r.db.QueryRow(ctx, query, projectID.String())
	var exists int
	if err := row.Scan(&exists); err != nil {
		// No rows means no scores
		return false, nil
	}
	return true, nil
}
