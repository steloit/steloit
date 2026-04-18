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
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	"brokle/internal/infrastructure/database"
	"brokle/pkg/pagination"
	"brokle/pkg/uid"
)

const (
	manualTriggerStream = "evaluation:manual-triggers"
	defaultSampleLimit  = 1000
	defaultTimeRange    = 24 * time.Hour
)

// ManualTriggerWorkerConfig holds configuration for the manual trigger worker
type ManualTriggerWorkerConfig struct {
	ConsumerGroup  string
	ConsumerID     string
	BlockDuration  time.Duration
	MaxRetries     int
	RetryBackoff   time.Duration
	MaxConcurrency int
	MaxFilterPages int // Max pages to iterate when applying FilterClauses (0 = no limit, default; positive = fail if limit reached without enough matches)
}

// ManualTriggerWorker consumes manual trigger messages and processes historical spans
type ManualTriggerWorker struct {
	redis            *database.RedisDB
	traceService     observability.TraceService
	executionService evaluation.EvaluatorExecutionService
	logger           *slog.Logger

	// Consumer configuration
	consumerGroup  string
	consumerID     string
	blockDuration  time.Duration
	maxRetries     int
	retryBackoff   time.Duration
	maxConcurrency int
	maxFilterPages int // Max pages to iterate when applying FilterClauses

	// State management
	quit    chan struct{}
	running int64

	// Metrics
	triggersProcessed int64
	spansProcessed    int64
	jobsEmitted       int64
	errorsCount       int64
}

// NewManualTriggerWorker creates a new manual trigger worker
func NewManualTriggerWorker(
	redisDB *database.RedisDB,
	traceService observability.TraceService,
	executionService evaluation.EvaluatorExecutionService,
	logger *slog.Logger,
	config *ManualTriggerWorkerConfig,
) *ManualTriggerWorker {
	if config == nil {
		config = &ManualTriggerWorkerConfig{
			ConsumerGroup:  "manual-trigger-workers",
			ConsumerID:     "manual-trigger-" + uid.New().String(),
			BlockDuration:  time.Second,
			MaxRetries:     3,
			RetryBackoff:   500 * time.Millisecond,
			MaxConcurrency: 3,
			MaxFilterPages: 0, // 0 = no limit (correct results); set positive value to cap pages
		}
	}

	return &ManualTriggerWorker{
		redis:            redisDB,
		traceService:     traceService,
		executionService: executionService,
		logger:           logger,
		consumerGroup:    config.ConsumerGroup,
		consumerID:       config.ConsumerID,
		blockDuration:    config.BlockDuration,
		maxRetries:       config.MaxRetries,
		retryBackoff:     config.RetryBackoff,
		maxConcurrency:   config.MaxConcurrency,
		maxFilterPages:   config.MaxFilterPages,
		quit:             make(chan struct{}),
	}
}

// Start begins the manual trigger worker
func (w *ManualTriggerWorker) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&w.running, 0, 1) {
		return errors.New("manual trigger worker already running")
	}

	w.logger.Info("Starting manual trigger worker",
		"consumer_group", w.consumerGroup,
		"consumer_id", w.consumerID,
	)

	if err := w.ensureConsumerGroup(ctx); err != nil {
		return fmt.Errorf("failed to ensure consumer group: %w", err)
	}

	go w.consumeLoop(ctx)

	return nil
}

// Stop gracefully stops the manual trigger worker
func (w *ManualTriggerWorker) Stop() {
	if !atomic.CompareAndSwapInt64(&w.running, 1, 0) {
		return
	}

	w.logger.Info("Stopping manual trigger worker")
	close(w.quit)

	w.logger.Info("Manual trigger worker stopped",
		"triggers_processed", atomic.LoadInt64(&w.triggersProcessed),
		"spans_processed", atomic.LoadInt64(&w.spansProcessed),
		"jobs_emitted", atomic.LoadInt64(&w.jobsEmitted),
		"errors_count", atomic.LoadInt64(&w.errorsCount),
	)
}

func (w *ManualTriggerWorker) ensureConsumerGroup(ctx context.Context) error {
	err := w.redis.Client.XGroupCreateMkStream(ctx, manualTriggerStream, w.consumerGroup, "0").Err()
	if err != nil && !isGroupExistsError(err) {
		return err
	}
	return nil
}

