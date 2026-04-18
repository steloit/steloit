package observability

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"brokle/pkg/pagination"
)

// SessionSummary represents aggregated session-level metrics computed from traces.
// Sessions are identified by session_id attribute on root spans.
type SessionSummary struct {
	SessionID     string          `json:"session_id" db:"session_id"`
	TraceCount    int64           `json:"trace_count" db:"trace_count"`
	FirstTrace    time.Time       `json:"first_trace" db:"first_trace"`
	LastTrace     time.Time       `json:"last_trace" db:"last_trace"`
	TotalDuration uint64          `json:"total_duration" db:"total_duration"` // nanoseconds
	TotalTokens   uint64          `json:"total_tokens" db:"total_tokens"`
	TotalCost     decimal.Decimal `json:"total_cost" db:"total_cost"`
	ErrorCount    int64           `json:"error_count" db:"error_count"`
	UserIDs       []string        `json:"user_ids" db:"user_ids"`
}

// SessionFilter represents filtering options for listing sessions.
type SessionFilter struct {
	ProjectID uuid.UUID `json:"project_id"`

	// Time range filter
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`

	// Search filter (searches in session_id)
	Search *string `json:"search,omitempty"`

	// User filter
	UserID *string `json:"user_id,omitempty"`

	// Pagination and sorting
	pagination.Params
}

// SetDefaults applies default values to the filter.
func (f *SessionFilter) SetDefaults(defaultSort string) {
	if f.SortBy == "" {
		f.SortBy = defaultSort
	}
	if f.SortDir == "" {
		f.SortDir = "desc"
	}
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
}
