package workers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/billing"
	"brokle/internal/core/domain/common"
	"brokle/internal/core/domain/organization"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/pagination"
	"brokle/pkg/uid"
	"brokle/pkg/units"
)

// UsageAggregationWorker syncs ClickHouse usage data to PostgreSQL billing state
// and checks budget thresholds to trigger alerts
type UsageAggregationWorker struct {
	config                   *config.Config
	logger                   *slog.Logger
	transactor               common.Transactor
	usageRepo                billing.BillableUsageRepository
	billingRepo              billing.OrganizationBillingRepository
	budgetRepo               billing.UsageBudgetRepository
	alertRepo                billing.UsageAlertRepository
	orgRepo                  organization.OrganizationRepository
	pricingService           billing.PricingService
	notificationWorker       *NotificationWorker
	quit                     chan struct{}
	wg                       sync.WaitGroup
	ticker                   *time.Ticker
	alertDeduplicationWindow time.Duration
}

// NewUsageAggregationWorker creates a new usage aggregation worker
func NewUsageAggregationWorker(
	config *config.Config,
	logger *slog.Logger,
	transactor common.Transactor,
	usageRepo billing.BillableUsageRepository,
	billingRepo billing.OrganizationBillingRepository,
	budgetRepo billing.UsageBudgetRepository,
	alertRepo billing.UsageAlertRepository,
	orgRepo organization.OrganizationRepository,
	pricingService billing.PricingService,
	notificationWorker *NotificationWorker,
) *UsageAggregationWorker {
	// Get alert deduplication window from config (default 24 hours)
	alertDeduplicationHours := config.Workers.AlertDeduplicationHours
	if alertDeduplicationHours <= 0 {
		alertDeduplicationHours = 24
	}

	return &UsageAggregationWorker{
		config:                   config,
		logger:                   logger,
		transactor:               transactor,
		usageRepo:                usageRepo,
		billingRepo:              billingRepo,
		budgetRepo:               budgetRepo,
		alertRepo:                alertRepo,
		orgRepo:                  orgRepo,
		pricingService:           pricingService,
		notificationWorker:       notificationWorker,
		quit:                     make(chan struct{}),
		alertDeduplicationWindow: time.Duration(alertDeduplicationHours) * time.Hour,
	}
}

// Start starts the usage aggregation worker
func (w *UsageAggregationWorker) Start() {
	w.logger.Info("Starting usage aggregation worker")

	// Get sync interval from config (default 5 minutes)
	interval := 5 * time.Minute
	if w.config.Workers.UsageSyncIntervalMinutes > 0 {
		interval = time.Duration(w.config.Workers.UsageSyncIntervalMinutes) * time.Minute
	}

	w.ticker = time.NewTicker(interval)

	// Start the main loop goroutine (handles both immediate and ticker runs)
	w.wg.Add(1)
	go w.mainLoop()
}

// Stop stops the usage aggregation worker and waits for graceful shutdown
func (w *UsageAggregationWorker) Stop() {
	w.logger.Info("Stopping usage aggregation worker")
	close(w.quit)
	w.wg.Wait()
}

// mainLoop handles the worker lifecycle: immediate run, then ticker-based runs
func (w *UsageAggregationWorker) mainLoop() {
	defer w.wg.Done()

	// Run immediately on start
	w.run()

	// Then run on ticker
	for {
		select {
		case <-w.ticker.C:
			w.run()
		case <-w.quit:
			w.ticker.Stop()
			w.logger.Info("Usage aggregation worker stopped")
			return
		}
	}
}

