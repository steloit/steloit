package billing

import (
	"context"
	"log/slog"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/pkg/pointers"
	"brokle/pkg/units"
)

type pricingService struct {
	billingRepo  billing.OrganizationBillingRepository
	planRepo     billing.PlanRepository
	contractRepo billing.ContractRepository
	tierRepo     billing.VolumeDiscountTierRepository
	logger       *slog.Logger
}

func NewPricingService(
	billingRepo billing.OrganizationBillingRepository,
	planRepo billing.PlanRepository,
	contractRepo billing.ContractRepository,
	tierRepo billing.VolumeDiscountTierRepository,
	logger *slog.Logger,
) billing.PricingService {
	return &pricingService{
		billingRepo:  billingRepo,
		planRepo:     planRepo,
		contractRepo: contractRepo,
		tierRepo:     tierRepo,
		logger:       logger,
	}
}

// GetEffectivePricing resolves pricing: contract overrides > plan defaults
func (s *pricingService) GetEffectivePricing(ctx context.Context, orgID uuid.UUID) (*billing.EffectivePricing, error) {
	// Get organization's billing state
	orgBilling, err := s.billingRepo.GetByOrgID(ctx, orgID)
	if err != nil {
		return nil, err
	}

	return s.GetEffectivePricingWithBilling(ctx, orgID, orgBilling)
}

// GetEffectivePricingWithBilling resolves pricing using pre-fetched orgBilling
// Use this when orgBilling is already available to avoid redundant DB query
func (s *pricingService) GetEffectivePricingWithBilling(ctx context.Context, orgID uuid.UUID, orgBilling *billing.OrganizationBilling) (*billing.EffectivePricing, error) {
	// 1. Get organization's base plan
	plan, err := s.planRepo.GetByID(ctx, orgBilling.PlanID)
	if err != nil {
		return nil, err
	}

	// 2. Check for active contract
	contract, err := s.contractRepo.GetActiveByOrgID(ctx, orgID)
	if err != nil {
		return nil, err // Real database error
	}
	// contract will be nil if no active contract exists (valid state)

	effective := &billing.EffectivePricing{
		OrganizationID: orgID,
		BasePlan:       plan,
		Contract:       contract,
	}

	// 3. Resolve pricing (contract overrides plan)
	if contract != nil {
		effective.FreeSpans = pointers.CoalesceInt64(contract.CustomFreeSpans, plan.FreeSpans)
		effective.PricePer100KSpans = pointers.CoalesceDecimal(contract.CustomPricePer100KSpans, plan.PricePer100KSpans)
		effective.FreeGB = pointers.CoalesceDecimal(contract.CustomFreeGB, &plan.FreeGB)
		effective.PricePerGB = pointers.CoalesceDecimal(contract.CustomPricePerGB, plan.PricePerGB)
		effective.FreeScores = pointers.CoalesceInt64(contract.CustomFreeScores, plan.FreeScores)
		effective.PricePer1KScores = pointers.CoalesceDecimal(contract.CustomPricePer1KScores, plan.PricePer1KScores)

		// Load volume tiers
		tiers, err := s.tierRepo.GetByContractID(ctx, contract.ID)
		if err != nil {
			return nil, err
		}

		if len(tiers) > 0 {
			effective.HasVolumeTiers = true
			effective.VolumeTiers = tiers
		}
	} else {
		// No contract, use plan defaults
		effective.FreeSpans = plan.FreeSpans
		effective.PricePer100KSpans = pointers.DerefDecimal(plan.PricePer100KSpans)
		effective.FreeGB = plan.FreeGB
		effective.PricePerGB = pointers.DerefDecimal(plan.PricePerGB)
		effective.FreeScores = plan.FreeScores
		effective.PricePer1KScores = pointers.DerefDecimal(plan.PricePer1KScores)
	}

	return effective, nil
}

// CalculateCostWithTiers calculates cost with volume tier support
func (s *pricingService) CalculateCostWithTiers(ctx context.Context, orgID uuid.UUID, usage *billing.BillableUsageSummary) (decimal.Decimal, error) {
	effective, err := s.GetEffectivePricing(ctx, orgID)
	if err != nil {
		return decimal.Zero, err
	}

	if effective.HasVolumeTiers {
		return s.calculateWithTiers(usage, effective), nil
	}

	return s.calculateFlat(usage, effective), nil
}

