package observability

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"brokle/internal/core/domain/observability"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type scoreAnalyticsRepository struct {
	db clickhouse.Conn
}

func NewScoreAnalyticsRepository(db clickhouse.Conn) observability.ScoreAnalyticsRepository {
	return &scoreAnalyticsRepository{db: db}
}

func (r *scoreAnalyticsRepository) GetStatistics(ctx context.Context, filter *observability.ScoreAnalyticsFilter) (*observability.ScoreStatistics, error) {
	query := `
		SELECT
			count() as count,
			ifNull(avg(value), 0) as mean,
			ifNull(stddevPop(value), 0) as std_dev,
			ifNull(min(value), 0) as min_val,
			ifNull(max(value), 0) as max_val,
			ifNull(median(value), 0) as median_val
		FROM scores
		WHERE project_id = ?
		  AND name = ?
		  AND value IS NOT NULL
	`
	args := []any{filter.ProjectID, filter.ScoreName}

	if filter.FromTimestamp != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.ToTimestamp)
	}

	var stats observability.ScoreStatistics
	row := r.db.QueryRow(ctx, query, args...)
	err := row.Scan(&stats.Count, &stats.Mean, &stats.StdDev, &stats.Min, &stats.Max, &stats.Median)
	if err != nil {
		return nil, fmt.Errorf("query score statistics: %w", err)
	}

	// Handle NaN values from empty datasets
	if math.IsNaN(stats.Mean) {
		stats.Mean = 0
	}
	if math.IsNaN(stats.StdDev) {
		stats.StdDev = 0
	}
	if math.IsNaN(stats.Median) {
		stats.Median = 0
	}

	modeQuery := `
		SELECT
			string_value,
			count() as cnt,
			count() * 100.0 / sum(count()) OVER () as pct
		FROM scores
		WHERE project_id = ?
		  AND name = ?
		  AND string_value IS NOT NULL
		  AND string_value != ''
	`
	modeArgs := []any{filter.ProjectID, filter.ScoreName}

	if filter.FromTimestamp != nil {
		modeQuery += " AND timestamp >= ?"
		modeArgs = append(modeArgs, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		modeQuery += " AND timestamp <= ?"
		modeArgs = append(modeArgs, *filter.ToTimestamp)
	}

	modeQuery += " GROUP BY string_value ORDER BY cnt DESC LIMIT 1"

	var modeValue string
	var modeCnt uint64
	var modePct float64
	err = r.db.QueryRow(ctx, modeQuery, modeArgs...).Scan(&modeValue, &modeCnt, &modePct)
	if err == nil && modeValue != "" {
		stats.Mode = &modeValue
		stats.ModePercent = &modePct
	}

	return &stats, nil
}

func (r *scoreAnalyticsRepository) GetTimeSeries(ctx context.Context, filter *observability.ScoreAnalyticsFilter) ([]observability.TimeSeriesPoint, error) {
	var intervalFunc string
	switch filter.Interval {
	case "hour":
		intervalFunc = "toStartOfHour(timestamp)"
	case "week":
		intervalFunc = "toStartOfWeek(timestamp)"
	default: // day
		intervalFunc = "toStartOfDay(timestamp)"
	}

	query := fmt.Sprintf(`
		SELECT
			%s as time_bucket,
			avg(value) as avg_value,
			count() as count
		FROM scores
		WHERE project_id = ?
		  AND name = ?
		  AND value IS NOT NULL
	`, intervalFunc)

	args := []any{filter.ProjectID, filter.ScoreName}

	if filter.FromTimestamp != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.ToTimestamp)
	}

	query += " GROUP BY time_bucket ORDER BY time_bucket ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query time series: %w", err)
	}
	defer rows.Close()

	var points []observability.TimeSeriesPoint
	for rows.Next() {
		var point observability.TimeSeriesPoint
		if err := rows.Scan(&point.Timestamp, &point.AvgValue, &point.Count); err != nil {
			return nil, fmt.Errorf("scan time series point: %w", err)
		}
		// Handle NaN
		if math.IsNaN(point.AvgValue) {
			point.AvgValue = 0
		}
		points = append(points, point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate time series rows: %w", err)
	}

	return points, nil
}