// run executes a single aggregation cycle with paginated organization iteration
func (w *UsageAggregationWorker) run() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	w.logger.Debug("Starting usage aggregation cycle")
	startTime := time.Now()

	var syncedCount, alertCount int
	page := 1

	// Iterate through all organizations with pagination
	// This ensures ALL orgs are synced, not just the first 50
	for {
		filters := &organization.OrganizationFilters{
			Params: pagination.Params{
				Page:  page,
				Limit: pagination.MaxPageSize, // 100 orgs per batch
			},
		}

		orgs, err := w.orgRepo.List(ctx, filters)
		if err != nil {
			w.logger.Error("failed to list organizations", "error", err, "page", page)
			return
		}

		// No more organizations to process
		if len(orgs) == 0 {
			break
		}

		for _, org := range orgs {
			// Sync billing state for each organization
			if err := w.syncOrganizationUsage(ctx, org.ID); err != nil {
				w.logger.Error("failed to sync organization usage",
					"error", err,
					"organization_id", org.ID,
				)
				continue
			}
			syncedCount++

			// Check budgets and trigger alerts
			alerts, err := w.checkBudgets(ctx, org.ID)
			if err != nil {
				w.logger.Error("failed to check budgets",
					"error", err,
					"organization_id", org.ID,
				)
				continue
			}
			alertCount += len(alerts)

			// Send notifications for new alerts
			for _, alert := range alerts {
				w.sendAlertNotification(ctx, org, alert)
			}
		}

		// If we got fewer than batch size, we've reached the end
		if len(orgs) < pagination.MaxPageSize {
			break
		}

		page++
	}

	duration := time.Since(startTime)
	w.logger.Info("Usage aggregation cycle completed",
		"organizations_synced", syncedCount,
		"alerts_triggered", alertCount,
		"duration_ms", duration.Milliseconds(),
	)
}

// syncOrganizationUsage syncs ClickHouse usage to PostgreSQL billing state
func (w *UsageAggregationWorker) syncOrganizationUsage(ctx context.Context, orgID uuid.UUID) error {
	// Get current billing state
	orgBilling, err := w.billingRepo.GetByOrgID(ctx, orgID)
	if err != nil {
		// Organization might not have billing set up yet
		w.logger.Debug("no billing record for organization", "organization_id", orgID)
		return nil
	}

	// Get effective pricing (plan + contract overrides)
	effectivePricing, err := w.pricingService.GetEffectivePricingWithBilling(ctx, orgID, orgBilling)
	if err != nil {
		return err
	}

	// Check if we need to reset the billing period
	if time.Now().After(w.calculatePeriodEnd(orgBilling.BillingCycleStart, orgBilling.BillingCycleAnchorDay)) {
		if err := w.resetBillingPeriod(ctx, orgID, orgBilling); err != nil {
			return err
		}
		// Refresh billing state after reset
		orgBilling, err = w.billingRepo.GetByOrgID(ctx, orgID)
		if err != nil {
			return err
		}
	}

	// Query current period usage from ClickHouse
	filter := &billing.BillableUsageFilter{
		OrganizationID: orgID,
		Start:          orgBilling.BillingCycleStart,
		End:            time.Now(),
		Granularity:    "hourly",
	}

	summary, err := w.usageRepo.GetUsageSummary(ctx, filter)
	if err != nil {
		return err
	}

	// Calculate cost (tier-aware)
	cost := w.calculateCost(summary, effectivePricing)

	// Calculate free tier remaining
	freeSpansRemaining := max(0, effectivePricing.FreeSpans-summary.TotalSpans)
	freeBytesTotal := effectivePricing.FreeGB.Mul(decimal.NewFromInt(units.BytesPerGB)).IntPart()
	freeBytesRemaining := max(0, freeBytesTotal-summary.TotalBytes)
	freeScoresRemaining := max(0, effectivePricing.FreeScores-summary.TotalScores)

	// Wrap billing and budget updates in a transaction for atomicity
	// If budget update fails, billing update is rolled back
	return w.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		// Use SetUsage (idempotent) - sets cumulative values instead of adding
		// Multiple workers processing the same org will set the same values, preventing race conditions
		if err := w.billingRepo.SetUsage(ctx, orgID,
			summary.TotalSpans, summary.TotalBytes, summary.TotalScores, cost,
			freeSpansRemaining, freeBytesRemaining, freeScoresRemaining,
		); err != nil {
			return fmt.Errorf("set usage: %w", err)
		}

		if err := w.syncBudgetUsage(ctx, orgID, summary, cost, effectivePricing); err != nil {
			return fmt.Errorf("sync budget usage: %w", err)
		}

		return nil
	})
}

