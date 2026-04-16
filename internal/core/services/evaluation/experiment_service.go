package evaluation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
)

type experimentService struct {
	repo        evaluation.ExperimentRepository
	datasetRepo evaluation.DatasetRepository
	scoreRepo   observability.ScoreRepository
	logger      *slog.Logger
}

func NewExperimentService(
	repo evaluation.ExperimentRepository,
	datasetRepo evaluation.DatasetRepository,
	scoreRepo observability.ScoreRepository,
	logger *slog.Logger,
) evaluation.ExperimentService {
	return &experimentService{
		repo:        repo,
		datasetRepo: datasetRepo,
		scoreRepo:   scoreRepo,
		logger:      logger,
	}
}

func (s *experimentService) Create(ctx context.Context, projectID uuid.UUID, req *evaluation.CreateExperimentRequest) (*evaluation.Experiment, error) {
	experiment := evaluation.NewExperiment(projectID, req.Name)
	experiment.Description = req.Description
	if req.Metadata != nil {
		experiment.Metadata = req.Metadata
	}

	if req.DatasetID != nil {
		datasetID, err := uuid.Parse(*req.DatasetID)
		if err != nil {
			return nil, appErrors.NewValidationError("dataset_id", "must be a valid UUID")
		}
		if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
			if errors.Is(err, evaluation.ErrDatasetNotFound) {
				return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", *req.DatasetID))
			}
			return nil, appErrors.NewInternalError("failed to verify dataset", err)
		}
		experiment.DatasetID = &datasetID
	}

	if validationErrors := experiment.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	if err := s.repo.Create(ctx, experiment); err != nil {
		return nil, appErrors.NewInternalError("failed to create experiment", err)
	}

	s.logger.Info("experiment created",
		"experiment_id", experiment.ID,
		"project_id", projectID,
		"name", experiment.Name,
	)

	return experiment, nil
}

func (s *experimentService) Update(ctx context.Context, id uuid.UUID, projectID uuid.UUID, req *evaluation.UpdateExperimentRequest) (*evaluation.Experiment, error) {
	experiment, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return nil, appErrors.NewInternalError("failed to get experiment", err)
	}

	if req.Name != nil {
		experiment.Name = *req.Name
	}
	if req.Description != nil {
		experiment.Description = req.Description
	}
	if req.Metadata != nil {
		experiment.Metadata = req.Metadata
	}
	if req.Status != nil {
		oldStatus := experiment.Status
		experiment.Status = *req.Status

		now := time.Now()
		if oldStatus == evaluation.ExperimentStatusPending && *req.Status == evaluation.ExperimentStatusRunning {
			experiment.StartedAt = &now
		}
		// Set CompletedAt when transitioning to any terminal status
		if (*req.Status == evaluation.ExperimentStatusCompleted ||
			*req.Status == evaluation.ExperimentStatusFailed ||
			*req.Status == evaluation.ExperimentStatusPartial ||
			*req.Status == evaluation.ExperimentStatusCancelled) &&
			experiment.CompletedAt == nil {
			experiment.CompletedAt = &now
		}
	}

	experiment.UpdatedAt = time.Now()

	if validationErrors := experiment.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	if err := s.repo.Update(ctx, experiment, projectID); err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return nil, appErrors.NewInternalError("failed to update experiment", err)
	}

	s.logger.Info("experiment updated",
		"experiment_id", id,
		"project_id", projectID,
		"status", experiment.Status,
	)

	return experiment, nil
}

func (s *experimentService) Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error {
	experiment, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return appErrors.NewInternalError("failed to get experiment", err)
	}

	if err := s.repo.Delete(ctx, id, projectID); err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return appErrors.NewInternalError("failed to delete experiment", err)
	}

	s.logger.Info("experiment deleted",
		"experiment_id", id,
		"project_id", projectID,
		"name", experiment.Name,
	)

	return nil
}

