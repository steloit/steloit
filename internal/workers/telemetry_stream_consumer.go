package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/observability"
	observabilitySvc "brokle/internal/core/services/observability"
	"brokle/internal/infrastructure/database"
	"brokle/internal/infrastructure/streams"
	"brokle/pkg/uid"
)

var (
	ErrMovedToDLQ = errors.New("message moved to DLQ")
)

const (
	dlqStreamPrefix    = "telemetry:dlq:batches"
	dlqRetentionPeriod = 7 * 24 * time.Hour
	dlqMaxLength       = 1000
)

// TelemetryStreamConsumer consumes telemetry batches from Redis Streams and writes to ClickHouse
type TelemetryStreamConsumer struct {
	deduplicationSvc    observability.TelemetryDeduplicationService
	traceService        observability.TraceService
	scoreService        observability.ScoreService
	metricsService      observability.MetricsService
	logsService         observability.LogsService
	genaiEventsService  observability.GenAIEventsService
	archiveService      *observabilitySvc.ArchiveService
	archiveConfig       *config.ArchiveConfig
	redis               *database.RedisDB
	logger              *slog.Logger
	activeStreams       map[string]bool
	quit                chan struct{}
	consumerGroup       string
	consumerID          string
	wg                  sync.WaitGroup
	discoveryInterval   time.Duration
	batchesProcessed    int64
	maxStreamsPerRead   int
	running             int64
	maxRetries          int
	blockDuration       time.Duration
	maxDiscoveryBackoff time.Duration
	retryBackoff        time.Duration
	eventsProcessed     int64
	errorsCount         int64
	dlqMessagesCount    int64
	archiveErrorsCount  int64
	batchSize           int
	discoveryBackoff    time.Duration
	streamRotation      int
	streamsMutex        sync.RWMutex
	statsLock           sync.RWMutex
}

// TelemetryStreamConsumerConfig holds configuration for the consumer
type TelemetryStreamConsumerConfig struct {
	ConsumerGroup     string
	ConsumerID        string
	BatchSize         int
	BlockDuration     time.Duration
	MaxRetries        int
	RetryBackoff      time.Duration
	DiscoveryInterval time.Duration
	MaxStreamsPerRead int
}

// NewTelemetryStreamConsumer creates a new telemetry stream consumer
func NewTelemetryStreamConsumer(
	redis *database.RedisDB,
	deduplicationSvc observability.TelemetryDeduplicationService,
	logger *slog.Logger,
	consumerConfig *TelemetryStreamConsumerConfig,
	traceService observability.TraceService,
	scoreService observability.ScoreService,
	metricsService observability.MetricsService,
	logsService observability.LogsService,
	genaiEventsService observability.GenAIEventsService,
	archiveService *observabilitySvc.ArchiveService,
	archiveConfig *config.ArchiveConfig,
) *TelemetryStreamConsumer {
	if consumerConfig == nil {
		consumerConfig = &TelemetryStreamConsumerConfig{
			ConsumerGroup:     "telemetry-workers",
			ConsumerID:        "worker-" + uid.New().String(),
			BatchSize:         50,
			BlockDuration:     time.Second,
			MaxRetries:        3,
			RetryBackoff:      500 * time.Millisecond,
			DiscoveryInterval: 30 * time.Second,
			MaxStreamsPerRead: 10,
		}
	}

	return &TelemetryStreamConsumer{
		redis:               redis,
		deduplicationSvc:    deduplicationSvc,
		logger:              logger,
		traceService:        traceService,
		scoreService:        scoreService,
		metricsService:      metricsService,
		logsService:         logsService,
		genaiEventsService:  genaiEventsService,
		archiveService:      archiveService,
		archiveConfig:       archiveConfig,
		consumerGroup:       consumerConfig.ConsumerGroup,
		consumerID:          consumerConfig.ConsumerID,
		batchSize:           consumerConfig.BatchSize,
		blockDuration:       consumerConfig.BlockDuration,
		maxRetries:          consumerConfig.MaxRetries,
		retryBackoff:        consumerConfig.RetryBackoff,
		discoveryInterval:   consumerConfig.DiscoveryInterval,
		maxStreamsPerRead:   consumerConfig.MaxStreamsPerRead,
		quit:                make(chan struct{}),
		activeStreams:       make(map[string]bool),
		discoveryBackoff:    time.Second,
		maxDiscoveryBackoff: 30 * time.Second,
	}
}

// Start begins consuming from Redis Streams
func (c *TelemetryStreamConsumer) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&c.running, 0, 1) {
		return errors.New("consumer already running")
	}

	c.logger.Info("Starting telemetry stream consumer", "consumer_group", c.consumerGroup, "consumer_id", c.consumerID, "batch_size", c.batchSize, "discovery_interval", c.discoveryInterval, "max_streams", c.maxStreamsPerRead)

	// Start consumption loop
	c.wg.Add(1)
	go c.consumeLoop(ctx)

	// Start stream discovery loop
	c.wg.Add(1)
	go c.discoveryLoop(ctx)

	return nil
}

