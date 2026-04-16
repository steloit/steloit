package evaluation

import (
	"context"

	"github.com/google/uuid"

	"brokle/pkg/pagination"
)

type ScoreConfigService interface {
	Create(ctx context.Context, projectID uuid.UUID, req *CreateScoreConfigRequest) (*ScoreConfig, error)
	Update(ctx context.Context, id uuid.UUID, projectID uuid.UUID, req *UpdateScoreConfigRequest) (*ScoreConfig, error)
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*ScoreConfig, error)
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (*ScoreConfig, error)
	List(ctx context.Context, projectID uuid.UUID, page, limit int) ([]*ScoreConfig, int64, error)
}

type DatasetService interface {
	Create(ctx context.Context, projectID uuid.UUID, req *CreateDatasetRequest) (*Dataset, error)
	Update(ctx context.Context, id uuid.UUID, projectID uuid.UUID, req *UpdateDatasetRequest) (*Dataset, error)
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*Dataset, error)
	List(ctx context.Context, projectID uuid.UUID, filter *DatasetFilter, page, limit int) ([]*Dataset, int64, error)
	ListWithFilters(ctx context.Context, projectID uuid.UUID, filter *DatasetFilter, params pagination.Params) ([]*DatasetWithItemCount, int64, error)
}

type DatasetItemService interface {
	Create(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *CreateDatasetItemRequest) (*DatasetItem, error)
	CreateBatch(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *CreateDatasetItemsBatchRequest) (int, error)
	List(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, limit, offset int) ([]*DatasetItem, int64, error)
	Delete(ctx context.Context, id uuid.UUID, datasetID uuid.UUID, projectID uuid.UUID) error

	// Bulk import methods
	ImportFromJSON(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *ImportDatasetItemsFromJSONRequest) (*BulkImportResult, error)
	ImportFromCSV(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *ImportDatasetItemsFromCSVRequest) (*BulkImportResult, error)
	CreateFromTraces(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *CreateDatasetItemsFromTracesRequest) (*BulkImportResult, error)
	CreateFromSpans(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *CreateDatasetItemsFromSpansRequest) (*BulkImportResult, error)

	// Export method
	ExportItems(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) ([]*DatasetItem, error)
}

type DatasetVersionService interface {
	// CreateVersion creates a new version snapshot of the current dataset items
	CreateVersion(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *CreateDatasetVersionRequest) (*DatasetVersion, error)
	// GetVersion gets a specific version by ID
	GetVersion(ctx context.Context, versionID uuid.UUID, datasetID uuid.UUID, projectID uuid.UUID) (*DatasetVersion, error)
	// ListVersions lists all versions for a dataset
	ListVersions(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) ([]*DatasetVersion, error)
	// GetLatestVersion gets the most recent version
	GetLatestVersion(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) (*DatasetVersion, error)
	// GetVersionItems gets items for a specific version with pagination
	GetVersionItems(ctx context.Context, versionID uuid.UUID, datasetID uuid.UUID, projectID uuid.UUID, limit, offset int) ([]*DatasetItem, int64, error)
	// PinVersion pins the dataset to a specific version (nil to unpin)
	PinVersion(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, versionID *uuid.UUID) (*Dataset, error)
	// GetDatasetWithVersionInfo gets a dataset with its version information
	GetDatasetWithVersionInfo(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) (*DatasetWithVersionResponse, error)
}

type ExperimentService interface {
	Create(ctx context.Context, projectID uuid.UUID, req *CreateExperimentRequest) (*Experiment, error)
	Update(ctx context.Context, id uuid.UUID, projectID uuid.UUID, req *UpdateExperimentRequest) (*Experiment, error)
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*Experiment, error)
	List(ctx context.Context, projectID uuid.UUID, filter *ExperimentFilter, page, limit int) ([]*Experiment, int64, error)

	// CompareExperiments compares score metrics across multiple experiments
	CompareExperiments(ctx context.Context, projectID uuid.UUID, experimentIDs []uuid.UUID, baselineID *uuid.UUID) (*CompareExperimentsResponse, error)

	// Rerun creates a new experiment based on an existing one, using the same dataset.
	// The new experiment starts in pending status ready for SDK to run with a new task function.
	Rerun(ctx context.Context, sourceID uuid.UUID, projectID uuid.UUID, req *RerunExperimentRequest) (*Experiment, error)

	// Progress tracking methods
	// GetProgress returns the current progress for an experiment
	GetProgress(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*ExperimentProgressResponse, error)
	// SetTotalItems sets the total number of items for an experiment
	SetTotalItems(ctx context.Context, id uuid.UUID, projectID uuid.UUID, total int) error
	// IncrementProgress atomically increments completed and/or failed counters
	IncrementProgress(ctx context.Context, id uuid.UUID, projectID uuid.UUID, completed, failed int) error
	// IncrementAndCheckCompletion atomically increments counters and checks if experiment is complete.
	// Returns true if the experiment was marked as complete.
	IncrementAndCheckCompletion(ctx context.Context, id uuid.UUID, projectID uuid.UUID, completed, failed int) (bool, error)

	// GetMetrics returns comprehensive metrics for an experiment including progress,
	// performance, and score aggregations from ClickHouse.
	GetMetrics(ctx context.Context, projectID, experimentID uuid.UUID) (*ExperimentMetricsResponse, error)
}

