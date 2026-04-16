package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	"brokle/internal/infrastructure/database"
	"brokle/internal/infrastructure/streams"
	"brokle/pkg/uid"
)

const (
	evaluationJobsStream = "evaluation:jobs"
	evaluatorCacheTTL    = 30 * time.Second
)

// EvaluationJob represents a matched span-evaluator pair to be processed by EvaluationWorker
type EvaluationJob struct {
	JobID        uuid.UUID              `json:"job_id"`
	EvaluatorID  uuid.UUID              `json:"evaluator_id"`
	ProjectID    uuid.UUID              `json:"project_id"`
	ExecutionID  *uuid.UUID             `json:"execution_id,omitempty"` // Optional: links job to an evaluator execution (for manual triggers)
	SpanData     map[string]interface{} `json:"span_data"`
	TraceID      string                 `json:"trace_id"`
	SpanID       string                 `json:"span_id"`
	ScorerType   evaluation.ScorerType  `json:"scorer_type"`
	ScorerConfig map[string]any         `json:"scorer_config"`
	Variables    map[string]string      `json:"variables"` // Extracted variables from span
	CreatedAt    time.Time              `json:"created_at"`
}

// EvaluatorWorkerConfig holds configuration for the evaluator worker
type EvaluatorWorkerConfig struct {
	ConsumerGroup     string
	ConsumerID        string
	BatchSize         int
	BlockDuration     time.Duration
	MaxRetries        int
	RetryBackoff      time.Duration
	DiscoveryInterval time.Duration
	MaxStreamsPerRead int
	EvaluatorCacheTTL time.Duration
}

// EvaluatorCache provides thread-safe caching of active evaluators per project
type EvaluatorCache struct {
	cache map[string]evaluatorCacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

type evaluatorCacheEntry struct {
	evaluators []*evaluation.Evaluator
	expiresAt  time.Time
}

func NewEvaluatorCache(ttl time.Duration) *EvaluatorCache {
	return &EvaluatorCache{
		cache: make(map[string]evaluatorCacheEntry),
		ttl:   ttl,
	}
}

func (c *EvaluatorCache) Get(projectID string) ([]*evaluation.Evaluator, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[projectID]
	if !exists {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.evaluators, true
}

func (c *EvaluatorCache) Set(projectID string, evaluators []*evaluation.Evaluator) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[projectID] = evaluatorCacheEntry{
		evaluators: evaluators,
		expiresAt:  time.Now().Add(c.ttl),
	}
}

func (c *EvaluatorCache) Invalidate(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, projectID)
}

// EvaluatorWorker consumes spans from telemetry streams, matches against active evaluators, and emits evaluation jobs
type EvaluatorWorker struct {
	redis            *database.RedisDB
	evaluatorService evaluation.EvaluatorService
	executionService evaluation.EvaluatorExecutionService
	evaluatorCache   *EvaluatorCache
	logger           *slog.Logger

	// Consumer configuration
	consumerGroup     string
	consumerID        string
	batchSize         int
	blockDuration     time.Duration
	maxRetries        int
	retryBackoff      time.Duration
	discoveryInterval time.Duration
	maxStreamsPerRead int

	// State management
	activeStreams       map[string]bool
	streamsMutex        sync.RWMutex
	streamRotation      int
	quit                chan struct{}
	wg                  sync.WaitGroup
	running             int64
	discoveryBackoff    time.Duration
	maxDiscoveryBackoff time.Duration

	// Metrics
	spansProcessed    int64
	evaluatorsMatched int64
	jobsEmitted       int64
	errorsCount       int64
}

