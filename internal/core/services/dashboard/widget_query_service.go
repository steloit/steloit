package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	dashboardDomain "brokle/internal/core/domain/dashboard"
	appErrors "brokle/pkg/errors"
)

type widgetQueryService struct {
	dashboardRepo dashboardDomain.DashboardRepository
	queryRepo     dashboardDomain.WidgetQueryRepository
	queryBuilder  *WidgetQueryBuilder
	logger        *slog.Logger
}

func NewWidgetQueryService(
	dashboardRepo dashboardDomain.DashboardRepository,
	queryRepo dashboardDomain.WidgetQueryRepository,
	logger *slog.Logger,
) dashboardDomain.WidgetQueryService {
	return &widgetQueryService{
		dashboardRepo: dashboardRepo,
		queryRepo:     queryRepo,
		queryBuilder:  NewWidgetQueryBuilder(),
		logger:        logger,
	}
}

func (s *widgetQueryService) ExecuteWidgetQuery(
	ctx context.Context,
	projectID uuid.UUID,
	widget *dashboardDomain.Widget,
	timeRange *dashboardDomain.TimeRange,
) (*dashboardDomain.QueryResult, error) {
	startTime := time.Now()

	start, end := s.resolveTimeRange(timeRange)

	result := &dashboardDomain.QueryResult{
		WidgetID: widget.ID,
		Metadata: &dashboardDomain.QueryMetadata{
			ExecutedAt: startTime,
			Cached:     false,
		},
	}

	switch widget.Type {
	case dashboardDomain.WidgetTypeText:
		// Text widgets have static content, no query needed
		result.Data = []map[string]interface{}{
			{"content": widget.Config["content"]},
		}
		result.Metadata.RowCount = 1
		result.Metadata.DurationMs = time.Since(startTime).Milliseconds()
		return result, nil

	case dashboardDomain.WidgetTypeTraceList:
		return s.executeTraceListQuery(ctx, projectID, widget, start, end, startTime)

	case dashboardDomain.WidgetTypeHistogram:
		return s.executeHistogramQuery(ctx, projectID, widget, start, end, startTime)

	default:
		return s.executeStandardQuery(ctx, projectID, widget, start, end, startTime)
	}
}

func (s *widgetQueryService) executeStandardQuery(
	ctx context.Context,
	projectID uuid.UUID,
	widget *dashboardDomain.Widget,
	startTime, endTime *time.Time,
	executionStart time.Time,
) (*dashboardDomain.QueryResult, error) {
	result := &dashboardDomain.QueryResult{
		WidgetID: widget.ID,
		Metadata: &dashboardDomain.QueryMetadata{
			ExecutedAt: executionStart,
			Cached:     false,
		},
	}

	queryResult, err := s.queryBuilder.BuildWidgetQuery(&widget.Query, projectID.String(), startTime, endTime)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	s.logger.Debug("executing widget query",
		"widget_id", widget.ID,
		"widget_type", widget.Type,
		"query", queryResult.Query,
	)

	data, err := s.queryRepo.ExecuteQuery(ctx, queryResult.Query, queryResult.Args)
	if err != nil {
		s.logger.Error("failed to execute widget query",
			"widget_id", widget.ID,
			"error", err,
		)
		result.Error = "Failed to execute query"
		return result, nil
	}

	// Ensure Data is never nil - always an empty slice for proper JSON serialization
	if data == nil {
		data = make([]map[string]interface{}, 0)
	}
	result.Data = data
	result.Metadata.RowCount = len(data)
	result.Metadata.DurationMs = time.Since(executionStart).Milliseconds()

	s.logger.Debug("query result details",
		"widget_id", widget.ID,
		"widget_type", widget.Type,
		"view", widget.Query.View,
		"data_len", len(data),
	)

	return result, nil
}

