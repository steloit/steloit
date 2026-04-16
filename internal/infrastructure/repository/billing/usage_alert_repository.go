package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/shared"
)

type usageAlertRepository struct {
	db *gorm.DB
}

func NewUsageAlertRepository(db *gorm.DB) billing.UsageAlertRepository {
	return &usageAlertRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *usageAlertRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *usageAlertRepository) GetByID(ctx context.Context, id uuid.UUID) (*billing.UsageAlert, error) {
	var alert billing.UsageAlert
	err := r.getDB(ctx).WithContext(ctx).Where("id = ?", id).First(&alert).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, billing.NewAlertNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get alert: %w", err)
	}
	return &alert, nil
}

func (r *usageAlertRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID, limit int) ([]*billing.UsageAlert, error) {
	var alerts []*billing.UsageAlert
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("triggered_at DESC").
		Limit(limit).
		Find(&alerts).Error
	if err != nil {
		return nil, fmt.Errorf("get alerts by org: %w", err)
	}
	return alerts, nil
}

func (r *usageAlertRepository) GetByBudgetID(ctx context.Context, budgetID uuid.UUID) ([]*billing.UsageAlert, error) {
	var alerts []*billing.UsageAlert
	err := r.getDB(ctx).WithContext(ctx).
		Where("budget_id = ?", budgetID).
		Order("triggered_at DESC").
		Find(&alerts).Error
	if err != nil {
		return nil, fmt.Errorf("get alerts by budget: %w", err)
	}
	return alerts, nil
}

func (r *usageAlertRepository) GetUnacknowledged(ctx context.Context, orgID uuid.UUID) ([]*billing.UsageAlert, error) {
	var alerts []*billing.UsageAlert
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND status = ?", orgID, billing.AlertStatusTriggered).
		Order("triggered_at DESC").
		Find(&alerts).Error
	if err != nil {
		return nil, fmt.Errorf("get unacknowledged alerts: %w", err)
	}
	return alerts, nil
}

func (r *usageAlertRepository) Create(ctx context.Context, alert *billing.UsageAlert) error {
	return r.getDB(ctx).WithContext(ctx).Create(alert).Error
}

func (r *usageAlertRepository) Acknowledge(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.getDB(ctx).WithContext(ctx).
		Model(&billing.UsageAlert{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          billing.AlertStatusAcknowledged,
			"acknowledged_at": now,
		}).Error
}

func (r *usageAlertRepository) Resolve(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.getDB(ctx).WithContext(ctx).
		Model(&billing.UsageAlert{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      billing.AlertStatusResolved,
			"resolved_at": now,
		}).Error
}

func (r *usageAlertRepository) MarkNotificationSent(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&billing.UsageAlert{}).
		Where("id = ?", id).
		Update("notification_sent", true).Error
}
