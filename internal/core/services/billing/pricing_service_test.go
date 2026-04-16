package billing

import (
	"context"
	"testing"

	"brokle/internal/core/domain/billing"
	"brokle/pkg/pointers"
	"brokle/pkg/uid"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests use shared mocks from mocks_test.go

func TestPricingService_GetEffectivePricing_NoContract(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	// Setup mock data
	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "pro",
		FreeSpans:         1000000,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		FreeScores:        100,
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(nil, nil)

	effectivePricing, err := service.GetEffectivePricing(ctx, orgID)

	require.NoError(t, err)
	require.NotNil(t, effectivePricing)
	assert.Equal(t, orgID, effectivePricing.OrganizationID)
	assert.Equal(t, plan, effectivePricing.BasePlan)
	assert.Nil(t, effectivePricing.Contract)
	assert.Equal(t, int64(1000000), effectivePricing.FreeSpans)
	assert.True(t, effectivePricing.PricePer100KSpans.Equal(decimal.NewFromFloat(0.50)))
	assert.True(t, effectivePricing.FreeGB.Equal(decimal.NewFromFloat(10.0)))
	assert.True(t, effectivePricing.PricePerGB.Equal(decimal.NewFromFloat(2.00)))
	assert.Equal(t, int64(100), effectivePricing.FreeScores)
	assert.True(t, effectivePricing.PricePer1KScores.Equal(decimal.NewFromFloat(0.10)))
	assert.False(t, effectivePricing.HasVolumeTiers)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
}

func TestPricingService_GetEffectivePricing_WithContractOverrides(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "enterprise",
		FreeSpans:         1000000,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		FreeScores:        100,
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	contract := &billing.Contract{
		ID:                      contractID,
		OrganizationID:          orgID,
		Status:                  billing.ContractStatusActive,
		CustomFreeSpans:         ptrInt64(50000000),                               // Override
		CustomPricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.25)),  // Override
		CustomFreeGB:            pointers.PtrDecimal(decimal.NewFromFloat(100.0)), // Override
		CustomPricePerGB:        nil,                                              // Use plan default
		CustomFreeScores:        ptrInt64(1000),                                   // Override
		CustomPricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.05)),  // Override
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return([]*billing.VolumeDiscountTier{}, nil)

	effectivePricing, err := service.GetEffectivePricing(ctx, orgID)

	require.NoError(t, err)
	require.NotNil(t, effectivePricing)
	assert.Equal(t, contract, effectivePricing.Contract)
	// Check overrides are applied
	assert.Equal(t, int64(50000000), effectivePricing.FreeSpans)
	assert.True(t, effectivePricing.PricePer100KSpans.Equal(decimal.NewFromFloat(0.25)))
	assert.True(t, effectivePricing.FreeGB.Equal(decimal.NewFromFloat(100.0)))
	assert.True(t, effectivePricing.PricePerGB.Equal(decimal.NewFromFloat(2.00))) // Plan default
	assert.Equal(t, int64(1000), effectivePricing.FreeScores)
	assert.True(t, effectivePricing.PricePer1KScores.Equal(decimal.NewFromFloat(0.05)))
	assert.False(t, effectivePricing.HasVolumeTiers)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

func TestPricingService_GetEffectivePricing_WithVolumeTiers(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "enterprise",
		FreeSpans:         1000000,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100000000),
			PricePerUnit: decimal.NewFromFloat(0.30),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100000000,
			TierMax:      nil, // unlimited
			PricePerUnit: decimal.NewFromFloat(0.25),
			Priority:     1,
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	effectivePricing, err := service.GetEffectivePricing(ctx, orgID)

	require.NoError(t, err)
	require.NotNil(t, effectivePricing)
	assert.True(t, effectivePricing.HasVolumeTiers)
	assert.Len(t, effectivePricing.VolumeTiers, 2)
	assert.True(t, effectivePricing.VolumeTiers[0].PricePerUnit.Equal(decimal.NewFromFloat(0.30)))
	assert.True(t, effectivePricing.VolumeTiers[1].PricePerUnit.Equal(decimal.NewFromFloat(0.25)))

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

