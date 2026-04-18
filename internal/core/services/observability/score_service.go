package observability

import (
	"github.com/google/uuid"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type ScoreService struct {
	scoreRepo observability.ScoreRepository
	traceRepo observability.TraceRepository
}

func NewScoreService(
	scoreRepo observability.ScoreRepository,
	traceRepo observability.TraceRepository,
) *ScoreService {
	return &ScoreService{
		scoreRepo: scoreRepo,
		traceRepo: traceRepo,
	}
}

func (s *ScoreService) CreateScore(ctx context.Context, score *observability.Score) error {
	if score.ProjectID == uuid.Nil {
		return appErrors.NewValidationError("project_id is required", "score must have a valid project_id")
	}
	if score.Name == "" {
		return appErrors.NewValidationError("name is required", "score name cannot be empty")
	}

	// Scores must have EITHER trace/span linkage OR experiment linkage
	hasTraceLinkage := score.TraceID != nil && *score.TraceID != "" &&
		score.SpanID != nil && *score.SpanID != ""
	hasExperimentLinkage := score.ExperimentID != nil && *score.ExperimentID != uuid.Nil

	if !hasTraceLinkage && !hasExperimentLinkage {
		return appErrors.NewValidationError(
			"score must have either trace_id+span_id or experiment_id",
			"all scores must be linked to a trace/span or experiment",
		)
	}
	if err := s.validateScoreData(score); err != nil {
		return err
	}
	if score.ID == uuid.Nil {
		score.ID = uid.New()
	}
	if score.Timestamp.IsZero() {
		score.Timestamp = time.Now()
	}
	if err := s.validateScoreTargets(ctx, score); err != nil {
		return err
	}
	if err := s.scoreRepo.Create(ctx, score); err != nil {
		return appErrors.NewInternalError("failed to create score", err)
	}

	return nil
}

func (s *ScoreService) UpdateScore(ctx context.Context, score *observability.Score) error {
	existing, err := s.scoreRepo.GetByID(ctx, score.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appErrors.NewNotFoundError("score " + score.ID.String())
		}
		return appErrors.NewInternalError("failed to get score", err)
	}

	mergeScoreFields(existing, score)

	if err := s.validateScoreData(existing); err != nil {
		return err
	}
	if err := s.scoreRepo.Update(ctx, existing); err != nil {
		return appErrors.NewInternalError("failed to update score", err)
	}

	return nil
}

func mergeScoreFields(dst *observability.Score, src *observability.Score) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Value != nil {
		dst.Value = src.Value
	}
	if src.StringValue != nil {
		dst.StringValue = src.StringValue
	}
	if src.Type != "" {
		dst.Type = src.Type
	}
	if src.Source != "" {
		dst.Source = src.Source
	}
	if src.Reason != nil {
		dst.Reason = src.Reason
	}
	if len(src.Metadata) > 0 {
		dst.Metadata = src.Metadata
	}
	if !src.Timestamp.IsZero() {
		dst.Timestamp = src.Timestamp
	}
}

func (s *ScoreService) DeleteScore(ctx context.Context, id uuid.UUID) error {
	_, err := s.scoreRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appErrors.NewNotFoundError("score " + id.String())
		}
		return appErrors.NewInternalError("failed to get score", err)
	}
	if err := s.scoreRepo.Delete(ctx, id); err != nil {
		return appErrors.NewInternalError("failed to delete score", err)
	}

	return nil
}

func (s *ScoreService) GetScoreByID(ctx context.Context, id uuid.UUID) (*observability.Score, error) {
	score, err := s.scoreRepo.GetByID(ctx, id)
	if err != nil {
		return nil, appErrors.NewNotFoundError("score " + id.String())
	}

	return score, nil
}

func (s *ScoreService) GetScoresByTraceID(ctx context.Context, traceID string) ([]*observability.Score, error) {
	if traceID == "" {
		return nil, appErrors.NewValidationError("trace_id is required", "scores query requires a valid trace_id")
	}

	scores, err := s.scoreRepo.GetByTraceID(ctx, traceID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get scores", err)
	}

	return scores, nil
}

func (s *ScoreService) GetScoresBySpanID(ctx context.Context, spanID string) ([]*observability.Score, error) {
	if spanID == "" {
		return nil, appErrors.NewValidationError("span_id is required", "scores query requires a valid span_id")
	}

	scores, err := s.scoreRepo.GetBySpanID(ctx, spanID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get scores", err)
	}

	return scores, nil
}

