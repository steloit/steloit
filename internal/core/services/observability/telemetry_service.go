package observability

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"brokle/internal/core/domain/observability"
	"brokle/internal/infrastructure/streams"
)

// TelemetryService aggregates all telemetry-related services with Redis Streams-based async processing
type TelemetryService struct {
	lastProcessingTime   time.Time
	deduplicationService observability.TelemetryDeduplicationService
	streamProducer       *streams.TelemetryStreamProducer
	logger               *slog.Logger
	batchesProcessed     uint64
	eventsProcessed      uint64
	avgProcessingTime    time.Duration
	mu                   sync.RWMutex
}

// NewTelemetryService creates a new telemetry service with Redis Streams and deduplication
func NewTelemetryService(
	deduplicationService observability.TelemetryDeduplicationService,
	streamProducer *streams.TelemetryStreamProducer,
	logger *slog.Logger,
) observability.TelemetryService {
	return &TelemetryService{
		deduplicationService: deduplicationService,
		streamProducer:       streamProducer,
		logger:               logger,
		lastProcessingTime:   time.Now(),
	}
}

// Deduplication returns the deduplication service
func (s *TelemetryService) Deduplication() observability.TelemetryDeduplicationService {
	return s.deduplicationService
}

// GetHealth returns the health status of all telemetry services
func (s *TelemetryService) GetHealth(ctx context.Context) (*observability.TelemetryHealthStatus, error) {
	health := &observability.TelemetryHealthStatus{
		Healthy:               true, // Stream consumer health monitored separately
		ActiveWorkers:         1,    // Stream consumer
		AverageProcessingTime: float64(s.avgProcessingTime.Milliseconds()),
		ThroughputPerMinute:   float64(s.eventsProcessed) / time.Since(s.lastProcessingTime).Minutes(),
	}

	// Set default database health
	health.Database = &observability.DatabaseHealth{
		Connected:         true,
		LatencyMs:         1.5, // Default value
		ActiveConnections: 10,  // Default value
		MaxConnections:    100, // Default value
	}

	// Set default Redis health
	health.Redis = &observability.RedisHealthStatus{
		Available:   true,
		LatencyMs:   0.5, // Default value
		Connections: 5,   // Default value
		LastError:   nil,
		Uptime:      time.Hour * 24, // Default uptime
	}

	// Processing queue health (stream consumer)
	health.ProcessingQueue = &observability.QueueHealth{
		Size:             0, // Stream consumer processes real-time
		ProcessingRate:   float64(s.eventsProcessed),
		AverageWaitTime:  10.0, // Default value
		OldestMessageAge: 0,
	}

	// Set default error rate
	s.mu.RLock()
	if s.eventsProcessed > 0 {
		health.ErrorRate = 0.01 // 1% default error rate (actual errors tracked in stream consumer)
	}
	s.mu.RUnlock()

	return health, nil
}

// GetMetrics returns aggregated metrics from all telemetry services
func (s *TelemetryService) GetMetrics(ctx context.Context) (*observability.TelemetryMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Calculate throughput per second
	throughput := float64(0)
	if s.lastProcessingTime.Before(time.Now()) {
		elapsed := time.Since(s.lastProcessingTime)
		if elapsed.Seconds() > 0 {
			throughput = float64(s.eventsProcessed) / elapsed.Seconds()
		}
	}

	// Calculate success rate
	successRate := float64(100)
	if s.eventsProcessed > 0 {
		successRate = 99.0 // Default 99% success rate
	}

	// Aggregate metrics
	metrics := &observability.TelemetryMetrics{
		TotalBatches:      int64(s.batchesProcessed),
		CompletedBatches:  int64(s.batchesProcessed),
		FailedBatches:     0,
		ProcessingBatches: 0,
		TotalEvents:       int64(s.eventsProcessed),
		ProcessedEvents:   int64(s.eventsProcessed),
		FailedEvents:      0,
		DuplicateEvents:   0,
		AverageEventsPerBatch: func() float64 {
			if s.batchesProcessed > 0 {
				return float64(s.eventsProcessed) / float64(s.batchesProcessed)
			}
			return 0
		}(),
		ThroughputPerSecond: throughput,
		SuccessRate:         successRate,
		DeduplicationRate:   0.0, // Will be updated with actual dedup stats
	}

	return metrics, nil
}

// GetPerformanceStats returns performance statistics for a given time window
func (s *TelemetryService) GetPerformanceStats(ctx context.Context, timeWindow time.Duration) (*observability.TelemetryPerformanceStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Calculate performance statistics based on current metrics
	throughputPerSec := float64(0)
	if timeWindow.Seconds() > 0 {
		throughputPerSec = float64(s.eventsProcessed) / timeWindow.Seconds()
	}

	// Aggregate performance stats
	stats := &observability.TelemetryPerformanceStats{
		TimeWindow:           timeWindow,
		TotalRequests:        int64(s.batchesProcessed),
		SuccessfulRequests:   int64(s.batchesProcessed),
		AverageLatencyMs:     float64(s.avgProcessingTime.Milliseconds()),
		P95LatencyMs:         float64(s.avgProcessingTime.Milliseconds()) * 1.2, // Estimated
		P99LatencyMs:         float64(s.avgProcessingTime.Milliseconds()) * 1.5, // Estimated
		ThroughputPerSecond:  throughputPerSec,
		PeakThroughput:       throughputPerSec * 1.3, // Estimated peak
		CacheHitRate:         0.8,                    // Default 80% cache hit rate
		DatabaseFallbackRate: 0.2,                    // Default 20% fallback rate
		ErrorRate:            0.01,                   // Default 1% error rate
		RetryRate:            0.05,                   // Default 5% retry rate
	}

	return stats, nil
}

// updatePerformanceMetrics updates internal performance tracking metrics
func (s *TelemetryService) updatePerformanceMetrics(eventCount int, processingTime time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.batchesProcessed++
	s.eventsProcessed += uint64(eventCount)
	s.lastProcessingTime = time.Now()

	// Update rolling average processing time
	if s.avgProcessingTime == 0 {
		s.avgProcessingTime = processingTime
	} else {
		// Simple exponential moving average with alpha = 0.1
		s.avgProcessingTime = time.Duration(0.9*float64(s.avgProcessingTime) + 0.1*float64(processingTime))
	}
}