// NewEvaluatorWorker creates a new evaluator worker
func NewEvaluatorWorker(
	redis *database.RedisDB,
	evaluatorService evaluation.EvaluatorService,
	executionService evaluation.EvaluatorExecutionService,
	logger *slog.Logger,
	config *EvaluatorWorkerConfig,
) *EvaluatorWorker {
	if config == nil {
		config = &EvaluatorWorkerConfig{
			ConsumerGroup:     "evaluation-evaluator-workers",
			ConsumerID:        "evaluator-worker-" + uid.New().String(),
			BatchSize:         50,
			BlockDuration:     time.Second,
			MaxRetries:        3,
			RetryBackoff:      500 * time.Millisecond,
			DiscoveryInterval: 30 * time.Second,
			MaxStreamsPerRead: 10,
			EvaluatorCacheTTL: evaluatorCacheTTL,
		}
	}

	return &EvaluatorWorker{
		redis:               redis,
		evaluatorService:    evaluatorService,
		executionService:    executionService,
		evaluatorCache:      NewEvaluatorCache(config.EvaluatorCacheTTL),
		logger:              logger,
		consumerGroup:       config.ConsumerGroup,
		consumerID:          config.ConsumerID,
		batchSize:           config.BatchSize,
		blockDuration:       config.BlockDuration,
		maxRetries:          config.MaxRetries,
		retryBackoff:        config.RetryBackoff,
		discoveryInterval:   config.DiscoveryInterval,
		maxStreamsPerRead:   config.MaxStreamsPerRead,
		activeStreams:       make(map[string]bool),
		quit:                make(chan struct{}),
		discoveryBackoff:    time.Second,
		maxDiscoveryBackoff: 30 * time.Second,
	}
}

// Start begins the evaluator worker
func (w *EvaluatorWorker) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&w.running, 0, 1) {
		return errors.New("evaluator worker already running")
	}

	w.logger.Info("Starting evaluator worker",
		"consumer_group", w.consumerGroup,
		"consumer_id", w.consumerID,
		"batch_size", w.batchSize,
		"discovery_interval", w.discoveryInterval,
	)

	// Start consumption loop
	w.wg.Add(1)
	go w.consumeLoop(ctx)

	// Start stream discovery loop
	w.wg.Add(1)
	go w.discoveryLoop(ctx)

	return nil
}

// Stop gracefully stops the evaluator worker
func (w *EvaluatorWorker) Stop() {
	if !atomic.CompareAndSwapInt64(&w.running, 1, 0) {
		return
	}

	w.logger.Info("Stopping evaluator worker")
	close(w.quit)
	w.wg.Wait()

	w.logger.Info("Evaluator worker stopped",
		"spans_processed", atomic.LoadInt64(&w.spansProcessed),
		"evaluators_matched", atomic.LoadInt64(&w.evaluatorsMatched),
		"jobs_emitted", atomic.LoadInt64(&w.jobsEmitted),
		"errors_count", atomic.LoadInt64(&w.errorsCount),
	)
}

// discoveryLoop periodically discovers telemetry streams
func (w *EvaluatorWorker) discoveryLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.discoveryInterval)
	defer ticker.Stop()

	// Initial discovery
	if err := w.performDiscovery(ctx); err != nil {
		w.logger.Error("Initial stream discovery failed", "error", err)
	}

	for {
		select {
		case <-w.quit:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.performDiscovery(ctx); err != nil {
				w.logger.Error("Stream discovery failed", "error", err, "backoff", w.discoveryBackoff)
				time.Sleep(w.discoveryBackoff)
				w.discoveryBackoff = minDuration(w.discoveryBackoff*2, w.maxDiscoveryBackoff)
			} else {
				w.discoveryBackoff = time.Second
			}
		}
	}
}

func (w *EvaluatorWorker) performDiscovery(ctx context.Context) error {
	streams, err := w.discoverStreams(ctx)
	if err != nil {
		return err
	}

	if len(streams) == 0 {
		w.logger.Debug("No telemetry streams discovered")
		return nil
	}

	w.cleanupInactiveStreams(streams)
	return w.ensureConsumerGroups(ctx, streams)
}

