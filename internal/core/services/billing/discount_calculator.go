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

// DiscountCalculator handles discount calculations for billing
type DiscountCalculator struct {
	logger *slog.Logger
}

// DiscountRule represents a discount rule

// DiscountCalculation represents the result of discount calculation
type DiscountCalculation struct {
	Currency         string            `json:"currency"`
	AppliedDiscounts []AppliedDiscount `json:"applied_discounts"`
	OriginalAmount   float64           `json:"original_amount"`
	TotalDiscount    float64           `json:"total_discount"`
	NetAmount        float64           `json:"net_amount"`
}

// AppliedDiscount represents a discount that was applied
type AppliedDiscount struct {
	RuleName    string                     `json:"rule_name"`
	Type        billingDomain.DiscountType `json:"type"`
	Description string                     `json:"description"`
	Value       float64                    `json:"value"`
	Amount      float64                    `json:"amount"`
	RuleID      uuid.UUID                  `json:"rule_id"`
}

// DiscountContext provides context for discount calculation
type DiscountContext struct {
	Timestamp       time.Time  `json:"timestamp"`
	UsageSummary    *UsageData `json:"usage_summary"`
	RequestType     *string    `json:"request_type,omitempty"`
	ProviderID      *uuid.UUID `json:"provider_id,omitempty"`
	ModelID         *uuid.UUID `json:"model_id,omitempty"`
	BillingTier     string     `json:"billing_tier"`
	OrganizationID  uuid.UUID  `json:"organization_id"`
	IsFirstCustomer bool       `json:"is_first_customer"`
}

