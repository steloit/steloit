package observability

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/google/uuid"

	"brokle/internal/core/domain/observability"
	"brokle/pkg/uid"
)

// OTLPMetricsConverterService handles conversion of OTLP metrics to Brokle domain entities
// Follows the proven traces converter pattern with type-safe conversions
type OTLPMetricsConverterService struct {
	logger *slog.Logger
}

// NewOTLPMetricsConverterService creates a new OTLP metrics converter service
func NewOTLPMetricsConverterService(logger *slog.Logger) *OTLPMetricsConverterService {
	return &OTLPMetricsConverterService{
		logger: logger,
	}
}

// ConvertMetricsRequest converts OTLP MetricsData to Brokle telemetry events
// Returns typed domain entities (NOT map[string]any) for type safety
func (s *OTLPMetricsConverterService) ConvertMetricsRequest(
	ctx context.Context,
	metricsData *metricspb.MetricsData,
	projectID uuid.UUID,
) ([]*observability.TelemetryEventRequest, error) {
	var events []*observability.TelemetryEventRequest

	// Process each ResourceMetrics
	for _, resourceMetrics := range metricsData.GetResourceMetrics() {
		// Extract resource attributes once per resource
		resourceAttrs := extractResourceAttributes(resourceMetrics.GetResource())

		// Extract resource schema URL (from ResourceMetrics, not Resource)
		resourceSchemaURL := resourceMetrics.GetSchemaUrl()

		// Process each ScopeMetrics within this resource
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			scopeName := scopeMetrics.GetScope().GetName()
			scopeVersion := scopeMetrics.GetScope().GetVersion()
			scopeAttrs := extractScopeAttributes(scopeMetrics.GetScope())

			// Extract scope schema URL (from ScopeMetrics, not Scope)
			scopeSchemaURL := scopeMetrics.GetSchemaUrl()

			// Process each Metric within this scope
			for _, metric := range scopeMetrics.GetMetrics() {
				metricName := metric.GetName()
				metricDescription := metric.GetDescription()
				metricUnit := metric.GetUnit()

				// Route by metric data type
				switch data := metric.GetData().(type) {
				case *metricspb.Metric_Sum:
					sumEvents := s.convertSum(
						data.Sum,
						metricName,
						metricDescription,
						metricUnit,
						resourceAttrs,
						scopeName,
						scopeVersion,
						scopeAttrs,
						resourceSchemaURL,
						scopeSchemaURL,
						projectID,
					)
					events = append(events, sumEvents...)

				case *metricspb.Metric_Gauge:
					gaugeEvents := s.convertGauge(
						data.Gauge,
						metricName,
						metricDescription,
						metricUnit,
						resourceAttrs,
						scopeName,
						scopeVersion,
						scopeAttrs,
						resourceSchemaURL,
						scopeSchemaURL,
						projectID,
					)
					events = append(events, gaugeEvents...)

				case *metricspb.Metric_Histogram:
					histogramEvents := s.convertHistogram(
						data.Histogram,
						metricName,
						metricDescription,
						metricUnit,
						resourceAttrs,
						scopeName,
						scopeVersion,
						scopeAttrs,
						resourceSchemaURL,
						scopeSchemaURL,
						projectID,
					)
					events = append(events, histogramEvents...)

				case *metricspb.Metric_ExponentialHistogram:
					expHistogramEvents := s.convertExponentialHistogram(
						data.ExponentialHistogram,
						metricName,
						metricDescription,
						metricUnit,
						resourceAttrs,
						scopeName,
						scopeVersion,
						scopeAttrs,
						resourceSchemaURL,
						scopeSchemaURL,
						projectID,
					)
					events = append(events, expHistogramEvents...)

				default:
					// Skip unsupported metric types (Summary)
					s.logger.Warn("unsupported metric type, skipping", "metric_name", metricName, "metric_type", fmt.Sprintf("%T", metric.Data))
				}
			}
		}
	}

	return events, nil
}