func (r *scoreAnalyticsRepository) GetDistribution(ctx context.Context, filter *observability.ScoreAnalyticsFilter, bins int) ([]observability.DistributionBin, error) {
	if bins <= 0 {
		bins = 10
	}

	boundsQuery := `
		SELECT
			ifNull(min(value), 0) as min_val,
			ifNull(max(value), 0) as max_val
		FROM scores
		WHERE project_id = ?
		  AND name = ?
		  AND value IS NOT NULL
	`
	boundsArgs := []any{filter.ProjectID, filter.ScoreName}

	if filter.FromTimestamp != nil {
		boundsQuery += " AND timestamp >= ?"
		boundsArgs = append(boundsArgs, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		boundsQuery += " AND timestamp <= ?"
		boundsArgs = append(boundsArgs, *filter.ToTimestamp)
	}

	var minVal, maxVal float64
	err := r.db.QueryRow(ctx, boundsQuery, boundsArgs...).Scan(&minVal, &maxVal)
	if err != nil {
		return nil, fmt.Errorf("query bounds: %w", err)
	}

	if math.IsNaN(minVal) || math.IsNaN(maxVal) || minVal == maxVal {
		return []observability.DistributionBin{}, nil
	}

	binWidth := (maxVal - minVal) / float64(bins)
	if binWidth == 0 {
		binWidth = 1
	}

	query := `
		SELECT
			floor((value - ?) / ?) as bin_idx,
			count() as count
		FROM scores
		WHERE project_id = ?
		  AND name = ?
		  AND value IS NOT NULL
	`
	args := []any{minVal, binWidth, filter.ProjectID, filter.ScoreName}

	if filter.FromTimestamp != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.ToTimestamp)
	}

	query += " GROUP BY bin_idx ORDER BY bin_idx ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query distribution: %w", err)
	}
	defer rows.Close()

	distribution := make([]observability.DistributionBin, bins)
	for i := 0; i < bins; i++ {
		distribution[i] = observability.DistributionBin{
			BinStart: minVal + float64(i)*binWidth,
			BinEnd:   minVal + float64(i+1)*binWidth,
			Count:    0,
		}
	}

	for rows.Next() {
		var binIdx int64
		var count uint64
		if err := rows.Scan(&binIdx, &count); err != nil {
			return nil, fmt.Errorf("scan distribution bin: %w", err)
		}
		// Clamp to valid bin range
		if binIdx >= 0 && int(binIdx) < bins {
			distribution[binIdx].Count = count
		} else if binIdx >= int64(bins) {
			// Values at max boundary go to last bin
			distribution[bins-1].Count += count
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate distribution rows: %w", err)
	}

	return distribution, nil
}

