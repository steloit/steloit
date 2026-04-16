package billing

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
)

// BillingService defines the interface for billing operations
type BillingService interface {
	// Usage recording
	RecordUsage(ctx context.Context, usage *CostMetric) error

	// Billing calculation
	CalculateBill(ctx context.Context, orgID uuid.UUID, period string) (*BillingSummary, error)
	GetBillingHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*BillingRecord, error)

	// Payment processing
	ProcessPayment(ctx context.Context, billingRecordID uuid.UUID) error
	CreateBillingRecord(ctx context.Context, summary *BillingSummary) (*BillingRecord, error)

	// Quota management
	CheckUsageQuotas(ctx context.Context, orgID uuid.UUID) (*QuotaStatus, error)

	// Health monitoring
	GetHealth() map[string]interface{}
}

// OrganizationService provides organization-related data for billing context
type OrganizationService interface {
	GetBillingTier(ctx context.Context, orgID uuid.UUID) (string, error)
	GetDiscountRate(ctx context.Context, orgID uuid.UUID) (decimal.Decimal, error)
	GetPaymentMethod(ctx context.Context, orgID uuid.UUID) (*PaymentMethod, error)
}

// QuotaStatus represents the current quota status for an organization
type QuotaStatus struct {
	Status               string    `json:"status"`
	RequestsUsagePercent float64   `json:"requests_usage_percent"`
	TokensUsagePercent   float64   `json:"tokens_usage_percent"`
	CostUsagePercent     float64   `json:"cost_usage_percent"`
	OrganizationID       uuid.UUID `json:"organization_id"`
	RequestsOK           bool      `json:"requests_ok"`
	TokensOK             bool      `json:"tokens_ok"`
	CostOK               bool      `json:"cost_ok"`
}

// ============================================================================
// Usage-Based Billing Services (Spans + GB + Scores)
// ============================================================================

// UsageOverview represents the current usage overview for display
type UsageOverview struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`

	// Current usage (3 dimensions)
	Spans  int64 `json:"spans"`
	Bytes  int64 `json:"bytes"`
	Scores int64 `json:"scores"`

	// Free tier remaining
	FreeSpansRemaining  int64 `json:"free_spans_remaining"`
	FreeBytesRemaining  int64 `json:"free_bytes_remaining"`
	FreeScoresRemaining int64 `json:"free_scores_remaining"`

	// Free tier totals (for progress display)
	FreeSpansTotal  int64 `json:"free_spans_total"`
	FreeBytesTotal  int64 `json:"free_bytes_total"`
	FreeScoresTotal int64 `json:"free_scores_total"`

	// Calculated cost
	EstimatedCost decimal.Decimal `json:"estimated_cost"`
}

// BillableUsageService handles billable usage queries and cost calculation
type BillableUsageService interface {
	// Get current period overview (for dashboard cards)
	GetUsageOverview(ctx context.Context, orgID uuid.UUID) (*UsageOverview, error)

	// Get usage time series (for charts)
	GetUsageTimeSeries(ctx context.Context, orgID uuid.UUID, start, end time.Time, granularity string) ([]*BillableUsage, error)

	// Get usage breakdown by project
	GetUsageByProject(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*BillableUsageSummary, error)

	// Calculate cost for usage
	CalculateCost(ctx context.Context, usage *BillableUsageSummary, plan *Plan) float64

	// ProvisionOrganizationBilling creates initial billing record for new organization
	// Creates billing record with default plan and free tier counters
	// Safe to call within a transaction - uses provided context
	ProvisionOrganizationBilling(ctx context.Context, orgID uuid.UUID) error
}

// BudgetService handles budget CRUD and monitoring
type BudgetService interface {
	// CRUD
	CreateBudget(ctx context.Context, budget *UsageBudget) error
	GetBudget(ctx context.Context, id uuid.UUID) (*UsageBudget, error)
	GetBudgetsByOrg(ctx context.Context, orgID uuid.UUID) ([]*UsageBudget, error)
	UpdateBudget(ctx context.Context, budget *UsageBudget) error
	DeleteBudget(ctx context.Context, id uuid.UUID) error

	// Monitoring
	CheckBudgets(ctx context.Context, orgID uuid.UUID) ([]*UsageAlert, error)
	GetAlerts(ctx context.Context, orgID uuid.UUID, limit int) ([]*UsageAlert, error)
	AcknowledgeAlert(ctx context.Context, orgID, alertID uuid.UUID) error
}

// ============================================================================
// Enterprise Custom Pricing Services
// ============================================================================

// PricingService resolves effective pricing (contract overrides plan)
type PricingService interface {
	// Get effective pricing for an organization (contract overrides > plan defaults)
	GetEffectivePricing(ctx context.Context, orgID uuid.UUID) (*EffectivePricing, error)

	// Get effective pricing using pre-fetched orgBilling to avoid redundant DB query
	// Use this when orgBilling is already available (e.g., in workers)
	GetEffectivePricingWithBilling(ctx context.Context, orgID uuid.UUID, orgBilling *OrganizationBilling) (*EffectivePricing, error)

	// Calculate cost with tier support
	CalculateCostWithTiers(ctx context.Context, orgID uuid.UUID, usage *BillableUsageSummary) (decimal.Decimal, error)

	// Calculate cost with tiers but without free tier deductions
	// Used for project-level budgets where free tier is org-level only
	CalculateCostWithTiersNoFreeTier(ctx context.Context, orgID uuid.UUID, usage *BillableUsageSummary) (decimal.Decimal, error)

	// Calculate cost for a single dimension with tier support (exported for worker usage)
	// Uses absolute position mapping with free tier offset for correct tier calculations
	CalculateDimensionWithTiers(usage, freeTier int64, dimension TierDimension, allTiers []*VolumeDiscountTier, pricing *EffectivePricing) decimal.Decimal
}

// ContractService handles enterprise contract lifecycle
type ContractService interface {
	// CRUD
	CreateContract(ctx context.Context, contract *Contract) error
	GetContract(ctx context.Context, contractID uuid.UUID) (*Contract, error)
	GetContractsByOrg(ctx context.Context, orgID uuid.UUID) ([]*Contract, error)
	GetActiveContract(ctx context.Context, orgID uuid.UUID) (*Contract, error)
	UpdateContract(ctx context.Context, contract *Contract) error

	// Lifecycle management
	ActivateContract(ctx context.Context, contractID uuid.UUID, userID uuid.UUID) error
	CancelContract(ctx context.Context, contractID uuid.UUID, reason string, userID uuid.UUID) error
	ExpireContract(ctx context.Context, contractID uuid.UUID) error

	// Volume tiers
	AddVolumeTiers(ctx context.Context, contractID uuid.UUID, tiers []*VolumeDiscountTier) error
	UpdateVolumeTiers(ctx context.Context, contractID uuid.UUID, tiers []*VolumeDiscountTier) error

	// Audit trail
	GetContractHistory(ctx context.Context, contractID uuid.UUID) ([]*ContractHistory, error)

	// Worker support
	GetExpiringContracts(ctx context.Context, days int) ([]*Contract, error)
}