// convertSum converts OTLP Sum metric to MetricSum domain entities
func (s *OTLPMetricsConverterService) convertSum(
	sum *metricspb.Sum,
	metricName, metricDescription, metricUnit string,
	resourceAttrs map[string]string,
	scopeName, scopeVersion string,
	scopeAttrs map[string]string,
	resourceSchemaURL, scopeSchemaURL string,
	projectID uuid.UUID,
) []*observability.TelemetryEventRequest {
	var events []*observability.TelemetryEventRequest

	// Process each data point
	for _, dp := range sum.GetDataPoints() {
		// Extract value based on type
		var value float64
		switch v := dp.GetValue().(type) {
		case *metricspb.NumberDataPoint_AsDouble:
			value = v.AsDouble
		case *metricspb.NumberDataPoint_AsInt:
			value = float64(v.AsInt)
		default:
			s.logger.Warn("unknown sum value type, defaulting to 0")
			value = 0
		}

		// Extract exemplars (all fields)
		exemplarsTimestamp, exemplarsValue, exemplarsFilteredAttrs, exemplarsTraceID, exemplarsSpanID := extractExemplars(dp.GetExemplars())

		// Convert schema URLs to pointer types (nil if empty)
		var resourceSchemaURLPtr, scopeSchemaURLPtr *string
		if resourceSchemaURL != "" {
			resourceSchemaURLPtr = &resourceSchemaURL
		}
		if scopeSchemaURL != "" {
			scopeSchemaURLPtr = &scopeSchemaURL
		}

		// Create MetricSum entity
		entity := &observability.MetricSum{
			ResourceAttributes:          resourceAttrs,
			ServiceName:                 resourceAttrs["service.name"], // MATERIALIZED column (auto-populated)
			ScopeName:                   scopeName,
			ScopeVersion:                scopeVersion,
			ScopeAttributes:             scopeAttrs,
			MetricName:                  metricName,
			MetricDescription:           metricDescription,
			MetricUnit:                  metricUnit,
			Attributes:                  extractDataPointAttributes(dp.GetAttributes()),
			StartTimeUnix:               time.Unix(0, int64(dp.GetStartTimeUnixNano())),
			TimeUnix:                    time.Unix(0, int64(dp.GetTimeUnixNano())),
			Value:                       value,
			AggregationTemporality:      observability.ConvertAggregationTemporality(int32(sum.GetAggregationTemporality())),
			IsMonotonic:                 sum.GetIsMonotonic(),
			ResourceSchemaURL:           resourceSchemaURLPtr,
			ScopeSchemaURL:              scopeSchemaURLPtr,
			ExemplarsTimestamp:          exemplarsTimestamp,
			ExemplarsValue:              exemplarsValue,
			ExemplarsFilteredAttributes: exemplarsFilteredAttrs,
			ExemplarsTraceID:            exemplarsTraceID,
			ExemplarsSpanID:             exemplarsSpanID,
			ProjectID: projectID,
		}

		timestamp := entity.TimeUnix
		events = append(events, &observability.TelemetryEventRequest{
			EventType: observability.TelemetryEventTypeMetricSum,
			EventID:   uid.New(),
			Timestamp: &timestamp,
			TraceID:   "", // Metrics don't have trace correlation by default
			SpanID:    "", // Metrics don't have span correlation by default
			Payload:   convertEntityToPayload(entity),
		})
	}

	return events
}

// convertGauge converts OTLP Gauge metric to MetricGauge domain entities
func (s *OTLPMetricsConverterService) convertGauge(
	gauge *metricspb.Gauge,
	metricName, metricDescription, metricUnit string,
	resourceAttrs map[string]string,
	scopeName, scopeVersion string,
	scopeAttrs map[string]string,
	resourceSchemaURL, scopeSchemaURL string,
	projectID uuid.UUID,
) []*observability.TelemetryEventRequest {
	var events []*observability.TelemetryEventRequest

	// Process each data point
	for _, dp := range gauge.GetDataPoints() {
		// Extract value based on type
		var value float64
		switch v := dp.GetValue().(type) {
		case *metricspb.NumberDataPoint_AsDouble:
			value = v.AsDouble
		case *metricspb.NumberDataPoint_AsInt:
			value = float64(v.AsInt)
		default:
			s.logger.Warn("unknown gauge value type, defaulting to 0")
			value = 0
		}

		// Extract exemplars (all fields)
		exemplarsTimestamp, exemplarsValue, exemplarsFilteredAttrs, exemplarsTraceID, exemplarsSpanID := extractExemplars(dp.GetExemplars())

		// Convert schema URLs to pointer types (nil if empty)
		var resourceSchemaURLPtr, scopeSchemaURLPtr *string
		if resourceSchemaURL != "" {
			resourceSchemaURLPtr = &resourceSchemaURL
		}
		if scopeSchemaURL != "" {
			scopeSchemaURLPtr = &scopeSchemaURL
		}

		// Create MetricGauge entity
		// Note: Gauge includes start_time_unix (differs from OTLP spec but matches our schema)
		entity := &observability.MetricGauge{
			ResourceAttributes:          resourceAttrs,
			ServiceName:                 resourceAttrs["service.name"], // MATERIALIZED column
			ScopeName:                   scopeName,
			ScopeVersion:                scopeVersion,
			ScopeAttributes:             scopeAttrs,
			MetricName:                  metricName,
			MetricDescription:           metricDescription,
			MetricUnit:                  metricUnit,
			Attributes:                  extractDataPointAttributes(dp.GetAttributes()),
			StartTimeUnix:               time.Unix(0, int64(dp.GetStartTimeUnixNano())), // Gauge HAS start_time in schema
			TimeUnix:                    time.Unix(0, int64(dp.GetTimeUnixNano())),
			Value:                       value,
			ResourceSchemaURL:           resourceSchemaURLPtr,
			ScopeSchemaURL:              scopeSchemaURLPtr,
			ExemplarsTimestamp:          exemplarsTimestamp,
			ExemplarsValue:              exemplarsValue,
			ExemplarsFilteredAttributes: exemplarsFilteredAttrs,
			ExemplarsTraceID:            exemplarsTraceID,
			ExemplarsSpanID:             exemplarsSpanID,
			ProjectID: projectID,
		}

		timestamp := entity.TimeUnix
		events = append(events, &observability.TelemetryEventRequest{
			EventType: observability.TelemetryEventTypeMetricGauge,
			EventID:   uid.New(),
			Timestamp: &timestamp,
			TraceID:   "", // Metrics don't have trace correlation by default
			SpanID:    "", // Metrics don't have span correlation by default
			Payload:   convertEntityToPayload(entity),
		})
	}

	return events
}