type ExperimentItemService interface {
	CreateBatch(ctx context.Context, experimentID uuid.UUID, projectID uuid.UUID, req *CreateExperimentItemsBatchRequest) (int, error)
	List(ctx context.Context, experimentID uuid.UUID, projectID uuid.UUID, limit, offset int) ([]*ExperimentItem, int64, error)
}

// ExperimentWizardService handles the creation and configuration of experiments via the dashboard wizard.
type ExperimentWizardService interface {
	// CreateFromWizard creates a new experiment from the wizard configuration.
	// It creates both the experiment and its associated config, and optionally runs it immediately.
	CreateFromWizard(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *CreateExperimentFromWizardRequest) (*Experiment, error)

	// ValidateStep validates a specific step of the wizard.
	ValidateStep(ctx context.Context, projectID uuid.UUID, req *ValidateStepRequest) (*ValidateStepResponse, error)

	// EstimateCost estimates the cost of running an experiment with the given configuration.
	EstimateCost(ctx context.Context, projectID uuid.UUID, req *EstimateCostRequest) (*EstimateCostResponse, error)

	// GetDatasetFields returns the schema of dataset fields for variable mapping.
	GetDatasetFields(ctx context.Context, projectID uuid.UUID, datasetID uuid.UUID) (*DatasetFieldsResponse, error)

	// GetExperimentConfig returns the config for a specific experiment.
	GetExperimentConfig(ctx context.Context, experimentID uuid.UUID, projectID uuid.UUID) (*ExperimentConfig, error)
}

type EvaluatorService interface {
	Create(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *CreateEvaluatorRequest) (*Evaluator, error)
	Update(ctx context.Context, id uuid.UUID, projectID uuid.UUID, req *UpdateEvaluatorRequest) (*Evaluator, error)
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*Evaluator, error)
	List(ctx context.Context, projectID uuid.UUID, filter *EvaluatorFilter, params pagination.Params) ([]*Evaluator, int64, error)
	Activate(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	Deactivate(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	GetActiveByProjectID(ctx context.Context, projectID uuid.UUID) ([]*Evaluator, error)

	// TriggerEvaluator starts a manual evaluation of the evaluator against matching spans
	TriggerEvaluator(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID, opts *TriggerOptions) (*TriggerResponse, error)

	// TestEvaluator executes an evaluator against sample spans for testing/preview without persisting scores.
	// This allows users to validate evaluator configuration before activation.
	TestEvaluator(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID, req *TestEvaluatorRequest) (*TestEvaluatorResponse, error)

	// GetAnalytics returns performance analytics for an evaluator over the specified time period.
	GetAnalytics(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID, params *EvaluatorAnalyticsParams) (*EvaluatorAnalyticsResponse, error)
}

type EvaluatorExecutionService interface {
	StartExecution(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID, triggerType TriggerType) (*EvaluatorExecution, error)
	CompleteExecution(ctx context.Context, executionID uuid.UUID, projectID uuid.UUID, spansMatched, spansScored, errorsCount int) error
	FailExecution(ctx context.Context, executionID uuid.UUID, projectID uuid.UUID, errorMessage string) error
	CancelExecution(ctx context.Context, executionID uuid.UUID, projectID uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*EvaluatorExecution, error)
	ListByEvaluatorID(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID, filter *ExecutionFilter, params pagination.Params) ([]*EvaluatorExecution, int64, error)
	GetLatestByEvaluatorID(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID) (*EvaluatorExecution, error)

	// IncrementCounters atomically increments spans_scored and errors_count for an execution (used by workers)
	IncrementCounters(ctx context.Context, executionID string, projectID uuid.UUID, spansScored, errorsCount int) error

	// StartExecutionWithCount creates an execution with known spans_matched count upfront.
	// Used for automatic evaluations where we know the count before emitting jobs.
	StartExecutionWithCount(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID, triggerType TriggerType, spansMatched int) (*EvaluatorExecution, error)

	// IncrementAndCheckCompletion atomically increments counters and marks execution as complete
	// if all spans have been processed. Returns true if execution was marked complete.
	IncrementAndCheckCompletion(ctx context.Context, executionID uuid.UUID, projectID uuid.UUID, spansScored, errorsCount int) (bool, error)

	// UpdateSpansMatched updates the spans_matched count for an execution.
	// Used by manual triggers after discovering how many spans will be processed.
	UpdateSpansMatched(ctx context.Context, executionID uuid.UUID, projectID uuid.UUID, spansMatched int) error

	// GetExecutionDetail returns detailed execution information including span-level results.
	// The evaluatorID parameter ensures the execution belongs to the specified evaluator.
	GetExecutionDetail(ctx context.Context, executionID uuid.UUID, projectID uuid.UUID, evaluatorID uuid.UUID) (*ExecutionDetailResponse, error)
}
