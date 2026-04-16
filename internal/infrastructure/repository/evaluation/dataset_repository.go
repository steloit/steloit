package evaluation

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/shared"
	"brokle/pkg/pagination"

	"gorm.io/gorm"
)

type DatasetRepository struct {
	db *gorm.DB
}

func NewDatasetRepository(db *gorm.DB) *DatasetRepository {
	return &DatasetRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *DatasetRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *DatasetRepository) Create(ctx context.Context, dataset *evaluation.Dataset) error {
	result := r.getDB(ctx).WithContext(ctx).Create(dataset)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return evaluation.ErrDatasetExists
		}
		return result.Error
	}
	return nil
}

func (r *DatasetRepository) GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.Dataset, error) {
	var dataset evaluation.Dataset
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		First(&dataset)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetNotFound
		}
		return nil, result.Error
	}
	return &dataset, nil
}

func (r *DatasetRepository) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*evaluation.Dataset, error) {
	var dataset evaluation.Dataset
	result := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ? AND name = ?", projectID.String(), name).
		First(&dataset)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &dataset, nil
}

func (r *DatasetRepository) List(ctx context.Context, projectID uuid.UUID, filter *evaluation.DatasetFilter, offset, limit int) ([]*evaluation.Dataset, int64, error) {
	var datasets []*evaluation.Dataset
	var total int64

	query := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ?", projectID.String())

	// Apply search filter
	if filter != nil && filter.Search != nil && *filter.Search != "" {
		search := "%" + strings.ToLower(*filter.Search) + "%"
		query = query.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", search, search)
	}

	if err := query.Model(&evaluation.Dataset{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&datasets)

	if result.Error != nil {
		return nil, 0, result.Error
	}
	return datasets, total, nil
}

func (r *DatasetRepository) Update(ctx context.Context, dataset *evaluation.Dataset, projectID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", dataset.ID.String(), projectID.String()).
		Save(dataset)

	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return evaluation.ErrDatasetExists
		}
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrDatasetNotFound
	}
	return nil
}

func (r *DatasetRepository) Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		Delete(&evaluation.Dataset{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrDatasetNotFound
	}
	return nil
}

func (r *DatasetRepository) ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error) {
	var count int64
	result := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.Dataset{}).
		Where("project_id = ? AND name = ?", projectID.String(), name).
		Count(&count)

	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// ListWithFilters returns datasets with filtering, sorting, and pagination, including item counts.
// Allowed sort fields: name, created_at, updated_at, item_count
func (r *DatasetRepository) ListWithFilters(
	ctx context.Context,
	projectID uuid.UUID,
	filter *evaluation.DatasetFilter,
	params pagination.Params,
) ([]*evaluation.DatasetWithItemCount, int64, error) {
	// Allowed sort fields for SQL injection prevention
	allowedSortFields := []string{"name", "created_at", "updated_at", "item_count"}

	// Validate and set defaults for pagination params
	params.SetDefaults("updated_at")
	if _, err := pagination.ValidateSortField(params.SortBy, allowedSortFields); err != nil {
		params.SortBy = "updated_at"
	}

	// Build base query for counting (without sorting and pagination)
	baseQuery := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.Dataset{}).
		Where("project_id = ?", projectID.String())

	// Apply search filter if provided
	if filter != nil && filter.Search != nil && *filter.Search != "" {
		searchPattern := "%" + strings.ToLower(*filter.Search) + "%"
		baseQuery = baseQuery.Where("LOWER(name) LIKE ?", searchPattern)
	}

	// Count total matching records
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []*evaluation.DatasetWithItemCount{}, 0, nil
	}

	// Build the query with item counts using a subquery
	// This uses LEFT JOIN with a subquery to count items for each dataset
	selectQuery := r.getDB(ctx).WithContext(ctx).
		Table("datasets d").
		Select("d.*, COALESCE(item_counts.count, 0) as item_count").
		Joins("LEFT JOIN (SELECT dataset_id, COUNT(*) as count FROM dataset_items GROUP BY dataset_id) item_counts ON item_counts.dataset_id = d.id").
		Where("d.project_id = ?", projectID.String())

	// Apply search filter
	if filter != nil && filter.Search != nil && *filter.Search != "" {
		searchPattern := "%" + strings.ToLower(*filter.Search) + "%"
		selectQuery = selectQuery.Where("LOWER(d.name) LIKE ?", searchPattern)
	}

	// Apply sorting with defensive validation of sort direction
	sortDir := strings.ToUpper(params.SortDir)
	if sortDir != "ASC" && sortDir != "DESC" {
		sortDir = "DESC"
	}

	sortOrder := params.GetSortOrder(params.SortBy, "d.id")
	// Prefix non-item_count columns with "d."
	if params.SortBy != "item_count" {
		sortOrder = "d." + params.SortBy + " " + sortDir + ", d.id " + sortDir
	}
	selectQuery = selectQuery.Order(sortOrder)

	// Apply pagination
	selectQuery = selectQuery.Offset(params.GetOffset()).Limit(params.Limit)

	// Execute query
	type datasetWithCount struct {
		evaluation.Dataset
		ItemCount int64 `gorm:"column:item_count"`
	}
	var results []datasetWithCount
	if err := selectQuery.Find(&results).Error; err != nil {
		return nil, 0, err
	}

	// Convert to domain type
	datasets := make([]*evaluation.DatasetWithItemCount, len(results))
	for i, r := range results {
		datasets[i] = &evaluation.DatasetWithItemCount{
			Dataset:   r.Dataset,
			ItemCount: r.ItemCount,
		}
	}

	return datasets, total, nil
}

func isDatasetUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "23505") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "duplicate key")
}
