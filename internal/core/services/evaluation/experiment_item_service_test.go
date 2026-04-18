package evaluation

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockExperimentRepository struct {
	mock.Mock
}

func (m *MockExperimentRepository) Create(ctx context.Context, experiment *evaluation.Experiment) error {
	args := m.Called(ctx, experiment)
	return args.Error(0)
}

func (m *MockExperimentRepository) GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.Experiment, error) {
	args := m.Called(ctx, id, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*evaluation.Experiment), args.Error(1)
}

func (m *MockExperimentRepository) List(ctx context.Context, projectID uuid.UUID, filter *evaluation.ExperimentFilter, offset, limit int) ([]*evaluation.Experiment, int64, error) {
	args := m.Called(ctx, projectID, filter, offset, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*evaluation.Experiment), args.Get(1).(int64), args.Error(2)
}

func (m *MockExperimentRepository) Update(ctx context.Context, experiment *evaluation.Experiment, projectID uuid.UUID) error {
	args := m.Called(ctx, experiment, projectID)
	return args.Error(0)
}

func (m *MockExperimentRepository) Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error {
	args := m.Called(ctx, id, projectID)
	return args.Error(0)
}

func (m *MockExperimentRepository) SetTotalItems(ctx context.Context, id, projectID uuid.UUID, total int) error {
	args := m.Called(ctx, id, projectID, total)
	return args.Error(0)
}

func (m *MockExperimentRepository) IncrementCounters(ctx context.Context, id, projectID uuid.UUID, completed, failed int) error {
	args := m.Called(ctx, id, projectID, completed, failed)
	return args.Error(0)
}

func (m *MockExperimentRepository) IncrementCountersAndUpdateStatus(ctx context.Context, id, projectID uuid.UUID, completed, failed int) (bool, error) {
	args := m.Called(ctx, id, projectID, completed, failed)
	return args.Bool(0), args.Error(1)
}

func (m *MockExperimentRepository) GetProgress(ctx context.Context, id, projectID uuid.UUID) (*evaluation.Experiment, error) {
	args := m.Called(ctx, id, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*evaluation.Experiment), args.Error(1)
}

type MockExperimentItemRepository struct {
	mock.Mock
}

func (m *MockExperimentItemRepository) Create(ctx context.Context, item *evaluation.ExperimentItem) error {
	args := m.Called(ctx, item)
	return args.Error(0)
}

func (m *MockExperimentItemRepository) CreateBatch(ctx context.Context, items []*evaluation.ExperimentItem) error {
	args := m.Called(ctx, items)
	return args.Error(0)
}

func (m *MockExperimentItemRepository) List(ctx context.Context, experimentID uuid.UUID, limit, offset int) ([]*evaluation.ExperimentItem, int64, error) {
	args := m.Called(ctx, experimentID, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*evaluation.ExperimentItem), args.Get(1).(int64), args.Error(2)
}

func (m *MockExperimentItemRepository) CountByExperiment(ctx context.Context, experimentID uuid.UUID) (int64, error) {
	args := m.Called(ctx, experimentID)
	return args.Get(0).(int64), args.Error(1)
}

type MockDatasetItemRepository struct {
	mock.Mock
}

func (m *MockDatasetItemRepository) Create(ctx context.Context, item *evaluation.DatasetItem) error {
	args := m.Called(ctx, item)
	return args.Error(0)
}

func (m *MockDatasetItemRepository) CreateBatch(ctx context.Context, items []*evaluation.DatasetItem) error {
	args := m.Called(ctx, items)
	return args.Error(0)
}

func (m *MockDatasetItemRepository) GetByID(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) (*evaluation.DatasetItem, error) {
	args := m.Called(ctx, id, datasetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*evaluation.DatasetItem), args.Error(1)
}

func (m *MockDatasetItemRepository) GetByIDForProject(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.DatasetItem, error) {
	args := m.Called(ctx, id, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*evaluation.DatasetItem), args.Error(1)
}

func (m *MockDatasetItemRepository) List(ctx context.Context, datasetID uuid.UUID, limit, offset int) ([]*evaluation.DatasetItem, int64, error) {
	args := m.Called(ctx, datasetID, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*evaluation.DatasetItem), args.Get(1).(int64), args.Error(2)
}

