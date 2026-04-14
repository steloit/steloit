package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/ulid"
)

type experimentItemService struct {
	itemRepo        evaluation.ExperimentItemRepository
	experimentRepo  evaluation.ExperimentRepository
	datasetItemRepo evaluation.DatasetItemRepository
	scoreService    observability.ScoreService
	logger          *slog.Logger
}

// itemScoreData holds scores associated with an experiment item
type itemScoreData struct {
	itemID ulid.ULID
	scores []evaluation.ExperimentItemScore
}

func NewExperimentItemService(
	itemRepo evaluation.ExperimentItemRepository,
	experimentRepo evaluation.ExperimentRepository,
	datasetItemRepo evaluation.DatasetItemRepository,
	scoreService observability.ScoreService,
	logger *slog.Logger,
) evaluation.ExperimentItemService {
	return &experimentItemService{
		itemRepo:        itemRepo,
		experimentRepo:  experimentRepo,
		datasetItemRepo: datasetItemRepo,
		scoreService:    scoreService,
		logger:          logger,
	}
}

func (s *experimentItemService) CreateBatch(ctx context.Context, experimentID ulid.ULID, projectID ulid.ULID, req *evaluation.CreateExperimentItemsBatchRequest) (int, error) {
	experiment, err := s.experimentRepo.GetByID(ctx, experimentID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return 0, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", experimentID))
		}
		return 0, appErrors.NewInternalError("failed to verify experiment", err)
	}

	if len(req.Items) == 0 {
		return 0, appErrors.NewValidationError("items", "items array cannot be empty")
	}

	items := make([]*evaluation.ExperimentItem, 0, len(req.Items))
	// Collect scores from all items for batch creation
	var allItemScores []itemScoreData

	for i, itemReq := range req.Items {
		item := evaluation.NewExperimentItem(experimentID, itemReq.Input)
		item.Output = itemReq.Output
		item.Expected = itemReq.Expected
		item.TraceID = itemReq.TraceID
		item.Error = itemReq.Error
		if itemReq.Metadata != nil {
			item.Metadata = itemReq.Metadata
		}
		if itemReq.TrialNumber != nil {
			item.TrialNumber = *itemReq.TrialNumber
		}

		if itemReq.DatasetItemID != nil {
			if experiment.DatasetID == nil {
				return 0, appErrors.NewValidationError(
					fmt.Sprintf("items[%d].dataset_item_id", i),
					"cannot reference dataset items when experiment has no dataset",
				)
			}

			datasetItemID, err := ulid.Parse(*itemReq.DatasetItemID)
			if err != nil {
				return 0, appErrors.NewValidationError(
					fmt.Sprintf("items[%d].dataset_item_id", i),
					"must be a valid ULID",
				)
			}

			if _, err := s.datasetItemRepo.GetByID(ctx, datasetItemID, *experiment.DatasetID); err != nil {
				if errors.Is(err, evaluation.ErrDatasetItemNotFound) {
					return 0, appErrors.NewValidationError(
						fmt.Sprintf("items[%d].dataset_item_id", i),
						fmt.Sprintf("dataset item %s not found in experiment's dataset", datasetItemID),
					)
				}
				return 0, appErrors.NewInternalError("failed to verify dataset item", err)
			}

			item.DatasetItemID = &datasetItemID
		}

		if validationErrors := item.Validate(); len(validationErrors) > 0 {
			return 0, appErrors.NewValidationError(
				fmt.Sprintf("items[%d].%s", i, validationErrors[0].Field),
				validationErrors[0].Message,
			)
		}
		items = append(items, item)

		// Collect scores for this item
		if len(itemReq.Scores) > 0 {
			allItemScores = append(allItemScores, itemScoreData{
				itemID: item.ID,
				scores: itemReq.Scores,
			})
		}
	}

	if err := s.itemRepo.CreateBatch(ctx, items); err != nil {
		return 0, appErrors.NewInternalError("failed to create experiment items", err)
	}

	// Create scores for all items
	if len(allItemScores) > 0 {
		if err := s.createExperimentScores(ctx, experimentID, projectID, allItemScores); err != nil {
			// Log warning but don't fail the whole operation - scores are supplementary
			s.logger.Warn("failed to create experiment scores",
				"experiment_id", experimentID,
				"error", err,
			)
		}
	}

	s.logger.Info("experiment items batch created",
		"experiment_id", experimentID,
		"count", len(items),
	)

	return len(items), nil
}

// createExperimentScores creates scores for experiment items using the ScoreService
func (s *experimentItemService) createExperimentScores(
	ctx context.Context,
	experimentID ulid.ULID,
	projectID ulid.ULID,
	itemScores []itemScoreData,
) error {
	var scores []*observability.Score

	for _, itemData := range itemScores {
		for _, sc := range itemData.scores {
			// Skip failed scorers
			if sc.ScoringFailed != nil && *sc.ScoringFailed {
				continue
			}

			metadataJSON := json.RawMessage("{}")
			if sc.Metadata != nil {
				if b, err := json.Marshal(sc.Metadata); err == nil {
					metadataJSON = b
				}
			}

			// Determine data type (default to NUMERIC)
			dataType := sc.Type
			if dataType == "" {
				dataType = "NUMERIC"
			}

			expID := experimentID.String()
			itemID := itemData.itemID.String()

			score := &observability.Score{
				ID:               ulid.New().String(),
				ProjectID:        projectID.String(),
				TraceID:          nil, // No trace for experiment-only scores
				SpanID:           nil,
				Name:             sc.Name,
				Value:            sc.Value,
				StringValue:      sc.StringValue,
				Type:             dataType,
				Source:           observability.ScoreSourceAPI,
				Reason:           sc.Reason,
				Metadata:         metadataJSON,
				ExperimentID:     &expID,
				ExperimentItemID: &itemID,
				Timestamp:        time.Now(),
			}
			scores = append(scores, score)
		}
	}

	if len(scores) == 0 {
		return nil
	}

	return s.scoreService.CreateScoreBatch(ctx, scores)
}

func (s *experimentItemService) List(ctx context.Context, experimentID ulid.ULID, projectID ulid.ULID, limit, offset int) ([]*evaluation.ExperimentItem, int64, error) {
	if _, err := s.experimentRepo.GetByID(ctx, experimentID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, 0, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", experimentID))
		}
		return nil, 0, appErrors.NewInternalError("failed to verify experiment", err)
	}

	items, total, err := s.itemRepo.List(ctx, experimentID, limit, offset)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list experiment items", err)
	}
	return items, total, nil
}
