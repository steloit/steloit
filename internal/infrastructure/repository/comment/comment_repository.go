package comment

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"brokle/internal/core/domain/comment"
	"brokle/internal/core/domain/user"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

type CommentRepository struct {
	db *gorm.DB
}

func NewCommentRepository(db *gorm.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

func (r *CommentRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *CommentRepository) Create(ctx context.Context, c *comment.Comment) error {
	result := r.getDB(ctx).WithContext(ctx).Create(c)
	return result.Error
}

func (r *CommentRepository) GetByID(ctx context.Context, id uuid.UUID) (*comment.Comment, error) {
	var c comment.Comment
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id.String()).
		First(&c)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, comment.ErrNotFound
		}
		return nil, result.Error
	}
	return &c, nil
}

func (r *CommentRepository) GetByIDWithUser(ctx context.Context, id uuid.UUID) (*comment.CommentWithUser, error) {
	var c comment.Comment
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id.String()).
		First(&c)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, comment.ErrNotFound
		}
		return nil, result.Error
	}

	var author *comment.CommentUser
	if c.CreatedBy != nil {
		var err error
		author, err = r.getUserByID(ctx, *c.CreatedBy)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	var editor *comment.CommentUser
	if c.UpdatedBy != nil {
		var err error
		editor, err = r.getUserByID(ctx, *c.UpdatedBy)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	return &comment.CommentWithUser{
		Comment: c,
		Author:  author,
		Editor:  editor,
	}, nil
}

func (r *CommentRepository) Update(ctx context.Context, c *comment.Comment) error {
	result := r.getDB(ctx).WithContext(ctx).Save(c)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return comment.ErrNotFound
	}
	return nil
}

func (r *CommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id.String()).
		Delete(&comment.Comment{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return comment.ErrNotFound
	}
	return nil
}

func (r *CommentRepository) HasActiveReplies(ctx context.Context, parentID uuid.UUID) (bool, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&comment.Comment{}).
		Where("parent_id = ? AND deleted_at IS NULL", parentID.String()).
		Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// ListByEntity implements tombstone pattern: returns non-deleted comments OR deleted with active replies.
func (r *CommentRepository) ListByEntity(ctx context.Context, entityType comment.EntityType, entityID string, projectID uuid.UUID) ([]*comment.CommentWithUser, error) {
	var comments []comment.Comment

	// Subquery: parent IDs with active replies
	subquery := r.getDB(ctx).WithContext(ctx).
		Model(&comment.Comment{}).
		Select("DISTINCT parent_id").
		Where("entity_type = ? AND entity_id = ? AND project_id = ?", string(entityType), entityID, projectID.String()).
		Where("parent_id IS NOT NULL AND deleted_at IS NULL")

	result := r.getDB(ctx).WithContext(ctx).Unscoped().
		Where("entity_type = ? AND entity_id = ? AND project_id = ? AND parent_id IS NULL", string(entityType), entityID, projectID.String()).
		Where("deleted_at IS NULL OR id IN (?)", subquery).
		Order("created_at ASC").
		Find(&comments)

	if result.Error != nil {
		return nil, result.Error
	}

	userIDMap := make(map[string]bool)
	for _, c := range comments {
		if c.CreatedBy != nil {
			userIDMap[c.CreatedBy.String()] = true
		}
		if c.UpdatedBy != nil {
			userIDMap[c.UpdatedBy.String()] = true
		}
	}

	userIDs := make([]string, 0, len(userIDMap))
	for id := range userIDMap {
		userIDs = append(userIDs, id)
	}

	userMap := make(map[string]*comment.CommentUser)
	if len(userIDs) > 0 {
		var users []user.User
		if err := r.getDB(ctx).WithContext(ctx).
			Where("id IN ?", userIDs).
			Find(&users).Error; err != nil {
			return nil, err
		}
		for _, u := range users {
			userMap[u.ID.String()] = &comment.CommentUser{
				ID:    u.ID,
				Name:  u.GetFullName(),
				Email: u.Email,
			}
		}

		var profiles []user.UserProfile
		if err := r.getDB(ctx).WithContext(ctx).
			Where("user_id IN ?", userIDs).
			Find(&profiles).Error; err != nil {
			return nil, err
		}
		for _, p := range profiles {
			if cu, ok := userMap[p.UserID.String()]; ok {
				cu.AvatarURL = p.AvatarURL
			}
		}
	}

	result2 := make([]*comment.CommentWithUser, len(comments))
	for i, c := range comments {
		cwu := &comment.CommentWithUser{
			Comment: c,
		}
		if c.CreatedBy != nil {
			cwu.Author = userMap[c.CreatedBy.String()]
		}
		if c.UpdatedBy != nil {
			cwu.Editor = userMap[c.UpdatedBy.String()]
		}
		result2[i] = cwu
	}

	return result2, nil
}