func (w *ManualTriggerWorker) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-w.quit:
			return
		case <-ctx.Done():
			return
		default:
			if err := w.consumeMessages(ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					w.logger.Error("Error consuming messages", "error", err)
					atomic.AddInt64(&w.errorsCount, 1)
				}
				time.Sleep(w.retryBackoff)
			}
		}
	}
}

func (w *ManualTriggerWorker) consumeMessages(ctx context.Context) error {
	streams, err := w.redis.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerID,
		Streams:  []string{manualTriggerStream, ">"},
		Count:    1,
		Block:    w.blockDuration,
	}).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return err
	}

	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := w.processMessage(ctx, message); err != nil {
				w.logger.Error("Failed to process manual trigger message",
					"message_id", message.ID,
					"error", err,
				)
				atomic.AddInt64(&w.errorsCount, 1)
			}

			if err := w.redis.Client.XAck(ctx, manualTriggerStream, w.consumerGroup, message.ID).Err(); err != nil {
				w.logger.Error("Failed to ack message", "message_id", message.ID, "error", err)
			}
		}
	}

	return nil
}

func (w *ManualTriggerWorker) processMessage(ctx context.Context, message redis.XMessage) error {
	dataStr, ok := message.Values["data"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid data field in message")
	}

	var trigger ManualTriggerMessageData
	if err := json.Unmarshal([]byte(dataStr), &trigger); err != nil {
		return fmt.Errorf("failed to unmarshal trigger message: %w", err)
	}

	w.logger.Info("Processing manual trigger",
		"execution_id", trigger.ExecutionID,
		"evaluator_id", trigger.EvaluatorID,
		"project_id", trigger.ProjectID,
		"sample_limit", trigger.SampleLimit,
	)

	spansMatched, jobsEnqueued, enqueueErrors, err := w.processTrigger(ctx, &trigger)

	if err != nil {
		failErr := w.executionService.FailExecution(ctx, trigger.ExecutionID, trigger.ProjectID, err.Error())
		if failErr != nil {
			w.logger.Error("Failed to mark execution as failed", "error", failErr)
		}
		return err
	}

	// Edge case: No spans matched - complete immediately with zeros
	if spansMatched == 0 {
		if err := w.executionService.CompleteExecution(
			ctx, trigger.ExecutionID, trigger.ProjectID, 0, 0, 0,
		); err != nil {
			w.logger.Error("Failed to complete execution with zero spans", "error", err)
			return err
		}
		w.logger.Info("Manual trigger completed with no matching spans",
			"execution_id", trigger.ExecutionID,
			"evaluator_id", trigger.EvaluatorID,
		)
		atomic.AddInt64(&w.triggersProcessed, 1)
		return nil
	}

	// Edge case: Zero jobs after sampling - this is a valid completion, not a failure.
	// Sampling can reduce the set to zero even when spansMatched > 0.
	// Complete with spans_matched = 0 to accurately reflect that zero spans were processed.
	if jobsEnqueued == 0 && enqueueErrors == 0 {
		if err := w.executionService.CompleteExecution(
			ctx, trigger.ExecutionID, trigger.ProjectID, 0, 0, 0,
		); err != nil {
			w.logger.Error("Failed to complete execution with zero jobs after sampling", "error", err)
			return err
		}
		w.logger.Info("Manual trigger completed with zero jobs after sampling",
			"execution_id", trigger.ExecutionID,
			"evaluator_id", trigger.EvaluatorID,
			"pre_sampling_matches", spansMatched,
		)
		atomic.AddInt64(&w.triggersProcessed, 1)
		return nil
	}

	// Edge case: All enqueue attempts failed (there were actual attempts that failed)
	if jobsEnqueued == 0 && enqueueErrors > 0 {
		errMsg := fmt.Sprintf("all %d job enqueue attempts failed", enqueueErrors)
		if failErr := w.executionService.FailExecution(ctx, trigger.ExecutionID, trigger.ProjectID, errMsg); failErr != nil {
			w.logger.Error("Failed to mark execution as failed", "error", failErr)
		}
		return errors.New(errMsg)
	}

	// Normal case: Jobs enqueued successfully
	// spans_matched was already set in processTrigger BEFORE jobs were emitted,
	// which prevents the race condition where evaluation workers could complete
	// the execution prematurely (when spans_matched was 0).
	// IncrementAndCheckCompletion will auto-complete when all jobs finish.

	atomic.AddInt64(&w.triggersProcessed, 1)

	w.logger.Info("Manual trigger jobs enqueued, awaiting processing",
		"execution_id", trigger.ExecutionID,
		"evaluator_id", trigger.EvaluatorID,
		"spans_matched", spansMatched,
		"jobs_enqueued", jobsEnqueued,
		"enqueue_errors", enqueueErrors,
	)

	return nil
}