func (s *widgetQueryService) executeTraceListQuery(
	ctx context.Context,
	projectID uuid.UUID,
	widget *dashboardDomain.Widget,
	startTime, endTime *time.Time,
	executionStart time.Time,
) (*dashboardDomain.QueryResult, error) {
	result := &dashboardDomain.QueryResult{
		WidgetID: widget.ID,
		Metadata: &dashboardDomain.QueryMetadata{
			ExecutedAt: executionStart,
			Cached:     false,
		},
	}

	queryResult, err := s.queryBuilder.BuildTraceListQuery(&widget.Query, projectID.String(), startTime, endTime)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	s.logger.Debug("executing trace list query",
		"widget_id", widget.ID,
		"query", queryResult.Query,
	)

	traces, err := s.queryRepo.ExecuteTraceListQuery(ctx, queryResult.Query, queryResult.Args)
	if err != nil {
		s.logger.Error("failed to execute trace list query",
			"widget_id", widget.ID,
			"error", err,
		)
		result.Error = "Failed to execute query"
		return result, nil
	}

	// Ensure traces is never nil for proper JSON serialization
	if traces == nil {
		traces = make([]*dashboardDomain.TraceListItem, 0)
	}

	data := make([]map[string]interface{}, len(traces))
	for i, trace := range traces {
		data[i] = map[string]interface{}{
			"trace_id":      trace.TraceID,
			"name":          trace.Name,
			"start_time":    trace.StartTime,
			"duration_nano": trace.DurationNano,
			"status_code":   trace.StatusCode,
		}
		if trace.TotalCost != nil {
			data[i]["total_cost"] = *trace.TotalCost
		}
		if trace.ModelName != nil {
			data[i]["model_name"] = *trace.ModelName
		}
		if trace.ProviderName != nil {
			data[i]["provider_name"] = *trace.ProviderName
		}
		if trace.ServiceName != nil {
			data[i]["service_name"] = *trace.ServiceName
		}
	}

	result.Data = data
	result.Metadata.RowCount = len(data)
	result.Metadata.DurationMs = time.Since(executionStart).Milliseconds()

	return result, nil
}

func (s *widgetQueryService) executeHistogramQuery(
	ctx context.Context,
	projectID uuid.UUID,
	widget *dashboardDomain.Widget,
	startTime, endTime *time.Time,
	executionStart time.Time,
) (*dashboardDomain.QueryResult, error) {
	result := &dashboardDomain.QueryResult{
		WidgetID: widget.ID,
		Metadata: &dashboardDomain.QueryMetadata{
			ExecutedAt: executionStart,
			Cached:     false,
		},
	}

	bucketCount := 20
	if bc, ok := widget.Config["bucket_count"].(float64); ok {
		bucketCount = int(bc)
	}

	queryResult, err := s.queryBuilder.BuildHistogramQuery(&widget.Query, projectID.String(), startTime, endTime, bucketCount)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	s.logger.Debug("executing histogram query",
		"widget_id", widget.ID,
		"query", queryResult.Query,
	)

	histogramData, err := s.queryRepo.ExecuteHistogramQuery(ctx, queryResult.Query, queryResult.Args)
	if err != nil {
		s.logger.Error("failed to execute histogram query",
			"widget_id", widget.ID,
			"error", err,
		)
		result.Error = "Failed to execute query"
		return result, nil
	}

	// Ensure histogramData is never nil for proper JSON serialization
	if histogramData == nil {
		histogramData = &dashboardDomain.HistogramData{
			Buckets: make([]dashboardDomain.HistogramBucket, 0),
		}
	}

	data := make([]map[string]interface{}, len(histogramData.Buckets))
	for i, bucket := range histogramData.Buckets {
		data[i] = map[string]interface{}{
			"bucket":      i,
			"lower_bound": bucket.LowerBound,
			"upper_bound": bucket.UpperBound,
			"count":       bucket.Count,
		}
	}

	if histogramData.Stats != nil {
		result.Metadata = &dashboardDomain.QueryMetadata{
			ExecutedAt: executionStart,
			Cached:     false,
			RowCount:   len(data),
			DurationMs: time.Since(executionStart).Milliseconds(),
		}
	}

	result.Data = data
	result.Metadata.RowCount = len(data)
	result.Metadata.DurationMs = time.Since(executionStart).Milliseconds()

	return result, nil
}

