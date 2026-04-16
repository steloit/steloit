package evaluation

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

// ExperimentSource represents the origin of an experiment.
type ExperimentSource string

const (
	ExperimentSourceSDK       ExperimentSource = "sdk"
	ExperimentSourceDashboard ExperimentSource = "dashboard"
)

// IsValid checks if the source is a valid ExperimentSource value.
func (s ExperimentSource) IsValid() bool {
	switch s {
	case ExperimentSourceSDK, ExperimentSourceDashboard:
		return true
	default:
		return false
	}
}

// VariableMappingSource defines where to get the variable value from.
type VariableMappingSource string

const (
	VariableMappingSourceInput    VariableMappingSource = "dataset_input"
	VariableMappingSourceExpected VariableMappingSource = "dataset_expected"
	VariableMappingSourceMetadata VariableMappingSource = "dataset_metadata"
)

// IsValid checks if the source is valid.
func (s VariableMappingSource) IsValid() bool {
	switch s {
	case VariableMappingSourceInput, VariableMappingSourceExpected, VariableMappingSourceMetadata:
		return true
	default:
		return false
	}
}

// ExperimentVariableMapping defines how to map a prompt template variable to a dataset field.
type ExperimentVariableMapping struct {
	VariableName string                `json:"variable_name"`  // Template variable: {{query}}, {{context}}
	Source       VariableMappingSource `json:"source"`         // dataset_input, dataset_expected, dataset_metadata
	FieldPath    string                `json:"field_path"`     // JSON path: "text", "messages[0].content"
	IsAutoMapped bool                  `json:"is_auto_mapped"` // Whether this was auto-mapped or manually set
}

// ExperimentEvaluator defines an evaluator configuration for an experiment.
type ExperimentEvaluator struct {
	Name            string         `json:"name"`
	ScorerType      ScorerType     `json:"scorer_type"` // llm, builtin, regex (reused from rule.go)
	ScorerConfig    map[string]any `json:"scorer_config"`
	VariableMapping []VariableMap  `json:"variable_mapping,omitempty"` // Reuse from rule.go
}

// ExperimentConfig stores the configuration for experiments created via the dashboard wizard.
type ExperimentConfig struct {
	ID               uuid.UUID                   `json:"id" gorm:"type:uuid;primaryKey"`
	ExperimentID     uuid.UUID                   `json:"experiment_id" gorm:"type:uuid;unique;not null;index"`
	PromptID         uuid.UUID                   `json:"prompt_id" gorm:"type:uuid;not null"`
	PromptVersionID  uuid.UUID                   `json:"prompt_version_id" gorm:"type:uuid;not null"`
	ModelConfig      map[string]any              `json:"model_config,omitempty" gorm:"type:jsonb;serializer:json"`
	DatasetID        uuid.UUID                   `json:"dataset_id" gorm:"type:uuid;not null"`
	DatasetVersionID *uuid.UUID                  `json:"dataset_version_id,omitempty" gorm:"type:uuid"`
	VariableMapping  []ExperimentVariableMapping `json:"variable_mapping" gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	Evaluators       []ExperimentEvaluator       `json:"evaluators" gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	CreatedAt        time.Time                   `json:"created_at" gorm:"not null;autoCreateTime"`
	UpdatedAt        time.Time                   `json:"updated_at" gorm:"not null;autoUpdateTime"`
}

func (ExperimentConfig) TableName() string {
	return "experiment_configs"
}

