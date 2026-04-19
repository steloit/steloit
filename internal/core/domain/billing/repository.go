package billing

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
)

// ============================================================================
// Billing Repositories
// ============================================================================

// BillingRecordRepository handles billing records and summaries persistence
type BillingRecordRepository interface {
	// Billing records
	InsertBillingRecord(ctx context.Context, record *BillingRecord) error
	UpdateBillingRecord(ctx context.Context, recordID uuid.UUID, record *BillingRecord) error
	GetBillingRecord(ctx context.Context, recordID uuid.UUID) (*BillingRecord, error)
	GetBillingHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*BillingRecord, error)

	// Billing summaries
	InsertBillingSummary(ctx context.Context, summary *BillingSummary) error
	GetBillingSummary(ctx context.Context, orgID uuid.UUID, period string) (*BillingSummary, error)
	GetBillingSummaryHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*BillingSummary, error)
}

// ============================================================================
// Usage-Based Billing Repositories (Spans + GB + Scores)
// ============================================================================

// BillableUsageFilter defines filters for querying billable usage
type BillableUsageFilter struct {
	OrganizationID uuid.UUID
	ProjectID      *uuid.UUID // nil for org-level
	Start          time.Time
	End            time.Time
	Granularity    string // "hourly" or "daily"
}

// BillableUsageRepository handles billable usage data access (ClickHouse)
type BillableUsageRepository interface {
	// Get aggregated usage for a time range
	GetUsage(ctx context.Context, filter *BillableUsageFilter) ([]*BillableUsage, error)

	// Get usage summary (totals) for a period
	GetUsageSummary(ctx context.Context, filter *BillableUsageFilter) (*BillableUsageSummary, error)

	// Get usage breakdown by project
	GetUsageByProject(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*BillableUsageSummary, error)
}

// PlanRepository handles pricing plans (PostgreSQL)
type PlanRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Plan, error)
	GetByName(ctx context.Context, name string) (*Plan, error)
	GetDefault(ctx context.Context) (*Plan, error)
	GetActive(ctx context.Context) ([]*Plan, error)
	Create(ctx context.Context, plan *Plan) error
	Update(ctx context.Context, plan *Plan) error
}

// OrganizationBillingRepository handles org billing state (PostgreSQL)
type OrganizationBillingRepository interface {
	GetByOrgID(ctx context.Context, orgID uuid.UUID) (*OrganizationBilling, error)
	Create(ctx context.Context, billing *OrganizationBilling) error
	Update(ctx context.Context, billing *OrganizationBilling) error

	// SetUsage sets cumulative usage counters and free tier remaining (idempotent - can be called multiple times safely)
	// This replaces values rather than adding, preventing race condition double-counting
	SetUsage(ctx context.Context, orgID uuid.UUID, spans, bytes, scores int64, cost decimal.Decimal, freeSpansRemaining, freeBytesRemaining, freeScoresRemaining int64) error

	ResetPeriod(ctx context.Context, orgID uuid.UUID, newCycleStart time.Time) error
}

// UsageBudgetRepository handles budget CRUD (PostgreSQL)
type UsageBudgetRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*UsageBudget, error)
	GetByOrgID(ctx context.Context, orgID uuid.UUID) ([]*UsageBudget, error)
	GetByProjectID(ctx context.Context, projectID uuid.UUID) ([]*UsageBudget, error)
	GetActive(ctx context.Context, orgID uuid.UUID) ([]*UsageBudget, error)
	Create(ctx context.Context, budget *UsageBudget) error
	Update(ctx context.Context, budget *UsageBudget) error
	UpdateUsage(ctx context.Context, budgetID uuid.UUID, spans, bytes, scores int64, cost decimal.Decimal) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// UsageAlertRepository handles alert history (PostgreSQL)
type UsageAlertRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*UsageAlert, error)
	GetByOrgID(ctx context.Context, orgID uuid.UUID, limit int) ([]*UsageAlert, error)
	GetByBudgetID(ctx context.Context, budgetID uuid.UUID) ([]*UsageAlert, error)
	GetUnacknowledged(ctx context.Context, orgID uuid.UUID) ([]*UsageAlert, error)
	Create(ctx context.Context, alert *UsageAlert) error
	Acknowledge(ctx context.Context, id uuid.UUID) error
	Resolve(ctx context.Context, id uuid.UUID) error
	MarkNotificationSent(ctx context.Context, id uuid.UUID) error
}

// ============================================================================
// Enterprise Custom Pricing Repositories
// ============================================================================

// ContractRepository handles enterprise contract CRUD (PostgreSQL)
//
// Error Return Patterns:
//   - Required Records (GetByID): Returns (nil, error) if not found
//   - Optional Records (GetActiveByOrgID): Returns (nil, nil) if not found
//   - Collections (GetByOrgID): Returns ([], nil) if empty
type ContractRepository interface {
	Create(ctx context.Context, contract *Contract) error

	// GetByID retrieves a contract by its primary key.
	// Returns (nil, error) if contract does not exist (required record).
	GetByID(ctx context.Context, id uuid.UUID) (*Contract, error)

	// GetActiveByOrgID retrieves the active contract for an organization.
	// Returns (nil, nil) if no active contract exists (optional record).
	// Returns (nil, error) for database errors only.
	GetActiveByOrgID(ctx context.Context, orgID uuid.UUID) (*Contract, error)

	// GetByOrgID retrieves all contracts for an organization.
	// Returns ([], nil) if organization has no contracts (empty collection is valid).
	GetByOrgID(ctx context.Context, orgID uuid.UUID) ([]*Contract, error)

	Update(ctx context.Context, contract *Contract) error
	Expire(ctx context.Context, contractID uuid.UUID) error
	Cancel(ctx context.Context, contractID uuid.UUID) error

	// GetExpiring retrieves contracts expiring on or before the target time.
	// Uses timestamp-based comparison (not date-only).
	// The days parameter specifies how many days from now:
	//   - days = 0: contracts with expires_at <= now (expired already)
	//   - days = 1: contracts with expires_at <= now + 24 hours
	//   - days = -1: contracts with expires_at <= now - 24 hours
	// Example: Worker runs Jan 9 00:00, finds contract expiring Jan 8 10:15 (14 hours ago).
	// Returns ([], nil) if no contracts are expiring (collection).
	GetExpiring(ctx context.Context, days int) ([]*Contract, error)
}

// VolumeDiscountTierRepository handles volume pricing tiers (PostgreSQL)
type VolumeDiscountTierRepository interface {
	Create(ctx context.Context, tier *VolumeDiscountTier) error
	CreateBatch(ctx context.Context, tiers []*VolumeDiscountTier) error
	GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*VolumeDiscountTier, error)
	DeleteByContractID(ctx context.Context, contractID uuid.UUID) error
}

// ContractHistoryRepository handles contract audit trail (PostgreSQL)
type ContractHistoryRepository interface {
	Log(ctx context.Context, history *ContractHistory) error
	GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*ContractHistory, error)
}