func (s *widgetQueryService) ExecuteDashboardQueries(
	ctx context.Context,
	req *dashboardDomain.QueryExecutionRequest,
) (*dashboardDomain.DashboardQueryResults, error) {
	dashboard, err := s.dashboardRepo.GetByID(ctx, req.DashboardID)
	if err != nil {
		if errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
			return nil, appErrors.NewNotFoundError("dashboard")
		}
		return nil, appErrors.NewInternalError("failed to get dashboard", err)
	}

	if dashboard.ProjectID != req.ProjectID {
		return nil, appErrors.NewNotFoundError("dashboard")
	}

	results := &dashboardDomain.DashboardQueryResults{
		DashboardID: req.DashboardID,
		Results:     make(map[string]*dashboardDomain.QueryResult),
		ExecutedAt:  time.Now(),
	}

	widgets := dashboard.Config.Widgets
	if req.WidgetID != nil {
		// Execute only the specified widget
		for _, w := range dashboard.Config.Widgets {
			if w.ID == *req.WidgetID {
				widgets = []dashboardDomain.Widget{w}
				break
			}
		}
		if len(widgets) == 0 || widgets[0].ID != *req.WidgetID {
			return nil, appErrors.NewNotFoundError("widget")
		}
	}

	timeRange := req.TimeRange
	if timeRange == nil && dashboard.Config.TimeRange != nil {
		timeRange = dashboard.Config.TimeRange
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Use a semaphore to limit concurrent queries (prevent overwhelming ClickHouse)
	maxConcurrent := 10
	sem := make(chan struct{}, maxConcurrent)

	for _, widget := range widgets {
		wg.Add(1)
		widgetCopy := widget // avoid closure capture issue

		go func(w dashboardDomain.Widget) {
			defer wg.Done()

			// Acquire semaphore slot
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check if context is cancelled
			if ctx.Err() != nil {
				return
			}

			result, err := s.ExecuteWidgetQuery(ctx, req.ProjectID, &w, timeRange)
			if err != nil {
				s.logger.Error("failed to execute widget query",
					"widget_id", w.ID,
					"error", err,
				)
				result = &dashboardDomain.QueryResult{
					WidgetID: w.ID,
					Error:    "Failed to execute query",
				}
			}

			mu.Lock()
			results.Results[w.ID] = result
			mu.Unlock()
		}(widgetCopy)
	}

	wg.Wait()

	s.logger.Info("executed dashboard queries",
		"dashboard_id", req.DashboardID,
		"widget_count", len(widgets),
		"duration_ms", time.Since(results.ExecutedAt).Milliseconds(),
	)

	return results, nil
}

func (s *widgetQueryService) GetViewDefinitions(ctx context.Context) (*dashboardDomain.ViewDefinitionResponse, error) {
	allViews := dashboardDomain.GetAllViewDefinitions()

	response := &dashboardDomain.ViewDefinitionResponse{
		Views: make(map[dashboardDomain.ViewType]*dashboardDomain.ViewDefinitionPublic),
	}

	for viewType, viewDef := range allViews {
		measures := make([]dashboardDomain.MeasurePublic, 0, len(viewDef.Measures))
		for _, m := range viewDef.Measures {
			measures = append(measures, dashboardDomain.MeasurePublic{
				ID:          m.ID,
				Label:       m.Label,
				Description: m.Description,
				Type:        string(m.Type),
				Unit:        string(m.Unit),
			})
		}

		dimensions := make([]dashboardDomain.DimensionPublic, 0, len(viewDef.Dimensions))
		for _, d := range viewDef.Dimensions {
			dimensions = append(dimensions, dashboardDomain.DimensionPublic{
				ID:          d.ID,
				Label:       d.Label,
				Description: d.Description,
				ColumnType:  d.ColumnType,
				Bucketable:  d.Bucketable,
			})
		}

		response.Views[viewType] = &dashboardDomain.ViewDefinitionPublic{
			Name:        viewDef.Name,
			Description: viewDef.Description,
			Measures:    measures,
			Dimensions:  dimensions,
		}
	}

	return response, nil
}