func (r *scoreAnalyticsRepository) GetHeatmap(ctx context.Context, filter *observability.ScoreAnalyticsFilter, bins int) ([]observability.HeatmapCell, error) {
	if filter.CompareScoreName == nil {
		return []observability.HeatmapCell{}, nil
	}
	if bins <= 0 {
		bins = 10
	}

	boundsQuery := `
		SELECT
			min(if(name = ?, value, NULL)) as s1_min,
			max(if(name = ?, value, NULL)) as s1_max,
			min(if(name = ?, value, NULL)) as s2_min,
			max(if(name = ?, value, NULL)) as s2_max
		FROM scores
		WHERE project_id = ?
		  AND name IN (?, ?)
		  AND value IS NOT NULL
	`
	boundsArgs := []any{
		filter.ScoreName, filter.ScoreName,
		*filter.CompareScoreName, *filter.CompareScoreName,
		filter.ProjectID,
		filter.ScoreName, *filter.CompareScoreName,
	}

	if filter.FromTimestamp != nil {
		boundsQuery += " AND timestamp >= ?"
		boundsArgs = append(boundsArgs, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		boundsQuery += " AND timestamp <= ?"
		boundsArgs = append(boundsArgs, *filter.ToTimestamp)
	}

	var s1Min, s1Max, s2Min, s2Max *float64
	err := r.db.QueryRow(ctx, boundsQuery, boundsArgs...).Scan(&s1Min, &s1Max, &s2Min, &s2Max)
	if err != nil {
		return nil, fmt.Errorf("query heatmap bounds: %w", err)
	}

	if s1Min == nil || s1Max == nil || s2Min == nil || s2Max == nil {
		return []observability.HeatmapCell{}, nil
	}

	s1Width := (*s1Max - *s1Min) / float64(bins)
	s2Width := (*s2Max - *s2Min) / float64(bins)
	if s1Width == 0 {
		s1Width = 1
	}
	if s2Width == 0 {
		s2Width = 1
	}

	query := `
		SELECT
			floor((s1.value - ?) / ?) as bin1,
			floor((s2.value - ?) / ?) as bin2,
			count() as count
		FROM scores s1
		INNER JOIN scores s2 ON s1.trace_id = s2.trace_id AND s1.project_id = s2.project_id
		WHERE s1.project_id = ?
		  AND s1.name = ?
		  AND s2.name = ?
		  AND s1.value IS NOT NULL
		  AND s2.value IS NOT NULL
	`
	args := []any{
		*s1Min, s1Width,
		*s2Min, s2Width,
		filter.ProjectID,
		filter.ScoreName, *filter.CompareScoreName,
	}

	if filter.FromTimestamp != nil {
		query += " AND s1.timestamp >= ? AND s2.timestamp >= ?"
		args = append(args, *filter.FromTimestamp, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		query += " AND s1.timestamp <= ? AND s2.timestamp <= ?"
		args = append(args, *filter.ToTimestamp, *filter.ToTimestamp)
	}

	query += " GROUP BY bin1, bin2"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query heatmap: %w", err)
	}
	defer rows.Close()

	var cells []observability.HeatmapCell
	for rows.Next() {
		var bin1, bin2 int64
		var count uint64
		if err := rows.Scan(&bin1, &bin2, &count); err != nil {
			return nil, fmt.Errorf("scan heatmap cell: %w", err)
		}
		if bin1 >= int64(bins) {
			bin1 = int64(bins - 1)
		}
		if bin2 >= int64(bins) {
			bin2 = int64(bins - 1)
		}
		if bin1 < 0 {
			bin1 = 0
		}
		if bin2 < 0 {
			bin2 = 0
		}

		cells = append(cells, observability.HeatmapCell{
			Row:      int(bin1),
			Col:      int(bin2),
			Value:    count,
			RowLabel: fmt.Sprintf("%.2f-%.2f", *s1Min+float64(bin1)*s1Width, *s1Min+float64(bin1+1)*s1Width),
			ColLabel: fmt.Sprintf("%.2f-%.2f", *s2Min+float64(bin2)*s2Width, *s2Min+float64(bin2+1)*s2Width),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate heatmap rows: %w", err)
	}

	return cells, nil
}

// Uses ClickHouse aggregations for memory efficiency on large datasets
func (r *scoreAnalyticsRepository) GetComparisonMetrics(ctx context.Context, filter *observability.ScoreAnalyticsFilter) (*observability.ComparisonMetrics, error) {
	if filter.CompareScoreName == nil {
		return nil, nil
	}

	// Use ClickHouse's built-in corr() for Pearson correlation - computed on database side
	// Also compute MAE and RMSE via aggregations to avoid loading all data into memory
	aggregateQuery := `
		SELECT
			count() as matched_count,
			corr(s1.value, s2.value) as pearson_correlation,
			avg(abs(s1.value - s2.value)) as mae,
			sqrt(avg(pow(s1.value - s2.value, 2))) as rmse
		FROM scores s1
		INNER JOIN scores s2 ON s1.trace_id = s2.trace_id AND s1.project_id = s2.project_id
		WHERE s1.project_id = ?
		  AND s1.name = ?
		  AND s2.name = ?
		  AND s1.value IS NOT NULL
		  AND s2.value IS NOT NULL
	`
	args := []any{filter.ProjectID, filter.ScoreName, *filter.CompareScoreName}

	if filter.FromTimestamp != nil {
		aggregateQuery += " AND s1.timestamp >= ? AND s2.timestamp >= ?"
		args = append(args, *filter.FromTimestamp, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		aggregateQuery += " AND s1.timestamp <= ? AND s2.timestamp <= ?"
		args = append(args, *filter.ToTimestamp, *filter.ToTimestamp)
	}

	var matchedCount uint64
	var pearson, mae, rmse float64
	err := r.db.QueryRow(ctx, aggregateQuery, args...).Scan(&matchedCount, &pearson, &mae, &rmse)
	if err != nil {
		return nil, fmt.Errorf("query comparison aggregates: %w", err)
	}

	if matchedCount == 0 {
		return &observability.ComparisonMetrics{MatchedCount: 0}, nil
	}

	metrics := &observability.ComparisonMetrics{
		MatchedCount: matchedCount,
		MAE:          mae,
		RMSE:         rmse,
	}

	// Handle NaN from corr() on insufficient data
	if math.IsNaN(pearson) {
		metrics.PearsonCorrelation = 0
	} else {
		metrics.PearsonCorrelation = pearson
	}
	if math.IsNaN(mae) {
		metrics.MAE = 0
	}
	if math.IsNaN(rmse) {
		metrics.RMSE = 0
	}

	// For Spearman correlation, sample up to 100k pairs to avoid memory issues
	// ClickHouse doesn't have a built-in Spearman function, so we compute it in Go
	const spearmanSampleLimit = 100000
	spearmanQuery := `
		SELECT s1.value, s2.value
		FROM scores s1
		INNER JOIN scores s2 ON s1.trace_id = s2.trace_id AND s1.project_id = s2.project_id
		WHERE s1.project_id = ?
		  AND s1.name = ?
		  AND s2.name = ?
		  AND s1.value IS NOT NULL
		  AND s2.value IS NOT NULL
	`
	spearmanArgs := []any{filter.ProjectID, filter.ScoreName, *filter.CompareScoreName}

	if filter.FromTimestamp != nil {
		spearmanQuery += " AND s1.timestamp >= ? AND s2.timestamp >= ?"
		spearmanArgs = append(spearmanArgs, *filter.FromTimestamp, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		spearmanQuery += " AND s1.timestamp <= ? AND s2.timestamp <= ?"
		spearmanArgs = append(spearmanArgs, *filter.ToTimestamp, *filter.ToTimestamp)
	}

	spearmanQuery += fmt.Sprintf(" LIMIT %d", spearmanSampleLimit)

	rows, err := r.db.Query(ctx, spearmanQuery, spearmanArgs...)
	if err != nil {
		return nil, fmt.Errorf("query spearman sample: %w", err)
	}
	defer rows.Close()

	var values1, values2 []float64
	for rows.Next() {
		var v1, v2 float64
		if err := rows.Scan(&v1, &v2); err != nil {
			return nil, fmt.Errorf("scan spearman values: %w", err)
		}
		values1 = append(values1, v1)
		values2 = append(values2, v2)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate spearman rows: %w", err)
	}

	if len(values1) > 0 {
		metrics.SpearmanCorrelation = calculateSpearmanCorrelation(values1, values2)
	}

	return metrics, nil
}

func (r *scoreAnalyticsRepository) GetDistinctScoreNames(ctx context.Context, projectID string) ([]string, error) {
	query := `
		SELECT DISTINCT name
		FROM scores
		WHERE project_id = ?
		ORDER BY name ASC
		LIMIT 1000
	`

	rows, err := r.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("query distinct score names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan score name: %w", err)
		}
		names = append(names, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate score names: %w", err)
	}

	return names, nil
}

// calculatePearsonCorrelation computes Pearson correlation coefficient.
// Note: Pearson correlation for analytics is primarily computed via ClickHouse's corr() function.
// This Go implementation is used internally by calculateSpearmanCorrelation for rank-based correlation.
func calculatePearsonCorrelation(x, y []float64) float64 {
	n := float64(len(x))
	if n == 0 {
		return 0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range x {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}

	numerator := n*sumXY - sumX*sumY
	denominator := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))

	if denominator == 0 {
		return 0
	}

	r := numerator / denominator
	if math.IsNaN(r) {
		return 0
	}
	return r
}

// calculateSpearmanCorrelation computes Spearman rank correlation coefficient
func calculateSpearmanCorrelation(x, y []float64) float64 {
	n := len(x)
	if n == 0 {
		return 0
	}

	// Compute ranks
	ranksX := computeRanks(x)
	ranksY := computeRanks(y)

	// Calculate Pearson correlation on ranks
	return calculatePearsonCorrelation(ranksX, ranksY)
}

// computeRanks computes ranks for a slice of values (handling ties with average rank)
func computeRanks(values []float64) []float64 {
	n := len(values)
	if n == 0 {
		return nil
	}

	type indexedValue struct {
		index int
		value float64
	}
	indexed := make([]indexedValue, n)
	for i, v := range values {
		indexed[i] = indexedValue{index: i, value: v}
	}

	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].value < indexed[j].value
	})

	ranks := make([]float64, n)
	i := 0
	for i < n {
		j := i
		for j < n && indexed[j].value == indexed[i].value {
			j++
		}
		avgRank := float64(i+j+1) / 2.0
		for k := i; k < j; k++ {
			ranks[indexed[k].index] = avgRank
		}
		i = j
	}

	return ranks
}

