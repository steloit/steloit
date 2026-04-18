package observability

import (
	"github.com/google/uuid"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"

	lru "github.com/hashicorp/golang-lru/v2"
)

// filterOptionsCacheEntry wraps cached filter options with expiration
type filterOptionsCacheEntry struct {
	options   *observability.TraceFilterOptions
	expiresAt time.Time
}

const filterOptionsCacheTTL = 5 * time.Minute

type TraceService struct {
	traceRepo            observability.TraceRepository
	logger               *slog.Logger
	filterOptionsCache   *lru.Cache[string, *filterOptionsCacheEntry]
	filterOptionsCacheMu sync.Mutex
}

func NewTraceService(
	traceRepo observability.TraceRepository,
	logger *slog.Logger,
) *TraceService {
	cache, _ := lru.New[string, *filterOptionsCacheEntry](500)

	return &TraceService{
		traceRepo:          traceRepo,
		logger:             logger,
		filterOptionsCache: cache,
	}
}

func (s *TraceService) IngestSpan(ctx context.Context, span *observability.Span) error {
	if span.TraceID == "" {
		return appErrors.NewValidationError("trace_id is required", "span must be linked to a trace")
	}
	if span.ProjectID == uuid.Nil {
		return appErrors.NewValidationError("project_id is required", "span must have a valid project_id")
	}
	if span.SpanName == "" {
		return appErrors.NewValidationError("span_name is required", "span name cannot be empty")
	}
	if span.SpanID == "" {
		return appErrors.NewValidationError("span_id is required", "span must have OTEL span_id")
	}
	if len(span.SpanID) != 16 {
		return appErrors.NewValidationError("invalid span_id", "OTEL span_id must be 16 hex characters")
	}

	if span.StatusCode == 0 {
		span.StatusCode = observability.StatusCodeUnset
	}
	if span.SpanKind == 0 {
		span.SpanKind = observability.SpanKindInternal
	}
	if span.SpanAttributes == nil {
		span.SpanAttributes = make(map[string]string)
	}
	if span.ResourceAttributes == nil {
		span.ResourceAttributes = make(map[string]string)
	}
	if span.ScopeAttributes == nil {
		span.ScopeAttributes = make(map[string]string)
	}

	span.CalculateDuration()

	if err := s.traceRepo.InsertSpan(ctx, span); err != nil {
		return appErrors.NewInternalError("failed to create span", err)
	}

	return nil
}

func (s *TraceService) IngestSpanBatch(ctx context.Context, spans []*observability.Span) error {
	if len(spans) == 0 {
		return nil
	}

	for i, span := range spans {
		if span.TraceID == "" {
			return appErrors.NewValidationError(fmt.Sprintf("span[%d].trace_id", i), "trace_id is required")
		}
		if span.ProjectID == uuid.Nil {
			return appErrors.NewValidationError(fmt.Sprintf("span[%d].project_id", i), "project_id is required")
		}
		if span.SpanName == "" {
			return appErrors.NewValidationError(fmt.Sprintf("span[%d].span_name", i), "span_name is required")
		}
		if span.SpanID == "" {
			return appErrors.NewValidationError(fmt.Sprintf("span[%d].span_id", i), "OTEL span_id is required")
		}

		// Set defaults
		if span.StatusCode == 0 {
			span.StatusCode = observability.StatusCodeUnset
		}
		if span.SpanKind == 0 {
			span.SpanKind = observability.SpanKindInternal
		}
		if span.SpanAttributes == nil {
			span.SpanAttributes = make(map[string]string)
		}
		if span.ResourceAttributes == nil {
			span.ResourceAttributes = make(map[string]string)
		}
		if span.ScopeAttributes == nil {
			span.ScopeAttributes = make(map[string]string)
		}

		span.CalculateDuration()
	}

	if err := s.traceRepo.InsertSpanBatch(ctx, spans); err != nil {
		return appErrors.NewInternalError("failed to create span batch", err)
	}

	return nil
}

func (s *TraceService) GetSpan(ctx context.Context, spanID string) (*observability.Span, error) {
	span, err := s.traceRepo.GetSpan(ctx, spanID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appErrors.NewNotFoundError("span " + spanID)
		}
		return nil, appErrors.NewInternalError("failed to get span", err)
	}

	return span, nil
}

