package annotation

import (
	"context"

	"github.com/google/uuid"
)

// QueueRepository defines the interface for annotation queue persistence.
type QueueRepository interface {
	// Create creates a new annotation queue.
	Create(ctx context.Context, queue *AnnotationQueue) error

	// GetByID retrieves an annotation queue by its ID.
	GetByID(ctx context.Context, id, projectID uuid.UUID) (*AnnotationQueue, error)

	// GetByName retrieves an annotation queue by name within a project.
	GetByName(ctx context.Context, name string, projectID uuid.UUID) (*AnnotationQueue, error)

	// List retrieves all annotation queues for a project with optional filtering and pagination.
	List(ctx context.Context, projectID uuid.UUID, filter *QueueFilter, offset, limit int) ([]*AnnotationQueue, int64, error)

	// Update updates an existing annotation queue.
	Update(ctx context.Context, queue *AnnotationQueue) error

	// Delete removes an annotation queue by ID.
	Delete(ctx context.Context, id, projectID uuid.UUID) error

	// ExistsByName checks if a queue with the given name exists in the project.
	ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error)
}

// ItemRepository defines the interface for queue item persistence.
type ItemRepository interface {
	// Create creates a new queue item.
	Create(ctx context.Context, item *QueueItem) error

	// CreateBatch creates multiple queue items in a single operation.
	// Uses ON CONFLICT DO NOTHING to skip duplicates.
	// Returns the number of items actually inserted (excluding duplicates).
	CreateBatch(ctx context.Context, items []*QueueItem) (int64, error)

	// GetByID retrieves a queue item by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*QueueItem, error)

	// GetByIDForQueue retrieves a queue item by its ID within a specific queue.
	GetByIDForQueue(ctx context.Context, id, queueID uuid.UUID) (*QueueItem, error)

	// List retrieves queue items with optional filtering and pagination.
	List(ctx context.Context, queueID uuid.UUID, filter *ItemFilter) ([]*QueueItem, int64, error)

	// Update updates an existing queue item.
	Update(ctx context.Context, item *QueueItem) error

	// Delete removes a queue item by ID.
	Delete(ctx context.Context, id, queueID uuid.UUID) error

	// ExistsByObject checks if an item for the given object exists in the queue.
	ExistsByObject(ctx context.Context, queueID uuid.UUID, objectID string, objectType ObjectType) (bool, error)

	// FetchAndLockNext finds and locks the next available item for annotation.
	// Follows Langfuse pattern: finds first pending item where:
	// - Never locked, OR
	// - Lock expired, OR
	// - Locked by current user (can reclaim)
	// Uses SELECT ... FOR UPDATE SKIP LOCKED for concurrent safety.
	// The seenItemIDs parameter allows excluding items already shown to the user.
	FetchAndLockNext(ctx context.Context, queueID, userID uuid.UUID, lockTimeout int, seenItemIDs []uuid.UUID) (*QueueItem, error)

	// Complete marks an item as completed by the annotator.
	// Sets annotator_user_id and completed_at.
	Complete(ctx context.Context, id, userID uuid.UUID) error

	// Skip marks an item as skipped by the annotator.
	Skip(ctx context.Context, id, userID uuid.UUID) error

	// ReleaseLock releases the lock on an item.
	ReleaseLock(ctx context.Context, id uuid.UUID) error

	// ReleaseExpiredLocks releases all locks that have expired.
	// Used by the background worker for lock expiry.
	ReleaseExpiredLocks(ctx context.Context, queueID uuid.UUID, lockTimeout int) (int64, error)

	// GetStats retrieves aggregated statistics for a queue.
	GetStats(ctx context.Context, queueID uuid.UUID, lockTimeout int) (*QueueStats, error)
}

// AssignmentRepository defines the interface for queue assignment persistence.
type AssignmentRepository interface {
	// Create creates a new queue assignment.
	Create(ctx context.Context, assignment *QueueAssignment) error

	// Delete removes a queue assignment by queue and user ID.
	Delete(ctx context.Context, queueID, userID uuid.UUID) error

	// GetByQueueAndUser retrieves an assignment by queue and user ID.
	GetByQueueAndUser(ctx context.Context, queueID, userID uuid.UUID) (*QueueAssignment, error)

	// List retrieves all assignments for a queue.
	List(ctx context.Context, queueID uuid.UUID) ([]*QueueAssignment, error)

	// ListByUser retrieves all queue assignments for a user.
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*QueueAssignment, error)

	// IsAssigned checks if a user is assigned to a queue.
	IsAssigned(ctx context.Context, queueID, userID uuid.UUID) (bool, error)

	// HasRole checks if a user has a specific role (or higher) for a queue.
	HasRole(ctx context.Context, queueID, userID uuid.UUID, minRole AssignmentRole) (bool, error)
}
