package billing

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/shared"
)

type planRepository struct {
	db *gorm.DB
}

func NewPlanRepository(db *gorm.DB) billing.PlanRepository {
	return &planRepository{db: db}
}

// getDB extracts transaction from context if available
func (r *planRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *planRepository) GetByID(ctx context.Context, id uuid.UUID) (*billing.Plan, error) {
	var plan billing.Plan
	err := r.getDB(ctx).WithContext(ctx).Where("id = ?", id).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, billing.NewPlanNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get plan: %w", err)
	}
	return &plan, nil
}

func (r *planRepository) GetByName(ctx context.Context, name string) (*billing.Plan, error) {
	var plan billing.Plan
	err := r.getDB(ctx).WithContext(ctx).Where("name = ?", name).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, billing.NewPlanNotFoundError(name)
		}
		return nil, fmt.Errorf("get plan by name: %w", err)
	}
	return &plan, nil
}

func (r *planRepository) GetDefault(ctx context.Context) (*billing.Plan, error) {
	var plan billing.Plan
	err := r.getDB(ctx).WithContext(ctx).Where("is_default = ? AND is_active = ?", true, true).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, billing.NewPlanNotFoundError("default")
		}
		return nil, fmt.Errorf("get default plan: %w", err)
	}
	return &plan, nil
}

func (r *planRepository) GetActive(ctx context.Context) ([]*billing.Plan, error) {
	var plans []*billing.Plan
	err := r.getDB(ctx).WithContext(ctx).Where("is_active = ?", true).Find(&plans).Error
	if err != nil {
		return nil, fmt.Errorf("get active plans: %w", err)
	}
	return plans, nil
}

func (r *planRepository) Create(ctx context.Context, plan *billing.Plan) error {
	return r.getDB(ctx).WithContext(ctx).Create(plan).Error
}

func (r *planRepository) Update(ctx context.Context, plan *billing.Plan) error {
	return r.getDB(ctx).WithContext(ctx).Save(plan).Error
}
