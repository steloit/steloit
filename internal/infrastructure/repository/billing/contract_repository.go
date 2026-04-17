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

// contractRepository is the pgx+sqlc implementation of
// billingDomain.ContractRepository. Volume tiers are preloaded by
// fetching them separately per returned contract — sqlc doesn't do
// GORM-style Preload, and one extra round-trip per Get is cheaper
// than the JSON-aggregation dance here.
type contractRepository struct {
	tm *db.TxManager
}

// NewContractRepository returns the pgx-backed repository.
func NewContractRepository(tm *db.TxManager) billingDomain.ContractRepository {
	return &contractRepository{tm: tm}
}

func (r *contractRepository) Create(ctx context.Context, c *billingDomain.Contract) error {
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	if c.Currency == "" {
		c.Currency = "USD"
	}
	if c.Status == "" {
		c.Status = billingDomain.ContractStatusDraft
	}
	createdBy, err := parseActorID(c.CreatedBy)
	if err != nil {
		return fmt.Errorf("create contract %s: %w", c.ID, err)
	}
	if err := r.tm.Queries(ctx).CreateContract(ctx, gen.CreateContractParams{
		ID:                      c.ID,
		OrganizationID:          c.OrganizationID,
		ContractName:            c.ContractName,
		ContractNumber:          c.ContractNumber,
		StartDate:               c.StartDate,
		EndDate:                 c.EndDate,
		MinimumCommitAmount:     c.MinimumCommitAmount,
		Currency:                c.Currency,
		AccountOwner:            emptyToNilString(c.AccountOwner),
		SalesRepEmail:           emptyToNilString(c.SalesRepEmail),
		Status:                  string(c.Status),
		CustomFreeSpans:         c.CustomFreeSpans,
		CustomPricePer100kSpans: c.CustomPricePer100KSpans,
		CustomFreeGb:            c.CustomFreeGB,
		CustomPricePerGb:        c.CustomPricePerGB,
		CustomFreeScores:        c.CustomFreeScores,
		CustomPricePer1kScores:  c.CustomPricePer1KScores,
		CreatedBy:               createdBy,
		CreatedAt:               c.CreatedAt,
		UpdatedAt:               c.UpdatedAt,
		Notes:                   emptyToNilString(c.Notes),
	}); err != nil {
		return fmt.Errorf("create contract %s: %w", c.ID, err)
	}
	return nil
}

