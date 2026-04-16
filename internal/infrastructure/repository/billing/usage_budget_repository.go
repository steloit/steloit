package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/shared"
)

type usageBudgetRepository struct {
	db *gorm.DB
}

func NewUsageBudgetRepository(db *gorm.DB) billing.UsageBudgetRepository {
	return &usageBudgetRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *usageBudgetRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *usageBudgetRepository) GetByID(ctx context.Context, id uuid.UUID) (*billing.UsageBudget, error) {
	var budget billing.UsageBudget
	err := r.getDB(ctx).WithContext(ctx).Where("id = ?", id).First(&budget).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, billing.NewBudgetNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get budget: %w", err)
	}
	return &budget, nil
}

func (r *usageBudgetRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) ([]*billing.UsageBudget, error) {
	var budgets []*billing.UsageBudget
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&budgets).Error
	if err != nil {
		return nil, fmt.Errorf("get budgets by org: %w", err)
	}
	return budgets, nil
}

func (r *usageBudgetRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID) ([]*billing.UsageBudget, error) {
	var budgets []*billing.UsageBudget
	err := r.getDB(ctx).WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&budgets).Error
	if err != nil {
		return nil, fmt.Errorf("get budgets by project: %w", err)
	}
	return budgets, nil
}

func (r *usageBudgetRepository) GetActive(ctx context.Context, orgID uuid.UUID) ([]*billing.UsageBudget, error) {
	var budgets []*billing.UsageBudget
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND is_active = ?", orgID, true).
		Order("created_at DESC").
		Find(&budgets).Error
	if err != nil {
		return nil, fmt.Errorf("get active budgets: %w", err)
	}
	return budgets, nil
}

func (r *usageBudgetRepository) Create(ctx context.Context, budget *billing.UsageBudget) error {
	return r.getDB(ctx).WithContext(ctx).Create(budget).Error
}

func (r *usageBudgetRepository) Update(ctx context.Context, budget *billing.UsageBudget) error {
	budget.UpdatedAt = time.Now()
	return r.getDB(ctx).WithContext(ctx).Save(budget).Error
}

// UpdateUsage sets usage counters for a budget (expects cumulative totals, not deltas)
func (r *usageBudgetRepository) UpdateUsage(ctx context.Context, budgetID uuid.UUID, spans, bytes, scores int64, cost decimal.Decimal) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&billing.UsageBudget{}).
		Where("id = ?", budgetID).
		Updates(map[string]interface{}{
			"current_spans":  spans,
			"current_bytes":  bytes,
			"current_scores": scores,
			"current_cost":   cost,
			"updated_at":     time.Now(),
		}).Error
}

// Delete soft deletes a budget by setting is_active to false
func (r *usageBudgetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&billing.UsageBudget{}).
		Where("id = ?", id).
		Update("is_active", false).Error
}