// resetBillingPeriod resets the billing period for an organization
func (w *UsageAggregationWorker) resetBillingPeriod(ctx context.Context, orgID uuid.UUID, current *billing.OrganizationBilling) error {
	w.logger.Info("Resetting billing period",
		"organization_id", orgID,
		"old_cycle_start", current.BillingCycleStart,
	)

	// Calculate new cycle start
	newCycleStart := w.calculatePeriodEnd(current.BillingCycleStart, current.BillingCycleAnchorDay)

	return w.billingRepo.ResetPeriod(ctx, orgID, newCycleStart)
}

// syncBudgetUsage syncs usage to all budgets for an organization
func (w *UsageAggregationWorker) syncBudgetUsage(ctx context.Context, orgID uuid.UUID, summary *billing.BillableUsageSummary, cost decimal.Decimal, effectivePricing *billing.EffectivePricing) error {
	budgets, err := w.budgetRepo.GetActive(ctx, orgID)
	if err != nil {
		return err
	}

	for _, budget := range budgets {
		var spans, bytes, scores int64
		var budgetCost decimal.Decimal

		if budget.ProjectID != nil {
			// Project-level budget - query project-specific usage
			filter := &billing.BillableUsageFilter{
				OrganizationID: orgID,
				ProjectID:      budget.ProjectID,
				Start:          w.getBudgetPeriodStart(budget),
				End:            time.Now(),
				Granularity:    "hourly",
			}
			projectSummary, err := w.usageRepo.GetUsageSummary(ctx, filter)
			if err != nil {
				w.logger.Warn("failed to get project usage",
					"error", err,
					"project_id", budget.ProjectID,
				)
				continue
			}
			spans = projectSummary.TotalSpans
			bytes = projectSummary.TotalBytes
			scores = projectSummary.TotalScores
			budgetCost = w.calculateRawCost(projectSummary, effectivePricing)
		} else {
			// Org-level budget - calculate cost using marginal cost approach
			result, err := w.calculateBudgetCost(ctx, orgID, budget, summary, cost, effectivePricing)
			if err != nil {
				w.logger.Warn("failed to calculate budget cost",
					"error", err,
					"budget_id", budget.ID,
				)
				continue
			}
			spans = result.spans
			bytes = result.bytes
			scores = result.scores
			budgetCost = result.cost
		}

		if err := w.budgetRepo.UpdateUsage(ctx, budget.ID, spans, bytes, scores, budgetCost); err != nil {
			w.logger.Warn("failed to update budget usage",
				"error", err,
				"budget_id", budget.ID,
			)
		}
	}

	return nil
}

// checkBudgets checks all budgets and returns any new alerts
func (w *UsageAggregationWorker) checkBudgets(ctx context.Context, orgID uuid.UUID) ([]*billing.UsageAlert, error) {
	budgets, err := w.budgetRepo.GetActive(ctx, orgID)
	if err != nil {
		return nil, err
	}

	var newAlerts []*billing.UsageAlert

	for _, budget := range budgets {
		alerts := w.evaluateBudget(budget)
		for _, alert := range alerts {
			// Check if we already have a recent alert for this budget/threshold/dimension
			if w.hasRecentAlert(ctx, budget.ID, alert.AlertThreshold, alert.Dimension) {
				continue
			}

			if err := w.alertRepo.Create(ctx, alert); err != nil {
				// Check if this is a duplicate alert (concurrent worker already created it)
				if appErrors.IsUniqueViolation(err) {
					w.logger.Debug("alert already exists (concurrent creation)",
						"budget_id", budget.ID,
						"threshold", alert.AlertThreshold,
						"dimension", alert.Dimension,
					)
					continue
				}
				w.logger.Error("failed to create alert",
					"error", err,
					"budget_id", budget.ID,
				)
				continue
			}
			newAlerts = append(newAlerts, alert)

			w.logger.Warn("budget alert triggered",
				"alert_id", alert.ID,
				"budget_id", budget.ID,
				"budget_name", budget.Name,
				"alert_threshold", alert.AlertThreshold,
				"dimension", alert.Dimension,
				"percent_used", alert.PercentUsed,
			)
		}
	}

	return newAlerts, nil
}

