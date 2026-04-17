// Package playground provides the playground session management domain model.
//
// The playground domain handles interactive prompt testing with persistent storage.
// Sessions are created when users explicitly save them from the playground UI.
package playground

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Session represents a playground session with persistent storage.
// Sessions are created when users explicitly save them from the playground UI.
type Session struct {
	// Primary key (shareable URL ID)
	ID uuid.UUID `json:"id"`

	// Project scope
	ProjectID uuid.UUID `json:"project_id"`

	// Session metadata
	Name        *string        `json:"name,omitempty"`
	Description *string        `json:"description,omitempty"`
	Tags        []string       `json:"tags"`

	// Session content
	Variables JSON `json:"variables"` // Variable values
	Config    JSON `json:"config,omitempty"`              // Model config
	Windows   JSON `json:"windows,omitempty"`             // Array of window states
	LastRun   JSON `json:"last_run,omitempty"`            // Last execution result

	// Audit fields
	CreatedBy  *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt time.Time  `json:"last_used_at"`
}

// TableName returns the database table name
// ----------------------------
// Template Types
// ----------------------------

// ChatMessage represents a single message in a chat template
type ChatMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatTemplate represents the template structure for chat prompts
type ChatTemplate struct {
	Messages []ChatMessage `json:"messages"`
}

// TextTemplate represents the template structure for text prompts
type TextTemplate struct {
	Content string `json:"content"`
}

// ----------------------------
// Configuration Types
// ----------------------------

// SessionConfig represents the model configuration for execution
type SessionConfig struct {
	Model            string   `json:"model,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	Stop             []string `json:"stop,omitempty"`
}

// ----------------------------
// Execution Types
// ----------------------------

// LastRun represents the last execution result
type LastRun struct {
	Content   string      `json:"content"`
	Metrics   *RunMetrics `json:"metrics,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Error     *string     `json:"error,omitempty"`
}

// RunMetrics represents execution metrics
type RunMetrics struct {
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
	LatencyMs        int64   `json:"latency_ms,omitempty"`
	TTFTMs           int64   `json:"ttft_ms,omitempty"` // Time to first token
	Model            string  `json:"model,omitempty"`
}

// ----------------------------
// Multi-Window Types
// ----------------------------

// WindowState represents the state of a single comparison window
type WindowState struct {
	Template  json.RawMessage `json:"template" swaggertype:"object"`
	Variables json.RawMessage `json:"variables,omitempty" swaggertype:"object"`
	Config    json.RawMessage `json:"config,omitempty" swaggertype:"object"`
	LastRun   *LastRun        `json:"last_run,omitempty"`
}

// ----------------------------
// Response DTOs
// ----------------------------

// SessionResponse is the API response for a session
type SessionResponse struct {
	ID          uuid.UUID       `json:"id"`
	ProjectID   uuid.UUID       `json:"project_id"`
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Tags        []string        `json:"tags"`
	Variables   json.RawMessage `json:"variables" swaggertype:"object"`
	Config      json.RawMessage `json:"config,omitempty" swaggertype:"object"`
	Windows     json.RawMessage `json:"windows,omitempty" swaggertype:"object"`
	LastRun     json.RawMessage `json:"last_run,omitempty" swaggertype:"object"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	LastUsedAt  time.Time       `json:"last_used_at"`
}

// SessionSummary is a lightweight response for sidebar listing
type SessionSummary struct {
	ID          uuid.UUID `json:"id"`
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Tags        []string  `json:"tags"`
	LastUsedAt  time.Time `json:"last_used_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToResponse converts a Session to SessionResponse
func (s *Session) ToResponse() *SessionResponse {
	tags := make([]string, len(s.Tags))
	copy(tags, s.Tags)
	if tags == nil {
		tags = []string{}
	}

	return &SessionResponse{
		ID:          s.ID,
		ProjectID:   s.ProjectID,
		Name:        s.Name,
		Description: s.Description,
		Tags:        tags,
		Variables:   json.RawMessage(s.Variables),
		Config:      json.RawMessage(s.Config),
		Windows:     json.RawMessage(s.Windows),
		LastRun:     json.RawMessage(s.LastRun),
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
		LastUsedAt:  s.LastUsedAt,
	}
}

// ToSummary converts a Session to SessionSummary
func (s *Session) ToSummary() *SessionSummary {
	tags := make([]string, len(s.Tags))
	copy(tags, s.Tags)
	if tags == nil {
		tags = []string{}
	}

	return &SessionSummary{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Tags:        tags,
		LastUsedAt:  s.LastUsedAt,
		CreatedAt:   s.CreatedAt,
	}
}

// ----------------------------
// JSON Type for JSONB columns
// ----------------------------

// JSON is a custom type for JSONB columns
type JSON json.RawMessage

// Value returns the JSON value for database storage
func (j JSON) Value() (interface{}, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return []byte(j), nil
}

// Scan implements the Scanner interface for reading from the database
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append((*j)[0:0], v...)
	case string:
		*j = append((*j)[0:0], []byte(v)...)
	}
	return nil
}

// MarshalJSON returns the JSON encoding
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON sets the JSON data
func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return nil
	}
	*j = append((*j)[0:0], data...)
	return nil
}

// ----------------------------
// Constants
// ----------------------------

const (
	// MaxWindowsCount is the maximum number of comparison windows allowed
	MaxWindowsCount = 3

	// MaxTagsCount is the maximum number of tags per session
	MaxTagsCount = 10

	// MaxNameLength is the maximum length of a session name
	MaxNameLength = 200
)
