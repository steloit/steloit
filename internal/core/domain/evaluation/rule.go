package evaluation

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"

	"github.com/lib/pq"
)

// EvaluatorStatus represents the current state of an evaluator.
type EvaluatorStatus string

const (
	EvaluatorStatusActive   EvaluatorStatus = "active"
	EvaluatorStatusInactive EvaluatorStatus = "inactive"
	EvaluatorStatusPaused   EvaluatorStatus = "paused"
)

// EvaluatorTrigger defines when an evaluator is executed.
type EvaluatorTrigger string

const (
	EvaluatorTriggerOnSpanComplete EvaluatorTrigger = "on_span_complete"
)

// TargetScope defines the scope of evaluation (span or trace level).
type TargetScope string

const (
	TargetScopeSpan  TargetScope = "span"
	TargetScopeTrace TargetScope = "trace"
)

// ScorerType defines the type of scorer used for evaluation.
type ScorerType string

const (
	ScorerTypeLLM     ScorerType = "llm"
	ScorerTypeBuiltin ScorerType = "builtin"
	ScorerTypeRegex   ScorerType = "regex"
)

// FilterClause represents a single filter condition for matching spans.
type FilterClause struct {
	Field    string      `json:"field"`    // e.g., "input", "output", "metadata.key", "span_kind"
	Operator string      `json:"operator"` // equals, not_equals, contains, gt, lt, is_empty
	Value    interface{} `json:"value"`
}

// VariableMap defines how to extract a variable from span data.
type VariableMap struct {
	VariableName string `json:"variable_name"` // Template variable: {input}, {output}
	Source       string `json:"source"`        // span_input, span_output, span_metadata, trace_input
	JSONPath     string `json:"json_path"`     // Optional: "messages[0].content", "data.result"
}