func (s *experimentService) GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.Experiment, error) {
	experiment, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return nil, appErrors.NewInternalError("failed to get experiment", err)
	}
	return experiment, nil
}

func (s *experimentService) List(ctx context.Context, projectID uuid.UUID, filter *evaluation.ExperimentFilter, page, limit int) ([]*evaluation.Experiment, int64, error) {
	offset := (page - 1) * limit
	experiments, total, err := s.repo.List(ctx, projectID, filter, offset, limit)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list experiments", err)
	}
	return experiments, total, nil
}

// Rerun creates a new experiment based on an existing one, using the same dataset.
func (s *experimentService) Rerun(ctx context.Context, sourceID uuid.UUID, projectID uuid.UUID, req *evaluation.RerunExperimentRequest) (*evaluation.Experiment, error) {
	// Get the source experiment
	sourceExp, err := s.repo.GetByID(ctx, sourceID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", sourceID))
		}
		return nil, appErrors.NewInternalError("failed to get source experiment", err)
	}

	// Generate default name if not provided: "Original Name (Re-run)"
	name := fmt.Sprintf("%s (Re-run)", sourceExp.Name)
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}

	// Create the new experiment
	newExp := evaluation.NewExperiment(projectID, name)
	newExp.DatasetID = sourceExp.DatasetID
	newExp.Description = req.Description
	if newExp.Description == nil {
		newExp.Description = sourceExp.Description
	}
	if req.Metadata != nil {
		newExp.Metadata = req.Metadata
	} else if sourceExp.Metadata != nil {
		// Copy source metadata and add rerun reference
		newExp.Metadata = make(map[string]interface{})
		for k, v := range sourceExp.Metadata {
			newExp.Metadata[k] = v
		}
	}
	// Add reference to source experiment (ensure map is initialized)
	if newExp.Metadata == nil {
		newExp.Metadata = make(map[string]interface{})
	}
	newExp.Metadata["source_experiment_id"] = sourceID.String()

	if validationErrors := newExp.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	if err := s.repo.Create(ctx, newExp); err != nil {
		return nil, appErrors.NewInternalError("failed to create experiment", err)
	}

	s.logger.Info("experiment rerun created",
		"experiment_id", newExp.ID,
		"source_experiment_id", sourceID,
		"project_id", projectID,
		"name", newExp.Name,
	)

	return newExp, nil
}

