package annotation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/annotation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ItemRepository implements annotation.ItemRepository using PostgreSQL.
type ItemRepository struct {
	db *gorm.DB
}

// NewItemRepository creates a new ItemRepository.
func NewItemRepository(db *gorm.DB) *ItemRepository {
	return &ItemRepository{db: db}
}

// getDB returns transaction-aware DB instance.
func (r *ItemRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new queue item.
func (r *ItemRepository) Create(ctx context.Context, item *annotation.QueueItem) error {
	result := r.getDB(ctx).WithContext(ctx).Create(item)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return annotation.ErrItemExists
		}
		return result.Error
	}
	return nil
}

// CreateBatch creates multiple queue items in a single operation.
// Uses ON CONFLICT DO NOTHING to skip duplicates (idempotent batch inserts).
// Returns the number of items actually inserted (excluding duplicates).
func (r *ItemRepository) CreateBatch(ctx context.Context, items []*annotation.QueueItem) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}

	// Use ON CONFLICT DO NOTHING to skip duplicates
	result := r.getDB(ctx).WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "queue_id"}, {Name: "object_id"}, {Name: "object_type"}},
			DoNothing: true,
		}).
		CreateInBatches(items, 100)

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// GetByID retrieves a queue item by its ID.
func (r *ItemRepository) GetByID(ctx context.Context, id uuid.UUID) (*annotation.QueueItem, error) {
	var item annotation.QueueItem
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id.String()).
		First(&item)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, annotation.ErrItemNotFound
		}
		return nil, result.Error
	}
	return &item, nil
}

// GetByIDForQueue retrieves a queue item by its ID within a specific queue.
func (r *ItemRepository) GetByIDForQueue(ctx context.Context, id, queueID uuid.UUID) (*annotation.QueueItem, error) {
	var item annotation.QueueItem
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND queue_id = ?", id.String(), queueID.String()).
		First(&item)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, annotation.ErrItemNotFound
		}
		return nil, result.Error
	}
	return &item, nil
}

// List retrieves queue items with optional filtering and pagination.
func (r *ItemRepository) List(ctx context.Context, queueID uuid.UUID, filter *annotation.ItemFilter) ([]*annotation.QueueItem, int64, error) {
	var items []*annotation.QueueItem
	var total int64

	baseQuery := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("queue_id = ?", queueID.String())

	if filter != nil && filter.Status != nil {
		baseQuery = baseQuery.Where("status = ?", string(*filter.Status))
	}

	// Count total
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	query := r.getDB(ctx).WithContext(ctx).
		Where("queue_id = ?", queueID.String())

	if filter != nil && filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	// Order by priority (DESC) then created_at (ASC) for FIFO within same priority
	query = query.Order("priority DESC, created_at ASC")

	if filter != nil {
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	result := query.Find(&items)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return items, total, nil
}

// Update updates an existing queue item.
func (r *ItemRepository) Update(ctx context.Context, item *annotation.QueueItem) error {
	item.UpdatedAt = time.Now()
	result := r.getDB(ctx).WithContext(ctx).Save(item)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrItemNotFound
	}
	return nil
}

// Delete removes a queue item by ID.
func (r *ItemRepository) Delete(ctx context.Context, id, queueID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND queue_id = ?", id.String(), queueID.String()).
		Delete(&annotation.QueueItem{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrItemNotFound
	}
	return nil
}

// ExistsByObject checks if an item for the given object exists in the queue.
func (r *ItemRepository) ExistsByObject(ctx context.Context, queueID uuid.UUID, objectID string, objectType annotation.ObjectType) (bool, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("queue_id = ? AND object_id = ? AND object_type = ?", queueID.String(), objectID, string(objectType)).
		Count(&count)

	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// FetchAndLockNext finds and locks the next available item for annotation.
// Follows Langfuse pattern: finds first pending item where:
// - Never locked, OR
// - Lock expired (locked_at + lock_timeout < NOW()), OR
// - Locked by current user (can reclaim)
// Uses SELECT ... FOR UPDATE SKIP LOCKED for concurrent safety.
func (r *ItemRepository) FetchAndLockNext(ctx context.Context, queueID, userID uuid.UUID, lockTimeout int, seenItemIDs []uuid.UUID) (*annotation.QueueItem, error) {
	var item annotation.QueueItem
	now := time.Now()
	lockExpiry := now.Add(-time.Duration(lockTimeout) * time.Second)

	// Build the query
	query := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("queue_id = ?", queueID.String()).
		Where("status = ?", string(annotation.ItemStatusPending))

	// Exclude seen items
	if len(seenItemIDs) > 0 {
		seenIDs := make([]string, len(seenItemIDs))
		for i, id := range seenItemIDs {
			seenIDs[i] = id.String()
		}
		query = query.Where("id NOT IN ?", seenIDs)
	}

	// Find items that can be claimed:
	// 1. Never locked (locked_at IS NULL), OR
	// 2. Lock expired (locked_at < lockExpiry), OR
	// 3. Locked by current user (locked_by_user_id = userID)
	query = query.Where(
		"(locked_at IS NULL) OR (locked_at < ?) OR (locked_by_user_id = ?)",
		lockExpiry, userID.String(),
	)

	// Order by priority (higher first), then FIFO within same priority
	query = query.Order("priority DESC, created_at ASC")

	// Use FOR UPDATE SKIP LOCKED for concurrent safety
	query = query.Clauses(clause.Locking{
		Strength: "UPDATE",
		Options:  "SKIP LOCKED",
	})

	// Get the first available item
	result := query.First(&item)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, annotation.ErrNoItemsAvailable
		}
		return nil, result.Error
	}

	// Lock the item
	item.LockedAt = &now
	item.LockedByUserID = &userID
	item.UpdatedAt = now

	if err := r.getDB(ctx).WithContext(ctx).Save(&item).Error; err != nil {
		return nil, err
	}

	return &item, nil
}

