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

type organizationBillingRepository struct {
	db *gorm.DB
}

func NewOrganizationBillingRepository(db *gorm.DB) billing.OrganizationBillingRepository {
	return &organizationBillingRepository{db: db}
}

// getDB extracts transaction from context if available
func (r *organizationBillingRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *organizationBillingRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) (*billing.OrganizationBilling, error) {
	var orgBilling billing.OrganizationBilling
	err := r.getDB(ctx).WithContext(ctx).Where("organization_id = ?", orgID).First(&orgBilling).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, billing.NewBillingNotFoundError(orgID.String())
		}
		return nil, fmt.Errorf("get organization billing: %w", err)
	}
	return &orgBilling, nil
}

func (r *organizationBillingRepository) Create(ctx context.Context, orgBilling *billing.OrganizationBilling) error {
	return r.getDB(ctx).WithContext(ctx).Create(orgBilling).Error
}

func (r *organizationBillingRepository) Update(ctx context.Context, orgBilling *billing.OrganizationBilling) error {
	orgBilling.UpdatedAt = time.Now()
	return r.getDB(ctx).WithContext(ctx).Save(orgBilling).Error
}

// SetUsage sets cumulative usage counters and free tier remaining (idempotent - can be called multiple times safely)
// This replaces values rather than adding, preventing race condition double-counting
func (r *organizationBillingRepository) SetUsage(ctx context.Context, orgID uuid.UUID, spans, bytes, scores int64, cost decimal.Decimal, freeSpansRemaining, freeBytesRemaining, freeScoresRemaining int64) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&billing.OrganizationBilling{}).
		Where("organization_id = ?", orgID).
		Updates(map[string]interface{}{
			"current_period_spans":  spans,
			"current_period_bytes":  bytes,
			"current_period_scores": scores,
			"current_period_cost":   cost,
			"free_spans_remaining":  freeSpansRemaining,
			"free_bytes_remaining":  freeBytesRemaining,
			"free_scores_remaining": freeScoresRemaining,
			"last_synced_at":        time.Now(),
			"updated_at":            time.Now(),
		}).Error
}

func (r *organizationBillingRepository) ResetPeriod(ctx context.Context, orgID uuid.UUID, newCycleStart time.Time) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&billing.OrganizationBilling{}).
		Where("organization_id = ?", orgID).
		Updates(map[string]interface{}{
			"billing_cycle_start":   newCycleStart,
			"current_period_spans":  0,
			"current_period_bytes":  0,
			"current_period_scores": 0,
			"current_period_cost":   0,
			"free_spans_remaining":  gorm.Expr("(SELECT free_spans FROM plans WHERE id = organization_billing.plan_id)"),
			"free_bytes_remaining":  gorm.Expr("(SELECT CAST(free_gb * 1073741824 AS BIGINT) FROM plans WHERE id = organization_billing.plan_id)"),
			"free_scores_remaining": gorm.Expr("(SELECT free_scores FROM plans WHERE id = organization_billing.plan_id)"),
			"last_synced_at":        time.Now(),
			"updated_at":            time.Now(),
		}).Error
}
