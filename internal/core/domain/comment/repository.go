package comment

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, comment *Comment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Comment, error)
	GetByIDWithUser(ctx context.Context, id uuid.UUID) (*CommentWithUser, error)
	Update(ctx context.Context, comment *Comment) error
	Delete(ctx context.Context, id uuid.UUID) error

	// ListByEntity returns top-level comments (parent_id IS NULL) ordered by created_at ascending.
	ListByEntity(ctx context.Context, entityType EntityType, entityID string, projectID uuid.UUID) ([]*CommentWithUser, error)

	// ListReplies returns a map of parent_id -> replies.
	ListReplies(ctx context.Context, parentIDs []uuid.UUID) (map[string][]*CommentWithUser, error)

	// CountReplies returns a map of parent_id -> reply count.
	CountReplies(ctx context.Context, parentIDs []uuid.UUID) (map[string]int, error)

	// CountByEntity returns count of non-deleted comments (including replies).
	CountByEntity(ctx context.Context, entityType EntityType, entityID string, projectID uuid.UUID) (int64, error)

	HasActiveReplies(ctx context.Context, parentID uuid.UUID) (bool, error)
}
