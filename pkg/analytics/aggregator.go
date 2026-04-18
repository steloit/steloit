package analytics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// AggregationType represents different types of aggregations
type AggregationType string

const (
	AggSum    AggregationType = "sum"
	AggAvg    AggregationType = "avg"
	AggMin    AggregationType = "min"
	AggMax    AggregationType = "max"
	AggCount  AggregationType = "count"
	AggP50    AggregationType = "p50"
	AggP90    AggregationType = "p90"
	AggP95    AggregationType = "p95"
	AggP99    AggregationType = "p99"
	AggStdDev AggregationType = "stddev"
	AggRate   AggregationType = "rate"
	AggGrowth AggregationType = "growth"
)

// TimeWindow represents a time window for aggregation
type TimeWindow struct {
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Duration time.Duration `json:"duration"`
}

// DataPoint represents a single data point
type DataPoint struct {
	Timestamp time.Time              `json:"timestamp"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Value     float64                `json:"value"`
}

// TimeSeries represents a time series of data points
type TimeSeries struct {
	Labels     map[string]string      `json:"labels,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Name       string                 `json:"name"`
	DataPoints []DataPoint            `json:"data_points"`
}

// AggregationRequest represents a request for aggregation
type AggregationRequest struct {
	Filters     map[string]string      `json:"filters,omitempty"`
	FillValue   *float64               `json:"fill_value,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	TimeWindow  TimeWindow             `json:"time_window"`
	MetricName  string                 `json:"metric_name"`
	Aggregation AggregationType        `json:"aggregation"`
	GroupBy     []string               `json:"group_by,omitempty"`
	Interval    time.Duration          `json:"interval,omitempty"`
}

// AggregationResult represents the result of an aggregation
type AggregationResult struct {
	ComputedAt  time.Time              `json:"computed_at"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	TimeWindow  TimeWindow             `json:"time_window"`
	MetricName  string                 `json:"metric_name"`
	Aggregation AggregationType        `json:"aggregation"`
	Series      []TimeSeries           `json:"series,omitempty"`
	Value       float64                `json:"value"`
	Count       int64                  `json:"count"`
}

// Aggregator handles metric aggregations
type Aggregator struct {
	ctx    context.Context
	data   map[string][]DataPoint
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewAggregator creates a new aggregator
func NewAggregator() *Aggregator {
	ctx, cancel := context.WithCancel(context.Background())

	return &Aggregator{
		data:   make(map[string][]DataPoint),
		ctx:    ctx,
		cancel: cancel,
	}
}

// AddDataPoint adds a data point to the aggregator
func (a *Aggregator) AddDataPoint(metricName string, point DataPoint) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.data[metricName] == nil {
		a.data[metricName] = make([]DataPoint, 0)
	}

	a.data[metricName] = append(a.data[metricName], point)
}

// AddDataPoints adds multiple data points
func (a *Aggregator) AddDataPoints(metricName string, points []DataPoint) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.data[metricName] == nil {
		a.data[metricName] = make([]DataPoint, 0)
	}

	a.data[metricName] = append(a.data[metricName], points...)
}

// Aggregate performs aggregation on the specified metric
func (a *Aggregator) Aggregate(req *AggregationRequest) (*AggregationResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	points, exists := a.data[req.MetricName]
	if !exists || len(points) == 0 {
		return nil, fmt.Errorf("no data found for metric: %s", req.MetricName)
	}

	// Filter data points by time window and filters
	filteredPoints := a.filterPoints(points, req)
	if len(filteredPoints) == 0 {
		return nil, errors.New("no data points match the criteria")
	}

	result := &AggregationResult{
		MetricName:  req.MetricName,
		Aggregation: req.Aggregation,
		TimeWindow:  req.TimeWindow,
		Count:       int64(len(filteredPoints)),
		ComputedAt:  time.Now().UTC(),
		Metadata:    make(map[string]any),
	}

	// Group by labels if specified
	if len(req.GroupBy) > 0 {
		groups := a.groupByLabels(filteredPoints, req.GroupBy)
		series := make([]TimeSeries, 0, len(groups))

		for groupKey, groupPoints := range groups {
			_, err := a.computeAggregation(groupPoints, req.Aggregation)
			if err != nil {
				return nil, err
			}

			// Parse labels from group key
			labels := a.parseGroupKey(groupKey, req.GroupBy)
			timeSeries := a.createTimeSeries(req.MetricName, labels, groupPoints, req.Interval)
			series = append(series, timeSeries)
		}

		result.Series = series
		// Set overall value to average of all series if applicable
		if len(series) > 0 {
			totalValue := 0.0
			for _, ts := range series {
				for _, dp := range ts.DataPoints {
					totalValue += dp.Value
				}
			}
			result.Value = totalValue / float64(len(series))
		}
	} else {
		// Single aggregation
		value, err := a.computeAggregation(filteredPoints, req.Aggregation)
		if err != nil {
			return nil, err
		}
		result.Value = value

		// Create time series if interval is specified
		if req.Interval > 0 {
			timeSeries := a.createTimeSeries(req.MetricName, nil, filteredPoints, req.Interval)
			result.Series = []TimeSeries{timeSeries}
		}
	}

	return result, nil
}

