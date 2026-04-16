package annotation

import "time"

// Queue request/response types

// CreateQueueRequest represents the request body for creating an annotation queue.
// @Description Create annotation queue request
type CreateQueueRequest struct {
	Name           string         `json:"name" binding:"required,min=1,max=255"`
	Description    *string        `json:"description,omitempty"`
	Instructions   *string        `json:"instructions,omitempty"`
	ScoreConfigIDs []string       `json:"score_config_ids,omitempty"`
	Settings       *QueueSettings `json:"settings,omitempty"`
}

// UpdateQueueRequest represents the request body for updating an annotation queue.
// @Description Update annotation queue request
type UpdateQueueRequest struct {
	Name           *string        `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Description    *string        `json:"description,omitempty"`
	Instructions   *string        `json:"instructions,omitempty"`
	ScoreConfigIDs *[]string      `json:"score_config_ids,omitempty"` // Pointer to distinguish nil (no change) from empty (clear all)
	Status         *string        `json:"status,omitempty" binding:"omitempty,oneof=active paused archived"`
	Settings       *QueueSettings `json:"settings,omitempty"`
}

// QueueSettings represents configurable settings for an annotation queue.
// @Description Queue settings configuration
type QueueSettings struct {
	LockTimeoutSeconds int  `json:"lock_timeout_seconds,omitempty"`
	AutoAssignment     bool `json:"auto_assignment,omitempty"`
}

// QueueResponse represents an annotation queue in API responses.
// @Description Annotation queue data
type QueueResponse struct {
	ID             string         `json:"id"`
	ProjectID      string         `json:"project_id"`
	Name           string         `json:"name"`
	Description    *string        `json:"description,omitempty"`
	Instructions   *string        `json:"instructions,omitempty"`
	ScoreConfigIDs []string       `json:"score_config_ids"`
	Status         string         `json:"status"`
	Settings       *QueueSettings `json:"settings"`
	CreatedBy      *string        `json:"created_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// QueueWithStatsResponse represents an annotation queue with statistics.
// @Description Annotation queue with statistics
type QueueWithStatsResponse struct {
	Queue *QueueResponse `json:"queue"`
	Stats *StatsResponse `json:"stats"`
}

// StatsResponse represents queue statistics.
// @Description Queue statistics
type StatsResponse struct {
	TotalItems      int64 `json:"total_items"`
	PendingItems    int64 `json:"pending_items"`
	InProgressItems int64 `json:"in_progress_items"`
	CompletedItems  int64 `json:"completed_items"`
	SkippedItems    int64 `json:"skipped_items"`
}

// Item request/response types

// AddItemRequest represents a single item to add to a queue.
// @Description Add item to queue request
type AddItemRequest struct {
	ObjectID   string                 `json:"object_id" binding:"required"`
	ObjectType string                 `json:"object_type" binding:"required,oneof=trace span"`
	Priority   int                    `json:"priority,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// AddItemsBatchRequest represents a batch request to add items to a queue.
// @Description Batch add items to queue request
type AddItemsBatchRequest struct {
	Items []AddItemRequest `json:"items" binding:"required,min=1,max=1000,dive"`
}

// ClaimNextRequest represents the request to claim the next item.
// @Description Claim next item request
type ClaimNextRequest struct {
	SeenItemIDs []string `json:"seen_item_ids,omitempty"`
}

// CompleteItemRequest represents the request to complete an item.
// @Description Complete item request
type CompleteItemRequest struct {
	Scores []ScoreSubmission `json:"scores,omitempty"`
}

// ScoreSubmission represents a score to submit with item completion.
// @Description Score submission
type ScoreSubmission struct {
	ScoreConfigID string      `json:"score_config_id" binding:"required"`
	Value         interface{} `json:"value" binding:"required"`
	Comment       *string     `json:"comment,omitempty"`
}

// SkipItemRequest represents the request to skip an item.
// @Description Skip item request
type SkipItemRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// ItemResponse represents a queue item in API responses.
// @Description Queue item data
type ItemResponse struct {
	ID              string                 `json:"id"`
	QueueID         string                 `json:"queue_id"`
	ObjectID        string                 `json:"object_id"`
	ObjectType      string                 `json:"object_type"`
	Status          string                 `json:"status"`
	Priority        int                    `json:"priority"`
	LockedAt        *time.Time             `json:"locked_at,omitempty"`
	LockedByUserID  *string                `json:"locked_by_user_id,omitempty"`
	AnnotatorUserID *string                `json:"annotator_user_id,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// BatchAddItemsResponse represents the response for batch item creation.
// @Description Batch add items response
type BatchAddItemsResponse struct {
	Created int `json:"created"`
}

// Assignment request/response types

// AssignUserRequest represents the request to assign a user to a queue.
// @Description Assign user to queue request
type AssignUserRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required,oneof=annotator reviewer admin"`
}

// AssignmentResponse represents a queue assignment in API responses.
// @Description Queue assignment data
type AssignmentResponse struct {
	ID         string    `json:"id"`
	QueueID    string    `json:"queue_id"`
	UserID     string    `json:"user_id"`
	Role       string    `json:"role"`
	AssignedAt time.Time `json:"assigned_at"`
	AssignedBy *string   `json:"assigned_by,omitempty"`
}
