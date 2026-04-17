package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	"brokle/pkg/uid"
)

// Ensure BillingRecordRepository implements the interface.
var _ billingDomain.BillingRecordRepository = (*BillingRecordRepository)(nil)

// BillingRecordRepository is the pgx+sqlc implementation of
// billingDomain.BillingRecordRepository. billing_records holds payment
// transactions; billing_summaries holds per-period aggregates (one row
// per org × period × period_start, upserted on replay).
//
// Domain fields absent from the schema (BillingRecord.UpdatedAt /
// Metadata, BillingSummary.CreatedAt / TotalAmount / RecordCount) are
// dropped at the boundary — they will be stripped from the domain in
// P2.B.11 once billing finishes migrating.
type BillingRecordRepository struct {
	tm     *db.TxManager
	logger *slog.Logger
}

// NewBillingRecordRepository returns the pgx-backed repository.
func NewBillingRecordRepository(tm *db.TxManager, logger *slog.Logger) *BillingRecordRepository {
	return &BillingRecordRepository{tm: tm, logger: logger}
}

// ----- billing_records ----------------------------------------------

func (r *BillingRecordRepository) InsertBillingRecord(ctx context.Context, rec *billingDomain.BillingRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	if err := r.tm.Queries(ctx).CreateBillingRecord(ctx, gen.CreateBillingRecordParams{
		ID:             rec.ID,
		OrganizationID: rec.OrganizationID,
		Period:         rec.Period,
		Amount:         rec.Amount,
		Currency:       rec.Currency,
		Status:         rec.Status,
		TransactionID:  rec.TransactionID,
		PaymentMethod:  rec.PaymentMethod,
		CreatedAt:      rec.CreatedAt,
		ProcessedAt:    rec.ProcessedAt,
	}); err != nil {
		r.logger.Error("failed to insert billing record", "error", err, "record_id", rec.ID)
		return fmt.Errorf("insert billing record: %w", err)
	}
	r.logger.Debug("inserted billing record",
		"record_id", rec.ID,
		"organization_id", rec.OrganizationID,
		"amount", rec.Amount,
		"period", rec.Period,
	)
	return nil
}

func (r *BillingRecordRepository) UpdateBillingRecord(ctx context.Context, recordID uuid.UUID, rec *billingDomain.BillingRecord) error {
	n, err := r.tm.Queries(ctx).UpdateBillingRecord(ctx, gen.UpdateBillingRecordParams{
		ID:            recordID,
		Period:        rec.Period,
		Amount:        rec.Amount,
		Currency:      rec.Currency,
		Status:        rec.Status,
		TransactionID: rec.TransactionID,
		PaymentMethod: rec.PaymentMethod,
		ProcessedAt:   rec.ProcessedAt,
	})
	if err != nil {
		return fmt.Errorf("update billing record %s: %w", recordID, err)
	}
	if n == 0 {
		return fmt.Errorf("billing record not found: %s", recordID)
	}
	return nil
}

func (r *BillingRecordRepository) GetBillingRecord(ctx context.Context, recordID uuid.UUID) (*billingDomain.BillingRecord, error) {
	row, err := r.tm.Queries(ctx).GetBillingRecordByID(ctx, recordID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("billing record not found: %s", recordID)
		}
		return nil, fmt.Errorf("get billing record %s: %w", recordID, err)
	}
	return billingRecordFromRow(&row), nil
}

func (r *BillingRecordRepository) GetBillingHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.BillingRecord, error) {
	rows, err := r.tm.Queries(ctx).ListBillingRecordsByOrg(ctx, gen.ListBillingRecordsByOrgParams{
		OrganizationID: orgID,
		CreatedAt:      start,
		CreatedAt_2:    end,
	})
	if err != nil {
		return nil, fmt.Errorf("list billing history for org %s: %w", orgID, err)
	}
	out := make([]*billingDomain.BillingRecord, 0, len(rows))
	for i := range rows {
		out = append(out, billingRecordFromRow(&rows[i]))
	}
	return out, nil
}

// ----- billing_summaries --------------------------------------------