// UsageData represents usage data for discount calculation
type UsageData struct {
	Currency      string  `json:"currency"`
	TotalRequests int64   `json:"total_requests"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCost     float64 `json:"total_cost"`
}

// NewDiscountCalculator creates a new discount calculator
func NewDiscountCalculator(logger *slog.Logger) *DiscountCalculator {
	return &DiscountCalculator{
		logger: logger,
	}
}

// CalculateDiscounts calculates applicable discounts for a billing amount
func (c *DiscountCalculator) CalculateDiscounts(
	ctx context.Context,
	amount float64,
	currency string,
	discountContext *DiscountContext,
	rules []*billingDomain.DiscountRule,
) (*DiscountCalculation, error) {

	calculation := &DiscountCalculation{
		OriginalAmount:   amount,
		TotalDiscount:    0,
		NetAmount:        amount,
		AppliedDiscounts: []AppliedDiscount{},
		Currency:         currency,
	}

	// Filter and sort applicable rules
	applicableRules := c.filterApplicableRules(rules, discountContext)
	if len(applicableRules) == 0 {
		return calculation, nil
	}

	c.logger.Debug("Calculating discounts", "org_id", discountContext.OrganizationID, "original_amount", amount, "applicable_rules", len(applicableRules))

	// Apply discounts in priority order
	currentAmount := amount

	for _, rule := range applicableRules {
		discount, err := c.calculateSingleDiscount(rule, currentAmount, discountContext)
		if err != nil {
			c.logger.Error("Failed to calculate discount", "error", err, "rule_id", rule.ID)
			continue
		}

		if discount.Amount > 0 {
			// Apply maximum discount limit if set
			maxDiscount := rule.MaximumDiscount.InexactFloat64()
			if maxDiscount > 0 && discount.Amount > maxDiscount {
				discount.Amount = maxDiscount
			}

			calculation.AppliedDiscounts = append(calculation.AppliedDiscounts, *discount)
			calculation.TotalDiscount += discount.Amount
			currentAmount -= discount.Amount

			// Ensure we don't go negative
			if currentAmount < 0 {
				calculation.TotalDiscount += currentAmount // Reduce total discount by the negative amount
				currentAmount = 0
				break
			}
		}
	}

	calculation.NetAmount = currentAmount

	c.logger.Debug("Discount calculation completed", "org_id", discountContext.OrganizationID, "original_amount", amount, "total_discount", calculation.TotalDiscount, "net_amount", calculation.NetAmount, "discounts_count", len(calculation.AppliedDiscounts))

	return calculation, nil
}

// GetOrganizationDiscountRate gets the default discount rate for an organization
func (c *DiscountCalculator) GetOrganizationDiscountRate(
	ctx context.Context,
	orgID uuid.UUID,
	billingTier string,
) (float64, error) {
	// Default discount rates by billing tier
	discountRates := map[string]float64{
		"free":       0.0,  // No discount for free tier
		"pro":        0.05, // 5% discount for pro tier
		"business":   0.10, // 10% discount for business tier
		"enterprise": 0.15, // 15% discount for enterprise tier
	}

	if rate, exists := discountRates[billingTier]; exists {
		return rate, nil
	}

	return 0.0, nil // Default to no discount
}

// CreateVolumeDiscountRule creates a volume-based discount rule
func (c *DiscountCalculator) CreateVolumeDiscountRule(
	orgID *uuid.UUID,
	name string,
	description string,
	tiers []billingDomain.VolumeTier,
	validFrom time.Time,
	validUntil *time.Time,
) *billingDomain.DiscountRule {
	return &billingDomain.DiscountRule{
		ID:              uid.New(),
		OrganizationID:  orgID,
		Name:            name,
		Description:     description,
		Type:            billingDomain.DiscountTypeTiered,
		Value:           decimal.Zero, // Value is determined by tiers
		MinimumAmount:   decimal.Zero,
		MaximumDiscount: decimal.Zero, // No maximum for volume discounts
		Conditions: &billingDomain.DiscountCondition{
			VolumeThreshold: &billingDomain.VolumeDiscount{
				Tiers: tiers,
			},
		},
		ValidFrom:  validFrom,
		ValidUntil: validUntil,
		IsActive:   true,
		Priority:   100, // High priority for volume discounts
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// CreateFirstTimeCustomerDiscount creates a first-time customer discount rule
func (c *DiscountCalculator) CreateFirstTimeCustomerDiscount(
	percentage float64,
	maxDiscount float64,
	validFor time.Duration,
) *billingDomain.DiscountRule {
	validUntil := time.Now().Add(validFor)

	return &billingDomain.DiscountRule{
		ID:              uid.New(),
		OrganizationID:  nil, // Global rule
		Name:            "First Time Customer Discount",
		Description:     fmt.Sprintf("%.0f%% discount for first-time customers", percentage*100),
		Type:            billingDomain.DiscountTypePercentage,
		Value:           decimal.NewFromFloat(percentage),
		MinimumAmount:   decimal.Zero,
		MaximumDiscount: decimal.NewFromFloat(maxDiscount),
		Conditions: &billingDomain.DiscountCondition{
			FirstTimeCustomer: true,
		},
		ValidFrom:  time.Now(),
		ValidUntil: &validUntil,
		IsActive:   true,
		Priority:   200, // High priority for first-time customers
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// Internal methods

func (c *DiscountCalculator) filterApplicableRules(
	rules []*billingDomain.DiscountRule,
	context *DiscountContext,
) []*billingDomain.DiscountRule {
	var applicable []*billingDomain.DiscountRule
	now := context.Timestamp

	for _, rule := range rules {
		if !c.isRuleApplicable(rule, context, now) {
			continue
		}
		applicable = append(applicable, rule)
	}

	// Sort by priority (highest first)
	for i := range len(applicable) - 1 {
		for j := i + 1; j < len(applicable); j++ {
			if applicable[i].Priority < applicable[j].Priority {
				applicable[i], applicable[j] = applicable[j], applicable[i]
			}
		}
	}

	return applicable
}

func (c *DiscountCalculator) isRuleApplicable(
	rule *billingDomain.DiscountRule,
	context *DiscountContext,
	now time.Time,
) bool {
	// Check if rule is active
	if !rule.IsActive {
		return false
	}

	// Check validity period
	if now.Before(rule.ValidFrom) {
		return false
	}
	if rule.ValidUntil != nil && now.After(*rule.ValidUntil) {
		return false
	}

	// Check usage limit
	if rule.UsageLimit != nil && rule.UsageCount >= *rule.UsageLimit {
		return false
	}

	// Check organization-specific rule
	if rule.OrganizationID != nil && *rule.OrganizationID != context.OrganizationID {
		return false
	}

	// Check conditions if they exist
	if rule.Conditions != nil {
		return c.checkDiscountConditions(rule.Conditions, context, now)
	}

	return true
}

func (c *DiscountCalculator) checkDiscountConditions(
	conditions *billingDomain.DiscountCondition,
	context *DiscountContext,
	now time.Time,
) bool {
	// Check billing tier
	if len(conditions.BillingTiers) > 0 {
		found := false
		for _, tier := range conditions.BillingTiers {
			if tier == context.BillingTier {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check minimum usage
	if conditions.MinUsage != nil && context.UsageSummary != nil {
		usage := context.UsageSummary
		minCost := conditions.MinUsage.Cost.InexactFloat64()
		if usage.TotalRequests < conditions.MinUsage.Requests ||
			usage.TotalTokens < conditions.MinUsage.Tokens ||
			usage.TotalCost < minCost {
			return false
		}
	}

	// Check request types
	if len(conditions.RequestTypes) > 0 && context.RequestType != nil {
		found := false
		for _, reqType := range conditions.RequestTypes {
			if reqType == *context.RequestType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check providers
	if len(conditions.Providers) > 0 && context.ProviderID != nil {
		found := false
		for _, providerID := range conditions.Providers {
			if providerID == *context.ProviderID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check models
	if len(conditions.Models) > 0 && context.ModelID != nil {
		found := false
		for _, modelID := range conditions.Models {
			if modelID == *context.ModelID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check first-time customer
	if conditions.FirstTimeCustomer && !context.IsFirstCustomer {
		return false
	}

	// Check time of day
	if conditions.TimeOfDay != nil {
		// Simple time range check (ignoring timezone complexities for now)
		currentTime := now.Format("15:04")
		startTime := conditions.TimeOfDay.Start.Format("15:04")
		endTime := conditions.TimeOfDay.End.Format("15:04")

		if currentTime < startTime || currentTime > endTime {
			return false
		}
	}

	// Check days of week
	if len(conditions.DaysOfWeek) > 0 {
		currentDay := now.Weekday()
		found := false
		for _, day := range conditions.DaysOfWeek {
			if day == currentDay {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (c *DiscountCalculator) calculateSingleDiscount(
	rule *billingDomain.DiscountRule,
	amount float64,
	context *DiscountContext,
) (*AppliedDiscount, error) {
	// Check minimum amount requirement
	minAmount := rule.MinimumAmount.InexactFloat64()
	if amount < minAmount {
		return &AppliedDiscount{Amount: 0}, nil
	}

	var discountAmount float64
	var description string
	ruleValue := rule.Value.InexactFloat64()

	switch rule.Type {
	case billingDomain.DiscountTypePercentage:
		discountAmount = amount * ruleValue
		description = fmt.Sprintf("%.1f%% discount", ruleValue*100)

	case billingDomain.DiscountTypeFixed:
		discountAmount = ruleValue
		description = fmt.Sprintf("$%.2f fixed discount", ruleValue)

	case billingDomain.DiscountTypeTiered:
		if rule.Conditions != nil && rule.Conditions.VolumeThreshold != nil {
			discountAmount = c.calculateVolumeDiscount(amount, rule.Conditions.VolumeThreshold)
			description = "Volume-based discount"
		}

	default:
		return nil, fmt.Errorf("unsupported discount type: %s", rule.Type)
	}

	// Ensure discount doesn't exceed the original amount
	if discountAmount > amount {
		discountAmount = amount
	}

	return &AppliedDiscount{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Type:        rule.Type,
		Value:       ruleValue,
		Amount:      discountAmount,
		Description: description,
	}, nil
}

func (c *DiscountCalculator) calculateVolumeDiscount(amount float64, volumeDiscount *billingDomain.VolumeDiscount) float64 {
	var totalDiscount float64
	remainingAmount := amount

	// Sort tiers by minimum amount (ascending)
	tiers := make([]billingDomain.VolumeTier, len(volumeDiscount.Tiers))
	copy(tiers, volumeDiscount.Tiers)

	for i := range len(tiers) - 1 {
		for j := i + 1; j < len(tiers); j++ {
			if tiers[i].MinAmount.GreaterThan(tiers[j].MinAmount) {
				tiers[i], tiers[j] = tiers[j], tiers[i]
			}
		}
	}

	// Apply tiered discounts
	for i, tier := range tiers {
		if remainingAmount <= 0 {
			break
		}

		tierAmount := remainingAmount
		if i < len(tiers)-1 {
			// Not the last tier, calculate amount in this tier
			nextTierMin := tiers[i+1].MinAmount.InexactFloat64()
			tierMin := tier.MinAmount.InexactFloat64()
			if amount > nextTierMin {
				tierAmount = nextTierMin - tierMin
				if tierAmount > remainingAmount {
					tierAmount = remainingAmount
				}
			}
		}

		if tierAmount > 0 {
			totalDiscount += tierAmount * tier.Discount.InexactFloat64()
			remainingAmount -= tierAmount
		}
	}

	return totalDiscount
}

// Health check
func (c *DiscountCalculator) GetHealth() map[string]any {
	return map[string]any{
		"service": "discount_calculator",
		"status":  "healthy",
	}
}