func (w *ManualTriggerWorker) processTrigger(ctx context.Context, trigger *ManualTriggerMessageData) (spansMatched, spansScored, errorCount int, err error) {
	// ════════════════════════════════════════════════════════════════════
	// PHASE 1: DISCOVERY - Determine which spans to process
	// ════════════════════════════════════════════════════════════════════
	var spans []*observability.Span

	// SHORT-CIRCUIT: If explicit SpanIDs provided, fetch directly by ID
	// This bypasses time range and pagination constraints that would otherwise
	// cause spans outside the 24h window or beyond the page limit to be missed
	if len(trigger.SpanIDs) > 0 {
		spans, err = w.fetchSpansByIDs(ctx, trigger)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to fetch spans by ID: %w", err)
		}
	} else {
		// Normal flow: fetch with pagination until we have enough matches
		// SpanNames are pushed to database level for efficiency
		// FilterClauses are applied in-memory with pagination loop
		spans, err = w.fetchSpansWithFilters(ctx, trigger)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to fetch spans: %w", err)
		}
	}

	spansMatched = len(spans)

	if spansMatched == 0 {
		w.logger.Info("No spans matched for manual trigger",
			"execution_id", trigger.ExecutionID,
			"evaluator_id", trigger.EvaluatorID,
		)
		return 0, 0, 0, nil
	}

	// Apply sampling if configured
	if trigger.SamplingRate > 0 && trigger.SamplingRate < 1.0 {
		spans = applySampling(spans, trigger.SamplingRate, trigger.SampleLimit)
	} else if len(spans) > trigger.SampleLimit {
		spans = spans[:trigger.SampleLimit]
	}

	// Calculate final job count BEFORE enqueueing
	jobCount := len(spans)
	if jobCount == 0 {
		// Sampling reduced to zero - this is valid, not an error
		// Return early before UpdateSpansMatched to avoid setting a target of 0
		return spansMatched, 0, 0, nil
	}

	// ════════════════════════════════════════════════════════════════════
	// PHASE 2: SET TARGET - Update spans_matched BEFORE any jobs are enqueued
	// ════════════════════════════════════════════════════════════════════
	// This prevents the race condition where evaluation workers pick up jobs
	// and call IncrementAndCheckCompletion before spans_matched is set,
	// causing premature completion (since 1 + 0 >= 0 is true).
	if err := w.executionService.UpdateSpansMatched(
		ctx, trigger.ExecutionID, trigger.ProjectID, jobCount,
	); err != nil {
		// Critical: If we can't set the target, don't enqueue jobs
		// Otherwise we'll have the same race condition
		return 0, 0, 0, fmt.Errorf("failed to set spans_matched before enqueueing: %w", err)
	}

	// ════════════════════════════════════════════════════════════════════
	// PHASE 3: ENQUEUE - Now safe to emit jobs
	// ════════════════════════════════════════════════════════════════════
	for _, span := range spans {
		job := w.createEvaluationJob(trigger, span)
		if err := w.emitJob(ctx, job); err != nil {
			w.logger.Error("Failed to emit evaluation job",
				"span_id", span.SpanID,
				"error", err,
			)
			errorCount++
			continue
		}
		spansScored++
		atomic.AddInt64(&w.jobsEmitted, 1)
	}

	// If some jobs failed to enqueue, increment errors_count immediately.
	// This ensures spans_scored + errors_count can still reach spans_matched
	// for completion, even when the evaluation worker only processes spansScored jobs.
	// Without this, partial enqueue failures would leave executions stuck running.
	if errorCount > 0 {
		if _, err := w.executionService.IncrementAndCheckCompletion(
			ctx, trigger.ExecutionID, trigger.ProjectID, 0, errorCount,
		); err != nil {
			w.logger.Error("Failed to increment errors_count for enqueue failures",
				"execution_id", trigger.ExecutionID,
				"enqueue_errors", errorCount,
				"error", err,
			)
			// Continue - execution may hang but jobs are still being processed
		}
	}

	atomic.AddInt64(&w.spansProcessed, int64(spansMatched))

	return spansMatched, spansScored, errorCount, nil
}