func (s *TraceService) GetSpanByProject(ctx context.Context, spanID string, projectID uuid.UUID) (*observability.Span, error) {
	span, err := s.traceRepo.GetSpanByProject(ctx, spanID, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appErrors.NewNotFoundError("span " + spanID)
		}
		return nil, appErrors.NewInternalError("failed to get span by project", err)
	}

	return span, nil
}

func (s *TraceService) GetSpansByFilter(ctx context.Context, filter *observability.SpanFilter) ([]*observability.Span, error) {
	spans, err := s.traceRepo.GetSpansByFilter(ctx, filter)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get spans", err)
	}

	return spans, nil
}

func (s *TraceService) CountSpans(ctx context.Context, filter *observability.SpanFilter) (int64, error) {
	count, err := s.traceRepo.CountSpansByFilter(ctx, filter)
	if err != nil {
		return 0, appErrors.NewInternalError("failed to count spans", err)
	}

	return count, nil
}

func (s *TraceService) GetSpanChildren(ctx context.Context, parentSpanID string) ([]*observability.Span, error) {
	spans, err := s.traceRepo.GetSpanChildren(ctx, parentSpanID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get child spans", err)
	}

	return spans, nil
}

func (s *TraceService) GetTrace(ctx context.Context, traceID string) (*observability.TraceSummary, error) {
	if len(traceID) != 32 {
		return nil, appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	summary, err := s.traceRepo.GetTraceSummary(ctx, traceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appErrors.NewNotFoundError("trace " + traceID)
		}
		return nil, appErrors.NewInternalError("failed to get trace summary", err)
	}

	return summary, nil
}

func (s *TraceService) GetTraceSpans(ctx context.Context, traceID string) ([]*observability.Span, error) {
	if len(traceID) != 32 {
		return nil, appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	spans, err := s.traceRepo.GetSpansByTraceID(ctx, traceID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get spans by trace", err)
	}

	return spans, nil
}

func (s *TraceService) GetTraceTree(ctx context.Context, traceID string) ([]*observability.Span, error) {
	if len(traceID) != 32 {
		return nil, appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	spans, err := s.traceRepo.GetSpanTree(ctx, traceID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get span tree", err)
	}

	return spans, nil
}

func (s *TraceService) GetRootSpan(ctx context.Context, traceID string) (*observability.Span, error) {
	if len(traceID) != 32 {
		return nil, appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	rootSpan, err := s.traceRepo.GetRootSpan(ctx, traceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appErrors.NewNotFoundError("root span for trace " + traceID)
		}
		return nil, appErrors.NewInternalError("failed to get root span", err)
	}

	return rootSpan, nil
}

func (s *TraceService) ListTraces(ctx context.Context, filter *observability.TraceFilter) ([]*observability.TraceSummary, error) {
	if filter == nil {
		return nil, appErrors.NewValidationError("filter is required", "trace filter cannot be nil")
	}
	if filter.ProjectID == uuid.Nil {
		return nil, appErrors.NewValidationError("project_id is required", "filter must include project_id for scoping")
	}

	filter.SetDefaults("trace_start")

	traces, err := s.traceRepo.ListTraces(ctx, filter)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list traces", err)
	}

	return traces, nil
}

func (s *TraceService) CountTraces(ctx context.Context, filter *observability.TraceFilter) (int64, error) {
	if filter == nil {
		return 0, appErrors.NewValidationError("filter is required", "trace filter cannot be nil")
	}

	count, err := s.traceRepo.CountTraces(ctx, filter)
	if err != nil {
		return 0, appErrors.NewInternalError("failed to count traces", err)
	}

	return count, nil
}

func (s *TraceService) GetTracesBySession(ctx context.Context, sessionID string) ([]*observability.TraceSummary, error) {
	if sessionID == "" {
		return nil, appErrors.NewValidationError("session_id is required", "session_id cannot be empty")
	}

	traces, err := s.traceRepo.GetTracesBySessionID(ctx, sessionID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get traces by session", err)
	}

	return traces, nil
}

func (s *TraceService) GetTracesByUser(ctx context.Context, userID string, filter *observability.TraceFilter) ([]*observability.TraceSummary, error) {
	if userID == "" {
		return nil, appErrors.NewValidationError("user_id is required", "user_id cannot be empty")
	}

	traces, err := s.traceRepo.GetTracesByUserID(ctx, userID, filter)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get traces by user", err)
	}

	return traces, nil
}