// Stop gracefully stops the consumer
func (c *TelemetryStreamConsumer) Stop() {
	if !atomic.CompareAndSwapInt64(&c.running, 1, 0) {
		return
	}

	c.logger.Info("Stopping telemetry stream consumer")
	close(c.quit)
	c.wg.Wait()

	c.statsLock.RLock()
	c.logger.Info("Telemetry stream consumer stopped", "batches_processed", c.batchesProcessed, "events_processed", c.eventsProcessed, "errors_count", c.errorsCount)
	c.statsLock.RUnlock()
}

// discoveryLoop periodically discovers and initializes new streams
func (c *TelemetryStreamConsumer) discoveryLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.discoveryInterval)
	defer ticker.Stop()

	// Initial discovery on startup
	if err := c.performDiscovery(ctx); err != nil {
		c.logger.Error("Initial stream discovery failed", "error", err)
	}

	for {
		select {
		case <-c.quit:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.performDiscovery(ctx); err != nil {
				c.logger.Error("Stream discovery failed, backing off", "error", err, "backoff", c.discoveryBackoff)

				// Exponential backoff on failure
				time.Sleep(c.discoveryBackoff)
				c.discoveryBackoff = minDuration(c.discoveryBackoff*2, c.maxDiscoveryBackoff)
			} else {
				// Reset backoff on success
				c.discoveryBackoff = time.Second
			}
		}
	}
}

// performDiscovery discovers streams and initializes consumer groups
func (c *TelemetryStreamConsumer) performDiscovery(ctx context.Context) error {
	streams, err := c.discoverStreams(ctx)
	if err != nil {
		return err
	}

	if len(streams) == 0 {
		c.logger.Debug("No telemetry streams discovered")
		return nil
	}

	// Cleanup inactive streams before adding new ones
	c.cleanupInactiveStreams(streams)

	return c.ensureConsumerGroups(ctx, streams)
}

// discoverStreams discovers active telemetry streams using Redis SCAN
func (c *TelemetryStreamConsumer) discoverStreams(ctx context.Context) ([]string, error) {
	var allStreams []string
	cursor := uint64(0)
	pattern := "telemetry:batches:*"

	// Use SCAN for production-safe iteration (non-blocking)
	for {
		keys, nextCursor, err := c.redis.Client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan streams: %w", err)
		}

		allStreams = append(allStreams, keys...)
		cursor = nextCursor

		if cursor == 0 {
			break // Completed full scan
		}
	}

	c.logger.Debug("Discovered telemetry streams", "stream_count", len(allStreams), "pattern", pattern)

	return allStreams, nil
}

// ensureConsumerGroups creates consumer groups for discovered streams
func (c *TelemetryStreamConsumer) ensureConsumerGroups(ctx context.Context, streams []string) error {
	for _, streamKey := range streams {
		// Check if stream is new
		c.streamsMutex.RLock()
		exists := c.activeStreams[streamKey]
		c.streamsMutex.RUnlock()

		if exists {
			continue // Already initialized
		}

		// Create consumer group (idempotent operation)
		// Use "0" to read from beginning, ensuring we don't miss messages that arrived before consumer started
		err := c.redis.Client.XGroupCreateMkStream(ctx, streamKey, c.consumerGroup, "0").Err()
		if err != nil {
			// Ignore BUSYGROUP error (group already exists)
			if !strings.Contains(err.Error(), "BUSYGROUP") {
				c.logger.Warn("Failed to create consumer group", "error", err, "stream", streamKey)
				continue
			}
		}

		// Mark as active
		c.streamsMutex.Lock()
		c.activeStreams[streamKey] = true
		c.streamsMutex.Unlock()

		c.logger.Debug("Consumer group initialized for stream", "stream", streamKey, "consumer_group", c.consumerGroup)
	}

	return nil
}

// cleanupInactiveStreams removes streams that no longer exist in Redis
func (c *TelemetryStreamConsumer) cleanupInactiveStreams(discoveredStreams []string) {
	// Build set of current streams for O(1) lookup
	currentStreams := make(map[string]bool, len(discoveredStreams))
	for _, streamKey := range discoveredStreams {
		currentStreams[streamKey] = true
	}

	// Find streams that are active but no longer exist
	c.streamsMutex.Lock()
	defer c.streamsMutex.Unlock()

	var removedStreams []string
	for streamKey := range c.activeStreams {
		if !currentStreams[streamKey] {
			delete(c.activeStreams, streamKey)
			removedStreams = append(removedStreams, streamKey)
		}
	}

	if len(removedStreams) > 0 {
		c.logger.Info("Cleaned up inactive streams", "removed_count", len(removedStreams), "streams", removedStreams)
	}
}