func (w *ManualTriggerWorker) buildSpanFilter(trigger *ManualTriggerMessageData) *observability.SpanFilter {
	filter := &observability.SpanFilter{
		ProjectID: trigger.ProjectID,
		SpanNames: trigger.SpanNames, // Push to database level for efficient filtering
		Params: pagination.Params{
			Page:  1,
			Limit: trigger.SampleLimit,
		},
	}

	// Apply time range
	if trigger.TimeRangeStart != nil {
		filter.StartTime = trigger.TimeRangeStart
	} else {
		// Default to last 24 hours
		defaultStart := time.Now().Add(-defaultTimeRange)
		filter.StartTime = &defaultStart
	}

	if trigger.TimeRangeEnd != nil {
		filter.EndTime = trigger.TimeRangeEnd
	}

	return filter
}

// fetchSpansWithFilters fetches spans across pages, applying FilterClauses until
// we have enough matches or exhaust available data. SpanNames are pushed to the
// database query for efficient filtering.
//
// If maxFilterPages is reached before finding enough matches AND more data may exist,
// returns an error to prevent incorrect execution results.
func (w *ManualTriggerWorker) fetchSpansWithFilters(
	ctx context.Context,
	trigger *ManualTriggerMessageData,
) ([]*observability.Span, error) {
	// If no FilterClauses, single page is sufficient (SpanNames already in DB query)
	if len(trigger.Filter) == 0 {
		filter := w.buildSpanFilter(trigger)
		return w.traceService.GetSpansByFilter(ctx, filter)
	}

	// With FilterClauses: iterate pages until we have enough filtered matches
	var matchedSpans []*observability.Span
	targetCount := trigger.SampleLimit

	for page := 1; w.maxFilterPages == 0 || page <= w.maxFilterPages; page++ {
		filter := w.buildSpanFilterForPage(trigger, page)

		pageSpans, err := w.traceService.GetSpansByFilter(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to query page %d: %w", page, err)
		}

		// No more results - we've exhausted the data
		if len(pageSpans) == 0 {
			break
		}

		// Apply FilterClauses to this page
		for _, span := range pageSpans {
			if w.matchSpanFilters(span, trigger.Filter) {
				matchedSpans = append(matchedSpans, span)
				if len(matchedSpans) >= targetCount {
					return matchedSpans, nil
				}
			}
		}

		// Log progress for debugging sparse match scenarios
		if page > 1 && page%10 == 0 {
			w.logger.Debug("Pagination in progress",
				"page", page,
				"matched_so_far", len(matchedSpans),
				"target", targetCount,
				"page_size", len(pageSpans),
			)
		}

		// Fail if we hit the cap and haven't found enough matches
		// (only when cap is configured, i.e., maxFilterPages > 0)
		if w.maxFilterPages > 0 && page == w.maxFilterPages && len(matchedSpans) < targetCount {
			return nil, fmt.Errorf(
				"pagination limit reached: scanned %d pages but only found %d/%d matching spans; "+
					"consider using more specific SpanNames, narrowing the time range, or increasing MaxFilterPages",
				w.maxFilterPages, len(matchedSpans), targetCount,
			)
		}
	}

	return matchedSpans, nil
}

// buildSpanFilterForPage creates a filter for a specific page during pagination
func (w *ManualTriggerWorker) buildSpanFilterForPage(trigger *ManualTriggerMessageData, page int) *observability.SpanFilter {
	filter := &observability.SpanFilter{
		ProjectID: trigger.ProjectID,
		SpanNames: trigger.SpanNames, // Push to database level
		Params: pagination.Params{
			Page:  page,
			Limit: trigger.SampleLimit, // Fetch full pages
		},
	}

	if trigger.TimeRangeStart != nil {
		filter.StartTime = trigger.TimeRangeStart
	} else {
		defaultStart := time.Now().Add(-defaultTimeRange)
		filter.StartTime = &defaultStart
	}

	if trigger.TimeRangeEnd != nil {
		filter.EndTime = trigger.TimeRangeEnd
	}

	return filter
}