func (m *MockDatasetItemRepository) ListAll(ctx context.Context, datasetID uuid.UUID) ([]*evaluation.DatasetItem, error) {
	args := m.Called(ctx, datasetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*evaluation.DatasetItem), args.Error(1)
}

func (m *MockDatasetItemRepository) Delete(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) error {
	args := m.Called(ctx, id, datasetID)
	return args.Error(0)
}

func (m *MockDatasetItemRepository) CountByDataset(ctx context.Context, datasetID uuid.UUID) (int64, error) {
	args := m.Called(ctx, datasetID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockDatasetItemRepository) FindByContentHash(ctx context.Context, datasetID uuid.UUID, contentHash string) (*evaluation.DatasetItem, error) {
	args := m.Called(ctx, datasetID, contentHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*evaluation.DatasetItem), args.Error(1)
}

func (m *MockDatasetItemRepository) FindByContentHashes(ctx context.Context, datasetID uuid.UUID, contentHashes []string) (map[string]bool, error) {
	args := m.Called(ctx, datasetID, contentHashes)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]bool), args.Error(1)
}

// MockScoreService mocks the observability.ScoreService interface
type MockScoreService struct {
	mock.Mock
}

func (m *MockScoreService) CreateScore(ctx context.Context, score *observability.Score) error {
	args := m.Called(ctx, score)
	return args.Error(0)
}

func (m *MockScoreService) CreateScoreBatch(ctx context.Context, scores []*observability.Score) error {
	args := m.Called(ctx, scores)
	return args.Error(0)
}

func (m *MockScoreService) GetScoreByID(ctx context.Context, id uuid.UUID) (*observability.Score, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.Score), args.Error(1)
}

func (m *MockScoreService) GetScoresByTraceID(ctx context.Context, traceID string) ([]*observability.Score, error) {
	args := m.Called(ctx, traceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Score), args.Error(1)
}

func (m *MockScoreService) GetScoresBySpanID(ctx context.Context, spanID string) ([]*observability.Score, error) {
	args := m.Called(ctx, spanID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Score), args.Error(1)
}

func (m *MockScoreService) GetScoresByFilter(ctx context.Context, filter *observability.ScoreFilter) ([]*observability.Score, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Score), args.Error(1)
}

func (m *MockScoreService) UpdateScore(ctx context.Context, score *observability.Score) error {
	args := m.Called(ctx, score)
	return args.Error(0)
}

func (m *MockScoreService) DeleteScore(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockScoreService) CountScores(ctx context.Context, filter *observability.ScoreFilter) (int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(int64), args.Error(1)
}