// CompareExperiments compares score metrics across multiple experiments
func (s *experimentService) CompareExperiments(
	ctx context.Context,
	projectID uuid.UUID,
	experimentIDs []uuid.UUID,
	baselineID *uuid.UUID,
) (*evaluation.CompareExperimentsResponse, error) {
	if len(experimentIDs) < 2 {
		return nil, appErrors.NewValidationError("experiment_ids", "at least 2 experiments required for comparison")
	}

	// 1. Validate all experiments exist and belong to the project
	experimentSummaries := make(map[string]*evaluation.ExperimentSummary)
	experimentIDStrings := make([]string, len(experimentIDs))

	for i, expID := range experimentIDs {
		exp, err := s.repo.GetByID(ctx, expID, projectID)
		if err != nil {
			if errors.Is(err, evaluation.ErrExperimentNotFound) {
				return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", expID))
			}
			return nil, appErrors.NewInternalError("failed to get experiment", err)
		}

		experimentSummaries[expID.String()] = &evaluation.ExperimentSummary{
			Name:   exp.Name,
			Status: string(exp.Status),
		}
		experimentIDStrings[i] = expID.String()
	}

	// 2. Validate baseline is in the list (if provided)
	if baselineID != nil {
		found := false
		for _, expID := range experimentIDs {
			if expID == *baselineID {
				found = true
				break
			}
		}
		if !found {
			return nil, appErrors.NewValidationError("baseline_id", "baseline must be one of the compared experiments")
		}
	}

	// 3. Get score aggregations from ClickHouse
	scoreAggregations, err := s.scoreRepo.GetAggregationsByExperiments(ctx, projectID.String(), experimentIDStrings)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get score aggregations", err)
	}

	// 4. Convert observability.ScoreAggregation to evaluation.ScoreAggregation
	scores := make(map[string]map[string]*evaluation.ScoreAggregation)
	for scoreName, expScores := range scoreAggregations {
		scores[scoreName] = make(map[string]*evaluation.ScoreAggregation)
		for expID, agg := range expScores {
			scores[scoreName][expID] = &evaluation.ScoreAggregation{
				Mean:   agg.Mean,
				StdDev: agg.StdDev,
				Min:    agg.Min,
				Max:    agg.Max,
				Count:  agg.Count,
			}
		}
	}

	// 5. Calculate diffs if baseline is provided
	var diffs map[string]map[string]*evaluation.ScoreDiff
	if baselineID != nil {
		diffs = make(map[string]map[string]*evaluation.ScoreDiff)
		baselineIDStr := baselineID.String()

		for scoreName, expScores := range scores {
			baselineAgg := expScores[baselineIDStr]
			if baselineAgg == nil {
				continue
			}

			diffs[scoreName] = make(map[string]*evaluation.ScoreDiff)
			for expID, agg := range expScores {
				if expID == baselineIDStr {
					continue // Don't diff baseline against itself
				}
				diffs[scoreName][expID] = evaluation.CalculateDiff(baselineAgg, agg)
			}
		}
	}

	s.logger.Info("experiments compared",
		"project_id", projectID,
		"experiment_count", len(experimentIDs),
		"score_names", len(scores),
	)

	return &evaluation.CompareExperimentsResponse{
		Experiments: experimentSummaries,
		Scores:      scores,
		Diffs:       diffs,
	}, nil
}

// GetProgress returns the current progress for an experiment.
func (s *experimentService) GetProgress(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.ExperimentProgressResponse, error) {
	exp, err := s.repo.GetProgress(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return nil, appErrors.NewInternalError("failed to get experiment progress", err)
	}

	return exp.ToProgressResponse(), nil
}

// SetTotalItems sets the total number of items for an experiment.
func (s *experimentService) SetTotalItems(ctx context.Context, id uuid.UUID, projectID uuid.UUID, total int) error {
	if total < 0 {
		return appErrors.NewValidationError("total_items", "must be non-negative")
	}

	if err := s.repo.SetTotalItems(ctx, id, projectID, total); err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return appErrors.NewInternalError("failed to set total items", err)
	}

	return nil
}

// IncrementProgress atomically increments completed and/or failed counters.
func (s *experimentService) IncrementProgress(ctx context.Context, id uuid.UUID, projectID uuid.UUID, completed, failed int) error {
	if err := s.repo.IncrementCounters(ctx, id, projectID, completed, failed); err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return appErrors.NewInternalError("failed to increment progress", err)
	}

	return nil
}

// IncrementAndCheckCompletion atomically increments counters and checks if experiment is complete.
func (s *experimentService) IncrementAndCheckCompletion(ctx context.Context, id uuid.UUID, projectID uuid.UUID, completed, failed int) (bool, error) {
	isComplete, err := s.repo.IncrementCountersAndUpdateStatus(ctx, id, projectID, completed, failed)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return false, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", id))
		}
		return false, appErrors.NewInternalError("failed to increment and check completion", err)
	}

	if isComplete {
		s.logger.Info("experiment completed",
			"experiment_id", id,
			"project_id", projectID,
		)
	}

	return isComplete, nil
}

