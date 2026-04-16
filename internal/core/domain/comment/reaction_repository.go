package comment

import (
	"context"

	"github.com/google/uuid"
)

type ReactionRepository interface {
	// Toggle adds a reaction if it doesn't exist, removes it if it does.
	// Returns true if added, false if removed.
	Toggle(ctx context.Context, commentID, userID uuid.UUID, emoji string) (added bool, err error)

	// GetByComments retrieves reaction summaries for multiple comments.
	// If currentUserID is provided, sets HasUser flag for emojis the user has reacted with.
	GetByComments(ctx context.Context, commentIDs []uuid.UUID, currentUserID *uuid.UUID) (map[string][]ReactionSummary, error)

	GetByComment(ctx context.Context, commentID uuid.UUID, currentUserID *uuid.UUID) ([]ReactionSummary, error)

	// CountUniqueEmojis is used to enforce the max 6 emojis per comment constraint.
	CountUniqueEmojis(ctx context.Context, commentID uuid.UUID) (int, error)

	UserHasReacted(ctx context.Context, commentID, userID uuid.UUID, emoji string) (bool, error)
}