// NewExperimentConfig creates a new experiment config.
func NewExperimentConfig(
	experimentID uuid.UUID,
	promptID uuid.UUID,
	promptVersionID uuid.UUID,
	datasetID uuid.UUID,
) *ExperimentConfig {
	now := time.Now()
	return &ExperimentConfig{
		ID:              uid.New(),
		ExperimentID:    experimentID,
		PromptID:        promptID,
		PromptVersionID: promptVersionID,
		DatasetID:       datasetID,
		VariableMapping: []ExperimentVariableMapping{},
		Evaluators:      []ExperimentEvaluator{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// Validate validates the experiment config.
func (c *ExperimentConfig) Validate() []ValidationError {
	var errors []ValidationError

	if c.PromptID == uuid.Nil {
		errors = append(errors, ValidationError{Field: "prompt_id", Message: "prompt_id is required"})
	}
	if c.PromptVersionID == uuid.Nil {
		errors = append(errors, ValidationError{Field: "prompt_version_id", Message: "prompt_version_id is required"})
	}
	if c.DatasetID == uuid.Nil {
		errors = append(errors, ValidationError{Field: "dataset_id", Message: "dataset_id is required"})
	}
	if len(c.Evaluators) == 0 {
		errors = append(errors, ValidationError{Field: "evaluators", Message: "at least one evaluator is required"})
	}

	// Validate each evaluator
	for i, eval := range c.Evaluators {
		if eval.Name == "" {
			errors = append(errors, ValidationError{
				Field:   "evaluators",
				Message: "evaluator name is required at index " + string(rune('0'+i)),
			})
		}
		switch eval.ScorerType {
		case ScorerTypeLLM, ScorerTypeBuiltin, ScorerTypeRegex:
			// Valid
		default:
			errors = append(errors, ValidationError{
				Field:   "evaluators",
				Message: "invalid scorer type at index " + string(rune('0'+i)),
			})
		}
		if eval.ScorerConfig == nil {
			errors = append(errors, ValidationError{
				Field:   "evaluators",
				Message: "scorer_config is required at index " + string(rune('0'+i)),
			})
		}
	}

	// Validate variable mappings
	for i, mapping := range c.VariableMapping {
		if mapping.VariableName == "" {
			errors = append(errors, ValidationError{
				Field:   "variable_mapping",
				Message: "variable_name is required at index " + string(rune('0'+i)),
			})
		}
		if !mapping.Source.IsValid() {
			errors = append(errors, ValidationError{
				Field:   "variable_mapping",
				Message: "invalid source at index " + string(rune('0'+i)),
			})
		}
		if mapping.FieldPath == "" {
			errors = append(errors, ValidationError{
				Field:   "variable_mapping",
				Message: "field_path is required at index " + string(rune('0'+i)),
			})
		}
	}

	return errors
}

// ============================================================================
// Request/Response Types
// ============================================================================

// CreateExperimentFromWizardRequest is the request to create an experiment via the wizard.
type CreateExperimentFromWizardRequest struct {
	// Step 1: Basic Info
	Name        string  `json:"name" binding:"required,min=1,max=255"`
	Description *string `json:"description,omitempty"`

	// Step 1: Prompt Configuration
	PromptID            string         `json:"prompt_id" binding:"required"`
	PromptVersionID     string         `json:"prompt_version_id" binding:"required"`
	ModelConfigOverride map[string]any `json:"model_config_override,omitempty"`

	// Step 2: Dataset Configuration
	DatasetID        string                      `json:"dataset_id" binding:"required"`
	DatasetVersionID *string                     `json:"dataset_version_id,omitempty"`
	VariableMapping  []ExperimentVariableMapping `json:"variable_mapping" binding:"required"`

	// Step 3: Evaluators
	Evaluators []ExperimentEvaluator `json:"evaluators" binding:"required,min=1"`

	// Options
	RunImmediately bool `json:"run_immediately"`
}

// ValidateStepRequest is the request to validate a wizard step.
type ValidateStepRequest struct {
	Step int            `json:"step" binding:"required,min=1,max=4"`
	Data map[string]any `json:"data" binding:"required"`
}

// ValidateStepResponse is the response from step validation.
type ValidateStepResponse struct {
	IsValid  bool              `json:"is_valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
}

// EstimateCostRequest is the request to estimate experiment cost.
type EstimateCostRequest struct {
	PromptID         string                `json:"prompt_id" binding:"required"`
	PromptVersionID  string                `json:"prompt_version_id" binding:"required"`
	DatasetID        string                `json:"dataset_id" binding:"required"`
	DatasetVersionID *string               `json:"dataset_version_id,omitempty"`
	Evaluators       []ExperimentEvaluator `json:"evaluators" binding:"required,min=1"`
}

// CostItem represents a single cost component.
type CostItem struct {
	Description    string  `json:"description"`
	EstimatedCost  float64 `json:"estimated_cost"`
	EstimatedUnits int64   `json:"estimated_units"` // tokens, items, etc.
	UnitType       string  `json:"unit_type"`       // tokens, items
}

// EstimateCostResponse is the response from cost estimation.
type EstimateCostResponse struct {
	ItemCount       int        `json:"item_count"`
	EstimatedTokens int64      `json:"estimated_tokens"`
	EstimatedCost   float64    `json:"estimated_cost"`
	Currency        string     `json:"currency"`
	CostBreakdown   []CostItem `json:"cost_breakdown"`
}

// FieldInfo represents a field in a dataset item.
type FieldInfo struct {
	Path        string `json:"path"`
	Type        string `json:"type"` // string, number, boolean, object, array
	SampleValue any    `json:"sample_value,omitempty"`
}

// DatasetFieldsResponse contains the schema of dataset fields.
type DatasetFieldsResponse struct {
	InputFields    []FieldInfo `json:"input_fields"`
	ExpectedFields []FieldInfo `json:"expected_fields"`
	MetadataFields []FieldInfo `json:"metadata_fields"`
}

// ExperimentConfigResponse is the API response for an experiment config.
type ExperimentConfigResponse struct {
	ID               string                      `json:"id"`
	ExperimentID     string                      `json:"experiment_id"`
	PromptID         string                      `json:"prompt_id"`
	PromptVersionID  string                      `json:"prompt_version_id"`
	ModelConfig      map[string]any              `json:"model_config,omitempty"`
	DatasetID        string                      `json:"dataset_id"`
	DatasetVersionID *string                     `json:"dataset_version_id,omitempty"`
	VariableMapping  []ExperimentVariableMapping `json:"variable_mapping"`
	Evaluators       []ExperimentEvaluator       `json:"evaluators"`
	CreatedAt        time.Time                   `json:"created_at"`
	UpdatedAt        time.Time                   `json:"updated_at"`
}

// ToResponse converts ExperimentConfig to its API response format.
func (c *ExperimentConfig) ToResponse() *ExperimentConfigResponse {
	var datasetVersionID *string
	if c.DatasetVersionID != nil {
		id := c.DatasetVersionID.String()
		datasetVersionID = &id
	}

	variableMapping := c.VariableMapping
	if variableMapping == nil {
		variableMapping = []ExperimentVariableMapping{}
	}

	evaluators := c.Evaluators
	if evaluators == nil {
		evaluators = []ExperimentEvaluator{}
	}

	return &ExperimentConfigResponse{
		ID:               c.ID.String(),
		ExperimentID:     c.ExperimentID.String(),
		PromptID:         c.PromptID.String(),
		PromptVersionID:  c.PromptVersionID.String(),
		ModelConfig:      c.ModelConfig,
		DatasetID:        c.DatasetID.String(),
		DatasetVersionID: datasetVersionID,
		VariableMapping:  variableMapping,
		Evaluators:       evaluators,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}
}
