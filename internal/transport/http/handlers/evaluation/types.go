package evaluation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// @Description Score type (NUMERIC, CATEGORICAL, BOOLEAN)
type ScoreType string

const (
	ScoreTypeNumeric     ScoreType = "NUMERIC"
	ScoreTypeCategorical ScoreType = "CATEGORICAL"
	ScoreTypeBoolean     ScoreType = "BOOLEAN"
)

// @Description Dataset item data
type DatasetItemResponse struct {
	ID            uuid.UUID      `json:"id"`
	DatasetID     uuid.UUID      `json:"dataset_id"`
	Input         map[string]any `json:"input"`
	Expected      map[string]any `json:"expected,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Source        string         `json:"source"`
	SourceTraceID *string        `json:"source_trace_id,omitempty"` // W3C hex
	SourceSpanID  *string        `json:"source_span_id,omitempty"`  // W3C hex
	CreatedAt     time.Time      `json:"created_at"`
}

// @Description Bulk import result
type BulkImportResponse struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// @Description Keys mapping for field extraction
type KeysMappingRequest struct {
	InputKeys    []string `json:"input_keys,omitempty"`
	ExpectedKeys []string `json:"expected_keys,omitempty"`
	MetadataKeys []string `json:"metadata_keys,omitempty"`
}

// @Description Import from JSON request
type ImportFromJSONRequest struct {
	Items       []map[string]any `json:"items" binding:"required,min=1"`
	KeysMapping *KeysMappingRequest      `json:"keys_mapping,omitempty"`
	Deduplicate bool                     `json:"deduplicate"`
	Source      string                   `json:"source,omitempty"`
}

// @Description Create from traces request
type CreateFromTracesRequest struct {
	TraceIDs    []string            `json:"trace_ids" binding:"required,min=1"`
	KeysMapping *KeysMappingRequest `json:"keys_mapping,omitempty"`
	Deduplicate bool                `json:"deduplicate"`
}

// @Description Create from spans request
type CreateFromSpansRequest struct {
	SpanIDs     []string            `json:"span_ids" binding:"required,min=1"`
	KeysMapping *KeysMappingRequest `json:"keys_mapping,omitempty"`
	Deduplicate bool                `json:"deduplicate"`
}

// @Description CSV column mapping for field extraction
type CSVColumnMappingRequest struct {
	InputColumn     string   `json:"input_column" binding:"required"`
	ExpectedColumn  string   `json:"expected_column,omitempty"`
	MetadataColumns []string `json:"metadata_columns,omitempty"`
}

// @Description Import from CSV request
type ImportFromCSVRequest struct {
	Content       string                  `json:"content" binding:"required"`
	ColumnMapping CSVColumnMappingRequest `json:"column_mapping" binding:"required"`
	HasHeader     bool                    `json:"has_header"`
	Deduplicate   bool                    `json:"deduplicate"`
}

// @Description Experiment item data
type ExperimentItemResponse struct {
	ID            uuid.UUID      `json:"id"`
	ExperimentID  uuid.UUID      `json:"experiment_id"`
	DatasetItemID *uuid.UUID     `json:"dataset_item_id,omitempty"`
	TraceID       *string        `json:"trace_id,omitempty"` // W3C hex
	Input         map[string]any `json:"input"`
	Output        any            `json:"output,omitempty"`
	Expected      any            `json:"expected,omitempty"`
	TrialNumber   int            `json:"trial_number"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

// @Description Paginated experiment items response
type ExperimentItemListResponse struct {
	Items []*ExperimentItemResponse `json:"items"`
	Total int64                     `json:"total"`
}

// @Description Score config creation request
type CreateRequest struct {
	Name        string                 `json:"name" binding:"required,min=1,max=100"`
	Description *string                `json:"description,omitempty"`
	Type        ScoreType              `json:"type" binding:"required,oneof=NUMERIC CATEGORICAL BOOLEAN"`
	MinValue    *float64               `json:"min_value,omitempty"`
	MaxValue    *float64               `json:"max_value,omitempty"`
	Categories  []string               `json:"categories,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// @Description Score config update request
type UpdateRequest struct {
	Name        *string                `json:"name,omitempty" binding:"omitempty,min=1,max=100"`
	Description *string                `json:"description,omitempty"`
	Type        *ScoreType             `json:"type,omitempty" binding:"omitempty,oneof=NUMERIC CATEGORICAL BOOLEAN"`
	MinValue    *float64               `json:"min_value,omitempty"`
	MaxValue    *float64               `json:"max_value,omitempty"`
	Categories  []string               `json:"categories,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// @Description Batch creation response with count
type SDKBatchCreateItemsResponse struct {
	Created int `json:"created"`
}

// @Description Batch experiment items creation response
type SDKBatchCreateExperimentItemsResponse struct {
	Created int `json:"created"`
}

// @Description SDK score creation request
type CreateScoreRequest struct {
	TraceID          *string        `json:"trace_id,omitempty"` // W3C hex — required for trace-linked scores, nil for experiment-only scores
	SpanID           *string        `json:"span_id,omitempty"`  // W3C hex
	Name             string         `json:"name" binding:"required"`
	Value            *float64       `json:"value,omitempty"`
	StringValue      *string        `json:"string_value,omitempty"`
	Type             string         `json:"type" binding:"required,oneof=NUMERIC CATEGORICAL BOOLEAN"`
	Reason           *string        `json:"reason,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	ExperimentID     *uuid.UUID     `json:"experiment_id,omitempty"`
	ExperimentItemID *string        `json:"experiment_item_id,omitempty"` // CH column is Nullable(String)
}

// @Description Batch score creation request
type BatchScoreRequest struct {
	Scores []CreateScoreRequest `json:"scores" binding:"required,dive"`
}

// @Description Score data
type ScoreResponse struct {
	ID               uuid.UUID       `json:"id"`
	ProjectID        uuid.UUID       `json:"project_id"`
	TraceID          *string         `json:"trace_id,omitempty"` // W3C hex; nil for experiment-only scores
	SpanID           *string         `json:"span_id,omitempty"`  // W3C hex; nil for experiment-only scores
	Name             string          `json:"name"`
	Value            *float64        `json:"value,omitempty"`
	StringValue      *string         `json:"string_value,omitempty"`
	Type             string          `json:"type"`
	Source           string          `json:"source"`
	Reason           *string         `json:"reason,omitempty"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
	ExperimentID     *uuid.UUID      `json:"experiment_id,omitempty"`
	ExperimentItemID *string         `json:"experiment_item_id,omitempty"`
	Timestamp        time.Time       `json:"timestamp"`
}

// @Description Batch score creation response
type BatchScoreResponse struct {
	Created int `json:"created"`
}

// ============================================================================
// Experiment Comparison Types
// ============================================================================

// @Description Request to compare multiple experiments
type CompareExperimentsRequest struct {
	ExperimentIDs []string `json:"experiment_ids" binding:"required,min=2,max=10"`
	BaselineID    *string  `json:"baseline_id,omitempty"`
}

// @Description Score aggregation statistics
type ScoreAggregationResponse struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Count  uint64  `json:"count"`
}

// @Description Score difference from baseline
type ScoreDiffResponse struct {
	Type       string  `json:"type"`
	Difference float64 `json:"difference,omitempty"`
	Direction  string  `json:"direction,omitempty"`
}

// @Description Experiment summary for comparison
type ExperimentSummaryResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// @Description Response containing experiment comparison results
type CompareExperimentsResponse struct {
	Experiments map[string]*ExperimentSummaryResponse           `json:"experiments"`
	Scores      map[string]map[string]*ScoreAggregationResponse `json:"scores"`
	Diffs       map[string]map[string]*ScoreDiffResponse        `json:"diffs,omitempty"`
}