// convertHistogram converts OTLP Histogram metric to MetricHistogram domain entities
func (s *OTLPMetricsConverterService) convertHistogram(
	histogram *metricspb.Histogram,
	metricName, metricDescription, metricUnit string,
	resourceAttrs map[string]string,
	scopeName, scopeVersion string,
	scopeAttrs map[string]string,
	resourceSchemaURL, scopeSchemaURL string,
	projectID uuid.UUID,
) []*observability.TelemetryEventRequest {
	var events []*observability.TelemetryEventRequest

	// Process each data point
	for _, dp := range histogram.GetDataPoints() {
		// Handle nullable sum/min/max fields (use pointer types)
		// OTLP: These fields are optional - protobuf uses default zero values
		// We set pointers only when values are meaningful (non-zero or count > 0)
		var sum, min, max *float64

		// Sum: Only set if count > 0 (histogram has data) or sum is explicitly non-zero
		if dp.GetCount() > 0 || dp.GetSum() != 0 {
			val := dp.GetSum()
			sum = &val
		}

		// Min/Max: Only set if non-zero (OTLP exporters omit zero values)
		if dp.GetMin() != 0 {
			val := dp.GetMin()
			min = &val
		}
		if dp.GetMax() != 0 {
			val := dp.GetMax()
			max = &val
		}

		// Extract bucket counts and explicit bounds (ensure non-nil arrays)
		bucketCounts := make([]uint64, len(dp.GetBucketCounts()))
		copy(bucketCounts, dp.GetBucketCounts())

		explicitBounds := make([]float64, len(dp.GetExplicitBounds()))
		copy(explicitBounds, dp.GetExplicitBounds())

		// Extract exemplars (all fields)
		exemplarsTimestamp, exemplarsValue, exemplarsFilteredAttrs, exemplarsTraceID, exemplarsSpanID := extractExemplars(dp.GetExemplars())

		// Convert schema URLs to pointer types (nil if empty)
		var resourceSchemaURLPtr, scopeSchemaURLPtr *string
		if resourceSchemaURL != "" {
			resourceSchemaURLPtr = &resourceSchemaURL
		}
		if scopeSchemaURL != "" {
			scopeSchemaURLPtr = &scopeSchemaURL
		}

		// Create MetricHistogram entity
		entity := &observability.MetricHistogram{
			ResourceAttributes:          resourceAttrs,
			ServiceName:                 resourceAttrs["service.name"], // MATERIALIZED column
			ScopeName:                   scopeName,
			ScopeVersion:                scopeVersion,
			ScopeAttributes:             scopeAttrs,
			MetricName:                  metricName,
			MetricDescription:           metricDescription,
			MetricUnit:                  metricUnit,
			Attributes:                  extractDataPointAttributes(dp.GetAttributes()),
			StartTimeUnix:               time.Unix(0, int64(dp.GetStartTimeUnixNano())),
			TimeUnix:                    time.Unix(0, int64(dp.GetTimeUnixNano())),
			Count:                       dp.GetCount(),
			Sum:                         sum, // Nullable(Float64) - pointer type
			Min:                         min, // Nullable(Float64) - pointer type
			Max:                         max, // Nullable(Float64) - pointer type
			BucketCounts:                bucketCounts,
			ExplicitBounds:              explicitBounds,
			AggregationTemporality:      observability.ConvertAggregationTemporality(int32(histogram.GetAggregationTemporality())),
			ResourceSchemaURL:           resourceSchemaURLPtr,
			ScopeSchemaURL:              scopeSchemaURLPtr,
			ExemplarsTimestamp:          exemplarsTimestamp,
			ExemplarsValue:              exemplarsValue,
			ExemplarsFilteredAttributes: exemplarsFilteredAttrs,
			ExemplarsTraceID:            exemplarsTraceID,
			ExemplarsSpanID:             exemplarsSpanID,
			ProjectID: projectID,
		}

		timestamp := entity.TimeUnix
		events = append(events, &observability.TelemetryEventRequest{
			EventType: observability.TelemetryEventTypeMetricHistogram,
			EventID:   uid.New(),
			Timestamp: &timestamp,
			TraceID:   "", // Metrics don't have trace correlation by default
			SpanID:    "", // Metrics don't have span correlation by default
			Payload:   convertEntityToPayload(entity),
		})
	}

	return events
}