// evaluateBudget checks a single budget and returns any triggered alerts
func (w *UsageAggregationWorker) evaluateBudget(budget *billing.UsageBudget) []*billing.UsageAlert {
	var alerts []*billing.UsageAlert

	// Ensure thresholds are sorted ascending for correct reverse iteration
	sort.Slice(budget.AlertThresholds, func(i, j int) bool {
		return budget.AlertThresholds[i] < budget.AlertThresholds[j]
	})

	// Check each dimension
	dimensions := []struct {
		dimension      billing.AlertDimension
		current        int64
		limit          *int64
		currentDecimal decimal.Decimal
		limitDecimal   *decimal.Decimal
	}{
		{billing.AlertDimensionSpans, budget.CurrentSpans, budget.SpanLimit, decimal.Zero, nil},
		{billing.AlertDimensionBytes, budget.CurrentBytes, budget.BytesLimit, decimal.Zero, nil},
		{billing.AlertDimensionScores, budget.CurrentScores, budget.ScoreLimit, decimal.Zero, nil},
		{billing.AlertDimensionCost, 0, nil, budget.CurrentCost, budget.CostLimit},
	}

	for _, dim := range dimensions {
		var percentUsed float64
		var actualValue int64
		var thresholdValue int64

		if dim.dimension == billing.AlertDimensionCost {
			if dim.limitDecimal == nil || dim.limitDecimal.IsZero() {
				continue
			}
			percentUsed = dim.currentDecimal.Div(*dim.limitDecimal).Mul(decimal.NewFromInt(100)).InexactFloat64()
			actualValue = dim.currentDecimal.Mul(decimal.NewFromInt(100)).IntPart() // Store as cents
			thresholdValue = dim.limitDecimal.Mul(decimal.NewFromInt(100)).IntPart()
		} else {
			if dim.limit == nil || *dim.limit == 0 {
				continue
			}
			percentUsed = (float64(dim.current) / float64(*dim.limit)) * 100
			actualValue = dim.current
			thresholdValue = *dim.limit
		}

		// Iterate over flexible thresholds (sorted descending to trigger highest first)
		for i := len(budget.AlertThresholds) - 1; i >= 0; i-- {
			threshold := budget.AlertThresholds[i]
			if percentUsed >= float64(threshold) {
				alert := &billing.UsageAlert{
					ID:             uid.New(),
					BudgetID:       &budget.ID,
					OrganizationID: budget.OrganizationID,
					ProjectID:      budget.ProjectID,
					AlertThreshold: threshold,
					Dimension:      dim.dimension,
					Severity:       getSeverityForThreshold(threshold),
					ThresholdValue: thresholdValue,
					ActualValue:    actualValue,
					PercentUsed:    decimal.NewFromFloat(percentUsed),
					Status:         billing.AlertStatusTriggered,
					TriggeredAt:    time.Now(),
				}
				alerts = append(alerts, alert)
				break // Only trigger the highest threshold per dimension
			}
		}
	}

	return alerts
}

// getSeverityForThreshold returns the appropriate severity based on threshold percentage
func getSeverityForThreshold(threshold int64) billing.AlertSeverity {
	switch {
	case threshold >= 100:
		return billing.AlertSeverityCritical
	case threshold >= 80:
		return billing.AlertSeverityWarning
	default:
		return billing.AlertSeverityInfo
	}
}