// CalculateCostWithTiersNoFreeTier calculates cost with tier support but without free tier deductions
// Used for project-level budgets where free tier is org-level only
func (s *pricingService) CalculateCostWithTiersNoFreeTier(ctx context.Context, orgID uuid.UUID, usage *billing.BillableUsageSummary) (decimal.Decimal, error) {
	effective, err := s.GetEffectivePricing(ctx, orgID)
	if err != nil {
		return decimal.Zero, err
	}

	if effective.HasVolumeTiers {
		return s.calculateWithTiersNoFreeTier(usage, effective), nil
	}

	return s.calculateFlatNoFreeTier(usage, effective), nil
}

// calculateFlat uses simple linear pricing (current implementation)
func (s *pricingService) calculateFlat(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	totalCost := decimal.Zero

	// Spans
	billableSpans := max(0, usage.TotalSpans-pricing.FreeSpans)
	spanCost := decimal.NewFromInt(billableSpans).Div(decimal.NewFromInt(units.SpansPer100K)).Mul(pricing.PricePer100KSpans)
	totalCost = totalCost.Add(spanCost)

	// Bytes
	freeBytes := pricing.FreeGB.Mul(decimal.NewFromInt(units.BytesPerGB)).IntPart()
	billableBytes := max(0, usage.TotalBytes-freeBytes)
	billableGB := decimal.NewFromInt(billableBytes).Div(decimal.NewFromInt(units.BytesPerGB))
	dataCost := billableGB.Mul(pricing.PricePerGB)
	totalCost = totalCost.Add(dataCost)

	// Scores
	billableScores := max(0, usage.TotalScores-pricing.FreeScores)
	scoreCost := decimal.NewFromInt(billableScores).Div(decimal.NewFromInt(units.ScoresPer1K)).Mul(pricing.PricePer1KScores)
	totalCost = totalCost.Add(scoreCost)

	return totalCost
}

// calculateWithTiers uses progressive tier pricing
func (s *pricingService) calculateWithTiers(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	totalCost := decimal.Zero

	// Calculate each dimension
	totalCost = totalCost.Add(s.CalculateDimensionWithTiers(usage.TotalSpans, pricing.FreeSpans, billing.TierDimensionSpans, pricing.VolumeTiers, pricing))

	freeBytes := pricing.FreeGB.Mul(decimal.NewFromInt(units.BytesPerGB)).IntPart()
	totalCost = totalCost.Add(s.CalculateDimensionWithTiers(usage.TotalBytes, freeBytes, billing.TierDimensionBytes, pricing.VolumeTiers, pricing))

	totalCost = totalCost.Add(s.CalculateDimensionWithTiers(usage.TotalScores, pricing.FreeScores, billing.TierDimensionScores, pricing.VolumeTiers, pricing))

	return totalCost
}

// calculateFlatNoFreeTier uses simple linear pricing without free tier deductions
func (s *pricingService) calculateFlatNoFreeTier(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	totalCost := decimal.Zero

	// Spans
	spanCost := decimal.NewFromInt(usage.TotalSpans).Div(decimal.NewFromInt(units.SpansPer100K)).Mul(pricing.PricePer100KSpans)
	totalCost = totalCost.Add(spanCost)

	// Bytes
	billableGB := decimal.NewFromInt(usage.TotalBytes).Div(decimal.NewFromInt(units.BytesPerGB))
	dataCost := billableGB.Mul(pricing.PricePerGB)
	totalCost = totalCost.Add(dataCost)

	// Scores
	scoreCost := decimal.NewFromInt(usage.TotalScores).Div(decimal.NewFromInt(units.ScoresPer1K)).Mul(pricing.PricePer1KScores)
	totalCost = totalCost.Add(scoreCost)

	return totalCost
}

// calculateWithTiersNoFreeTier uses progressive tier pricing without free tier deductions
func (s *pricingService) calculateWithTiersNoFreeTier(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	totalCost := decimal.Zero

	// Calculate each dimension without free tier
	totalCost = totalCost.Add(s.CalculateDimensionWithTiers(usage.TotalSpans, 0, billing.TierDimensionSpans, pricing.VolumeTiers, pricing))
	totalCost = totalCost.Add(s.CalculateDimensionWithTiers(usage.TotalBytes, 0, billing.TierDimensionBytes, pricing.VolumeTiers, pricing))
	totalCost = totalCost.Add(s.CalculateDimensionWithTiers(usage.TotalScores, 0, billing.TierDimensionScores, pricing.VolumeTiers, pricing))

	return totalCost
}

