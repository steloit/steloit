package workers

import (
	"log/slog"
	"os"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"brokle/internal/core/domain/billing"
	billingService "brokle/internal/core/services/billing"
	"brokle/pkg/uid"
)

// createTestPricingService creates a real PricingService for testing
// This ensures worker tests use the same pricing logic as production
func createTestPricingService() billing.PricingService {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	// PricingService doesn't use repos for CalculateDimensionWithTiers, so we can pass nil
	return billingService.NewPricingService(nil, nil, nil, nil, logger)
}

func TestUsageAggregationWorker_CalculateCost_FlatPricing(t *testing.T) {
	worker := &UsageAggregationWorker{
		pricingService: createTestPricingService(),
	}

	pricing := &billing.EffectivePricing{
		FreeSpans:         1000000,
		PricePer100KSpans: decimal.NewFromFloat(0.50),
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        decimal.NewFromFloat(2.00),
		FreeScores:        100,
		PricePer1KScores:  decimal.NewFromFloat(0.10),
		HasVolumeTiers:    false,
	}

	usage := &billing.BillableUsageSummary{
		TotalSpans:  5000000,                // 5M spans
		TotalBytes:  int64(50 * 1073741824), // 50 GB
		TotalScores: 500,                    // 500 scores
	}

	cost := worker.calculateCost(usage, pricing)

	// Expected cost:
	// Spans: (5M - 1M) / 100K * 0.50 = 4M / 100K * 0.50 = 40 * 0.50 = $20
	// Bytes: (50GB - 10GB) * 2.00 = 40 * 2.00 = $80
	// Scores: (500 - 100) / 1K * 0.10 = 400 / 1K * 0.10 = 0.4 * 0.10 = $0.04
	// Total: $100.04
	expectedCost := 20.0 + 80.0 + 0.04
	assert.InDelta(t, expectedCost, cost.InexactFloat64(), 0.01)
}

func TestUsageAggregationWorker_CalculateRawCost_FlatPricing(t *testing.T) {
	worker := &UsageAggregationWorker{
		pricingService: createTestPricingService(),
	}

	pricing := &billing.EffectivePricing{
		FreeSpans:         1000000, // Should be ignored
		PricePer100KSpans: decimal.NewFromFloat(0.50),
		FreeGB:            decimal.NewFromFloat(10.0), // Should be ignored
		PricePerGB:        decimal.NewFromFloat(2.00),
		FreeScores:        100, // Should be ignored
		PricePer1KScores:  decimal.NewFromFloat(0.10),
		HasVolumeTiers:    false,
	}

	usage := &billing.BillableUsageSummary{
		TotalSpans:  5000000,                // 5M spans
		TotalBytes:  int64(50 * 1073741824), // 50 GB
		TotalScores: 500,                    // 500 scores
	}

	cost := worker.calculateRawCost(usage, pricing)

	// Expected cost (no free tier deduction):
	// Spans: 5M / 100K * 0.50 = 50 * 0.50 = $25
	// Bytes: 50GB * 2.00 = $100
	// Scores: 500 / 1K * 0.10 = 0.5 * 0.10 = $0.05
	// Total: $125.05
	expectedCost := 25.0 + 100.0 + 0.05
	assert.InDelta(t, expectedCost, cost.InexactFloat64(), 0.01)
}

