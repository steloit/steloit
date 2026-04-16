package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	dashboardDomain "brokle/internal/core/domain/dashboard"
)

type dashboardRepository struct {
	db *gorm.DB
}

func NewDashboardRepository(db *gorm.DB) dashboardDomain.DashboardRepository {
	return &dashboardRepository{
		db: db,
	}
}

func (r *dashboardRepository) Create(ctx context.Context, dashboard *dashboardDomain.Dashboard) error {
	return r.db.WithContext(ctx).Create(dashboard).Error
}

func (r *dashboardRepository) GetByID(ctx context.Context, id uuid.UUID) (*dashboardDomain.Dashboard, error) {
	var dashboard dashboardDomain.Dashboard
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&dashboard).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get dashboard by ID %s: %w", id, dashboardDomain.ErrDashboardNotFound)
		}
		return nil, err
	}
	return &dashboard, nil
}

func (r *dashboardRepository) Update(ctx context.Context, dashboard *dashboardDomain.Dashboard) error {
	dashboard.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(dashboard).Error
}

func (r *dashboardRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&dashboardDomain.Dashboard{}).Error
}

func (r *dashboardRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID, filter *dashboardDomain.DashboardFilter) (*dashboardDomain.DashboardListResponse, error) {
	var dashboards []*dashboardDomain.Dashboard
	var total int64

	query := r.db.WithContext(ctx).
		Model(&dashboardDomain.Dashboard{}).
		Where("project_id = ? AND deleted_at IS NULL", projectID)

	if filter != nil {
		if filter.Name != "" {
			searchPattern := "%" + filter.Name + "%"
			query = query.Where("name ILIKE ?", searchPattern)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	limit := 50
	offset := 0
	if filter != nil {
		if filter.Limit > 0 {
			limit = filter.Limit
		}
		if filter.Offset > 0 {
			offset = filter.Offset
		}
	}

	query = query.Limit(limit).Offset(offset).Order("created_at DESC")

	if err := query.Find(&dashboards).Error; err != nil {
		return nil, err
	}

	return &dashboardDomain.DashboardListResponse{
		Dashboards: dashboards,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (r *dashboardRepository) GetByNameAndProject(ctx context.Context, projectID uuid.UUID, name string) (*dashboardDomain.Dashboard, error) {
	var dashboard dashboardDomain.Dashboard
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND name = ? AND deleted_at IS NULL", projectID, name).
		First(&dashboard).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get dashboard by name %s: %w", name, dashboardDomain.ErrDashboardNotFound)
		}
		return nil, err
	}
	return &dashboard, nil
}

func (r *dashboardRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&dashboardDomain.Dashboard{}).
		Where("id = ?", id).
		Update("deleted_at", time.Now()).Error
}

func (r *dashboardRepository) CountByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&dashboardDomain.Dashboard{}).
		Where("project_id = ? AND deleted_at IS NULL", projectID).
		Count(&count).Error
	return count, err
}
