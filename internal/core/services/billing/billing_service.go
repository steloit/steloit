package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/pkg/uid"
)

// BillingService implements billing operations for gateway usage
type BillingService struct {
	logger             *slog.Logger
	usageRepo          billingDomain.UsageRepository
	billingRecordRepo  billingDomain.BillingRecordRepository
	quotaRepo          billingDomain.QuotaRepository
	orgService         billingDomain.OrganizationService
	usageTracker       *UsageTracker
	discountCalculator *DiscountCalculator
	invoiceGenerator   *InvoiceGenerator
}

// BillingConfig holds billing service configuration
type BillingConfig struct {
	DefaultCurrency    string
	BillingPeriod      string // monthly, quarterly, annually
	PaymentGracePeriod time.Duration
	OverageChargeRate  float64
	EnableAutoBilling  bool
	InvoiceGeneration  bool
}

// DefaultBillingConfig returns default billing configuration
func DefaultBillingConfig() *BillingConfig {
	return &BillingConfig{
		DefaultCurrency:    "USD",
		BillingPeriod:      "monthly",
		PaymentGracePeriod: 7 * 24 * time.Hour, // 7 days
		OverageChargeRate:  1.25,               // 25% markup for overage
		EnableAutoBilling:  true,
		InvoiceGeneration:  true,
	}
}

// NewBillingService creates a new billing service instance
func NewBillingService(
	logger *slog.Logger,
	config *BillingConfig,
	usageRepo billingDomain.UsageRepository,
	billingRecordRepo billingDomain.BillingRecordRepository,
	quotaRepo billingDomain.QuotaRepository,
	orgService billingDomain.OrganizationService,
) *BillingService {
	if config == nil {
		config = DefaultBillingConfig()
	}

	return &BillingService{
		logger:             logger,
		usageRepo:          usageRepo,
		billingRecordRepo:  billingRecordRepo,
		quotaRepo:          quotaRepo,
		orgService:         orgService,
		usageTracker:       NewUsageTracker(logger, usageRepo, quotaRepo),
		discountCalculator: NewDiscountCalculator(logger),
		invoiceGenerator:   NewInvoiceGenerator(logger, config),
	}
}

// RecordUsage records usage for billing
func (s *BillingService) RecordUsage(ctx context.Context, usage *billingDomain.CostMetric) error {
	// Get organization billing tier
	billingTier, err := s.orgService.GetBillingTier(ctx, usage.OrganizationID)
	if err != nil {
		s.logger.Error("Failed to get billing tier", "error", err, "org_id", usage.OrganizationID)
		billingTier = "free" // Default fallback
	}

	// Calculate discounts
	discountRate, err := s.orgService.GetDiscountRate(ctx, usage.OrganizationID)
	if err != nil {
		s.logger.Error("Failed to get discount rate", "error", err, "org_id", usage.OrganizationID)
		discountRate = decimal.Zero // No discount on error
	}

	discountAmount := usage.TotalCost.Mul(discountRate)
	netCost := usage.TotalCost.Sub(discountAmount)

	// Create usage record
	record := &billingDomain.UsageRecord{
		ID:             uid.New(),
		OrganizationID: usage.OrganizationID,
		RequestID:      usage.RequestID,
		ProviderID:     usage.ProviderID,
		ModelID:        usage.ModelID,
		RequestType:    string(usage.RequestType),
		InputTokens:    usage.InputTokens,
		OutputTokens:   usage.OutputTokens,
		TotalTokens:    usage.TotalTokens,
		Cost:           usage.TotalCost,
		Currency:       usage.Currency,
		BillingTier:    billingTier,
		Discounts:      discountAmount,
		NetCost:        netCost,
		CreatedAt:      time.Now(),
	}

	// Store usage record
	if err := s.usageRepo.InsertUsageRecord(ctx, record); err != nil {
		s.logger.Error("Failed to insert usage record", "error", err, "record_id", record.ID)
		return fmt.Errorf("failed to record usage: %w", err)
	}

	// Update usage tracking
	if err := s.usageTracker.UpdateUsage(ctx, usage.OrganizationID, record); err != nil {
		s.logger.Error("Failed to update usage tracking", "error", err, "org_id", usage.OrganizationID)
		// Don't fail the entire operation for tracking errors
	}

	s.logger.Debug("Recorded usage for billing", "org_id", usage.OrganizationID, "request_id", usage.RequestID, "cost", usage.TotalCost, "net_cost", netCost)

	return nil
}

