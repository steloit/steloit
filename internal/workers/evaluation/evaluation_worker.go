package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	"brokle/internal/infrastructure/database"
	"brokle/pkg/uid"
)

// ScorerResult represents the output of a scorer execution
type ScorerResult struct {
	Scores []ScoreOutput `json:"scores"`
	Error  *string       `json:"error,omitempty"`
}

// ScoreOutput represents a single score from a scorer
type ScoreOutput struct {
	Name        string   `json:"name"`
	Value       *float64 `json:"value,omitempty"`
	StringValue *string  `json:"string_value,omitempty"`
	Type        string   `json:"type"` // NUMERIC, CATEGORICAL, BOOLEAN
	Reason      *string  `json:"reason,omitempty"`
}

// Scorer is the interface for all scorer implementations
type Scorer interface {
	Execute(ctx context.Context, job *EvaluationJob) (*ScorerResult, error)
	Type() evaluation.ScorerType
}

// EvaluationWorkerConfig holds configuration for the evaluation worker
type EvaluationWorkerConfig struct {
	ConsumerGroup  string
	ConsumerID     string
	BatchSize      int
	BlockDuration  time.Duration
	MaxRetries     int
	RetryBackoff   time.Duration
	MaxConcurrency int
}

// EvaluationWorker consumes evaluation jobs and executes scorers
type EvaluationWorker struct {
	redis            *database.RedisDB
	scoreService     observability.ScoreService
	executionService evaluation.EvaluatorExecutionService
	llmScorer        Scorer
	builtinScorer    Scorer
	regexScorer      Scorer
	logger           *slog.Logger

	// Consumer configuration
	consumerGroup  string
	consumerID     string
	batchSize      int
	blockDuration  time.Duration
	maxRetries     int
	retryBackoff   time.Duration
	maxConcurrency int

	// State management
	quit    chan struct{}
	wg      sync.WaitGroup
	running int64

	// Execution tracking (for manual triggers)
	executionStats   map[string]*executionProgress // keyed by execution_id
	executionStatsMu sync.RWMutex

	// Metrics
	jobsProcessed int64
	scoresCreated int64
	errorsCount   int64
	llmCalls      int64
	builtinCalls  int64
	regexCalls    int64
}

// executionProgress tracks progress for a single evaluator execution
type executionProgress struct {
	executionID  string
	projectID    uuid.UUID
	spansScored  int64
	errorsCount  int64
	lastActivity time.Time
}

// NewEvaluationWorker creates a new evaluation worker
func NewEvaluationWorker(
	redis *database.RedisDB,
	scoreService observability.ScoreService,
	executionService evaluation.EvaluatorExecutionService,
	llmScorer Scorer,
	builtinScorer Scorer,
	regexScorer Scorer,
	logger *slog.Logger,
	config *EvaluationWorkerConfig,
) *EvaluationWorker {
	if config == nil {
		config = &EvaluationWorkerConfig{
			ConsumerGroup:  "evaluation-execution-workers",
			ConsumerID:     "eval-worker-" + uid.New().String(),
			BatchSize:      10,
			BlockDuration:  time.Second,
			MaxRetries:     3,
			RetryBackoff:   500 * time.Millisecond,
			MaxConcurrency: 5,
		}
	}

	return &EvaluationWorker{
		redis:            redis,
		scoreService:     scoreService,
		executionService: executionService,
		llmScorer:        llmScorer,
		builtinScorer:    builtinScorer,
		regexScorer:      regexScorer,
		logger:           logger,
		consumerGroup:    config.ConsumerGroup,
		consumerID:       config.ConsumerID,
		batchSize:        config.BatchSize,
		blockDuration:    config.BlockDuration,
		maxRetries:       config.MaxRetries,
		retryBackoff:     config.RetryBackoff,
		maxConcurrency:   config.MaxConcurrency,
		quit:             make(chan struct{}),
		executionStats:   make(map[string]*executionProgress),
	}
}

// Start begins the evaluation worker
func (w *EvaluationWorker) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&w.running, 0, 1) {
		return errors.New("evaluation worker already running")
	}

	w.logger.Info("Starting evaluation worker",
		"consumer_group", w.consumerGroup,
		"consumer_id", w.consumerID,
		"batch_size", w.batchSize,
		"max_concurrency", w.maxConcurrency,
	)

	if err := w.ensureConsumerGroup(ctx); err != nil {
		return fmt.Errorf("failed to ensure consumer group: %w", err)
	}

	w.wg.Add(1)
	go w.consumeLoop(ctx)

	return nil
}

