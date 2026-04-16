package comment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/comment"
	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
)

type commentService struct {
	commentRepo  comment.Repository
	reactionRepo comment.ReactionRepository
	traceRepo    observability.TraceRepository
	logger       *slog.Logger
}

func NewCommentService(
	commentRepo comment.Repository,
	reactionRepo comment.ReactionRepository,
	traceRepo observability.TraceRepository,
	logger *slog.Logger,
) comment.Service {
	return &commentService{
		commentRepo:  commentRepo,
		reactionRepo: reactionRepo,
		traceRepo:    traceRepo,
		logger:       logger,
	}
}

func (s *commentService) CreateComment(ctx context.Context, projectID uuid.UUID, traceID string, userID uuid.UUID, req *comment.CreateCommentRequest) (*comment.CommentResponse, error) {
	if err := s.validateTraceOwnership(ctx, traceID, projectID); err != nil {
		return nil, err
	}

	c := comment.NewComment(comment.EntityTypeTrace, traceID, projectID, userID, req.Content)

	if err := s.commentRepo.Create(ctx, c); err != nil {
		return nil, appErrors.NewInternalError("failed to create comment", err)
	}

	cwu, err := s.commentRepo.GetByIDWithUser(ctx, c.ID)
	if err != nil {
		// Graceful degradation: return basic comment if user info fetch fails
		s.logger.Warn("failed to get comment with user info after create",
			"comment_id", c.ID,
			"error", err,
		)
		return c.ToResponse(), nil
	}

	s.logger.Info("comment created",
		"comment_id", c.ID,
		"trace_id", traceID,
		"project_id", projectID,
		"user_id", userID,
	)

	return cwu.ToResponse(), nil
}

func (s *commentService) UpdateComment(ctx context.Context, projectID uuid.UUID, traceID string, commentID, userID uuid.UUID, req *comment.UpdateCommentRequest) (*comment.CommentResponse, error) {
	if err := s.validateTraceOwnership(ctx, traceID, projectID); err != nil {
		return nil, err
	}

	c, err := s.commentRepo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
		}
		return nil, appErrors.NewInternalError("failed to get comment", err)
	}

	if c.EntityID != traceID || c.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
	}

	if c.CreatedBy == nil || *c.CreatedBy != userID {
		return nil, appErrors.NewForbiddenError("you can only edit your own comments")
	}

	c.Content = req.Content
	c.UpdatedBy = &userID
	c.UpdatedAt = time.Now()

	if err := s.commentRepo.Update(ctx, c); err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
		}
		return nil, appErrors.NewInternalError("failed to update comment", err)
	}

	cwu, err := s.commentRepo.GetByIDWithUser(ctx, c.ID)
	if err != nil {
		s.logger.Warn("failed to get comment with user info after update",
			"comment_id", c.ID,
			"error", err,
		)
		return c.ToResponse(), nil
	}

	s.logger.Info("comment updated",
		"comment_id", commentID,
		"trace_id", traceID,
		"project_id", projectID,
		"user_id", userID,
	)

	return cwu.ToResponse(), nil
}

func (s *commentService) DeleteComment(ctx context.Context, projectID uuid.UUID, traceID string, commentID, userID uuid.UUID) error {
	if err := s.validateTraceOwnership(ctx, traceID, projectID); err != nil {
		return err
	}

	c, err := s.commentRepo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
		}
		return appErrors.NewInternalError("failed to get comment", err)
	}

	if c.EntityID != traceID || c.ProjectID != projectID {
		return appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
	}

	if c.CreatedBy == nil || *c.CreatedBy != userID {
		return appErrors.NewForbiddenError("you can only delete your own comments")
	}

	// Tombstone pattern: soft-deleted parents with active replies remain visible as "[deleted]"

	if err := s.commentRepo.Delete(ctx, commentID); err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
		}
		return appErrors.NewInternalError("failed to delete comment", err)
	}

	s.logger.Info("comment deleted",
		"comment_id", commentID,
		"trace_id", traceID,
		"project_id", projectID,
		"user_id", userID,
	)

	return nil
}

func (s *commentService) ListComments(ctx context.Context, projectID uuid.UUID, traceID string, currentUserID *uuid.UUID) (*comment.ListCommentsResponse, error) {
	topLevelComments, err := s.commentRepo.ListByEntity(ctx, comment.EntityTypeTrace, traceID, projectID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list comments", err)
	}

	if len(topLevelComments) == 0 {
		return &comment.ListCommentsResponse{
			Comments: []*comment.CommentResponse{},
			Total:    0,
		}, nil
	}

	topLevelIDs := make([]uuid.UUID, len(topLevelComments))
	for i, c := range topLevelComments {
		topLevelIDs[i] = c.ID
	}

	repliesMap, err := s.commentRepo.ListReplies(ctx, topLevelIDs)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list replies", err)
	}

	replyCounts, err := s.commentRepo.CountReplies(ctx, topLevelIDs)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to count replies", err)
	}

	allCommentIDs := make([]uuid.UUID, 0, len(topLevelComments))
	allCommentIDs = append(allCommentIDs, topLevelIDs...)
	for _, replies := range repliesMap {
		for _, reply := range replies {
			allCommentIDs = append(allCommentIDs, reply.ID)
		}
	}

	reactionsMap, err := s.reactionRepo.GetByComments(ctx, allCommentIDs, currentUserID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get reactions", err)
	}

	responses := make([]*comment.CommentResponse, len(topLevelComments))
	totalCount := int64(0)

	for i, c := range topLevelComments {
		resp := c.ToResponse()

		// Exclude tombstones from count so badge matches list total
		if !c.IsSoftDeleted() {
			totalCount++
		}

		if reactions, ok := reactionsMap[c.ID.String()]; ok {
			resp.Reactions = reactions
		}

		if count, ok := replyCounts[c.ID.String()]; ok {
			resp.ReplyCount = count
			totalCount += int64(count)
		}

		if replies, ok := repliesMap[c.ID.String()]; ok && len(replies) > 0 {
			replyResponses := make([]*comment.CommentResponse, len(replies))
			for j, reply := range replies {
				replyResp := reply.ToResponse()
				if replyReactions, ok := reactionsMap[reply.ID.String()]; ok {
					replyResp.Reactions = replyReactions
				}
				replyResponses[j] = replyResp
			}
			resp.Replies = replyResponses
		}

		responses[i] = resp
	}

	return &comment.ListCommentsResponse{
		Comments: responses,
		Total:    totalCount,
	}, nil
}

