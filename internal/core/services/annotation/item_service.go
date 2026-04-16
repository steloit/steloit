package annotation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/annotation"
	"brokle/internal/core/domain/common"
	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	"brokle/internal/core/domain/organization"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type itemService struct {
	queueRepo          annotation.QueueRepository
	itemRepo           annotation.ItemRepository
	assignmentRepo     annotation.AssignmentRepository
	scoreConfigService evaluation.ScoreConfigService
	scoreService       observability.ScoreService
	projectRepo        organization.ProjectRepository
	transactor         common.Transactor
	logger             *slog.Logger
}

// NewItemService creates a new ItemService implementation.
func NewItemService(
	queueRepo annotation.QueueRepository,
	itemRepo annotation.ItemRepository,
	assignmentRepo annotation.AssignmentRepository,
	scoreConfigService evaluation.ScoreConfigService,
	scoreService observability.ScoreService,
	projectRepo organization.ProjectRepository,
	transactor common.Transactor,
	logger *slog.Logger,
) annotation.ItemService {
	return &itemService{
		queueRepo:          queueRepo,
		itemRepo:           itemRepo,
		assignmentRepo:     assignmentRepo,
		scoreConfigService: scoreConfigService,
		scoreService:       scoreService,
		projectRepo:        projectRepo,
		transactor:         transactor,
		logger:             logger,
	}
}

// AddItems adds items to a queue.
// Returns the count of items actually created (excluding duplicates).
func (s *itemService) AddItems(ctx context.Context, queueID, projectID uuid.UUID, req *annotation.AddItemsBatchRequest) (int, error) {
	// Verify queue exists and belongs to project
	queue, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return 0, appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return 0, appErrors.NewInternalError("failed to get annotation queue", err)
	}

	// Check queue is active
	if queue.Status == annotation.QueueStatusArchived {
		return 0, appErrors.NewBadRequestError("cannot add items to an archived queue", "queue is archived")
	}

	// Create items
	items := make([]*annotation.QueueItem, 0, len(req.Items))
	for _, itemReq := range req.Items {
		item := annotation.NewQueueItem(queueID, itemReq.ObjectID, itemReq.ObjectType)
		item.Priority = itemReq.Priority
		if itemReq.Metadata != nil {
			item.Metadata = itemReq.Metadata
		}

		if validationErrors := item.Validate(); len(validationErrors) > 0 {
			return 0, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
		}

		items = append(items, item)
	}

	// Batch create with duplicate handling (ON CONFLICT DO NOTHING)
	createdCount, err := s.itemRepo.CreateBatch(ctx, items)
	if err != nil {
		return 0, appErrors.NewInternalError("failed to add items to queue", err)
	}

	s.logger.Info("items added to annotation queue",
		"queue_id", queueID,
		"project_id", projectID,
		"created_count", createdCount,
		"requested_count", len(items),
	)

	return int(createdCount), nil
}

// ListItems retrieves items in a queue with optional filtering and pagination.
func (s *itemService) ListItems(ctx context.Context, queueID, projectID uuid.UUID, filter *annotation.ItemFilter) ([]*annotation.QueueItem, int64, error) {
	// Verify queue exists and belongs to project
	_, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return nil, 0, appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return nil, 0, appErrors.NewInternalError("failed to get annotation queue", err)
	}

	items, total, err := s.itemRepo.List(ctx, queueID, filter)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list queue items", err)
	}

	return items, total, nil
}