// filterSpans applies evaluator filters and specific span IDs to candidate spans.
// Note: SpanNames are now handled at database level, not here.
// Filtering order is optimized for performance: SpanIDs (hash lookup) → FilterClauses (most expensive).
func (w *ManualTriggerWorker) filterSpans(
	spans []*observability.Span,
	trigger *ManualTriggerMessageData,
) []*observability.Span {
	// Fast path: no filters specified
	// Note: SpanNames excluded - now handled at database level
	if len(trigger.SpanIDs) == 0 && len(trigger.Filter) == 0 {
		return spans
	}

	// Filter by specific span IDs first (most selective, fast hash lookup)
	if len(trigger.SpanIDs) > 0 {
		spanIDSet := make(map[string]bool, len(trigger.SpanIDs))
		for _, id := range trigger.SpanIDs {
			spanIDSet[id] = true
		}

		filtered := make([]*observability.Span, 0, len(trigger.SpanIDs))
		for _, span := range spans {
			if spanIDSet[span.SpanID] {
				filtered = append(filtered, span)
			}
		}
		spans = filtered
	}

	// SpanNames filtering REMOVED - now handled at database level via SpanFilter.SpanNames

	// Apply filter clauses (most expensive - string operations, regex)
	if len(trigger.Filter) > 0 {
		filtered := make([]*observability.Span, 0, len(spans))
		for _, span := range spans {
			if w.matchSpanFilters(span, trigger.Filter) {
				filtered = append(filtered, span)
			}
		}
		spans = filtered
	}

	return spans
}

// fetchSpansByIDs fetches spans directly by their IDs, bypassing time range and pagination.
// Uses TraceService.GetSpanByProject() for each ID with project validation.
func (w *ManualTriggerWorker) fetchSpansByIDs(
	ctx context.Context,
	trigger *ManualTriggerMessageData,
) ([]*observability.Span, error) {
	projectID := trigger.ProjectID
	spans := make([]*observability.Span, 0, len(trigger.SpanIDs))

	for _, spanID := range trigger.SpanIDs {
		span, err := w.traceService.GetSpanByProject(ctx, spanID, projectID)
		if err != nil {
			// Log warning but continue - span may have been deleted or belongs to different project
			w.logger.Warn("Failed to fetch span by ID",
				"span_id", spanID,
				"project_id", projectID,
				"error", err,
			)
			continue
		}
		if span != nil {
			spans = append(spans, span)
		}
	}

	// Apply SpanNames and FilterClauses (but skip SpanIDs filtering - already done by direct fetch)
	if len(trigger.SpanNames) > 0 || len(trigger.Filter) > 0 {
		spans = w.filterSpansExcludingIDs(spans, trigger)
	}

	return spans, nil
}

// filterSpansExcludingIDs applies SpanNames and FilterClauses but skips SpanID filtering
// (used when spans were already fetched by explicit IDs)
func (w *ManualTriggerWorker) filterSpansExcludingIDs(
	spans []*observability.Span,
	trigger *ManualTriggerMessageData,
) []*observability.Span {
	// Filter by span names
	if len(trigger.SpanNames) > 0 {
		nameSet := make(map[string]bool, len(trigger.SpanNames))
		for _, name := range trigger.SpanNames {
			nameSet[name] = true
		}

		filtered := make([]*observability.Span, 0, len(spans))
		for _, span := range spans {
			if nameSet[span.SpanName] {
				filtered = append(filtered, span)
			}
		}
		spans = filtered
	}

	// Apply filter clauses
	if len(trigger.Filter) > 0 {
		filtered := make([]*observability.Span, 0, len(spans))
		for _, span := range spans {
			if w.matchSpanFilters(span, trigger.Filter) {
				filtered = append(filtered, span)
			}
		}
		spans = filtered
	}

	return spans
}

// matchSpanFilters checks if a span matches all filter clauses (AND logic)
func (w *ManualTriggerWorker) matchSpanFilters(span *observability.Span, filters []evaluation.FilterClause) bool {
	for _, clause := range filters {
		if !w.matchFilterClause(clause, span) {
			return false
		}
	}
	return true
}

