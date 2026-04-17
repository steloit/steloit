package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// Ensure QuotaRepository implements the interface.
var _ billingDomain.QuotaRepository = (*QuotaRepository)(nil)

// QuotaRepository is the pgx+sqlc implementation of
// billingDomain.QuotaRepository. usage_quotas is one row per org;
// UpdateUsageQuota is an upsert keyed on organization_id.
type QuotaRepository struct {
	tm     *db.TxManager
	logger *slog.Logger
}

// NewQuotaRepository returns the pgx-backed repository.
func NewQuotaRepository(tm *db.TxManager, logger *slog.Logger) *QuotaRepository {
	return &QuotaRepository{tm: tm, logger: logger}
}

// GetUsageQuota returns (nil, nil) when no quota is configured for the
// organization — preserving the "optional record" contract that the
// GORM implementation exposed. Actual database errors propagate.
func (r *QuotaRepository) GetUsageQuota(ctx context.Context, orgID uuid.UUID) (*billingDomain.UsageQuota, error) {
	row, err := r.tm.Queries(ctx).GetUsageQuota(ctx, orgID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get usage quota for %s: %w", orgID, err)
	}
	return usageQuotaFromRow(&row), nil
}

// UpdateUsageQuota inserts or updates the quota row for orgID.
func (r *QuotaRepository) UpdateUsageQuota(ctx context.Context, orgID uuid.UUID, q *billingDomain.UsageQuota) error {
	if q.LastUpdated.IsZero() {
		q.LastUpdated = time.Now()
	}
	if err := r.tm.Queries(ctx).UpsertUsageQuota(ctx, gen.UpsertUsageQuotaParams{
		OrganizationID:      orgID,
		BillingTier:         q.BillingTier,
		MonthlyRequestLimit: q.MonthlyRequestLimit,
		MonthlyTokenLimit:   q.MonthlyTokenLimit,
		MonthlyCostLimit:    q.MonthlyCostLimit,
		CurrentRequests:     q.CurrentRequests,
		CurrentTokens:       q.CurrentTokens,
		CurrentCost:         q.CurrentCost,
		Currency:            q.Currency,
		ResetDate:           q.ResetDate,
		LastUpdated:         q.LastUpdated,
	}); err != nil {
		r.logger.Error("failed to upsert usage quota", "error", err, "org_id", orgID)
		return fmt.Errorf("upsert usage quota for %s: %w", orgID, err)
	}
	r.logger.Debug("upserted usage quota",
		"org_id", orgID,
		"billing_tier", q.BillingTier,
		"request_limit", q.MonthlyRequestLimit,
		"cost_limit", q.MonthlyCostLimit,
	)
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func usageQuotaFromRow(row *gen.UsageQuota) *billingDomain.UsageQuota {
	return &billingDomain.UsageQuota{
		OrganizationID:      row.OrganizationID,
		BillingTier:         row.BillingTier,
		MonthlyRequestLimit: row.MonthlyRequestLimit,
		MonthlyTokenLimit:   row.MonthlyTokenLimit,
		MonthlyCostLimit:    row.MonthlyCostLimit,
		CurrentRequests:     row.CurrentRequests,
		CurrentTokens:       row.CurrentTokens,
		CurrentCost:         row.CurrentCost,
		Currency:            row.Currency,
		ResetDate:           row.ResetDate,
		LastUpdated:         row.LastUpdated,
	}
}
