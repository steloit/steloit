package comment

import (
	"context"

	"github.com/google/uuid"
)

type Service interface {
	CreateComment(ctx context.Context, projectID uuid.UUID, traceID string, userID uuid.UUID, req *CreateCommentRequest) (*CommentResponse, error)

	// UpdateComment - only the comment owner can update their comment.
	UpdateComment(ctx context.Context, projectID uuid.UUID, traceID string, commentID, userID uuid.UUID, req *UpdateCommentRequest) (*CommentResponse, error)

	// DeleteComment - only the comment owner can delete their comment.
	DeleteComment(ctx context.Context, projectID uuid.UUID, traceID string, commentID, userID uuid.UUID) error

	// ListComments returns comments ordered by created_at ascending.
	// currentUserID is used to set the HasUser flag on reaction summaries.
	ListComments(ctx context.Context, projectID uuid.UUID, traceID string, currentUserID *uuid.UUID) (*ListCommentsResponse, error)

	GetCommentCount(ctx context.Context, projectID uuid.UUID, traceID string) (*CommentCountResponse, error)

	// ToggleReaction enforces max 6 unique emoji types per comment.
	ToggleReaction(ctx context.Context, projectID uuid.UUID, traceID string, commentID, userID uuid.UUID, req *ToggleReactionRequest) ([]ReactionSummary, error)

	// CreateReply - replies cannot have replies (one level deep only).
	CreateReply(ctx context.Context, projectID uuid.UUID, traceID string, parentID, userID uuid.UUID, req *CreateCommentRequest) (*CommentResponse, error)
}
