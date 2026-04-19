package billing

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
)

// Billing Entities

type PaymentMethod struct {
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Type           string    `json:"type"`
	Provider       string    `json:"provider"`
	ExternalID     string    `json:"external_id"`
	Last4          string    `json:"last_4"`
	ExpiryMonth    int       `json:"expiry_month"`
	ExpiryYear     int       `json:"expiry_year"`
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	IsDefault      bool      `json:"is_default"`
}

type Invoice struct {
	DueDate          time.Time              `json:"due_date"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CreatedAt        time.Time              `json:"created_at"`
	PeriodStart      time.Time              `json:"period_start"`
	PeriodEnd        time.Time              `json:"period_end"`
	IssueDate        time.Time              `json:"issue_date"`
	PaidAt           *time.Time             `json:"paid_at,omitempty"`
	BillingAddress   *BillingAddress        `json:"billing_address"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Currency         string                 `json:"currency"`
	Period           string                 `json:"period"`
	InvoiceNumber    string                 `json:"invoice_number"`
	OrganizationName string                 `json:"organization_name"`
	Notes            string                 `json:"notes,omitempty"`
	PaymentTerms     string                 `json:"payment_terms"`
	Status           InvoiceStatus          `json:"status"`
	LineItems        []InvoiceLineItem      `json:"line_items"`
	TotalAmount      decimal.Decimal        `json:"total_amount"`
	DiscountAmount   decimal.Decimal        `json:"discount_amount"`
	TaxAmount        decimal.Decimal        `json:"tax_amount"`
	Subtotal         decimal.Decimal        `json:"subtotal"`
	ID               uuid.UUID              `json:"id"`
	OrganizationID   uuid.UUID              `json:"organization_id"`
}

type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusSent      InvoiceStatus = "sent"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusOverdue   InvoiceStatus = "overdue"
	InvoiceStatusCancelled InvoiceStatus = "cancelled"
	InvoiceStatusRefunded  InvoiceStatus = "refunded"
)

type InvoiceLineItem struct {
	ProviderID   *uuid.UUID      `json:"provider_id,omitempty"`
	ModelID      *uuid.UUID      `json:"model_id,omitempty"`
	Description  string          `json:"description"`
	ProviderName string          `json:"provider_name,omitempty"`
	ModelName    string          `json:"model_name,omitempty"`
	RequestType  string          `json:"request_type,omitempty"`
	Quantity     decimal.Decimal `json:"quantity"`
	UnitPrice    decimal.Decimal `json:"unit_price"`
	Amount       decimal.Decimal `json:"amount"`
	Tokens       int64           `json:"tokens,omitempty"`
	Requests     int64           `json:"requests,omitempty"`
	ID           uuid.UUID       `json:"id"`
}

