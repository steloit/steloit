package comment

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
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
	ID         uuid.UUID      `json:"id"`
	EntityType EntityType     `json:"entity_type"`
	EntityID   string         `json:"entity_id"` // trace_id or span_id
	ProjectID  uuid.UUID      `json:"project_id"`
	ParentID   *uuid.UUID     `json:"parent_id,omitempty"` // For reply threading (one level deep)
	Content    string         `json:"content"`
	CreatedBy  *uuid.UUID     `json:"created_by"`
	UpdatedBy  *uuid.UUID     `json:"updated_by,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  *time.Time     `json:"-"`
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
	return c.DeletedAt != nil
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
	ID         uuid.UUID          `json:"id"`
	EntityType EntityType         `json:"entity_type"`
	EntityID   string             `json:"entity_id"` // trace_id or span_id — W3C hex, not UUID
	ProjectID  uuid.UUID          `json:"project_id"`
	ParentID   *uuid.UUID         `json:"parent_id,omitempty"`
	Content    string             `json:"content"`
	CreatedBy  *uuid.UUID         `json:"created_by"`
	UpdatedBy  *uuid.UUID         `json:"updated_by,omitempty"`
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
	// For tombstones (soft-deleted comments), hide the content
	content := c.Content
	isDeleted := c.IsSoftDeleted()
	if isDeleted {
		content = "[deleted]"
	}

	return &CommentResponse{
		ID:         c.ID,
		EntityType: c.EntityType,
		EntityID:   c.EntityID,
		ProjectID:  c.ProjectID,
		ParentID:   c.ParentID,
		Content:    content,
		CreatedBy:  c.CreatedBy,
		UpdatedBy:  c.UpdatedBy,
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
	Author *CommentUser `json:"author,omitempty"`
	Editor *CommentUser `json:"editor,omitempty"`
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