func (r *CommentRepository) CountByEntity(ctx context.Context, entityType comment.EntityType, entityID string, projectID uuid.UUID) (int64, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&comment.Comment{}).
		Where("entity_type = ? AND entity_id = ? AND project_id = ? AND deleted_at IS NULL", string(entityType), entityID, projectID.String()).
		Count(&count)

	if result.Error != nil {
		return 0, result.Error
	}
	return count, nil
}

func (r *CommentRepository) ListReplies(ctx context.Context, parentIDs []uuid.UUID) (map[string][]*comment.CommentWithUser, error) {
	if len(parentIDs) == 0 {
		return make(map[string][]*comment.CommentWithUser), nil
	}

	ids := make([]string, len(parentIDs))
	for i, id := range parentIDs {
		ids[i] = id.String()
	}

	var replies []comment.Comment
	if err := r.getDB(ctx).WithContext(ctx).
		Where("parent_id IN ? AND deleted_at IS NULL", ids).
		Order("created_at ASC").
		Find(&replies).Error; err != nil {
		return nil, err
	}

	userIDMap := make(map[string]bool)
	for _, c := range replies {
		if c.CreatedBy != nil {
			userIDMap[c.CreatedBy.String()] = true
		}
		if c.UpdatedBy != nil {
			userIDMap[c.UpdatedBy.String()] = true
		}
	}

	userMap := make(map[string]*comment.CommentUser)
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
			userMap[u.ID.String()] = &comment.CommentUser{
				ID:    u.ID,
				Name:  u.GetFullName(),
				Email: u.Email,
			}
		}

		var profiles []user.UserProfile
		if err := r.getDB(ctx).WithContext(ctx).
			Where("user_id IN ?", userIDs).
			Find(&profiles).Error; err != nil {
			return nil, err
		}
		for _, p := range profiles {
			if cu, ok := userMap[p.UserID.String()]; ok {
				cu.AvatarURL = p.AvatarURL
			}
		}
	}

	result := make(map[string][]*comment.CommentWithUser)
	for _, c := range replies {
		parentID := c.ParentID.String()
		cwu := &comment.CommentWithUser{
			Comment: c,
		}
		if c.CreatedBy != nil {
			cwu.Author = userMap[c.CreatedBy.String()]
		}
		if c.UpdatedBy != nil {
			cwu.Editor = userMap[c.UpdatedBy.String()]
		}
		result[parentID] = append(result[parentID], cwu)
	}

	// Ensure all requested IDs have an entry
	for _, id := range ids {
		if _, ok := result[id]; !ok {
			result[id] = []*comment.CommentWithUser{}
		}
	}

	return result, nil
}

func (r *CommentRepository) CountReplies(ctx context.Context, parentIDs []uuid.UUID) (map[string]int, error) {
	if len(parentIDs) == 0 {
		return make(map[string]int), nil
	}

	ids := make([]string, len(parentIDs))
	for i, id := range parentIDs {
		ids[i] = id.String()
	}

	type replyCount struct {
		ParentID string `gorm:"column:parent_id"`
		Count    int    `gorm:"column:count"`
	}

	var counts []replyCount
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&comment.Comment{}).
		Select("parent_id, COUNT(*) as count").
		Where("parent_id IN ? AND deleted_at IS NULL", ids).
		Group("parent_id").
		Scan(&counts).Error; err != nil {
		return nil, err
	}

	result := make(map[string]int)
	for _, c := range counts {
		result[c.ParentID] = c.Count
	}

	// Ensure all requested IDs have an entry
	for _, id := range ids {
		if _, ok := result[id]; !ok {
			result[id] = 0
		}
	}

	return result, nil
}

func (r *CommentRepository) getUserByID(ctx context.Context, id uuid.UUID) (*comment.CommentUser, error) {
	var u user.User
	if err := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id.String()).
		First(&u).Error; err != nil {
		return nil, err
	}

	cu := &comment.CommentUser{
		ID:    u.ID,
		Name:  u.GetFullName(),
		Email: u.Email,
	}

	var profile user.UserProfile
	if err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ?", id.String()).
		First(&profile).Error; err == nil {
		cu.AvatarURL = profile.AvatarURL
	}

	return cu, nil
}
