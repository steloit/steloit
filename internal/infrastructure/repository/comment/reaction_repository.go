package comment

import (
	"context"

	"github.com/google/uuid"

	"brokle/internal/core/domain/comment"
	"brokle/internal/core/domain/user"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

type ReactionRepository struct {
	db *gorm.DB
}

func NewReactionRepository(db *gorm.DB) *ReactionRepository {
	return &ReactionRepository{db: db}
}

func (r *ReactionRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *ReactionRepository) Toggle(ctx context.Context, commentID, userID uuid.UUID, emoji string) (bool, error) {
	// Check if reaction already exists
	var existing comment.Reaction
	result := r.getDB(ctx).WithContext(ctx).
		Where("comment_id = ? AND user_id = ? AND emoji = ?", commentID.String(), userID.String(), emoji).
		First(&existing)

	if result.Error == nil {
		// Reaction exists, remove it
		if err := r.getDB(ctx).WithContext(ctx).
			Where("comment_id = ? AND user_id = ? AND emoji = ?", commentID.String(), userID.String(), emoji).
			Delete(&comment.Reaction{}).Error; err != nil {
			return false, err
		}
		return false, nil // Removed
	}

	if result.Error != gorm.ErrRecordNotFound {
		return false, result.Error
	}

	// Reaction doesn't exist, add it
	reaction := comment.NewReaction(commentID, userID, emoji)
	if err := r.getDB(ctx).WithContext(ctx).Create(reaction).Error; err != nil {
		return false, err
	}
	return true, nil // Added
}

func (r *ReactionRepository) GetByComments(ctx context.Context, commentIDs []uuid.UUID, currentUserID *uuid.UUID) (map[string][]comment.ReactionSummary, error) {
	if len(commentIDs) == 0 {
		return make(map[string][]comment.ReactionSummary), nil
	}

	ids := make([]string, len(commentIDs))
	for i, id := range commentIDs {
		ids[i] = id.String()
	}

	var reactions []comment.Reaction
	if err := r.getDB(ctx).WithContext(ctx).
		Where("comment_id IN ?", ids).
		Order("created_at ASC").
		Find(&reactions).Error; err != nil {
		return nil, err
	}

	userIDMap := make(map[string]bool)
	for _, rx := range reactions {
		userIDMap[rx.UserID.String()] = true
	}

	userNames := make(map[string]string)
	if len(userIDMap) > 0 {
		userIDs := make([]string, 0, len(userIDMap))
		for id := range userIDMap {
			userIDs = append(userIDs, id)
		}

		var users []user.User
		if err := r.getDB(ctx).WithContext(ctx).
			Where("id IN ?", userIDs).
			Find(&users).Error; err != nil {
			return nil, err
		}

		for _, u := range users {
			userNames[u.ID.String()] = u.GetFullName()
		}
	}

	// Build result map: commentID -> emoji -> aggregation
	type emojiAgg struct {
		Count   int
		Users   []string
		HasUser bool
	}

	commentEmojis := make(map[string]map[string]*emojiAgg)
	for _, rx := range reactions {
		cid := rx.CommentID.String()
		if _, ok := commentEmojis[cid]; !ok {
			commentEmojis[cid] = make(map[string]*emojiAgg)
		}
		if _, ok := commentEmojis[cid][rx.Emoji]; !ok {
			commentEmojis[cid][rx.Emoji] = &emojiAgg{}
		}

		agg := commentEmojis[cid][rx.Emoji]
		agg.Count++
		agg.Users = append(agg.Users, userNames[rx.UserID.String()])

		if currentUserID != nil && rx.UserID == *currentUserID {
			agg.HasUser = true
		}
	}

	result := make(map[string][]comment.ReactionSummary)
	for cid, emojis := range commentEmojis {
		summaries := make([]comment.ReactionSummary, 0, len(emojis))
		for emoji, agg := range emojis {
			summaries = append(summaries, comment.ReactionSummary{
				Emoji:   emoji,
				Count:   agg.Count,
				Users:   agg.Users,
				HasUser: agg.HasUser,
			})
		}
		result[cid] = summaries
	}

	// Ensure all requested comment IDs have an entry (even if empty)
	for _, id := range ids {
		if _, ok := result[id]; !ok {
			result[id] = []comment.ReactionSummary{}
		}
	}

	return result, nil
}

func (r *ReactionRepository) GetByComment(ctx context.Context, commentID uuid.UUID, currentUserID *uuid.UUID) ([]comment.ReactionSummary, error) {
	result, err := r.GetByComments(ctx, []uuid.UUID{commentID}, currentUserID)
	if err != nil {
		return nil, err
	}

	summaries, ok := result[commentID.String()]
	if !ok {
		return []comment.ReactionSummary{}, nil
	}
	return summaries, nil
}

func (r *ReactionRepository) CountUniqueEmojis(ctx context.Context, commentID uuid.UUID) (int, error) {
	var count int64
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&comment.Reaction{}).
		Where("comment_id = ?", commentID.String()).
		Distinct("emoji").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *ReactionRepository) UserHasReacted(ctx context.Context, commentID, userID uuid.UUID, emoji string) (bool, error) {
	var count int64
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&comment.Reaction{}).
		Where("comment_id = ? AND user_id = ? AND emoji = ?", commentID.String(), userID.String(), emoji).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