func (s *TraceService) CalculateTraceCost(ctx context.Context, traceID string) (float64, error) {
	if len(traceID) != 32 {
		return 0, appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	totalCost, err := s.traceRepo.CalculateTotalCost(ctx, traceID)
	if err != nil {
		return 0, appErrors.NewInternalError("failed to calculate trace cost", err)
	}

	return totalCost, nil
}

func (s *TraceService) CalculateTraceTokens(ctx context.Context, traceID string) (uint64, error) {
	if len(traceID) != 32 {
		return 0, appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	totalTokens, err := s.traceRepo.CalculateTotalTokens(ctx, traceID)
	if err != nil {
		return 0, appErrors.NewInternalError("failed to calculate trace tokens", err)
	}

	return totalTokens, nil
}

func (s *TraceService) DeleteSpan(ctx context.Context, spanID string) error {
	_, err := s.traceRepo.GetSpan(ctx, spanID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appErrors.NewNotFoundError("span " + spanID)
		}
		return appErrors.NewInternalError("failed to get span", err)
	}

	if err := s.traceRepo.DeleteSpan(ctx, spanID); err != nil {
		return appErrors.NewInternalError("failed to delete span", err)
	}

	return nil
}

func (s *TraceService) DeleteTrace(ctx context.Context, traceID string) error {
	if len(traceID) != 32 {
		return appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	count, err := s.traceRepo.CountSpansInTrace(ctx, traceID)
	if err != nil {
		return appErrors.NewInternalError("failed to check trace existence", err)
	}
	if count == 0 {
		return appErrors.NewNotFoundError("trace " + traceID)
	}

	if err := s.traceRepo.DeleteTrace(ctx, traceID); err != nil {
		return appErrors.NewInternalError("failed to delete trace", err)
	}

	return nil
}

// UpdateTraceTags updates the tags for a trace.
// Validates that the trace exists and belongs to the specified project before updating.
func (s *TraceService) UpdateTraceTags(ctx context.Context, projectID uuid.UUID, traceID string, tags []string) ([]string, error) {
	if projectID == uuid.Nil {
		return nil, appErrors.NewValidationError("project_id is required", "project_id cannot be empty")
	}
	if len(traceID) != 32 {
		return nil, appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	if len(tags) > observability.MaxTagsPerTrace {
		return nil, appErrors.NewValidationError("too many tags", fmt.Sprintf("maximum %d tags allowed", observability.MaxTagsPerTrace))
	}

	// Verify trace exists and belongs to project
	rootSpan, err := s.traceRepo.GetRootSpanByProject(ctx, traceID, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appErrors.NewNotFoundError("trace " + traceID)
		}
		return nil, appErrors.NewInternalError("failed to verify trace ownership", err)
	}
	if rootSpan == nil || rootSpan.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError("trace " + traceID)
	}

	normalizedTags := observability.NormalizeTags(tags)

	if err := s.traceRepo.UpdateTraceTags(ctx, projectID, traceID, normalizedTags); err != nil {
		s.logger.Error("failed to update trace tags", "trace_id", traceID, "error", err)
		return nil, appErrors.NewInternalError("failed to update tags", err)
	}

	s.logger.Info("trace tags updated", "trace_id", traceID, "project_id", projectID, "tag_count", len(normalizedTags))
	return normalizedTags, nil
}

// UpdateTraceBookmark updates the bookmark status for a trace.
// Validates that the trace exists and belongs to the specified project before updating.
func (s *TraceService) UpdateTraceBookmark(ctx context.Context, projectID uuid.UUID, traceID string, bookmarked bool) error {
	if projectID == uuid.Nil {
		return appErrors.NewValidationError("project_id is required", "project_id cannot be empty")
	}
	if len(traceID) != 32 {
		return appErrors.NewValidationError("invalid trace_id", "OTEL trace_id must be 32 hex characters")
	}

	// Verify trace exists and belongs to project
	rootSpan, err := s.traceRepo.GetRootSpanByProject(ctx, traceID, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appErrors.NewNotFoundError("trace " + traceID)
		}
		return appErrors.NewInternalError("failed to verify trace ownership", err)
	}
	if rootSpan == nil || rootSpan.ProjectID != projectID {
		return appErrors.NewNotFoundError("trace " + traceID)
	}

	if err := s.traceRepo.UpdateTraceBookmark(ctx, projectID, traceID, bookmarked); err != nil {
		s.logger.Error("failed to update trace bookmark", "trace_id", traceID, "error", err)
		return appErrors.NewInternalError("failed to update bookmark", err)
	}

	s.logger.Info("trace bookmark updated", "trace_id", traceID, "project_id", projectID, "bookmarked", bookmarked)
	return nil
}