// ClaimNext claims the next available item for the user to annotate.
// Follows Langfuse pattern: finds first pending item where lock is available or reclaimable.
func (s *itemService) ClaimNext(ctx context.Context, queueID, projectID, userID uuid.UUID, seenItemIDs []uuid.UUID) (*annotation.QueueItem, error) {
	// Verify queue exists and belongs to project
	queue, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return nil, appErrors.NewInternalError("failed to get annotation queue", err)
	}

	// Check queue is active
	if queue.Status != annotation.QueueStatusActive {
		return nil, appErrors.NewBadRequestError(fmt.Sprintf("cannot claim items from %s queue", queue.Status), "queue is not active")
	}

	// Fetch and lock the next available item within a transaction.
	// This ensures the FOR UPDATE lock is held until the update completes,
	// preventing concurrent callers from claiming the same item.
	var item *annotation.QueueItem
	err = s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var txErr error
		item, txErr = s.itemRepo.FetchAndLockNext(txCtx, queueID, userID, queue.Settings.LockTimeoutSeconds, seenItemIDs)
		return txErr
	})

	if err != nil {
		if errors.Is(err, annotation.ErrNoItemsAvailable) {
			return nil, appErrors.NewNotFoundError("no items available for annotation")
		}
		return nil, appErrors.NewInternalError("failed to claim next item", err)
	}

	s.logger.Info("item claimed for annotation",
		"item_id", item.ID,
		"queue_id", queueID,
		"user_id", userID,
		"object_id", item.ObjectID,
		"object_type", item.ObjectType,
	)

	return item, nil
}

// Complete marks an item as completed and submits the scores.
func (s *itemService) Complete(ctx context.Context, itemID, queueID, projectID, userID uuid.UUID, req *annotation.CompleteItemRequest) error {
	// Verify queue exists and belongs to project
	queue, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return appErrors.NewInternalError("failed to get annotation queue", err)
	}

	// Get the item
	item, err := s.itemRepo.GetByIDForQueue(ctx, itemID, queueID)
	if err != nil {
		if errors.Is(err, annotation.ErrItemNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("queue item %s", itemID))
		}
		return appErrors.NewInternalError("failed to get queue item", err)
	}

	// Verify item status
	if item.Status == annotation.ItemStatusCompleted {
		return appErrors.NewBadRequestError("item is already completed", "status is completed")
	}
	if item.Status == annotation.ItemStatusSkipped {
		return appErrors.NewBadRequestError("item was skipped and cannot be completed", "status is skipped")
	}

	// Verify user holds the lock (or lock expired and they can complete anyway)
	lockDuration := queue.GetLockDuration()
	if item.IsLocked(lockDuration) && !item.IsLockedBy(userID, lockDuration) {
		return appErrors.NewForbiddenError("item is locked by another user")
	}

	// Submit scores to ClickHouse if any were provided
	if len(req.Scores) > 0 {
		if err := s.submitScores(ctx, item, queue, projectID, userID, req.Scores); err != nil {
			// Log but don't fail the completion - scores can be retried
			s.logger.Error("failed to submit scores",
				"error", err,
				"item_id", itemID,
				"queue_id", queueID,
				"scores_count", len(req.Scores),
			)
			// Continue with completion even if score submission fails
		}
	}

	// Mark as completed
	if err := s.itemRepo.Complete(ctx, itemID, userID); err != nil {
		return appErrors.NewInternalError("failed to complete item", err)
	}

	s.logger.Info("item completed",
		"item_id", itemID,
		"queue_id", queueID,
		"user_id", userID,
		"scores_count", len(req.Scores),
	)

	return nil
}