type BillingAddress struct {
	Company    string `json:"company"`
	Address1   string `json:"address_1"`
	Address2   string `json:"address_2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
	TaxID      string `json:"tax_id,omitempty"`
}

type TaxConfiguration struct {
	TaxName     string          `json:"tax_name"`
	TaxID       string          `json:"tax_id"`
	TaxRate     decimal.Decimal `json:"tax_rate"`
	IsInclusive bool            `json:"is_inclusive"`
}

type DiscountRule struct {
	UpdatedAt       time.Time          `json:"updated_at"`
	CreatedAt       time.Time          `json:"created_at"`
	ValidFrom       time.Time          `json:"valid_from"`
	Conditions      *DiscountCondition `json:"conditions,omitempty"`
	OrganizationID  *uuid.UUID         `json:"organization_id,omitempty"`
	UsageLimit      *int               `json:"usage_limit,omitempty"`
	ValidUntil      *time.Time         `json:"valid_until,omitempty"`
	Type            DiscountType       `json:"type"`
	Description     string             `json:"description"`
	Name            string             `json:"name"`
	MaximumDiscount decimal.Decimal    `json:"maximum_discount"`
	MinimumAmount   decimal.Decimal    `json:"minimum_amount"`
	Value           decimal.Decimal    `json:"value"`
	UsageCount      int                `json:"usage_count"`
	Priority        int                `json:"priority"`
	ID              uuid.UUID          `json:"id"`
	IsActive        bool               `json:"is_active"`
}

type DiscountType string

const (
	DiscountTypePercentage DiscountType = "percentage"
	DiscountTypeFixed      DiscountType = "fixed"
	DiscountTypeTiered     DiscountType = "tiered"
)

type DiscountCondition struct {
	MinUsage          *UsageThreshold `json:"min_usage,omitempty"`
	TimeOfDay         *TimeRange      `json:"time_of_day,omitempty"`
	VolumeThreshold   *VolumeDiscount `json:"volume_threshold,omitempty"`
	BillingTiers      []string        `json:"billing_tiers,omitempty"`
	RequestTypes      []string        `json:"request_types,omitempty"`
	Providers         []uuid.UUID     `json:"providers,omitempty"`
	Models            []uuid.UUID     `json:"models,omitempty"`
	DaysOfWeek        []time.Weekday  `json:"days_of_week,omitempty"`
	FirstTimeCustomer bool            `json:"first_time_customer"`
}

type UsageThreshold struct {
	Requests int64           `json:"requests"`
	Tokens   int64           `json:"tokens"`
	Cost     decimal.Decimal `json:"cost"`
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type VolumeDiscount struct {
	Tiers []VolumeTier `json:"tiers"`
}

type VolumeTier struct {
	MinAmount decimal.Decimal `json:"min_amount"`
	Discount  decimal.Decimal `json:"discount"` // percentage or fixed amount
}

// BillingRecord represents a billing record (moved from deleted analytics worker)
type BillingRecord struct {
	UpdatedAt      time.Time              `json:"updated_at"`
	CreatedAt      time.Time              `json:"created_at"`
	Metadata       map[string]any `json:"metadata"`
	TransactionID  *string                `json:"transaction_id,omitempty"`
	PaymentMethod  *string                `json:"payment_method,omitempty"`
	ProcessedAt    *time.Time             `json:"processed_at,omitempty"`
	Period         string                 `json:"period"`
	Currency       string                 `json:"currency"`
	Status         string                 `json:"status"`
	Amount         decimal.Decimal        `json:"amount"`
	NetCost        decimal.Decimal        `json:"net_cost"`
	ID             uuid.UUID              `json:"id"`
	OrganizationID uuid.UUID              `json:"organization_id"`
}

// BillingSummary represents aggregated billing data (moved from deleted analytics worker)
type BillingSummary struct {
	PeriodStart       time.Time              `json:"period_start"`
	PeriodEnd         time.Time              `json:"period_end"`
	CreatedAt         time.Time              `json:"created_at"`
	GeneratedAt       time.Time              `json:"generated_at"`
	ModelBreakdown    map[string]any `json:"model_breakdown"`
	ProviderBreakdown map[string]any `json:"provider_breakdown"`
	Currency          string                 `json:"currency"`
	Period            string                 `json:"period"`
	Status            string                 `json:"status"`
	TotalAmount       decimal.Decimal        `json:"total_amount"`
	Discounts         decimal.Decimal        `json:"discounts"`
	NetCost           decimal.Decimal        `json:"net_cost"`
	RecordCount       int                    `json:"record_count"`
	TotalCost         decimal.Decimal        `json:"total_cost"`
	TotalTokens       int                    `json:"total_tokens"`
	TotalRequests     int                    `json:"total_requests"`
	ID                uuid.UUID              `json:"id"`
	OrganizationID    uuid.UUID              `json:"organization_id"`
}


// Usage-Based Billing Entities

// Queried from ClickHouse billable_usage_hourly/daily tables
type BillableUsage struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	ProjectID      uuid.UUID `json:"project_id"`
	BucketTime     time.Time `json:"bucket_time"`

	// Three billable dimensions
	SpanCount      int64 `json:"span_count"`      // All spans (traces + child spans)
	BytesProcessed int64 `json:"bytes_processed"` // Total payload bytes (input + output)
	ScoreCount     int64 `json:"score_count"`     // Quality scores

	// Informational (not billable by Brokle)
	AIProviderCost decimal.Decimal `json:"ai_provider_cost"`

	LastUpdated time.Time `json:"last_updated"`
}

type BillableUsageSummary struct {
	OrganizationID uuid.UUID  `json:"organization_id"`
	ProjectID      *uuid.UUID `json:"project_id,omitempty"` // nil for org-level summary
	PeriodStart    time.Time  `json:"period_start"`
	PeriodEnd      time.Time  `json:"period_end"`

	// Totals for period
	TotalSpans  int64 `json:"total_spans"`
	TotalBytes  int64 `json:"total_bytes"`
	TotalScores int64 `json:"total_scores"`

	// Calculated cost
	TotalCost decimal.Decimal `json:"total_cost"`

	// Informational
	TotalAIProviderCost decimal.Decimal `json:"total_ai_provider_cost"`
}

type Plan struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"` // free, pro, enterprise
	IsActive  bool      `json:"is_active"`
	IsDefault bool      `json:"is_default"` // Default plan for new organizations
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Span pricing (per 100K)
	FreeSpans         int64            `json:"free_spans"`
	PricePer100KSpans *decimal.Decimal `json:"price_per_100k_spans,omitempty"` // nil = unlimited in free tier

	// Data volume pricing (per GB)
	FreeGB     decimal.Decimal  `json:"free_gb"`
	PricePerGB *decimal.Decimal `json:"price_per_gb,omitempty"` // nil = unlimited in free tier

	// Score pricing (per 1K)
	FreeScores       int64            `json:"free_scores"`
	PricePer1KScores *decimal.Decimal `json:"price_per_1k_scores,omitempty"` // nil = unlimited in free tier
}