// Stop gracefully stops the evaluation worker
func (w *EvaluationWorker) Stop() {
	if !atomic.CompareAndSwapInt64(&w.running, 1, 0) {
		return
	}

	w.logger.Info("Stopping evaluation worker")
	close(w.quit)
	w.wg.Wait()

	w.logger.Info("Evaluation worker stopped",
		"jobs_processed", atomic.LoadInt64(&w.jobsProcessed),
		"scores_created", atomic.LoadInt64(&w.scoresCreated),
		"errors_count", atomic.LoadInt64(&w.errorsCount),
		"llm_calls", atomic.LoadInt64(&w.llmCalls),
		"builtin_calls", atomic.LoadInt64(&w.builtinCalls),
		"regex_calls", atomic.LoadInt64(&w.regexCalls),
	)
}

func (w *EvaluationWorker) ensureConsumerGroup(ctx context.Context) error {
	err := w.redis.Client.XGroupCreateMkStream(ctx, evaluationJobsStream, w.consumerGroup, "0").Err()
	if err != nil {
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			return err
		}
	}
	return nil
}

func (w *EvaluationWorker) consumeLoop(ctx context.Context) {
	defer w.wg.Done()

	// Semaphore for concurrency control
	sem := make(chan struct{}, w.maxConcurrency)

	for {
		select {
		case <-w.quit:
			return
		case <-ctx.Done():
			return
		default:
			if err := w.consumeBatch(ctx, sem); err != nil {
				if err != redis.Nil {
					w.logger.Error("Error consuming batch", "error", err)
					atomic.AddInt64(&w.errorsCount, 1)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (w *EvaluationWorker) consumeBatch(ctx context.Context, sem chan struct{}) error {
	results, err := w.redis.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerID,
		Streams:  []string{evaluationJobsStream, ">"},
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
			// Acquire semaphore slot
			select {
			case sem <- struct{}{}:
			case <-w.quit:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}

			// Process in goroutine for concurrency
			go func(msg redis.XMessage) {
				defer func() { <-sem }() // Release slot

				if err := w.processJob(ctx, msg); err != nil {
					w.logger.Error("Failed to process job",
						"error", err,
						"message_id", msg.ID,
					)
					atomic.AddInt64(&w.errorsCount, 1)
				}

				// Always acknowledge - evaluation is best-effort
				if ackErr := w.redis.Client.XAck(ctx, evaluationJobsStream, w.consumerGroup, msg.ID).Err(); ackErr != nil {
					w.logger.Warn("Failed to acknowledge message",
						"error", ackErr,
						"message_id", msg.ID,
					)
				}
			}(msg)
		}
	}

	return nil
}

func (w *EvaluationWorker) processJob(ctx context.Context, msg redis.XMessage) error {
	dataStr, ok := msg.Values["data"].(string)
	if !ok {
		return errors.New("invalid message format: missing data field")
	}

	var job EvaluationJob
	if err := json.Unmarshal([]byte(dataStr), &job); err != nil {
		return fmt.Errorf("failed to unmarshal job data: %w", err)
	}

	atomic.AddInt64(&w.jobsProcessed, 1)

	// Select appropriate scorer
	var scorer Scorer
	switch job.ScorerType {
	case evaluation.ScorerTypeLLM:
		scorer = w.llmScorer
		atomic.AddInt64(&w.llmCalls, 1)
	case evaluation.ScorerTypeBuiltin:
		scorer = w.builtinScorer
		atomic.AddInt64(&w.builtinCalls, 1)
	case evaluation.ScorerTypeRegex:
		scorer = w.regexScorer
		atomic.AddInt64(&w.regexCalls, 1)
	default:
		w.trackExecutionError(ctx, &job)
		return fmt.Errorf("unknown scorer type: %s", job.ScorerType)
	}

	if scorer == nil {
		w.trackExecutionError(ctx, &job)
		return fmt.Errorf("scorer not configured for type: %s", job.ScorerType)
	}

	// Execute scorer with retry
	var result *ScorerResult
	var lastErr error
	for attempt := 0; attempt <= w.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * w.retryBackoff)
		}

		result, lastErr = scorer.Execute(ctx, &job)
		if lastErr == nil {
			break
		}

		w.logger.Warn("Scorer execution failed, retrying",
			"error", lastErr,
			"job_id", job.JobID,
			"attempt", attempt+1,
		)
	}

	if lastErr != nil {
		w.trackExecutionError(ctx, &job)
		return fmt.Errorf("scorer execution failed after retries: %w", lastErr)
	}

	if result == nil || len(result.Scores) == 0 {
		w.logger.Debug("Scorer returned no scores",
			"job_id", job.JobID,
			"evaluator_id", job.EvaluatorID,
		)
		// Still track as successful scoring (just no output)
		w.trackExecutionSuccess(ctx, &job)
		return nil
	}

	scores := make([]*observability.Score, 0, len(result.Scores))
	for _, output := range result.Scores {
		score := &observability.Score{
			ID:          uid.New(),
			ProjectID:   job.ProjectID,
			TraceID:     &job.TraceID,
			SpanID:      &job.SpanID,
			Name:        output.Name,
			Value:       output.Value,
			StringValue: output.StringValue,
			Type:        output.Type,
			Source:      observability.ScoreSourceEval,
			Reason:      output.Reason,
			Metadata:    w.buildScoreMetadata(job),
			Timestamp:   time.Now(),
		}
		scores = append(scores, score)
	}

	if err := w.scoreService.CreateScoreBatch(ctx, scores); err != nil {
		w.trackExecutionError(ctx, &job)
		return fmt.Errorf("failed to create scores: %w", err)
	}

	atomic.AddInt64(&w.scoresCreated, int64(len(scores)))

	// Track successful scoring for execution
	w.trackExecutionSuccess(ctx, &job)

	w.logger.Debug("Created scores from evaluation",
		"job_id", job.JobID,
		"evaluator_id", job.EvaluatorID,
		"score_count", len(scores),
		"scorer_type", job.ScorerType,
	)

	return nil
}