// hasRecentAlert checks if there's a recent unresolved alert for the same budget/threshold/dimension
func (w *UsageAggregationWorker) hasRecentAlert(ctx context.Context, budgetID uuid.UUID, alertThreshold int64, dimension billing.AlertDimension) bool {
	alerts, err := w.alertRepo.GetByBudgetID(ctx, budgetID)
	if err != nil {
		return false
	}

	// Check for recent alerts within the deduplication window
	cutoff := time.Now().Add(-w.alertDeduplicationWindow)
	for _, alert := range alerts {
		if alert.AlertThreshold == alertThreshold &&
			alert.Dimension == dimension &&
			alert.TriggeredAt.After(cutoff) &&
			alert.Status != billing.AlertStatusResolved {
			return true
		}
	}

	return false
}

// sendAlertNotification sends notification for a new alert
func (w *UsageAggregationWorker) sendAlertNotification(ctx context.Context, org *organization.Organization, alert *billing.UsageAlert) {
	if w.notificationWorker == nil {
		return
	}

	// Get budget name for context
	budgetName := "Organization"
	if alert.BudgetID != nil {
		budget, err := w.budgetRepo.GetByID(ctx, *alert.BudgetID)
		if err == nil {
			budgetName = budget.Name
		}
	}

	// Format the dimension value
	var valueStr string
	switch alert.Dimension {
	case billing.AlertDimensionSpans:
		valueStr = formatNumber(alert.ActualValue)
	case billing.AlertDimensionBytes:
		valueStr = formatBytes(alert.ActualValue)
	case billing.AlertDimensionScores:
		valueStr = formatNumber(alert.ActualValue)
	case billing.AlertDimensionCost:
		valueStr = formatCurrency(float64(alert.ActualValue) / 100)
	}

	// Send email notification
	if org.BillingEmail != nil && *org.BillingEmail != "" {
		w.notificationWorker.QueueEmail(EmailJob{
			To:       []string{*org.BillingEmail},
			Subject:  "Usage Alert: " + string(alert.Dimension) + " threshold exceeded",
			Template: "usage_alert",
			TemplateData: map[string]any{
				"organization_name": org.Name,
				"budget_name":       budgetName,
				"dimension":         string(alert.Dimension),
				"percent_used":      alert.PercentUsed,
				"current_value":     valueStr,
				"severity":          string(alert.Severity),
			},
			Priority: "high",
		})
	}

	// Mark notification as sent
	if err := w.alertRepo.MarkNotificationSent(ctx, alert.ID); err != nil {
		w.logger.Warn("failed to mark notification sent",
			"error", err,
			"alert_id", alert.ID,
		)
	}
}

// calculateCost computes total cost from three billable dimensions with tier support
func (w *UsageAggregationWorker) calculateCost(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	if pricing.HasVolumeTiers {
		return w.calculateWithTiers(usage, pricing)
	}
	return w.calculateFlat(usage, pricing)
}