func (w *EvaluatorWorker) discoverStreams(ctx context.Context) ([]string, error) {
	var allStreams []string
	cursor := uint64(0)
	pattern := "telemetry:batches:*"

	for {
		keys, nextCursor, err := w.redis.Client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan streams: %w", err)
		}

		allStreams = append(allStreams, keys...)
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	w.logger.Debug("Discovered telemetry streams", "stream_count", len(allStreams))
	return allStreams, nil
}

func (w *EvaluatorWorker) ensureConsumerGroups(ctx context.Context, streamKeys []string) error {
	for _, streamKey := range streamKeys {
		w.streamsMutex.RLock()
		exists := w.activeStreams[streamKey]
		w.streamsMutex.RUnlock()

		if exists {
			continue
		}

		// Create consumer group (use "$" to only read new messages, not historical)
		err := w.redis.Client.XGroupCreateMkStream(ctx, streamKey, w.consumerGroup, "$").Err()
		if err != nil {
			if !strings.Contains(err.Error(), "BUSYGROUP") {
				w.logger.Warn("Failed to create consumer group", "error", err, "stream", streamKey)
				continue
			}
		}

		w.streamsMutex.Lock()
		w.activeStreams[streamKey] = true
		w.streamsMutex.Unlock()

		w.logger.Debug("Consumer group initialized", "stream", streamKey, "consumer_group", w.consumerGroup)
	}

	return nil
}

func (w *EvaluatorWorker) cleanupInactiveStreams(discoveredStreams []string) {
	currentStreams := make(map[string]bool, len(discoveredStreams))
	for _, streamKey := range discoveredStreams {
		currentStreams[streamKey] = true
	}

	w.streamsMutex.Lock()
	defer w.streamsMutex.Unlock()

	var removedStreams []string
	for streamKey := range w.activeStreams {
		if !currentStreams[streamKey] {
			delete(w.activeStreams, streamKey)
			removedStreams = append(removedStreams, streamKey)
		}
	}

	if len(removedStreams) > 0 {
		w.logger.Info("Cleaned up inactive streams", "removed_count", len(removedStreams))
	}
}