func (s *ScoreService) GetScoresByFilter(ctx context.Context, filter *observability.ScoreFilter) ([]*observability.Score, error) {
	scores, err := s.scoreRepo.GetByFilter(ctx, filter)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get scores by filter", err)
	}

	return scores, nil
}

func (s *ScoreService) CreateScoreBatch(ctx context.Context, scores []*observability.Score) error {
	if len(scores) == 0 {
		return appErrors.NewValidationError("scores array cannot be empty", "batch create requires at least one score")
	}

	// Validate all scores
	for i, score := range scores {
		if score.ProjectID == uuid.Nil {
			return appErrors.NewValidationError(
				fmt.Sprintf("score[%d]: project_id is required", i),
				"all scores must have valid project_id",
			)
		}
		if score.Name == "" {
			return appErrors.NewValidationError(
				fmt.Sprintf("score[%d]: name is required", i),
				"all scores must have a name",
			)
		}

		// Scores must have EITHER trace/span linkage OR experiment linkage
		hasTraceLinkage := score.TraceID != nil && *score.TraceID != "" &&
			score.SpanID != nil && *score.SpanID != ""
		hasExperimentLinkage := score.ExperimentID != nil && *score.ExperimentID != uuid.Nil

		if !hasTraceLinkage && !hasExperimentLinkage {
			return appErrors.NewValidationError(
				fmt.Sprintf("score[%d]: must have either trace_id+span_id or experiment_id", i),
				"all scores must be linked to a trace/span or experiment",
			)
		}

		// Validate data type and value
		if err := s.validateScoreData(score); err != nil {
			return err
		}

		// Generate ID if not provided
		if score.ID == uuid.Nil {
			score.ID = uid.New()
		}

		// Set timestamp if not provided
		if score.Timestamp.IsZero() {
			score.Timestamp = time.Now()
		}
	}

	// Create batch
	if err := s.scoreRepo.CreateBatch(ctx, scores); err != nil {
		return appErrors.NewInternalError("failed to create score batch", err)
	}

	return nil
}

func (s *ScoreService) CountScores(ctx context.Context, filter *observability.ScoreFilter) (int64, error) {
	count, err := s.scoreRepo.Count(ctx, filter)
	if err != nil {
		return 0, appErrors.NewInternalError("failed to count scores", err)
	}

	return count, nil
}

func (s *ScoreService) validateScoreData(score *observability.Score) error {
	switch score.Type {
	case observability.ScoreTypeNumeric:
		if score.Value == nil {
			return appErrors.NewValidationError("numeric score must have value", "value is required for NUMERIC type")
		}
		if score.StringValue != nil {
			return appErrors.NewValidationError("numeric score cannot have string_value", "string_value not allowed for NUMERIC type")
		}

	case observability.ScoreTypeCategorical:
		if score.StringValue == nil {
			return appErrors.NewValidationError("categorical score must have string_value", "string_value is required for CATEGORICAL type")
		}
		if score.Value != nil {
			return appErrors.NewValidationError("categorical score cannot have numeric value", "value not allowed for CATEGORICAL type")
		}

	case observability.ScoreTypeBoolean:
		if score.Value == nil {
			return appErrors.NewValidationError("boolean score must have value", "value is required for BOOLEAN type")
		}
		if *score.Value != 0 && *score.Value != 1 {
			return appErrors.NewValidationError("boolean score value must be 0 or 1", "value must be 0 (false) or 1 (true)")
		}
		if score.StringValue != nil {
			return appErrors.NewValidationError("boolean score cannot have string_value", "string_value not allowed for BOOLEAN type")
		}

	default:
		return appErrors.NewValidationError("invalid type", "type must be NUMERIC, CATEGORICAL, or BOOLEAN")
	}

	return nil
}

func (s *ScoreService) validateScoreTargets(ctx context.Context, score *observability.Score) error {
	if score.TraceID != nil && *score.TraceID != "" {
		_, err := s.traceRepo.GetRootSpan(ctx, *score.TraceID)
		if err != nil {
			return appErrors.NewNotFoundError("trace " + *score.TraceID)
		}
	}

	if score.SpanID != nil && *score.SpanID != "" {
		_, err := s.traceRepo.GetSpan(ctx, *score.SpanID)
		if err != nil {
			return appErrors.NewNotFoundError("span " + *score.SpanID)
		}
	}

	return nil
}