// matchFilterClause checks if a span matches a single filter clause
func (w *ManualTriggerWorker) matchFilterClause(clause evaluation.FilterClause, span *observability.Span) bool {
	value := w.extractSpanFieldValue(span, clause.Field)

	switch clause.Operator {
	case "equals", "eq":
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", clause.Value)
	case "not_equals", "neq":
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", clause.Value)
	case "contains":
		return strings.Contains(fmt.Sprintf("%v", value), fmt.Sprintf("%v", clause.Value))
	case "not_contains":
		return !strings.Contains(fmt.Sprintf("%v", value), fmt.Sprintf("%v", clause.Value))
	case "starts_with":
		return strings.HasPrefix(fmt.Sprintf("%v", value), fmt.Sprintf("%v", clause.Value))
	case "ends_with":
		return strings.HasSuffix(fmt.Sprintf("%v", value), fmt.Sprintf("%v", clause.Value))
	case "regex":
		matched, err := regexp.MatchString(fmt.Sprintf("%v", clause.Value), fmt.Sprintf("%v", value))
		if err != nil {
			w.logger.Warn("Invalid regex in filter clause", "pattern", clause.Value, "error", err)
			return true // Graceful fallback: treat as match
		}
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
		return true // Graceful fallback: treat as match
	}
}

// extractSpanFieldValue extracts a value from a span using dot notation for nested paths
func (w *ManualTriggerWorker) extractSpanFieldValue(span *observability.Span, field string) interface{} {
	parts := strings.Split(field, ".")

	// Handle top-level span fields
	switch parts[0] {
	case "input":
		if len(parts) > 1 && span.Input != nil {
			return extractNestedValue(*span.Input, parts[1:])
		}
		if span.Input != nil {
			return *span.Input
		}
		return nil
	case "output":
		if len(parts) > 1 && span.Output != nil {
			return extractNestedValue(*span.Output, parts[1:])
		}
		if span.Output != nil {
			return *span.Output
		}
		return nil
	case "span_name", "name":
		return span.SpanName
	case "span_kind":
		return span.SpanKind
	case "model", "model_name":
		if span.ModelName != nil {
			return *span.ModelName
		}
		return nil
	case "provider", "provider_name":
		if span.ProviderName != nil {
			return *span.ProviderName
		}
		return nil
	case "span_attributes":
		if len(parts) > 1 && span.SpanAttributes != nil {
			return span.SpanAttributes[parts[1]]
		}
		return span.SpanAttributes
	case "resource_attributes":
		if len(parts) > 1 && span.ResourceAttributes != nil {
			return span.ResourceAttributes[parts[1]]
		}
		return span.ResourceAttributes
	case "service_name":
		if span.ServiceName != nil {
			return *span.ServiceName
		}
		return nil
	}

	return nil
}

// extractNestedValue extracts nested value from JSON string or interface using path parts
func extractNestedValue(data interface{}, path []string) interface{} {
	if len(path) == 0 {
		return data
	}

	// Handle string JSON
	if str, ok := data.(string); ok {
		var parsed map[string]interface{}
		if json.Unmarshal([]byte(str), &parsed) == nil {
			data = parsed
		} else {
			return nil // Not valid JSON
		}
	}

	// Navigate path
	if m, ok := data.(map[string]interface{}); ok {
		if val, exists := m[path[0]]; exists {
			return extractNestedValue(val, path[1:])
		}
	}

	return nil
}

func (w *ManualTriggerWorker) createEvaluationJob(trigger *ManualTriggerMessageData, span *observability.Span) *EvaluationJob {
	// Build span data map from span
	spanData := make(map[string]interface{})
	spanData["input"] = span.Input
	spanData["output"] = span.Output
	spanData["span_attributes"] = span.SpanAttributes
	spanData["span_kind"] = span.SpanKind
	spanData["name"] = span.SpanName
	// Model and provider may be in span attributes
	if span.SpanAttributes != nil {
		if model, ok := span.SpanAttributes["gen_ai.response.model"]; ok {
			spanData["model"] = model
		}
		if provider, ok := span.SpanAttributes["gen_ai.system"]; ok {
			spanData["provider"] = provider
		}
	}

	// Extract variables based on mapping
	variables := extractVariablesFromSpan(trigger.VariableMapping, span)

	return &EvaluationJob{
		JobID:        uid.New(),
		EvaluatorID:  trigger.EvaluatorID,
		ProjectID:    trigger.ProjectID,
		ExecutionID:  &trigger.ExecutionID,
		SpanData:     spanData,
		TraceID:      span.TraceID,
		SpanID:       span.SpanID,
		ScorerType:   trigger.ScorerType,
		ScorerConfig: trigger.ScorerConfig,
		Variables:    variables,
		CreatedAt:    time.Now(),
	}
}