func TestExperimentItemService_CreateBatch(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	projectID := uid.New()
	experimentID := uid.New()
	datasetID := uid.New()
	datasetItemID := uid.New()

	t.Run("success with valid dataset item from experiment's dataset", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: &datasetID,
			Name:      "Test Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		datasetItem := &evaluation.DatasetItem{
			ID:        datasetItemID,
			DatasetID: datasetID,
			Input:     map[string]interface{}{"prompt": "test"},
		}

		datasetItemIDStr := datasetItemID.String()
		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					DatasetItemID: &datasetItemIDStr,
					Input:         map[string]interface{}{"prompt": "test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)
		datasetItemRepo.On("GetByID", ctx, datasetItemID, datasetID).Return(datasetItem, nil)
		itemRepo.On("CreateBatch", ctx, mock.AnythingOfType("[]*evaluation.ExperimentItem")).Return(nil)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.NoError(t, err)
		assert.Equal(t, 1, count)
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertExpectations(t)
		itemRepo.AssertExpectations(t)
	})

	t.Run("reject dataset item from different dataset", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: &datasetID,
			Name:      "Test Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		differentDatasetItemID := uid.New()
		differentDatasetItemIDStr := differentDatasetItemID.String()

		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					DatasetItemID: &differentDatasetItemIDStr,
					Input:         map[string]interface{}{"prompt": "test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)
		datasetItemRepo.On("GetByID", ctx, differentDatasetItemID, datasetID).Return(nil, evaluation.ErrDatasetItemNotFound)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, appErrors.ValidationError, appErr.Type)
		assert.Contains(t, appErr.Details, "not found in experiment's dataset")
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertExpectations(t)
		itemRepo.AssertNotCalled(t, "CreateBatch")
	})

	t.Run("reject dataset item when experiment has no dataset", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: nil,
			Name:      "Ad-hoc Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		datasetItemIDStr := datasetItemID.String()
		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					DatasetItemID: &datasetItemIDStr,
					Input:         map[string]interface{}{"prompt": "test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, appErrors.ValidationError, appErr.Type)
		assert.Contains(t, appErr.Details, "cannot reference dataset items when experiment has no dataset")
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertNotCalled(t, "GetByID")
		itemRepo.AssertNotCalled(t, "CreateBatch")
	})

	t.Run("success without dataset item reference", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: &datasetID,
			Name:      "Test Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					DatasetItemID: nil,
					Input:         map[string]interface{}{"prompt": "ad-hoc test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)
		itemRepo.On("CreateBatch", ctx, mock.AnythingOfType("[]*evaluation.ExperimentItem")).Return(nil)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.NoError(t, err)
		assert.Equal(t, 1, count)
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertNotCalled(t, "GetByID")
		itemRepo.AssertExpectations(t)
	})

	t.Run("success without dataset item reference when experiment has no dataset", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: nil,
			Name:      "Ad-hoc Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					DatasetItemID: nil,
					Input:         map[string]interface{}{"prompt": "ad-hoc test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)
		itemRepo.On("CreateBatch", ctx, mock.AnythingOfType("[]*evaluation.ExperimentItem")).Return(nil)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.NoError(t, err)
		assert.Equal(t, 1, count)
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertNotCalled(t, "GetByID")
		itemRepo.AssertExpectations(t)
	})

	t.Run("reject non-existent dataset item", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: &datasetID,
			Name:      "Test Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		nonExistentItemID := uid.New()
		nonExistentItemIDStr := nonExistentItemID.String()
		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					DatasetItemID: &nonExistentItemIDStr,
					Input:         map[string]interface{}{"prompt": "test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)
		datasetItemRepo.On("GetByID", ctx, nonExistentItemID, datasetID).Return(nil, evaluation.ErrDatasetItemNotFound)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, appErrors.ValidationError, appErr.Type)
		assert.Contains(t, appErr.Details, "not found in experiment's dataset")
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertExpectations(t)
		itemRepo.AssertNotCalled(t, "CreateBatch")
	})

	t.Run("reject invalid dataset item ID format", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: &datasetID,
			Name:      "Test Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		invalidID := "not-a-valid-uuid"
		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					DatasetItemID: &invalidID,
					Input:         map[string]interface{}{"prompt": "test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, appErrors.ValidationError, appErr.Type)
		assert.Contains(t, appErr.Details, "must be a valid UUID")
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertNotCalled(t, "GetByID")
		itemRepo.AssertNotCalled(t, "CreateBatch")
	})

	t.Run("reject experiment not found", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{
				{
					Input: map[string]interface{}{"prompt": "test"},
				},
			},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(nil, evaluation.ErrExperimentNotFound)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, appErrors.NotFoundError, appErr.Type)
		assert.Contains(t, appErr.Message, "experiment")
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertNotCalled(t, "GetByID")
		itemRepo.AssertNotCalled(t, "CreateBatch")
	})

	t.Run("reject empty items array", func(t *testing.T) {
		experimentRepo := new(MockExperimentRepository)
		itemRepo := new(MockExperimentItemRepository)
		datasetItemRepo := new(MockDatasetItemRepository)
		scoreService := new(MockScoreService)

		service := NewExperimentItemService(itemRepo, experimentRepo, datasetItemRepo, scoreService, logger)

		experiment := &evaluation.Experiment{
			ID:        experimentID,
			ProjectID: projectID,
			DatasetID: &datasetID,
			Name:      "Test Experiment",
			Status:    evaluation.ExperimentStatusRunning,
		}

		req := &evaluation.CreateExperimentItemsBatchRequest{
			Items: []evaluation.CreateExperimentItemRequest{},
		}

		experimentRepo.On("GetByID", ctx, experimentID, projectID).Return(experiment, nil)

		count, err := service.CreateBatch(ctx, experimentID, projectID, req)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, appErrors.ValidationError, appErr.Type)
		assert.Contains(t, appErr.Details, "items array cannot be empty")
		experimentRepo.AssertExpectations(t)
		datasetItemRepo.AssertNotCalled(t, "GetByID")
		itemRepo.AssertNotCalled(t, "CreateBatch")
	})
}
