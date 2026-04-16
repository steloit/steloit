package comment

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"

	"gorm.io/gorm"
)

type EntityType string

const (
	EntityTypeTrace EntityType = "trace"
	EntityTypeSpan  EntityType = "span" // Future: span-level comments
)

func (e EntityType) IsValid() bool {
	switch e {
	case EntityTypeTrace, EntityTypeSpan:
		return true
	default:
		return false
	}
}

type Comment struct {
	ID         uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	EntityType EntityType     `json:"entity_type" gorm:"type:comment_entity_type;not null;default:'trace'"`
	EntityID   string         `json:"entity_id" gorm:"type:varchar(64);not null"` // trace_id or span_id
	ProjectID  uuid.UUID      `json:"project_id" gorm:"type:uuid;not null"`
	ParentID   *uuid.UUID     `json:"parent_id,omitempty" gorm:"type:uuid"` // For reply threading (one level deep)
	Content    string         `json:"content" gorm:"type:text;not null"`
	CreatedBy  *uuid.UUID     `json:"created_by" gorm:"type:uuid"`
	UpdatedBy  *uuid.UUID     `json:"updated_by,omitempty" gorm:"type:uuid"`
	CreatedAt  time.Time      `json:"created_at" gorm:"not null;autoCreateTime"`
	UpdatedAt  time.Time      `json:"updated_at" gorm:"not null;autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Comment) TableName() string {
	return "trace_comments"
}

func NewComment(entityType EntityType, entityID string, projectID, createdBy uuid.UUID, content string) *Comment {
	now := time.Now()
	return &Comment{
		ID:         uid.New(),
		EntityType: entityType,
		EntityID:   entityID,
		ProjectID:  projectID,
		Content:    content,
		CreatedBy:  &createdBy,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func NewReplyComment(entityType EntityType, entityID string, projectID, parentID, createdBy uuid.UUID, content string) *Comment {
	now := time.Now()
	return &Comment{
		ID:         uid.New(),
		EntityType: entityType,
		EntityID:   entityID,
		ProjectID:  projectID,
		ParentID:   &parentID,
		Content:    content,
		CreatedBy:  &createdBy,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func (c *Comment) IsReply() bool {
	return c.ParentID != nil
}

func (c *Comment) IsEdited() bool {
	return c.UpdatedBy != nil
}

func (c *Comment) IsSoftDeleted() bool {
	return c.DeletedAt.Valid
}

func (c *Comment) getCreatedByString() string {
	if c.CreatedBy == nil {
		return ""
	}
	return c.CreatedBy.String()
}

type CommentUser struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
}

type CreateCommentRequest struct {
	Content string `json:"content" binding:"required,min=1,max=10000"`
}

type UpdateCommentRequest struct {
	Content string `json:"content" binding:"required,min=1,max=10000"`
}

type CommentResponse struct {
	ID         string             `json:"id"`
	EntityType EntityType         `json:"entity_type"`
	EntityID   string             `json:"entity_id"`
	ProjectID  string             `json:"project_id"`
	ParentID   *string            `json:"parent_id,omitempty"`
	Content    string             `json:"content"`
	CreatedBy  string             `json:"created_by"`
	UpdatedBy  *string            `json:"updated_by,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
	IsEdited   bool               `json:"is_edited"`
	IsDeleted  bool               `json:"is_deleted"` // True if this is a tombstone (soft-deleted with active replies)
	Author     *CommentUser       `json:"author,omitempty"`
	Editor     *CommentUser       `json:"editor,omitempty"`
	Reactions  []ReactionSummary  `json:"reactions"`
	Replies    []*CommentResponse `json:"replies,omitempty"`
	ReplyCount int                `json:"reply_count"`
}

func (c *Comment) ToResponse() *CommentResponse {
	var updatedBy *string
	if c.UpdatedBy != nil {
		id := c.UpdatedBy.String()
		updatedBy = &id
	}

	var parentID *string
	if c.ParentID != nil {
		id := c.ParentID.String()
		parentID = &id
	}

	// For tombstones (soft-deleted comments), hide the content
	content := c.Content
	isDeleted := c.IsSoftDeleted()
	if isDeleted {
		content = "[deleted]"
	}

	return &CommentResponse{
		ID:         c.ID.String(),
		EntityType: c.EntityType,
		EntityID:   c.EntityID,
		ProjectID:  c.ProjectID.String(),
		ParentID:   parentID,
		Content:    content,
		CreatedBy:  c.getCreatedByString(),
		UpdatedBy:  updatedBy,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
		IsEdited:   c.IsEdited(),
		IsDeleted:  isDeleted,
		Reactions:  []ReactionSummary{}, // Initialize as empty slice
		Replies:    nil,
		ReplyCount: 0,
	}
}

type CommentWithUser struct {
	Comment
	Author *CommentUser `json:"author,omitempty" gorm:"-"`
	Editor *CommentUser `json:"editor,omitempty" gorm:"-"`
}

func (c *CommentWithUser) ToResponse() *CommentResponse {
	resp := c.Comment.ToResponse()
	// For tombstones (soft-deleted comments), hide author info
	if !c.IsSoftDeleted() {
		resp.Author = c.Author
		resp.Editor = c.Editor
	}
	return resp
}

type CommentCountResponse struct {
	Count int64 `json:"count"`
}

type ListCommentsResponse struct {
	Comments []*CommentResponse `json:"comments"`
	Total    int64              `json:"total"`
}
