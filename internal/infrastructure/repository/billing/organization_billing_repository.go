package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// organizationBillingRepository is the pgx+sqlc implementation of
// billingDomain.OrganizationBillingRepository. Owns per-org billing
// state: current-period counters, free-tier remaining, cycle dates.
type organizationBillingRepository struct {
	tm *db.TxManager
}

// NewOrganizationBillingRepository returns the pgx-backed repository.
func NewOrganizationBillingRepository(tm *db.TxManager) billingDomain.OrganizationBillingRepository {
	return &organizationBillingRepository{tm: tm}
}

func (r *organizationBillingRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) (*billingDomain.OrganizationBilling, error) {
	row, err := r.tm.Queries(ctx).GetOrganizationBillingByOrgID(ctx, orgID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, billingDomain.NewBillingNotFoundError(orgID.String())
		}
		return nil, fmt.Errorf("get organization billing for %s: %w", orgID, err)
	}
	return organizationBillingFromRow(&row), nil
}

func (r *organizationBillingRepository) Create(ctx context.Context, b *billingDomain.OrganizationBilling) error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = now
	}
	if b.LastSyncedAt.IsZero() {
		b.LastSyncedAt = now
	}
	if err := r.tm.Queries(ctx).CreateOrganizationBilling(ctx, gen.CreateOrganizationBillingParams{
		OrganizationID:        b.OrganizationID,
		PlanID:                b.PlanID,
		BillingCycleStart:     b.BillingCycleStart,
		BillingCycleAnchorDay: int32(b.BillingCycleAnchorDay),
		CurrentPeriodSpans:    b.CurrentPeriodSpans,
		CurrentPeriodBytes:    b.CurrentPeriodBytes,
		CurrentPeriodScores:   b.CurrentPeriodScores,
		CurrentPeriodCost:     b.CurrentPeriodCost,
		FreeSpansRemaining:    b.FreeSpansRemaining,
		FreeBytesRemaining:    b.FreeBytesRemaining,
		FreeScoresRemaining:   b.FreeScoresRemaining,
		LastSyncedAt:          b.LastSyncedAt,
		CreatedAt:             b.CreatedAt,
		UpdatedAt:             b.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create organization billing for %s: %w", b.OrganizationID, err)
	}
	return nil
}

func (r *organizationBillingRepository) Update(ctx context.Context, b *billingDomain.OrganizationBilling) error {
	b.UpdatedAt = time.Now()
	if err := r.tm.Queries(ctx).UpdateOrganizationBilling(ctx, gen.UpdateOrganizationBillingParams{
		OrganizationID:        b.OrganizationID,
		PlanID:                b.PlanID,
		BillingCycleStart:     b.BillingCycleStart,
		BillingCycleAnchorDay: int32(b.BillingCycleAnchorDay),
		CurrentPeriodSpans:    b.CurrentPeriodSpans,
		CurrentPeriodBytes:    b.CurrentPeriodBytes,
		CurrentPeriodScores:   b.CurrentPeriodScores,
		CurrentPeriodCost:     b.CurrentPeriodCost,
		FreeSpansRemaining:    b.FreeSpansRemaining,
		FreeBytesRemaining:    b.FreeBytesRemaining,
		FreeScoresRemaining:   b.FreeScoresRemaining,
		LastSyncedAt:          b.LastSyncedAt,
	}); err != nil {
		return fmt.Errorf("update organization billing for %s: %w", b.OrganizationID, err)
	}
	return nil
}

// SetUsage replaces cumulative usage counters + free-tier remaining
// idempotently. Safe to call on worker retries — same inputs produce
// the same row.
func (r *organizationBillingRepository) SetUsage(
	ctx context.Context,
	orgID uuid.UUID,
	spans, bytes, scores int64,
	cost decimal.Decimal,
	freeSpansRemaining, freeBytesRemaining, freeScoresRemaining int64,
) error {
	if err := r.tm.Queries(ctx).SetOrganizationBillingUsage(ctx, gen.SetOrganizationBillingUsageParams{
		OrganizationID:      orgID,
		CurrentPeriodSpans:  spans,
		CurrentPeriodBytes:  bytes,
		CurrentPeriodScores: scores,
		CurrentPeriodCost:   cost,
		FreeSpansRemaining:  freeSpansRemaining,
		FreeBytesRemaining:  freeBytesRemaining,
		FreeScoresRemaining: freeScoresRemaining,
	}); err != nil {
		return fmt.Errorf("set usage for org %s: %w", orgID, err)
	}
	return nil
}

func (r *organizationBillingRepository) ResetPeriod(ctx context.Context, orgID uuid.UUID, newCycleStart time.Time) error {
	if err := r.tm.Queries(ctx).ResetOrganizationBillingPeriod(ctx, gen.ResetOrganizationBillingPeriodParams{
		OrganizationID:    orgID,
		BillingCycleStart: newCycleStart,
	}); err != nil {
		return fmt.Errorf("reset billing period for org %s: %w", orgID, err)
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func organizationBillingFromRow(row *gen.OrganizationBilling) *billingDomain.OrganizationBilling {
	return &billingDomain.OrganizationBilling{
		OrganizationID:        row.OrganizationID,
		PlanID:                row.PlanID,
		BillingCycleStart:     row.BillingCycleStart,
		BillingCycleAnchorDay: int(row.BillingCycleAnchorDay),
		CurrentPeriodSpans:    row.CurrentPeriodSpans,
		CurrentPeriodBytes:    row.CurrentPeriodBytes,
		CurrentPeriodScores:   row.CurrentPeriodScores,
		CurrentPeriodCost:     row.CurrentPeriodCost,
		FreeSpansRemaining:    row.FreeSpansRemaining,
		FreeBytesRemaining:    row.FreeBytesRemaining,
		FreeScoresRemaining:   row.FreeScoresRemaining,
		LastSyncedAt:          row.LastSyncedAt,
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
}
