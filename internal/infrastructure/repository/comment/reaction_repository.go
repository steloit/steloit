package comment

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	commentDomain "brokle/internal/core/domain/comment"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type ReactionRepository struct {
	tm *db.TxManager
}

func NewReactionRepository(tm *db.TxManager) *ReactionRepository {
	return &ReactionRepository{tm: tm}
}

// Toggle adds the reaction if absent, removes it if present. Returns
// true when the reaction was added.
func (r *ReactionRepository) Toggle(ctx context.Context, commentID, userID uuid.UUID, emoji string) (bool, error) {
	q := r.tm.Queries(ctx)
	_, err := q.GetCommentReactionByUserEmoji(ctx, gen.GetCommentReactionByUserEmojiParams{
		CommentID: commentID,
		UserID:    userID,
		Emoji:     emoji,
	})
	if err == nil {
		if _, err := q.DeleteCommentReaction(ctx, gen.DeleteCommentReactionParams{
			CommentID: commentID,
			UserID:    userID,
			Emoji:     emoji,
		}); err != nil {
			return false, fmt.Errorf("remove reaction: %w", err)
		}
		return false, nil
	}
	if !db.IsNoRows(err) {
		return false, fmt.Errorf("lookup reaction: %w", err)
	}
	reaction := commentDomain.NewReaction(commentID, userID, emoji)
	if err := q.CreateCommentReaction(ctx, gen.CreateCommentReactionParams{
		ID:        reaction.ID,
		CommentID: reaction.CommentID,
		UserID:    reaction.UserID,
		Emoji:     reaction.Emoji,
	}); err != nil {
		return false, fmt.Errorf("add reaction: %w", err)
	}
	return true, nil
}

func (r *ReactionRepository) GetByComments(ctx context.Context, commentIDs []uuid.UUID, currentUserID *uuid.UUID) (map[string][]commentDomain.ReactionSummary, error) {
	out := make(map[string][]commentDomain.ReactionSummary, len(commentIDs))
	for _, id := range commentIDs {
		out[id.String()] = []commentDomain.ReactionSummary{}
	}
	if len(commentIDs) == 0 {
		return out, nil
	}
	reactions, err := r.tm.Queries(ctx).ListCommentReactionsByComments(ctx, commentIDs)
	if err != nil {
		return nil, fmt.Errorf("list reactions: %w", err)
	}

	// Collect distinct user IDs for name lookup.
	userSet := make(map[uuid.UUID]struct{}, len(reactions))
	for _, rx := range reactions {
		userSet[rx.UserID] = struct{}{}
	}
	userIDs := make([]uuid.UUID, 0, len(userSet))
	for id := range userSet {
		userIDs = append(userIDs, id)
	}
	names := make(map[uuid.UUID]string, len(userIDs))
	if len(userIDs) > 0 {
		users, err := r.tm.Queries(ctx).ListUsersForCommentEnrichment(ctx, userIDs)
		if err != nil {
			return nil, fmt.Errorf("load reaction users: %w", err)
		}
		for _, u := range users {
			names[u.ID] = u.FirstName + " " + u.LastName
		}
	}

	// Aggregate: comment -> emoji -> { count, users, hasUser }.
	type agg struct {
		count   int
		users   []string
		hasUser bool
	}
	byComment := make(map[string]map[string]*agg, len(commentIDs))
	for _, rx := range reactions {
		cid := rx.CommentID.String()
		if _, ok := byComment[cid]; !ok {
			byComment[cid] = make(map[string]*agg)
		}
		a, ok := byComment[cid][rx.Emoji]
		if !ok {
			a = &agg{}
			byComment[cid][rx.Emoji] = a
		}
		a.count++
		a.users = append(a.users, names[rx.UserID])
		if currentUserID != nil && rx.UserID == *currentUserID {
			a.hasUser = true
		}
	}
	for cid, emojis := range byComment {
		summaries := make([]commentDomain.ReactionSummary, 0, len(emojis))
		for emoji, a := range emojis {
			summaries = append(summaries, commentDomain.ReactionSummary{
				Emoji:   emoji,
				Count:   a.count,
				Users:   a.users,
				HasUser: a.hasUser,
			})
		}
		out[cid] = summaries
	}
	return out, nil
}

func (r *ReactionRepository) GetByComment(ctx context.Context, commentID uuid.UUID, currentUserID *uuid.UUID) ([]commentDomain.ReactionSummary, error) {
	result, err := r.GetByComments(ctx, []uuid.UUID{commentID}, currentUserID)
	if err != nil {
		return nil, err
	}
	return result[commentID.String()], nil
}

func (r *ReactionRepository) CountUniqueEmojis(ctx context.Context, commentID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountDistinctEmojisOnComment(ctx, commentID)
	if err != nil {
		return 0, fmt.Errorf("count distinct emojis: %w", err)
	}
	return int(n), nil
}

func (r *ReactionRepository) UserHasReacted(ctx context.Context, commentID, userID uuid.UUID, emoji string) (bool, error) {
	return r.tm.Queries(ctx).CommentReactionExists(ctx, gen.CommentReactionExistsParams{
		CommentID: commentID,
		UserID:    userID,
		Emoji:     emoji,
	})
}
