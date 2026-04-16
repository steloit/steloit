package evaluation

import (
	"context"

	"github.com/google/uuid"

	"brokle/pkg/pagination"
)

type ScoreConfigRepository interface {
	Create(ctx context.Context, config *ScoreConfig) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*ScoreConfig, error)
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (*ScoreConfig, error)
	List(ctx context.Context, projectID uuid.UUID, offset, limit int) ([]*ScoreConfig, int64, error)
	Update(ctx context.Context, config *ScoreConfig, projectID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error)
}

type DatasetRepository interface {
	Create(ctx context.Context, dataset *Dataset) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*Dataset, error)
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (*Dataset, error)
	List(ctx context.Context, projectID uuid.UUID, filter *DatasetFilter, offset, limit int) ([]*Dataset, int64, error)
	ListWithFilters(ctx context.Context, projectID uuid.UUID, filter *DatasetFilter, params pagination.Params) ([]*DatasetWithItemCount, int64, error)
	Update(ctx context.Context, dataset *Dataset, projectID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error)
}

type DatasetItemRepository interface {
	Create(ctx context.Context, item *DatasetItem) error
	CreateBatch(ctx context.Context, items []*DatasetItem) error
	GetByID(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) (*DatasetItem, error)
	GetByIDForProject(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*DatasetItem, error)
	List(ctx context.Context, datasetID uuid.UUID, limit, offset int) ([]*DatasetItem, int64, error)
	ListAll(ctx context.Context, datasetID uuid.UUID) ([]*DatasetItem, error)
	Delete(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) error
	CountByDataset(ctx context.Context, datasetID uuid.UUID) (int64, error)
	FindByContentHash(ctx context.Context, datasetID uuid.UUID, contentHash string) (*DatasetItem, error)
	FindByContentHashes(ctx context.Context, datasetID uuid.UUID, contentHashes []string) (map[string]bool, error)
}

type DatasetVersionRepository interface {
	// Create creates a new dataset version
	Create(ctx context.Context, version *DatasetVersion) error
	// GetByID gets a version by its ID
	GetByID(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) (*DatasetVersion, error)
	// GetByVersionNumber gets a version by dataset ID and version number
	GetByVersionNumber(ctx context.Context, datasetID uuid.UUID, versionNum int) (*DatasetVersion, error)
	// GetLatest gets the latest version for a dataset
	GetLatest(ctx context.Context, datasetID uuid.UUID) (*DatasetVersion, error)
	// List lists all versions for a dataset
	List(ctx context.Context, datasetID uuid.UUID) ([]*DatasetVersion, error)
	// GetNextVersionNumber returns the next version number for a dataset
	GetNextVersionNumber(ctx context.Context, datasetID uuid.UUID) (int, error)

	// Item-Version associations
	// AddItems associates items with a version (batch)
	AddItems(ctx context.Context, versionID uuid.UUID, itemIDs []uuid.UUID) error
	// GetItemIDs gets all item IDs for a version
	GetItemIDs(ctx context.Context, versionID uuid.UUID) ([]uuid.UUID, error)
	// GetItems gets all items for a version with pagination
	GetItems(ctx context.Context, versionID uuid.UUID, limit, offset int) ([]*DatasetItem, int64, error)
}

type ExperimentRepository interface {
	Create(ctx context.Context, experiment *Experiment) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*Experiment, error)
	List(ctx context.Context, projectID uuid.UUID, filter *ExperimentFilter, offset, limit int) ([]*Experiment, int64, error)
	Update(ctx context.Context, experiment *Experiment, projectID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error

	// Progress tracking methods
	// SetTotalItems sets the total number of items for an experiment
	SetTotalItems(ctx context.Context, id, projectID uuid.UUID, total int) error
	// IncrementCounters atomically increments completed and/or failed counters
	IncrementCounters(ctx context.Context, id, projectID uuid.UUID, completed, failed int) error
	// IncrementCountersAndUpdateStatus atomically increments counters and updates status if complete.
	// Returns true if the experiment was marked as complete (completed, failed, or partial).
	IncrementCountersAndUpdateStatus(ctx context.Context, id, projectID uuid.UUID, completed, failed int) (bool, error)
	// GetProgress gets minimal experiment data for progress polling
	GetProgress(ctx context.Context, id, projectID uuid.UUID) (*Experiment, error)
}

type ExperimentItemRepository interface {
	Create(ctx context.Context, item *ExperimentItem) error
	CreateBatch(ctx context.Context, items []*ExperimentItem) error
	List(ctx context.Context, experimentID uuid.UUID, limit, offset int) ([]*ExperimentItem, int64, error)
	CountByExperiment(ctx context.Context, experimentID uuid.UUID) (int64, error)
}

// ExperimentConfigRepository handles persistence for experiment configurations created via the wizard.
type ExperimentConfigRepository interface {
	// Create creates a new experiment config
	Create(ctx context.Context, config *ExperimentConfig) error
	// GetByID gets an experiment config by its ID
	GetByID(ctx context.Context, id uuid.UUID) (*ExperimentConfig, error)
	// GetByExperimentID gets the config for a specific experiment
	GetByExperimentID(ctx context.Context, experimentID uuid.UUID) (*ExperimentConfig, error)
	// Update updates an existing experiment config
	Update(ctx context.Context, config *ExperimentConfig) error
	// Delete deletes an experiment config
	Delete(ctx context.Context, id uuid.UUID) error
}

type EvaluatorRepository interface {
	Create(ctx context.Context, evaluator *Evaluator) error
	Update(ctx context.Context, evaluator *Evaluator) error
	Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*Evaluator, error)
	GetByProjectID(ctx context.Context, projectID uuid.UUID, filter *EvaluatorFilter, params pagination.Params) ([]*Evaluator, int64, error)
	GetActiveByProjectID(ctx context.Context, projectID uuid.UUID) ([]*Evaluator, error)
	ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error)
}

type EvaluatorExecutionRepository interface {
	Create(ctx context.Context, execution *EvaluatorExecution) error
	Update(ctx context.Context, execution *EvaluatorExecution) error
	GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*EvaluatorExecution, error)
	GetByEvaluatorID(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID, filter *ExecutionFilter, params pagination.Params) ([]*EvaluatorExecution, int64, error)
	GetLatestByEvaluatorID(ctx context.Context, evaluatorID uuid.UUID, projectID uuid.UUID) (*EvaluatorExecution, error)

	// IncrementCounters atomically increments spans_scored and errors_count counters
	IncrementCounters(ctx context.Context, id uuid.UUID, projectID uuid.UUID, spansScored, errorsCount int) error

	// IncrementCountersAndComplete atomically increments counters and marks execution as completed
	// if spans_scored + errors_count >= spans_matched. Returns true if execution was marked complete.
	IncrementCountersAndComplete(ctx context.Context, id uuid.UUID, projectID uuid.UUID, spansScored, errorsCount int) (bool, error)

	// UpdateSpansMatched updates only the spans_matched field for an execution.
	// Used by manual triggers after discovering how many spans will be processed.
	UpdateSpansMatched(ctx context.Context, id uuid.UUID, projectID uuid.UUID, spansMatched int) error
}