type OrganizationBilling struct {
	OrganizationID        uuid.UUID `json:"organization_id"`
	PlanID                uuid.UUID `json:"plan_id"`
	BillingCycleStart     time.Time `json:"billing_cycle_start"`
	BillingCycleAnchorDay int       `json:"billing_cycle_anchor_day"` // Day of month (1-28)

	// Current period usage (three dimensions)
	CurrentPeriodSpans  int64 `json:"current_period_spans"`
	CurrentPeriodBytes  int64 `json:"current_period_bytes"`
	CurrentPeriodScores int64 `json:"current_period_scores"`

	// Calculated cost this period
	CurrentPeriodCost decimal.Decimal `json:"current_period_cost"`

	// Free tier remaining (three dimensions)
	FreeSpansRemaining  int64 `json:"free_spans_remaining"`
	FreeBytesRemaining  int64 `json:"free_bytes_remaining"`
	FreeScoresRemaining int64 `json:"free_scores_remaining"`

	LastSyncedAt time.Time `json:"last_synced_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BudgetType string

const (
	BudgetTypeMonthly BudgetType = "monthly"
	BudgetTypeWeekly  BudgetType = "weekly"
)

type UsageBudget struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ProjectID      *uuid.UUID `json:"project_id,omitempty"` // nil for org-level budget
	Name           string     `json:"name"`
	BudgetType     BudgetType `json:"budget_type"`

	// Limits (any can be set, nil = no limit)
	SpanLimit  *int64           `json:"span_limit,omitempty"`
	BytesLimit *int64           `json:"bytes_limit,omitempty"`
	ScoreLimit *int64           `json:"score_limit,omitempty"`
	CostLimit  *decimal.Decimal `json:"cost_limit,omitempty"`

	// Current usage
	CurrentSpans  int64           `json:"current_spans"`
	CurrentBytes  int64           `json:"current_bytes"`
	CurrentScores int64           `json:"current_scores"`
	CurrentCost   decimal.Decimal `json:"current_cost"`

	// Alert thresholds (flexible array of percentages, e.g., [50, 80, 100])
	AlertThresholds []int64 `json:"alert_thresholds" swaggertype:"array,integer"`

	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AlertDimension string

const (
	AlertDimensionSpans  AlertDimension = "spans"
	AlertDimensionBytes  AlertDimension = "bytes"
	AlertDimensionScores AlertDimension = "scores"
	AlertDimensionCost   AlertDimension = "cost"
)

type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

type AlertStatus string

const (
	AlertStatusTriggered    AlertStatus = "triggered"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
)

type UsageAlert struct {
	ID             uuid.UUID  `json:"id"`
	BudgetID       *uuid.UUID `json:"budget_id,omitempty"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ProjectID      *uuid.UUID `json:"project_id,omitempty"`

	AlertThreshold int64           `json:"alert_threshold"`
	Dimension      AlertDimension  `json:"dimension"`
	Severity       AlertSeverity   `json:"severity"`
	ThresholdValue int64           `json:"threshold_value"`
	ActualValue    int64           `json:"actual_value"`
	PercentUsed    decimal.Decimal `json:"percent_used"`

	Status           AlertStatus `json:"status"`
	TriggeredAt      time.Time   `json:"triggered_at"`
	AcknowledgedAt   *time.Time  `json:"acknowledged_at,omitempty"`
	ResolvedAt       *time.Time  `json:"resolved_at,omitempty"`
	NotificationSent bool        `json:"notification_sent"`
}

// Enterprise Contracts & Custom Pricing

