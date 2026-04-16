package billing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
)

// UsageTracker manages organization usage tracking and quotas
type UsageTracker struct {
	usageRepo    billingDomain.UsageRepository
	quotaRepo    billingDomain.QuotaRepository
	logger       *slog.Logger
	quotaCache   map[uuid.UUID]*billingDomain.UsageQuota
	stopCh       chan struct{}
	wg           sync.WaitGroup
	cacheExpiry  time.Duration
	syncInterval time.Duration
	cacheMutex   sync.RWMutex
}

// UsageUpdate represents an update to organization usage
type UsageUpdate struct {
	Timestamp      time.Time
	Currency       string
	Requests       int64
	Tokens         int64
	Cost           float64
	OrganizationID uuid.UUID
}

// NewUsageTracker creates a new usage tracker instance
func NewUsageTracker(
	logger *slog.Logger,
	usageRepo billingDomain.UsageRepository,
	quotaRepo billingDomain.QuotaRepository,
) *UsageTracker {
	tracker := &UsageTracker{
		logger:       logger,
		usageRepo:    usageRepo,
		quotaRepo:    quotaRepo,
		quotaCache:   make(map[uuid.UUID]*billingDomain.UsageQuota),
		cacheExpiry:  5 * time.Minute,
		syncInterval: 1 * time.Minute,
		stopCh:       make(chan struct{}),
	}

	// Start background sync
	tracker.wg.Add(1)
	go tracker.backgroundSync()

	return tracker
}

// UpdateUsage updates usage tracking for an organization
func (t *UsageTracker) UpdateUsage(ctx context.Context, orgID uuid.UUID, record *billingDomain.UsageRecord) error {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	// Get or load quota
	quota, err := t.getQuotaLocked(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get quota: %w", err)
	}

	if quota == nil {
		// No quota exists, create default one
		quota = &billingDomain.UsageQuota{
			OrganizationID:      orgID,
			BillingTier:         record.BillingTier,
			MonthlyRequestLimit: 0,            // Unlimited by default
			MonthlyTokenLimit:   0,            // Unlimited by default
			MonthlyCostLimit:    decimal.Zero, // Unlimited by default
			Currency:            record.Currency,
			ResetDate:           t.getNextResetDate(),
			LastUpdated:         time.Now(),
		}
	}

	// Update current usage
	quota.CurrentRequests++
	quota.CurrentTokens += int64(record.TotalTokens)
	quota.CurrentCost = quota.CurrentCost.Add(record.NetCost)
	quota.LastUpdated = time.Now()

	// Check if we need to reset monthly counters
	if time.Now().After(quota.ResetDate) {
		quota.CurrentRequests = 1 // This request
		quota.CurrentTokens = int64(record.TotalTokens)
		quota.CurrentCost = record.NetCost
		quota.ResetDate = t.getNextResetDate()
	}

	// Update cache
	t.quotaCache[orgID] = quota

	// Persist to database (async to avoid blocking)
	// Clone quota to avoid data race - the cached pointer may be modified by subsequent calls
	quotaSnapshot := quota.Clone()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := t.quotaRepo.UpdateUsageQuota(ctx, orgID, quotaSnapshot); err != nil {
			t.logger.Error("Failed to persist usage quota", "error", err, "org_id", orgID)
		}
	}()

	return nil
}

// GetUsageQuota retrieves current usage quota for an organization
func (t *UsageTracker) GetUsageQuota(ctx context.Context, orgID uuid.UUID) (*billingDomain.UsageQuota, error) {
	t.cacheMutex.RLock()
	defer t.cacheMutex.RUnlock()

	quota, err := t.getQuotaLocked(ctx, orgID)
	if err != nil {
		return nil, err
	}
	// Return a clone to prevent callers from modifying cached data
	return quota.Clone(), nil
}

