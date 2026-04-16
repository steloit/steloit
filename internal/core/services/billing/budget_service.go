package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	orgDomain "brokle/internal/core/domain/organization"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type budgetService struct {
	budgetRepo  billing.UsageBudgetRepository
	alertRepo   billing.UsageAlertRepository
	projectRepo orgDomain.ProjectRepository
	logger      *slog.Logger
}

func NewBudgetService(
	budgetRepo billing.UsageBudgetRepository,
	alertRepo billing.UsageAlertRepository,
	projectRepo orgDomain.ProjectRepository,
	logger *slog.Logger,
) billing.BudgetService {
	return &budgetService{
		budgetRepo:  budgetRepo,
		alertRepo:   alertRepo,
		projectRepo: projectRepo,
		logger:      logger,
	}
}

func (s *budgetService) CreateBudget(ctx context.Context, budget *billing.UsageBudget) error {
	// Validate project ownership if project_id is provided
	if budget.ProjectID != nil && *budget.ProjectID != uuid.Nil {
		project, err := s.projectRepo.GetByID(ctx, *budget.ProjectID)
		if err != nil {
			if errors.Is(err, orgDomain.ErrProjectNotFound) {
				s.logger.Warn("budget creation failed: project not found",
					"project_id", budget.ProjectID,
					"organization_id", budget.OrganizationID,
				)
				return appErrors.NewNotFoundError(fmt.Sprintf("project %s", *budget.ProjectID))
			}
			// Database/infrastructure error - return 500
			s.logger.Error("budget creation failed: project lookup error",
				"project_id", budget.ProjectID,
				"organization_id", budget.OrganizationID,
				"error", err,
			)
			return appErrors.NewInternalError("failed to validate project", err)
		}

		// Verify project belongs to the same organization
		if project.OrganizationID != budget.OrganizationID {
			s.logger.Warn("budget creation blocked: project belongs to different organization",
				"project_id", budget.ProjectID,
				"project_org_id", project.OrganizationID,
				"budget_org_id", budget.OrganizationID,
			)
			return appErrors.NewNotFoundError(fmt.Sprintf("project %s", *budget.ProjectID))
		}
	}

	if budget.ID == uuid.Nil {
		budget.ID = uid.New()
	}
	budget.CreatedAt = time.Now()
	budget.UpdatedAt = time.Now()
	budget.IsActive = true

	if err := s.budgetRepo.Create(ctx, budget); err != nil {
		s.logger.Error("failed to create budget",
			"error", err,
			"organization_id", budget.OrganizationID,
			"name", budget.Name,
		)
		return err
	}

	s.logger.Info("budget created",
		"budget_id", budget.ID,
		"organization_id", budget.OrganizationID,
		"name", budget.Name,
	)

	return nil
}

func (s *budgetService) GetBudget(ctx context.Context, id uuid.UUID) (*billing.UsageBudget, error) {
	return s.budgetRepo.GetByID(ctx, id)
}

func (s *budgetService) GetBudgetsByOrg(ctx context.Context, orgID uuid.UUID) ([]*billing.UsageBudget, error) {
	return s.budgetRepo.GetByOrgID(ctx, orgID)
}

func (s *budgetService) UpdateBudget(ctx context.Context, budget *billing.UsageBudget) error {
	budget.UpdatedAt = time.Now()

	if err := s.budgetRepo.Update(ctx, budget); err != nil {
		s.logger.Error("failed to update budget",
			"error", err,
			"budget_id", budget.ID,
		)
		return err
	}

	s.logger.Info("budget updated",
		"budget_id", budget.ID,
		"organization_id", budget.OrganizationID,
	)

	return nil
}

func (s *budgetService) DeleteBudget(ctx context.Context, id uuid.UUID) error {
	if err := s.budgetRepo.Delete(ctx, id); err != nil {
		s.logger.Error("failed to delete budget",
			"error", err,
			"budget_id", id,
		)
		return err
	}

	s.logger.Info("budget deleted",
		"budget_id", id,
	)

	return nil
}

func (s *budgetService) CheckBudgets(ctx context.Context, orgID uuid.UUID) ([]*billing.UsageAlert, error) {
	budgets, err := s.budgetRepo.GetActive(ctx, orgID)
	if err != nil {
		return nil, err
	}

	var newAlerts []*billing.UsageAlert

	for _, budget := range budgets {
		alerts := s.evaluateBudget(budget)
		for _, alert := range alerts {
			if err := s.alertRepo.Create(ctx, alert); err != nil {
				s.logger.Error("failed to create alert",
					"error", err,
					"budget_id", budget.ID,
				)
				continue
			}
			newAlerts = append(newAlerts, alert)

			s.logger.Warn("budget alert triggered",
				"alert_id", alert.ID,
				"budget_id", budget.ID,
				"alert_threshold", alert.AlertThreshold,
				"dimension", alert.Dimension,
				"percent_used", alert.PercentUsed,
			)
		}
	}

	return newAlerts, nil
}

func (s *budgetService) evaluateBudget(budget *billing.UsageBudget) []*billing.UsageAlert {
	var alerts []*billing.UsageAlert

	// Ensure thresholds are sorted ascending for correct reverse iteration
	sort.Slice(budget.AlertThresholds, func(i, j int) bool {
		return budget.AlertThresholds[i] < budget.AlertThresholds[j]
	})

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
			// Calculate percent using decimal, then convert to float64 for comparison
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

func (s *budgetService) GetAlerts(ctx context.Context, orgID uuid.UUID, limit int) ([]*billing.UsageAlert, error) {
	return s.alertRepo.GetByOrgID(ctx, orgID, limit)
}

func (s *budgetService) AcknowledgeAlert(ctx context.Context, orgID, alertID uuid.UUID) error {
	// Fetch alert and verify organization ownership
	alert, err := s.alertRepo.GetByID(ctx, alertID)
	if err != nil {
		s.logger.Error("failed to get alert for acknowledgement",
			"error", err,
			"alert_id", alertID,
		)
		return err
	}

	// Security: verify alert belongs to the requesting organization
	if alert.OrganizationID != orgID {
		s.logger.Warn("unauthorized alert acknowledgement attempt",
			"alert_id", alertID,
			"alert_org_id", alert.OrganizationID,
			"requested_org_id", orgID,
		)
		return fmt.Errorf("%w: %s", billing.ErrAlertNotFound, alertID) // Return sentinel error to avoid info leak
	}

	if err := s.alertRepo.Acknowledge(ctx, alertID); err != nil {
		s.logger.Error("failed to acknowledge alert",
			"error", err,
			"alert_id", alertID,
		)
		return err
	}

	s.logger.Info("alert acknowledged",
		"alert_id", alertID,
		"organization_id", orgID,
	)

	return nil
}