type ContractStatus string

const (
	ContractStatusDraft     ContractStatus = "draft"
	ContractStatusActive    ContractStatus = "active"
	ContractStatusExpired   ContractStatus = "expired"
	ContractStatusCancelled ContractStatus = "cancelled"
)

type Contract struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	ContractName   string    `json:"contract_name"`
	ContractNumber string    `json:"contract_number"`

	// Timestamps (full precision, no normalization)
	// Access rule: contract is active when now < EndDate
	StartDate time.Time  `json:"start_date"`       // Contract start timestamp (e.g., 2026-01-08T10:15:00Z)
	EndDate   *time.Time `json:"end_date,omitempty"` // Contract expiry timestamp (null = no expiration)

	// Financial terms
	MinimumCommitAmount *decimal.Decimal `json:"minimum_commit_amount,omitempty"`
	Currency            string           `json:"currency"`

	// Account management (both nullable VARCHAR)
	AccountOwner  *string `json:"account_owner,omitempty"`
	SalesRepEmail *string `json:"sales_rep_email,omitempty"`

	// Status
	Status ContractStatus `json:"status"`

	// Custom pricing overrides (NULL = use plan default)
	CustomFreeSpans         *int64           `json:"custom_free_spans,omitempty"`
	CustomPricePer100KSpans *decimal.Decimal `json:"custom_price_per_100k_spans,omitempty"`
	CustomFreeGB            *decimal.Decimal `json:"custom_free_gb,omitempty"`
	CustomPricePerGB        *decimal.Decimal `json:"custom_price_per_gb,omitempty"`
	CustomFreeScores        *int64           `json:"custom_free_scores,omitempty"`
	CustomPricePer1KScores  *decimal.Decimal `json:"custom_price_per_1k_scores,omitempty"`

	// Audit
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Notes     *string   `json:"notes,omitempty"` // nullable TEXT

	// Relations
	VolumeTiers []VolumeDiscountTier `json:"volume_tiers,omitempty"`
}

type TierDimension string

const (
	TierDimensionSpans  TierDimension = "spans"
	TierDimensionBytes  TierDimension = "bytes"
	TierDimensionScores TierDimension = "scores"
)

type VolumeDiscountTier struct {
	ID           uuid.UUID       `json:"id"`
	ContractID   uuid.UUID       `json:"contract_id"`
	Dimension    TierDimension   `json:"dimension"`
	TierMin      int64           `json:"tier_min"`
	TierMax      *int64          `json:"tier_max,omitempty"` // NULL = unlimited
	PricePerUnit decimal.Decimal `json:"price_per_unit"`
	Priority     int             `json:"priority"`
	CreatedAt    time.Time       `json:"created_at"`
}

type ContractAction string

const (
	ContractActionCreated        ContractAction = "created"
	ContractActionUpdated        ContractAction = "updated"
	ContractActionCancelled      ContractAction = "cancelled"
	ContractActionExpired        ContractAction = "expired"
	ContractActionPricingChanged ContractAction = "pricing_changed"
)

type ContractHistory struct {
	ID             uuid.UUID       `json:"id"`
	ContractID     uuid.UUID       `json:"contract_id"`
	Action         ContractAction  `json:"action"`
	ChangedBy      string          `json:"changed_by,omitempty"`
	ChangedByEmail *string         `json:"changed_by_email,omitempty"` // nullable: sales team may leave empty
	ChangedAt      time.Time       `json:"changed_at"`
	Changes        json.RawMessage `json:"changes" swaggertype:"object"`
	Reason         *string         `json:"reason,omitempty"` // nullable TEXT
}


// EffectivePricing represents resolved pricing (contract overrides plan)
type EffectivePricing struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	BasePlan       *Plan     `json:"base_plan"`
	Contract       *Contract `json:"contract,omitempty"`

	// Resolved pricing (after contract overrides)
	FreeSpans         int64           `json:"free_spans"`
	PricePer100KSpans decimal.Decimal `json:"price_per_100k_spans"`
	FreeGB            decimal.Decimal `json:"free_gb"`
	PricePerGB        decimal.Decimal `json:"price_per_gb"`
	FreeScores        int64           `json:"free_scores"`
	PricePer1KScores  decimal.Decimal `json:"price_per_1k_scores"`

	HasVolumeTiers bool                  `json:"has_volume_tiers"`
	VolumeTiers    []*VolumeDiscountTier `json:"volume_tiers,omitempty"`
}