// SetUsageQuota sets usage quota limits for an organization
func (t *UsageTracker) SetUsageQuota(ctx context.Context, orgID uuid.UUID, quota *billingDomain.UsageQuota) error {
	quota.LastUpdated = time.Now()

	// Update database
	if err := t.quotaRepo.UpdateUsageQuota(ctx, orgID, quota); err != nil {
		return fmt.Errorf("failed to update usage quota: %w", err)
	}

	// Update cache with a clone to prevent external modification
	t.cacheMutex.Lock()
	t.quotaCache[orgID] = quota.Clone()
	t.cacheMutex.Unlock()

	t.logger.Info("Updated usage quota", "org_id", orgID, "request_limit", quota.MonthlyRequestLimit, "token_limit", quota.MonthlyTokenLimit, "cost_limit", quota.MonthlyCostLimit)

	return nil
}

// CheckQuotaExceeded checks if organization has exceeded any quotas
func (t *UsageTracker) CheckQuotaExceeded(ctx context.Context, orgID uuid.UUID) (*billingDomain.QuotaStatus, error) {
	quota, err := t.GetUsageQuota(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage quota: %w", err)
	}

	if quota == nil {
		// No quota set, allow unlimited usage
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
	}

	// Check request limits
	if quota.MonthlyRequestLimit > 0 {
		status.RequestsOK = quota.CurrentRequests < quota.MonthlyRequestLimit
		status.RequestsUsagePercent = float64(quota.CurrentRequests) / float64(quota.MonthlyRequestLimit) * 100
	} else {
		status.RequestsOK = true
	}

	// Check token limits
	if quota.MonthlyTokenLimit > 0 {
		status.TokensOK = quota.CurrentTokens < quota.MonthlyTokenLimit
		status.TokensUsagePercent = float64(quota.CurrentTokens) / float64(quota.MonthlyTokenLimit) * 100
	} else {
		status.TokensOK = true
	}

	// Check cost limits
	if !quota.MonthlyCostLimit.IsZero() {
		status.CostOK = quota.CurrentCost.LessThan(quota.MonthlyCostLimit)
		status.CostUsagePercent = quota.CurrentCost.Div(quota.MonthlyCostLimit).Mul(decimal.NewFromInt(100)).InexactFloat64()
	} else {
		status.CostOK = true
	}

	// Determine overall status
	if !status.RequestsOK {
		status.Status = "requests_exceeded"
	} else if !status.TokensOK {
		status.Status = "tokens_exceeded"
	} else if !status.CostOK {
		status.Status = "cost_exceeded"
	} else {
		status.Status = "within_limits"
	}

	return status, nil
}

// GetUsageHistory retrieves usage history for an organization
func (t *UsageTracker) GetUsageHistory(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billingDomain.UsageRecord, error) {
	return t.usageRepo.GetUsageRecords(ctx, orgID, start, end)
}

// ResetMonthlyUsage resets monthly usage counters for an organization
func (t *UsageTracker) ResetMonthlyUsage(ctx context.Context, orgID uuid.UUID) error {
	quota, err := t.GetUsageQuota(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get usage quota: %w", err)
	}

	if quota == nil {
		return fmt.Errorf("no usage quota found for organization %s", orgID)
	}

	// Reset counters
	quota.CurrentRequests = 0
	quota.CurrentTokens = 0
	quota.CurrentCost = decimal.Zero
	quota.ResetDate = t.getNextResetDate()
	quota.LastUpdated = time.Now()

	// Update database and cache
	if err := t.SetUsageQuota(ctx, orgID, quota); err != nil {
		return fmt.Errorf("failed to reset usage quota: %w", err)
	}

	t.logger.Info("Reset monthly usage counters", "org_id", orgID)
	return nil
}

// Stop stops the usage tracker background processes
func (t *UsageTracker) Stop() {
	close(t.stopCh)
	t.wg.Wait()
}

// Health check
func (t *UsageTracker) GetHealth() map[string]interface{} {
	t.cacheMutex.RLock()
	cacheSize := len(t.quotaCache)
	t.cacheMutex.RUnlock()

	return map[string]interface{}{
		"service":               "usage_tracker",
		"status":                "healthy",
		"cached_quotas":         cacheSize,
		"cache_expiry_minutes":  t.cacheExpiry.Minutes(),
		"sync_interval_seconds": t.syncInterval.Seconds(),
	}
}

