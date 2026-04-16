package annotation

import "errors"

var (
	// Queue errors
	ErrQueueNotFound   = errors.New("annotation queue not found")
	ErrQueueExists     = errors.New("annotation queue with this name already exists")
	ErrQueuePaused     = errors.New("annotation queue is paused")
	ErrQueueArchived   = errors.New("annotation queue is archived")
	ErrInvalidQueueID  = errors.New("invalid queue ID")
	ErrQueueValidation = errors.New("annotation queue validation failed")

	// Item errors
	ErrItemNotFound         = errors.New("queue item not found")
	ErrItemExists           = errors.New("item already exists in queue")
	ErrItemAlreadyCompleted = errors.New("item is already completed")
	ErrItemAlreadySkipped   = errors.New("item is already skipped")
	ErrItemLocked           = errors.New("item is locked by another user")
	ErrItemNotLocked        = errors.New("item is not currently locked")
	ErrItemLockExpired      = errors.New("item lock has expired")
	ErrNoItemsAvailable     = errors.New("no items available for annotation")
	ErrItemValidation       = errors.New("queue item validation failed")

	// Assignment errors
	ErrAssignmentNotFound = errors.New("queue assignment not found")
	ErrAssignmentExists   = errors.New("user is already assigned to this queue")
	ErrNotAssigned        = errors.New("user is not assigned to this queue")
	ErrInsufficientRole   = errors.New("insufficient role for this operation")

	// Score submission errors
	ErrInvalidScoreConfig = errors.New("invalid score config ID")
	ErrScoreRequired      = errors.New("required scores were not submitted")
	ErrInvalidScoreValue  = errors.New("invalid score value")
)