func TestPricingService_CalculateCostWithTiers_FlatPricing(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "pro",
		FreeSpans:         1000000,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		FreeScores:        100,
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(nil, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  5000000,                // 5M spans
		TotalBytes:  int64(50 * 1073741824), // 50 GB
		TotalScores: 500,                    // 500 scores
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Calculate expected cost:
	// Spans: (5M - 1M) / 100K * 0.50 = 4M / 100K * 0.50 = 40 * 0.50 = $20
	// Bytes: (50GB - 10GB) * 2.00 = 40 * 2.00 = $80
	// Scores: (500 - 100) / 1K * 0.10 = 400 / 1K * 0.10 = 0.4 * 0.10 = $0.04
	// Total: $100.04
	expectedCost := decimal.NewFromFloat(20.0 + 80.0 + 0.04)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
}

func TestPricingService_CalculateCostWithTiers_ProgressiveTiers(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "enterprise",
		FreeSpans:         50000000, // 50M free
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	// Progressive tiers:
	// 0-100M: $0.30 per 100K
	// 100M+: $0.25 per 100K
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100000000),
			PricePerUnit: decimal.NewFromFloat(0.30),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100000000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
			Priority:     1,
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	// Usage: 600M spans total
	usage := &billing.BillableUsageSummary{
		TotalSpans:  600000000,
		TotalBytes:  0,
		TotalScores: 0,
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// NOTE: This test passes with both buggy and fixed implementations because
	// the free tier (50M) is much smaller than the first tier span (0-100M).
	// The billable range [50M, 600M) happens to align correctly with tier boundaries
	// even when using incorrect relative coordinate logic.
	// See TestPricingService_CalculateCostWithTiers_FreeTierOffset for a test that
	// specifically exposes the free tier offset bug.

	// Calculate expected cost using ABSOLUTE COORDINATES with free tier offset:
	// Free tier: 50M
	// Usage: 600M spans
	// Billable absolute range: [50M, 600M)
	//
	// Tier 1 [0, 100M) @ $0.30/100K:
	//   Overlap: [max(50M, 0), min(600M, 100M)] = [50M, 100M)
	//   Usage in tier: 100M - 50M = 50M
	//   Cost: 50M / 100K * 0.30 = 500 * 0.30 = $150
	//
	// Tier 2 [100M, ∞) @ $0.25/100K:
	//   Overlap: [max(50M, 100M), 600M] = [100M, 600M)
	//   Usage in tier: 600M - 100M = 500M
	//   Cost: 500M / 100K * 0.25 = 5000 * 0.25 = $1,250
	//
	// Total: $150 + $1,250 = $1,400
	expectedCost := decimal.NewFromFloat(150.0 + 1250.0)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

func TestPricingService_CalculateCostWithTiers_WithinFreeTier(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "free",
		FreeSpans:         1000000,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		FreeScores:        100,
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(nil, nil)

	// Usage within free tier
	usage := &billing.BillableUsageSummary{
		TotalSpans:  500000,                // 500K spans (< 1M free)
		TotalBytes:  int64(5 * 1073741824), // 5 GB (< 10GB free)
		TotalScores: 50,                    // 50 scores (< 100 free)
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)
	assert.True(t, cost.IsZero(), "expected zero cost, got %s", cost) // All usage within free tier

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
}

func TestPricingService_CalculateCostWithTiers_MixedTiersAndFlat(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "enterprise",
		FreeSpans:         1000000, // 1M free
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		FreeScores:        100,
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	// Tiers ONLY for spans - bytes and scores should fallback to flat pricing
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100000000),
			PricePerUnit: decimal.NewFromFloat(0.30),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100000000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
			Priority:     1,
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	// Usage across all three dimensions
	usage := &billing.BillableUsageSummary{
		TotalSpans:  150000000,              // 150M spans
		TotalBytes:  int64(50 * 1073741824), // 50 GB
		TotalScores: 500,                    // 500 scores
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Calculate expected cost using ABSOLUTE COORDINATES with free tier offset:
	//
	// SPANS (tiered):
	//   Free tier: 1M
	//   Usage: 150M spans
	//   Billable absolute range: [1M, 150M)
	//
	//   Tier 1 [0, 100M) @ $0.30/100K:
	//     Overlap: [max(1M, 0), min(150M, 100M)] = [1M, 100M)
	//     Usage in tier: 100M - 1M = 99M
	//     Cost: 99M / 100K * 0.30 = 990 * 0.30 = $297
	//
	//   Tier 2 [100M, ∞) @ $0.25/100K:
	//     Overlap: [max(1M, 100M), 150M] = [100M, 150M)
	//     Usage in tier: 150M - 100M = 50M
	//     Cost: 50M / 100K * 0.25 = 500 * 0.25 = $125
	//
	//   Span cost: $297 + $125 = $422
	//
	// BYTES (flat fallback - no tiers defined):
	//   Free tier: 10 GB
	//   Billable: 50 GB - 10 GB = 40 GB
	//   Cost: 40 * 2.00 = $80
	//
	// SCORES (flat fallback - no tiers defined):
	//   Free tier: 100
	//   Billable: 500 - 100 = 400
	//   Cost: 400 / 1000 * 0.10 = 0.4 * 0.10 = $0.04
	//
	// Total: $422 + $80 + $0.04 = $502.04
	expectedCost := decimal.NewFromFloat(422.0 + 80.0 + 0.04)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

func TestPricingService_CalculateCostWithTiers_NoTiersFallbackToFlat(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "enterprise",
		FreeSpans:         1000000,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		FreeScores:        100,
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	// Empty tier list - contract exists but has no volume tiers
	// This is a valid state when enterprise customer hasn't negotiated tiers yet
	tiers := []*billing.VolumeDiscountTier{}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  5000000,                // 5M spans
		TotalBytes:  int64(50 * 1073741824), // 50 GB
		TotalScores: 500,                    // 500 scores
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Calculate expected cost (should use flat pricing since no tiers):
	// Spans: (5M - 1M) / 100K * 0.50 = 4M / 100K * 0.50 = 40 * 0.50 = $20
	// Bytes: (50GB - 10GB) * 2.00 = 40 * 2.00 = $80
	// Scores: (500 - 100) / 1K * 0.10 = 400 / 1K * 0.10 = 0.4 * 0.10 = $0.04
	// Total: $100.04
	expectedCost := decimal.NewFromFloat(20.0 + 80.0 + 0.04)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

// Test: Edge case - Usage exactly at tier boundary
func TestPricingService_CalculateCostWithTiers_UsageAtBoundary(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                planID,
		Name:              "Enterprise Plan",
		FreeSpans:         0, // No free tier for this test
		FreeGB:            decimal.Zero,
		FreeScores:        0,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.30)), // Default flat pricing
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	contract := &billing.Contract{
		ID:                      contractID,
		OrganizationID:          orgID,
		Status:                  billing.ContractStatusActive,
		CustomFreeSpans:         ptrInt64(0),
		CustomPricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.30)),
	}

	// Tiers: [0, 100M) @ $0.30
	// Usage exactly at 100M boundary should charge entire 100M at Tier 1 rate only
	tiers := []*billing.VolumeDiscountTier{
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(0.30),
		},
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  100_000_000, // Exactly at tier boundary
		TotalBytes:  0,
		TotalScores: 0,
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Calculate expected cost:
	// Spans: 100M spans at tier 1 rate
	// Tier 1: [0, 100M) - entire 100M at $0.30/100K = 1000 * 0.30 = $300
	// Tier 2: [100M, inf) - no usage (exactly at boundary, not past it)
	expectedCost := decimal.NewFromFloat(300.0)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

// Test: Edge case - Usage spans multiple tiers
func TestPricingService_CalculateCostWithTiers_MultiTierOverlap(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                planID,
		Name:              "Enterprise Plan",
		FreeSpans:         0,
		FreeGB:            decimal.Zero,
		FreeScores:        0,
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.30)),
		PricePerGB:        pointers.PtrDecimal(decimal.NewFromFloat(2.00)),
		PricePer1KScores:  pointers.PtrDecimal(decimal.NewFromFloat(0.10)),
	}

	contract := &billing.Contract{
		ID:                      contractID,
		OrganizationID:          orgID,
		Status:                  billing.ContractStatusActive,
		CustomFreeSpans:         ptrInt64(0),
		CustomPricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.30)),
	}

	// Three-tier pricing: verify correct progressive charging
	// Tier 1: [0, 100M) @ $0.30/100K
	// Tier 2: [100M, 200M) @ $0.25/100K
	// Tier 3: [200M, inf) @ $0.20/100K
	// Usage: 250M spans (spans all three tiers)
	tiers := []*billing.VolumeDiscountTier{
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(0.30),
		},
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      ptrInt64(200_000_000),
			PricePerUnit: decimal.NewFromFloat(0.25),
		},
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      200_000_000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.20),
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  250_000_000, // 250M spans
		TotalBytes:  0,
		TotalScores: 0,
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Calculate expected cost:
	// Tier 1: [0, 100M) - 100M spans @ $0.30/100K = 1000 * 0.30 = $300
	// Tier 2: [100M, 200M) - 100M spans @ $0.25/100K = 1000 * 0.25 = $250
	// Tier 3: [200M, inf) - 50M spans @ $0.20/100K = 500 * 0.20 = $100
	// Total: $650
	expectedCost := decimal.NewFromFloat(300.0 + 250.0 + 100.0)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

