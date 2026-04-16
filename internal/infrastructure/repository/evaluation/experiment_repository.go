package evaluation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/shared"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExperimentRepository struct {
	db *gorm.DB
}

func NewExperimentRepository(db *gorm.DB) *ExperimentRepository {
	return &ExperimentRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *ExperimentRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *ExperimentRepository) Create(ctx context.Context, experiment *evaluation.Experiment) error {
	return r.getDB(ctx).WithContext(ctx).Create(experiment).Error
}

func (r *ExperimentRepository) GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.Experiment, error) {
	var experiment evaluation.Experiment
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		First(&experiment)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrExperimentNotFound
		}
		return nil, result.Error
	}
	return &experiment, nil
}

func (r *ExperimentRepository) List(ctx context.Context, projectID uuid.UUID, filter *evaluation.ExperimentFilter, offset, limit int) ([]*evaluation.Experiment, int64, error) {
	var experiments []*evaluation.Experiment
	var total int64

	query := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ?", projectID.String())

	if filter != nil {
		if filter.DatasetID != nil {
			query = query.Where("dataset_id = ?", filter.DatasetID.String())
		}
		if filter.Status != nil {
			query = query.Where("status = ?", string(*filter.Status))
		}
		if filter.Search != nil && *filter.Search != "" {
			search := "%" + strings.ToLower(*filter.Search) + "%"
			query = query.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", search, search)
		}
		if len(filter.IDs) > 0 {
			idStrings := make([]string, len(filter.IDs))
			for i, id := range filter.IDs {
				idStrings[i] = id.String()
			}
			query = query.Where("id IN ?", idStrings)
		}
	}

	if err := query.Model(&evaluation.Experiment{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&experiments)

	if result.Error != nil {
		return nil, 0, result.Error
	}
	return experiments, total, nil
}

func (r *ExperimentRepository) Update(ctx context.Context, experiment *evaluation.Experiment, projectID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", experiment.ID.String(), projectID.String()).
		Save(experiment)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrExperimentNotFound
	}
	return nil
}

func (r *ExperimentRepository) Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		Delete(&evaluation.Experiment{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrExperimentNotFound
	}
	return nil
}

// SetTotalItems sets the total number of items for an experiment.
func (r *ExperimentRepository) SetTotalItems(ctx context.Context, id, projectID uuid.UUID, total int) error {
	result := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.Experiment{}).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		Update("total_items", total)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrExperimentNotFound
	}
	return nil
}

// IncrementCounters atomically increments completed and/or failed counters.
func (r *ExperimentRepository) IncrementCounters(ctx context.Context, id, projectID uuid.UUID, completed, failed int) error {
	updates := map[string]interface{}{}

	if completed > 0 {
		updates["completed_items"] = gorm.Expr("completed_items + ?", completed)
	}
	if failed > 0 {
		updates["failed_items"] = gorm.Expr("failed_items + ?", failed)
	}

	if len(updates) == 0 {
		return nil
	}

	result := r.getDB(ctx).WithContext(ctx).
		Model(&evaluation.Experiment{}).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrExperimentNotFound
	}
	return nil
}

// IncrementCountersAndUpdateStatus atomically increments counters and updates status if complete.
// Uses row locking to ensure atomicity. Returns true if the experiment was marked as complete.
func (r *ExperimentRepository) IncrementCountersAndUpdateStatus(ctx context.Context, id, projectID uuid.UUID, completed, failed int) (bool, error) {
	var isComplete bool

	err := r.getDB(ctx).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the row for update
		var exp evaluation.Experiment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND project_id = ?", id.String(), projectID.String()).
			First(&exp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return evaluation.ErrExperimentNotFound
			}
			return err
		}

		// Update counters
		exp.CompletedItems += completed
		exp.FailedItems += failed

		// Check if complete
		processedItems := exp.CompletedItems + exp.FailedItems
		if processedItems >= exp.TotalItems && exp.TotalItems > 0 {
			isComplete = true
			now := time.Now()
			exp.CompletedAt = &now

			// Determine final status
			if exp.FailedItems == 0 {
				exp.Status = evaluation.ExperimentStatusCompleted
			} else if exp.CompletedItems == 0 {
				exp.Status = evaluation.ExperimentStatusFailed
			} else {
				exp.Status = evaluation.ExperimentStatusPartial
			}
		}

		return tx.Save(&exp).Error
	})

	return isComplete, err
}

// GetProgress gets minimal experiment data for progress polling.
func (r *ExperimentRepository) GetProgress(ctx context.Context, id, projectID uuid.UUID) (*evaluation.Experiment, error) {
	var exp evaluation.Experiment
	err := r.getDB(ctx).WithContext(ctx).
		Select("id", "status", "total_items", "completed_items", "failed_items", "started_at", "completed_at").
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		First(&exp).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrExperimentNotFound
		}
		return nil, err
	}
	return &exp, nil
}