// CalculateBill generates a billing summary for an organization
func (s *BillingService) CalculateBill(ctx context.Context, orgID uuid.UUID, period string) (*billingDomain.BillingSummary, error) {
	// Calculate period start and end dates
	start, end := s.calculatePeriodBounds(period)

	// Get usage records for the period
	usageRecords, err := s.usageRepo.GetUsageRecords(ctx, orgID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage records: %w", err)
	}

	if len(usageRecords) == 0 {
		return &billingDomain.BillingSummary{
			ID:             uid.New(),
			OrganizationID: orgID,
			Period:         period,
			PeriodStart:    start,
			PeriodEnd:      end,
			TotalRequests:  0,
			TotalTokens:    0,
			TotalCost:      decimal.Zero,
			Currency:       "USD",
			NetCost:        decimal.Zero,
			Status:         "no_usage",
			GeneratedAt:    time.Now(),
		}, nil
	}

	// Calculate summary statistics
	summary := &billingDomain.BillingSummary{
		ID:                uid.New(),
		OrganizationID:    orgID,
		Period:            period,
		PeriodStart:       start,
		PeriodEnd:         end,
		Currency:          usageRecords[0].Currency,
		ProviderBreakdown: make(map[string]any),
		ModelBreakdown:    make(map[string]any),
		GeneratedAt:       time.Now(),
	}

	var totalRequests int64
	var totalTokens int64
	totalCost := decimal.Zero
	totalDiscounts := decimal.Zero
	totalNetCost := decimal.Zero

	for _, record := range usageRecords {
		totalRequests++
		totalTokens += int64(record.TotalTokens)
		totalCost = totalCost.Add(record.Cost)
		totalDiscounts = totalDiscounts.Add(record.Discounts)
		totalNetCost = totalNetCost.Add(record.NetCost)

		// Provider breakdown
		providerKey := record.ProviderID.String()
		if val, ok := summary.ProviderBreakdown[providerKey].(decimal.Decimal); ok {
			summary.ProviderBreakdown[providerKey] = val.Add(record.NetCost)
		} else {
			summary.ProviderBreakdown[providerKey] = record.NetCost
		}

		// Model breakdown
		modelKey := record.ModelID.String()
		if val, ok := summary.ModelBreakdown[modelKey].(decimal.Decimal); ok {
			summary.ModelBreakdown[modelKey] = val.Add(record.NetCost)
		} else {
			summary.ModelBreakdown[modelKey] = record.NetCost
		}
	}

	summary.TotalRequests = int(totalRequests)
	summary.TotalTokens = int(totalTokens)
	summary.TotalCost = totalCost
	summary.Discounts = totalDiscounts
	summary.NetCost = totalNetCost

	// Determine billing status
	if totalNetCost.GreaterThan(decimal.Zero) {
		summary.Status = "pending"
	} else {
		summary.Status = "no_charge"
	}

	// Store the billing summary
	if err := s.billingRecordRepo.InsertBillingSummary(ctx, summary); err != nil {
		s.logger.Error("Failed to store billing summary", "error", err, "org_id", orgID)
		// Continue without failing - the summary is still valid
	}

	return summary, nil
}

// GetBillingHistory retrieves billing history for an organization
func (s *BillingService) GetBillingHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.BillingRecord, error) {
	return s.billingRecordRepo.GetBillingHistory(ctx, orgID, start, end)
}

// ProcessPayment processes a payment for a billing record
func (s *BillingService) ProcessPayment(ctx context.Context, billingRecordID uuid.UUID) error {
	// Get billing record
	record, err := s.billingRecordRepo.GetBillingRecord(ctx, billingRecordID)
	if err != nil {
		return fmt.Errorf("failed to get billing record: %w", err)
	}

	if record.Status == "paid" {
		return fmt.Errorf("billing record %s is already paid", billingRecordID)
	}

	// Get payment method
	paymentMethod, err := s.orgService.GetPaymentMethod(ctx, record.OrganizationID)
	if err != nil {
		return fmt.Errorf("failed to get payment method: %w", err)
	}

	if paymentMethod == nil {
		return fmt.Errorf("no payment method found for organization %s", record.OrganizationID)
	}

	// TODO: Integrate with payment processor (Stripe, etc.)
	// This is a placeholder for actual payment processing
	transactionID := fmt.Sprintf("txn_%s", uid.New())

	// Update billing record with payment information
	now := time.Now()
	record.Status = "paid"
	record.TransactionID = &transactionID
	record.PaymentMethod = &paymentMethod.Type
	record.ProcessedAt = &now

	if err := s.billingRecordRepo.UpdateBillingRecord(ctx, billingRecordID, record); err != nil {
		return fmt.Errorf("failed to update billing record: %w", err)
	}

	s.logger.Info("Payment processed successfully", "billing_record_id", billingRecordID, "organization_id", record.OrganizationID, "amount", record.Amount, "transaction_id", transactionID)

	return nil
}