// TestPricingService_CalculateCostWithTiers_FreeTierOffset tests the PRIMARY bug scenario:
// free tier partially overlaps first tier, exposing coordinate system mismatch
func TestPricingService_CalculateCostWithTiers_FreeTierOffset(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "test",
		FreeSpans:         50_000_000, // 50M free spans
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(1.0)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	// Configuration:
	// - Free tier: 50M spans
	// - Tiers: [0-100M @ $1, 100M+ @ $0.50]
	// - Usage: 150M spans
	//
	// Expected calculation (ABSOLUTE coordinates):
	// - Billable range: [50M, 150M)
	// - Tier 1 overlap: [50M, 100M) = 50M spans @ $1/100K = 500 * $1 = $500
	// - Tier 2 overlap: [100M, 150M) = 50M spans @ $0.50/100K = 500 * $0.50 = $250
	// - Total: $750
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(1.0),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.50),
			Priority:     1,
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  150_000_000,
		TotalBytes:  0,
		TotalScores: 0,
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Tier 1: 50M / 100K @ $1 = 500 * $1 = $500
	// Tier 2: 50M / 100K @ $0.50 = 500 * $0.50 = $250
	// Total: $750
	expectedCost := decimal.NewFromFloat(500.0 + 250.0)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

// TestPricingService_CalculateCostWithTiers_FreeTierExceedsFirstTier tests scenario where
// free tier is larger than the first tier boundary
func TestPricingService_CalculateCostWithTiers_FreeTierExceedsFirstTier(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "test",
		FreeSpans:         150_000_000, // 150M free spans
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(1.0)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	// Configuration:
	// - Free tier: 150M spans
	// - Tiers: [0-100M @ $1, 100M-200M @ $0.75, 200M+ @ $0.50]
	// - Usage: 300M spans
	//
	// Expected calculation:
	// - Billable range: [150M, 300M)
	// - Tier 1 [0, 100M): No overlap (free tier starts at 150M)
	// - Tier 2 [100M, 200M): Overlap [150M, 200M) = 50M @ $0.75/100K = 500 * $0.75 = $375
	// - Tier 3 [200M, 300M): Overlap [200M, 300M) = 100M @ $0.50/100K = 1000 * $0.50 = $500
	// - Total: $875
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(1.0),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      ptrInt64(200_000_000),
			PricePerUnit: decimal.NewFromFloat(0.75),
			Priority:     1,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      200_000_000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.50),
			Priority:     2,
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  300_000_000,
		TotalBytes:  0,
		TotalScores: 0,
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Tier 2: 50M / 100K @ $0.75 = 500 * $0.75 = $375
	// Tier 3: 100M / 100K @ $0.50 = 1000 * $0.50 = $500
	// Total: $875
	expectedCost := decimal.NewFromFloat(375.0 + 500.0)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

