package prompt

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/infrastructure/shared"
)

// protectedLabelRepository implements promptDomain.ProtectedLabelRepository using GORM
type protectedLabelRepository struct {
	db *gorm.DB
}

// NewProtectedLabelRepository creates a new protected label repository instance
func NewProtectedLabelRepository(db *gorm.DB) promptDomain.ProtectedLabelRepository {
	return &protectedLabelRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *protectedLabelRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new protected label
func (r *protectedLabelRepository) Create(ctx context.Context, label *promptDomain.ProtectedLabel) error {
	return r.getDB(ctx).WithContext(ctx).Create(label).Error
}

// Delete deletes a protected label
func (r *protectedLabelRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Where("id = ?", id).Delete(&promptDomain.ProtectedLabel{}).Error
}

// GetByProjectAndLabel retrieves a protected label by project and label name
func (r *protectedLabelRepository) GetByProjectAndLabel(ctx context.Context, projectID uuid.UUID, labelName string) (*promptDomain.ProtectedLabel, error) {
	var label promptDomain.ProtectedLabel
	err := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ? AND label_name = ?", projectID, labelName).
		First(&label).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get protected label %s: %w", labelName, promptDomain.ErrLabelNotFound)
		}
		return nil, err
	}
	return &label, nil
}

// ListByProject retrieves all protected labels for a project
func (r *protectedLabelRepository) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*promptDomain.ProtectedLabel, error) {
	var labels []*promptDomain.ProtectedLabel
	err := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("label_name ASC").
		Find(&labels).Error
	return labels, err
}

// IsProtected checks if a label is protected in a project
func (r *protectedLabelRepository) IsProtected(ctx context.Context, projectID uuid.UUID, labelName string) (bool, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&promptDomain.ProtectedLabel{}).
		Where("project_id = ? AND label_name = ?", projectID, labelName).
		Count(&count).Error
	return count > 0, err
}

// SetProtectedLabels replaces all protected labels for a project
func (r *protectedLabelRepository) SetProtectedLabels(ctx context.Context, projectID uuid.UUID, labels []string, createdBy *uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", projectID).Delete(&promptDomain.ProtectedLabel{}).Error; err != nil {
			return err
		}

		for _, labelName := range labels {
			label := promptDomain.NewProtectedLabel(projectID, labelName, createdBy)
			if err := tx.Create(label).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// DeleteByProject deletes all protected labels for a project
func (r *protectedLabelRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Where("project_id = ?", projectID).
		Delete(&promptDomain.ProtectedLabel{}).Error
}