// convertExponentialHistogram converts OTLP ExponentialHistogram metric to MetricExponentialHistogram domain entities
// OTLP 1.38+: Modern histogram using exponential bucketing (memory-efficient)
func (s *OTLPMetricsConverterService) convertExponentialHistogram(
	expHistogram *metricspb.ExponentialHistogram,
	metricName, metricDescription, metricUnit string,
	resourceAttrs map[string]string,
	scopeName, scopeVersion string,
	scopeAttrs map[string]string,
	resourceSchemaURL, scopeSchemaURL string,
	projectID uuid.UUID,
) []*observability.TelemetryEventRequest {
	var events []*observability.TelemetryEventRequest

	// Process each data point
	for _, dp := range expHistogram.GetDataPoints() {
		// Handle nullable sum/min/max fields (use pointer types)
		var sum, min, max *float64

		// Sum: Only set if count > 0 or sum is explicitly non-zero
		if dp.GetCount() > 0 || dp.GetSum() != 0 {
			val := dp.GetSum()
			sum = &val
		}

		// Min/Max: Only set if non-zero
		if dp.GetMin() != 0 {
			val := dp.GetMin()
			min = &val
		}
		if dp.GetMax() != 0 {
			val := dp.GetMax()
			max = &val
		}

		// Extract positive bucket counts
		var positiveBucketCounts []uint64
		if positive := dp.GetPositive(); positive != nil {
			positiveBucketCounts = make([]uint64, len(positive.GetBucketCounts()))
			copy(positiveBucketCounts, positive.GetBucketCounts())
		} else {
			positiveBucketCounts = []uint64{} // Empty array if no positive buckets
		}

		// Extract negative bucket counts
		var negativeBucketCounts []uint64
		if negative := dp.GetNegative(); negative != nil {
			negativeBucketCounts = make([]uint64, len(negative.GetBucketCounts()))
			copy(negativeBucketCounts, negative.GetBucketCounts())
		} else {
			negativeBucketCounts = []uint64{} // Empty array if no negative buckets
		}

		// Extract offsets (default to 0 if buckets are nil)
		var positiveOffset, negativeOffset int32
		if positive := dp.GetPositive(); positive != nil {
			positiveOffset = positive.GetOffset()
		}
		if negative := dp.GetNegative(); negative != nil {
			negativeOffset = negative.GetOffset()
		}

		// Extract exemplars (all fields)
		exemplarsTimestamp, exemplarsValue, exemplarsFilteredAttrs, exemplarsTraceID, exemplarsSpanID := extractExemplars(dp.GetExemplars())

		// Convert schema URLs to pointer types (nil if empty)
		var resourceSchemaURLPtr, scopeSchemaURLPtr *string
		if resourceSchemaURL != "" {
			resourceSchemaURLPtr = &resourceSchemaURL
		}
		if scopeSchemaURL != "" {
			scopeSchemaURLPtr = &scopeSchemaURL
		}

		// Create MetricExponentialHistogram entity
		entity := &observability.MetricExponentialHistogram{
			ResourceAttributes:          resourceAttrs,
			ServiceName:                 resourceAttrs["service.name"], // MATERIALIZED column
			ScopeName:                   scopeName,
			ScopeVersion:                scopeVersion,
			ScopeAttributes:             scopeAttrs,
			MetricName:                  metricName,
			MetricDescription:           metricDescription,
			MetricUnit:                  metricUnit,
			Attributes:                  extractDataPointAttributes(dp.GetAttributes()),
			StartTimeUnix:               time.Unix(0, int64(dp.GetStartTimeUnixNano())),
			TimeUnix:                    time.Unix(0, int64(dp.GetTimeUnixNano())),
			Count:                       dp.GetCount(),
			Sum:                         sum, // Nullable(Float64) - pointer type
			Scale:                       dp.GetScale(),
			ZeroCount:                   dp.GetZeroCount(),
			PositiveOffset:              positiveOffset,
			PositiveBucketCounts:        positiveBucketCounts,
			NegativeOffset:              negativeOffset,
			NegativeBucketCounts:        negativeBucketCounts,
			Min:                         min, // Nullable(Float64) - pointer type
			Max:                         max, // Nullable(Float64) - pointer type
			AggregationTemporality:      observability.ConvertAggregationTemporality(int32(expHistogram.GetAggregationTemporality())),
			ResourceSchemaURL:           resourceSchemaURLPtr,
			ScopeSchemaURL:              scopeSchemaURLPtr,
			ExemplarsTimestamp:          exemplarsTimestamp,
			ExemplarsValue:              exemplarsValue,
			ExemplarsFilteredAttributes: exemplarsFilteredAttrs,
			ExemplarsTraceID:            exemplarsTraceID,
			ExemplarsSpanID:             exemplarsSpanID,
			ProjectID: projectID,
		}

		timestamp := entity.TimeUnix
		events = append(events, &observability.TelemetryEventRequest{
			EventType: observability.TelemetryEventTypeMetricExponentialHistogram,
			EventID:   uid.New(),
			Timestamp: &timestamp,
			TraceID:   "", // Metrics don't have trace correlation by default
			SpanID:    "", // Metrics don't have span correlation by default
			Payload:   convertEntityToPayload(entity),
		})
	}

	return events
}

