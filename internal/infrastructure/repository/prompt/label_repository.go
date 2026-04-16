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
	"brokle/pkg/uid"
)

// labelRepository implements promptDomain.LabelRepository using GORM
type labelRepository struct {
	db *gorm.DB
}

// NewLabelRepository creates a new label repository instance
func NewLabelRepository(db *gorm.DB) promptDomain.LabelRepository {
	return &labelRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *labelRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new label
func (r *labelRepository) Create(ctx context.Context, label *promptDomain.Label) error {
	return r.getDB(ctx).WithContext(ctx).Create(label).Error
}

// GetByID retrieves a label by ID
func (r *labelRepository) GetByID(ctx context.Context, id uuid.UUID) (*promptDomain.Label, error) {
	var label promptDomain.Label
	err := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id).
		First(&label).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get label by ID %s: %w", id, promptDomain.ErrLabelNotFound)
		}
		return nil, err
	}
	return &label, nil
}

// Update updates a label
func (r *labelRepository) Update(ctx context.Context, label *promptDomain.Label) error {
	label.UpdatedAt = time.Now()
	return r.getDB(ctx).WithContext(ctx).Save(label).Error
}

// Delete deletes a label
func (r *labelRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Where("id = ?", id).Delete(&promptDomain.Label{}).Error
}

// GetByPromptAndName retrieves a label by prompt and name
func (r *labelRepository) GetByPromptAndName(ctx context.Context, promptID uuid.UUID, name string) (*promptDomain.Label, error) {
	var label promptDomain.Label
	err := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ? AND name = ?", promptID, name).
		First(&label).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get label %s: %w", name, promptDomain.ErrLabelNotFound)
		}
		return nil, err
	}
	return &label, nil
}

// ListByPrompt retrieves all labels for a prompt
func (r *labelRepository) ListByPrompt(ctx context.Context, promptID uuid.UUID) ([]*promptDomain.Label, error) {
	var labels []*promptDomain.Label
	err := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ?", promptID).
		Order("name ASC").
		Find(&labels).Error
	return labels, err
}

// ListByPrompts retrieves all labels for multiple prompts in a single query
// This is a batch operation to avoid N+1 query problems
func (r *labelRepository) ListByPrompts(ctx context.Context, promptIDs []uuid.UUID) ([]*promptDomain.Label, error) {
	if len(promptIDs) == 0 {
		return []*promptDomain.Label{}, nil
	}

	var labels []*promptDomain.Label
	err := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id IN ?", promptIDs).
		Order("prompt_id ASC, name ASC").
		Find(&labels).Error

	return labels, err
}

// ListByVersion retrieves all labels pointing to a version
func (r *labelRepository) ListByVersion(ctx context.Context, versionID uuid.UUID) ([]*promptDomain.Label, error) {
	var labels []*promptDomain.Label
	err := r.getDB(ctx).WithContext(ctx).
		Where("version_id = ?", versionID).
		Order("name ASC").
		Find(&labels).Error
	return labels, err
}

// ListByVersions retrieves all labels for multiple versions in a single query
// This is a batch operation to avoid N+1 query problems in ListVersions
func (r *labelRepository) ListByVersions(ctx context.Context, versionIDs []uuid.UUID) ([]*promptDomain.Label, error) {
	if len(versionIDs) == 0 {
		return []*promptDomain.Label{}, nil
	}

	var labels []*promptDomain.Label
	err := r.getDB(ctx).WithContext(ctx).
		Where("version_id IN ?", versionIDs).
		Order("version_id ASC, name ASC").
		Find(&labels).Error

	return labels, err
}

// SetLabel atomically sets a label to point to a version (upsert)
func (r *labelRepository) SetLabel(ctx context.Context, promptID, versionID uuid.UUID, name string, createdBy *uuid.UUID) error {
	now := time.Now()

	var existing promptDomain.Label
	err := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ? AND name = ?", promptID, name).
		First(&existing).Error

	if err == nil {
		existing.VersionID = versionID
		existing.UpdatedAt = now
		return r.getDB(ctx).WithContext(ctx).Save(&existing).Error
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	label := &promptDomain.Label{
		ID:        uid.New(),
		PromptID:  promptID,
		VersionID: versionID,
		Name:      name,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return r.getDB(ctx).WithContext(ctx).Create(label).Error
}

// RemoveLabel removes a label from a prompt
func (r *labelRepository) RemoveLabel(ctx context.Context, promptID uuid.UUID, name string) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ? AND name = ?", promptID, name).
		Delete(&promptDomain.Label{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("remove label %s: %w", name, promptDomain.ErrLabelNotFound)
	}
	return nil
}

// DeleteByPrompt deletes all labels for a prompt
func (r *labelRepository) DeleteByPrompt(ctx context.Context, promptID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ?", promptID).
		Delete(&promptDomain.Label{}).Error
}
