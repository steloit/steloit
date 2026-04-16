package evaluation

import (
	"context"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

type ExperimentItemRepository struct {
	db *gorm.DB
}

func NewExperimentItemRepository(db *gorm.DB) *ExperimentItemRepository {
	return &ExperimentItemRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *ExperimentItemRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *ExperimentItemRepository) Create(ctx context.Context, item *evaluation.ExperimentItem) error {
	return r.getDB(ctx).WithContext(ctx).Create(item).Error
}

func (r *ExperimentItemRepository) CreateBatch(ctx context.Context, items []*evaluation.ExperimentItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.getDB(ctx).WithContext(ctx).CreateInBatches(items, 100).Error
}

func (r *ExperimentItemRepository) List(ctx context.Context, experimentID uuid.UUID, limit, offset int) ([]*evaluation.ExperimentItem, int64, error) {
	var items []*evaluation.ExperimentItem
	var total int64

	baseQuery := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.ExperimentItem{}).
		Where("experiment_id = ?", experimentID.String())

	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := r.getDB(ctx).WithContext(ctx).
		Where("experiment_id = ?", experimentID.String()).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items)

	if result.Error != nil {
		return nil, 0, result.Error
	}
	return items, total, nil
}

func (r *ExperimentItemRepository) CountByExperiment(ctx context.Context, experimentID uuid.UUID) (int64, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.ExperimentItem{}).
		Where("experiment_id = ?", experimentID.String()).
		Count(&count)

	if result.Error != nil {
		return 0, result.Error
	}
	return count, nil
}
