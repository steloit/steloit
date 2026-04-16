package annotation

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"brokle/internal/core/domain/annotation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

// QueueRepository implements annotation.QueueRepository using PostgreSQL.
type QueueRepository struct {
	db *gorm.DB
}

// NewQueueRepository creates a new QueueRepository.
func NewQueueRepository(db *gorm.DB) *QueueRepository {
	return &QueueRepository{db: db}
}

// getDB returns transaction-aware DB instance.
func (r *QueueRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new annotation queue.
func (r *QueueRepository) Create(ctx context.Context, queue *annotation.AnnotationQueue) error {
	result := r.getDB(ctx).WithContext(ctx).Create(queue)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return annotation.ErrQueueExists
		}
		return result.Error
	}
	return nil
}

// GetByID retrieves an annotation queue by its ID.
func (r *QueueRepository) GetByID(ctx context.Context, id, projectID uuid.UUID) (*annotation.AnnotationQueue, error) {
	var queue annotation.AnnotationQueue
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		First(&queue)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, annotation.ErrQueueNotFound
		}
		return nil, result.Error
	}
	return &queue, nil
}

// GetByName retrieves an annotation queue by name within a project.
func (r *QueueRepository) GetByName(ctx context.Context, name string, projectID uuid.UUID) (*annotation.AnnotationQueue, error) {
	var queue annotation.AnnotationQueue
	result := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ? AND name = ?", projectID.String(), name).
		First(&queue)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &queue, nil
}

// List retrieves all annotation queues for a project with optional filtering and pagination.
func (r *QueueRepository) List(ctx context.Context, projectID uuid.UUID, filter *annotation.QueueFilter, offset, limit int) ([]*annotation.AnnotationQueue, int64, error) {
	var queues []*annotation.AnnotationQueue
	var total int64

	query := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ?", projectID.String())

	if filter != nil {
		if filter.Status != nil {
			query = query.Where("status = ?", string(*filter.Status))
		}
		if filter.Search != nil && *filter.Search != "" {
			search := "%" + strings.ToLower(*filter.Search) + "%"
			query = query.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", search, search)
		}
	}

	if err := query.Model(&annotation.AnnotationQueue{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&queues)
	if result.Error != nil {
		return nil, 0, result.Error
	}
	return queues, total, nil
}

// Update updates an existing annotation queue.
func (r *QueueRepository) Update(ctx context.Context, queue *annotation.AnnotationQueue) error {
	result := r.getDB(ctx).WithContext(ctx).Save(queue)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return annotation.ErrQueueExists
		}
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrQueueNotFound
	}
	return nil
}

// Delete removes an annotation queue by ID.
func (r *QueueRepository) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		Delete(&annotation.AnnotationQueue{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrQueueNotFound
	}
	return nil
}

// ExistsByName checks if a queue with the given name exists in the project.
func (r *QueueRepository) ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.AnnotationQueue{}).
		Where("project_id = ? AND name = ?", projectID.String(), name).
		Count(&count)

	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// ListAllActive retrieves all active annotation queues across all projects.
// This is used by the lock expiry worker to release expired locks.
func (r *QueueRepository) ListAllActive(ctx context.Context) ([]*annotation.AnnotationQueue, error) {
	var queues []*annotation.AnnotationQueue

	result := r.getDB(ctx).WithContext(ctx).
		Where("status = ?", string(annotation.QueueStatusActive)).
		Find(&queues)

	if result.Error != nil {
		return nil, result.Error
	}
	return queues, nil
}

// isUniqueViolation checks if the error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "23505") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "duplicate key")
}
