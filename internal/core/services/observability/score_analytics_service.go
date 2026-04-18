package observability

import (
	"github.com/google/uuid"
	"context"
	"log/slog"

	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
)

var validIntervals = map[string]bool{
	"hour": true,
	"day":  true,
	"week": true,
}

type ScoreAnalyticsService struct {
	analyticsRepo observability.ScoreAnalyticsRepository
	logger        *slog.Logger
}

func NewScoreAnalyticsService(analyticsRepo observability.ScoreAnalyticsRepository, logger *slog.Logger) *ScoreAnalyticsService {
	return &ScoreAnalyticsService{
		analyticsRepo: analyticsRepo,
		logger:        logger,
	}
}

func (s *ScoreAnalyticsService) GetAnalytics(ctx context.Context, filter *observability.ScoreAnalyticsFilter) (*observability.ScoreAnalyticsResponse, error) {
	if filter.ProjectID == uuid.Nil {
		return nil, appErrors.NewValidationError("project_id is required", "analytics query requires a project_id")
	}
	if filter.ScoreName == "" {
		return nil, appErrors.NewValidationError("score_name is required", "analytics query requires a score_name")
	}

	// Validate and default interval
	if filter.Interval == "" {
		filter.Interval = "day"
	} else if !validIntervals[filter.Interval] {
		return nil, appErrors.NewValidationError("invalid interval", "interval must be one of: hour, day, week")
	}

	response := &observability.ScoreAnalyticsResponse{}

	stats, err := s.analyticsRepo.GetStatistics(ctx, filter)
	if err != nil {
		s.logger.Error("GetStatistics failed", "error", err, "project_id", filter.ProjectID, "score_name", filter.ScoreName)
		return nil, appErrors.NewInternalError("failed to get score statistics", err)
	}
	response.Statistics = stats

	timeSeries, err := s.analyticsRepo.GetTimeSeries(ctx, filter)
	if err != nil {
		s.logger.Error("GetTimeSeries failed", "error", err, "project_id", filter.ProjectID, "score_name", filter.ScoreName)
		return nil, appErrors.NewInternalError("failed to get score time series", err)
	}
	response.TimeSeries = timeSeries

	distribution, err := s.analyticsRepo.GetDistribution(ctx, filter, 10)
	if err != nil {
		s.logger.Error("GetDistribution failed", "error", err, "project_id", filter.ProjectID, "score_name", filter.ScoreName)
		return nil, appErrors.NewInternalError("failed to get score distribution", err)
	}
	response.Distribution = distribution

	if filter.CompareScoreName != nil && *filter.CompareScoreName != "" {
		heatmap, err := s.analyticsRepo.GetHeatmap(ctx, filter, 10)
		if err != nil {
			s.logger.Error("GetHeatmap failed", "error", err, "project_id", filter.ProjectID, "score_name", filter.ScoreName, "compare_score_name", *filter.CompareScoreName)
			return nil, appErrors.NewInternalError("failed to get score heatmap", err)
		}
		response.Heatmap = heatmap

		comparison, err := s.analyticsRepo.GetComparisonMetrics(ctx, filter)
		if err != nil {
			s.logger.Error("GetComparisonMetrics failed", "error", err, "project_id", filter.ProjectID, "score_name", filter.ScoreName, "compare_score_name", *filter.CompareScoreName)
			return nil, appErrors.NewInternalError("failed to get score comparison metrics", err)
		}
		response.Comparison = comparison
	}

	return response, nil
}

func (s *ScoreAnalyticsService) GetDistinctScoreNames(ctx context.Context, projectID string) ([]string, error) {
	if projectID == "" {
		return nil, appErrors.NewValidationError("project_id is required", "score names query requires a project_id")
	}

	names, err := s.analyticsRepo.GetDistinctScoreNames(ctx, projectID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get distinct score names", err)
	}

	return names, nil
}