// AggregateMultiple performs multiple aggregations
func (a *Aggregator) AggregateMultiple(requests []*AggregationRequest) ([]*AggregationResult, error) {
	results := make([]*AggregationResult, 0, len(requests))

	for _, req := range requests {
		result, err := a.Aggregate(req)
		if err != nil {
			return nil, fmt.Errorf("aggregation failed for %s: %w", req.MetricName, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// GetMetrics returns all available metrics
func (a *Aggregator) GetMetrics() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics := make([]string, 0, len(a.data))
	for metric := range a.data {
		metrics = append(metrics, metric)
	}

	return metrics
}

// GetDataPoints returns data points for a metric within a time window
func (a *Aggregator) GetDataPoints(metricName string, timeWindow TimeWindow) ([]DataPoint, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	points, exists := a.data[metricName]
	if !exists {
		return nil, fmt.Errorf("metric not found: %s", metricName)
	}

	var filteredPoints []DataPoint
	for _, point := range points {
		if point.Timestamp.After(timeWindow.Start) && point.Timestamp.Before(timeWindow.End) {
			filteredPoints = append(filteredPoints, point)
		}
	}

	return filteredPoints, nil
}

// Clear removes all data
func (a *Aggregator) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.data = make(map[string][]DataPoint)
}

// ClearMetric removes data for a specific metric
func (a *Aggregator) ClearMetric(metricName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.data, metricName)
}

// Internal helper methods

// filterPoints filters data points based on time window and filters
func (a *Aggregator) filterPoints(points []DataPoint, req *AggregationRequest) []DataPoint {
	var filtered []DataPoint

	for _, point := range points {
		// Check time window
		if point.Timestamp.Before(req.TimeWindow.Start) || point.Timestamp.After(req.TimeWindow.End) {
			continue
		}

		// Check filters
		if !a.matchesFilters(point, req.Filters) {
			continue
		}

		filtered = append(filtered, point)
	}

	return filtered
}

// matchesFilters checks if a data point matches the given filters
func (a *Aggregator) matchesFilters(point DataPoint, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}

	for key, expectedValue := range filters {
		if actualValue, exists := point.Labels[key]; !exists || actualValue != expectedValue {
			return false
		}
	}

	return true
}

// groupByLabels groups data points by the specified labels
func (a *Aggregator) groupByLabels(points []DataPoint, groupBy []string) map[string][]DataPoint {
	groups := make(map[string][]DataPoint)

	for _, point := range points {
		key := a.createGroupKey(point.Labels, groupBy)
		if groups[key] == nil {
			groups[key] = make([]DataPoint, 0)
		}
		groups[key] = append(groups[key], point)
	}

	return groups
}

// createGroupKey creates a key for grouping based on labels
func (a *Aggregator) createGroupKey(labels map[string]string, groupBy []string) string {
	var keyParts []string
	for _, label := range groupBy {
		if value, exists := labels[label]; exists {
			keyParts = append(keyParts, fmt.Sprintf("%s=%s", label, value))
		} else {
			keyParts = append(keyParts, label+"=")
		}
	}

	if len(keyParts) == 0 {
		return "default"
	}

	return fmt.Sprintf("{%s}", joinStrings(keyParts, ","))
}

