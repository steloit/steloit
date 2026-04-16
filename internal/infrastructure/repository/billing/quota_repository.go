package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
)

// Ensure QuotaRepository implements the interface
var _ billingDomain.QuotaRepository = (*QuotaRepository)(nil)

// QuotaRepository handles usage quota management
type QuotaRepository struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewQuotaRepository creates a new quota repository instance
func NewQuotaRepository(db *gorm.DB, logger *slog.Logger) *QuotaRepository {
	return &QuotaRepository{
		db:     db,
		logger: logger,
	}
}

// GetUsageQuota retrieves the usage quota for an organization
func (r *QuotaRepository) GetUsageQuota(ctx context.Context, orgID uuid.UUID) (*billingDomain.UsageQuota, error) {
	query := `
		SELECT
			organization_id, billing_tier, monthly_request_limit, monthly_token_limit,
			monthly_cost_limit, current_requests, current_tokens, current_cost,
			currency, reset_date, last_updated
		FROM usage_quotas
		WHERE organization_id = ?`

	quota := &billingDomain.UsageQuota{}
	err := r.db.WithContext(ctx).Raw(query, orgID).Scan(quota).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No quota found, return nil without error
		}
		return nil, fmt.Errorf("failed to get usage quota: %w", err)
	}

	// Check if we got empty result
	if quota.OrganizationID == uuid.Nil {
		return nil, nil
	}

	return quota, nil
}

// UpdateUsageQuota updates or inserts a usage quota for an organization
func (r *QuotaRepository) UpdateUsageQuota(ctx context.Context, orgID uuid.UUID, quota *billingDomain.UsageQuota) error {
	query := `
		INSERT INTO usage_quotas (
			organization_id, billing_tier, monthly_request_limit, monthly_token_limit,
			monthly_cost_limit, current_requests, current_tokens, current_cost,
			currency, reset_date, last_updated
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?
		)
		ON CONFLICT (organization_id)
		DO UPDATE SET
			billing_tier = EXCLUDED.billing_tier,
			monthly_request_limit = EXCLUDED.monthly_request_limit,
			monthly_token_limit = EXCLUDED.monthly_token_limit,
			monthly_cost_limit = EXCLUDED.monthly_cost_limit,
			current_requests = EXCLUDED.current_requests,
			current_tokens = EXCLUDED.current_tokens,
			current_cost = EXCLUDED.current_cost,
			currency = EXCLUDED.currency,
			reset_date = EXCLUDED.reset_date,
			last_updated = EXCLUDED.last_updated`

	err := r.db.WithContext(ctx).Exec(query,
		quota.OrganizationID,
		quota.BillingTier,
		quota.MonthlyRequestLimit,
		quota.MonthlyTokenLimit,
		quota.MonthlyCostLimit,
		quota.CurrentRequests,
		quota.CurrentTokens,
		quota.CurrentCost,
		quota.Currency,
		quota.ResetDate,
		quota.LastUpdated,
	).Error

	if err != nil {
		r.logger.Error("Failed to update usage quota", "error", err, "org_id", orgID)
		return fmt.Errorf("failed to update usage quota: %w", err)
	}

	r.logger.Debug("Updated usage quota", "org_id", orgID, "billing_tier", quota.BillingTier, "request_limit", quota.MonthlyRequestLimit, "cost_limit", quota.MonthlyCostLimit)

	return nil
}