// Evaluator defines an automated evaluator for scoring spans.
type Evaluator struct {
	ID              uuid.UUID        `json:"id" gorm:"type:uuid;primaryKey"`
	ProjectID       uuid.UUID        `json:"project_id" gorm:"type:uuid;not null;index"`
	Name            string           `json:"name" gorm:"type:varchar(100);not null"`
	Description     *string          `json:"description,omitempty" gorm:"type:text"`
	Status          EvaluatorStatus  `json:"status" gorm:"type:varchar(20);not null;default:'inactive'"`
	TriggerType     EvaluatorTrigger `json:"trigger_type" gorm:"type:varchar(30);not null;default:'on_span_complete'"`
	TargetScope     TargetScope      `json:"target_scope" gorm:"type:varchar(20);not null;default:'span'"`
	Filter          []FilterClause   `json:"filter" gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	SpanNames       pq.StringArray   `json:"span_names" gorm:"type:text[];default:'{}'"`
	SamplingRate    float64          `json:"sampling_rate" gorm:"type:decimal(5,4);not null;default:1.0"`
	ScorerType      ScorerType       `json:"scorer_type" gorm:"type:varchar(20);not null"`
	ScorerConfig    map[string]any   `json:"scorer_config" gorm:"type:jsonb;serializer:json;not null"`
	VariableMapping []VariableMap    `json:"variable_mapping" gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	CreatedBy       *string          `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt       time.Time        `json:"created_at" gorm:"not null;autoCreateTime"`
	UpdatedAt       time.Time        `json:"updated_at" gorm:"not null;autoUpdateTime"`
}

func (Evaluator) TableName() string {
	return "evaluators"
}

func NewEvaluator(projectID uuid.UUID, name string, scorerType ScorerType, scorerConfig map[string]any) *Evaluator {
	now := time.Now()
	return &Evaluator{
		ID:              uid.New(),
		ProjectID:       projectID,
		Name:            name,
		Status:          EvaluatorStatusInactive,
		TriggerType:     EvaluatorTriggerOnSpanComplete,
		TargetScope:     TargetScopeSpan,
		Filter:          []FilterClause{},
		SpanNames:       []string{},
		SamplingRate:    1.0,
		ScorerType:      scorerType,
		ScorerConfig:    scorerConfig,
		VariableMapping: []VariableMap{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (e *Evaluator) Validate() []ValidationError {
	var errors []ValidationError

	if e.Name == "" {
		errors = append(errors, ValidationError{Field: "name", Message: "name is required"})
	}
	if len(e.Name) > 100 {
		errors = append(errors, ValidationError{Field: "name", Message: "name must be 100 characters or less"})
	}

	switch e.Status {
	case EvaluatorStatusActive, EvaluatorStatusInactive, EvaluatorStatusPaused:
	default:
		errors = append(errors, ValidationError{Field: "status", Message: "invalid status, must be active, inactive, or paused"})
	}

	switch e.TriggerType {
	case EvaluatorTriggerOnSpanComplete:
	default:
		errors = append(errors, ValidationError{Field: "trigger_type", Message: "invalid trigger type"})
	}

	switch e.TargetScope {
	case TargetScopeSpan, TargetScopeTrace:
	default:
		errors = append(errors, ValidationError{Field: "target_scope", Message: "invalid target scope, must be span or trace"})
	}

	if e.SamplingRate < 0.0 || e.SamplingRate > 1.0 {
		errors = append(errors, ValidationError{Field: "sampling_rate", Message: "sampling rate must be between 0 and 1"})
	}

	switch e.ScorerType {
	case ScorerTypeLLM, ScorerTypeBuiltin, ScorerTypeRegex:
	default:
		errors = append(errors, ValidationError{Field: "scorer_type", Message: "invalid scorer type, must be llm, builtin, or regex"})
	}

	if e.ScorerConfig == nil {
		errors = append(errors, ValidationError{Field: "scorer_config", Message: "scorer_config is required"})
	}

	return errors
}

// Request/Response types

type CreateEvaluatorRequest struct {
	Name            string            `json:"name" binding:"required,min=1,max=100"`
	Description     *string           `json:"description,omitempty"`
	Status          *EvaluatorStatus  `json:"status,omitempty"`
	TriggerType     *EvaluatorTrigger `json:"trigger_type,omitempty"`
	TargetScope     *TargetScope      `json:"target_scope,omitempty"`
	Filter          []FilterClause    `json:"filter,omitempty"`
	SpanNames       []string          `json:"span_names,omitempty"`
	SamplingRate    *float64          `json:"sampling_rate,omitempty"`
	ScorerType      ScorerType        `json:"scorer_type" binding:"required,oneof=llm builtin regex"`
	ScorerConfig    map[string]any    `json:"scorer_config" binding:"required"`
	VariableMapping []VariableMap     `json:"variable_mapping,omitempty"`
}

type UpdateEvaluatorRequest struct {
	Name            *string           `json:"name,omitempty" binding:"omitempty,min=1,max=100"`
	Description     *string           `json:"description,omitempty"`
	Status          *EvaluatorStatus  `json:"status,omitempty" binding:"omitempty,oneof=active inactive paused"`
	TriggerType     *EvaluatorTrigger `json:"trigger_type,omitempty"`
	TargetScope     *TargetScope      `json:"target_scope,omitempty" binding:"omitempty,oneof=span trace"`
	Filter          []FilterClause    `json:"filter,omitempty"`
	SpanNames       []string          `json:"span_names,omitempty"`
	SamplingRate    *float64          `json:"sampling_rate,omitempty"`
	ScorerType      *ScorerType       `json:"scorer_type,omitempty" binding:"omitempty,oneof=llm builtin regex"`
	ScorerConfig    map[string]any    `json:"scorer_config,omitempty"`
	VariableMapping []VariableMap     `json:"variable_mapping,omitempty"`
}

type EvaluatorResponse struct {
	ID              string           `json:"id"`
	ProjectID       string           `json:"project_id"`
	Name            string           `json:"name"`
	Description     *string          `json:"description,omitempty"`
	Status          EvaluatorStatus  `json:"status"`
	TriggerType     EvaluatorTrigger `json:"trigger_type"`
	TargetScope     TargetScope      `json:"target_scope"`
	Filter          []FilterClause   `json:"filter"`
	SpanNames       []string         `json:"span_names"`
	SamplingRate    float64          `json:"sampling_rate"`
	ScorerType      ScorerType       `json:"scorer_type"`
	ScorerConfig    map[string]any   `json:"scorer_config"`
	VariableMapping []VariableMap    `json:"variable_mapping"`
	CreatedBy       *string          `json:"created_by,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

func (e *Evaluator) ToResponse() *EvaluatorResponse {
	var createdBy *string
	if e.CreatedBy != nil {
		createdBy = e.CreatedBy
	}

	spanNames := e.SpanNames
	if spanNames == nil {
		spanNames = []string{}
	}

	filter := e.Filter
	if filter == nil {
		filter = []FilterClause{}
	}

	variableMapping := e.VariableMapping
	if variableMapping == nil {
		variableMapping = []VariableMap{}
	}

	return &EvaluatorResponse{
		ID:              e.ID.String(),
		ProjectID:       e.ProjectID.String(),
		Name:            e.Name,
		Description:     e.Description,
		Status:          e.Status,
		TriggerType:     e.TriggerType,
		TargetScope:     e.TargetScope,
		Filter:          filter,
		SpanNames:       spanNames,
		SamplingRate:    e.SamplingRate,
		ScorerType:      e.ScorerType,
		ScorerConfig:    e.ScorerConfig,
		VariableMapping: variableMapping,
		CreatedBy:       createdBy,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}

// EvaluatorFilter for listing evaluators with pagination.
type EvaluatorFilter struct {
	Status     *EvaluatorStatus
	ScorerType *ScorerType
	Search     *string
}

// LLM Scorer Configuration Types

type LLMMessage struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // Template with {input}, {output}, {expected}
}

type OutputField struct {
	Name        string   `json:"name"`                  // Score name: "relevance", "coherence"
	Type        string   `json:"type"`                  // numeric, categorical, boolean
	Description string   `json:"description,omitempty"` // What to evaluate
	MinValue    *float64 `json:"min_value,omitempty"`
	MaxValue    *float64 `json:"max_value,omitempty"`
	Categories  []string `json:"categories,omitempty"` // For categorical: ["good", "bad"]
}

type LLMScorerConfig struct {
	CredentialID   string        `json:"credential_id"`   // Project's AI credential
	Model          string        `json:"model"`           // gpt-4o, claude-3-5-sonnet
	Messages       []LLMMessage  `json:"messages"`        // System + User messages
	Temperature    float64       `json:"temperature"`     // 0.0-1.0
	ResponseFormat string        `json:"response_format"` // json, text
	OutputSchema   []OutputField `json:"output_schema"`   // Expected output structure
}

// Builtin Scorer Configuration Types

type BuiltinScorerConfig struct {
	ScorerName string         `json:"scorer_name"` // contains, json_valid, length_check
	Config     map[string]any `json:"config"`      // Scorer-specific configuration
}

// Regex Scorer Configuration Types

type RegexScorerConfig struct {
	Pattern      string  `json:"pattern"`                  // Regex pattern
	ScoreName    string  `json:"score_name"`               // Name for the generated score
	MatchScore   float64 `json:"match_score,omitempty"`    // Score when pattern matches (default 1.0)
	NoMatchScore float64 `json:"no_match_score,omitempty"` // Score when pattern doesn't match (default 0.0)
	CaptureGroup *int    `json:"capture_group,omitempty"`  // Capture group to use for value extraction
}