// trackExecutionSuccess atomically increments spans_scored and checks for completion
func (w *EvaluationWorker) trackExecutionSuccess(ctx context.Context, job *EvaluationJob) {
	if job.ExecutionID == nil || w.executionService == nil {
		return
	}

	completed, err := w.executionService.IncrementAndCheckCompletion(
		ctx,
		*job.ExecutionID,
		job.ProjectID,
		1, // spansScored
		0, // errorsCount
	)
	if err != nil {
		w.logger.Error("failed to track execution success",
			"execution_id", job.ExecutionID,
			"job_id", job.JobID,
			"error", err,
		)
		return
	}

	if completed {
		w.logger.Info("execution auto-completed",
			"execution_id", job.ExecutionID,
			"evaluator_id", job.EvaluatorID,
			"project_id", job.ProjectID,
		)
	}
}

// trackExecutionError atomically increments errors_count and checks for completion
func (w *EvaluationWorker) trackExecutionError(ctx context.Context, job *EvaluationJob) {
	if job.ExecutionID == nil || w.executionService == nil {
		return
	}

	completed, err := w.executionService.IncrementAndCheckCompletion(
		ctx,
		*job.ExecutionID,
		job.ProjectID,
		0, // spansScored
		1, // errorsCount
	)
	if err != nil {
		w.logger.Error("failed to track execution error",
			"execution_id", job.ExecutionID,
			"job_id", job.JobID,
			"error", err,
		)
		return
	}

	if completed {
		w.logger.Info("execution auto-completed with errors",
			"execution_id", job.ExecutionID,
			"evaluator_id", job.EvaluatorID,
			"project_id", job.ProjectID,
		)
	}
}

// FlushExecutionStats persists accumulated execution stats to the database.
// NOTE: With atomic tracking via IncrementAndCheckCompletion, this is now mostly
// a no-op for automatic evaluations. Stats are updated atomically per-job.
// This remains for backward compatibility and cleanup of any orphaned in-memory stats.
func (w *EvaluationWorker) FlushExecutionStats(ctx context.Context) {
	if w.executionService == nil {
		return
	}

	w.executionStatsMu.Lock()
	statsToFlush := make(map[string]*executionProgress)
	for id, progress := range w.executionStats {
		statsToFlush[id] = progress
	}
	w.executionStats = make(map[string]*executionProgress)
	w.executionStatsMu.Unlock()

	for execID, progress := range statsToFlush {
		if err := w.executionService.IncrementCounters(
			ctx,
			execID,
			progress.projectID,
			int(progress.spansScored),
			int(progress.errorsCount),
		); err != nil {
			w.logger.Error("Failed to flush execution stats",
				"execution_id", execID,
				"project_id", progress.projectID,
				"spans_scored", progress.spansScored,
				"errors_count", progress.errorsCount,
				"error", err,
			)
			// Re-add to stats on failure
			w.executionStatsMu.Lock()
			existing, exists := w.executionStats[execID]
			if exists {
				existing.spansScored += progress.spansScored
				existing.errorsCount += progress.errorsCount
			} else {
				w.executionStats[execID] = progress
			}
			w.executionStatsMu.Unlock()
		}
	}
}

func (w *EvaluationWorker) buildScoreMetadata(job EvaluationJob) json.RawMessage {
	metadata := map[string]any{
		"evaluator_id": job.EvaluatorID.String(),
		"scorer_type":  string(job.ScorerType),
		"job_id":       job.JobID.String(),
	}
	data, _ := json.Marshal(metadata)
	return data
}

// GetStats returns current worker statistics
func (w *EvaluationWorker) GetStats() map[string]int64 {
	return map[string]int64{
		"jobs_processed": atomic.LoadInt64(&w.jobsProcessed),
		"scores_created": atomic.LoadInt64(&w.scoresCreated),
		"errors_count":   atomic.LoadInt64(&w.errorsCount),
		"llm_calls":      atomic.LoadInt64(&w.llmCalls),
		"builtin_calls":  atomic.LoadInt64(&w.builtinCalls),
		"regex_calls":    atomic.LoadInt64(&w.regexCalls),
	}
}