func TestUsageAggregationWorker_CalculateCost_WithProgressiveTiers(t *testing.T) {
	worker := &UsageAggregationWorker{
		pricingService: createTestPricingService(),
	}

	// Progressive tiers for spans only
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100000000), // 0-100M
			PricePerUnit: decimal.NewFromFloat(0.30),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100000000, // 100M+
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
			Priority:     1,
		},
	}

	pricing := &billing.EffectivePricing{
		FreeSpans:         50000000,                   // 50M free
		PricePer100KSpans: decimal.NewFromFloat(0.50), // Fallback for non-tiered dimensions
		FreeGB:            decimal.NewFromFloat(10.0),
		PricePerGB:        decimal.NewFromFloat(2.00),
		FreeScores:        100,
		PricePer1KScores:  decimal.NewFromFloat(0.10),
		HasVolumeTiers:    true,
		VolumeTiers:       tiers,
	}

	usage := &billing.BillableUsageSummary{
		TotalSpans:  600000000,              // 600M spans
		TotalBytes:  int64(50 * 1073741824), // 50 GB
		TotalScores: 500,                    // 500 scores
	}

	cost := worker.calculateCost(usage, pricing)

	// Expected cost using ABSOLUTE COORDINATES with free tier offset:
	// SPANS (tiered):
	//   Free tier: 50M
	//   Usage: 600M spans
	//   Billable absolute range: [50M, 600M)
	//
	//   Tier 1 [0, 100M) @ $0.30/100K:
	//     Overlap: [max(50M, 0), min(600M, 100M)] = [50M, 100M)
	//     Usage in tier: 100M - 50M = 50M
	//     Cost: 50M / 100K * 0.30 = 500 * 0.30 = $150
	//
	//   Tier 2 [100M, ∞) @ $0.25/100K:
	//     Overlap: [max(50M, 100M), 600M] = [100M, 600M)
	//     Usage in tier: 600M - 100M = 500M
	//     Cost: 500M / 100K * 0.25 = 5000 * 0.25 = $1,250
	//
	//   Span cost: $150 + $1,250 = $1,400
	//
	// BYTES (flat fallback):
	//   Billable: 50GB - 10GB = 40GB
	//   Cost: 40 * 2.00 = $80
	//
	// SCORES (flat fallback):
	//   Billable: 500 - 100 = 400
	//   Cost: 400 / 1000 * 0.10 = $0.04
	//
	// Total: $1,480.04
	expectedCost := 1400.0 + 80.0 + 0.04
	assert.InDelta(t, expectedCost, cost.InexactFloat64(), 0.01)
}

func TestUsageAggregationWorker_CalculateRawCost_WithProgressiveTiers(t *testing.T) {
	worker := &UsageAggregationWorker{
		pricingService: createTestPricingService(),
	}

	// Progressive tiers for spans only
	tiers := []*billing.VolumeDiscountTier{
		{
			ID:           uid.New(),
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100000000),
			PricePerUnit: decimal.NewFromFloat(0.30),
			Priority:     0,
		},
		{
			ID:           uid.New(),
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100000000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
			Priority:     1,
		},
	}

	pricing := &billing.EffectivePricing{
		FreeSpans:         50000000, // Should be ignored
		PricePer100KSpans: decimal.NewFromFloat(0.50),
		FreeGB:            decimal.NewFromFloat(10.0), // Should be ignored
		PricePerGB:        decimal.NewFromFloat(2.00),
		FreeScores:        100, // Should be ignored
		PricePer1KScores:  decimal.NewFromFloat(0.10),
		HasVolumeTiers:    true,
		VolumeTiers:       tiers,
	}

	usage := &billing.BillableUsageSummary{
		TotalSpans:  150000000,              // 150M spans
		TotalBytes:  int64(50 * 1073741824), // 50 GB
		TotalScores: 500,                    // 500 scores
	}

	cost := worker.calculateRawCost(usage, pricing)

	// Expected cost (no free tier):
	// SPANS (tiered, no free tier):
	//   Tier 1 (0-100M): 100M / 100K * 0.30 = 1000 * 0.30 = $300
	//   Tier 2 (100M-150M): 50M / 100K * 0.25 = 500 * 0.25 = $125
	//   Span cost: $425
	//
	// BYTES (flat, no free tier):
	//   Cost: 50GB * 2.00 = $100
	//
	// SCORES (flat, no free tier):
	//   Cost: 500 / 1000 * 0.10 = $0.05
	//
	// Total: $525.05
	expectedCost := 425.0 + 100.0 + 0.05
	assert.InDelta(t, expectedCost, cost.InexactFloat64(), 0.01)
}

// NOTE: Removed TestUsageAggregationWorker_CalculateDimensionWithTiers_MixedTiersAndFlat
// and TestUsageAggregationWorker_CalculateFlatDimension tests because these methods
// are now delegated to PricingService. The pricing logic is comprehensively tested
// in internal/core/services/billing/pricing_service_test.go, including:
// - Free tier offset scenarios
// - Progressive tier calculations
// - Mixed tier and flat pricing
// - Edge cases and boundary conditions
//
// Worker tests now focus on orchestration (calculateWithTiers/calculateWithTiersNoFreeTier)
// rather than implementation details.

// Helper function
func ptrInt64(v int64) *int64 {
	return &v
}
