package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
)

// Ensure UsageRepository implements the interface
var _ billingDomain.UsageRepository = (*UsageRepository)(nil)

// UsageRepository handles usage tracking persistence
type UsageRepository struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewUsageRepository creates a new usage repository instance
func NewUsageRepository(db *gorm.DB, logger *slog.Logger) *UsageRepository {
	return &UsageRepository{
		db:     db,
		logger: logger,
	}
}

// InsertUsageRecord inserts a new usage record
func (r *UsageRepository) InsertUsageRecord(ctx context.Context, record *billingDomain.UsageRecord) error {
	query := `
		INSERT INTO usage_records (
			id, organization_id, request_id, provider_id, provider_name,
			model_id, model_name, request_type, input_tokens, output_tokens,
			total_tokens, cost, currency, billing_tier, discounts,
			net_cost, created_at, processed_at
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?
		)`

	err := r.db.WithContext(ctx).Exec(query,
		record.ID,
		record.OrganizationID,
		record.RequestID,
		record.ProviderID,
		record.ProviderName,
		record.ModelID,
		record.ModelName,
		record.RequestType,
		record.InputTokens,
		record.OutputTokens,
		record.TotalTokens,
		record.Cost,
		record.Currency,
		record.BillingTier,
		record.Discounts,
		record.NetCost,
		record.CreatedAt,
		record.ProcessedAt,
	).Error

	if err != nil {
		r.logger.Error("Failed to insert usage record", "error", err, "record_id", record.ID)
		return fmt.Errorf("failed to insert usage record: %w", err)
	}

	r.logger.Debug("Inserted usage record", "record_id", record.ID, "organization_id", record.OrganizationID, "net_cost", record.NetCost)

	return nil
}

// GetUsageRecords retrieves usage records for an organization within a time range
func (r *UsageRepository) GetUsageRecords(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.UsageRecord, error) {
	query := `
		SELECT
			id, organization_id, request_id, provider_id, provider_name,
			model_id, model_name, request_type, input_tokens, output_tokens,
			total_tokens, cost, currency, billing_tier, discounts,
			net_cost, created_at, processed_at
		FROM usage_records
		WHERE organization_id = ?
			AND created_at >= ?
			AND created_at < ?
		ORDER BY created_at DESC`

	var records []*billingDomain.UsageRecord
	err := r.db.WithContext(ctx).Raw(query, orgID, start, end).Scan(&records).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get usage records: %w", err)
	}

	return records, nil
}

// UpdateUsageRecord updates an existing usage record
func (r *UsageRepository) UpdateUsageRecord(ctx context.Context, recordID uuid.UUID, record *billingDomain.UsageRecord) error {
	query := `
		UPDATE usage_records
		SET
			request_type = ?,
			input_tokens = ?,
			output_tokens = ?,
			total_tokens = ?,
			cost = ?,
			currency = ?,
			billing_tier = ?,
			discounts = ?,
			net_cost = ?,
			processed_at = ?
		WHERE id = ?`

	result := r.db.WithContext(ctx).Exec(query,
		record.RequestType,
		record.InputTokens,
		record.OutputTokens,
		record.TotalTokens,
		record.Cost,
		record.Currency,
		record.BillingTier,
		record.Discounts,
		record.NetCost,
		record.ProcessedAt,
		recordID,
	)

	if result.Error != nil {
		return fmt.Errorf("failed to update usage record: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("usage record not found: %s", recordID)
	}

	return nil
}