// ===== Helper Functions =====

// extractDataPointAttributes converts OTLP KeyValue attributes to map[string]string
func extractDataPointAttributes(attrs []*commonpb.KeyValue) map[string]string {
	return convertKeyValuesToMap(attrs)
}

// extractExemplars extracts all exemplar data from OTLP exemplars
// Returns parallel arrays for all exemplar fields
func extractExemplars(exemplars []*metricspb.Exemplar) (
	timestamps []time.Time,
	values []float64,
	filteredAttrs []map[string]string,
	traceIDs []string,
	spanIDs []string,
) {
	timestamps = make([]time.Time, 0, len(exemplars))
	values = make([]float64, 0, len(exemplars))
	filteredAttrs = make([]map[string]string, 0, len(exemplars))
	traceIDs = make([]string, 0, len(exemplars))
	spanIDs = make([]string, 0, len(exemplars))

	for _, exemplar := range exemplars {
		// Extract timestamp (nanosecond precision)
		timestamps = append(timestamps, time.Unix(0, int64(exemplar.GetTimeUnixNano())))

		// Extract value (handle AsDouble and AsInt union types)
		var value float64
		switch v := exemplar.GetValue().(type) {
		case *metricspb.Exemplar_AsDouble:
			value = v.AsDouble
		case *metricspb.Exemplar_AsInt:
			value = float64(v.AsInt)
		default:
			value = 0
		}
		values = append(values, value)

		// Extract filtered attributes
		filteredAttrs = append(filteredAttrs, convertKeyValuesToMap(exemplar.GetFilteredAttributes()))

		// Extract trace ID (auto-padded to 32 hex chars via hex.EncodeToString)
		if traceIDBytes := exemplar.GetTraceId(); len(traceIDBytes) > 0 {
			traceIDs = append(traceIDs, hex.EncodeToString(traceIDBytes))
		} else {
			traceIDs = append(traceIDs, "")
		}

		// Extract span ID (auto-padded to 16 hex chars via hex.EncodeToString)
		if spanIDBytes := exemplar.GetSpanId(); len(spanIDBytes) > 0 {
			spanIDs = append(spanIDs, hex.EncodeToString(spanIDBytes))
		} else {
			spanIDs = append(spanIDs, "")
		}
	}

	return timestamps, values, filteredAttrs, traceIDs, spanIDs
}