// CheckUsageQuotas checks if organization is within usage quotas
func (s *BillingService) CheckUsageQuotas(ctx context.Context, orgID uuid.UUID) (*billingDomain.QuotaStatus, error) {
	quota, err := s.quotaRepo.GetUsageQuota(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage quota: %w", err)
	}

	if quota == nil {
		// No quota set, assume unlimited for now
		return &billingDomain.QuotaStatus{
			OrganizationID: orgID,
			RequestsOK:     true,
			TokensOK:       true,
			CostOK:         true,
			Status:         "unlimited",
		}, nil
	}

	status := &billingDomain.QuotaStatus{
		OrganizationID: orgID,
		RequestsOK:     quota.MonthlyRequestLimit == 0 || quota.CurrentRequests < quota.MonthlyRequestLimit,
		TokensOK:       quota.MonthlyTokenLimit == 0 || quota.CurrentTokens < quota.MonthlyTokenLimit,
		CostOK:         quota.MonthlyCostLimit.IsZero() || quota.CurrentCost.LessThan(quota.MonthlyCostLimit),
	}

	if status.RequestsOK && status.TokensOK && status.CostOK {
		status.Status = "within_limits"
	} else if quota.CurrentRequests >= quota.MonthlyRequestLimit {
		status.Status = "requests_exceeded"
	} else if quota.CurrentTokens >= quota.MonthlyTokenLimit {
		status.Status = "tokens_exceeded"
	} else if quota.CurrentCost.GreaterThanOrEqual(quota.MonthlyCostLimit) {
		status.Status = "cost_exceeded"
	}

	// Calculate usage percentages
	if quota.MonthlyRequestLimit > 0 {
		status.RequestsUsagePercent = float64(quota.CurrentRequests) / float64(quota.MonthlyRequestLimit) * 100
	}
	if quota.MonthlyTokenLimit > 0 {
		status.TokensUsagePercent = float64(quota.CurrentTokens) / float64(quota.MonthlyTokenLimit) * 100
	}
	if !quota.MonthlyCostLimit.IsZero() {
		status.CostUsagePercent = quota.CurrentCost.Div(quota.MonthlyCostLimit).Mul(decimal.NewFromInt(100)).InexactFloat64()
	}

	return status, nil
}

// QuotaStatus represents the current quota status for an organization

// CreateBillingRecord creates a new billing record for an organization
func (s *BillingService) CreateBillingRecord(ctx context.Context, summary *billingDomain.BillingSummary) (*billingDomain.BillingRecord, error) {
	if summary.NetCost.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("no charges to bill for organization %s", summary.OrganizationID)
	}

	record := &billingDomain.BillingRecord{
		ID:             uid.New(),
		OrganizationID: summary.OrganizationID,
		Period:         summary.Period,
		Amount:         summary.NetCost,
		Currency:       summary.Currency,
		Status:         "pending",
		CreatedAt:      time.Now(),
	}

	if err := s.billingRecordRepo.InsertBillingRecord(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create billing record: %w", err)
	}

	s.logger.Info("Created billing record", "billing_record_id", record.ID, "organization_id", record.OrganizationID, "amount", record.Amount, "period", record.Period)

	return record, nil
}

// Helper methods

func (s *BillingService) calculatePeriodBounds(period string) (start, end time.Time) {
	now := time.Now()

	switch period {
	case "daily":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.Add(24 * time.Hour)
	case "weekly":
		// Start of week (Sunday)
		weekday := int(now.Weekday())
		start = now.AddDate(0, 0, -weekday)
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
		end = start.Add(7 * 24 * time.Hour)
	case "monthly":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0)
	case "yearly":
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(1, 0, 0)
	default:
		// Default to current month
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0)
	}

	return start, end
}

// Health check
func (s *BillingService) GetHealth() map[string]any {
	return map[string]any{
		"service":           "billing",
		"status":            "healthy",
		"usage_tracker":     s.usageTracker.GetHealth(),
		"invoice_generator": s.invoiceGenerator.GetHealth(),
	}
}