func (s *widgetQueryService) GetVariableOptions(
	ctx context.Context,
	req *dashboardDomain.VariableOptionsRequest,
) (*dashboardDomain.VariableOptionsResponse, error) {
	viewDef := dashboardDomain.GetViewDefinition(dashboardDomain.ViewType(req.View))
	if viewDef == nil {
		return nil, appErrors.NewValidationError("view", "invalid view type: "+string(req.View))
	}

	var dimension *dashboardDomain.DimensionConfig
	for _, d := range viewDef.Dimensions {
		if d.ID == req.Dimension {
			dimCopy := d // avoid taking address of loop variable
			dimension = &dimCopy
			break
		}
	}
	if dimension == nil {
		return nil, appErrors.NewValidationError("dimension", "invalid dimension for view: "+req.Dimension)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000 // Cap at 1000 to prevent performance issues
	}

	queryResult := s.queryBuilder.BuildVariableOptionsQuery(
		dashboardDomain.ViewType(req.View),
		dimension.SQL,
		req.ProjectID.String(),
		limit,
	)
	if queryResult == nil {
		return nil, appErrors.NewInternalError("failed to build variable options query", nil)
	}

	s.logger.Debug("executing variable options query",
		"view", req.View,
		"dimension", req.Dimension,
		"query", queryResult.Query,
	)

	data, err := s.queryRepo.ExecuteQuery(ctx, queryResult.Query, queryResult.Args)
	if err != nil {
		s.logger.Error("failed to execute variable options query",
			"view", req.View,
			"dimension", req.Dimension,
			"error", err,
		)
		return nil, appErrors.NewInternalError("failed to fetch variable options", err)
	}

	values := make([]string, 0, len(data))
	for _, row := range data {
		if val, ok := row["value"]; ok && val != nil {
			switch v := val.(type) {
			case string:
				if v != "" {
					values = append(values, v)
				}
			case int:
				values = append(values, strconv.Itoa(v))
			case int64:
				values = append(values, strconv.FormatInt(v, 10))
			case float64:
				values = append(values, fmt.Sprintf("%g", v))
			default:
				// Use fmt.Sprintf as fallback
				strVal := fmt.Sprintf("%v", v)
				if strVal != "" {
					values = append(values, strVal)
				}
			}
		}
	}

	return &dashboardDomain.VariableOptionsResponse{
		Values: values,
	}, nil
}

func (s *widgetQueryService) resolveTimeRange(tr *dashboardDomain.TimeRange) (*time.Time, *time.Time) {
	if tr == nil {
		// Default to last 24 hours
		end := time.Now()
		start := end.Add(-24 * time.Hour)
		return &start, &end
	}

	if tr.From != nil && tr.To != nil {
		return tr.From, tr.To
	}

	if tr.Relative != "" {
		end := time.Now()
		var start time.Time

		switch tr.Relative {
		case "15m":
			start = end.Add(-15 * time.Minute)
		case "30m":
			start = end.Add(-30 * time.Minute)
		case "1h":
			start = end.Add(-1 * time.Hour)
		case "3h":
			start = end.Add(-3 * time.Hour)
		case "6h":
			start = end.Add(-6 * time.Hour)
		case "12h":
			start = end.Add(-12 * time.Hour)
		case "24h":
			start = end.Add(-24 * time.Hour)
		case "7d":
			start = end.Add(-7 * 24 * time.Hour)
		case "14d":
			start = end.Add(-14 * 24 * time.Hour)
		case "30d":
			start = end.Add(-30 * 24 * time.Hour)
		default:
			// Default to 24 hours if unknown relative
			start = end.Add(-24 * time.Hour)
		}

		return &start, &end
	}

	end := time.Now()
	start := end.Add(-24 * time.Hour)
	return &start, &end
}
