package evaluation

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

// ExecutionStatus represents the current state of an evaluator execution.
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
)

// TriggerType defines how the evaluation was initiated.
type TriggerType string

const (
	TriggerTypeAutomatic TriggerType = "automatic"
	TriggerTypeManual    TriggerType = "manual"
)

// EvaluatorExecution tracks the execution history of an evaluator.
// Inspired by Langfuse's JobExecution and Opik's automation rule logs.
type EvaluatorExecution struct {
	ID           uuid.UUID       `json:"id"`
	EvaluatorID  uuid.UUID       `json:"evaluator_id"`
	ProjectID    uuid.UUID       `json:"project_id"`
	Status       ExecutionStatus `json:"status"`
	TriggerType  TriggerType     `json:"trigger_type"`
	SpansMatched int             `json:"spans_matched"`
	SpansScored  int             `json:"spans_scored"`
	ErrorsCount  int             `json:"errors_count"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	DurationMs   *int            `json:"duration_ms,omitempty"`
	Metadata     map[string]any  `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

// NewEvaluatorExecution creates a new evaluator execution record.
func NewEvaluatorExecution(evaluatorID, projectID uuid.UUID, triggerType TriggerType) *EvaluatorExecution {
	now := time.Now()
	return &EvaluatorExecution{
		ID:           uid.New(),
		EvaluatorID:  evaluatorID,
		ProjectID:    projectID,
		Status:       ExecutionStatusPending,
		TriggerType:  triggerType,
		SpansMatched: 0,
		SpansScored:  0,
		ErrorsCount:  0,
		Metadata:     make(map[string]any),
		CreatedAt:    now,
	}
}

// Start marks the execution as running.
func (e *EvaluatorExecution) Start() {
	now := time.Now()
	e.Status = ExecutionStatusRunning
	e.StartedAt = &now
}

// Complete marks the execution as successfully completed with counts.
func (e *EvaluatorExecution) Complete(spansMatched, spansScored, errorsCount int) {
	now := time.Now()
	e.Status = ExecutionStatusCompleted
	e.SpansMatched = spansMatched
	e.SpansScored = spansScored
	e.ErrorsCount = errorsCount
	e.CompletedAt = &now

	if e.StartedAt != nil {
		durationMs := int(now.Sub(*e.StartedAt).Milliseconds())
		e.DurationMs = &durationMs
	}
}

// Fail marks the execution as failed with an error message.
func (e *EvaluatorExecution) Fail(errorMessage string) {
	now := time.Now()
	e.Status = ExecutionStatusFailed
	e.ErrorMessage = &errorMessage
	e.CompletedAt = &now

	if e.StartedAt != nil {
		durationMs := int(now.Sub(*e.StartedAt).Milliseconds())
		e.DurationMs = &durationMs
	}
}

// Cancel marks the execution as cancelled.
func (e *EvaluatorExecution) Cancel() {
	now := time.Now()
	e.Status = ExecutionStatusCancelled
	e.CompletedAt = &now

	if e.StartedAt != nil {
		durationMs := int(now.Sub(*e.StartedAt).Milliseconds())
		e.DurationMs = &durationMs
	}
}

// SetMetadata adds contextual metadata to the execution.
func (e *EvaluatorExecution) SetMetadata(key string, value any) {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
}

// IsTerminal returns true if the execution is in a final state.
func (e *EvaluatorExecution) IsTerminal() bool {
	switch e.Status {
	case ExecutionStatusCompleted, ExecutionStatusFailed, ExecutionStatusCancelled:
		return true
	default:
		return false
	}
}

// Response types

type EvaluatorExecutionResponse struct {
	ID           string          `json:"id"`
	EvaluatorID  string          `json:"evaluator_id"`
	ProjectID    string          `json:"project_id"`
	Status       ExecutionStatus `json:"status"`
	TriggerType  TriggerType     `json:"trigger_type"`
	SpansMatched int             `json:"spans_matched"`
	SpansScored  int             `json:"spans_scored"`
	ErrorsCount  int             `json:"errors_count"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	DurationMs   *int            `json:"duration_ms,omitempty"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (e *EvaluatorExecution) ToResponse() *EvaluatorExecutionResponse {
	metadata := e.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}

	return &EvaluatorExecutionResponse{
		ID:           e.ID.String(),
		EvaluatorID:  e.EvaluatorID.String(),
		ProjectID:    e.ProjectID.String(),
		Status:       e.Status,
		TriggerType:  e.TriggerType,
		SpansMatched: e.SpansMatched,
		SpansScored:  e.SpansScored,
		ErrorsCount:  e.ErrorsCount,
		ErrorMessage: e.ErrorMessage,
		StartedAt:    e.StartedAt,
		CompletedAt:  e.CompletedAt,
		DurationMs:   e.DurationMs,
		Metadata:     metadata,
		CreatedAt:    e.CreatedAt,
	}
}

type EvaluatorExecutionListResponse struct {
	Executions []*EvaluatorExecutionResponse `json:"executions"`
	Total      int64                         `json:"total"`
	Page       int                           `json:"page"`
	Limit      int                           `json:"limit"`
}

// Filter for listing executions
type ExecutionFilter struct {
	Status      *ExecutionStatus
	TriggerType *TriggerType
}

// TriggerOptions for manual evaluation trigger
type TriggerOptions struct {
	TimeRangeStart *time.Time `json:"time_range_start,omitempty"` // Optional: start of time range to evaluate
	TimeRangeEnd   *time.Time `json:"time_range_end,omitempty"`   // Optional: end of time range to evaluate
	SpanIDs        []string   `json:"span_ids,omitempty"`         // Optional: specific spans to evaluate
	SampleLimit    int        `json:"sample_limit,omitempty"`     // Optional: max spans to process (default: 1000)
}

// TriggerResponse returned when triggering manual evaluation
type TriggerResponse struct {
	ExecutionID string `json:"execution_id"`
	SpansQueued int    `json:"spans_queued"`
	Message     string `json:"message"`
}
