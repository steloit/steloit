package observability

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"brokle/internal/core/domain/observability"
)

type filterPresetRepository struct {
	db *gorm.DB
}

// NewFilterPresetRepository creates a new PostgreSQL filter preset repository.
func NewFilterPresetRepository(db *gorm.DB) observability.FilterPresetRepository {
	return &filterPresetRepository{db: db}
}

func (r *filterPresetRepository) Create(ctx context.Context, preset *observability.FilterPreset) error {
	if err := r.db.WithContext(ctx).Create(preset).Error; err != nil {
		return fmt.Errorf("create filter preset: %w", err)
	}
	return nil
}

func (r *filterPresetRepository) GetByID(ctx context.Context, id uuid.UUID) (*observability.FilterPreset, error) {
	var preset observability.FilterPreset
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&preset).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get filter preset by id: %w", err)
	}
	return &preset, nil
}

func (r *filterPresetRepository) Update(ctx context.Context, preset *observability.FilterPreset) error {
	result := r.db.WithContext(ctx).Model(preset).Updates(map[string]interface{}{
		"name":              preset.Name,
		"description":       preset.Description,
		"filters":           preset.Filters,
		"column_order":      preset.ColumnOrder,
		"column_visibility": preset.ColumnVisibility,
		"search_query":      preset.SearchQuery,
		"search_types":      preset.SearchTypes,
		"is_public":         preset.IsPublic,
	})
	if result.Error != nil {
		return fmt.Errorf("update filter preset: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *filterPresetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&observability.FilterPreset{})
	if result.Error != nil {
		return fmt.Errorf("delete filter preset: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *filterPresetRepository) List(ctx context.Context, filter *observability.FilterPresetFilter) ([]*observability.FilterPreset, error) {
	query := r.buildListQuery(ctx, filter).Order("updated_at DESC")

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var presets []*observability.FilterPreset
	if err := query.Find(&presets).Error; err != nil {
		return nil, fmt.Errorf("list filter presets: %w", err)
	}

	return presets, nil
}

func (r *filterPresetRepository) Count(ctx context.Context, filter *observability.FilterPresetFilter) (int64, error) {
	var count int64
	if err := r.buildListQuery(ctx, filter).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count filter presets: %w", err)
	}

	return count, nil
}

// buildListQuery constructs the shared WHERE clause used by List and Count.
func (r *filterPresetRepository) buildListQuery(ctx context.Context, filter *observability.FilterPresetFilter) *gorm.DB {
	query := r.db.WithContext(ctx).Model(&observability.FilterPreset{}).Where("project_id = ?", filter.ProjectID)

	if filter.TargetTable != nil {
		query = query.Where("table_name = ?", *filter.TargetTable)
	}

	// Handle visibility: user can see their own + optionally public presets
	if filter.UserID != nil {
		if filter.IncludeAll {
			query = query.Where("(created_by = ? OR is_public = true)", *filter.UserID)
		} else {
			query = query.Where("created_by = ?", *filter.UserID)
		}
	} else if filter.CreatedBy != nil {
		query = query.Where("created_by = ?", *filter.CreatedBy)
	} else if filter.IsPublic != nil {
		query = query.Where("is_public = ?", *filter.IsPublic)
	}

	return query
}

func (r *filterPresetRepository) ExistsByName(ctx context.Context, projectID uuid.UUID, name string, excludeID *uuid.UUID) (bool, error) {
	query := r.db.WithContext(ctx).Model(&observability.FilterPreset{}).
		Where("project_id = ? AND name = ?", projectID, name)

	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check filter preset exists: %w", err)
	}

	return count > 0, nil
}