// Internal methods

func (t *UsageTracker) getQuotaLocked(ctx context.Context, orgID uuid.UUID) (*billingDomain.UsageQuota, error) {
	// Check cache first
	if quota, exists := t.quotaCache[orgID]; exists {
		// Check if cache entry is still valid
		if time.Since(quota.LastUpdated) < t.cacheExpiry {
			return quota, nil
		}
		// Cache expired, remove it
		delete(t.quotaCache, orgID)
	}

	// Load from database
	quota, err := t.quotaRepo.GetUsageQuota(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to load usage quota from database: %w", err)
	}

	// Update cache if quota exists
	if quota != nil {
		t.quotaCache[orgID] = quota
	}

	return quota, nil
}

func (t *UsageTracker) getNextResetDate() time.Time {
	now := time.Now()
	// Reset on the first of next month
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
}

func (t *UsageTracker) backgroundSync() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.syncQuotas()
		}
	}
}

func (t *UsageTracker) syncQuotas() {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	// Check for expired quotas and quotas that need monthly reset
	now := time.Now()
	var expiredOrgs []uuid.UUID

	for orgID, quota := range t.quotaCache {
		// Check if cache entry expired
		if now.Sub(quota.LastUpdated) > t.cacheExpiry {
			expiredOrgs = append(expiredOrgs, orgID)
			continue
		}

		// Check if monthly reset is needed
		if now.After(quota.ResetDate) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

			// Reset monthly counters
			quota.CurrentRequests = 0
			quota.CurrentTokens = 0
			quota.CurrentCost = decimal.Zero
			quota.ResetDate = t.getNextResetDate()
			quota.LastUpdated = now

			// Persist reset
			if err := t.quotaRepo.UpdateUsageQuota(ctx, orgID, quota); err != nil {
				t.logger.Error("Failed to sync quota reset", "error", err, "org_id", orgID)
			} else {
				t.logger.Info("Monthly usage quota reset", "org_id", orgID)
			}

			cancel()
		}
	}

	// Remove expired cache entries
	for _, orgID := range expiredOrgs {
		delete(t.quotaCache, orgID)
	}

	if len(expiredOrgs) > 0 {
		t.logger.Debug("Cleared expired quota cache entries", "expired_count", len(expiredOrgs))
	}
}

// GetUsageMetrics returns usage metrics for monitoring
func (t *UsageTracker) GetUsageMetrics(ctx context.Context, orgID uuid.UUID) (map[string]interface{}, error) {
	quota, err := t.GetUsageQuota(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage quota: %w", err)
	}

	if quota == nil {
		return map[string]interface{}{
			"organization_id": orgID,
			"has_quota":       false,
			"unlimited":       true,
		}, nil
	}

	return map[string]interface{}{
		"organization_id":       orgID,
		"has_quota":             true,
		"billing_tier":          quota.BillingTier,
		"current_requests":      quota.CurrentRequests,
		"current_tokens":        quota.CurrentTokens,
		"current_cost":          quota.CurrentCost,
		"monthly_request_limit": quota.MonthlyRequestLimit,
		"monthly_token_limit":   quota.MonthlyTokenLimit,
		"monthly_cost_limit":    quota.MonthlyCostLimit,
		"currency":              quota.Currency,
		"reset_date":            quota.ResetDate,
		"last_updated":          quota.LastUpdated,
		"requests_usage_percent": func() float64 {
			if quota.MonthlyRequestLimit > 0 {
				return float64(quota.CurrentRequests) / float64(quota.MonthlyRequestLimit) * 100
			}
			return 0
		}(),
		"tokens_usage_percent": func() float64 {
			if quota.MonthlyTokenLimit > 0 {
				return float64(quota.CurrentTokens) / float64(quota.MonthlyTokenLimit) * 100
			}
			return 0
		}(),
		"cost_usage_percent": func() float64 {
			if !quota.MonthlyCostLimit.IsZero() {
				return quota.CurrentCost.Div(quota.MonthlyCostLimit).Mul(decimal.NewFromInt(100)).InexactFloat64()
			}
			return 0
		}(),
	}, nil
}
