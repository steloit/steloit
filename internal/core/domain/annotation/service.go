package annotation

import (
	"context"

	"github.com/google/uuid"
)

// QueueService defines the interface for annotation queue business logic.
type QueueService interface {
	// Create creates a new annotation queue.
	Create(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *CreateQueueRequest) (*AnnotationQueue, error)

	// GetByID retrieves an annotation queue by its ID.
	GetByID(ctx context.Context, id, projectID uuid.UUID) (*AnnotationQueue, error)

	// List retrieves all annotation queues for a project with optional filtering and pagination.
	List(ctx context.Context, projectID uuid.UUID, filter *QueueFilter, page, limit int) ([]*AnnotationQueue, int64, error)

	// ListWithStats retrieves all annotation queues with their statistics and pagination.
	ListWithStats(ctx context.Context, projectID uuid.UUID, filter *QueueFilter, page, limit int) ([]*AnnotationQueue, []*QueueStats, int64, error)

	// Update updates an existing annotation queue.
	Update(ctx context.Context, id, projectID uuid.UUID, req *UpdateQueueRequest) (*AnnotationQueue, error)

	// Delete removes an annotation queue by ID.
	// Also deletes all items and assignments in the queue.
	Delete(ctx context.Context, id, projectID uuid.UUID) error

	// GetWithStats retrieves a queue with its statistics.
	GetWithStats(ctx context.Context, id, projectID uuid.UUID) (*AnnotationQueue, *QueueStats, error)
}

// ItemService defines the interface for queue item business logic.
type ItemService interface {
	// AddItems adds items to a queue.
	// Returns the count of items actually created (excluding duplicates).
	AddItems(ctx context.Context, queueID, projectID uuid.UUID, req *AddItemsBatchRequest) (int, error)

	// ListItems retrieves items in a queue with optional filtering and pagination.
	ListItems(ctx context.Context, queueID, projectID uuid.UUID, filter *ItemFilter) ([]*QueueItem, int64, error)

	// ClaimNext claims the next available item for the user to annotate.
	// Follows Langfuse pattern: finds first pending item where lock is available or reclaimable.
	// The seenItemIDs allows excluding items already shown to the user in the current session.
	ClaimNext(ctx context.Context, queueID, projectID, userID uuid.UUID, seenItemIDs []uuid.UUID) (*QueueItem, error)

	// Complete marks an item as completed and submits the scores.
	// The scores are validated against the queue's score_config_ids.
	Complete(ctx context.Context, itemID, queueID, projectID, userID uuid.UUID, req *CompleteItemRequest) error

	// Skip marks an item as skipped by the user.
	Skip(ctx context.Context, itemID, queueID, projectID, userID uuid.UUID, req *SkipItemRequest) error

	// ReleaseLock releases the lock on an item, returning it to the pending pool.
	ReleaseLock(ctx context.Context, itemID, queueID, projectID, userID uuid.UUID) error

	// DeleteItem removes an item from the queue.
	DeleteItem(ctx context.Context, itemID, queueID, projectID uuid.UUID) error

	// GetStats retrieves statistics for a queue.
	GetStats(ctx context.Context, queueID, projectID uuid.UUID) (*QueueStats, error)
}

// AssignmentService defines the interface for queue assignment business logic.
type AssignmentService interface {
	// Assign assigns a user to a queue with the specified role.
	Assign(ctx context.Context, queueID, projectID, userID uuid.UUID, role AssignmentRole, assignedBy *uuid.UUID) (*QueueAssignment, error)

	// Unassign removes a user's assignment from a queue.
	Unassign(ctx context.Context, queueID, projectID, userID uuid.UUID) error

	// ListAssignments retrieves all assignments for a queue.
	ListAssignments(ctx context.Context, queueID, projectID uuid.UUID) ([]*QueueAssignment, error)

	// GetUserQueues retrieves all queues a user is assigned to.
	GetUserQueues(ctx context.Context, userID uuid.UUID) ([]*QueueAssignment, error)

	// CheckAccess verifies if a user has access to a queue with the minimum required role.
	CheckAccess(ctx context.Context, queueID, userID uuid.UUID, minRole AssignmentRole) error
}