// minDuration returns the minimum of two durations
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// consumeLoop is the main consumption loop
func (c *TelemetryStreamConsumer) consumeLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-c.quit:
			return
		case <-ctx.Done():
			return
		default:
			if err := c.consumeBatch(ctx); err != nil {
				if err != redis.Nil {
					c.logger.Error("Error consuming batch", "error", err)
					c.incrementErrors()
				}
				// Brief pause on error to prevent tight error loops
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

// consumeBatch reads and processes a batch of messages from discovered streams
func (c *TelemetryStreamConsumer) consumeBatch(ctx context.Context) error {
	// Get active streams with rotation
	c.streamsMutex.Lock()
	var allStreamKeys []string
	for streamKey := range c.activeStreams {
		allStreamKeys = append(allStreamKeys, streamKey)
	}

	// Round-robin rotation for fairness
	// Ensures all projects get processed even if total streams > maxStreamsPerRead
	if len(allStreamKeys) > 0 && c.streamRotation >= len(allStreamKeys) {
		c.streamRotation = 0 // Reset rotation
	}

	// Rotate streams for fair distribution
	if c.streamRotation > 0 && len(allStreamKeys) > c.streamRotation {
		allStreamKeys = append(allStreamKeys[c.streamRotation:], allStreamKeys[:c.streamRotation]...)
	}
	c.streamsMutex.Unlock()

	if len(allStreamKeys) == 0 {
		// No streams discovered yet - wait for discovery loop
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	// Limit streams per read (Redis best practice)
	streamKeys := allStreamKeys
	if len(streamKeys) > c.maxStreamsPerRead {
		streamKeys = streamKeys[:c.maxStreamsPerRead]
	}

	// Build XReadGroup arguments with ">" marker for each stream
	streamArgs := make([]string, 0, len(streamKeys)*2)
	for _, streamKey := range streamKeys {
		streamArgs = append(streamArgs, streamKey)
	}
	for range streamKeys {
		streamArgs = append(streamArgs, ">") // Read only new messages
	}

	// Read from multiple streams
	streams, err := c.redis.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.consumerGroup,
		Consumer: c.consumerID,
		Streams:  streamArgs,
		Count:    int64(c.batchSize),
		Block:    c.blockDuration,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			// No messages available - normal condition (this is expected most of the time)
			return nil
		}
		c.logger.Error("XReadGroup failed", "error", err)
		return err
	}

	// Process messages from all streams
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			// Process message (may succeed, move to DLQ, or fail before DLQ)
			err := c.processMessage(ctx, stream.Stream, msg)

			// Determine if message should be acknowledged:
			// - Success (err == nil): Data in ClickHouse → Safe to ack
			// - In DLQ (ErrMovedToDLQ): Data preserved in DLQ → Safe to ack
			// - Failed before DLQ (other errors): No data preservation → Leave pending for retry
			shouldAck := err == nil || errors.Is(err, ErrMovedToDLQ)

			if shouldAck {
				// Acknowledge message - either processed successfully or safely in DLQ
				if ackErr := c.redis.Client.XAck(ctx, stream.Stream, c.consumerGroup, msg.ID).Err(); ackErr != nil {
					c.logger.Warn("Failed to acknowledge message", "error", ackErr,
						"stream", stream.Stream,
						"message_id", msg.ID)
				}
			} else {
				// Leave message pending for retry (parse errors, DLQ write failures, etc.)
				c.logger.Error("Message processing failed - leaving pending for retry", "error", err, "stream", stream.Stream, "message_id", msg.ID)
			}

			// Track errors for non-DLQ failures
			if err != nil && !errors.Is(err, ErrMovedToDLQ) {
				c.incrementErrors()
			}
		}
	}

	// Increment rotation for next read
	c.streamsMutex.Lock()
	c.streamRotation += c.maxStreamsPerRead
	c.streamsMutex.Unlock()

	return nil
}

// processMessage processes a single stream message
func (c *TelemetryStreamConsumer) processMessage(ctx context.Context, streamKey string, msg redis.XMessage) error {
	// Extract batch data from message
	dataStr, ok := msg.Values["data"].(string)
	if !ok {
		return errors.New("invalid message format: missing data field")
	}

	// Deserialize batch
	var batch streams.TelemetryStreamMessage
	if err := json.Unmarshal([]byte(dataStr), &batch); err != nil {
		return fmt.Errorf("failed to unmarshal batch data: %w", err)
	}

	// Process with retry logic
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * c.retryBackoff
			c.logger.Info("Retrying batch processing", "batch_id", batch.BatchID.String(), "attempt", attempt, "backoff", backoff)
			time.Sleep(backoff)
		}

		if err := c.processBatch(ctx, &batch); err != nil {
			lastErr = err
			continue
		}

		// Success
		c.incrementStats(1, int64(len(batch.Events)))
		c.logger.Debug("Successfully processed batch from stream", "batch_id", batch.BatchID.String(), "project_id", batch.ProjectID.String(), "event_count", len(batch.Events), "message_id", msg.ID)
		return nil
	}

	// Max retries exceeded - move to DLQ
	if err := c.moveToDLQ(ctx, streamKey, msg, &batch, lastErr); err != nil {
		c.logger.Error("Failed to move message to DLQ", "error", err)
		// DLQ write failed - keep claims held, message stays pending
		return fmt.Errorf("max retries exceeded AND failed to move to DLQ: %w", lastErr)
	}

	// DLQ write succeeded - release claims so client retries can proceed
	if len(batch.ClaimedSpanIDs) > 0 {
		if releaseErr := c.deduplicationSvc.ReleaseEvents(ctx, batch.ClaimedSpanIDs); releaseErr != nil {
			c.logger.Error("Failed to release claims after DLQ write", "error", releaseErr, "batch_id", batch.BatchID.String(), "event_count", len(batch.ClaimedSpanIDs))
			// Don't fail - DLQ write succeeded, worst case: 24h TTL expires
		} else {
			c.logger.Info("Released claims after moving batch to DLQ", "batch_id", batch.BatchID.String(), "event_count", len(batch.ClaimedSpanIDs))
		}
	}

	// Successfully moved to DLQ and released claims
	return ErrMovedToDLQ
}

