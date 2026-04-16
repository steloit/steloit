package billing

import (
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"

	"github.com/google/uuid"
)

// Usage & Billing Entities

// Note: provider_id and model_id are now stored as text (no foreign keys to gateway tables)
// These values come from ClickHouse spans for cost calculation
type UsageRecord struct {
	CreatedAt      time.Time       `json:"created_at"`
	ProcessedAt    *time.Time      `json:"processed_at,omitempty"`
	RequestType    string          `json:"request_type"`
	BillingTier    string          `json:"billing_tier"`
	Currency       string          `json:"currency"`
	ProviderName   string          `json:"provider_name,omitempty"` // Human-readable provider name (e.g., "openai", "anthropic")
	ModelName      string          `json:"model_name,omitempty"`    // Human-readable model name (e.g., "gpt-4", "claude-3-opus")
	Cost           decimal.Decimal `json:"cost" gorm:"type:decimal(18,6)"`
	NetCost        decimal.Decimal `json:"net_cost" gorm:"type:decimal(18,6)"`
	Discounts      decimal.Decimal `json:"discounts" gorm:"type:decimal(18,6)"`
	TotalTokens    int32           `json:"total_tokens"`
	OutputTokens   int32           `json:"output_tokens"`
	InputTokens    int32           `json:"input_tokens"`
	ID             uuid.UUID       `json:"id"`
	ModelID        uuid.UUID       `json:"model_id"`    // Model ID from models table (for pricing lookup)
	ProviderID     uuid.UUID       `json:"provider_id"` // Provider identifier (text, not FK)
	RequestID      uuid.UUID       `json:"request_id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
}

// UsageQuota represents organization usage quotas and limits
type UsageQuota struct {
	ResetDate           time.Time       `json:"reset_date"`
	LastUpdated         time.Time       `json:"last_updated"`
	BillingTier         string          `json:"billing_tier"`
	Currency            string          `json:"currency"`
	MonthlyRequestLimit int64           `json:"monthly_request_limit"`
	MonthlyTokenLimit   int64           `json:"monthly_token_limit"`
	MonthlyCostLimit    decimal.Decimal `json:"monthly_cost_limit" gorm:"type:decimal(18,6)"`
	CurrentRequests     int64           `json:"current_requests"`
	CurrentTokens       int64           `json:"current_tokens"`
	CurrentCost         decimal.Decimal `json:"current_cost" gorm:"type:decimal(18,6)"`
	OrganizationID      uuid.UUID       `json:"organization_id"`
}

// Clone returns a deep copy of the UsageQuota
func (q *UsageQuota) Clone() *UsageQuota {
	if q == nil {
		return nil
	}
	return &UsageQuota{
		OrganizationID:      q.OrganizationID,
		BillingTier:         q.BillingTier,
		Currency:            q.Currency,
		MonthlyRequestLimit: q.MonthlyRequestLimit,
		MonthlyTokenLimit:   q.MonthlyTokenLimit,
		MonthlyCostLimit:    q.MonthlyCostLimit,
		CurrentRequests:     q.CurrentRequests,
		CurrentTokens:       q.CurrentTokens,
		CurrentCost:         q.CurrentCost,
		ResetDate:           q.ResetDate,
		LastUpdated:         q.LastUpdated,
	}
}

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
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Currency         string                 `json:"currency"`
	Period           string                 `json:"period"`
	InvoiceNumber    string                 `json:"invoice_number"`
	OrganizationName string                 `json:"organization_name"`
	Notes            string                 `json:"notes,omitempty"`
	PaymentTerms     string                 `json:"payment_terms"`
	Status           InvoiceStatus          `json:"status"`
	LineItems        []InvoiceLineItem      `json:"line_items"`
	TotalAmount      decimal.Decimal        `json:"total_amount" gorm:"type:decimal(18,6)"`
	DiscountAmount   decimal.Decimal        `json:"discount_amount" gorm:"type:decimal(18,6)"`
	TaxAmount        decimal.Decimal        `json:"tax_amount" gorm:"type:decimal(18,6)"`
	Subtotal         decimal.Decimal        `json:"subtotal" gorm:"type:decimal(18,6)"`
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
	Quantity     decimal.Decimal `json:"quantity" gorm:"type:decimal(18,6)"`
	UnitPrice    decimal.Decimal `json:"unit_price" gorm:"type:decimal(18,6)"`
	Amount       decimal.Decimal `json:"amount" gorm:"type:decimal(18,6)"`
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
	TaxRate     decimal.Decimal `json:"tax_rate" gorm:"type:decimal(18,6)"`
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
	MaximumDiscount decimal.Decimal    `json:"maximum_discount" gorm:"type:decimal(18,6)"`
	MinimumAmount   decimal.Decimal    `json:"minimum_amount" gorm:"type:decimal(18,6)"`
	Value           decimal.Decimal    `json:"value" gorm:"type:decimal(18,6)"`
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
	Cost     decimal.Decimal `json:"cost" gorm:"type:decimal(18,6)"`
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type VolumeDiscount struct {
	Tiers []VolumeTier `json:"tiers"`
}

type VolumeTier struct {
	MinAmount decimal.Decimal `json:"min_amount" gorm:"type:decimal(18,6)"`
	Discount  decimal.Decimal `json:"discount" gorm:"type:decimal(18,6)"` // percentage or fixed amount
}

// BillingRecord represents a billing record (moved from deleted analytics worker)
type BillingRecord struct {
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	Metadata       map[string]interface{} `json:"metadata" db:"metadata"`
	TransactionID  *string                `json:"transaction_id,omitempty" db:"transaction_id"`
	PaymentMethod  *string                `json:"payment_method,omitempty" db:"payment_method"`
	ProcessedAt    *time.Time             `json:"processed_at,omitempty" db:"processed_at"`
	Period         string                 `json:"period" db:"period"`
	Currency       string                 `json:"currency" db:"currency"`
	Status         string                 `json:"status" db:"status"`
	Amount         decimal.Decimal        `json:"amount" db:"amount" gorm:"type:decimal(18,6)"`
	NetCost        decimal.Decimal        `json:"net_cost" db:"net_cost" gorm:"type:decimal(18,6)"`
	ID             uuid.UUID              `json:"id" db:"id"`
	OrganizationID uuid.UUID              `json:"organization_id" db:"organization_id"`
}

// BillingSummary represents aggregated billing data (moved from deleted analytics worker)
type BillingSummary struct {
	PeriodStart       time.Time              `json:"period_start" db:"period_start"`
	PeriodEnd         time.Time              `json:"period_end" db:"period_end"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at"`
	GeneratedAt       time.Time              `json:"generated_at" db:"generated_at"`
	ModelBreakdown    map[string]interface{} `json:"model_breakdown"`
	ProviderBreakdown map[string]interface{} `json:"provider_breakdown"`
	Currency          string                 `json:"currency" db:"currency"`
	Period            string                 `json:"period" db:"period"`
	Status            string                 `json:"status" db:"status"`
	TotalAmount       decimal.Decimal        `json:"total_amount" db:"total_amount" gorm:"type:decimal(18,6)"`
	Discounts         decimal.Decimal        `json:"discounts" db:"discounts" gorm:"type:decimal(18,6)"`
	NetCost           decimal.Decimal        `json:"net_cost" db:"net_cost" gorm:"type:decimal(18,6)"`
	RecordCount       int                    `json:"record_count" db:"record_count"`
	TotalCost         decimal.Decimal        `json:"total_cost" db:"total_cost" gorm:"type:decimal(18,6)"`
	TotalTokens       int                    `json:"total_tokens" db:"total_tokens"`
	TotalRequests     int                    `json:"total_requests" db:"total_requests"`
	ID                uuid.UUID              `json:"id" db:"id"`
	OrganizationID    uuid.UUID              `json:"organization_id" db:"organization_id"`
}