// submitScores creates Score entities from annotation submissions and sends them to ClickHouse.
func (s *itemService) submitScores(
	ctx context.Context,
	item *annotation.QueueItem,
	queue *annotation.AnnotationQueue,
	projectID, userID uuid.UUID,
	submissions []annotation.ScoreSubmission,
) error {
	// Get project to retrieve organization ID
	project, err := s.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// Build scores from submissions
	scores := make([]*observability.Score, 0, len(submissions))
	now := time.Now()

	for _, sub := range submissions {
		// Look up the score config to get the name and data type
		scoreConfigID, err := uuid.Parse(sub.ScoreConfigID)
		if err != nil {
			s.logger.Warn("invalid score config ID, skipping",
				"score_config_id", sub.ScoreConfigID,
				"error", err,
			)
			continue
		}

		scoreConfig, err := s.scoreConfigService.GetByID(ctx, scoreConfigID, projectID)
		if err != nil {
			s.logger.Warn("score config not found, skipping",
				"score_config_id", sub.ScoreConfigID,
				"error", err,
			)
			continue
		}

		// Create the score entity
		score := &observability.Score{
			ID:             uid.New().String(),
			ProjectID:      projectID.String(),
			OrganizationID: project.OrganizationID.String(),
			Name:           scoreConfig.Name,
			Type:           string(scoreConfig.Type),
			Source:         observability.ScoreSourceAnnotation,
			Metadata:       s.buildScoreMetadata(queue, item, userID, sub.Comment),
		}

		// Set trace or span ID based on object type
		if item.ObjectType == annotation.ObjectTypeTrace {
			score.TraceID = &item.ObjectID
		} else {
			score.SpanID = &item.ObjectID
		}

		// Set value based on type
		switch scoreConfig.Type {
		case evaluation.ScoreTypeNumeric:
			if numVal, ok := sub.Value.(float64); ok {
				score.Value = &numVal
			} else if intVal, ok := sub.Value.(int); ok {
				floatVal := float64(intVal)
				score.Value = &floatVal
			}
		case evaluation.ScoreTypeCategorical:
			if strVal, ok := sub.Value.(string); ok {
				score.StringValue = &strVal
			}
		case evaluation.ScoreTypeBoolean:
			// Convert boolean to numeric (1.0 for true, 0.0 for false)
			if boolVal, ok := sub.Value.(bool); ok {
				var numVal float64
				if boolVal {
					numVal = 1.0
				}
				score.Value = &numVal
			}
		}

		// Set reason/comment if provided
		if sub.Comment != nil && *sub.Comment != "" {
			score.Reason = sub.Comment
		}

		// Validate the score has a value
		if score.Value == nil && score.StringValue == nil {
			s.logger.Warn("score has no value, skipping",
				"score_config_id", sub.ScoreConfigID,
				"type", scoreConfig.Type,
			)
			continue
		}

		scores = append(scores, score)
	}

	if len(scores) == 0 {
		return nil // No valid scores to submit
	}

	// Submit scores to ClickHouse
	if err := s.scoreService.CreateScoreBatch(ctx, scores); err != nil {
		return fmt.Errorf("failed to create scores: %w", err)
	}

	s.logger.Info("annotation scores submitted to ClickHouse",
		"item_id", item.ID,
		"queue_id", queue.ID,
		"scores_submitted", len(scores),
		"duration_ms", time.Since(now).Milliseconds(),
	)

	return nil
}

// buildScoreMetadata creates metadata JSON for annotation scores.
func (s *itemService) buildScoreMetadata(
	queue *annotation.AnnotationQueue,
	item *annotation.QueueItem,
	userID uuid.UUID,
	comment *string,
) json.RawMessage {
	metadata := map[string]interface{}{
		"source":       "annotation_queue",
		"queue_id":     queue.ID.String(),
		"queue_name":   queue.Name,
		"item_id":      item.ID.String(),
		"annotator_id": userID.String(),
	}

	if comment != nil && *comment != "" {
		metadata["comment"] = *comment
	}

	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage("{}")
	}
	return jsonBytes
}