// sortEventsByDependency sorts events to ensure dependencies are processed first
func (c *TelemetryStreamConsumer) sortEventsByDependency(events []streams.TelemetryEventData) []streams.TelemetryEventData {
	// Define processing priority (lower = processed first)
	eventPriority := map[observability.TelemetryEventType]int{
		observability.TelemetryEventTypeSpan:         1, // Spans first
		observability.TelemetryEventTypeQualityScore: 2, // Scores last (require spans)
	}

	// Create a copy to avoid modifying original slice
	sorted := make([]streams.TelemetryEventData, len(events))
	copy(sorted, events)

	// Stable sort by priority
	sort.SliceStable(sorted, func(i, j int) bool {
		typeI := observability.TelemetryEventType(sorted[i].EventType)
		typeJ := observability.TelemetryEventType(sorted[j].EventType)

		priorityI := eventPriority[typeI]
		priorityJ := eventPriority[typeJ]

		// Unknown types get lowest priority (processed last)
		if priorityI == 0 {
			priorityI = 999
		}
		if priorityJ == 0 {
			priorityJ = 999
		}

		return priorityI < priorityJ
	})

	return sorted
}

// safeExtractFromPayload safely extracts string from payload map (nil-safe)
func safeExtractFromPayload(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}

	val, ok := payload[key]
	if !ok {
		return ""
	}

	strVal, ok := val.(string)
	if !ok {
		return ""
	}

	return strVal
}

// mapToStruct converts map[string]interface{} to a struct using JSON marshaling
// This is a type-safe way to convert event payloads to domain types
func mapToStruct(input map[string]interface{}, output interface{}) error {
	// Marshal map to JSON
	jsonData, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal map: %w", err)
	}

	// Unmarshal JSON to struct
	if err := json.Unmarshal(jsonData, output); err != nil {
		return fmt.Errorf("failed to unmarshal to struct: %w", err)
	}

	return nil
}

