package evaluation

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

type ExperimentConfigRepository struct {
	db *gorm.DB
}

func NewExperimentConfigRepository(db *gorm.DB) *ExperimentConfigRepository {
	return &ExperimentConfigRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *ExperimentConfigRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *ExperimentConfigRepository) Create(ctx context.Context, config *evaluation.ExperimentConfig) error {
	return r.getDB(ctx).WithContext(ctx).Create(config).Error
}

func (r *ExperimentConfigRepository) GetByID(ctx context.Context, id uuid.UUID) (*evaluation.ExperimentConfig, error) {
	var config evaluation.ExperimentConfig
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id.String()).
		First(&config)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrExperimentConfigNotFound
		}
		return nil, result.Error
	}
	return &config, nil
}

func (r *ExperimentConfigRepository) GetByExperimentID(ctx context.Context, experimentID uuid.UUID) (*evaluation.ExperimentConfig, error) {
	var config evaluation.ExperimentConfig
	result := r.getDB(ctx).WithContext(ctx).
		Where("experiment_id = ?", experimentID.String()).
		First(&config)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrExperimentConfigNotFound
		}
		return nil, result.Error
	}
	return &config, nil
}

func (r *ExperimentConfigRepository) Update(ctx context.Context, config *evaluation.ExperimentConfig) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", config.ID.String()).
		Save(config)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrExperimentConfigNotFound
	}
	return nil
}

func (r *ExperimentConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id.String()).
		Delete(&evaluation.ExperimentConfig{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrExperimentConfigNotFound
	}
	return nil
}