// TestPricingService_CalculateCostWithTiers_FreeTierConsumesFirstTier tests scenario where
// free tier exactly consumes the first tier boundary
func TestPricingService_CalculateCostWithTiers_FreeTierConsumesFirstTier(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "test",
		FreeSpans:         100_000_000, // 100M free spans
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(1.0)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	// Configuration:
	// - Free tier: 100M spans
	// - Tiers: [0-100M @ $1, 100M+ @ $0.50]
	// - Usage: 150M spans
	//
	// Expected calculation:
	// - Billable range: [100M, 150M)
	// - Tier 1 [0, 100M): No overlap (free tier ends at 100M)
	// - Tier 2 [100M, 150M): Overlap [100M, 150M) = 50M @ $0.50/100K = 500 * $0.50 = $250
	// - Total: $250
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(1.0),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.50),
			Priority:     1,
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  150_000_000,
		TotalBytes:  0,
		TotalScores: 0,
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Only tier 2 charges: 50M / 100K @ $0.50 = 500 * $0.50 = $250
	expectedCost := decimal.NewFromFloat(250.0)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}

// TestPricingService_CalculateCostWithTiers_SmallFreeTier is a regression test
// verifying that small free tiers (relative to tier sizes) still work correctly
func TestPricingService_CalculateCostWithTiers_SmallFreeTier(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	orgID := uid.New()
	planID := uid.New()
	contractID := uid.New()

	billingRepo := new(MockOrganizationBillingRepository)
	planRepo := new(MockPlanRepository)
	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)

	service := NewPricingService(billingRepo, planRepo, contractRepo, tierRepo, logger)

	orgBilling := &billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         planID,
	}

	plan := &billing.Plan{
		ID:                uid.New(),
		Name:              "test",
		FreeSpans:         10_000_000, // 10M free (small relative to tiers)
		PricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.50)),
	}

	contract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	// Configuration:
	// - Free tier: 10M spans (small)
	// - Tiers: [0-100M @ $0.30, 100M+ @ $0.25]
	// - Usage: 150M spans
	//
	// Expected calculation:
	// - Billable range: [10M, 150M)
	// - Tier 1 [0, 100M): Overlap [10M, 100M) = 90M @ $0.30 = $270
	// - Tier 2 [100M, 150M): Overlap [100M, 150M) = 50M @ $0.25 = $125
	// - Total: $395
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(0.30),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
			Priority:     1,
		},
	}

	billingRepo.On("GetByOrgID", ctx, orgID).Return(orgBilling, nil)
	planRepo.On("GetByID", ctx, planID).Return(plan, nil)
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(contract, nil)
	tierRepo.On("GetByContractID", ctx, contractID).Return(tiers, nil)

	usage := &billing.BillableUsageSummary{
		TotalSpans:  150_000_000,
		TotalBytes:  0,
		TotalScores: 0,
	}

	cost, err := service.CalculateCostWithTiers(ctx, orgID, usage)

	require.NoError(t, err)

	// Tier 1: 90M / 100K @ $0.30 = 900 * 0.30 = $270
	// Tier 2: 50M / 100K @ $0.25 = 500 * 0.25 = $125
	// Total: $395
	expectedCost := decimal.NewFromFloat(270.0 + 125.0)
	assert.True(t, cost.Equal(expectedCost), "expected %s, got %s", expectedCost, cost)

	billingRepo.AssertExpectations(t)
	planRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
}