// processBatch processes a batch of telemetry events by routing to appropriate services based on event type
// This is the main orchestration method that decides how to handle each event
func (c *TelemetryStreamConsumer) processBatch(ctx context.Context, batch *streams.TelemetryStreamMessage) error {
	// batch.ProjectID is already uuid.UUID type
	projectID := batch.ProjectID

	// Track processing stats
	var (
		processedCount int
		failedCount    int
		lastError      error
	)

	// Sort events by dependency order: traces → sessions → spans → scores
	// This ensures parent entities exist before children are created
	sortedEvents := c.sortEventsByDependency(batch.Events)

	// Group events by type for batch insertion
	spans := make([]*observability.Span, 0, len(sortedEvents))
	scores := make([]*observability.Score, 0)
	metricsSums := make([]*observability.MetricSum, 0)
	metricsGauges := make([]*observability.MetricGauge, 0)
	metricsHistograms := make([]*observability.MetricHistogram, 0)
	metricsExpHistograms := make([]*observability.MetricExponentialHistogram, 0)
	logs := make([]*observability.Log, 0)
	genaiEvents := make([]*observability.GenAIEvent, 0)

	// Group events by type
	for _, event := range sortedEvents {
		switch observability.TelemetryEventType(event.EventType) {
		case observability.TelemetryEventTypeSpan:
			// Map event to Span struct
			var span observability.Span
			if err := mapToStruct(event.EventPayload, &span); err != nil {
				c.logger.Error("Failed to unmarshal span payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			span.ProjectID = projectID
			span.OrganizationID = batch.OrganizationID
			spans = append(spans, &span)

		case observability.TelemetryEventTypeQualityScore:
			// Map event to Score struct
			var score observability.Score
			if err := mapToStruct(event.EventPayload, &score); err != nil {
				c.logger.Error("Failed to unmarshal score payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			score.ProjectID = projectID
			score.OrganizationID = batch.OrganizationID
			scores = append(scores, &score)

		case observability.TelemetryEventTypeMetricSum:
			// Map event to MetricSum struct
			var metricSum observability.MetricSum
			if err := mapToStruct(event.EventPayload, &metricSum); err != nil {
				c.logger.Error("Failed to unmarshal metric_sum payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			metricSum.ProjectID = projectID
			metricsSums = append(metricsSums, &metricSum)

		case observability.TelemetryEventTypeMetricGauge:
			// Map event to MetricGauge struct
			var metricGauge observability.MetricGauge
			if err := mapToStruct(event.EventPayload, &metricGauge); err != nil {
				c.logger.Error("Failed to unmarshal metric_gauge payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			metricGauge.ProjectID = projectID
			metricsGauges = append(metricsGauges, &metricGauge)

		case observability.TelemetryEventTypeMetricHistogram:
			// Map event to MetricHistogram struct
			var metricHistogram observability.MetricHistogram
			if err := mapToStruct(event.EventPayload, &metricHistogram); err != nil {
				c.logger.Error("Failed to unmarshal metric_histogram payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			metricHistogram.ProjectID = projectID
			metricsHistograms = append(metricsHistograms, &metricHistogram)

		case observability.TelemetryEventTypeMetricExponentialHistogram:
			// Map event to MetricExponentialHistogram struct
			var metricExpHistogram observability.MetricExponentialHistogram
			if err := mapToStruct(event.EventPayload, &metricExpHistogram); err != nil {
				c.logger.Error("Failed to unmarshal metric_exponential_histogram payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			metricExpHistogram.ProjectID = projectID
			metricsExpHistograms = append(metricsExpHistograms, &metricExpHistogram)

		case observability.TelemetryEventTypeLog:
			// Map event to Log struct
			var log observability.Log
			if err := mapToStruct(event.EventPayload, &log); err != nil {
				c.logger.Error("Failed to unmarshal log payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			log.ProjectID = projectID
			logs = append(logs, &log)

		case observability.TelemetryEventTypeGenAIEvent:
			// Map event to GenAIEvent struct
			var genaiEvent observability.GenAIEvent
			if err := mapToStruct(event.EventPayload, &genaiEvent); err != nil {
				c.logger.Error("Failed to unmarshal genai_event payload", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
				failedCount++
				continue
			}
			genaiEvent.ProjectID = projectID
			genaiEvents = append(genaiEvents, &genaiEvent)

		default:
			// Unknown event type - log warning and skip
			c.logger.Warn("Unknown event type, skipping", "event_id", event.EventID.String(), "event_type", event.EventType, "batch_id", batch.BatchID.String())
			failedCount++
		}
	}

	// Bulk insert spans
	if len(spans) > 0 {
		if err := c.traceService.IngestSpanBatch(ctx, spans); err != nil {
			c.logger.Error("Failed to ingest span batch", "error", err, "batch_id", batch.BatchID.String(), "span_count", len(spans))
			failedCount += len(spans)
			lastError = err
		} else {
			processedCount += len(spans)
		}
	}

	// Bulk insert scores (1 DB call for all scores)
	if len(scores) > 0 {
		if err := c.scoreService.CreateScoreBatch(ctx, scores); err != nil {
			c.logger.Error("Failed to create score batch", "error", err, "batch_id", batch.BatchID.String(), "score_count", len(scores))
			failedCount += len(scores)
			lastError = err
		} else {
			processedCount += len(scores)
		}
	}

	// Bulk insert metric sums (1 DB call for all metric sums)
	if len(metricsSums) > 0 {
		if err := c.metricsService.CreateMetricSumBatch(ctx, metricsSums); err != nil {
			c.logger.Error("Failed to create metric sum batch", "error", err, "batch_id", batch.BatchID.String(), "metric_sum_count", len(metricsSums))
			failedCount += len(metricsSums)
			lastError = err
		} else {
			processedCount += len(metricsSums)
		}
	}

	// Bulk insert metric gauges (1 DB call for all metric gauges)
	if len(metricsGauges) > 0 {
		if err := c.metricsService.CreateMetricGaugeBatch(ctx, metricsGauges); err != nil {
			c.logger.Error("Failed to create metric gauge batch", "error", err, "batch_id", batch.BatchID.String(), "metric_gauge_count", len(metricsGauges))
			failedCount += len(metricsGauges)
			lastError = err
		} else {
			processedCount += len(metricsGauges)
		}
	}

	// Bulk insert metric histograms (1 DB call for all metric histograms)
	if len(metricsHistograms) > 0 {
		if err := c.metricsService.CreateMetricHistogramBatch(ctx, metricsHistograms); err != nil {
			c.logger.Error("Failed to create metric histogram batch", "error", err, "batch_id", batch.BatchID.String(), "metric_histogram_count", len(metricsHistograms))
			failedCount += len(metricsHistograms)
			lastError = err
		} else {
			processedCount += len(metricsHistograms)
		}
	}

	// Bulk insert exponential histograms (1 DB call for all exponential histograms)
	if len(metricsExpHistograms) > 0 {
		if err := c.metricsService.CreateMetricExponentialHistogramBatch(ctx, metricsExpHistograms); err != nil {
			c.logger.Error("Failed to create metric exponential histogram batch", "error", err, "batch_id", batch.BatchID.String(), "metric_exponential_histogram_count", len(metricsExpHistograms))
			failedCount += len(metricsExpHistograms)
			lastError = err
		} else {
			processedCount += len(metricsExpHistograms)
		}
	}

	// Bulk insert logs (1 DB call for all logs)
	if len(logs) > 0 {
		if err := c.logsService.CreateLogBatch(ctx, logs); err != nil {
			c.logger.Error("Failed to create log batch", "error", err, "batch_id", batch.BatchID.String(), "log_count", len(logs))
			failedCount += len(logs)
			lastError = err
		} else {
			processedCount += len(logs)
		}
	}

	// Bulk insert GenAI events (1 DB call for all GenAI events)
	if len(genaiEvents) > 0 {
		if err := c.genaiEventsService.CreateGenAIEventBatch(ctx, genaiEvents); err != nil {
			c.logger.Error("Failed to create GenAI event batch", "error", err, "batch_id", batch.BatchID.String(), "genai_event_count", len(genaiEvents))
			failedCount += len(genaiEvents)
			lastError = err
		} else {
			processedCount += len(genaiEvents)
		}
	}

	// Determine success: At least one event processed successfully
	if processedCount == 0 && failedCount > 0 {
		return fmt.Errorf("batch processing failed: 0/%d events processed, last error: %w", len(batch.Events), lastError)
	}

	// Log partial failures
	if failedCount > 0 {
		c.logger.Warn("Batch processed with partial failures", "batch_id", batch.BatchID.String(), "total_events", len(batch.Events), "processed_count", processedCount, "failed_count", failedCount)
	}

	// ✅ Deduplication already handled synchronously in HTTP handler via ClaimEvents
	// Events are claimed atomically before publishing to stream, so no async registration needed
	// This eliminates the race condition where duplicate check happens before async registration completes

	// ============================================================================
	// Fire-and-Forget S3 Archival (Non-Blocking)
	// ============================================================================
	// Archive raw telemetry to S3 for:
	// - Long-term compliance retention (6+ years)
	// - Vendor portability (replay capability)
	// - Cost-efficient cold storage (~50% savings)
	//
	// This runs in a goroutine to NOT block message ACK.
	// ClickHouse is source of truth; S3 archival is best-effort.
	if c.isArchiveEnabled() && processedCount > 0 {
		// Extract raw records from batch events
		rawRecords := c.extractRawRecords(batch)
		if len(rawRecords) > 0 {
			// Fire-and-forget: goroutine handles S3 upload with retry
			// Note: We intentionally use a fresh context here because the parent context
			// may be cancelled after message ACK, but we want the archive to complete.
			go c.archiveBatchToS3(context.Background(), batch, rawRecords) //nolint:contextcheck // intentional - fire-and-forget pattern
		}
	}

	return nil
}

// ptrTime is a helper to create a pointer to a time.Time value
func ptrTime(t time.Time) *time.Time {
	return &t
}

// serializeMetadata converts metadata map to JSON string
func (c *TelemetryStreamConsumer) serializeMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return "{}"
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		c.logger.Warn("Failed to serialize metadata", "error", err)
		return "{}"
	}

	return string(data)
}

// incrementStats atomically increments processing statistics
func (c *TelemetryStreamConsumer) incrementStats(batches, events int64) {
	atomic.AddInt64(&c.batchesProcessed, batches)
	atomic.AddInt64(&c.eventsProcessed, events)
}

// incrementErrors atomically increments error count
func (c *TelemetryStreamConsumer) incrementErrors() {
	atomic.AddInt64(&c.errorsCount, 1)
}

// moveToDLQ moves a failed message to the Dead Letter Queue
func (c *TelemetryStreamConsumer) moveToDLQ(ctx context.Context, streamKey string, msg redis.XMessage, batch *streams.TelemetryStreamMessage, err error) error {
	dlqKey := fmt.Sprintf("%s:%s", dlqStreamPrefix, batch.ProjectID.String())

	// Serialize DLQ entry with error metadata
	dlqData := map[string]interface{}{
		"original_stream": streamKey,
		"original_msg_id": msg.ID,
		"batch_id":        batch.BatchID.String(),
		"project_id":      batch.ProjectID.String(),
		"event_count":     len(batch.Events),
		"error_message":   err.Error(),
		"failed_at":       time.Now().Unix(),
		"retry_count":     c.maxRetries,
		"original_data":   msg.Values["data"], // Preserve original message data
	}

	// Add to DLQ stream with trimming and TTL
	result, addErr := c.redis.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqKey,
		MaxLen: dlqMaxLength, // Prevent unbounded growth
		Approx: true,
		Values: dlqData,
	}).Result()

	if addErr != nil {
		return fmt.Errorf("failed to add message to DLQ: %w", addErr)
	}

	// Set TTL on DLQ stream (7 days retention)
	if err := c.redis.Client.Expire(ctx, dlqKey, dlqRetentionPeriod).Err(); err != nil {
		c.logger.Warn("Failed to set DLQ TTL", "error", err)
	}

	// Increment DLQ counter
	atomic.AddInt64(&c.dlqMessagesCount, 1)

	c.logger.Warn("Moved failed batch to Dead Letter Queue", "dlq_id", result, "dlq_key", dlqKey, "batch_id", batch.BatchID.String(), "project_id", batch.ProjectID.String(), "error", err.Error(), "retry_count", c.maxRetries)

	return nil
}

// GetStats returns current consumer statistics
func (c *TelemetryStreamConsumer) GetStats() map[string]int64 {
	c.streamsMutex.RLock()
	activeStreamCount := int64(len(c.activeStreams))
	c.streamsMutex.RUnlock()

	return map[string]int64{
		"batches_processed": atomic.LoadInt64(&c.batchesProcessed),
		"events_processed":  atomic.LoadInt64(&c.eventsProcessed),
		"errors_count":      atomic.LoadInt64(&c.errorsCount),
		"dlq_messages":      atomic.LoadInt64(&c.dlqMessagesCount),
		"archive_errors":    atomic.LoadInt64(&c.archiveErrorsCount),
		"active_streams":    activeStreamCount,
	}
}

// GetDLQMessages retrieves messages from the Dead Letter Queue for a project
func (c *TelemetryStreamConsumer) GetDLQMessages(ctx context.Context, projectID uuid.UUID, count int64) ([]redis.XMessage, error) {
	dlqKey := fmt.Sprintf("%s:%s", dlqStreamPrefix, projectID.String())

	// Read messages from DLQ
	messages, err := c.redis.Client.XRevRange(ctx, dlqKey, "+", "-").Result()
	if err != nil {
		if err == redis.Nil {
			return []redis.XMessage{}, nil
		}
		return nil, fmt.Errorf("failed to read DLQ messages: %w", err)
	}

	// Limit results
	if count > 0 && int64(len(messages)) > count {
		messages = messages[:count]
	}

	return messages, nil
}

// RetryDLQMessage attempts to reprocess a message from the DLQ
func (c *TelemetryStreamConsumer) RetryDLQMessage(ctx context.Context, projectID uuid.UUID, messageID string) error {
	dlqKey := fmt.Sprintf("%s:%s", dlqStreamPrefix, projectID.String())

	// Read the message
	messages, err := c.redis.Client.XRange(ctx, dlqKey, messageID, messageID).Result()
	if err != nil || len(messages) == 0 {
		return fmt.Errorf("DLQ message not found: %s", messageID)
	}

	msg := messages[0]

	// Extract original data
	originalData, ok := msg.Values["original_data"].(string)
	if !ok {
		return errors.New("invalid DLQ message format: missing original_data")
	}

	// Deserialize batch
	var batch streams.TelemetryStreamMessage
	if err := json.Unmarshal([]byte(originalData), &batch); err != nil {
		return fmt.Errorf("failed to unmarshal DLQ batch data: %w", err)
	}

	// Attempt reprocessing
	if err := c.processBatch(ctx, &batch); err != nil {
		return fmt.Errorf("retry failed: %w", err)
	}

	// Remove from DLQ on success
	if err := c.redis.Client.XDel(ctx, dlqKey, messageID).Err(); err != nil {
		c.logger.Warn("Failed to remove message from DLQ after successful retry", "error", err)
	}

	c.logger.Info("Successfully retried DLQ message", "message_id", messageID, "batch_id", batch.BatchID.String(), "project_id", projectID.String())

	return nil
}

// ============================================================================
// S3 Raw Telemetry Archival (Fire-and-Forget)
// ============================================================================

// Archive constants
const (
	// archiveTimeout is the maximum time to wait for S3 upload
	archiveTimeout = 30 * time.Second

	// archiveMaxRetries is the number of retry attempts for transient S3 errors
	archiveMaxRetries = 3

	// archiveBaseBackoff is the initial backoff duration for retries
	archiveBaseBackoff = 100 * time.Millisecond
)

// extractRawRecords extracts raw OTLP JSON from batch events for S3 archival.
// This is an extraction-layer approach - we pull raw data from the batch
// without modifying the existing converters or data flow.
func (c *TelemetryStreamConsumer) extractRawRecords(batch *streams.TelemetryStreamMessage) []observability.RawTelemetryRecord {
	if batch == nil || len(batch.Events) == 0 {
		return nil
	}

	now := time.Now()
	records := make([]observability.RawTelemetryRecord, 0, len(batch.Events))

	for _, event := range batch.Events {
		// Serialize the full event payload to JSON for raw storage
		rawJSON, err := json.Marshal(event.EventPayload)
		if err != nil {
			c.logger.Warn("Failed to marshal event payload for archive", "error", err, "event_id", event.EventID.String(), "batch_id", batch.BatchID.String())
			continue
		}

		// Extract trace/span IDs - use event fields first, then fallback to payload
		traceID := event.TraceID
		if traceID == "" {
			traceID = safeExtractFromPayload(event.EventPayload, "trace_id")
		}
		spanID := event.SpanID
		if spanID == "" {
			spanID = safeExtractFromPayload(event.EventPayload, "span_id")
		}

		// Map event type to signal type
		signalType := mapEventTypeToSignal(observability.TelemetryEventType(event.EventType))

		// Extract timestamp from event payload or use current time as fallback
		timestamp := now
		if ts, ok := event.EventPayload["timestamp"].(string); ok {
			if parsedTS, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				timestamp = parsedTS
			}
		} else if ts, ok := event.EventPayload["start_time"].(string); ok {
			// Span events use start_time
			if parsedTS, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				timestamp = parsedTS
			}
		}

		records = append(records, observability.RawTelemetryRecord{
			RecordID:    event.EventID.String(),
			ProjectID:   batch.ProjectID.String(),
			SignalType:  signalType,
			Timestamp:   timestamp,
			TraceID:     traceID,
			SpanID:      spanID,
			SpanJSONRaw: string(rawJSON),
			ArchivedAt:  now,
		})
	}

	return records
}

// mapEventTypeToSignal converts TelemetryEventType to archive signal type
func mapEventTypeToSignal(eventType observability.TelemetryEventType) string {
	switch eventType {
	case observability.TelemetryEventTypeSpan:
		return observability.SignalTypeTraces
	case observability.TelemetryEventTypeLog:
		return observability.SignalTypeLogs
	case observability.TelemetryEventTypeMetricSum,
		observability.TelemetryEventTypeMetricGauge,
		observability.TelemetryEventTypeMetricHistogram,
		observability.TelemetryEventTypeMetricExponentialHistogram:
		return observability.SignalTypeMetrics
	case observability.TelemetryEventTypeGenAIEvent:
		return observability.SignalTypeGenAI
	default:
		return observability.SignalTypeTraces // Default to traces for unknown types
	}
}

// groupRecordsBySignalAndDay groups raw telemetry records by signal type AND day.
// This ensures each Parquet file contains only one signal type for one day,
// enabling correct Hive-style partitioning (project_id/signal/year/month/day).
// Returns: signalType -> dateKey (YYYY-MM-DD) -> []records
func (c *TelemetryStreamConsumer) groupRecordsBySignalAndDay(records []observability.RawTelemetryRecord) map[string]map[string][]observability.RawTelemetryRecord {
	groups := make(map[string]map[string][]observability.RawTelemetryRecord)
	for _, r := range records {
		dateKey := r.Timestamp.Format("2006-01-02")
		if groups[r.SignalType] == nil {
			groups[r.SignalType] = make(map[string][]observability.RawTelemetryRecord)
		}
		groups[r.SignalType][dateKey] = append(groups[r.SignalType][dateKey], r)
	}
	return groups
}

// archiveBatchToS3 archives raw telemetry records to S3 using fire-and-forget pattern.
// This method runs in a goroutine and does NOT block the main message ACK flow.
// S3 archival is best-effort - failures are logged but don't affect ClickHouse writes.
// Records are grouped by signal type AND day to ensure correct Hive-style partitioning.
func (c *TelemetryStreamConsumer) archiveBatchToS3(parentCtx context.Context, batch *streams.TelemetryStreamMessage, records []observability.RawTelemetryRecord) {
	// Create timeout context for S3 operations (derived from parent)
	ctx, cancel := context.WithTimeout(parentCtx, archiveTimeout)
	defer cancel()

	// Group records by signal type AND day to ensure correct partitioning
	// Each signal+day combination gets its own Parquet file in the correct partition
	recordsBySignalAndDay := c.groupRecordsBySignalAndDay(records)

	for signalType, dayGroups := range recordsBySignalAndDay {
		for _, dayRecords := range dayGroups {
			// Generate a unique batch ID for each signal+day group
			// This ensures each Parquet file has a unique name
			signalBatchID := uid.New()

			c.archiveSignalGroup(ctx, batch, signalType, signalBatchID, dayRecords)
		}
	}
}

// archiveSignalGroup archives a group of records of the same signal type to S3.
func (c *TelemetryStreamConsumer) archiveSignalGroup(
	ctx context.Context,
	batch *streams.TelemetryStreamMessage,
	signalType string,
	signalBatchID uuid.UUID,
	records []observability.RawTelemetryRecord,
) {
	// Retry logic with exponential backoff
	var lastErr error
	for attempt := range archiveMaxRetries {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			backoff := archiveBaseBackoff * time.Duration(1<<uint(attempt-1)) //nolint:gosec // attempt is always small (0-2)
			time.Sleep(backoff)

			c.logger.Debug("Retrying S3 archive", "batch_id", signalBatchID.String(), "signal_type", signalType, "attempt", attempt+1, "backoff", backoff)
		}

		result, err := c.archiveService.ArchiveBatch(ctx, batch.ProjectID, signalBatchID, records)
		if err == nil {
			// Success!
			c.logger.Debug("Successfully archived batch to S3", "batch_id", signalBatchID.String(), "project_id", batch.ProjectID.String(), "signal_type", signalType, "s3_path", result.S3Path, "record_count", result.RecordCount, "file_size", result.FileSizeBytes)
			return
		}

		lastErr = err

		// Check if error is transient (worth retrying)
		if !isTransientS3Error(err) {
			c.logger.Warn("Non-transient S3 archive error, not retrying", "error", err, "batch_id", signalBatchID.String(), "project_id", batch.ProjectID.String(), "signal_type", signalType)
			break
		}
	}

	// All retries exhausted or non-transient error
	atomic.AddInt64(&c.archiveErrorsCount, 1)
	c.logger.Error("Failed to archive batch to S3 after retries",
		"error", lastErr,
		"batch_id", signalBatchID.String(),
		"project_id", batch.ProjectID.String(),
		"signal_type", signalType,
		"record_count", len(records),
		"max_retries", archiveMaxRetries,
	)
}

// isTransientS3Error checks if an error is transient and worth retrying.
// Transient errors include network timeouts, temporary service unavailability, etc.
func isTransientS3Error(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for common transient error patterns
	transientPatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"service unavailable",
		"503",
		"500",
		"429", // Rate limiting
		"net/http",
		"i/o timeout",
		"TLS handshake",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// isArchiveEnabled returns true if S3 archival is enabled and properly configured
func (c *TelemetryStreamConsumer) isArchiveEnabled() bool {
	return c.archiveService != nil &&
		c.archiveConfig != nil &&
		c.archiveConfig.Enabled &&
		c.archiveService.IsEnabled()
}
