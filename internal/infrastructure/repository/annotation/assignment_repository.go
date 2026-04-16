package annotation

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"brokle/internal/core/domain/annotation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

// AssignmentRepository implements annotation.AssignmentRepository using PostgreSQL.
type AssignmentRepository struct {
	db *gorm.DB
}

// NewAssignmentRepository creates a new AssignmentRepository.
func NewAssignmentRepository(db *gorm.DB) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

// getDB returns transaction-aware DB instance.
func (r *AssignmentRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new queue assignment.
func (r *AssignmentRepository) Create(ctx context.Context, assignment *annotation.QueueAssignment) error {
	result := r.getDB(ctx).WithContext(ctx).Create(assignment)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return annotation.ErrAssignmentExists
		}
		return result.Error
	}
	return nil
}

// Delete removes a queue assignment by queue and user ID.
func (r *AssignmentRepository) Delete(ctx context.Context, queueID, userID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("queue_id = ? AND user_id = ?", queueID.String(), userID.String()).
		Delete(&annotation.QueueAssignment{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return annotation.ErrAssignmentNotFound
	}
	return nil
}

// GetByQueueAndUser retrieves an assignment by queue and user ID.
func (r *AssignmentRepository) GetByQueueAndUser(ctx context.Context, queueID, userID uuid.UUID) (*annotation.QueueAssignment, error) {
	var assignment annotation.QueueAssignment
	result := r.getDB(ctx).WithContext(ctx).
		Where("queue_id = ? AND user_id = ?", queueID.String(), userID.String()).
		First(&assignment)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, annotation.ErrAssignmentNotFound
		}
		return nil, result.Error
	}
	return &assignment, nil
}

// List retrieves all assignments for a queue.
func (r *AssignmentRepository) List(ctx context.Context, queueID uuid.UUID) ([]*annotation.QueueAssignment, error) {
	var assignments []*annotation.QueueAssignment
	result := r.getDB(ctx).WithContext(ctx).
		Where("queue_id = ?", queueID.String()).
		Order("assigned_at ASC").
		Find(&assignments)

	if result.Error != nil {
		return nil, result.Error
	}
	return assignments, nil
}

// ListByUser retrieves all queue assignments for a user.
func (r *AssignmentRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*annotation.QueueAssignment, error) {
	var assignments []*annotation.QueueAssignment
	result := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ?", userID.String()).
		Order("assigned_at DESC").
		Find(&assignments)

	if result.Error != nil {
		return nil, result.Error
	}
	return assignments, nil
}

// IsAssigned checks if a user is assigned to a queue.
func (r *AssignmentRepository) IsAssigned(ctx context.Context, queueID, userID uuid.UUID) (bool, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&annotation.QueueAssignment{}).
		Where("queue_id = ? AND user_id = ?", queueID.String(), userID.String()).
		Count(&count)

	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// HasRole checks if a user has a specific role (or higher) for a queue.
// Role hierarchy: admin > reviewer > annotator
func (r *AssignmentRepository) HasRole(ctx context.Context, queueID, userID uuid.UUID, minRole annotation.AssignmentRole) (bool, error) {
	var assignment annotation.QueueAssignment
	result := r.getDB(ctx).WithContext(ctx).
		Where("queue_id = ? AND user_id = ?", queueID.String(), userID.String()).
		First(&assignment)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, result.Error
	}

	return roleAtLeast(assignment.Role, minRole), nil
}

// roleAtLeast checks if the actual role meets or exceeds the minimum required role.
// Role hierarchy: admin > reviewer > annotator
func roleAtLeast(actual, minimum annotation.AssignmentRole) bool {
	roleLevel := map[annotation.AssignmentRole]int{
		annotation.RoleAnnotator: 1,
		annotation.RoleReviewer:  2,
		annotation.RoleAdmin:     3,
	}

	actualLevel, ok := roleLevel[actual]
	if !ok {
		return false
	}

	minLevel, ok := roleLevel[minimum]
	if !ok {
		return false
	}

	return actualLevel >= minLevel
}
