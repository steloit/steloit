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

// usageBudgetRepository is the pgx+sqlc implementation of
// billingDomain.UsageBudgetRepository. Budgets cap usage on one or
// more dimensions; nil limits mean "no cap on that dimension".
type usageBudgetRepository struct {
	tm *db.TxManager
}

// NewUsageBudgetRepository returns the pgx-backed repository.
func NewUsageBudgetRepository(tm *db.TxManager) billingDomain.UsageBudgetRepository {
	return &usageBudgetRepository{tm: tm}
}

func (r *usageBudgetRepository) GetByID(ctx context.Context, id uuid.UUID) (*billingDomain.UsageBudget, error) {
	row, err := r.tm.Queries(ctx).GetUsageBudgetByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, billingDomain.NewBudgetNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get budget %s: %w", id, err)
	}
	return usageBudgetFromRow(&row), nil
}

func (r *usageBudgetRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) ([]*billingDomain.UsageBudget, error) {
	rows, err := r.tm.Queries(ctx).ListUsageBudgetsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list budgets for org %s: %w", orgID, err)
	}
	return usageBudgetsFromRows(rows), nil
}

func (r *usageBudgetRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID) ([]*billingDomain.UsageBudget, error) {
	rows, err := r.tm.Queries(ctx).ListUsageBudgetsByProject(ctx, &projectID)
	if err != nil {
		return nil, fmt.Errorf("list budgets for project %s: %w", projectID, err)
	}
	return usageBudgetsFromRows(rows), nil
}

func (r *usageBudgetRepository) GetActive(ctx context.Context, orgID uuid.UUID) ([]*billingDomain.UsageBudget, error) {
	rows, err := r.tm.Queries(ctx).ListActiveUsageBudgetsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list active budgets for org %s: %w", orgID, err)
	}
	return usageBudgetsFromRows(rows), nil
}

func (r *usageBudgetRepository) Create(ctx context.Context, b *billingDomain.UsageBudget) error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = now
	}
	if err := r.tm.Queries(ctx).CreateUsageBudget(ctx, gen.CreateUsageBudgetParams{
		ID:              b.ID,
		OrganizationID:  b.OrganizationID,
		ProjectID:       b.ProjectID,
		Name:            b.Name,
		BudgetType:      string(b.BudgetType),
		SpanLimit:       b.SpanLimit,
		BytesLimit:      b.BytesLimit,
		ScoreLimit:      b.ScoreLimit,
		CostLimit:       b.CostLimit,
		CurrentSpans:    b.CurrentSpans,
		CurrentBytes:    b.CurrentBytes,
		CurrentScores:   b.CurrentScores,
		CurrentCost:     b.CurrentCost,
		AlertThresholds: thresholdsToInt32(b.AlertThresholds),
		IsActive:        b.IsActive,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create budget: %w", err)
	}
	return nil
}

func (r *usageBudgetRepository) Update(ctx context.Context, b *billingDomain.UsageBudget) error {
	b.UpdatedAt = time.Now()
	if err := r.tm.Queries(ctx).UpdateUsageBudget(ctx, gen.UpdateUsageBudgetParams{
		ID:              b.ID,
		OrganizationID:  b.OrganizationID,
		ProjectID:       b.ProjectID,
		Name:            b.Name,
		BudgetType:      string(b.BudgetType),
		SpanLimit:       b.SpanLimit,
		BytesLimit:      b.BytesLimit,
		ScoreLimit:      b.ScoreLimit,
		CostLimit:       b.CostLimit,
		CurrentSpans:    b.CurrentSpans,
		CurrentBytes:    b.CurrentBytes,
		CurrentScores:   b.CurrentScores,
		CurrentCost:     b.CurrentCost,
		AlertThresholds: thresholdsToInt32(b.AlertThresholds),
		IsActive:        b.IsActive,
	}); err != nil {
		return fmt.Errorf("update budget %s: %w", b.ID, err)
	}
	return nil
}

// UpdateUsage sets cumulative counters for a budget (idempotent — the
// caller passes absolute totals, not deltas).
func (r *usageBudgetRepository) UpdateUsage(ctx context.Context, budgetID uuid.UUID, spans, bytes, scores int64, cost decimal.Decimal) error {
	if err := r.tm.Queries(ctx).SetUsageBudgetUsage(ctx, gen.SetUsageBudgetUsageParams{
		ID:            budgetID,
		CurrentSpans:  spans,
		CurrentBytes:  bytes,
		CurrentScores: scores,
		CurrentCost:   cost,
	}); err != nil {
		return fmt.Errorf("update usage for budget %s: %w", budgetID, err)
	}
	return nil
}

// Delete performs a logical delete (is_active = FALSE); history and
// alert references survive.
func (r *usageBudgetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeactivateUsageBudget(ctx, id); err != nil {
		return fmt.Errorf("deactivate budget %s: %w", id, err)
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func usageBudgetFromRow(row *gen.UsageBudget) *billingDomain.UsageBudget {
	return &billingDomain.UsageBudget{
		ID:              row.ID,
		OrganizationID:  row.OrganizationID,
		ProjectID:       row.ProjectID,
		Name:            row.Name,
		BudgetType:      billingDomain.BudgetType(row.BudgetType),
		SpanLimit:       row.SpanLimit,
		BytesLimit:      row.BytesLimit,
		ScoreLimit:      row.ScoreLimit,
		CostLimit:       row.CostLimit,
		CurrentSpans:    row.CurrentSpans,
		CurrentBytes:    row.CurrentBytes,
		CurrentScores:   row.CurrentScores,
		CurrentCost:     row.CurrentCost,
		AlertThresholds: thresholdsToInt64(row.AlertThresholds),
		IsActive:        row.IsActive,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func usageBudgetsFromRows(rows []gen.UsageBudget) []*billingDomain.UsageBudget {
	out := make([]*billingDomain.UsageBudget, 0, len(rows))
	for i := range rows {
		out = append(out, usageBudgetFromRow(&rows[i]))
	}
	return out
}

// alert_thresholds is INTEGER[] in Postgres (0-100 percentage points), so
// sqlc emits []int32 while the domain uses []int64 — widen on read, narrow
// on write.

func thresholdsToInt64(in []int32) []int64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]int64, len(in))
	for i, v := range in {
		out[i] = int64(v)
	}
	return out
}

func thresholdsToInt32(in []int64) []int32 {
	if len(in) == 0 {
		return nil
	}
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}
