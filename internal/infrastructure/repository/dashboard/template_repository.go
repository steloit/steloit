package dashboard

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/google/uuid"

	dashboardDomain "brokle/internal/core/domain/dashboard"
)

// templateRepository implements dashboardDomain.TemplateRepository using GORM
type templateRepository struct {
	db *gorm.DB
}

// NewTemplateRepository creates a new template repository instance
func NewTemplateRepository(db *gorm.DB) dashboardDomain.TemplateRepository {
	return &templateRepository{
		db: db,
	}
}

// List retrieves all active templates with optional filtering
func (r *templateRepository) List(ctx context.Context, filter *dashboardDomain.TemplateFilter) ([]*dashboardDomain.Template, error) {
	var templates []*dashboardDomain.Template

	query := r.db.WithContext(ctx).Model(&dashboardDomain.Template{})

	// Default to active templates only
	if filter == nil || filter.IsActive == nil {
		query = query.Where("is_active = ?", true)
	} else {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	if filter != nil && filter.Category != nil {
		query = query.Where("category = ?", *filter.Category)
	}

	query = query.Order("name ASC")

	if err := query.Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	return templates, nil
}

// GetByID retrieves a template by its ID
func (r *templateRepository) GetByID(ctx context.Context, id uuid.UUID) (*dashboardDomain.Template, error) {
	var template dashboardDomain.Template
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&template).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get template by ID %s: %w", id, dashboardDomain.ErrTemplateNotFound)
		}
		return nil, fmt.Errorf("get template by ID %s: %w", id, err)
	}
	return &template, nil
}

// GetByName retrieves a template by its name
func (r *templateRepository) GetByName(ctx context.Context, name string) (*dashboardDomain.Template, error) {
	var template dashboardDomain.Template
	err := r.db.WithContext(ctx).
		Where("name = ?", name).
		First(&template).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get template by name %s: %w", name, dashboardDomain.ErrTemplateNotFound)
		}
		return nil, fmt.Errorf("get template by name %s: %w", name, err)
	}
	return &template, nil
}

// GetByCategory retrieves a template by its category
func (r *templateRepository) GetByCategory(ctx context.Context, category dashboardDomain.TemplateCategory) (*dashboardDomain.Template, error) {
	var template dashboardDomain.Template
	err := r.db.WithContext(ctx).
		Where("category = ? AND is_active = ?", category, true).
		First(&template).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get template by category %s: %w", category, dashboardDomain.ErrTemplateNotFound)
		}
		return nil, fmt.Errorf("get template by category %s: %w", category, err)
	}
	return &template, nil
}

// Create creates a new template
func (r *templateRepository) Create(ctx context.Context, template *dashboardDomain.Template) error {
	if err := r.db.WithContext(ctx).Create(template).Error; err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	return nil
}

// Update updates an existing template
func (r *templateRepository) Update(ctx context.Context, template *dashboardDomain.Template) error {
	if err := r.db.WithContext(ctx).Save(template).Error; err != nil {
		return fmt.Errorf("update template: %w", err)
	}
	return nil
}

// Delete removes a template by its ID
func (r *templateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&dashboardDomain.Template{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete template: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return dashboardDomain.ErrTemplateNotFound
	}
	return nil
}

// Upsert creates or updates a template by name (used for seeding)
func (r *templateRepository) Upsert(ctx context.Context, template *dashboardDomain.Template) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			UpdateAll: true,
		}).
		Create(template).Error
	if err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}
	return nil
}
