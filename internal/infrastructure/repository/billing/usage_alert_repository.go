package billing

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// usageAlertRepository is the pgx+sqlc implementation of
// billingDomain.UsageAlertRepository. Alerts are append-only records
// emitted when a usage_budget crosses a threshold.
type usageAlertRepository struct {
	tm *db.TxManager
}

// NewUsageAlertRepository returns the pgx-backed repository.
func NewUsageAlertRepository(tm *db.TxManager) billingDomain.UsageAlertRepository {
	return &usageAlertRepository{tm: tm}
}

func (r *usageAlertRepository) GetByID(ctx context.Context, id uuid.UUID) (*billingDomain.UsageAlert, error) {
	row, err := r.tm.Queries(ctx).GetUsageAlertByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, billingDomain.NewAlertNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get alert %s: %w", id, err)
	}
	return usageAlertFromRow(&row), nil
}

func (r *usageAlertRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID, limit int) ([]*billingDomain.UsageAlert, error) {
	rows, err := r.tm.Queries(ctx).ListUsageAlertsByOrg(ctx, gen.ListUsageAlertsByOrgParams{
		OrganizationID: orgID,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list alerts for org %s: %w", orgID, err)
	}
	return usageAlertsFromRows(rows), nil
}

func (r *usageAlertRepository) GetByBudgetID(ctx context.Context, budgetID uuid.UUID) ([]*billingDomain.UsageAlert, error) {
	rows, err := r.tm.Queries(ctx).ListUsageAlertsByBudget(ctx, &budgetID)
	if err != nil {
		return nil, fmt.Errorf("list alerts for budget %s: %w", budgetID, err)
	}
	return usageAlertsFromRows(rows), nil
}

func (r *usageAlertRepository) GetUnacknowledged(ctx context.Context, orgID uuid.UUID) ([]*billingDomain.UsageAlert, error) {
	rows, err := r.tm.Queries(ctx).ListUnacknowledgedUsageAlertsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list unacknowledged alerts for org %s: %w", orgID, err)
	}
	return usageAlertsFromRows(rows), nil
}

func (r *usageAlertRepository) Create(ctx context.Context, a *billingDomain.UsageAlert) error {
	if err := r.tm.Queries(ctx).CreateUsageAlert(ctx, gen.CreateUsageAlertParams{
		ID:               a.ID,
		BudgetID:         a.BudgetID,
		OrganizationID:   a.OrganizationID,
		ProjectID:        a.ProjectID,
		AlertThreshold:   int32(a.AlertThreshold),
		Dimension:        string(a.Dimension),
		Severity:         string(a.Severity),
		ThresholdValue:   a.ThresholdValue,
		ActualValue:      a.ActualValue,
		PercentUsed:      a.PercentUsed,
		Status:           string(a.Status),
		TriggeredAt:      a.TriggeredAt,
		AcknowledgedAt:   a.AcknowledgedAt,
		ResolvedAt:       a.ResolvedAt,
		NotificationSent: a.NotificationSent,
	}); err != nil {
		return fmt.Errorf("create usage alert: %w", err)
	}
	return nil
}

func (r *usageAlertRepository) Acknowledge(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).AcknowledgeUsageAlert(ctx, id); err != nil {
		return fmt.Errorf("acknowledge alert %s: %w", id, err)
	}
	return nil
}

func (r *usageAlertRepository) Resolve(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).ResolveUsageAlert(ctx, id); err != nil {
		return fmt.Errorf("resolve alert %s: %w", id, err)
	}
	return nil
}

func (r *usageAlertRepository) MarkNotificationSent(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).MarkUsageAlertNotified(ctx, id); err != nil {
		return fmt.Errorf("mark alert %s notified: %w", id, err)
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func usageAlertFromRow(row *gen.UsageAlert) *billingDomain.UsageAlert {
	return &billingDomain.UsageAlert{
		ID:               row.ID,
		BudgetID:         row.BudgetID,
		OrganizationID:   row.OrganizationID,
		ProjectID:        row.ProjectID,
		AlertThreshold:   int64(row.AlertThreshold),
		Dimension:        billingDomain.AlertDimension(row.Dimension),
		Severity:         billingDomain.AlertSeverity(row.Severity),
		ThresholdValue:   row.ThresholdValue,
		ActualValue:      row.ActualValue,
		PercentUsed:      row.PercentUsed,
		Status:           billingDomain.AlertStatus(row.Status),
		TriggeredAt:      row.TriggeredAt,
		AcknowledgedAt:   row.AcknowledgedAt,
		ResolvedAt:       row.ResolvedAt,
		NotificationSent: row.NotificationSent,
	}
}

func usageAlertsFromRows(rows []gen.UsageAlert) []*billingDomain.UsageAlert {
	out := make([]*billingDomain.UsageAlert, 0, len(rows))
	for i := range rows {
		out = append(out, usageAlertFromRow(&rows[i]))
	}
	return out
}
