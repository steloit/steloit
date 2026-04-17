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

// Ensure UsageRepository implements the interface.
var _ billingDomain.UsageRepository = (*UsageRepository)(nil)

// UsageRepository is the pgx+sqlc implementation of
// billingDomain.UsageRepository. Each row is the billing-side mirror
// of one gateway request; ClickHouse spans remain the source of truth
// for analytics.
type UsageRepository struct {
	tm     *db.TxManager
	logger *slog.Logger
}

// NewUsageRepository returns the pgx-backed repository.
func NewUsageRepository(tm *db.TxManager, logger *slog.Logger) *UsageRepository {
	return &UsageRepository{tm: tm, logger: logger}
}

func (r *UsageRepository) InsertUsageRecord(ctx context.Context, rec *billingDomain.UsageRecord) error {
	if err := r.tm.Queries(ctx).CreateUsageRecord(ctx, gen.CreateUsageRecordParams{
		ID:             rec.ID,
		OrganizationID: rec.OrganizationID,
		RequestID:      rec.RequestID,
		ProviderID:     rec.ProviderID,
		ProviderName:   emptyToNilString(rec.ProviderName),
		ModelID:        rec.ModelID,
		ModelName:      emptyToNilString(rec.ModelName),
		RequestType:    rec.RequestType,
		InputTokens:    rec.InputTokens,
		OutputTokens:   rec.OutputTokens,
		TotalTokens:    rec.TotalTokens,
		Cost:           rec.Cost,
		Currency:       rec.Currency,
		BillingTier:    rec.BillingTier,
		Discounts:      rec.Discounts,
		NetCost:        rec.NetCost,
		CreatedAt:      rec.CreatedAt,
		ProcessedAt:    rec.ProcessedAt,
	}); err != nil {
		r.logger.Error("failed to insert usage record", "error", err, "record_id", rec.ID)
		return fmt.Errorf("insert usage record: %w", err)
	}
	r.logger.Debug("inserted usage record",
		"record_id", rec.ID,
		"organization_id", rec.OrganizationID,
		"net_cost", rec.NetCost,
	)
	return nil
}

func (r *UsageRepository) GetUsageRecords(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.UsageRecord, error) {
	rows, err := r.tm.Queries(ctx).ListUsageRecordsByOrg(ctx, gen.ListUsageRecordsByOrgParams{
		OrganizationID: orgID,
		CreatedAt:      start,
		CreatedAt_2:    end,
	})
	if err != nil {
		return nil, fmt.Errorf("list usage records for org %s: %w", orgID, err)
	}
	out := make([]*billingDomain.UsageRecord, 0, len(rows))
	for i := range rows {
		out = append(out, usageRecordFromRow(&rows[i]))
	}
	return out, nil
}

func (r *UsageRepository) UpdateUsageRecord(ctx context.Context, recordID uuid.UUID, rec *billingDomain.UsageRecord) error {
	n, err := r.tm.Queries(ctx).UpdateUsageRecord(ctx, gen.UpdateUsageRecordParams{
		ID:           recordID,
		RequestType:  rec.RequestType,
		InputTokens:  rec.InputTokens,
		OutputTokens: rec.OutputTokens,
		TotalTokens:  rec.TotalTokens,
		Cost:         rec.Cost,
		Currency:     rec.Currency,
		BillingTier:  rec.BillingTier,
		Discounts:    rec.Discounts,
		NetCost:      rec.NetCost,
		ProcessedAt:  rec.ProcessedAt,
	})
	if err != nil {
		return fmt.Errorf("update usage record %s: %w", recordID, err)
	}
	if n == 0 {
		return fmt.Errorf("usage record not found: %s", recordID)
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func usageRecordFromRow(row *gen.UsageRecord) *billingDomain.UsageRecord {
	return &billingDomain.UsageRecord{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		RequestID:      row.RequestID,
		ProviderID:     row.ProviderID,
		ProviderName:   derefStringBilling(row.ProviderName),
		ModelID:        row.ModelID,
		ModelName:      derefStringBilling(row.ModelName),
		RequestType:    row.RequestType,
		InputTokens:    row.InputTokens,
		OutputTokens:   row.OutputTokens,
		TotalTokens:    row.TotalTokens,
		Cost:           row.Cost,
		Currency:       row.Currency,
		BillingTier:    row.BillingTier,
		Discounts:      row.Discounts,
		NetCost:        row.NetCost,
		CreatedAt:      row.CreatedAt,
		ProcessedAt:    row.ProcessedAt,
	}
}
