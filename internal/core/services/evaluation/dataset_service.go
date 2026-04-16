package evaluation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/pagination"
)

type datasetService struct {
	repo   evaluation.DatasetRepository
	logger *slog.Logger
}

func NewDatasetService(
	repo evaluation.DatasetRepository,
	logger *slog.Logger,
) evaluation.DatasetService {
	return &datasetService{
		repo:   repo,
		logger: logger,
	}
}

func (s *datasetService) Create(ctx context.Context, projectID uuid.UUID, req *evaluation.CreateDatasetRequest) (*evaluation.Dataset, error) {
	dataset := evaluation.NewDataset(projectID, req.Name)
	dataset.Description = req.Description
	if req.Metadata != nil {
		dataset.Metadata = req.Metadata
	}

	if validationErrors := dataset.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	exists, err := s.repo.ExistsByName(ctx, projectID, req.Name)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to check name uniqueness", err)
	}
	if exists {
		return nil, appErrors.NewConflictError(fmt.Sprintf("dataset '%s' already exists in this project", req.Name))
	}

	if err := s.repo.Create(ctx, dataset); err != nil {
		if errors.Is(err, evaluation.ErrDatasetExists) {
			return nil, appErrors.NewConflictError(fmt.Sprintf("dataset '%s' already exists in this project", req.Name))
		}
		return nil, appErrors.NewInternalError("failed to create dataset", err)
	}

	s.logger.Info("dataset created",
		"dataset_id", dataset.ID,
		"project_id", projectID,
		"name", dataset.Name,
	)

	return dataset, nil
}

func (s *datasetService) Update(ctx context.Context, id uuid.UUID, projectID uuid.UUID, req *evaluation.UpdateDatasetRequest) (*evaluation.Dataset, error) {
	dataset, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", id))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	if req.Name != nil && *req.Name != dataset.Name {
		exists, err := s.repo.ExistsByName(ctx, projectID, *req.Name)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check name uniqueness", err)
		}
		if exists {
			return nil, appErrors.NewConflictError(fmt.Sprintf("dataset '%s' already exists in this project", *req.Name))
		}
		dataset.Name = *req.Name
	}

	if req.Description != nil {
		dataset.Description = req.Description
	}
	if req.Metadata != nil {
		dataset.Metadata = req.Metadata
	}

	dataset.UpdatedAt = time.Now()

	if validationErrors := dataset.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	if err := s.repo.Update(ctx, dataset, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", id))
		}
		if errors.Is(err, evaluation.ErrDatasetExists) {
			return nil, appErrors.NewConflictError(fmt.Sprintf("dataset '%s' already exists in this project", dataset.Name))
		}
		return nil, appErrors.NewInternalError("failed to update dataset", err)
	}

	s.logger.Info("dataset updated",
		"dataset_id", id,
		"project_id", projectID,
	)

	return dataset, nil
}

func (s *datasetService) Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error {
	dataset, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", id))
		}
		return appErrors.NewInternalError("failed to get dataset", err)
	}

	if err := s.repo.Delete(ctx, id, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", id))
		}
		return appErrors.NewInternalError("failed to delete dataset", err)
	}

	s.logger.Info("dataset deleted",
		"dataset_id", id,
		"project_id", projectID,
		"name", dataset.Name,
	)

	return nil
}

func (s *datasetService) GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.Dataset, error) {
	dataset, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", id))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}
	return dataset, nil
}

func (s *datasetService) List(ctx context.Context, projectID uuid.UUID, filter *evaluation.DatasetFilter, page, limit int) ([]*evaluation.Dataset, int64, error) {
	offset := (page - 1) * limit
	datasets, total, err := s.repo.List(ctx, projectID, filter, offset, limit)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list datasets", err)
	}
	return datasets, total, nil
}

func (s *datasetService) ListWithFilters(
	ctx context.Context,
	projectID uuid.UUID,
	filter *evaluation.DatasetFilter,
	params pagination.Params,
) ([]*evaluation.DatasetWithItemCount, int64, error) {
	datasets, total, err := s.repo.ListWithFilters(ctx, projectID, filter, params)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list datasets", err)
	}
	return datasets, total, nil
}