func (w *ManualTriggerWorker) emitJob(ctx context.Context, job *EvaluationJob) error {
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
func (w *ManualTriggerWorker) GetStats() map[string]int64 {
	return map[string]int64{
		"triggers_processed": atomic.LoadInt64(&w.triggersProcessed),
		"spans_processed":    atomic.LoadInt64(&w.spansProcessed),
		"jobs_emitted":       atomic.LoadInt64(&w.jobsEmitted),
		"errors_count":       atomic.LoadInt64(&w.errorsCount),
	}
}

// ManualTriggerMessageData is the internal struct for parsing trigger messages
type ManualTriggerMessageData struct {
	ExecutionID     uuid.UUID                 `json:"execution_id"`
	EvaluatorID     uuid.UUID                 `json:"evaluator_id"`
	ProjectID       uuid.UUID                 `json:"project_id"`
	ScorerType      evaluation.ScorerType     `json:"scorer_type"`
	ScorerConfig    map[string]any            `json:"scorer_config"`
	TargetScope     evaluation.TargetScope    `json:"target_scope"`
	Filter          []evaluation.FilterClause `json:"filter,omitempty"`
	SpanNames       []string                  `json:"span_names,omitempty"`
	SamplingRate    float64                   `json:"sampling_rate"`
	VariableMapping []evaluation.VariableMap  `json:"variable_mapping,omitempty"`
	TimeRangeStart  *time.Time                `json:"time_range_start,omitempty"`
	TimeRangeEnd    *time.Time                `json:"time_range_end,omitempty"`
	SpanIDs         []string                  `json:"span_ids,omitempty"`
	SampleLimit     int                       `json:"sample_limit"`
	CreatedAt       time.Time                 `json:"created_at"`
}

// Helper functions

func applySampling(spans []*observability.Span, rate float64, limit int) []*observability.Span {
	sampled := make([]*observability.Span, 0, limit)
	for _, span := range spans {
		if rand.Float64() < rate {
			sampled = append(sampled, span)
			if len(sampled) >= limit {
				break
			}
		}
	}
	return sampled
}

func extractVariablesFromSpan(mapping []evaluation.VariableMap, span *observability.Span) map[string]string {
	variables := make(map[string]string)

	for _, m := range mapping {
		var value interface{}

		switch m.Source {
		case "span_input":
			if span.Input != nil {
				value = *span.Input
			}
		case "span_output":
			if span.Output != nil {
				value = *span.Output
			}
		case "span_metadata", "span_attributes":
			if m.JSONPath != "" {
				value = extractFromJSON(span.SpanAttributes, m.JSONPath)
			} else {
				value = span.SpanAttributes
			}
		default:
			// Try to extract from span attributes by default
			if m.JSONPath != "" {
				value = extractFromJSON(span.SpanAttributes, m.JSONPath)
			}
		}

		if value != nil {
			switch v := value.(type) {
			case string:
				variables[m.VariableName] = v
			case *string:
				if v != nil {
					variables[m.VariableName] = *v
				}
			default:
				if jsonBytes, err := json.Marshal(v); err == nil {
					variables[m.VariableName] = string(jsonBytes)
				}
			}
		}
	}

	return variables
}

func extractFromJSON(data interface{}, path string) interface{} {
	if data == nil || path == "" {
		return data
	}

	// Handle JSON bytes
	if bytes, ok := data.([]byte); ok {
		var parsed interface{}
		if err := json.Unmarshal(bytes, &parsed); err != nil {
			return nil
		}
		data = parsed
	}

	// Handle JSON string
	if str, ok := data.(string); ok {
		var parsed interface{}
		if err := json.Unmarshal([]byte(str), &parsed); err != nil {
			return str // Return original string if not JSON
		}
		data = parsed
	}

	// Simple path extraction (supports dot notation)
	// For complex JSONPath, would need a library
	if m, ok := data.(map[string]interface{}); ok {
		if val, exists := m[path]; exists {
			return val
		}
	}

	return nil
}

func isGroupExistsError(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}