// CostMetric represents cost tracking data (moved from deleted analytics worker)
type CostMetric struct {
	Timestamp      time.Time       `json:"timestamp"`
	Provider       string          `json:"provider"`
	Currency       string          `json:"currency"`
	RequestType    string          `json:"request_type"`
	Model          string          `json:"model"`
	TotalCost      decimal.Decimal `json:"total_cost" gorm:"type:decimal(18,6)"`
	OutputTokens   int32           `json:"output_tokens"`
	InputTokens    int32           `json:"input_tokens"`
	TotalTokens    int32           `json:"total_tokens"`
	ModelID        uuid.UUID       `json:"model_id"`
	RequestID      uuid.UUID       `json:"request_id"`
	ProviderID     uuid.UUID       `json:"provider_id"`
	ProjectID      uuid.UUID       `json:"project_id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
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
	AIProviderCost decimal.Decimal `json:"ai_provider_cost" gorm:"type:decimal(18,6)"`

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
	TotalCost decimal.Decimal `json:"total_cost" gorm:"type:decimal(18,6)"`

	// Informational
	TotalAIProviderCost decimal.Decimal `json:"total_ai_provider_cost" gorm:"type:decimal(18,6)"`
}

type Plan struct {
	ID        uuid.UUID `json:"id" gorm:"column:id;primaryKey"`
	Name      string    `json:"name" gorm:"column:name"` // free, pro, enterprise
	IsActive  bool      `json:"is_active" gorm:"column:is_active"`
	IsDefault bool      `json:"is_default" gorm:"column:is_default"` // Default plan for new organizations
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time `json:"updated_at" gorm:"column:updated_at"`

	// Span pricing (per 100K)
	FreeSpans         int64            `json:"free_spans" gorm:"column:free_spans"`
	PricePer100KSpans *decimal.Decimal `json:"price_per_100k_spans,omitempty" gorm:"column:price_per_100k_spans;type:decimal(18,6)"` // nil = unlimited in free tier

	// Data volume pricing (per GB)
	FreeGB     decimal.Decimal  `json:"free_gb" gorm:"column:free_gb;type:decimal(18,6)"`
	PricePerGB *decimal.Decimal `json:"price_per_gb,omitempty" gorm:"column:price_per_gb;type:decimal(18,6)"` // nil = unlimited in free tier

	// Score pricing (per 1K)
	FreeScores       int64            `json:"free_scores" gorm:"column:free_scores"`
	PricePer1KScores *decimal.Decimal `json:"price_per_1k_scores,omitempty" gorm:"column:price_per_1k_scores;type:decimal(18,6)"` // nil = unlimited in free tier
}

func (Plan) TableName() string {
	return "plans"
}

type OrganizationBilling struct {
	OrganizationID        uuid.UUID `json:"organization_id" db:"organization_id" gorm:"type:uuid;primaryKey"`
	PlanID                uuid.UUID `json:"plan_id" db:"plan_id"`
	BillingCycleStart     time.Time `json:"billing_cycle_start" db:"billing_cycle_start"`
	BillingCycleAnchorDay int       `json:"billing_cycle_anchor_day" db:"billing_cycle_anchor_day"` // Day of month (1-28)

	// Current period usage (three dimensions)
	CurrentPeriodSpans  int64 `json:"current_period_spans" db:"current_period_spans"`
	CurrentPeriodBytes  int64 `json:"current_period_bytes" db:"current_period_bytes"`
	CurrentPeriodScores int64 `json:"current_period_scores" db:"current_period_scores"`

	// Calculated cost this period
	CurrentPeriodCost decimal.Decimal `json:"current_period_cost" db:"current_period_cost" gorm:"type:decimal(18,6)"`

	// Free tier remaining (three dimensions)
	FreeSpansRemaining  int64 `json:"free_spans_remaining" db:"free_spans_remaining"`
	FreeBytesRemaining  int64 `json:"free_bytes_remaining" db:"free_bytes_remaining"`
	FreeScoresRemaining int64 `json:"free_scores_remaining" db:"free_scores_remaining"`

	LastSyncedAt time.Time `json:"last_synced_at" db:"last_synced_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type BudgetType string

const (
	BudgetTypeMonthly BudgetType = "monthly"
	BudgetTypeWeekly  BudgetType = "weekly"
)

type UsageBudget struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	OrganizationID uuid.UUID  `json:"organization_id" db:"organization_id"`
	ProjectID      *uuid.UUID `json:"project_id,omitempty" db:"project_id"` // nil for org-level budget
	Name           string     `json:"name" db:"name"`
	BudgetType     BudgetType `json:"budget_type" db:"budget_type"`

	// Limits (any can be set, nil = no limit)
	SpanLimit  *int64           `json:"span_limit,omitempty" db:"span_limit"`
	BytesLimit *int64           `json:"bytes_limit,omitempty" db:"bytes_limit"`
	ScoreLimit *int64           `json:"score_limit,omitempty" db:"score_limit"`
	CostLimit  *decimal.Decimal `json:"cost_limit,omitempty" db:"cost_limit" gorm:"type:decimal(18,6)"`

	// Current usage
	CurrentSpans  int64           `json:"current_spans" db:"current_spans"`
	CurrentBytes  int64           `json:"current_bytes" db:"current_bytes"`
	CurrentScores int64           `json:"current_scores" db:"current_scores"`
	CurrentCost   decimal.Decimal `json:"current_cost" db:"current_cost" gorm:"type:decimal(18,6)"`

	// Alert thresholds (flexible array of percentages, e.g., [50, 80, 100])
	AlertThresholds pq.Int64Array `json:"alert_thresholds" gorm:"column:alert_thresholds;type:integer[];default:'{50,80,100}'" swaggertype:"array,integer"`

	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
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
	ID             uuid.UUID  `json:"id" db:"id"`
	BudgetID       *uuid.UUID `json:"budget_id,omitempty" db:"budget_id"`
	OrganizationID uuid.UUID  `json:"organization_id" db:"organization_id"`
	ProjectID      *uuid.UUID `json:"project_id,omitempty" db:"project_id"`

	AlertThreshold int64           `json:"alert_threshold" db:"alert_threshold"`
	Dimension      AlertDimension  `json:"dimension" db:"dimension"`
	Severity       AlertSeverity   `json:"severity" db:"severity"`
	ThresholdValue int64           `json:"threshold_value" db:"threshold_value"`
	ActualValue    int64           `json:"actual_value" db:"actual_value"`
	PercentUsed    decimal.Decimal `json:"percent_used" db:"percent_used" gorm:"type:decimal(18,6)"`

	Status           AlertStatus `json:"status" db:"status"`
	TriggeredAt      time.Time   `json:"triggered_at" db:"triggered_at"`
	AcknowledgedAt   *time.Time  `json:"acknowledged_at,omitempty" db:"acknowledged_at"`
	ResolvedAt       *time.Time  `json:"resolved_at,omitempty" db:"resolved_at"`
	NotificationSent bool        `json:"notification_sent" db:"notification_sent"`
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
	ID             uuid.UUID `json:"id" gorm:"column:id;primaryKey"`
	OrganizationID uuid.UUID `json:"organization_id" gorm:"column:organization_id;type:uuid"`
	ContractName   string    `json:"contract_name" gorm:"column:contract_name"`
	ContractNumber string    `json:"contract_number" gorm:"column:contract_number;uniqueIndex"`

	// Timestamps (full precision, no normalization)
	// Access rule: contract is active when now < EndDate
	StartDate time.Time  `json:"start_date" gorm:"column:start_date"`       // Contract start timestamp (e.g., 2026-01-08T10:15:00Z)
	EndDate   *time.Time `json:"end_date,omitempty" gorm:"column:end_date"` // Contract expiry timestamp (null = no expiration)

	// Financial terms
	MinimumCommitAmount *decimal.Decimal `json:"minimum_commit_amount,omitempty" gorm:"column:minimum_commit_amount;type:decimal(18,6)"`
	Currency            string           `json:"currency" gorm:"column:currency;default:USD"`

	// Account management
	AccountOwner  string `json:"account_owner,omitempty" gorm:"column:account_owner"`
	SalesRepEmail string `json:"sales_rep_email,omitempty" gorm:"column:sales_rep_email"`

	// Status
	Status ContractStatus `json:"status" gorm:"column:status;default:active"`

	// Custom pricing overrides (NULL = use plan default)
	CustomFreeSpans         *int64           `json:"custom_free_spans,omitempty" gorm:"column:custom_free_spans"`
	CustomPricePer100KSpans *decimal.Decimal `json:"custom_price_per_100k_spans,omitempty" gorm:"column:custom_price_per_100k_spans;type:decimal(18,6)"`
	CustomFreeGB            *decimal.Decimal `json:"custom_free_gb,omitempty" gorm:"column:custom_free_gb;type:decimal(18,6)"`
	CustomPricePerGB        *decimal.Decimal `json:"custom_price_per_gb,omitempty" gorm:"column:custom_price_per_gb;type:decimal(18,6)"`
	CustomFreeScores        *int64           `json:"custom_free_scores,omitempty" gorm:"column:custom_free_scores"`
	CustomPricePer1KScores  *decimal.Decimal `json:"custom_price_per_1k_scores,omitempty" gorm:"column:custom_price_per_1k_scores;type:decimal(18,6)"`

	// Audit
	CreatedBy string    `json:"created_by,omitempty" gorm:"column:created_by"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time `json:"updated_at" gorm:"column:updated_at"`
	Notes     string    `json:"notes,omitempty" gorm:"column:notes;type:text"`

	// Relations
	VolumeTiers []VolumeDiscountTier `json:"volume_tiers,omitempty" gorm:"foreignKey:ContractID"`
}

func (Contract) TableName() string {
	return "contracts"
}

type TierDimension string

const (
	TierDimensionSpans  TierDimension = "spans"
	TierDimensionBytes  TierDimension = "bytes"
	TierDimensionScores TierDimension = "scores"
)

type VolumeDiscountTier struct {
	ID           uuid.UUID       `json:"id" gorm:"column:id;primaryKey"`
	ContractID   uuid.UUID       `json:"contract_id" gorm:"column:contract_id;type:uuid"`
	Dimension    TierDimension   `json:"dimension" gorm:"column:dimension"`
	TierMin      int64           `json:"tier_min" gorm:"column:tier_min;default:0"`
	TierMax      *int64          `json:"tier_max,omitempty" gorm:"column:tier_max"` // NULL = unlimited
	PricePerUnit decimal.Decimal `json:"price_per_unit" gorm:"column:price_per_unit;type:decimal(18,6)"`
	Priority     int             `json:"priority" gorm:"column:priority;default:0"`
	CreatedAt    time.Time       `json:"created_at" gorm:"column:created_at"`
}

func (VolumeDiscountTier) TableName() string {
	return "volume_discount_tiers"
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
	ID             uuid.UUID      `json:"id" gorm:"column:id;primaryKey"`
	ContractID     uuid.UUID      `json:"contract_id" gorm:"column:contract_id;type:uuid"`
	Action         ContractAction `json:"action" gorm:"column:action"`
	ChangedBy      string         `json:"changed_by,omitempty" gorm:"column:changed_by"`
	ChangedByEmail string         `json:"changed_by_email,omitempty" gorm:"column:changed_by_email"`
	ChangedAt      time.Time      `json:"changed_at" gorm:"column:changed_at"`
	Changes        datatypes.JSON `json:"changes" gorm:"column:changes;type:jsonb" swaggertype:"object"`
	Reason         string         `json:"reason,omitempty" gorm:"column:reason;type:text"`
}

func (ContractHistory) TableName() string {
	return "contract_history"
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
