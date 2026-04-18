// Package evaluation provides domain entities for quality scoring, datasets, and experiments.
package evaluation

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

type ScoreType string

const (
	ScoreTypeNumeric     ScoreType = "NUMERIC"
	ScoreTypeCategorical ScoreType = "CATEGORICAL"
	ScoreTypeBoolean     ScoreType = "BOOLEAN"
)

// ScoreConfig defines metadata and validation rules for a score type.
// Stored in PostgreSQL for transactional consistency.
type ScoreConfig struct {
	ID          uuid.UUID              `json:"id"`
	ProjectID   uuid.UUID              `json:"project_id"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Type        ScoreType              `json:"type"`
	MinValue    *float64               `json:"min_value,omitempty"`
	MaxValue    *float64               `json:"max_value,omitempty"`
	Categories  []string               `json:"categories,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

func NewScoreConfig(projectID uuid.UUID, name string, scoreType ScoreType) *ScoreConfig {
	now := time.Now()
	return &ScoreConfig{
		ID:        uid.New(),
		ProjectID: projectID,
		Name:      name,
		Type:      scoreType,
		Metadata:  make(map[string]any),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (sc *ScoreConfig) Validate() []ValidationError {
	var errors []ValidationError

	if sc.Name == "" {
		errors = append(errors, ValidationError{Field: "name", Message: "name is required"})
	}
	if len(sc.Name) > 100 {
		errors = append(errors, ValidationError{Field: "name", Message: "name must be 100 characters or less"})
	}

	switch sc.Type {
	case ScoreTypeNumeric:
		if sc.MinValue != nil && sc.MaxValue != nil && *sc.MinValue > *sc.MaxValue {
			errors = append(errors, ValidationError{Field: "max_value", Message: "max_value must be greater than or equal to min_value"})
		}
	case ScoreTypeCategorical:
		if len(sc.Categories) == 0 {
			errors = append(errors, ValidationError{Field: "categories", Message: "categories are required for CATEGORICAL type"})
		}
	case ScoreTypeBoolean:
	default:
		errors = append(errors, ValidationError{Field: "type", Message: "invalid type, must be NUMERIC, CATEGORICAL, or BOOLEAN"})
	}

	return errors
}

type CreateScoreConfigRequest struct {
	Name        string                 `json:"name" binding:"required,min=1,max=100"`
	Description *string                `json:"description,omitempty"`
	Type        ScoreType              `json:"type" binding:"required,oneof=NUMERIC CATEGORICAL BOOLEAN"`
	MinValue    *float64               `json:"min_value,omitempty"`
	MaxValue    *float64               `json:"max_value,omitempty"`
	Categories  []string               `json:"categories,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateScoreConfigRequest struct {
	Name        *string                `json:"name,omitempty" binding:"omitempty,min=1,max=100"`
	Description *string                `json:"description,omitempty"`
	Type        *ScoreType             `json:"type,omitempty" binding:"omitempty,oneof=NUMERIC CATEGORICAL BOOLEAN"`
	MinValue    *float64               `json:"min_value,omitempty"`
	MaxValue    *float64               `json:"max_value,omitempty"`
	Categories  []string               `json:"categories,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ScoreConfigResponse struct {
	ID          uuid.UUID      `json:"id"`
	ProjectID   uuid.UUID      `json:"project_id"`
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	Type        ScoreType      `json:"type"`
	MinValue    *float64       `json:"min_value,omitempty"`
	MaxValue    *float64       `json:"max_value,omitempty"`
	Categories  []string       `json:"categories,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (sc *ScoreConfig) ToResponse() *ScoreConfigResponse {
	return &ScoreConfigResponse{
		ID:          sc.ID,
		ProjectID:   sc.ProjectID,
		Name:        sc.Name,
		Description: sc.Description,
		Type:        sc.Type,
		MinValue:    sc.MinValue,
		MaxValue:    sc.MaxValue,
		Categories:  sc.Categories,
		Metadata:    sc.Metadata,
		CreatedAt:   sc.CreatedAt,
		UpdatedAt:   sc.UpdatedAt,
	}
}

// Dataset represents a collection of test cases for evaluation.
type Dataset struct {
	ID               uuid.UUID              `json:"id"`
	ProjectID        uuid.UUID              `json:"project_id"`
	Name             string                 `json:"name"`
	Description      *string                `json:"description,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CurrentVersionID *uuid.UUID             `json:"current_version_id,omitempty"` // Pinned version (nil = use latest)
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

func NewDataset(projectID uuid.UUID, name string) *Dataset {
	now := time.Now()
	return &Dataset{
		ID:        uid.New(),
		ProjectID: projectID,
		Name:      name,
		Metadata:  make(map[string]any),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (d *Dataset) Validate() []ValidationError {
	var errors []ValidationError

	if d.Name == "" {
		errors = append(errors, ValidationError{Field: "name", Message: "name is required"})
	}
	if len(d.Name) > 255 {
		errors = append(errors, ValidationError{Field: "name", Message: "name must be 255 characters or less"})
	}

	return errors
}

type CreateDatasetRequest struct {
	Name        string                 `json:"name" binding:"required,min=1,max=255"`
	Description *string                `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateDatasetRequest struct {
	Name        *string                `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Description *string                `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type DatasetResponse struct {
	ID               uuid.UUID      `json:"id"`
	ProjectID        uuid.UUID      `json:"project_id"`
	Name             string         `json:"name"`
	Description      *string        `json:"description,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CurrentVersionID *uuid.UUID     `json:"current_version_id,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func (d *Dataset) ToResponse() *DatasetResponse {
	return &DatasetResponse{
		ID:               d.ID,
		ProjectID:        d.ProjectID,
		Name:             d.Name,
		Description:      d.Description,
		Metadata:         d.Metadata,
		CurrentVersionID: d.CurrentVersionID,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}
}

// DatasetItemSource represents the origin of a dataset item.
type DatasetItemSource string

const (
	DatasetItemSourceManual DatasetItemSource = "manual"
	DatasetItemSourceTrace  DatasetItemSource = "trace"
	DatasetItemSourceSpan   DatasetItemSource = "span"
	DatasetItemSourceCSV    DatasetItemSource = "csv"
	DatasetItemSourceJSON   DatasetItemSource = "json"
	DatasetItemSourceSDK    DatasetItemSource = "sdk"
)

// IsValid checks if the source is a valid DatasetItemSource value.
func (s DatasetItemSource) IsValid() bool {
	switch s {
	case DatasetItemSourceManual, DatasetItemSourceTrace, DatasetItemSourceSpan,
		DatasetItemSourceCSV, DatasetItemSourceJSON, DatasetItemSourceSDK:
		return true
	default:
		return false
	}
}

// DatasetItem represents an individual test case within a dataset.
type DatasetItem struct {
	ID            uuid.UUID              `json:"id"`
	DatasetID     uuid.UUID              `json:"dataset_id"`
	Input         map[string]any `json:"input"`
	Expected      map[string]any `json:"expected,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Source        DatasetItemSource      `json:"source"`
	SourceTraceID *string                `json:"source_trace_id,omitempty"`
	SourceSpanID  *string                `json:"source_span_id,omitempty"`
	ContentHash   *string                `json:"content_hash,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

func NewDatasetItem(datasetID uuid.UUID, input map[string]any) *DatasetItem {
	return &DatasetItem{
		ID:        uid.New(),
		DatasetID: datasetID,
		Input:     input,
		Metadata:  make(map[string]any),
		Source:    DatasetItemSourceManual,
		CreatedAt: time.Now(),
	}
}

// NewDatasetItemWithSource creates a new dataset item with explicit source tracking.
func NewDatasetItemWithSource(datasetID uuid.UUID, input map[string]any, source DatasetItemSource) *DatasetItem {
	return &DatasetItem{
		ID:        uid.New(),
		DatasetID: datasetID,
		Input:     input,
		Metadata:  make(map[string]any),
		Source:    source,
		CreatedAt: time.Now(),
	}
}

func (di *DatasetItem) Validate() []ValidationError {
	var errors []ValidationError

	if di.Input == nil || len(di.Input) == 0 {
		errors = append(errors, ValidationError{Field: "input", Message: "input is required"})
	}

	return errors
}

type CreateDatasetItemRequest struct {
	Input    map[string]any `json:"input" binding:"required"`
	Expected map[string]any `json:"expected,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type CreateDatasetItemsBatchRequest struct {
	Items       []CreateDatasetItemRequest `json:"items" binding:"required,dive"`
	Deduplicate bool                       `json:"deduplicate,omitempty"`
}

// ============================================================================
// Bulk Import Types for Dataset Items
// ============================================================================

// KeysMapping defines how to map source fields to dataset item fields.
type KeysMapping struct {
	InputKeys    []string `json:"input_keys"`
	ExpectedKeys []string `json:"expected_keys"`
	MetadataKeys []string `json:"metadata_keys"`
}

// BulkImportResult contains the result of a bulk import operation.
type BulkImportResult struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// CreateDatasetItemsFromTracesRequest is the request to create dataset items from production traces.
type CreateDatasetItemsFromTracesRequest struct {
	TraceIDs    []string     `json:"trace_ids" binding:"required,min=1"`
	KeysMapping *KeysMapping `json:"keys_mapping,omitempty"`
	Deduplicate bool         `json:"deduplicate"`
}

// CreateDatasetItemsFromSpansRequest is the request to create dataset items from production spans.
type CreateDatasetItemsFromSpansRequest struct {
	SpanIDs     []string     `json:"span_ids" binding:"required,min=1"`
	KeysMapping *KeysMapping `json:"keys_mapping,omitempty"`
	Deduplicate bool         `json:"deduplicate"`
}

// ImportDatasetItemsFromJSONRequest is the request to import dataset items from JSON data.
type ImportDatasetItemsFromJSONRequest struct {
	Items       []map[string]any `json:"items" binding:"required,min=1"`
	KeysMapping *KeysMapping             `json:"keys_mapping,omitempty"`
	Deduplicate bool                     `json:"deduplicate"`
	Source      DatasetItemSource        `json:"source,omitempty"`
}

// CSVColumnMapping defines how CSV columns map to dataset item fields.
type CSVColumnMapping struct {
	InputColumn     string   `json:"input_column" binding:"required"`
	ExpectedColumn  string   `json:"expected_column,omitempty"`
	MetadataColumns []string `json:"metadata_columns,omitempty"`
}

// ImportDatasetItemsFromCSVRequest is the request to import dataset items from CSV data.
type ImportDatasetItemsFromCSVRequest struct {
	Content       string           `json:"content" binding:"required"`
	ColumnMapping CSVColumnMapping `json:"column_mapping" binding:"required"`
	HasHeader     bool             `json:"has_header"`
	Deduplicate   bool             `json:"deduplicate"`
}

// ExportFormat specifies the format for exporting dataset items.
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// ExportDatasetItemsRequest is the request to export dataset items.
type ExportDatasetItemsRequest struct {
	Format ExportFormat `json:"format" binding:"required,oneof=json csv"`
}

type DatasetItemResponse struct {
	ID            uuid.UUID         `json:"id"`
	DatasetID     uuid.UUID         `json:"dataset_id"`
	Input         map[string]any    `json:"input"`
	Expected      map[string]any    `json:"expected,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
	Source        DatasetItemSource `json:"source"`
	SourceTraceID *string           `json:"source_trace_id,omitempty"` // W3C hex
	SourceSpanID  *string           `json:"source_span_id,omitempty"`  // W3C hex
	CreatedAt     time.Time         `json:"created_at"`
}

func (di *DatasetItem) ToResponse() *DatasetItemResponse {
	return &DatasetItemResponse{
		ID:            di.ID,
		DatasetID:     di.DatasetID,
		Input:         di.Input,
		Expected:      di.Expected,
		Metadata:      di.Metadata,
		Source:        di.Source,
		SourceTraceID: di.SourceTraceID,
		SourceSpanID:  di.SourceSpanID,
		CreatedAt:     di.CreatedAt,
	}
}

// ExperimentStatus represents the current state of an experiment.
type ExperimentStatus string

const (
	ExperimentStatusPending   ExperimentStatus = "pending"
	ExperimentStatusRunning   ExperimentStatus = "running"
	ExperimentStatusCompleted ExperimentStatus = "completed"
	ExperimentStatusFailed    ExperimentStatus = "failed"
	ExperimentStatusPartial   ExperimentStatus = "partial"   // Some items completed, some failed
	ExperimentStatusCancelled ExperimentStatus = "cancelled" // User cancelled the experiment
)

// Experiment represents a batch evaluation run.
type Experiment struct {
	ID          uuid.UUID              `json:"id"`
	ProjectID   uuid.UUID              `json:"project_id"`
	DatasetID   *uuid.UUID             `json:"dataset_id,omitempty"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Status      ExperimentStatus       `json:"status"`
	Source      ExperimentSource       `json:"source"`
	ConfigID    *uuid.UUID             `json:"config_id,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	// Progress tracking fields
	TotalItems     int        `json:"total_items"`
	CompletedItems int        `json:"completed_items"`
	FailedItems    int        `json:"failed_items"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Relationships (optional, loaded when needed)
	Config *ExperimentConfig `json:"config,omitempty"`
}

func NewExperiment(projectID uuid.UUID, name string) *Experiment {
	now := time.Now()
	return &Experiment{
		ID:        uid.New(),
		ProjectID: projectID,
		Name:      name,
		Status:    ExperimentStatusPending,
		Source:    ExperimentSourceSDK, // Default to SDK for backwards compatibility
		Metadata:  make(map[string]any),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewExperimentFromDashboard creates a new experiment from the dashboard wizard.
func NewExperimentFromDashboard(projectID uuid.UUID, name string) *Experiment {
	now := time.Now()
	return &Experiment{
		ID:        uid.New(),
		ProjectID: projectID,
		Name:      name,
		Status:    ExperimentStatusPending,
		Source:    ExperimentSourceDashboard,
		Metadata:  make(map[string]any),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (e *Experiment) Validate() []ValidationError {
	var errors []ValidationError

	if e.Name == "" {
		errors = append(errors, ValidationError{Field: "name", Message: "name is required"})
	}
	if len(e.Name) > 255 {
		errors = append(errors, ValidationError{Field: "name", Message: "name must be 255 characters or less"})
	}

	switch e.Status {
	case ExperimentStatusPending, ExperimentStatusRunning, ExperimentStatusCompleted, ExperimentStatusFailed, ExperimentStatusPartial, ExperimentStatusCancelled:
	default:
		errors = append(errors, ValidationError{Field: "status", Message: "invalid status"})
	}

	return errors
}

type CreateExperimentRequest struct {
	Name        string         `json:"name" binding:"required,min=1,max=255"`
	DatasetID   *uuid.UUID     `json:"dataset_id,omitempty"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// RerunExperimentRequest is the request to create a new experiment based on an existing one.
// The new experiment will have the same dataset but can have a different name and metadata.
type RerunExperimentRequest struct {
	Name        *string                `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Description *string                `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateExperimentRequest struct {
	Name        *string                `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Description *string                `json:"description,omitempty"`
	Status      *ExperimentStatus      `json:"status,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ExperimentFilter struct {
	DatasetID *uuid.UUID
	Status    *ExperimentStatus
	Search    *string
	IDs       []uuid.UUID // Filter by specific experiment IDs
}

// DatasetFilter defines filter criteria for listing datasets.
type DatasetFilter struct {
	Search *string
}

type ExperimentResponse struct {
	ID             uuid.UUID        `json:"id"`
	ProjectID      uuid.UUID        `json:"project_id"`
	DatasetID      *uuid.UUID       `json:"dataset_id,omitempty"`
	Name           string           `json:"name"`
	Description    *string          `json:"description,omitempty"`
	Status         ExperimentStatus `json:"status"`
	Source         ExperimentSource `json:"source"`
	ConfigID       *uuid.UUID       `json:"config_id,omitempty"`
	Metadata       map[string]any   `json:"metadata,omitempty"`
	TotalItems     int              `json:"total_items"`
	CompletedItems int              `json:"completed_items"`
	FailedItems    int              `json:"failed_items"`
	StartedAt      *time.Time       `json:"started_at,omitempty"`
	CompletedAt    *time.Time       `json:"completed_at,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

func (e *Experiment) ToResponse() *ExperimentResponse {
	return &ExperimentResponse{
		ID:             e.ID,
		ProjectID:      e.ProjectID,
		DatasetID:      e.DatasetID,
		Name:           e.Name,
		Description:    e.Description,
		Status:         e.Status,
		Source:         e.Source,
		ConfigID:       e.ConfigID,
		Metadata:       e.Metadata,
		TotalItems:     e.TotalItems,
		CompletedItems: e.CompletedItems,
		FailedItems:    e.FailedItems,
		StartedAt:      e.StartedAt,
		CompletedAt:    e.CompletedAt,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

// ExperimentProgressResponse is a lightweight response for progress polling.
type ExperimentProgressResponse struct {
	ID             uuid.UUID        `json:"id"`
	Status         ExperimentStatus `json:"status"`
	TotalItems     int              `json:"total_items"`
	CompletedItems int              `json:"completed_items"`
	FailedItems    int              `json:"failed_items"`
	PendingItems   int              `json:"pending_items"`
	ProgressPct    float64          `json:"progress_pct"`
	StartedAt      *time.Time       `json:"started_at,omitempty"`
	CompletedAt    *time.Time       `json:"completed_at,omitempty"`
	ElapsedSeconds *float64         `json:"elapsed_seconds,omitempty"`
	ETASeconds     *float64         `json:"eta_seconds,omitempty"`
}

// ToProgressResponse creates a progress response with derived fields.
func (e *Experiment) ToProgressResponse() *ExperimentProgressResponse {
	resp := &ExperimentProgressResponse{
		ID:             e.ID,
		Status:         e.Status,
		TotalItems:     e.TotalItems,
		CompletedItems: e.CompletedItems,
		FailedItems:    e.FailedItems,
		PendingItems:   e.TotalItems - e.CompletedItems - e.FailedItems,
		StartedAt:      e.StartedAt,
		CompletedAt:    e.CompletedAt,
	}

	// Calculate progress percentage
	if e.TotalItems > 0 {
		resp.ProgressPct = float64(e.CompletedItems+e.FailedItems) / float64(e.TotalItems) * 100
	}

	// Calculate elapsed time
	if e.StartedAt != nil {
		var elapsed float64

		if e.CompletedAt != nil {
			// Finished experiment: use fixed duration from start to completion
			elapsed = e.CompletedAt.Sub(*e.StartedAt).Seconds()
		} else if e.Status == ExperimentStatusRunning {
			// Running experiment: use live elapsed time
			elapsed = time.Since(*e.StartedAt).Seconds()
		}

		// Only set elapsed if we calculated it (skip pending experiments without completion)
		if e.CompletedAt != nil || e.Status == ExperimentStatusRunning {
			resp.ElapsedSeconds = &elapsed
		}

		// Calculate ETA only for running experiments
		if e.Status == ExperimentStatusRunning {
			processed := e.CompletedItems + e.FailedItems
			if processed > 0 && e.TotalItems > processed {
				rate := elapsed / float64(processed)
				remaining := float64(e.TotalItems - processed)
				eta := rate * remaining
				resp.ETASeconds = &eta
			}
		}
	}

	return resp
}

// ExperimentItem represents an individual result from an experiment run.
type ExperimentItem struct {
	ID            uuid.UUID              `json:"id"`
	ExperimentID  uuid.UUID              `json:"experiment_id"`
	DatasetItemID *uuid.UUID             `json:"dataset_item_id,omitempty"`
	TraceID       *string                `json:"trace_id,omitempty"`
	Input         map[string]any `json:"input"`
	Output        any            `json:"output,omitempty"`
	Expected      any            `json:"expected,omitempty"`
	TrialNumber   int                    `json:"trial_number"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Error         *string                `json:"error,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

func NewExperimentItem(experimentID uuid.UUID, input map[string]any) *ExperimentItem {
	return &ExperimentItem{
		ID:           uid.New(),
		ExperimentID: experimentID,
		Input:        input,
		TrialNumber:  1,
		Metadata:     make(map[string]any),
		CreatedAt:    time.Now(),
	}
}

func (ei *ExperimentItem) Validate() []ValidationError {
	var errors []ValidationError

	if ei.Input == nil || len(ei.Input) == 0 {
		errors = append(errors, ValidationError{Field: "input", Message: "input is required"})
	}
	if ei.TrialNumber < 1 {
		errors = append(errors, ValidationError{Field: "trial_number", Message: "trial_number must be at least 1"})
	}

	return errors
}

// ExperimentItemScore represents a score submitted with an experiment item from SDK.
// These scores are computed by SDK evaluators and bundled with experiment items.
type ExperimentItemScore struct {
	Name          string                 `json:"name" binding:"required"`
	Value         *float64               `json:"value,omitempty"`
	Type          string                 `json:"type,omitempty"` // NUMERIC, CATEGORICAL, BOOLEAN
	StringValue   *string                `json:"string_value,omitempty"`
	Reason        *string                `json:"reason,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ScoringFailed *bool                  `json:"scoring_failed,omitempty"`
}

type CreateExperimentItemRequest struct {
	DatasetItemID *string                `json:"dataset_item_id,omitempty"`
	TraceID       *string                `json:"trace_id,omitempty"`
	Input         map[string]any `json:"input" binding:"required"`
	Output        any            `json:"output,omitempty"`
	Expected      any            `json:"expected,omitempty"`
	TrialNumber   *int                   `json:"trial_number,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	// Scores computed by SDK evaluators, bundled with the experiment item
	Scores []ExperimentItemScore `json:"scores,omitempty"`
	// Error message if task execution failed
	Error *string `json:"error,omitempty"`
}

type CreateExperimentItemsBatchRequest struct {
	Items []CreateExperimentItemRequest `json:"items" binding:"required,dive"`
}

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
	Error         *string        `json:"error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (ei *ExperimentItem) ToResponse() *ExperimentItemResponse {
	return &ExperimentItemResponse{
		ID:            ei.ID,
		ExperimentID:  ei.ExperimentID,
		DatasetItemID: ei.DatasetItemID,
		TraceID:       ei.TraceID,
		Input:         ei.Input,
		Output:        ei.Output,
		Expected:      ei.Expected,
		TrialNumber:   ei.TrialNumber,
		Metadata:      ei.Metadata,
		Error:         ei.Error,
		CreatedAt:     ei.CreatedAt,
	}
}

// ============================================================================
// Experiment Comparison Types
// ============================================================================

// ScoreAggregation holds statistical metrics for a score across experiment items.
type ScoreAggregation struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Count  uint64  `json:"count"`
}

// ScoreDiffType represents the type of score difference.
type ScoreDiffType string

const (
	ScoreDiffTypeNumeric     ScoreDiffType = "NUMERIC"
	ScoreDiffTypeCategorical ScoreDiffType = "CATEGORICAL"
)

// ScoreDiff represents the difference between a score and its baseline.
type ScoreDiff struct {
	Type        ScoreDiffType `json:"type"`
	Difference  float64       `json:"difference,omitempty"`   // Absolute difference for NUMERIC
	Direction   string        `json:"direction,omitempty"`    // "+" or "-" for NUMERIC
	IsDifferent bool          `json:"is_different,omitempty"` // For CATEGORICAL
}

// ExperimentSummary contains basic info about an experiment for comparison.
type ExperimentSummary struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	ItemCount int64  `json:"item_count"`
}

// CompareExperimentsRequest is the request body for comparing experiments.
type CompareExperimentsRequest struct {
	ExperimentIDs []string `json:"experiment_ids" binding:"required,min=2,max=10"`
	BaselineID    *string  `json:"baseline_id,omitempty"`
}

// CompareExperimentsResponse contains the comparison results.
type CompareExperimentsResponse struct {
	Experiments map[string]*ExperimentSummary           `json:"experiments"`
	Scores      map[string]map[string]*ScoreAggregation `json:"scores"`          // scoreName -> experimentID -> aggregation
	Diffs       map[string]map[string]*ScoreDiff        `json:"diffs,omitempty"` // scoreName -> experimentID -> diff (vs baseline)
}

// CalculateDiff computes the difference between two score aggregations.
func CalculateDiff(baseline, current *ScoreAggregation) *ScoreDiff {
	if baseline == nil || current == nil {
		return nil
	}

	difference := current.Mean - baseline.Mean
	direction := "+"
	if difference < 0 {
		direction = "-"
	}

	return &ScoreDiff{
		Type:       ScoreDiffTypeNumeric,
		Difference: abs(difference),
		Direction:  direction,
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ============================================================================
// Dataset Versioning Types
// ============================================================================

// DatasetVersion represents a snapshot of a dataset at a point in time.
// Versions are created automatically when items are added or removed.
type DatasetVersion struct {
	ID          uuid.UUID              `json:"id"`
	DatasetID   uuid.UUID              `json:"dataset_id"`
	Version     int                    `json:"version"`
	ItemCount   int                    `json:"item_count"`
	Description *string                `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedBy   *uuid.UUID             `json:"created_by,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// NewDatasetVersion creates a new dataset version.
func NewDatasetVersion(datasetID uuid.UUID, version int, itemCount int) *DatasetVersion {
	return &DatasetVersion{
		ID:        uid.New(),
		DatasetID: datasetID,
		Version:   version,
		ItemCount: itemCount,
		Metadata:  make(map[string]any),
		CreatedAt: time.Now(),
	}
}

func (dv *DatasetVersion) Validate() []ValidationError {
	var errors []ValidationError

	if dv.Version < 1 {
		errors = append(errors, ValidationError{Field: "version", Message: "version must be at least 1"})
	}
	if dv.ItemCount < 0 {
		errors = append(errors, ValidationError{Field: "item_count", Message: "item_count cannot be negative"})
	}

	return errors
}

// DatasetVersionResponse is the API response for a dataset version.
type DatasetVersionResponse struct {
	ID          uuid.UUID      `json:"id"`
	DatasetID   uuid.UUID      `json:"dataset_id"`
	Version     int            `json:"version"`
	ItemCount   int            `json:"item_count"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedBy   *uuid.UUID     `json:"created_by,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (dv *DatasetVersion) ToResponse() *DatasetVersionResponse {
	return &DatasetVersionResponse{
		ID:          dv.ID,
		DatasetID:   dv.DatasetID,
		Version:     dv.Version,
		ItemCount:   dv.ItemCount,
		Description: dv.Description,
		Metadata:    dv.Metadata,
		CreatedBy:   dv.CreatedBy,
		CreatedAt:   dv.CreatedAt,
	}
}

// DatasetItemVersion is the join table linking items to versions.
type DatasetItemVersion struct {
	DatasetVersionID uuid.UUID `json:"dataset_version_id"`
	DatasetItemID    uuid.UUID `json:"dataset_item_id"`
}

// CreateDatasetVersionRequest is the request to create a new version manually.
type CreateDatasetVersionRequest struct {
	Description *string                `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// PinDatasetVersionRequest is the request to pin a dataset to a specific version.
type PinDatasetVersionRequest struct {
	VersionID *uuid.UUID `json:"version_id"` // nil to unpin (use latest)
}

// DatasetVersionFilter is used for filtering versions.
type DatasetVersionFilter struct {
	DatasetID *uuid.UUID
}

// DatasetWithVersion extends Dataset to include version info in responses.
type DatasetWithVersionResponse struct {
	ID               uuid.UUID      `json:"id"`
	ProjectID        uuid.UUID      `json:"project_id"`
	Name             string         `json:"name"`
	Description      *string        `json:"description,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CurrentVersionID *uuid.UUID     `json:"current_version_id,omitempty"`
	CurrentVersion   *int           `json:"current_version,omitempty"`
	LatestVersion    *int           `json:"latest_version,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// ============================================================================
// Dataset Filter and Pagination Types
// ============================================================================

// ============================================================================
// Experiment Metrics Types
// ============================================================================

// ExperimentMetricsResponse is the response for GET /experiments/{id}/metrics.
// It provides comprehensive metrics including progress, performance, and scores.
type ExperimentMetricsResponse struct {
	ExperimentID uuid.UUID                    `json:"experiment_id"`
	Status       ExperimentStatus             `json:"status"`
	Progress     ExperimentProgressMetrics    `json:"progress"`
	Performance  ExperimentPerformanceMetrics `json:"performance"`
	Scores       map[string]*ScoreMetrics     `json:"scores,omitempty"`
}

// ExperimentProgressMetrics contains progress and success/error rate metrics.
type ExperimentProgressMetrics struct {
	TotalItems     int     `json:"total_items"`
	CompletedItems int     `json:"completed_items"`
	FailedItems    int     `json:"failed_items"`
	PendingItems   int     `json:"pending_items"`
	ProgressPct    float64 `json:"progress_pct"`
	SuccessRate    float64 `json:"success_rate"` // completed / (completed + failed) * 100
	ErrorRate      float64 `json:"error_rate"`   // failed / (completed + failed) * 100
}

// ExperimentPerformanceMetrics contains timing and ETA metrics.
type ExperimentPerformanceMetrics struct {
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	ElapsedSeconds *float64   `json:"elapsed_seconds,omitempty"`
	ETASeconds     *float64   `json:"eta_seconds,omitempty"`
}

// ScoreMetrics contains statistical metrics for a score type.
type ScoreMetrics struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Count  uint64  `json:"count"`
}

// DatasetWithItemCount extends Dataset to include item count in list responses.
type DatasetWithItemCount struct {
	Dataset
	ItemCount int64 `json:"item_count"`
}

// DatasetWithItemCountResponse is the API response for a dataset with item count.
type DatasetWithItemCountResponse struct {
	ID               uuid.UUID      `json:"id"`
	ProjectID        uuid.UUID      `json:"project_id"`
	Name             string         `json:"name"`
	Description      *string        `json:"description,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CurrentVersionID *uuid.UUID     `json:"current_version_id,omitempty"`
	ItemCount        int64          `json:"item_count"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// ToResponse converts DatasetWithItemCount to its API response format.
func (d *DatasetWithItemCount) ToResponse() *DatasetWithItemCountResponse {
	return &DatasetWithItemCountResponse{
		ID:               d.ID,
		ProjectID:        d.ProjectID,
		Name:             d.Name,
		Description:      d.Description,
		Metadata:         d.Metadata,
		CurrentVersionID: d.CurrentVersionID,
		ItemCount:        d.ItemCount,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}
}