func (r *contractRepository) GetByID(ctx context.Context, id uuid.UUID) (*billingDomain.Contract, error) {
	row, err := r.tm.Queries(ctx).GetContractByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, billingDomain.NewContractNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get contract %s: %w", id, err)
	}
	c := contractFromRow(&row)
	if err := r.loadTiers(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetActiveByOrgID returns (nil, nil) when no active contract exists —
// preserves the documented "optional record" contract.
func (r *contractRepository) GetActiveByOrgID(ctx context.Context, orgID uuid.UUID) (*billingDomain.Contract, error) {
	row, err := r.tm.Queries(ctx).GetActiveContractByOrg(ctx, orgID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get active contract for org %s: %w", orgID, err)
	}
	c := contractFromRow(&row)
	if err := r.loadTiers(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *contractRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) ([]*billingDomain.Contract, error) {
	rows, err := r.tm.Queries(ctx).ListContractsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list contracts for org %s: %w", orgID, err)
	}
	out := make([]*billingDomain.Contract, 0, len(rows))
	for i := range rows {
		c := contractFromRow(&rows[i])
		if err := r.loadTiers(ctx, c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *contractRepository) Update(ctx context.Context, c *billingDomain.Contract) error {
	c.UpdatedAt = time.Now()
	if err := r.tm.Queries(ctx).UpdateContract(ctx, gen.UpdateContractParams{
		ID:                      c.ID,
		ContractName:            c.ContractName,
		ContractNumber:          c.ContractNumber,
		StartDate:               c.StartDate,
		EndDate:                 c.EndDate,
		MinimumCommitAmount:     c.MinimumCommitAmount,
		Currency:                c.Currency,
		AccountOwner:            emptyToNilString(c.AccountOwner),
		SalesRepEmail:           emptyToNilString(c.SalesRepEmail),
		Status:                  string(c.Status),
		CustomFreeSpans:         c.CustomFreeSpans,
		CustomPricePer100kSpans: c.CustomPricePer100KSpans,
		CustomFreeGb:            c.CustomFreeGB,
		CustomPricePerGb:        c.CustomPricePerGB,
		CustomFreeScores:        c.CustomFreeScores,
		CustomPricePer1kScores:  c.CustomPricePer1KScores,
		Notes:                   emptyToNilString(c.Notes),
	}); err != nil {
		return fmt.Errorf("update contract %s: %w", c.ID, err)
	}
	return nil
}

func (r *contractRepository) Expire(ctx context.Context, contractID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).ExpireContract(ctx, contractID)
	if err != nil {
		return fmt.Errorf("expire contract %s: %w", contractID, err)
	}
	if n == 0 {
		return billingDomain.NewContractNotFoundError(contractID.String())
	}
	return nil
}

func (r *contractRepository) Cancel(ctx context.Context, contractID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).CancelContract(ctx, contractID)
	if err != nil {
		return fmt.Errorf("cancel contract %s: %w", contractID, err)
	}
	if n == 0 {
		return billingDomain.NewContractNotFoundError(contractID.String())
	}
	return nil
}

func (r *contractRepository) GetExpiring(ctx context.Context, days int) ([]*billingDomain.Contract, error) {
	target := time.Now().UTC().AddDate(0, 0, days)
	rows, err := r.tm.Queries(ctx).ListExpiringContracts(ctx, &target)
	if err != nil {
		return nil, fmt.Errorf("list expiring contracts (days=%d): %w", days, err)
	}
	out := make([]*billingDomain.Contract, 0, len(rows))
	for i := range rows {
		c := contractFromRow(&rows[i])
		if err := r.loadTiers(ctx, c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// loadTiers populates c.VolumeTiers via a separate query. One round-trip
// per contract — acceptable given the expected cardinality (enterprise
// contracts are low-volume).
func (r *contractRepository) loadTiers(ctx context.Context, c *billingDomain.Contract) error {
	tierRows, err := r.tm.Queries(ctx).ListVolumeDiscountTiersByContract(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("load volume tiers for contract %s: %w", c.ID, err)
	}
	c.VolumeTiers = make([]billingDomain.VolumeDiscountTier, 0, len(tierRows))
	for i := range tierRows {
		c.VolumeTiers = append(c.VolumeTiers, *volumeDiscountTierFromRow(&tierRows[i]))
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func contractFromRow(row *gen.Contract) *billingDomain.Contract {
	var createdBy string
	if row.CreatedBy != nil {
		createdBy = row.CreatedBy.String()
	}
	return &billingDomain.Contract{
		ID:                      row.ID,
		OrganizationID:          row.OrganizationID,
		ContractName:            row.ContractName,
		ContractNumber:          row.ContractNumber,
		StartDate:               row.StartDate,
		EndDate:                 row.EndDate,
		MinimumCommitAmount:     row.MinimumCommitAmount,
		Currency:                row.Currency,
		AccountOwner:            derefStringBilling(row.AccountOwner),
		SalesRepEmail:           derefStringBilling(row.SalesRepEmail),
		Status:                  billingDomain.ContractStatus(row.Status),
		CustomFreeSpans:         row.CustomFreeSpans,
		CustomPricePer100KSpans: row.CustomPricePer100kSpans,
		CustomFreeGB:            row.CustomFreeGb,
		CustomPricePerGB:        row.CustomPricePerGb,
		CustomFreeScores:        row.CustomFreeScores,
		CustomPricePer1KScores:  row.CustomPricePer1kScores,
		CreatedBy:               createdBy,
		CreatedAt:               row.CreatedAt,
		UpdatedAt:               row.UpdatedAt,
		Notes:                   derefStringBilling(row.Notes),
	}
}