// Complete marks an item as completed by the annotator.
func (r *ItemRepository) Complete(ctx context.Context, id, userID uuid.UUID) error {
	now := time.Now()
	result := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("id = ?", id.String()).
		Updates(map[string]interface{}{
			"status":            string(annotation.ItemStatusCompleted),
			"annotator_user_id": userID.String(),
			"completed_at":      now,
			"updated_at":        now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrItemNotFound
	}
	return nil
}

// Skip marks an item as skipped by the annotator.
func (r *ItemRepository) Skip(ctx context.Context, id, userID uuid.UUID) error {
	now := time.Now()
	result := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("id = ?", id.String()).
		Updates(map[string]interface{}{
			"status":            string(annotation.ItemStatusSkipped),
			"annotator_user_id": userID.String(),
			"completed_at":      now,
			"updated_at":        now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrItemNotFound
	}
	return nil
}

// ReleaseLock releases the lock on an item.
func (r *ItemRepository) ReleaseLock(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	result := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("id = ?", id.String()).
		Updates(map[string]interface{}{
			"locked_at":         nil,
			"locked_by_user_id": nil,
			"updated_at":        now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrItemNotFound
	}
	return nil
}

// ReleaseExpiredLocks releases all locks that have expired.
// Returns the count of locks released.
func (r *ItemRepository) ReleaseExpiredLocks(ctx context.Context, queueID uuid.UUID, lockTimeout int) (int64, error) {
	now := time.Now()
	lockExpiry := now.Add(-time.Duration(lockTimeout) * time.Second)

	result := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("queue_id = ?", queueID.String()).
		Where("locked_at IS NOT NULL").
		Where("locked_at < ?", lockExpiry).
		Where("status = ?", string(annotation.ItemStatusPending)).
		Updates(map[string]interface{}{
			"locked_at":         nil,
			"locked_by_user_id": nil,
			"updated_at":        now,
		})

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// GetStats retrieves aggregated statistics for a queue.
func (r *ItemRepository) GetStats(ctx context.Context, queueID uuid.UUID, lockTimeout int) (*annotation.QueueStats, error) {
	stats := &annotation.QueueStats{}
	lockExpiry := time.Now().Add(-time.Duration(lockTimeout) * time.Second)

	// Count total items
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("queue_id = ?", queueID.String()).
		Count(&stats.TotalItems).Error; err != nil {
		return nil, err
	}

	// Count by status
	type StatusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []StatusCount
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Select("status, COUNT(*) as count").
		Where("queue_id = ?", queueID.String()).
		Group("status").
		Scan(&statusCounts).Error; err != nil {
		return nil, err
	}

	for _, sc := range statusCounts {
		switch annotation.ItemStatus(sc.Status) {
		case annotation.ItemStatusPending:
			stats.PendingItems = sc.Count
		case annotation.ItemStatusCompleted:
			stats.CompletedItems = sc.Count
		case annotation.ItemStatusSkipped:
			stats.SkippedItems = sc.Count
		}
	}

	// Count in-progress items (pending with active lock)
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueItem{}).
		Where("queue_id = ?", queueID.String()).
		Where("status = ?", string(annotation.ItemStatusPending)).
		Where("locked_at IS NOT NULL").
		Where("locked_at >= ?", lockExpiry).
		Count(&stats.InProgressItems).Error; err != nil {
		return nil, err
	}

	// Adjust pending count to exclude in-progress items
	stats.PendingItems -= stats.InProgressItems

	return stats, nil
}
