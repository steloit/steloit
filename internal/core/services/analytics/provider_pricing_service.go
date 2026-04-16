package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/analytics"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/shopspring/decimal"
)

// ProviderPricingServiceImpl implements pricing lookup with LRU caching
type ProviderPricingServiceImpl struct {
	modelRepo analytics.ProviderModelRepository
	cache     *lru.Cache[string, *analytics.ProviderPricingSnapshot]
}

// NewProviderPricingService creates a new pricing service with 5-minute TTL cache
func NewProviderPricingService(modelRepo analytics.ProviderModelRepository) analytics.ProviderPricingService {
	// Cache 1000 pricing snapshots (5-minute TTL handled by cache key with timestamp)
	cache, _ := lru.New[string, *analytics.ProviderPricingSnapshot](1000)

	return &ProviderPricingServiceImpl{
		modelRepo: modelRepo,
		cache:     cache,
	}
}

// GetProviderPricingSnapshot retrieves pricing snapshot for a model at specific time
// Implements 5-minute caching for performance (pricing doesn't change frequently)
func (s *ProviderPricingServiceImpl) GetProviderPricingSnapshot(
	ctx context.Context,
	projectID *uuid.UUID,
	modelName string,
	atTime time.Time,
) (*analytics.ProviderPricingSnapshot, error) {
	// Build cache key (5-minute granularity)
	cacheTime := atTime.Truncate(5 * time.Minute)
	var cacheKey string
	if projectID != nil {
		cacheKey = fmt.Sprintf("%s:%s:%s", projectID.String(), modelName, cacheTime.Format("2006-01-02T15:04"))
	} else {
		cacheKey = fmt.Sprintf("global:%s:%s", modelName, cacheTime.Format("2006-01-02T15:04"))
	}

	// Check cache
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached, nil
	}

	// Lookup provider model with temporal versioning
	model, err := s.modelRepo.GetProviderModelAtTime(ctx, projectID, modelName, atTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider model: %w", err)
	}

	// Lookup provider prices for this model (project-specific override takes precedence)
	prices, err := s.modelRepo.GetProviderPrices(ctx, model.ID, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	// Build pricing snapshot
	snapshot := &analytics.ProviderPricingSnapshot{
		ModelName:    model.ModelName,
		Pricing:      make(map[string]decimal.Decimal),
		SnapshotTime: atTime,
	}

	// Add all prices to snapshot
	for _, price := range prices {
		snapshot.Pricing[price.UsageType] = price.Price
	}

	// Cache for 5 minutes
	s.cache.Add(cacheKey, snapshot)

	return snapshot, nil
}

// CalculateProviderCost calculates costs from usage and pricing snapshot
// Returns cost breakdown map with "total" key for aggregated cost
func (s *ProviderPricingServiceImpl) CalculateProviderCost(
	usage map[string]uint64,
	pricing *analytics.ProviderPricingSnapshot,
) map[string]decimal.Decimal {
	costs := make(map[string]decimal.Decimal)
	total := decimal.Zero

	// Calculate cost for each usage type
	for usageType, units := range usage {
		// Skip total (calculated below)
		if usageType == "total" {
			continue
		}

		// Lookup price for this usage type
		price, exists := pricing.Pricing[usageType]
		if !exists {
			// No pricing for this usage type - skip
			continue
		}

		// Calculate: (units / 1,000,000) * price_per_million
		cost := decimal.NewFromInt(int64(units)).
			Div(decimal.NewFromInt(1_000_000)).
			Mul(price)

		costs[usageType] = cost
		total = total.Add(cost)
	}

	// Add total cost
	costs["total"] = total

	return costs
}
