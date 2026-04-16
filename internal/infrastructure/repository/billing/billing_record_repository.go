package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/pkg/uid"
)

// Ensure BillingRecordRepository implements the interface
var _ billingDomain.BillingRecordRepository = (*BillingRecordRepository)(nil)

// Note: BillingRecord and BillingSummary types are now defined in billingDomain
// (previously in deleted analytics worker)

// BillingRecordRepository handles billing records and summaries persistence
type BillingRecordRepository struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewBillingRecordRepository creates a new billing record repository instance
func NewBillingRecordRepository(db *gorm.DB, logger *slog.Logger) *BillingRecordRepository {
	return &BillingRecordRepository{
		db:     db,
		logger: logger,
	}
}

// InsertBillingRecord inserts a new billing record
func (r *BillingRecordRepository) InsertBillingRecord(ctx context.Context, record *billingDomain.BillingRecord) error {
	query := `
		INSERT INTO billing_records (
			id, organization_id, period, amount, currency,
			status, transaction_id, payment_method, created_at, processed_at
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?
		)`

	err := r.db.WithContext(ctx).Exec(query,
		record.ID,
		record.OrganizationID,
		record.Period,
		record.Amount,
		record.Currency,
		record.Status,
		record.TransactionID,
		record.PaymentMethod,
		record.CreatedAt,
		record.ProcessedAt,
	).Error

	if err != nil {
		r.logger.Error("Failed to insert billing record", "error", err, "record_id", record.ID)
		return fmt.Errorf("failed to insert billing record: %w", err)
	}

	r.logger.Debug("Inserted billing record", "record_id", record.ID, "organization_id", record.OrganizationID, "amount", record.Amount, "period", record.Period)

	return nil
}

// UpdateBillingRecord updates an existing billing record
func (r *BillingRecordRepository) UpdateBillingRecord(ctx context.Context, recordID uuid.UUID, record *billingDomain.BillingRecord) error {
	query := `
		UPDATE billing_records
		SET
			period = ?,
			amount = ?,
			currency = ?,
			status = ?,
			transaction_id = ?,
			payment_method = ?,
			processed_at = ?
		WHERE id = ?`

	result := r.db.WithContext(ctx).Exec(query,
		record.Period,
		record.Amount,
		record.Currency,
		record.Status,
		record.TransactionID,
		record.PaymentMethod,
		record.ProcessedAt,
		recordID,
	)

	if result.Error != nil {
		return fmt.Errorf("failed to update billing record: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("billing record not found: %s", recordID)
	}

	return nil
}

// GetBillingRecord retrieves a billing record by ID
func (r *BillingRecordRepository) GetBillingRecord(ctx context.Context, recordID uuid.UUID) (*billingDomain.BillingRecord, error) {
	query := `
		SELECT
			id, organization_id, period, amount, currency,
			status, transaction_id, payment_method, created_at, processed_at
		FROM billing_records
		WHERE id = ?`

	record := &billingDomain.BillingRecord{}
	err := r.db.WithContext(ctx).Raw(query, recordID).Scan(record).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("billing record not found: %s", recordID)
		}
		return nil, fmt.Errorf("failed to get billing record: %w", err)
	}

	return record, nil
}

// GetBillingHistory retrieves billing history for an organization
func (r *BillingRecordRepository) GetBillingHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.BillingRecord, error) {
	query := `
		SELECT
			id, organization_id, period, amount, currency,
			status, transaction_id, payment_method, created_at, processed_at
		FROM billing_records
		WHERE organization_id = ?
			AND created_at >= ?
			AND created_at < ?
		ORDER BY created_at DESC`

	var records []*billingDomain.BillingRecord
	err := r.db.WithContext(ctx).Raw(query, orgID, start, end).Scan(&records).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get billing history: %w", err)
	}

	return records, nil
}

// InsertBillingSummary inserts or updates a billing summary
func (r *BillingRecordRepository) InsertBillingSummary(ctx context.Context, summary *billingDomain.BillingSummary) error {
	providerBreakdownJSON, err := json.Marshal(summary.ProviderBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal provider breakdown: %w", err)
	}

	modelBreakdownJSON, err := json.Marshal(summary.ModelBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal model breakdown: %w", err)
	}

	query := `
		INSERT INTO billing_summaries (
			id, organization_id, period, period_start, period_end,
			total_requests, total_tokens, total_cost, currency,
			provider_breakdown, model_breakdown, discounts, net_cost,
			status, generated_at
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?
		)
		ON CONFLICT (organization_id, period, period_start)
		DO UPDATE SET
			total_requests = EXCLUDED.total_requests,
			total_tokens = EXCLUDED.total_tokens,
			total_cost = EXCLUDED.total_cost,
			provider_breakdown = EXCLUDED.provider_breakdown,
			model_breakdown = EXCLUDED.model_breakdown,
			discounts = EXCLUDED.discounts,
			net_cost = EXCLUDED.net_cost,
			status = EXCLUDED.status,
			generated_at = EXCLUDED.generated_at`

	// Generate ID if not provided
	if summary.ID == uuid.Nil {
		summary.ID = uid.New()
	}

	err = r.db.WithContext(ctx).Exec(query,
		summary.ID,
		summary.OrganizationID,
		summary.Period,
		summary.PeriodStart,
		summary.PeriodEnd,
		summary.TotalRequests,
		summary.TotalTokens,
		summary.TotalCost,
		summary.Currency,
		providerBreakdownJSON,
		modelBreakdownJSON,
		summary.Discounts,
		summary.NetCost,
		summary.Status,
		summary.GeneratedAt,
	).Error

	if err != nil {
		r.logger.Error("Failed to insert billing summary", "error", err, "summary_id", summary.ID)
		return fmt.Errorf("failed to insert billing summary: %w", err)
	}

	r.logger.Debug("Inserted billing summary", "summary_id", summary.ID, "organization_id", summary.OrganizationID, "period", summary.Period, "net_cost", summary.NetCost)

	return nil
}

