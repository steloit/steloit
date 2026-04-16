package evaluation

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
)

type DatasetVersionRepository struct {
	db *gorm.DB
}

func NewDatasetVersionRepository(db *gorm.DB) *DatasetVersionRepository {
	return &DatasetVersionRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *DatasetVersionRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *DatasetVersionRepository) Create(ctx context.Context, version *evaluation.DatasetVersion) error {
	result := r.getDB(ctx).WithContext(ctx).Create(version)
	if result.Error != nil {
		if isVersionUniqueViolation(result.Error) {
			return evaluation.ErrDatasetVersionExists
		}
		return result.Error
	}
	return nil
}

func (r *DatasetVersionRepository) GetByID(ctx context.Context, id uuid.UUID, datasetID uuid.UUID) (*evaluation.DatasetVersion, error) {
	var version evaluation.DatasetVersion
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND dataset_id = ?", id.String(), datasetID.String()).
		First(&version)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetVersionNotFound
		}
		return nil, result.Error
	}
	return &version, nil
}

func (r *DatasetVersionRepository) GetByVersionNumber(ctx context.Context, datasetID uuid.UUID, versionNum int) (*evaluation.DatasetVersion, error) {
	var version evaluation.DatasetVersion
	result := r.getDB(ctx).WithContext(ctx).
		Where("dataset_id = ? AND version = ?", datasetID.String(), versionNum).
		First(&version)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetVersionNotFound
		}
		return nil, result.Error
	}
	return &version, nil
}

func (r *DatasetVersionRepository) GetLatest(ctx context.Context, datasetID uuid.UUID) (*evaluation.DatasetVersion, error) {
	var version evaluation.DatasetVersion
	result := r.getDB(ctx).WithContext(ctx).
		Where("dataset_id = ?", datasetID.String()).
		Order("version DESC").
		First(&version)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetVersionNotFound
		}
		return nil, result.Error
	}
	return &version, nil
}

func (r *DatasetVersionRepository) List(ctx context.Context, datasetID uuid.UUID) ([]*evaluation.DatasetVersion, error) {
	var versions []*evaluation.DatasetVersion
	result := r.getDB(ctx).WithContext(ctx).
		Where("dataset_id = ?", datasetID.String()).
		Order("version DESC").
		Find(&versions)

	if result.Error != nil {
		return nil, result.Error
	}
	return versions, nil
}

func (r *DatasetVersionRepository) GetNextVersionNumber(ctx context.Context, datasetID uuid.UUID) (int, error) {
	var maxVersion *int
	result := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.DatasetVersion{}).
		Select("MAX(version)").
		Where("dataset_id = ?", datasetID.String()).
		Scan(&maxVersion)

	if result.Error != nil {
		return 0, result.Error
	}

	if maxVersion == nil {
		return 1, nil
	}
	return *maxVersion + 1, nil
}

func (r *DatasetVersionRepository) AddItems(ctx context.Context, versionID uuid.UUID, itemIDs []uuid.UUID) error {
	if len(itemIDs) == 0 {
		return nil
	}

	// Create batch of associations
	associations := make([]evaluation.DatasetItemVersion, len(itemIDs))
	for i, itemID := range itemIDs {
		associations[i] = evaluation.DatasetItemVersion{
			DatasetVersionID: versionID,
			DatasetItemID:    itemID,
		}
	}

	result := r.getDB(ctx).WithContext(ctx).Create(&associations)
	return result.Error
}

func (r *DatasetVersionRepository) GetItemIDs(ctx context.Context, versionID uuid.UUID) ([]uuid.UUID, error) {
	var associations []evaluation.DatasetItemVersion
	result := r.getDB(ctx).WithContext(ctx).
		Where("dataset_version_id = ?", versionID.String()).
		Find(&associations)

	if result.Error != nil {
		return nil, result.Error
	}

	itemIDs := make([]uuid.UUID, len(associations))
	for i, assoc := range associations {
		itemIDs[i] = assoc.DatasetItemID
	}
	return itemIDs, nil
}

func (r *DatasetVersionRepository) GetItems(ctx context.Context, versionID uuid.UUID, limit, offset int) ([]*evaluation.DatasetItem, int64, error) {
	var total int64
	// Count total items in version
	if err := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.DatasetItemVersion{}).
		Where("dataset_version_id = ?", versionID.String()).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get items through join
	var items []*evaluation.DatasetItem
	result := r.getDB(ctx).WithContext(ctx).
		Joins("INNER JOIN dataset_item_versions div ON div.dataset_item_id = dataset_items.id").
		Where("div.dataset_version_id = ?", versionID.String()).
		Order("dataset_items.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items)

	if result.Error != nil {
		return nil, 0, result.Error
	}

	return items, total, nil
}

func isVersionUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "23505") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "duplicate key")
}
