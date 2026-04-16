package prompt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/infrastructure/shared"
)

// promptRepository implements promptDomain.PromptRepository using GORM
type promptRepository struct {
	db *gorm.DB
}

// NewPromptRepository creates a new prompt repository instance
func NewPromptRepository(db *gorm.DB) promptDomain.PromptRepository {
	return &promptRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *promptRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new prompt
func (r *promptRepository) Create(ctx context.Context, prompt *promptDomain.Prompt) error {
	return r.getDB(ctx).WithContext(ctx).Create(prompt).Error
}

// GetByID retrieves a prompt by ID
func (r *promptRepository) GetByID(ctx context.Context, id uuid.UUID) (*promptDomain.Prompt, error) {
	var prompt promptDomain.Prompt
	err := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&prompt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get prompt by ID %s: %w", id, promptDomain.ErrPromptNotFound)
		}
		return nil, err
	}
	return &prompt, nil
}

// GetByName retrieves a prompt by project and name
func (r *promptRepository) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*promptDomain.Prompt, error) {
	var prompt promptDomain.Prompt
	err := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ? AND name = ? AND deleted_at IS NULL", projectID, name).
		First(&prompt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get prompt by name %s: %w", name, promptDomain.ErrPromptNotFound)
		}
		return nil, err
	}
	return &prompt, nil
}

// Update updates a prompt
func (r *promptRepository) Update(ctx context.Context, prompt *promptDomain.Prompt) error {
	prompt.UpdatedAt = time.Now()
	return r.getDB(ctx).WithContext(ctx).Save(prompt).Error
}

// Delete hard deletes a prompt
func (r *promptRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Where("id = ?", id).Delete(&promptDomain.Prompt{}).Error
}

// ListByProject retrieves all prompts in a project with optional filters
func (r *promptRepository) ListByProject(ctx context.Context, projectID uuid.UUID, filters *promptDomain.PromptFilters) ([]*promptDomain.Prompt, int64, error) {
	var prompts []*promptDomain.Prompt
	var total int64

	query := r.getDB(ctx).WithContext(ctx).
		Model(&promptDomain.Prompt{}).
		Where("project_id = ? AND deleted_at IS NULL", projectID)

	if filters != nil {
		if filters.Type != nil {
			query = query.Where("type = ?", *filters.Type)
		}
		if len(filters.Tags) > 0 {
			query = query.Where("tags @> ?", filters.Tags) // PostgreSQL array contains
		}
		if filters.Search != nil && *filters.Search != "" {
			searchPattern := "%" + *filters.Search + "%"
			query = query.Where("name ILIKE ? OR description ILIKE ?", searchPattern, searchPattern)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if filters != nil && filters.Limit > 0 {
		query = query.Limit(filters.Limit)
		if filters.Page > 1 {
			offset := (filters.Page - 1) * filters.Limit
			query = query.Offset(offset)
		}
	}

	query = query.Order("created_at DESC")

	if err := query.Find(&prompts).Error; err != nil {
		return nil, 0, err
	}

	return prompts, total, nil
}

// CountByProject counts prompts in a project
func (r *promptRepository) CountByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&promptDomain.Prompt{}).
		Where("project_id = ? AND deleted_at IS NULL", projectID).
		Count(&count).Error
	return count, err
}

// SoftDelete soft deletes a prompt
func (r *promptRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&promptDomain.Prompt{}).
		Where("id = ?", id).
		Update("deleted_at", time.Now()).Error
}

// Restore restores a soft-deleted prompt
func (r *promptRepository) Restore(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&promptDomain.Prompt{}).
		Where("id = ?", id).
		Update("deleted_at", nil).Error
}
