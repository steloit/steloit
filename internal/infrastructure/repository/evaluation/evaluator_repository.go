package evaluation

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/pkg/pagination"

	"gorm.io/gorm"
)

type EvaluatorRepository struct {
	db *gorm.DB
}

func NewEvaluatorRepository(db *gorm.DB) *EvaluatorRepository {
	return &EvaluatorRepository{db: db}
}

func (r *EvaluatorRepository) Create(ctx context.Context, evaluator *evaluation.Evaluator) error {
	result := r.db.WithContext(ctx).Create(evaluator)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return evaluation.ErrEvaluatorExists
		}
		return result.Error
	}
	return nil
}

func (r *EvaluatorRepository) GetByID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (*evaluation.Evaluator, error) {
	var evaluator evaluation.Evaluator
	result := r.db.WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		First(&evaluator)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrEvaluatorNotFound
		}
		return nil, result.Error
	}
	return &evaluator, nil
}

func (r *EvaluatorRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID, filter *evaluation.EvaluatorFilter, params pagination.Params) ([]*evaluation.Evaluator, int64, error) {
	var evaluators []*evaluation.Evaluator
	var total int64

	query := r.db.WithContext(ctx).
		Model(&evaluation.Evaluator{}).
		Where("project_id = ?", projectID.String())

	if filter != nil {
		if filter.Status != nil {
			query = query.Where("status = ?", *filter.Status)
		}
		if filter.ScorerType != nil {
			query = query.Where("scorer_type = ?", *filter.ScorerType)
		}
		if filter.Search != nil && *filter.Search != "" {
			searchTerm := "%" + *filter.Search + "%"
			query = query.Where("name ILIKE ?", searchTerm)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	result := query.
		Order(params.GetSortOrder(params.SortBy, "id")).
		Limit(params.Limit).
		Offset(offset).
		Find(&evaluators)

	if result.Error != nil {
		return nil, 0, result.Error
	}
	return evaluators, total, nil
}

func (r *EvaluatorRepository) GetActiveByProjectID(ctx context.Context, projectID uuid.UUID) ([]*evaluation.Evaluator, error) {
	var evaluators []*evaluation.Evaluator
	result := r.db.WithContext(ctx).
		Where("project_id = ? AND status = ?", projectID.String(), evaluation.EvaluatorStatusActive).
		Order("created_at DESC").
		Find(&evaluators)

	if result.Error != nil {
		return nil, result.Error
	}
	return evaluators, nil
}

func (r *EvaluatorRepository) Update(ctx context.Context, evaluator *evaluation.Evaluator) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND project_id = ?", evaluator.ID.String(), evaluator.ProjectID.String()).
		Save(evaluator)

	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return evaluation.ErrEvaluatorExists
		}
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrEvaluatorNotFound
	}
	return nil
}

func (r *EvaluatorRepository) Delete(ctx context.Context, id uuid.UUID, projectID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND project_id = ?", id.String(), projectID.String()).
		Delete(&evaluation.Evaluator{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return evaluation.ErrEvaluatorNotFound
	}
	return nil
}

func (r *EvaluatorRepository) ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&evaluation.Evaluator{}).
		Where("project_id = ? AND name = ?", projectID.String(), name).
		Count(&count)

	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}
