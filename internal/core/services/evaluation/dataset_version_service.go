package evaluation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"brokle/internal/core/domain/common"
	"brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
)

type datasetVersionService struct {
	transactor  common.Transactor
	versionRepo evaluation.DatasetVersionRepository
	datasetRepo evaluation.DatasetRepository
	itemRepo    evaluation.DatasetItemRepository
	logger      *slog.Logger
}

func NewDatasetVersionService(
	transactor common.Transactor,
	versionRepo evaluation.DatasetVersionRepository,
	datasetRepo evaluation.DatasetRepository,
	itemRepo evaluation.DatasetItemRepository,
	logger *slog.Logger,
) evaluation.DatasetVersionService {
	return &datasetVersionService{
		transactor:  transactor,
		versionRepo: versionRepo,
		datasetRepo: datasetRepo,
		itemRepo:    itemRepo,
		logger:      logger,
	}
}

// CreateVersion creates a new version snapshot of the current dataset items
func (s *datasetVersionService) CreateVersion(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *evaluation.CreateDatasetVersionRequest) (*evaluation.DatasetVersion, error) {
	// Verify dataset exists (outside transaction - read-only)
	_, err := s.datasetRepo.GetByID(ctx, datasetID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	// Get next version number (outside transaction - read-only)
	nextVersion, err := s.versionRepo.GetNextVersionNumber(ctx, datasetID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get next version number", err)
	}

	// Create version entity with placeholder ItemCount (will be set inside transaction)
	version := evaluation.NewDatasetVersion(datasetID, nextVersion, 0)
	if req != nil {
		version.Description = req.Description
		if req.Metadata != nil {
			version.Metadata = req.Metadata
		}
	}

	// Wrap version creation and item association in a transaction
	// to ensure ItemCount matches the actual associated items
	err = s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		// Get all current items FIRST inside transaction to ensure consistency
		items, err := s.itemRepo.ListAll(txCtx, datasetID)
		if err != nil {
			return appErrors.NewInternalError("failed to list items", err)
		}

		// Derive ItemCount from actual items to prevent race condition
		version.ItemCount = len(items)

		// Validate after setting ItemCount
		if validationErrors := version.Validate(); len(validationErrors) > 0 {
			return appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
		}

		// Create the version record
		if err := s.versionRepo.Create(txCtx, version); err != nil {
			if errors.Is(err, evaluation.ErrDatasetVersionExists) {
				return appErrors.NewConflictError("version already exists")
			}
			return appErrors.NewInternalError("failed to create version", err)
		}

		// Associate items with this version
		if len(items) > 0 {
			itemIDs := make([]uuid.UUID, len(items))
			for i, item := range items {
				itemIDs[i] = item.ID
			}
			if err := s.versionRepo.AddItems(txCtx, version.ID, itemIDs); err != nil {
				return appErrors.NewInternalError("failed to associate items with version", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logger.Info("dataset version created",
		"version_id", version.ID,
		"dataset_id", datasetID,
		"version", version.Version,
		"item_count", version.ItemCount,
	)

	return version, nil
}

// GetVersion gets a specific version by ID
func (s *datasetVersionService) GetVersion(ctx context.Context, versionID uuid.UUID, datasetID uuid.UUID, projectID uuid.UUID) (*evaluation.DatasetVersion, error) {
	// Verify dataset exists and belongs to project
	_, err := s.datasetRepo.GetByID(ctx, datasetID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	version, err := s.versionRepo.GetByID(ctx, versionID, datasetID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetVersionNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %s", versionID))
		}
		return nil, appErrors.NewInternalError("failed to get version", err)
	}

	return version, nil
}

// ListVersions lists all versions for a dataset
func (s *datasetVersionService) ListVersions(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) ([]*evaluation.DatasetVersion, error) {
	// Verify dataset exists and belongs to project
	_, err := s.datasetRepo.GetByID(ctx, datasetID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	versions, err := s.versionRepo.List(ctx, datasetID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list versions", err)
	}

	return versions, nil
}

// GetLatestVersion gets the most recent version
func (s *datasetVersionService) GetLatestVersion(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) (*evaluation.DatasetVersion, error) {
	// Verify dataset exists and belongs to project
	_, err := s.datasetRepo.GetByID(ctx, datasetID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	version, err := s.versionRepo.GetLatest(ctx, datasetID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetVersionNotFound) {
			return nil, appErrors.NewNotFoundError("no versions exist for this dataset")
		}
		return nil, appErrors.NewInternalError("failed to get latest version", err)
	}

	return version, nil
}

// GetVersionItems gets items for a specific version with pagination
func (s *datasetVersionService) GetVersionItems(ctx context.Context, versionID uuid.UUID, datasetID uuid.UUID, projectID uuid.UUID, limit, offset int) ([]*evaluation.DatasetItem, int64, error) {
	// Verify dataset exists and belongs to project
	_, err := s.datasetRepo.GetByID(ctx, datasetID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, 0, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, 0, appErrors.NewInternalError("failed to get dataset", err)
	}

	// Verify version exists
	_, err = s.versionRepo.GetByID(ctx, versionID, datasetID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetVersionNotFound) {
			return nil, 0, appErrors.NewNotFoundError(fmt.Sprintf("version %s", versionID))
		}
		return nil, 0, appErrors.NewInternalError("failed to get version", err)
	}

	items, total, err := s.versionRepo.GetItems(ctx, versionID, limit, offset)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to get version items", err)
	}

	return items, total, nil
}

// PinVersion pins the dataset to a specific version (nil to unpin)
func (s *datasetVersionService) PinVersion(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, versionID *uuid.UUID) (*evaluation.Dataset, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	// If pinning to a specific version, verify it exists
	if versionID != nil {
		_, err := s.versionRepo.GetByID(ctx, *versionID, datasetID)
		if err != nil {
			if errors.Is(err, evaluation.ErrDatasetVersionNotFound) {
				return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %s", *versionID))
			}
			return nil, appErrors.NewInternalError("failed to get version", err)
		}
	}

	dataset.CurrentVersionID = versionID
	if err := s.datasetRepo.Update(ctx, dataset, projectID); err != nil {
		return nil, appErrors.NewInternalError("failed to update dataset", err)
	}

	action := "unpinned from version"
	if versionID != nil {
		action = fmt.Sprintf("pinned to version %s", versionID)
	}
	s.logger.Info("dataset version "+action,
		"dataset_id", datasetID,
		"project_id", projectID,
		"version_id", versionID,
	)

	return dataset, nil
}

// GetDatasetWithVersionInfo gets a dataset with its version information
func (s *datasetVersionService) GetDatasetWithVersionInfo(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) (*evaluation.DatasetWithVersionResponse, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	response := &evaluation.DatasetWithVersionResponse{
		ID:          dataset.ID,
		ProjectID:   dataset.ProjectID,
		Name:        dataset.Name,
		Description: dataset.Description,
		Metadata:    dataset.Metadata,
		CreatedAt:   dataset.CreatedAt,
		UpdatedAt:   dataset.UpdatedAt,
	}

	// Add current version ID if pinned
	if dataset.CurrentVersionID != nil {
		response.CurrentVersionID = dataset.CurrentVersionID

		// Get the pinned version number
		version, err := s.versionRepo.GetByID(ctx, *dataset.CurrentVersionID, datasetID)
		if err == nil {
			response.CurrentVersion = &version.Version
		}
	}

	// Get the latest version number
	latestVersion, err := s.versionRepo.GetLatest(ctx, datasetID)
	if err == nil {
		response.LatestVersion = &latestVersion.Version
	}

	return response, nil
}
