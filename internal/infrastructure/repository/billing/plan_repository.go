package billing

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// planRepository is the pgx+sqlc implementation of
// billingDomain.PlanRepository. Plans is a tiny lookup table — handful
// of rows, no filtering surface — so every method is sqlc-generated.
type planRepository struct {
	tm *db.TxManager
}

// NewPlanRepository returns the pgx-backed repository.
func NewPlanRepository(tm *db.TxManager) billingDomain.PlanRepository {
	return &planRepository{tm: tm}
}

func (r *planRepository) Create(ctx context.Context, p *billingDomain.Plan) error {
	if err := r.tm.Queries(ctx).CreatePlan(ctx, gen.CreatePlanParams{
		ID:                p.ID,
		Name:              p.Name,
		FreeSpans:         p.FreeSpans,
		PricePer100kSpans: p.PricePer100KSpans,
		FreeGb:            p.FreeGB,
		PricePerGb:        p.PricePerGB,
		FreeScores:        p.FreeScores,
		PricePer1kScores:  p.PricePer1KScores,
		IsActive:          p.IsActive,
		IsDefault:         p.IsDefault,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create plan %s: %w", p.Name, err)
	}
	return nil
}

func (r *planRepository) GetByID(ctx context.Context, id uuid.UUID) (*billingDomain.Plan, error) {
	row, err := r.tm.Queries(ctx).GetPlanByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, billingDomain.NewPlanNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get plan %s: %w", id, err)
	}
	return planFromRow(&row), nil
}

func (r *planRepository) GetByName(ctx context.Context, name string) (*billingDomain.Plan, error) {
	row, err := r.tm.Queries(ctx).GetPlanByName(ctx, name)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, billingDomain.NewPlanNotFoundError(name)
		}
		return nil, fmt.Errorf("get plan by name %s: %w", name, err)
	}
	return planFromRow(&row), nil
}

func (r *planRepository) GetDefault(ctx context.Context) (*billingDomain.Plan, error) {
	row, err := r.tm.Queries(ctx).GetDefaultPlan(ctx)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, billingDomain.NewPlanNotFoundError("default")
		}
		return nil, fmt.Errorf("get default plan: %w", err)
	}
	return planFromRow(&row), nil
}

func (r *planRepository) GetActive(ctx context.Context) ([]*billingDomain.Plan, error) {
	rows, err := r.tm.Queries(ctx).ListActivePlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active plans: %w", err)
	}
	out := make([]*billingDomain.Plan, 0, len(rows))
	for i := range rows {
		out = append(out, planFromRow(&rows[i]))
	}
	return out, nil
}

func (r *planRepository) Update(ctx context.Context, p *billingDomain.Plan) error {
	if err := r.tm.Queries(ctx).UpdatePlan(ctx, gen.UpdatePlanParams{
		ID:                p.ID,
		Name:              p.Name,
		FreeSpans:         p.FreeSpans,
		PricePer100kSpans: p.PricePer100KSpans,
		FreeGb:            p.FreeGB,
		PricePerGb:        p.PricePerGB,
		FreeScores:        p.FreeScores,
		PricePer1kScores:  p.PricePer1KScores,
		IsActive:          p.IsActive,
		IsDefault:         p.IsDefault,
	}); err != nil {
		return fmt.Errorf("update plan %s: %w", p.ID, err)
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func planFromRow(row *gen.Plan) *billingDomain.Plan {
	return &billingDomain.Plan{
		ID:                row.ID,
		Name:              row.Name,
		FreeSpans:         row.FreeSpans,
		PricePer100KSpans: row.PricePer100kSpans,
		FreeGB:            row.FreeGb,
		PricePerGB:        row.PricePerGb,
		FreeScores:        row.FreeScores,
		PricePer1KScores:  row.PricePer1kScores,
		IsActive:          row.IsActive,
		IsDefault:         row.IsDefault,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}