// CalculateDimensionWithTiers applies progressive pricing using absolute position mapping with free tier offset
// The algorithm works in absolute coordinate space: billable range is [freeTier, usage), not [0, billableUsage)
// This ensures free tier correctly offsets tier boundaries (e.g., free=500 with tier [0-1k] charges usage 500-1k in that tier)
// Exported for use by workers that need per-dimension cost calculation
func (s *pricingService) CalculateDimensionWithTiers(usage, freeTier int64, dimension billing.TierDimension, allTiers []*billing.VolumeDiscountTier, pricing *billing.EffectivePricing) decimal.Decimal {
	// Early exit: all usage covered by free tier
	if usage <= freeTier {
		return decimal.Zero
	}

	// Filter and sort tiers for this dimension
	var tiers []*billing.VolumeDiscountTier
	for _, t := range allTiers {
		if t.Dimension == dimension {
			tiers = append(tiers, t)
		}
	}

	if len(tiers) == 0 {
		// No tiers defined for this dimension, fallback to flat pricing on billable amount
		billableUsage := usage - freeTier
		return s.calculateFlatDimension(billableUsage, dimension, pricing)
	}

	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].TierMin < tiers[j].TierMin
	})

	totalCost := decimal.Zero

	for _, tier := range tiers {
		// Calculate overlap between billable range [freeTier, usage)
		// and tier range [tier.TierMin, tier.TierMax) in ABSOLUTE coordinates
		//
		// Example: free=500, tier=[0-1000], usage=1500
		//   overlapStart = max(500, 0) = 500
		//   overlapEnd = min(1500, 1000) = 1000
		//   usageInTier = 1000 - 500 = 500 (charges 500 units in this tier)

		overlapStart := max(freeTier, tier.TierMin)

		var overlapEnd int64
		if tier.TierMax == nil {
			overlapEnd = usage // Unlimited tier extends to total usage
		} else {
			overlapEnd = min(usage, *tier.TierMax)
		}

		// Skip if no overlap
		if overlapStart >= overlapEnd {
			continue
		}

		// Calculate billable usage in this tier
		usageInTier := overlapEnd - overlapStart

		// Convert to billable units and apply price
		unitSize := getDimensionUnitSize(dimension)
		unitsInTier := decimal.NewFromInt(usageInTier).Div(decimal.NewFromInt(unitSize))
		cost := unitsInTier.Mul(tier.PricePerUnit)

		totalCost = totalCost.Add(cost)

		// Optimization: stop if usage fully consumed
		if tier.TierMax == nil || usage <= *tier.TierMax {
			break
		}
	}

	return totalCost
}

// Helper functions

func getDimensionUnitSize(dimension billing.TierDimension) int64 {
	switch dimension {
	case billing.TierDimensionSpans:
		return units.SpansPer100K
	case billing.TierDimensionBytes:
		return units.BytesPerGB
	case billing.TierDimensionScores:
		return units.ScoresPer1K
	default:
		return 1
	}
}

// calculateFlatDimension calculates cost for a single dimension using flat pricing
// Used as fallback when no volume tiers are defined for a dimension
func (s *pricingService) calculateFlatDimension(billableUsage int64, dimension billing.TierDimension, pricing *billing.EffectivePricing) decimal.Decimal {
	if billableUsage == 0 {
		return decimal.Zero
	}

	unitSize := getDimensionUnitSize(dimension)
	unitsVal := decimal.NewFromInt(billableUsage).Div(decimal.NewFromInt(unitSize))

	switch dimension {
	case billing.TierDimensionSpans:
		return unitsVal.Mul(pricing.PricePer100KSpans)
	case billing.TierDimensionBytes:
		return unitsVal.Mul(pricing.PricePerGB)
	case billing.TierDimensionScores:
		return unitsVal.Mul(pricing.PricePer1KScores)
	default:
		return decimal.Zero
	}
}