// calculateFlat uses simple linear pricing
func (w *UsageAggregationWorker) calculateFlat(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
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
// Delegates to PricingService for correct tier calculation logic
func (w *UsageAggregationWorker) calculateWithTiers(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	totalCost := decimal.Zero

	// Delegate to PricingService for tier calculations
	totalCost = totalCost.Add(w.pricingService.CalculateDimensionWithTiers(usage.TotalSpans, pricing.FreeSpans, billing.TierDimensionSpans, pricing.VolumeTiers, pricing))

	freeBytes := pricing.FreeGB.Mul(decimal.NewFromInt(units.BytesPerGB)).IntPart()
	totalCost = totalCost.Add(w.pricingService.CalculateDimensionWithTiers(usage.TotalBytes, freeBytes, billing.TierDimensionBytes, pricing.VolumeTiers, pricing))

	totalCost = totalCost.Add(w.pricingService.CalculateDimensionWithTiers(usage.TotalScores, pricing.FreeScores, billing.TierDimensionScores, pricing.VolumeTiers, pricing))

	return totalCost
}

// calculateRawCost computes cost for usage without applying free tier deductions.
// Used for project-level budgets where free tier is already accounted at org level.
func (w *UsageAggregationWorker) calculateRawCost(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	if pricing.HasVolumeTiers {
		return w.calculateWithTiersNoFreeTier(usage, pricing)
	}
	return w.calculateFlatNoFreeTier(usage, pricing)
}

// calculateFlatNoFreeTier uses simple linear pricing without free tier deductions
func (w *UsageAggregationWorker) calculateFlatNoFreeTier(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
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
// Delegates to PricingService for correct tier calculation logic
func (w *UsageAggregationWorker) calculateWithTiersNoFreeTier(usage *billing.BillableUsageSummary, pricing *billing.EffectivePricing) decimal.Decimal {
	totalCost := decimal.Zero

	// Delegate to PricingService for tier calculations without free tier
	totalCost = totalCost.Add(w.pricingService.CalculateDimensionWithTiers(usage.TotalSpans, 0, billing.TierDimensionSpans, pricing.VolumeTiers, pricing))
	totalCost = totalCost.Add(w.pricingService.CalculateDimensionWithTiers(usage.TotalBytes, 0, billing.TierDimensionBytes, pricing.VolumeTiers, pricing))
	totalCost = totalCost.Add(w.pricingService.CalculateDimensionWithTiers(usage.TotalScores, 0, billing.TierDimensionScores, pricing.VolumeTiers, pricing))

	return totalCost
}

// NOTE: Removed duplicate calculateDimensionWithTiers, calculateFlatDimension, and getDimensionUnitSize.
// Worker now delegates to PricingService.CalculateDimensionWithTiers for all tier calculations.
// This ensures single source of truth for billing logic and prevents bugs from duplicate code.

// calculatePeriodEnd calculates the end of the current billing period
func (w *UsageAggregationWorker) calculatePeriodEnd(cycleStart time.Time, anchorDay int) time.Time {
	nextMonth := cycleStart.AddDate(0, 1, 0)
	year, month, _ := nextMonth.Date()
	loc := nextMonth.Location()

	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	day := anchorDay
	if day > lastDay {
		day = lastDay
	}

	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

// getBudgetPeriodStart returns the start of the current budget period
func (w *UsageAggregationWorker) getBudgetPeriodStart(budget *billing.UsageBudget) time.Time {
	now := time.Now()
	switch budget.BudgetType {
	case billing.BudgetTypeWeekly:
		// Start of current week (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
	case billing.BudgetTypeMonthly:
		// Start of current month
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	default:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
}

// budgetCostResult holds the calculated cost and usage for a budget period
type budgetCostResult struct {
	spans  int64
	bytes  int64
	scores int64
	cost   decimal.Decimal
}

// calculateBudgetCost determines the cost attributable to a budget period.
//
// This handles the complex case where budget periods differ from billing cycles:
// - Weekly budgets during monthly billing cycles
// - Monthly budgets with different anchor days
//
// The key insight is "marginal cost": if the budget period starts after the
// billing cycle, the free tier may already be consumed. We calculate:
//
//	budgetCost = totalCycleCost - costBeforeBudgetPeriod
//
// This correctly attributes the cost increase to the budget period that caused it.
func (w *UsageAggregationWorker) calculateBudgetCost(
	ctx context.Context,
	orgID uuid.UUID,
	budget *billing.UsageBudget,
	summary *billing.BillableUsageSummary,
	totalCycleCost decimal.Decimal,
	effectivePricing *billing.EffectivePricing,
) (*budgetCostResult, error) {
	budgetPeriodStart := w.getBudgetPeriodStart(budget)

	// Case 1: Budget period aligns with billing cycle - use pre-fetched summary
	if budgetPeriodStart.Equal(summary.PeriodStart) {
		return &budgetCostResult{
			spans:  summary.TotalSpans,
			bytes:  summary.TotalBytes,
			scores: summary.TotalScores,
			cost:   totalCycleCost,
		}, nil
	}

	// Case 2: Budget period differs from billing cycle
	return w.getBudgetUsageMarginal(ctx, orgID, budget, budgetPeriodStart,
		summary.PeriodStart, totalCycleCost, effectivePricing)
}

// getBudgetUsageMarginal calculates cost when budget period differs from billing cycle.
//
// Uses marginal cost approach:
// 1. Get usage for just the budget period window
// 2. Calculate cost consumed before budget period started
// 3. Budget cost = total cycle cost - pre-budget cost
//
// This ensures free tier is properly allocated to earlier periods.
func (w *UsageAggregationWorker) getBudgetUsageMarginal(
	ctx context.Context,
	orgID uuid.UUID,
	budget *billing.UsageBudget,
	budgetStart time.Time,
	cycleStart time.Time,
	totalCycleCost decimal.Decimal,
	pricing *billing.EffectivePricing,
) (*budgetCostResult, error) {
	// Clamp budget window to not extend before billing cycle
	usageWindowStart := budgetStart
	if budgetStart.Before(cycleStart) {
		usageWindowStart = cycleStart
	}

	// Query usage for just the budget period
	budgetUsage, err := w.usageRepo.GetUsageSummary(ctx, &billing.BillableUsageFilter{
		OrganizationID: orgID,
		Start:          usageWindowStart,
		End:            time.Now(),
		Granularity:    "hourly",
	})
	if err != nil {
		return nil, fmt.Errorf("get budget period usage: %w", err)
	}

	result := &budgetCostResult{
		spans:  budgetUsage.TotalSpans,
		bytes:  budgetUsage.TotalBytes,
		scores: budgetUsage.TotalScores,
	}

	// If budget includes cycle start, full cost applies (free tier accounted for)
	if budgetStart.Before(cycleStart) || budgetStart.Equal(cycleStart) {
		result.cost = totalCycleCost
		return result, nil
	}

	// Calculate marginal cost: total - cost_before_budget
	preBudgetUsage, err := w.usageRepo.GetUsageSummary(ctx, &billing.BillableUsageFilter{
		OrganizationID: orgID,
		Start:          cycleStart,
		End:            budgetStart,
		Granularity:    "hourly",
	})
	if err != nil {
		// Fallback: use raw cost (no free tier) on query failure
		w.logger.Warn("Failed to get pre-budget usage, using raw cost",
			"error", err, "org_id", orgID, "budget_id", budget.ID)
		result.cost = w.calculateRawCost(budgetUsage, pricing)
		return result, nil
	}

	costBeforeBudget := w.calculateCost(preBudgetUsage, pricing)
	result.cost = decimal.Max(decimal.Zero, totalCycleCost.Sub(costBeforeBudget))

	return result, nil
}

// Helper formatting functions
func formatNumber(n int64) string {
	if n >= 1000000 {
		return formatFloat(float64(n)/1000000) + "M"
	}
	if n >= 1000 {
		return formatFloat(float64(n)/1000) + "K"
	}
	return strconv.FormatInt(n, 10)
}

func formatBytes(b int64) string {
	if b >= units.BytesPerGB {
		return formatFloat(float64(b)/float64(units.BytesPerGB)) + " GB"
	}
	if b >= 1048576 {
		return formatFloat(float64(b)/1048576) + " MB"
	}
	if b >= 1024 {
		return formatFloat(float64(b)/1024) + " KB"
	}
	return strconv.FormatInt(b, 10) + " B"
}

func formatCurrency(f float64) string {
	return "$" + formatFloat(f)
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}