// computeAggregation computes the specified aggregation on data points
func (a *Aggregator) computeAggregation(points []DataPoint, aggType AggregationType) (float64, error) {
	if len(points) == 0 {
		return 0, errors.New("no data points to aggregate")
	}

	values := make([]float64, len(points))
	for i, point := range points {
		values[i] = point.Value
	}

	switch aggType {
	case AggSum:
		return a.sum(values), nil
	case AggAvg:
		return a.average(values), nil
	case AggMin:
		return a.min(values), nil
	case AggMax:
		return a.max(values), nil
	case AggCount:
		return float64(len(values)), nil
	case AggP50:
		return a.percentile(values, 0.5), nil
	case AggP90:
		return a.percentile(values, 0.9), nil
	case AggP95:
		return a.percentile(values, 0.95), nil
	case AggP99:
		return a.percentile(values, 0.99), nil
	case AggStdDev:
		return a.standardDeviation(values), nil
	case AggRate:
		return a.rate(points), nil
	case AggGrowth:
		return a.growth(values), nil
	default:
		return 0, fmt.Errorf("unsupported aggregation type: %s", aggType)
	}
}

// createTimeSeries creates a time series with the specified interval
func (a *Aggregator) createTimeSeries(metricName string, labels map[string]string, points []DataPoint, interval time.Duration) TimeSeries {
	timeSeries := TimeSeries{
		Name:       metricName,
		Labels:     labels,
		DataPoints: make([]DataPoint, 0),
		Metadata:   make(map[string]any),
	}

	if interval <= 0 {
		// No bucketing, return original points
		timeSeries.DataPoints = points
		return timeSeries
	}

	// Bucket data points by interval
	buckets := a.bucketByInterval(points, interval)

	for timestamp, bucketPoints := range buckets {
		if len(bucketPoints) > 0 {
			avgValue := a.average(extractValues(bucketPoints))
			timeSeries.DataPoints = append(timeSeries.DataPoints, DataPoint{
				Timestamp: timestamp,
				Value:     avgValue,
				Labels:    labels,
			})
		}
	}

	// Sort by timestamp
	sort.Slice(timeSeries.DataPoints, func(i, j int) bool {
		return timeSeries.DataPoints[i].Timestamp.Before(timeSeries.DataPoints[j].Timestamp)
	})

	return timeSeries
}

// bucketByInterval buckets data points by time interval
func (a *Aggregator) bucketByInterval(points []DataPoint, interval time.Duration) map[time.Time][]DataPoint {
	buckets := make(map[time.Time][]DataPoint)

	for _, point := range points {
		bucketTime := point.Timestamp.Truncate(interval)
		if buckets[bucketTime] == nil {
			buckets[bucketTime] = make([]DataPoint, 0)
		}
		buckets[bucketTime] = append(buckets[bucketTime], point)
	}

	return buckets
}

// Mathematical aggregation functions

func (a *Aggregator) sum(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum
}

func (a *Aggregator) average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return a.sum(values) / float64(len(values))
}

func (a *Aggregator) min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func (a *Aggregator) max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func (a *Aggregator) percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	index := p * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sorted[lower]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func (a *Aggregator) standardDeviation(values []float64) float64 {
	if len(values) <= 1 {
		return 0
	}

	mean := a.average(values)
	sumSquaredDiff := 0.0

	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}

	variance := sumSquaredDiff / float64(len(values)-1)
	return math.Sqrt(variance)
}

func (a *Aggregator) rate(points []DataPoint) float64 {
	if len(points) < 2 {
		return 0
	}

	// Sort by timestamp
	sortedPoints := make([]DataPoint, len(points))
	copy(sortedPoints, points)
	sort.Slice(sortedPoints, func(i, j int) bool {
		return sortedPoints[i].Timestamp.Before(sortedPoints[j].Timestamp)
	})

	first := sortedPoints[0]
	last := sortedPoints[len(sortedPoints)-1]

	timeDiff := last.Timestamp.Sub(first.Timestamp).Seconds()
	if timeDiff == 0 {
		return 0
	}

	valueDiff := last.Value - first.Value
	return valueDiff / timeDiff
}

func (a *Aggregator) growth(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	first := values[0]
	last := values[len(values)-1]

	if first == 0 {
		return 0
	}

	return ((last - first) / first) * 100
}

// parseGroupKey parses a group key back to label map
func (a *Aggregator) parseGroupKey(key string, groupBy []string) map[string]string {
	labels := make(map[string]string)

	// Remove the braces and split by comma
	if len(key) > 2 && key[0] == '{' && key[len(key)-1] == '}' {
		content := key[1 : len(key)-1]
		pairs := strings.Split(content, ",")

		for _, pair := range pairs {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				labels[parts[0]] = parts[1]
			}
		}
	}

	return labels
}

// Helper functions

func extractValues(points []DataPoint) []float64 {
	values := make([]float64, len(points))
	for i, point := range points {
		values[i] = point.Value
	}
	return values
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
