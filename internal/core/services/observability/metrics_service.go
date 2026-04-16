package observability

import (
	"context"
	"log/slog"

	"brokle/internal/core/domain/observability"
)

// MetricsService implements business logic for OTLP metrics management
type MetricsService struct {
	metricsRepo observability.MetricsRepository
	logger      *slog.Logger
}

// NewMetricsService creates a new metrics service instance
func NewMetricsService(
	metricsRepo observability.MetricsRepository,
	logger *slog.Logger,
) *MetricsService {
	return &MetricsService{
		metricsRepo: metricsRepo,
		logger:      logger,
	}
}

// CreateMetricSumBatch creates multiple metric sums in a single batch
// Used by workers for efficient OTLP metrics processing
func (s *MetricsService) CreateMetricSumBatch(ctx context.Context, metricsSums []*observability.MetricSum) error {
	if len(metricsSums) == 0 {
		return nil
	}

	// Delegate to repository (no validation needed - converter already validated)
	return s.metricsRepo.CreateMetricSumBatch(ctx, metricsSums)
}

// CreateMetricGaugeBatch creates multiple metric gauges in a single batch
// Used by workers for efficient OTLP metrics processing
func (s *MetricsService) CreateMetricGaugeBatch(ctx context.Context, metricsGauges []*observability.MetricGauge) error {
	if len(metricsGauges) == 0 {
		return nil
	}

	// Delegate to repository (no validation needed - converter already validated)
	return s.metricsRepo.CreateMetricGaugeBatch(ctx, metricsGauges)
}

// CreateMetricHistogramBatch creates multiple metric histograms in a single batch
// Used by workers for efficient OTLP metrics processing
func (s *MetricsService) CreateMetricHistogramBatch(ctx context.Context, metricsHistograms []*observability.MetricHistogram) error {
	if len(metricsHistograms) == 0 {
		return nil
	}

	// Delegate to repository (no validation needed - converter already validated)
	return s.metricsRepo.CreateMetricHistogramBatch(ctx, metricsHistograms)
}

// CreateMetricExponentialHistogramBatch creates multiple exponential histogram metrics in a single batch
// OTLP 1.38+: Used by workers for efficient OTLP exponential histogram processing
func (s *MetricsService) CreateMetricExponentialHistogramBatch(ctx context.Context, metricsExpHistograms []*observability.MetricExponentialHistogram) error {
	if len(metricsExpHistograms) == 0 {
		return nil
	}

	// Delegate to repository (no validation needed - converter already validated)
	return s.metricsRepo.CreateMetricExponentialHistogramBatch(ctx, metricsExpHistograms)
}