// ============================================================================
// Materialized View-Optimized Methods
// ============================================================================
// These methods query pre-aggregated materialized views for faster analytics
// at scale. The views are automatically maintained by ClickHouse.

// GetExperimentScoreSummary returns pre-aggregated score statistics for an experiment
// Uses the scores_by_experiment materialized view for O(1) lookup instead of scanning scores table
func (r *scoreAnalyticsRepository) GetExperimentScoreSummary(ctx context.Context, projectID string, experimentID string) ([]observability.ExperimentScoreSummary, error) {
	query := `
		SELECT
			experiment_id,
			name,
			countMerge(count_state) as total_count,
			sumMerge(sum_state) as total_sum,
			minMerge(min_state) as min_val,
			maxMerge(max_state) as max_val
		FROM scores_by_experiment
		WHERE project_id = ?
		  AND experiment_id = ?
		GROUP BY experiment_id, name
		ORDER BY name ASC
	`

	rows, err := r.db.Query(ctx, query, projectID, experimentID)
	if err != nil {
		return nil, fmt.Errorf("query experiment score summary: %w", err)
	}
	defer rows.Close()

	var summaries []observability.ExperimentScoreSummary
	for rows.Next() {
		var s observability.ExperimentScoreSummary
		if err := rows.Scan(&s.ExperimentID, &s.ScoreName, &s.Count, &s.SumValue, &s.MinValue, &s.MaxValue); err != nil {
			return nil, fmt.Errorf("scan experiment score summary: %w", err)
		}
		// Compute average
		if s.Count > 0 {
			s.AvgValue = s.SumValue / float64(s.Count)
		}
		// Handle NaN
		if math.IsNaN(s.AvgValue) {
			s.AvgValue = 0
		}
		if math.IsNaN(s.MinValue) {
			s.MinValue = 0
		}
		if math.IsNaN(s.MaxValue) {
			s.MaxValue = 0
		}
		summaries = append(summaries, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experiment score summary rows: %w", err)
	}

	return summaries, nil
}

// GetSourceDistribution returns score counts grouped by source type per day
// Uses the scores_source_distribution materialized view for fast aggregation
func (r *scoreAnalyticsRepository) GetSourceDistribution(ctx context.Context, projectID string, fromTimestamp, toTimestamp *time.Time) ([]observability.SourceDistributionPoint, error) {
	query := `
		SELECT
			source,
			day,
			sum(count) as total_count
		FROM scores_source_distribution
		WHERE project_id = ?
	`
	args := []any{projectID}

	if fromTimestamp != nil {
		query += " AND day >= toDate(?)"
		args = append(args, *fromTimestamp)
	}
	if toTimestamp != nil {
		query += " AND day <= toDate(?)"
		args = append(args, *toTimestamp)
	}

	query += " GROUP BY source, day ORDER BY day ASC, source ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query source distribution: %w", err)
	}
	defer rows.Close()

	var points []observability.SourceDistributionPoint
	for rows.Next() {
		var p observability.SourceDistributionPoint
		var sourceEnum uint8
		if err := rows.Scan(&sourceEnum, &p.Day, &p.Count); err != nil {
			return nil, fmt.Errorf("scan source distribution point: %w", err)
		}
		// Map enum to string (matches ClickHouse Enum8: api=1, eval=2, annotation=3)
		switch sourceEnum {
		case 1:
			p.Source = observability.ScoreSourceAPI
		case 2:
			p.Source = observability.ScoreSourceEval
		case 3:
			p.Source = observability.ScoreSourceAnnotation
		default:
			p.Source = "unknown"
		}
		points = append(points, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source distribution rows: %w", err)
	}

	return points, nil
}

