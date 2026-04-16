package evaluation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
)

type scoreConfigService struct {
	repo      evaluation.ScoreConfigRepository
	scoreRepo observability.ScoreRepository
	logger    *slog.Logger
}

func NewScoreConfigService(
	repo evaluation.ScoreConfigRepository,
	scoreRepo observability.ScoreRepository,
	logger *slog.Logger,
) evaluation.ScoreConfigService {
	return &scoreConfigService{
		repo:      repo,
		scoreRepo: scoreRepo,
		logger:    logger,
	}
}

func (s *scoreConfigService) Create(ctx context.Context, projectID uuid.UUID, req *evaluation.CreateScoreConfigRequest) (*evaluation.ScoreConfig, error) {
	config := evaluation.NewScoreConfig(projectID, req.Name, req.Type)
	config.Description = req.Description
	config.MinValue = req.MinValue
	config.MaxValue = req.MaxValue
	config.Categories = req.Categories
	if req.Metadata != nil {
		config.Metadata = req.Metadata
	}

	if validationErrors := config.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	exists, err := s.repo.ExistsByName(ctx, projectID, req.Name)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to check name uniqueness", err)
	}
	if exists {
		return nil, appErrors.NewConflictError(fmt.Sprintf("score config '%s' already exists in this project", req.Name))
	}

	if err := s.repo.Create(ctx, config); err != nil {
		if errors.Is(err, evaluation.ErrScoreConfigExists) {
			return nil, appErrors.NewConflictError(fmt.Sprintf("score config '%s' already exists in this project", req.Name))
		}
		return nil, appErrors.NewInternalError("failed to create score config", err)
	}

	s.logger.Info("score config created",
		"score_config_id", config.ID,
		"project_id", projectID,
		"name", config.Name,
		"type", config.Type,
	)

	return config, nil
}

func (s *scoreConfigService) Update(ctx context.Context, id uuid.UUID, projectID uuid.UUID, req *evaluation.UpdateScoreConfigRequest) (*evaluation.ScoreConfig, error) {
	config, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrScoreConfigNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("score config %s", id))
		}
		return nil, appErrors.NewInternalError("failed to get score config", err)
	}

	// Capture original name before any mutations for score existence check
	originalName := config.Name

	if req.Name != nil && *req.Name != config.Name {
		exists, err := s.repo.ExistsByName(ctx, projectID, *req.Name)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check name uniqueness", err)
		}
		if exists {
			return nil, appErrors.NewConflictError(fmt.Sprintf("score config '%s' already exists in this project", *req.Name))
		}
		config.Name = *req.Name
	}

	if req.Type != nil && *req.Type != config.Type {
		// Check if scores exist for this config (use original name, not potentially renamed one)
		exists, err := s.scoreRepo.ExistsByConfigName(ctx, projectID.String(), originalName)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check score existence", err)
		}
		if exists {
			return nil, appErrors.NewConflictError("cannot change type: scores already exist for this config")
		}
		config.Type = *req.Type
	}

	if req.Description != nil {
		config.Description = req.Description
	}
	if req.MinValue != nil {
		config.MinValue = req.MinValue
	}
	if req.MaxValue != nil {
		config.MaxValue = req.MaxValue
	}
	if req.Categories != nil {
		config.Categories = req.Categories
	}
	if req.Metadata != nil {
		config.Metadata = req.Metadata
	}

	config.UpdatedAt = time.Now()

	if validationErrors := config.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	if err := s.repo.Update(ctx, config, projectID); err != nil {
		if errors.Is(err, evaluation.ErrScoreConfigNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("score config %s", id))
		}
		if errors.Is(err, evaluation.ErrScoreConfigExists) {
			return nil, appErrors.NewConflictError(fmt.Sprintf("score config '%s' already exists in this project", config.Name))
		}
		return nil, appErrors.NewInternalError("failed to update score config", err)
	}

	s.logger.Info("score config updated",
		"score_config_id", id,
		"project_id", projectID,
	)

	return config, nil
}

func (s *scoreConfigService) Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error {
	config, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrScoreConfigNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("score config %s", id))
		}
		return appErrors.NewInternalError("failed to get score config", err)
	}

	if err := s.repo.Delete(ctx, id, projectID); err != nil {
		if errors.Is(err, evaluation.ErrScoreConfigNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("score config %s", id))
		}
		return appErrors.NewInternalError("failed to delete score config", err)
	}

	s.logger.Info("score config deleted",
		"score_config_id", id,
		"project_id", projectID,
		"name", config.Name,
	)

	return nil
}

func (s *scoreConfigService) GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.ScoreConfig, error) {
	config, err := s.repo.GetByID(ctx, id, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrScoreConfigNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("score config %s", id))
		}
		return nil, appErrors.NewInternalError("failed to get score config", err)
	}
	return config, nil
}

func (s *scoreConfigService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*evaluation.ScoreConfig, error) {
	config, err := s.repo.GetByName(ctx, projectID, name)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get score config", err)
	}
	if config == nil {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("score config '%s'", name))
	}
	return config, nil
}

func (s *scoreConfigService) List(ctx context.Context, projectID uuid.UUID, page, limit int) ([]*evaluation.ScoreConfig, int64, error) {
	offset := (page - 1) * limit
	configs, total, err := s.repo.List(ctx, projectID, offset, limit)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list score configs", err)
	}
	return configs, total, nil
}