// consumeLoop is the main consumption loop
func (w *EvaluatorWorker) consumeLoop(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-w.quit:
			return
		case <-ctx.Done():
			return
		default:
			if err := w.consumeBatch(ctx); err != nil {
				if err != redis.Nil {
					w.logger.Error("Error consuming batch", "error", err)
					atomic.AddInt64(&w.errorsCount, 1)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (w *EvaluatorWorker) consumeBatch(ctx context.Context) error {
	w.streamsMutex.Lock()
	var allStreamKeys []string
	for streamKey := range w.activeStreams {
		allStreamKeys = append(allStreamKeys, streamKey)
	}

	if len(allStreamKeys) > 0 && w.streamRotation >= len(allStreamKeys) {
		w.streamRotation = 0
	}

	if w.streamRotation > 0 && len(allStreamKeys) > w.streamRotation {
		allStreamKeys = append(allStreamKeys[w.streamRotation:], allStreamKeys[:w.streamRotation]...)
	}
	w.streamsMutex.Unlock()

	if len(allStreamKeys) == 0 {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	streamKeys := allStreamKeys
	if len(streamKeys) > w.maxStreamsPerRead {
		streamKeys = streamKeys[:w.maxStreamsPerRead]
	}

	streamArgs := make([]string, 0, len(streamKeys)*2)
	for _, streamKey := range streamKeys {
		streamArgs = append(streamArgs, streamKey)
	}
	for range streamKeys {
		streamArgs = append(streamArgs, ">")
	}

	results, err := w.redis.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerID,
		Streams:  streamArgs,
		Count:    int64(w.batchSize),
		Block:    w.blockDuration,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return err
	}

	for _, stream := range results {
		for _, msg := range stream.Messages {
			if err := w.processMessage(ctx, stream.Stream, msg); err != nil {
				w.logger.Error("Failed to process message", "error", err, "stream", stream.Stream, "message_id", msg.ID)
				atomic.AddInt64(&w.errorsCount, 1)
			}

			// Always acknowledge - evaluator matching is best-effort
			if ackErr := w.redis.Client.XAck(ctx, stream.Stream, w.consumerGroup, msg.ID).Err(); ackErr != nil {
				w.logger.Warn("Failed to acknowledge message", "error", ackErr, "stream", stream.Stream, "message_id", msg.ID)
			}
		}
	}

	w.streamsMutex.Lock()
	w.streamRotation += w.maxStreamsPerRead
	w.streamsMutex.Unlock()

	return nil
}

func (w *EvaluatorWorker) processMessage(ctx context.Context, streamKey string, msg redis.XMessage) error {
	dataStr, ok := msg.Values["data"].(string)
	if !ok {
		return errors.New("invalid message format: missing data field")
	}

	var batch streams.TelemetryStreamMessage
	if err := json.Unmarshal([]byte(dataStr), &batch); err != nil {
		return fmt.Errorf("failed to unmarshal batch data: %w", err)
	}

	// Get active evaluators for this project
	evaluators, err := w.getActiveEvaluators(ctx, batch.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get active evaluators: %w", err)
	}

	if len(evaluators) == 0 {
		return nil
	}

	jobsByEvaluator := make(map[uuid.UUID][]*EvaluationJob)

	for _, event := range batch.Events {
		if event.EventType != string(observability.TelemetryEventTypeSpan) {
			continue
		}

		atomic.AddInt64(&w.spansProcessed, 1)

		// Match span against each evaluator
		for _, evaluator := range evaluators {
			if w.matchEvaluator(evaluator, event) {
				atomic.AddInt64(&w.evaluatorsMatched, 1)

				// Apply sampling rate
				if evaluator.SamplingRate < 1.0 && rand.Float64() > evaluator.SamplingRate {
					continue
				}

				variables := w.extractVariables(evaluator, event)

				// Create evaluation job (execution ID will be set after batch collection)
				job := &EvaluationJob{
					JobID:        uid.New(),
					EvaluatorID:  evaluator.ID,
					ProjectID:    batch.ProjectID,
					SpanData:     event.EventPayload,
					TraceID:      event.TraceID,
					SpanID:       event.SpanID,
					ScorerType:   evaluator.ScorerType,
					ScorerConfig: evaluator.ScorerConfig,
					Variables:    variables,
					CreatedAt:    time.Now(),
				}

				jobsByEvaluator[evaluator.ID] = append(jobsByEvaluator[evaluator.ID], job)
			}
		}
	}

	// Create executions and emit jobs with execution IDs
	for evaluatorID, jobs := range jobsByEvaluator {
		if len(jobs) == 0 {
			continue
		}

		// Create execution record BEFORE emitting jobs to avoid race conditions
		var executionID *uuid.UUID
		if w.executionService != nil {
			execution, err := w.executionService.StartExecutionWithCount(
				ctx,
				evaluatorID,
				batch.ProjectID,
				evaluation.TriggerTypeAutomatic,
				len(jobs),
			)
			if err != nil {
				w.logger.Error("failed to create execution for automatic evaluator",
					"evaluator_id", evaluatorID,
					"project_id", batch.ProjectID,
					"error", err,
				)
				// Continue without execution tracking rather than failing entirely
				// Jobs will still be processed, just without execution record
			} else {
				executionID = &execution.ID
			}
		}

		var enqueueErrors int
		for _, job := range jobs {
			job.ExecutionID = executionID

			if err := w.emitJob(ctx, job); err != nil {
				w.logger.Error("Failed to emit evaluation job",
					"error", err,
					"job_id", job.JobID,
					"evaluator_id", evaluatorID,
					"span_id", job.SpanID,
				)
				enqueueErrors++
				continue
			}

			atomic.AddInt64(&w.jobsEmitted, 1)
			w.logger.Debug("Emitted evaluation job",
				"job_id", job.JobID,
				"evaluator_id", evaluatorID,
				"execution_id", executionID,
				"span_id", job.SpanID,
				"scorer_type", job.ScorerType,
			)
		}

		// If some jobs failed to enqueue, increment errors_count immediately.
		// This ensures spans_scored + errors_count can still reach spans_matched
		// for completion, even when the evaluation worker only processes fewer jobs.
		if enqueueErrors > 0 && executionID != nil && w.executionService != nil {
			if _, err := w.executionService.IncrementAndCheckCompletion(
				ctx, *executionID, batch.ProjectID, 0, enqueueErrors,
			); err != nil {
				w.logger.Error("Failed to increment errors_count for enqueue failures",
					"execution_id", executionID,
					"evaluator_id", evaluatorID,
					"enqueue_errors", enqueueErrors,
					"error", err,
				)
			}
		}

		if executionID != nil {
			w.logger.Debug("Created execution for automatic evaluation",
				"execution_id", executionID,
				"evaluator_id", evaluatorID,
				"project_id", batch.ProjectID,
				"jobs_count", len(jobs),
			)
		}
	}

	return nil
}

func (w *EvaluatorWorker) getActiveEvaluators(ctx context.Context, projectID uuid.UUID) ([]*evaluation.Evaluator, error) {
	// Check cache first
	if evaluators, ok := w.evaluatorCache.Get(projectID.String()); ok {
		return evaluators, nil
	}

	// Fetch from service
	evaluators, err := w.evaluatorService.GetActiveByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Cache the results
	w.evaluatorCache.Set(projectID.String(), evaluators)
	return evaluators, nil
}

// matchEvaluator checks if a span matches an evaluator's filters
func (w *EvaluatorWorker) matchEvaluator(evaluator *evaluation.Evaluator, event streams.TelemetryEventData) bool {
	// Check span name filter
	if len(evaluator.SpanNames) > 0 {
		spanName := safeExtractString(event.EventPayload, "span_name")
		matched := false
		for _, name := range evaluator.SpanNames {
			if name == spanName {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check filter clauses
	for _, clause := range evaluator.Filter {
		if !w.matchFilterClause(clause, event.EventPayload) {
			return false
		}
	}

	return true
}

func (w *EvaluatorWorker) matchFilterClause(clause evaluation.FilterClause, payload map[string]interface{}) bool {
	value := extractFieldValue(payload, clause.Field)

	switch clause.Operator {
	case "equals", "eq":
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", clause.Value)
	case "not_equals", "neq":
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", clause.Value)
	case "contains":
		strValue := fmt.Sprintf("%v", value)
		strClause := fmt.Sprintf("%v", clause.Value)
		return strings.Contains(strValue, strClause)
	case "not_contains":
		strValue := fmt.Sprintf("%v", value)
		strClause := fmt.Sprintf("%v", clause.Value)
		return !strings.Contains(strValue, strClause)
	case "starts_with":
		strValue := fmt.Sprintf("%v", value)
		strClause := fmt.Sprintf("%v", clause.Value)
		return strings.HasPrefix(strValue, strClause)
	case "ends_with":
		strValue := fmt.Sprintf("%v", value)
		strClause := fmt.Sprintf("%v", clause.Value)
		return strings.HasSuffix(strValue, strClause)
	case "regex":
		strValue := fmt.Sprintf("%v", value)
		pattern := fmt.Sprintf("%v", clause.Value)
		matched, _ := regexp.MatchString(pattern, strValue)
		return matched
	case "is_empty":
		return value == nil || fmt.Sprintf("%v", value) == ""
	case "is_not_empty":
		return value != nil && fmt.Sprintf("%v", value) != ""
	case "gt":
		return compareNumeric(value, clause.Value) > 0
	case "gte":
		return compareNumeric(value, clause.Value) >= 0
	case "lt":
		return compareNumeric(value, clause.Value) < 0
	case "lte":
		return compareNumeric(value, clause.Value) <= 0
	default:
		w.logger.Warn("Unknown filter operator", "operator", clause.Operator)
		return true // Unknown operator - skip filter
	}
}

// extractFieldValue extracts a value from the payload using dot notation
// Supports: "input", "output", "span_name", "metadata.key", "span_attributes.key"
func extractFieldValue(payload map[string]interface{}, field string) interface{} {
	parts := strings.Split(field, ".")
	var current interface{} = payload

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		case map[string]string:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}

// extractVariables extracts variables from span based on evaluator's variable mapping
func (w *EvaluatorWorker) extractVariables(evaluator *evaluation.Evaluator, event streams.TelemetryEventData) map[string]string {
	variables := make(map[string]string)

	for _, mapping := range evaluator.VariableMapping {
		var value interface{}

		switch mapping.Source {
		case "span_input":
			value = event.EventPayload["input"]
		case "span_output":
			value = event.EventPayload["output"]
		case "span_metadata":
			if metadata, ok := event.EventPayload["metadata"].(map[string]interface{}); ok {
				if mapping.JSONPath != "" {
					value = extractFieldValue(metadata, mapping.JSONPath)
				} else {
					value = metadata
				}
			}
		case "span_attributes":
			if attrs, ok := event.EventPayload["span_attributes"].(map[string]interface{}); ok {
				if mapping.JSONPath != "" {
					value = extractFieldValue(attrs, mapping.JSONPath)
				} else {
					value = attrs
				}
			}
		case "trace_input":
			// For trace-level input, use span input for now
			value = event.EventPayload["input"]
		default:
			// Direct field access
			if mapping.JSONPath != "" {
				value = extractFieldValue(event.EventPayload, mapping.JSONPath)
			} else {
				value = extractFieldValue(event.EventPayload, mapping.Source)
			}
		}

		if value != nil {
			switch v := value.(type) {
			case string:
				variables[mapping.VariableName] = v
			default:
				// Convert to JSON for complex types
				if jsonBytes, err := json.Marshal(v); err == nil {
					variables[mapping.VariableName] = string(jsonBytes)
				}
			}
		}
	}

	return variables
}

func (w *EvaluatorWorker) emitJob(ctx context.Context, job *EvaluationJob) error {
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	_, err = w.redis.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: evaluationJobsStream,
		Values: map[string]interface{}{
			"job_id":       job.JobID.String(),
			"evaluator_id": job.EvaluatorID.String(),
			"project_id":   job.ProjectID.String(),
			"span_id":      job.SpanID,
			"data":         string(jobData),
			"timestamp":    job.CreatedAt.Unix(),
		},
	}).Result()

	return err
}

// GetStats returns current worker statistics
func (w *EvaluatorWorker) GetStats() map[string]int64 {
	w.streamsMutex.RLock()
	activeStreamCount := int64(len(w.activeStreams))
	w.streamsMutex.RUnlock()

	return map[string]int64{
		"spans_processed":    atomic.LoadInt64(&w.spansProcessed),
		"evaluators_matched": atomic.LoadInt64(&w.evaluatorsMatched),
		"jobs_emitted":       atomic.LoadInt64(&w.jobsEmitted),
		"errors_count":       atomic.LoadInt64(&w.errorsCount),
		"active_streams":     activeStreamCount,
	}
}

// Utility functions

func safeExtractString(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	if val, ok := payload[key].(string); ok {
		return val
	}
	return ""
}

func compareNumeric(a, b interface{}) int {
	aFloat := toFloat64(a)
	bFloat := toFloat64(b)

	if aFloat < bFloat {
		return -1
	}
	if aFloat > bFloat {
		return 1
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