func (r *BillingRecordRepository) InsertBillingSummary(ctx context.Context, s *billingDomain.BillingSummary) error {
	if s.ID == uuid.Nil {
		s.ID = uid.New()
	}
	if s.GeneratedAt.IsZero() {
		s.GeneratedAt = time.Now()
	}
	providerJSON, err := marshalBreakdown(s.ProviderBreakdown)
	if err != nil {
		return fmt.Errorf("marshal provider breakdown: %w", err)
	}
	modelJSON, err := marshalBreakdown(s.ModelBreakdown)
	if err != nil {
		return fmt.Errorf("marshal model breakdown: %w", err)
	}
	if err := r.tm.Queries(ctx).UpsertBillingSummary(ctx, gen.UpsertBillingSummaryParams{
		ID:                s.ID,
		OrganizationID:    s.OrganizationID,
		Period:            s.Period,
		PeriodStart:       s.PeriodStart,
		PeriodEnd:         s.PeriodEnd,
		TotalRequests:     int64(s.TotalRequests),
		TotalTokens:       int64(s.TotalTokens),
		TotalCost:         s.TotalCost,
		Currency:          s.Currency,
		ProviderBreakdown: providerJSON,
		ModelBreakdown:    modelJSON,
		Discounts:         s.Discounts,
		NetCost:           s.NetCost,
		Status:            s.Status,
		GeneratedAt:       s.GeneratedAt,
	}); err != nil {
		r.logger.Error("failed to upsert billing summary", "error", err, "summary_id", s.ID)
		return fmt.Errorf("upsert billing summary: %w", err)
	}
	r.logger.Debug("upserted billing summary",
		"summary_id", s.ID,
		"organization_id", s.OrganizationID,
		"period", s.Period,
		"net_cost", s.NetCost,
	)
	return nil
}

func (r *BillingRecordRepository) GetBillingSummary(ctx context.Context, orgID uuid.UUID, period string) (*billingDomain.BillingSummary, error) {
	row, err := r.tm.Queries(ctx).GetLatestBillingSummary(ctx, gen.GetLatestBillingSummaryParams{
		OrganizationID: orgID,
		Period:         period,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("billing summary not found for organization %s and period %s", orgID, period)
		}
		return nil, fmt.Errorf("get billing summary for %s/%s: %w", orgID, period, err)
	}
	return billingSummaryFromRow(&row)
}

func (r *BillingRecordRepository) GetBillingSummaryHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.BillingSummary, error) {
	rows, err := r.tm.Queries(ctx).ListBillingSummariesByOrg(ctx, gen.ListBillingSummariesByOrgParams{
		OrganizationID: orgID,
		PeriodStart:    start,
		PeriodStart_2:  end,
	})
	if err != nil {
		return nil, fmt.Errorf("list billing summaries for org %s: %w", orgID, err)
	}
	out := make([]*billingDomain.BillingSummary, 0, len(rows))
	for i := range rows {
		s, err := billingSummaryFromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func billingRecordFromRow(row *gen.BillingRecord) *billingDomain.BillingRecord {
	return &billingDomain.BillingRecord{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Period:         row.Period,
		Amount:         row.Amount,
		Currency:       row.Currency,
		Status:         row.Status,
		TransactionID:  row.TransactionID,
		PaymentMethod:  row.PaymentMethod,
		CreatedAt:      row.CreatedAt,
		ProcessedAt:    row.ProcessedAt,
	}
}

func billingSummaryFromRow(row *gen.BillingSummary) (*billingDomain.BillingSummary, error) {
	s := &billingDomain.BillingSummary{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Period:         row.Period,
		PeriodStart:    row.PeriodStart,
		PeriodEnd:      row.PeriodEnd,
		TotalRequests:  int(row.TotalRequests),
		TotalTokens:    int(row.TotalTokens),
		TotalCost:      row.TotalCost,
		Currency:       row.Currency,
		Discounts:      row.Discounts,
		NetCost:        row.NetCost,
		Status:         row.Status,
		GeneratedAt:    row.GeneratedAt,
	}
	if err := unmarshalBreakdown(row.ProviderBreakdown, &s.ProviderBreakdown); err != nil {
		return nil, fmt.Errorf("unmarshal provider breakdown: %w", err)
	}
	if err := unmarshalBreakdown(row.ModelBreakdown, &s.ModelBreakdown); err != nil {
		return nil, fmt.Errorf("unmarshal model breakdown: %w", err)
	}
	return s, nil
}

// marshalBreakdown encodes a map[string]any to JSONB. An empty/nil map
// becomes `{}` (schema requires NOT NULL with DEFAULT '{}').
func marshalBreakdown(m map[string]interface{}) (json.RawMessage, error) {
	if len(m) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return json.Marshal(m)
}

func unmarshalBreakdown(raw json.RawMessage, dst *map[string]interface{}) error {
	if len(raw) == 0 {
		*dst = map[string]interface{}{}
		return nil
	}
	return json.Unmarshal(raw, dst)
}