// Skip marks an item as skipped by the user.
func (s *itemService) Skip(ctx context.Context, itemID, queueID, projectID, userID uuid.UUID, req *annotation.SkipItemRequest) error {
	// Verify queue exists and belongs to project
	queue, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return appErrors.NewInternalError("failed to get annotation queue", err)
	}

	// Get the item
	item, err := s.itemRepo.GetByIDForQueue(ctx, itemID, queueID)
	if err != nil {
		if errors.Is(err, annotation.ErrItemNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("queue item %s", itemID))
		}
		return appErrors.NewInternalError("failed to get queue item", err)
	}

	// Verify item status
	if item.Status == annotation.ItemStatusCompleted {
		return appErrors.NewBadRequestError("item is already completed", "status is completed")
	}
	if item.Status == annotation.ItemStatusSkipped {
		return appErrors.NewBadRequestError("item is already skipped", "status is skipped")
	}

	// Verify user holds the lock (or lock expired and they can skip anyway)
	lockDuration := queue.GetLockDuration()
	if item.IsLocked(lockDuration) && !item.IsLockedBy(userID, lockDuration) {
		return appErrors.NewForbiddenError("item is locked by another user")
	}

	// Mark as skipped
	if err := s.itemRepo.Skip(ctx, itemID, userID); err != nil {
		return appErrors.NewInternalError("failed to skip item", err)
	}

	reason := ""
	if req.Reason != nil {
		reason = *req.Reason
	}

	s.logger.Info("item skipped",
		"item_id", itemID,
		"queue_id", queueID,
		"user_id", userID,
		"reason", reason,
	)

	return nil
}

// ReleaseLock releases the lock on an item, returning it to the pending pool.
func (s *itemService) ReleaseLock(ctx context.Context, itemID, queueID, projectID, userID uuid.UUID) error {
	// Verify queue exists and belongs to project
	queue, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return appErrors.NewInternalError("failed to get annotation queue", err)
	}

	// Get the item
	item, err := s.itemRepo.GetByIDForQueue(ctx, itemID, queueID)
	if err != nil {
		if errors.Is(err, annotation.ErrItemNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("queue item %s", itemID))
		}
		return appErrors.NewInternalError("failed to get queue item", err)
	}

	// Verify item is pending and locked
	if item.Status != annotation.ItemStatusPending {
		return appErrors.NewBadRequestError("cannot release lock on completed or skipped item", "status is not pending")
	}

	lockDuration := queue.GetLockDuration()
	if !item.IsLocked(lockDuration) {
		return appErrors.NewBadRequestError("item is not currently locked", "no active lock")
	}

	// Verify user holds the lock
	if !item.IsLockedBy(userID, lockDuration) {
		return appErrors.NewForbiddenError("item is locked by another user")
	}

	// Release the lock
	if err := s.itemRepo.ReleaseLock(ctx, itemID); err != nil {
		return appErrors.NewInternalError("failed to release lock", err)
	}

	s.logger.Info("item lock released",
		"item_id", itemID,
		"queue_id", queueID,
		"user_id", userID,
	)

	return nil
}

// DeleteItem removes an item from the queue.
func (s *itemService) DeleteItem(ctx context.Context, itemID, queueID, projectID uuid.UUID) error {
	// Verify queue exists and belongs to project
	_, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return appErrors.NewInternalError("failed to get annotation queue", err)
	}

	// Delete the item
	if err := s.itemRepo.Delete(ctx, itemID, queueID); err != nil {
		if errors.Is(err, annotation.ErrItemNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("queue item %s", itemID))
		}
		return appErrors.NewInternalError("failed to delete queue item", err)
	}

	s.logger.Info("queue item deleted",
		"item_id", itemID,
		"queue_id", queueID,
		"project_id", projectID,
	)

	return nil
}

// GetStats retrieves statistics for a queue.
func (s *itemService) GetStats(ctx context.Context, queueID, projectID uuid.UUID) (*annotation.QueueStats, error) {
	// Verify queue exists and belongs to project
	queue, err := s.queueRepo.GetByID(ctx, queueID, projectID)
	if err != nil {
		if errors.Is(err, annotation.ErrQueueNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("annotation queue %s", queueID))
		}
		return nil, appErrors.NewInternalError("failed to get annotation queue", err)
	}

	stats, err := s.itemRepo.GetStats(ctx, queueID, queue.Settings.LockTimeoutSeconds)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get queue stats", err)
	}

	return stats, nil
}