// GetMetrics returns comprehensive metrics for an experiment including progress,
// performance, and score aggregations from ClickHouse.
func (s *experimentService) GetMetrics(ctx context.Context, projectID, experimentID uuid.UUID) (*evaluation.ExperimentMetricsResponse, error) {
	// 1. Get experiment (validates existence and project ownership)
	exp, err := s.repo.GetByID(ctx, experimentID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", experimentID))
		}
		return nil, appErrors.NewInternalError("failed to get experiment", err)
	}

	// 2. Get score aggregations from ClickHouse (non-fatal if fails - graceful degradation)
	var scoreAggs map[string]map[string]*observability.ScoreAggregation
	scoreAggs, err = s.scoreRepo.GetAggregationsByExperiments(ctx, projectID.String(), []string{experimentID.String()})
	if err != nil {
		s.logger.Warn("failed to get score aggregations",
			"error", err,
			"experiment_id", experimentID,
			"project_id", projectID,
		)
		// Continue without scores - graceful degradation
	}

	// 3. Build response
	return s.buildMetricsResponse(exp, scoreAggs), nil
}

// buildMetricsResponse constructs the metrics response from experiment and score data.
func (s *experimentService) buildMetricsResponse(
	exp *evaluation.Experiment,
	scoreAggs map[string]map[string]*observability.ScoreAggregation,
) *evaluation.ExperimentMetricsResponse {
	// Progress metrics
	pendingItems := exp.TotalItems - exp.CompletedItems - exp.FailedItems
	var progressPct, successRate, errorRate float64

	if exp.TotalItems > 0 {
		processed := exp.CompletedItems + exp.FailedItems
		progressPct = float64(processed) / float64(exp.TotalItems) * 100
	}

	processed := exp.CompletedItems + exp.FailedItems
	if processed > 0 {
		successRate = float64(exp.CompletedItems) / float64(processed) * 100
		errorRate = float64(exp.FailedItems) / float64(processed) * 100
	}

	// Performance metrics
	var elapsedSeconds, etaSeconds *float64
	if exp.StartedAt != nil {
		var elapsed float64

		if exp.CompletedAt != nil {
			// Finished experiment: use fixed duration from start to completion
			elapsed = exp.CompletedAt.Sub(*exp.StartedAt).Seconds()
		} else if exp.Status == evaluation.ExperimentStatusRunning {
			// Running experiment: use live elapsed time
			elapsed = time.Since(*exp.StartedAt).Seconds()
		}

		// Only set elapsed if we calculated it (skip non-running experiments without completion)
		if exp.CompletedAt != nil || exp.Status == evaluation.ExperimentStatusRunning {
			elapsedSeconds = &elapsed
		}

		// Calculate ETA only for running experiments
		if exp.Status == evaluation.ExperimentStatusRunning && processed > 0 {
			avgTimePerItem := elapsed / float64(processed)
			remainingItems := exp.TotalItems - processed
			eta := avgTimePerItem * float64(remainingItems)
			etaSeconds = &eta
		}
	}

	// Score metrics from ClickHouse
	var scores map[string]*evaluation.ScoreMetrics
	expIDStr := exp.ID.String()
	if scoreAggs != nil {
		scores = make(map[string]*evaluation.ScoreMetrics)
		for scoreName, expScores := range scoreAggs {
			if agg, ok := expScores[expIDStr]; ok {
				scores[scoreName] = &evaluation.ScoreMetrics{
					Mean:   agg.Mean,
					StdDev: agg.StdDev,
					Min:    agg.Min,
					Max:    agg.Max,
					Count:  agg.Count,
				}
			}
		}
	}

	return &evaluation.ExperimentMetricsResponse{
		ExperimentID: exp.ID.String(),
		Status:       exp.Status,
		Progress: evaluation.ExperimentProgressMetrics{
			TotalItems:     exp.TotalItems,
			CompletedItems: exp.CompletedItems,
			FailedItems:    exp.FailedItems,
			PendingItems:   pendingItems,
			ProgressPct:    progressPct,
			SuccessRate:    successRate,
			ErrorRate:      errorRate,
		},
		Performance: evaluation.ExperimentPerformanceMetrics{
			StartedAt:      exp.StartedAt,
			CompletedAt:    exp.CompletedAt,
			ElapsedSeconds: elapsedSeconds,
			ETASeconds:     etaSeconds,
		},
		Scores: scores,
	}
}
