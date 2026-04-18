// Package annotation provides domain entities for Human-in-the-Loop (HITL) evaluation workflows.
// Annotation queues enable quality assessment through manual human review of traces and spans.
package annotation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

// ============================================================================
// Enums and Constants
// ============================================================================

// QueueStatus represents the current state of an annotation queue.
type QueueStatus string

const (
	QueueStatusActive   QueueStatus = "active"
	QueueStatusPaused   QueueStatus = "paused"
	QueueStatusArchived QueueStatus = "archived"
)

// IsValid checks if the status is a valid QueueStatus value.
func (s QueueStatus) IsValid() bool {
	switch s {
	case QueueStatusActive, QueueStatusPaused, QueueStatusArchived:
		return true
	default:
		return false
	}
}

// ItemStatus represents the current state of an annotation queue item.
// Note: No "in_progress" status - Langfuse pattern uses locking timestamp to determine active state.
type ItemStatus string

const (
	ItemStatusPending   ItemStatus = "pending"
	ItemStatusCompleted ItemStatus = "completed"
	ItemStatusSkipped   ItemStatus = "skipped"
)

// IsValid checks if the status is a valid ItemStatus value.
func (s ItemStatus) IsValid() bool {
	switch s {
	case ItemStatusPending, ItemStatusCompleted, ItemStatusSkipped:
		return true
	default:
		return false
	}
}

// ObjectType represents the type of object being annotated.
type ObjectType string

const (
	ObjectTypeTrace ObjectType = "trace"
	ObjectTypeSpan  ObjectType = "span"
)

// IsValid checks if the type is a valid ObjectType value.
func (t ObjectType) IsValid() bool {
	switch t {
	case ObjectTypeTrace, ObjectTypeSpan:
		return true
	default:
		return false
	}
}

// AssignmentRole represents a user's role within an annotation queue.
type AssignmentRole string

const (
	RoleAnnotator AssignmentRole = "annotator"
	RoleReviewer  AssignmentRole = "reviewer"
	RoleAdmin     AssignmentRole = "admin"
)

// IsValid checks if the role is a valid AssignmentRole value.
func (r AssignmentRole) IsValid() bool {
	switch r {
	case RoleAnnotator, RoleReviewer, RoleAdmin:
		return true
	default:
		return false
	}
}

// DefaultLockTimeoutSeconds is the default lock duration (5 minutes) from Langfuse pattern.
const DefaultLockTimeoutSeconds = 300

// ============================================================================
// Queue Settings
// ============================================================================

// QueueSettings contains configurable settings for an annotation queue.
// Default values follow the Langfuse pattern (5-minute lock lease).
type QueueSettings struct {
	LockTimeoutSeconds int  `json:"lock_timeout_seconds"` // Default: 300 (5 min)
	AutoAssignment     bool `json:"auto_assignment"`      // Default: false
}

// DefaultQueueSettings returns the default queue settings.
func DefaultQueueSettings() QueueSettings {
	return QueueSettings{
		LockTimeoutSeconds: DefaultLockTimeoutSeconds,
		AutoAssignment:     false,
	}
}

// GetLockDuration returns the lock timeout as a time.Duration.
func (s QueueSettings) GetLockDuration() time.Duration {
	if s.LockTimeoutSeconds <= 0 {
		return time.Duration(DefaultLockTimeoutSeconds) * time.Second
	}
	return time.Duration(s.LockTimeoutSeconds) * time.Second
}

// ============================================================================
// Annotation Queue Entity
// ============================================================================

