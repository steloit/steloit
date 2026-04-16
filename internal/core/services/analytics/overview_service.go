package analytics

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"brokle/internal/core/domain/analytics"
	"brokle/internal/core/domain/credentials"
	"brokle/internal/core/domain/organization"

	"golang.org/x/sync/errgroup"
)

type overviewService struct {
	overviewRepo   analytics.OverviewRepository
	projectService organization.ProjectService
	credentialRepo credentials.ProviderCredentialRepository
	logger         *slog.Logger
}

// NewOverviewService creates a new overview service instance
func NewOverviewService(
	overviewRepo analytics.OverviewRepository,
	projectService organization.ProjectService,
	credentialRepo credentials.ProviderCredentialRepository,
	logger *slog.Logger,
) analytics.OverviewService {
	return &overviewService{
		overviewRepo:   overviewRepo,
		projectService: projectService,
		credentialRepo: credentialRepo,
		logger:         logger,
	}
}

// GetOverview retrieves the complete overview data for a project
func (s *overviewService) GetOverview(ctx context.Context, filter *analytics.OverviewFilter) (*analytics.OverviewResponse, error) {
	projectID := filter.ProjectID

	// Result holders (protected by errgroup's synchronization)
	var (
		stats           *analytics.OverviewStats
		traceVolume     []analytics.TimeSeriesPoint
		costTimeSeries  []analytics.TimeSeriesPoint
		tokenTimeSeries []analytics.TimeSeriesPoint
		errorTimeSeries []analytics.TimeSeriesPoint
		costByModel     []analytics.CostByModel
		recentTraces    []analytics.RecentTrace
		topErrors       []analytics.TopError
		scoresSummary   []analytics.ScoreSummary
		checklist       analytics.ChecklistStatus
	)

	// errgroup with context cancellation - cancels all goroutines on first error
	g, ctx := errgroup.WithContext(ctx)

	// Required queries - errors will cancel other goroutines and return
	g.Go(func() error {
		var err error
		stats, err = s.overviewRepo.GetStats(ctx, filter)
		if err != nil {
			s.logger.Error("failed to get overview stats", "error", err, "project_id", projectID)
			return fmt.Errorf("get overview stats: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		traceVolume, err = s.overviewRepo.GetTraceVolume(ctx, filter)
		if err != nil {
			s.logger.Error("failed to get trace volume", "error", err, "project_id", projectID)
			return fmt.Errorf("get trace volume: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		costTimeSeries, err = s.overviewRepo.GetCostTimeSeries(ctx, filter)
		if err != nil {
			s.logger.Error("failed to get cost time series", "error", err, "project_id", projectID)
			return fmt.Errorf("get cost time series: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		tokenTimeSeries, err = s.overviewRepo.GetTokenTimeSeries(ctx, filter)
		if err != nil {
			s.logger.Error("failed to get token time series", "error", err, "project_id", projectID)
			return fmt.Errorf("get token time series: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		errorTimeSeries, err = s.overviewRepo.GetErrorTimeSeries(ctx, filter)
		if err != nil {
			s.logger.Error("failed to get error time series", "error", err, "project_id", projectID)
			return fmt.Errorf("get error time series: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		costByModel, err = s.overviewRepo.GetCostByModel(ctx, filter)
		if err != nil {
			s.logger.Error("failed to get cost by model", "error", err, "project_id", projectID)
			return fmt.Errorf("get cost by model: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		recentTraces, err = s.overviewRepo.GetRecentTraces(ctx, filter, 5)
		if err != nil {
			s.logger.Error("failed to get recent traces", "error", err, "project_id", projectID)
			return fmt.Errorf("get recent traces: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		topErrors, err = s.overviewRepo.GetTopErrors(ctx, filter, 5)
		if err != nil {
			s.logger.Error("failed to get top errors", "error", err, "project_id", projectID)
			return fmt.Errorf("get top errors: %w", err)
		}
		return nil
	})

	// Optional queries - errors are logged but don't fail the request
	g.Go(func() error {
		var err error
		scoresSummary, err = s.overviewRepo.GetScoresSummary(ctx, filter, 3)
		if err != nil && ctx.Err() == nil {
			// Only log if not cancelled by another goroutine's failure
			s.logger.Warn("failed to get scores summary", "error", err, "project_id", projectID)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		checklist, err = s.getChecklistStatus(ctx, projectID)
		if err != nil {
			if ctx.Err() == nil {
				// Only log if not cancelled by another goroutine's failure
				s.logger.Warn("failed to get checklist status", "error", err, "project_id", projectID)
			}
			// Default to minimal checklist on error
			checklist = analytics.ChecklistStatus{HasProject: true}
		}
		return nil
	})

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Build response from collected results
	response := &analytics.OverviewResponse{
		Stats:           *stats,
		TraceVolume:     traceVolume,
		CostTimeSeries:  costTimeSeries,
		TokenTimeSeries: tokenTimeSeries,
		ErrorTimeSeries: errorTimeSeries,
		CostByModel:     costByModel,
		RecentTraces:    recentTraces,
		TopErrors:       topErrors,
		ScoresSummary:   scoresSummary,
		ChecklistStatus: checklist,
	}

	return response, nil
}

// getChecklistStatus retrieves the onboarding checklist status for a project
func (s *overviewService) getChecklistStatus(ctx context.Context, projectID uuid.UUID) (analytics.ChecklistStatus, error) {
	status := analytics.ChecklistStatus{
		HasProject: true, // Always true - they're viewing the page
	}

	// Check if project has traces
	hasTraces, err := s.overviewRepo.HasTraces(ctx, projectID)
	if err != nil {
		return status, fmt.Errorf("check has traces: %w", err)
	}
	status.HasTraces = hasTraces

	// Check if project has scores/evaluations
	hasScores, err := s.overviewRepo.HasScores(ctx, projectID)
	if err != nil {
		return status, fmt.Errorf("check has scores: %w", err)
	}
	status.HasEvaluations = hasScores

	// Check if organization has AI provider credentials configured
	if s.projectService != nil && s.credentialRepo != nil {
		project, err := s.projectService.GetProject(ctx, projectID)
		if err != nil {
			s.logger.Warn("failed to get project for checklist", "error", err, "project_id", projectID)
		} else {
			credentials, err := s.credentialRepo.ListByOrganization(ctx, project.OrganizationID)
			if err != nil {
				s.logger.Warn("failed to list credentials for checklist", "error", err, "org_id", project.OrganizationID)
			} else {
				status.HasAIProvider = len(credentials) > 0
			}
		}
	}

	return status, nil
}
