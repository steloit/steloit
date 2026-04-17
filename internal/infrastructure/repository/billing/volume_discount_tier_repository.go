package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// volumeDiscountTierRepository is the pgx+sqlc implementation of
// billingDomain.VolumeDiscountTierRepository. Each contract owns a
// small set of tiers addressed by (contract_id, dimension); tier_max
// is legitimately nullable (NULL = unlimited upper bound) and passes
// through as *int64 in both domain and gen.
type volumeDiscountTierRepository struct {
	tm *db.TxManager
}

// NewVolumeDiscountTierRepository returns the pgx-backed repository.
func NewVolumeDiscountTierRepository(tm *db.TxManager) billingDomain.VolumeDiscountTierRepository {
	return &volumeDiscountTierRepository{tm: tm}
}

func (r *volumeDiscountTierRepository) Create(ctx context.Context, t *billingDomain.VolumeDiscountTier) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if err := r.tm.Queries(ctx).CreateVolumeDiscountTier(ctx, gen.CreateVolumeDiscountTierParams{
		ID:           t.ID,
		ContractID:   t.ContractID,
		Dimension:    string(t.Dimension),
		TierMin:      t.TierMin,
		TierMax:      t.TierMax,
		PricePerUnit: t.PricePerUnit,
		Priority:     int32(t.Priority),
		CreatedAt:    t.CreatedAt,
	}); err != nil {
		return fmt.Errorf("create volume discount tier: %w", err)
	}
	return nil
}

func (r *volumeDiscountTierRepository) CreateBatch(ctx context.Context, tiers []*billingDomain.VolumeDiscountTier) error {
	if len(tiers) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		for _, t := range tiers {
			if err := r.Create(ctx, t); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *volumeDiscountTierRepository) GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*billingDomain.VolumeDiscountTier, error) {
	rows, err := r.tm.Queries(ctx).ListVolumeDiscountTiersByContract(ctx, contractID)
	if err != nil {
		return nil, fmt.Errorf("get volume tiers for contract %s: %w", contractID, err)
	}
	out := make([]*billingDomain.VolumeDiscountTier, 0, len(rows))
	for i := range rows {
		out = append(out, volumeDiscountTierFromRow(&rows[i]))
	}
	return out, nil
}

func (r *volumeDiscountTierRepository) DeleteByContractID(ctx context.Context, contractID uuid.UUID) error {
	if _, err := r.tm.Queries(ctx).DeleteVolumeDiscountTiersByContract(ctx, contractID); err != nil {
		return fmt.Errorf("delete volume tiers for contract %s: %w", contractID, err)
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func volumeDiscountTierFromRow(row *gen.VolumeDiscountTier) *billingDomain.VolumeDiscountTier {
	return &billingDomain.VolumeDiscountTier{
		ID:           row.ID,
		ContractID:   row.ContractID,
		Dimension:    billingDomain.TierDimension(row.Dimension),
		TierMin:      row.TierMin,
		TierMax:      row.TierMax,
		PricePerUnit: row.PricePerUnit,
		Priority:     int(row.Priority),
		CreatedAt:    row.CreatedAt,
	}
}