// AnnotationQueue represents a configuration for organizing annotation tasks.
// Design informed by Langfuse (5-min leasing) and Opik (instructions field).
type AnnotationQueue struct {
	ID             uuid.UUID     `json:"id"`
	ProjectID      uuid.UUID     `json:"project_id"`
	Name           string        `json:"name"`
	Description    *string       `json:"description,omitempty"`
	Instructions   *string       `json:"instructions,omitempty"` // Annotation guidelines (from Opik pattern)
	ScoreConfigIDs []uuid.UUID   `json:"score_config_ids"`       // Which scores to collect
	Status         QueueStatus   `json:"status"`
	Settings       QueueSettings `json:"settings"` // Lock timeout, auto-assignment, etc.
	CreatedBy      *uuid.UUID    `json:"created_by,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// NewAnnotationQueue creates a new annotation queue with default settings.
func NewAnnotationQueue(projectID uuid.UUID, name string) *AnnotationQueue {
	now := time.Now()
	return &AnnotationQueue{
		ID:             uid.New(),
		ProjectID:      projectID,
		Name:           name,
		ScoreConfigIDs: []uuid.UUID{},
		Status:         QueueStatusActive,
		Settings:       DefaultQueueSettings(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// ValidationError represents a validation error for an entity.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validate validates the annotation queue entity.
func (q *AnnotationQueue) Validate() []ValidationError {
	var errors []ValidationError

	if q.Name == "" {
		errors = append(errors, ValidationError{Field: "name", Message: "name is required"})
	}
	if len(q.Name) > 255 {
		errors = append(errors, ValidationError{Field: "name", Message: "name must be 255 characters or less"})
	}

	if !q.Status.IsValid() {
		errors = append(errors, ValidationError{Field: "status", Message: "invalid status, must be active, paused, or archived"})
	}

	if q.Settings.LockTimeoutSeconds < 0 {
		errors = append(errors, ValidationError{Field: "settings.lock_timeout_seconds", Message: "lock_timeout_seconds cannot be negative"})
	}

	return errors
}

// GetLockDuration returns the lock timeout as a time.Duration.
func (q *AnnotationQueue) GetLockDuration() time.Duration {
	return q.Settings.GetLockDuration()
}

// ============================================================================
// Queue Item Entity
// ============================================================================

// QueueItem represents an individual item pending human review in an annotation queue.
// Design follows Langfuse pattern: dual-user tracking (locked_by vs annotator_user_id).
type QueueItem struct {
	ID              uuid.UUID      `json:"id"`
	QueueID         uuid.UUID      `json:"queue_id"`
	ObjectID        string         `json:"object_id"` // trace_id or span_id — W3C hex, not UUID
	ObjectType      ObjectType     `json:"object_type"`
	Status          ItemStatus     `json:"status"`
	Priority        int            `json:"priority"`                    // Higher = more urgent
	LockedAt        *time.Time     `json:"locked_at,omitempty"`         // When item was locked for review
	LockedByUserID  *uuid.UUID     `json:"locked_by_user_id,omitempty"` // Who currently holds the lock
	AnnotatorUserID *uuid.UUID     `json:"annotator_user_id,omitempty"` // Who completed the annotation
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"` // Source info, sampling reason, etc.
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// NewQueueItem creates a new queue item with default values.
func NewQueueItem(queueID uuid.UUID, objectID string, objectType ObjectType) *QueueItem {
	now := time.Now()
	return &QueueItem{
		ID:         uid.New(),
		QueueID:    queueID,
		ObjectID:   objectID,
		ObjectType: objectType,
		Status:     ItemStatusPending,
		Priority:   0,
		Metadata:   make(map[string]any),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// Validate validates the queue item entity.
func (i *QueueItem) Validate() []ValidationError {
	var errors []ValidationError

	if i.ObjectID == "" {
		errors = append(errors, ValidationError{Field: "object_id", Message: "object_id is required"})
	}
	if len(i.ObjectID) > 32 {
		errors = append(errors, ValidationError{Field: "object_id", Message: "object_id must be 32 characters or less"})
	}

	if !i.ObjectType.IsValid() {
		errors = append(errors, ValidationError{Field: "object_type", Message: "invalid object_type, must be trace or span"})
	}

	if !i.Status.IsValid() {
		errors = append(errors, ValidationError{Field: "status", Message: "invalid status, must be pending, completed, or skipped"})
	}

	return errors
}

// IsLocked checks if the item is currently locked (within the specified lock timeout).
// Follows Langfuse pattern: timestamp-based expiration, not stored field.
func (i *QueueItem) IsLocked(lockTimeout time.Duration) bool {
	if i.LockedAt == nil || i.LockedByUserID == nil {
		return false
	}
	return time.Since(*i.LockedAt) < lockTimeout
}

// IsLockedBy checks if the item is locked by a specific user.
func (i *QueueItem) IsLockedBy(userID uuid.UUID, lockTimeout time.Duration) bool {
	if !i.IsLocked(lockTimeout) {
		return false
	}
	return *i.LockedByUserID == userID
}

// CanBeClaimed checks if the item can be claimed by a user.
// An item can be claimed if: never locked OR lock expired OR locked by current user.
func (i *QueueItem) CanBeClaimed(userID uuid.UUID, lockTimeout time.Duration) bool {
	// Already completed or skipped
	if i.Status != ItemStatusPending {
		return false
	}

	// Never locked
	if i.LockedAt == nil || i.LockedByUserID == nil {
		return true
	}

	// Lock expired
	if !i.IsLocked(lockTimeout) {
		return true
	}

	// Locked by current user (can reclaim)
	return *i.LockedByUserID == userID
}

// Lock marks the item as locked by a user.
func (i *QueueItem) Lock(userID uuid.UUID) {
	now := time.Now()
	i.LockedAt = &now
	i.LockedByUserID = &userID
	i.UpdatedAt = now
}

// ReleaseLock releases the lock on the item.
func (i *QueueItem) ReleaseLock() {
	i.LockedAt = nil
	i.LockedByUserID = nil
	i.UpdatedAt = time.Now()
}

// Complete marks the item as completed by the annotator.
func (i *QueueItem) Complete(userID uuid.UUID) {
	now := time.Now()
	i.Status = ItemStatusCompleted
	i.AnnotatorUserID = &userID
	i.CompletedAt = &now
	i.UpdatedAt = now
}

// Skip marks the item as skipped by the annotator.
func (i *QueueItem) Skip(userID uuid.UUID) {
	now := time.Now()
	i.Status = ItemStatusSkipped
	i.AnnotatorUserID = &userID
	i.CompletedAt = &now
	i.UpdatedAt = now
}

// ============================================================================
// Queue Assignment Entity
// ============================================================================

// QueueAssignment represents a user's assignment to an annotation queue.
// Defines who can annotate items in a queue and their role.
type QueueAssignment struct {
	ID         uuid.UUID      `json:"id"`
	QueueID    uuid.UUID      `json:"queue_id"`
	UserID     uuid.UUID      `json:"user_id"`
	Role       AssignmentRole `json:"role"`
	AssignedAt time.Time      `json:"assigned_at"`
	AssignedBy *uuid.UUID     `json:"assigned_by,omitempty"`
}

// TableName returns the database table name for GORM.
// NewQueueAssignment creates a new queue assignment.
func NewQueueAssignment(queueID, userID uuid.UUID, role AssignmentRole) *QueueAssignment {
	return &QueueAssignment{
		ID:         uid.New(),
		QueueID:    queueID,
		UserID:     userID,
		Role:       role,
		AssignedAt: time.Now(),
	}
}

// Validate validates the queue assignment entity.
func (a *QueueAssignment) Validate() []ValidationError {
	var errors []ValidationError

	if !a.Role.IsValid() {
		errors = append(errors, ValidationError{Field: "role", Message: "invalid role, must be annotator, reviewer, or admin"})
	}

	return errors
}

// ============================================================================
// Queue Statistics
// ============================================================================

// QueueStats contains aggregated statistics for an annotation queue.
type QueueStats struct {
	TotalItems      int64 `json:"total_items"`
	PendingItems    int64 `json:"pending_items"`
	InProgressItems int64 `json:"in_progress_items"` // Items currently locked
	CompletedItems  int64 `json:"completed_items"`
	SkippedItems    int64 `json:"skipped_items"`
}

// CompletionPercentage returns the percentage of completed items.
func (s *QueueStats) CompletionPercentage() float64 {
	if s.TotalItems == 0 {
		return 0
	}
	return float64(s.CompletedItems) / float64(s.TotalItems) * 100
}

// ProcessedPercentage returns the percentage of processed (completed + skipped) items.
func (s *QueueStats) ProcessedPercentage() float64 {
	if s.TotalItems == 0 {
		return 0
	}
	return float64(s.CompletedItems+s.SkippedItems) / float64(s.TotalItems) * 100
}

// ============================================================================
// Request/Response Types
// ============================================================================

// CreateQueueRequest is the request body for creating an annotation queue.
type CreateQueueRequest struct {
	Name           string         `json:"name" binding:"required,min=1,max=255"`
	Description    *string        `json:"description,omitempty"`
	Instructions   *string        `json:"instructions,omitempty"` // Annotation guidelines (from Opik pattern)
	ScoreConfigIDs []uuid.UUID    `json:"score_config_ids,omitempty"`
	Settings       *QueueSettings `json:"settings,omitempty"` // Optional, defaults to 5-min lock
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// UpdateQueueRequest is the request body for updating an annotation queue.
type UpdateQueueRequest struct {
	Name           *string        `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Description    *string        `json:"description,omitempty"`
	Instructions   *string        `json:"instructions,omitempty"`
	ScoreConfigIDs *[]uuid.UUID   `json:"score_config_ids,omitempty"` // Pointer to distinguish nil (no change) from empty (clear all)
	Status         *QueueStatus   `json:"status,omitempty"`
	Settings       *QueueSettings `json:"settings,omitempty"`
}

// QueueFilter defines filter criteria for listing queues.
type QueueFilter struct {
	Status *QueueStatus
	Search *string
}

// QueueResponse is the API response for an annotation queue.
type QueueResponse struct {
	ID             uuid.UUID     `json:"id"`
	ProjectID      uuid.UUID     `json:"project_id"`
	Name           string        `json:"name"`
	Description    *string       `json:"description,omitempty"`
	Instructions   *string       `json:"instructions,omitempty"`
	ScoreConfigIDs []uuid.UUID   `json:"score_config_ids"`
	Status         QueueStatus   `json:"status"`
	Settings       QueueSettings `json:"settings"`
	CreatedBy      *uuid.UUID    `json:"created_by,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Stats          *QueueStats   `json:"stats,omitempty"` // Optional stats when requested
}

// ToResponse converts an AnnotationQueue to a QueueResponse.
func (q *AnnotationQueue) ToResponse() *QueueResponse {
	return &QueueResponse{
		ID:             q.ID,
		ProjectID:      q.ProjectID,
		Name:           q.Name,
		Description:    q.Description,
		Instructions:   q.Instructions,
		ScoreConfigIDs: q.ScoreConfigIDs,
		Status:         q.Status,
		Settings:       q.Settings,
		CreatedBy:      q.CreatedBy,
		CreatedAt:      q.CreatedAt,
		UpdatedAt:      q.UpdatedAt,
	}
}

// AddItemRequest is the request to add an item to a queue.
type AddItemRequest struct {
	ObjectID   string         `json:"object_id" binding:"required,max=32"` // W3C hex trace/span ID
	ObjectType ObjectType     `json:"object_type" binding:"required,oneof=trace span"`
	Priority   int            `json:"priority,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// AddItemsBatchRequest is the request to add multiple items to a queue.
type AddItemsBatchRequest struct {
	Items []AddItemRequest `json:"items" binding:"required,min=1,max=1000,dive"` // Opik: 1-1000 items per batch
}

// ItemFilter defines filter criteria for listing queue items.
type ItemFilter struct {
	Status *ItemStatus
	Limit  int
	Offset int
}

// ItemResponse is the API response for a queue item.
type ItemResponse struct {
	ID              uuid.UUID      `json:"id"`
	QueueID         uuid.UUID      `json:"queue_id"`
	ObjectID        string         `json:"object_id"` // W3C hex trace/span ID
	ObjectType      ObjectType     `json:"object_type"`
	Status          ItemStatus     `json:"status"`
	Priority        int            `json:"priority"`
	LockedAt        *time.Time     `json:"locked_at,omitempty"`
	LockedByUserID  *uuid.UUID     `json:"locked_by_user_id,omitempty"`
	AnnotatorUserID *uuid.UUID     `json:"annotator_user_id,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// ToResponse converts a QueueItem to an ItemResponse.
func (i *QueueItem) ToResponse() *ItemResponse {
	return &ItemResponse{
		ID:              i.ID,
		QueueID:         i.QueueID,
		ObjectID:        i.ObjectID,
		ObjectType:      i.ObjectType,
		Status:          i.Status,
		Priority:        i.Priority,
		LockedAt:        i.LockedAt,
		LockedByUserID:  i.LockedByUserID,
		AnnotatorUserID: i.AnnotatorUserID,
		CompletedAt:     i.CompletedAt,
		Metadata:        i.Metadata,
		CreatedAt:       i.CreatedAt,
		UpdatedAt:       i.UpdatedAt,
	}
}

// ScoreSubmission represents a score submitted during annotation completion.
// Value is genuinely polymorphic: float64, string, or bool depending on the
// referenced score config's data type. Validated at the service layer.
type ScoreSubmission struct {
	ScoreConfigID uuid.UUID `json:"score_config_id" binding:"required"`
	Value         any       `json:"value"`
	Comment       *string   `json:"comment,omitempty"`
}

// CompleteItemRequest is the request body for completing an annotation.
type CompleteItemRequest struct {
	Scores []ScoreSubmission `json:"scores,omitempty"` // Scores submitted with the annotation
}

// SkipItemRequest is the request body for skipping an item.
type SkipItemRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// ClaimNextRequest is the request body for claiming the next item.
type ClaimNextRequest struct {
	SeenItemIDs []uuid.UUID `json:"seen_item_ids,omitempty"` // Items to skip (already shown to user)
}

// AssignmentRequest is the request body for assigning a user to a queue.
type AssignmentRequest struct {
	UserID uuid.UUID      `json:"user_id" binding:"required"`
	Role   AssignmentRole `json:"role,omitempty"` // Default: annotator
}

// AssignmentResponse is the API response for a queue assignment.
type AssignmentResponse struct {
	ID         uuid.UUID      `json:"id"`
	QueueID    uuid.UUID      `json:"queue_id"`
	UserID     uuid.UUID      `json:"user_id"`
	Role       AssignmentRole `json:"role"`
	AssignedAt time.Time      `json:"assigned_at"`
	AssignedBy *uuid.UUID     `json:"assigned_by,omitempty"`
}

// ToResponse converts a QueueAssignment to an AssignmentResponse.
func (a *QueueAssignment) ToResponse() *AssignmentResponse {
	return &AssignmentResponse{
		ID:         a.ID,
		QueueID:    a.QueueID,
		UserID:     a.UserID,
		Role:       a.Role,
		AssignedAt: a.AssignedAt,
		AssignedBy: a.AssignedBy,
	}
}

// ============================================================================
// JSON Marshaling for QueueSettings
// ============================================================================

// Scan implements the sql.Scanner interface for QueueSettings.
func (s *QueueSettings) Scan(value any) error {
	if value == nil {
		*s = DefaultQueueSettings()
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}
