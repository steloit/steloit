package evaluation

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

type DatasetItemRepository struct {
	db *gorm.DB
}

func NewDatasetItemRepository(db *gorm.DB) *DatasetItemRepository {
	return &DatasetItemRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *DatasetItemRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *DatasetItemRepository) Create(ctx context.Context, item *evaluation.DatasetItem) error {
	return r.getDB(ctx).WithContext(ctx).Create(item).Error
}

func (r *DatasetItemRepository) CreateBatch(ctx context.Context, items []*evaluation.DatasetItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.getDB(ctx).WithContext(ctx).CreateInBatches(items, 100).Error
}

func (r *DatasetItemRepository) GetByID(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) (*evaluation.DatasetItem, error) {
	var item evaluation.DatasetItem
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND dataset_id = ?", id.String(), datasetID.String()).
		First(&item)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetItemNotFound
		}
		return nil, result.Error
	}
	return &item, nil
}

func (r *DatasetItemRepository) GetByIDForProject(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.DatasetItem, error) {
	var item evaluation.DatasetItem
	result := r.getDB(ctx).WithContext(ctx).
		Joins("JOIN datasets ON datasets.id = dataset_items.dataset_id").
		Where("dataset_items.id = ? AND datasets.project_id = ?", id.String(), projectID.String()).
		First(&item)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetItemNotFound
		}
		return nil, result.Error
	}
	return &item, nil
}

func (r *DatasetItemRepository) List(ctx context.Context, datasetID uuid.UUID, limit, offset int) ([]*evaluation.DatasetItem, int64, error) {
	var items []*evaluation.DatasetItem
	var total int64

	baseQuery := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.DatasetItem{}).
		Where("dataset_id = ?", datasetID.String())

	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := r.getDB(ctx).WithContext(ctx).
		Where("dataset_id = ?", datasetID.String()).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items)

	if result.Error != nil {
		return nil, 0, result.Error
	}
	return items, total, nil
}

func (r *DatasetItemRepository) Delete(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND dataset_id = ?", id.String(), datasetID.String()).
		Delete(&evaluation.DatasetItem{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrDatasetItemNotFound
	}
	return nil
}

func (r *DatasetItemRepository) CountByDataset(ctx context.Context, datasetID uuid.UUID) (int64, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.DatasetItem{}).
		Where("dataset_id = ?", datasetID.String()).
		Count(&count)

	if result.Error != nil {
		return 0, result.Error
	}
	return count, nil
}

// FindByContentHash finds a dataset item by its content hash for deduplication.
func (r *DatasetItemRepository) FindByContentHash(ctx context.Context, datasetID uuid.UUID, contentHash string) (*evaluation.DatasetItem, error) {
	var item evaluation.DatasetItem
	result := r.getDB(ctx).WithContext(ctx).
		Where("dataset_id = ? AND content_hash = ?", datasetID.String(), contentHash).
		First(&item)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil // Not found is not an error for deduplication
		}
		return nil, result.Error
	}
	return &item, nil
}

// FindByContentHashes finds dataset items by their content hashes (batch lookup for deduplication).
func (r *DatasetItemRepository) FindByContentHashes(ctx context.Context, datasetID uuid.UUID, contentHashes []string) (map[string]bool, error) {
	if len(contentHashes) == 0 {
		return make(map[string]bool), nil
	}

	var hashes []string
	result := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.DatasetItem{}).
		Where("dataset_id = ? AND content_hash IN ?", datasetID.String(), contentHashes).
		Pluck("content_hash", &hashes)

	if result.Error != nil {
		return nil, result.Error
	}

	// Convert to map for O(1) lookup
	existing := make(map[string]bool, len(hashes))
	for _, h := range hashes {
		existing[h] = true
	}
	return existing, nil
}

// ListAll returns all dataset items for export (no pagination).
func (r *DatasetItemRepository) ListAll(ctx context.Context, datasetID uuid.UUID) ([]*evaluation.DatasetItem, error) {
	var items []*evaluation.DatasetItem
	result := r.getDB(ctx).WithContext(ctx).
		Where("dataset_id = ?", datasetID.String()).
		Order("created_at ASC").
		Find(&items)

	if result.Error != nil {
		return nil, result.Error
	}
	return items, nil
}