// GetBillingSummary retrieves a billing summary for an organization and period
func (r *BillingRecordRepository) GetBillingSummary(ctx context.Context, orgID uuid.UUID, period string) (*billingDomain.BillingSummary, error) {
	query := `
		SELECT
			id, organization_id, period, period_start, period_end,
			total_requests, total_tokens, total_cost, currency,
			provider_breakdown, model_breakdown, discounts, net_cost,
			status, generated_at
		FROM billing_summaries
		WHERE organization_id = ? AND period = ?
		ORDER BY period_start DESC
		LIMIT 1`

	type BillingSummaryRow struct {
		GeneratedAt          time.Time
		PeriodStart          time.Time
		PeriodEnd            time.Time
		Currency             string
		Period               string
		Status               string
		ProviderBreakdownRaw []byte `gorm:"column:provider_breakdown"`
		ModelBreakdownRaw    []byte `gorm:"column:model_breakdown"`
		TotalCost            float64
		TotalTokens          int64
		Discounts            float64
		NetCost              float64
		TotalRequests        int64
		ID                   uuid.UUID
		OrganizationID       uuid.UUID
	}

	var row BillingSummaryRow
	err := r.db.WithContext(ctx).Raw(query, orgID, period).Scan(&row).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("billing summary not found for organization %s and period %s", orgID, period)
		}
		return nil, fmt.Errorf("failed to get billing summary: %w", err)
	}

	// Check if we got empty result
	if row.ID == uuid.Nil {
		return nil, fmt.Errorf("billing summary not found for organization %s and period %s", orgID, period)
	}

	summary := &billingDomain.BillingSummary{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Period:         row.Period,
		PeriodStart:    row.PeriodStart,
		PeriodEnd:      row.PeriodEnd,
		TotalRequests:  int(row.TotalRequests),
		TotalTokens:    int(row.TotalTokens),
		TotalCost:      decimal.NewFromFloat(row.TotalCost),
		Currency:       row.Currency,
		Discounts:      decimal.NewFromFloat(row.Discounts),
		NetCost:        decimal.NewFromFloat(row.NetCost),
		Status:         row.Status,
		GeneratedAt:    row.GeneratedAt,
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(row.ProviderBreakdownRaw, &summary.ProviderBreakdown); err != nil {
		return nil, fmt.Errorf("failed to unmarshal provider breakdown: %w", err)
	}

	if err := json.Unmarshal(row.ModelBreakdownRaw, &summary.ModelBreakdown); err != nil {
		return nil, fmt.Errorf("failed to unmarshal model breakdown: %w", err)
	}

	return summary, nil
}

// GetBillingSummaryHistory retrieves billing summary history for an organization
func (r *BillingRecordRepository) GetBillingSummaryHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.BillingSummary, error) {
	query := `
		SELECT
			id, organization_id, period, period_start, period_end,
			total_requests, total_tokens, total_cost, currency,
			provider_breakdown, model_breakdown, discounts, net_cost,
			status, generated_at
		FROM billing_summaries
		WHERE organization_id = ?
			AND period_start >= ?
			AND period_start < ?
		ORDER BY period_start DESC`

	type BillingSummaryRow struct {
		GeneratedAt          time.Time
		PeriodStart          time.Time
		PeriodEnd            time.Time
		Currency             string
		Period               string
		Status               string
		ProviderBreakdownRaw []byte `gorm:"column:provider_breakdown"`
		ModelBreakdownRaw    []byte `gorm:"column:model_breakdown"`
		TotalCost            float64
		TotalTokens          int64
		Discounts            float64
		NetCost              float64
		TotalRequests        int64
		ID                   uuid.UUID
		OrganizationID       uuid.UUID
	}

	var rows []BillingSummaryRow
	err := r.db.WithContext(ctx).Raw(query, orgID, start, end).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get billing summary history: %w", err)
	}

	var summaries []*billingDomain.BillingSummary
	for _, row := range rows {
		summary := &billingDomain.BillingSummary{
			ID:             row.ID,
			OrganizationID: row.OrganizationID,
			Period:         row.Period,
			PeriodStart:    row.PeriodStart,
			PeriodEnd:      row.PeriodEnd,
			TotalRequests:  int(row.TotalRequests),
			TotalTokens:    int(row.TotalTokens),
			TotalCost:      decimal.NewFromFloat(row.TotalCost),
			Currency:       row.Currency,
			Discounts:      decimal.NewFromFloat(row.Discounts),
			NetCost:        decimal.NewFromFloat(row.NetCost),
			Status:         row.Status,
			GeneratedAt:    row.GeneratedAt,
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal(row.ProviderBreakdownRaw, &summary.ProviderBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider breakdown: %w", err)
		}

		if err := json.Unmarshal(row.ModelBreakdownRaw, &summary.ModelBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal model breakdown: %w", err)
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}