// GetDailySummary returns pre-aggregated daily score metrics for a specific score name
// Uses the scores_daily_summary materialized view for fast time series queries
func (r *scoreAnalyticsRepository) GetDailySummary(ctx context.Context, projectID string, scoreName string, fromTimestamp, toTimestamp *time.Time) ([]observability.DailySummaryPoint, error) {
	query := `
		SELECT
			day,
			countMerge(count_state) as total_count,
			sumMerge(sum_state) as total_sum,
			minMerge(min_state) as min_val,
			maxMerge(max_state) as max_val
		FROM scores_daily_summary
		WHERE project_id = ?
		  AND name = ?
	`
	args := []any{projectID, scoreName}

	if fromTimestamp != nil {
		query += " AND day >= toDate(?)"
		args = append(args, *fromTimestamp)
	}
	if toTimestamp != nil {
		query += " AND day <= toDate(?)"
		args = append(args, *toTimestamp)
	}

	query += " GROUP BY day ORDER BY day ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query daily summary: %w", err)
	}
	defer rows.Close()

	var points []observability.DailySummaryPoint
	for rows.Next() {
		var p observability.DailySummaryPoint
		if err := rows.Scan(&p.Day, &p.Count, &p.SumValue, &p.MinValue, &p.MaxValue); err != nil {
			return nil, fmt.Errorf("scan daily summary point: %w", err)
		}
		// Compute average
		if p.Count > 0 {
			p.AvgValue = p.SumValue / float64(p.Count)
		}
		// Handle NaN
		if math.IsNaN(p.AvgValue) {
			p.AvgValue = 0
		}
		if math.IsNaN(p.MinValue) {
			p.MinValue = 0
		}
		if math.IsNaN(p.MaxValue) {
			p.MaxValue = 0
		}
		points = append(points, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily summary rows: %w", err)
	}

	return points, nil
}