// GetFilterOptions returns available filter values for the traces filter UI.
// Results are cached for 5 minutes to reduce database load.
func (s *TraceService) GetFilterOptions(ctx context.Context, projectID uuid.UUID) (*observability.TraceFilterOptions, error) {
	if projectID == uuid.Nil {
		return nil, appErrors.NewValidationError("project_id is required", "project_id cannot be empty")
	}

	cacheKey := "filter_options:" + projectID.String()

	s.filterOptionsCacheMu.Lock()
	cached, ok := s.filterOptionsCache.Get(cacheKey)
	s.filterOptionsCacheMu.Unlock()

	if ok {
		if time.Now().Before(cached.expiresAt) {
			return cached.options, nil
		}
		s.filterOptionsCacheMu.Lock()
		s.filterOptionsCache.Remove(cacheKey)
		s.filterOptionsCacheMu.Unlock()
	}

	options, err := s.traceRepo.GetFilterOptions(ctx, projectID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get filter options", err)
	}

	s.filterOptionsCacheMu.Lock()
	s.filterOptionsCache.Add(cacheKey, &filterOptionsCacheEntry{
		options:   options,
		expiresAt: time.Now().Add(filterOptionsCacheTTL),
	})
	s.filterOptionsCacheMu.Unlock()

	return options, nil
}

// InvalidateFilterOptionsCache removes cached filter options for a project.
// Call this when traces are added/deleted to ensure fresh data.
func (s *TraceService) InvalidateFilterOptionsCache(projectID uuid.UUID) {
	cacheKey := "filter_options:" + projectID.String()
	s.filterOptionsCacheMu.Lock()
	s.filterOptionsCache.Remove(cacheKey)
	s.filterOptionsCacheMu.Unlock()
}

// DiscoverAttributes extracts unique attribute keys from span_attributes and resource_attributes.
// This enables dynamic filter UI autocomplete based on actual attribute data.
func (s *TraceService) DiscoverAttributes(ctx context.Context, req *observability.AttributeDiscoveryRequest) (*observability.AttributeDiscoveryResponse, error) {
	if req == nil {
		return nil, appErrors.NewValidationError("request is required", "attribute discovery request cannot be nil")
	}
	if req.ProjectID == uuid.Nil {
		return nil, appErrors.NewValidationError("project_id is required", "project_id cannot be empty")
	}

	response, err := s.traceRepo.DiscoverAttributes(ctx, req)
	if err != nil {
		s.logger.Error("failed to discover attributes",
			"error", err,
			"project_id", req.ProjectID,
			"prefix", req.Prefix,
		)
		return nil, appErrors.NewInternalError("failed to discover attributes", err)
	}

	return response, nil
}

// ListSessions returns paginated sessions aggregated from traces.
// Sessions are identified by session_id attribute on root spans.
func (s *TraceService) ListSessions(ctx context.Context, filter *observability.SessionFilter) ([]*observability.SessionSummary, error) {
	if filter == nil {
		return nil, appErrors.NewValidationError("filter is required", "session filter cannot be nil")
	}
	if filter.ProjectID == uuid.Nil {
		return nil, appErrors.NewValidationError("project_id is required", "filter must include project_id for scoping")
	}

	filter.SetDefaults("last_trace")

	sessions, err := s.traceRepo.ListSessions(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list sessions",
			"error", err,
			"project_id", filter.ProjectID,
		)
		return nil, appErrors.NewInternalError("failed to list sessions", err)
	}

	return sessions, nil
}

// CountSessions returns the total number of sessions matching the filter.
func (s *TraceService) CountSessions(ctx context.Context, filter *observability.SessionFilter) (int64, error) {
	if filter == nil {
		return 0, appErrors.NewValidationError("filter is required", "session filter cannot be nil")
	}
	if filter.ProjectID == uuid.Nil {
		return 0, appErrors.NewValidationError("project_id is required", "filter must include project_id for scoping")
	}

	count, err := s.traceRepo.CountSessions(ctx, filter)
	if err != nil {
		s.logger.Error("failed to count sessions",
			"error", err,
			"project_id", filter.ProjectID,
		)
		return 0, appErrors.NewInternalError("failed to count sessions", err)
	}

	return count, nil
}