func (s *commentService) GetCommentCount(ctx context.Context, projectID uuid.UUID, traceID string) (*comment.CommentCountResponse, error) {
	count, err := s.commentRepo.CountByEntity(ctx, comment.EntityTypeTrace, traceID, projectID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to count comments", err)
	}

	return &comment.CommentCountResponse{
		Count: count,
	}, nil
}

func (s *commentService) ToggleReaction(ctx context.Context, projectID uuid.UUID, traceID string, commentID, userID uuid.UUID, req *comment.ToggleReactionRequest) ([]comment.ReactionSummary, error) {
	if err := s.validateTraceOwnership(ctx, traceID, projectID); err != nil {
		return nil, err
	}

	c, err := s.commentRepo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
		}
		return nil, appErrors.NewInternalError("failed to get comment", err)
	}

	if c.EntityID != traceID || c.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("comment %s", commentID))
	}

	hasReacted, err := s.reactionRepo.UserHasReacted(ctx, commentID, userID, req.Emoji)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to check reaction", err)
	}

	// Check max emoji limit when adding a new emoji type
	if !hasReacted {
		uniqueCount, err := s.reactionRepo.CountUniqueEmojis(ctx, commentID)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to count emojis", err)
		}

		summaries, err := s.reactionRepo.GetByComment(ctx, commentID, nil)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to get reactions", err)
		}

		emojiExists := false
		for _, summary := range summaries {
			if summary.Emoji == req.Emoji {
				emojiExists = true
				break
			}
		}

		if !emojiExists && uniqueCount >= comment.MaxEmojisPerComment {
			return nil, appErrors.NewValidationError("emoji", fmt.Sprintf("maximum %d emoji types per comment", comment.MaxEmojisPerComment))
		}
	}

	added, err := s.reactionRepo.Toggle(ctx, commentID, userID, req.Emoji)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to toggle reaction", err)
	}

	action := "removed"
	if added {
		action = "added"
	}

	s.logger.Info("reaction toggled",
		"comment_id", commentID,
		"trace_id", traceID,
		"user_id", userID,
		"emoji", req.Emoji,
		"action", action,
	)

	summaries, err := s.reactionRepo.GetByComment(ctx, commentID, &userID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get reactions", err)
	}

	return summaries, nil
}

func (s *commentService) CreateReply(ctx context.Context, projectID uuid.UUID, traceID string, parentID, userID uuid.UUID, req *comment.CreateCommentRequest) (*comment.CommentResponse, error) {
	if err := s.validateTraceOwnership(ctx, traceID, projectID); err != nil {
		return nil, err
	}

	parent, err := s.commentRepo.GetByID(ctx, parentID)
	if err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("comment %s", parentID))
		}
		return nil, appErrors.NewInternalError("failed to get parent comment", err)
	}

	if parent.EntityID != traceID || parent.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("comment %s", parentID))
	}

	if parent.IsReply() {
		return nil, appErrors.NewValidationError("parent_id", "cannot reply to a reply")
	}

	c := comment.NewReplyComment(comment.EntityTypeTrace, traceID, projectID, parentID, userID, req.Content)

	if err := s.commentRepo.Create(ctx, c); err != nil {
		return nil, appErrors.NewInternalError("failed to create reply", err)
	}

	cwu, err := s.commentRepo.GetByIDWithUser(ctx, c.ID)
	if err != nil {
		s.logger.Warn("failed to get reply with user info after create",
			"comment_id", c.ID,
			"error", err,
		)
		return c.ToResponse(), nil
	}

	s.logger.Info("reply created",
		"comment_id", c.ID,
		"parent_id", parentID,
		"trace_id", traceID,
		"project_id", projectID,
		"user_id", userID,
	)

	return cwu.ToResponse(), nil
}

func (s *commentService) validateTraceOwnership(ctx context.Context, traceID string, projectID uuid.UUID) error {
	_, err := s.traceRepo.GetRootSpanByProject(ctx, traceID, projectID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appErrors.NewNotFoundError(fmt.Sprintf("trace %s", traceID))
		}
		return appErrors.NewInternalError("failed to validate trace ownership", err)
	}
	return nil
}
