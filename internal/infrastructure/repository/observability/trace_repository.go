package observability

import (
	"github.com/google/uuid"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"brokle/internal/core/domain/observability"
	"brokle/pkg/pagination"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/shopspring/decimal"
)

type traceRepository struct {
	db clickhouse.Conn
}

func NewTraceRepository(db clickhouse.Conn) observability.TraceRepository {
	return &traceRepository{db: db}
}

func marshalJSON(m map[string]interface{}) string {
	if m == nil || len(m) == 0 {
		return "{}"
	}
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		// Log error but return empty JSON to prevent batch failure
		return "{}"
	}
	return string(jsonBytes)
}

func convertEventsToArrays(events []observability.SpanEvent) (
	timestamps []time.Time,
	names []string,
	attributes []map[string]string,
) {
	if len(events) == 0 {
		return []time.Time{}, []string{}, []map[string]string{}
	}

	timestamps = make([]time.Time, len(events))
	names = make([]string, len(events))
	attributes = make([]map[string]string, len(events))

	for i, event := range events {
		timestamps[i] = event.Timestamp
		names[i] = event.Name
		attrs := make(map[string]string)
		for k, v := range event.Attributes {
			attrs[k] = fmt.Sprint(v)
		}
		attributes[i] = attrs
	}
	return
}

func convertLinksToArrays(links []observability.SpanLink) (
	traceIDs []string,
	spanIDs []string,
	traceStates []string,
	attributes []map[string]string,
) {
	if len(links) == 0 {
		return []string{}, []string{}, []string{}, []map[string]string{}
	}

	traceIDs = make([]string, len(links))
	spanIDs = make([]string, len(links))
	traceStates = make([]string, len(links))
	attributes = make([]map[string]string, len(links))

	for i, link := range links {
		traceIDs[i] = link.TraceID
		spanIDs[i] = link.SpanID
		traceStates[i] = link.TraceState
		attrs := make(map[string]string)
		for k, v := range link.Attributes {
			attrs[k] = fmt.Sprint(v)
		}
		attributes[i] = attrs
	}
	return
}

func convertArraysToEvents(
	timestamps []time.Time,
	names []string,
	attributes []map[string]string,
) []observability.SpanEvent {
	if len(timestamps) == 0 {
		return nil
	}

	events := make([]observability.SpanEvent, len(timestamps))
	for i := range timestamps {
		var attrs map[string]string
		if i < len(attributes) && attributes[i] != nil {
			attrs = attributes[i]
		} else {
			attrs = make(map[string]string)
		}
		events[i] = observability.SpanEvent{
			Timestamp:  timestamps[i],
			Name:       names[i],
			Attributes: attrs,
		}
	}
	return events
}

func convertArraysToLinks(
	traceIDs []string,
	spanIDs []string,
	traceStates []string,
	attributes []map[string]string,
) []observability.SpanLink {
	if len(traceIDs) == 0 {
		return nil
	}

	links := make([]observability.SpanLink, len(traceIDs))
	for i := range traceIDs {
		var attrs map[string]string
		if i < len(attributes) && attributes[i] != nil {
			attrs = attributes[i]
		} else {
			attrs = make(map[string]string)
		}
		links[i] = observability.SpanLink{
			TraceID:    traceIDs[i],
			SpanID:     spanIDs[i],
			TraceState: traceStates[i],
			Attributes: attrs,
		}
	}
	return links
}

func textSearchCondition(filter *observability.TraceFilter) (condition string, args []interface{}) {
	if filter == nil || filter.Search == nil || *filter.Search == "" {
		return "", nil
	}

	searchPattern := "%" + *filter.Search + "%"
	searchType := "all"
	if filter.SearchType != nil {
		searchType = *filter.SearchType
	}

	switch searchType {
	case "id":
		return " AND (trace_id ILIKE ? OR span_id ILIKE ? OR span_name ILIKE ?)",
			[]interface{}{searchPattern, searchPattern, searchPattern}
	case "content":
		return " AND (input ILIKE ? OR output ILIKE ?)",
			[]interface{}{searchPattern, searchPattern}
	default: // "all"
		return " AND (trace_id ILIKE ? OR span_name ILIKE ? OR input ILIKE ? OR output ILIKE ?)",
			[]interface{}{searchPattern, searchPattern, searchPattern, searchPattern}
	}
}

func statusHavingClauses(filter *observability.TraceFilter) (clauses []string, args []interface{}) {
	if filter == nil {
		return nil, nil
	}

	statusToCode := func(s string) int32 {
		switch s {
		case "unset":
			return 0
		case "ok":
			return 1
		case "error":
			return 2
		}
		return -1
	}

	buildInClause := func(statuses []string, not bool) (string, []interface{}) {
		var codes []int32
		for _, s := range statuses {
			if code := statusToCode(s); code >= 0 {
				codes = append(codes, code)
			}
		}
		if len(codes) == 0 {
			return "", nil
		}

		placeholders := make([]string, len(codes))
		clauseArgs := make([]interface{}, len(codes))
		for i, code := range codes {
			placeholders[i] = "?"
			clauseArgs[i] = code
		}

		op := "IN"
		if not {
			op = "NOT IN"
		}
		return fmt.Sprintf("root_status_code %s (%s)", op, strings.Join(placeholders, ",")), clauseArgs
	}

	if len(filter.Statuses) > 0 {
		if clause, clauseArgs := buildInClause(filter.Statuses, false); clause != "" {
			clauses = append(clauses, clause)
			args = append(args, clauseArgs...)
		}
	}

	if len(filter.StatusesNot) > 0 {
		if clause, clauseArgs := buildInClause(filter.StatusesNot, true); clause != "" {
			clauses = append(clauses, clause)
			args = append(args, clauseArgs...)
		}
	}

	return clauses, args
}

func ScanSpanRow(row driver.Row) (*observability.Span, error) {
	var span observability.Span

	var eventsTimestamps []time.Time
	var eventsNames []string
	var eventsAttributes []map[string]string

	var linksTraceIDs []string
	var linksSpanIDs []string
	var linksTraceStates []string
	var linksAttributes []map[string]string

	err := row.Scan(
		&span.SpanID,
		&span.TraceID,
		&span.ParentSpanID,
		&span.TraceState,
		&span.ProjectID,
		&span.SpanName,
		&span.SpanKind,
		&span.StartTime,
		&span.EndTime,
		&span.Duration,
		&span.CompletionStartTime,
		&span.StatusCode,
		&span.StatusMessage,
		&span.HasError,
		&span.Input,
		&span.Output,
		&span.ResourceAttributes,
		&span.SpanAttributes,
		&span.ScopeName,
		&span.ScopeVersion,
		&span.ScopeAttributes,
		&span.ResourceSchemaURL,
		&span.ScopeSchemaURL,
		&span.UsageDetails,
		&span.CostDetails,
		&span.PricingSnapshot,
		&span.TotalCost,
		&eventsTimestamps,
		&eventsNames,
		&eventsAttributes,
		&linksTraceIDs,
		&linksSpanIDs,
		&linksTraceStates,
		&linksAttributes,
		&span.Version,
		&span.DeletedAt,
		&span.ModelName,
		&span.ProviderName,
		&span.SpanType,
		&span.Level,
		&span.ServiceName,
	)

	if err != nil {
		return nil, fmt.Errorf("scan span: %w", err)
	}

	span.Events = convertArraysToEvents(eventsTimestamps, eventsNames, eventsAttributes)
	span.Links = convertArraysToLinks(linksTraceIDs, linksSpanIDs, linksTraceStates, linksAttributes)

	return &span, nil
}

func (r *traceRepository) scanSpans(rows driver.Rows) ([]*observability.Span, error) {
	spans := make([]*observability.Span, 0)

	for rows.Next() {
		var span observability.Span

		var eventsTimestamps []time.Time
		var eventsNames []string
		var eventsAttributes []map[string]string

		var linksTraceIDs []string
		var linksSpanIDs []string
		var linksTraceStates []string
		var linksAttributes []map[string]string

		err := rows.Scan(
			&span.SpanID,
			&span.TraceID,
			&span.ParentSpanID,
			&span.TraceState,
			&span.ProjectID,
			&span.SpanName,
			&span.SpanKind,
			&span.StartTime,
			&span.EndTime,
			&span.Duration,
			&span.CompletionStartTime,
			&span.StatusCode,
			&span.StatusMessage,
			&span.HasError,
			&span.Input,
			&span.Output,
			&span.ResourceAttributes,
			&span.SpanAttributes,
			&span.ScopeName,
			&span.ScopeVersion,
			&span.ScopeAttributes,
			&span.ResourceSchemaURL,
			&span.ScopeSchemaURL,
			&span.UsageDetails,
			&span.CostDetails,
			&span.PricingSnapshot,
			&span.TotalCost,
			&eventsTimestamps,
			&eventsNames,
			&eventsAttributes,
			&linksTraceIDs,
			&linksSpanIDs,
			&linksTraceStates,
			&linksAttributes,
			&span.Version,
			&span.DeletedAt,
			&span.ModelName,
			&span.ProviderName,
			&span.SpanType,
			&span.Level,
			&span.ServiceName,
		)

		if err != nil {
			return nil, fmt.Errorf("scan span: %w", err)
		}

		span.Events = convertArraysToEvents(eventsTimestamps, eventsNames, eventsAttributes)
		span.Links = convertArraysToLinks(linksTraceIDs, linksSpanIDs, linksTraceStates, linksAttributes)

		spans = append(spans, &span)
	}

	return spans, rows.Err()
}

// OTLP spans are immutable - no Update method per OTEL specification
func (r *traceRepository) InsertSpan(ctx context.Context, span *observability.Span) error {
	span.CalculateDuration()

	eventsTimestamps, eventsNames, eventsAttributes := convertEventsToArrays(span.Events)
	linksTraceIDs, linksSpanIDs, linksTraceStates, linksAttributes := convertLinksToArrays(span.Links)

	query := `
		INSERT INTO otel_traces (
			span_id, trace_id, parent_span_id, trace_state, project_id, organization_id,
			span_name, span_kind, start_time, end_time, duration_nano, completion_start_time,
			status_code, status_message,
			input, output,
			resource_attributes, span_attributes, scope_name, scope_version, scope_attributes,
			resource_schema_url, scope_schema_url,
			usage_details, cost_details, pricing_snapshot, total_cost,
			events_timestamp, events_name, events_attributes,
			links_trace_id, links_span_id, links_trace_state, links_attributes,
			deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return r.db.Exec(ctx, query,
		span.SpanID,
		span.TraceID,
		span.ParentSpanID,
		span.TraceState,
		span.ProjectID,
		span.OrganizationID,
		span.SpanName,
		span.SpanKind,
		span.StartTime,
		span.EndTime,
		span.Duration,
		span.CompletionStartTime,
		span.StatusCode,
		span.StatusMessage,
		span.Input,
		span.Output,
		span.ResourceAttributes,
		span.SpanAttributes,
		span.ScopeName,
		span.ScopeVersion,
		span.ScopeAttributes,
		span.ResourceSchemaURL,
		span.ScopeSchemaURL,
		span.UsageDetails,
		span.CostDetails,
		span.PricingSnapshot,
		span.TotalCost,
		eventsTimestamps,
		eventsNames,
		eventsAttributes,
		linksTraceIDs,
		linksSpanIDs,
		linksTraceStates,
		linksAttributes,
		span.DeletedAt,
	)
}

func (r *traceRepository) InsertSpanBatch(ctx context.Context, spans []*observability.Span) error {
	if len(spans) == 0 {
		return nil
	}

	batch, err := r.db.PrepareBatch(ctx, `
		INSERT INTO otel_traces (
			span_id, trace_id, parent_span_id, trace_state, project_id, organization_id,
			span_name, span_kind, start_time, end_time, duration_nano, completion_start_time,
			status_code, status_message,
			input, output,
			resource_attributes, span_attributes, scope_name, scope_version, scope_attributes,
			resource_schema_url, scope_schema_url,
			usage_details, cost_details, pricing_snapshot, total_cost,
			events_timestamp, events_name, events_attributes,
			links_trace_id, links_span_id, links_trace_state, links_attributes,
			deleted_at
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, span := range spans {
		span.CalculateDuration()

		eventsTimestamps, eventsNames, eventsAttributes := convertEventsToArrays(span.Events)
		linksTraceIDs, linksSpanIDs, linksTraceStates, linksAttributes := convertLinksToArrays(span.Links)

		err = batch.Append(
			span.SpanID,
			span.TraceID,
			span.ParentSpanID,
			span.TraceState,
			span.ProjectID,
			span.OrganizationID,
			span.SpanName,
			span.SpanKind,
			span.StartTime,
			span.EndTime,
			span.Duration,
			span.CompletionStartTime,
			span.StatusCode,
			span.StatusMessage,
			span.Input,
			span.Output,
			span.ResourceAttributes,
			span.SpanAttributes,
			span.ScopeName,
			span.ScopeVersion,
			span.ScopeAttributes,
			span.ResourceSchemaURL,
			span.ScopeSchemaURL,
			span.UsageDetails,
			span.CostDetails,
			span.PricingSnapshot,
			span.TotalCost,
			eventsTimestamps,
			eventsNames,
			eventsAttributes,
			linksTraceIDs,
			linksSpanIDs,
			linksTraceStates,
			linksAttributes,
			span.DeletedAt,
		)
		if err != nil {
			return fmt.Errorf("append to batch: %w", err)
		}
	}

	return batch.Send()
}

func (r *traceRepository) DeleteSpan(ctx context.Context, spanID string) error {
	query := `ALTER TABLE otel_traces DELETE WHERE span_id = ?`
	return r.db.Exec(ctx, query, spanID)
}

func (r *traceRepository) GetSpan(ctx context.Context, spanID string) (*observability.Span, error) {
	query := "SELECT " + observability.SpanSelectFields + " FROM otel_traces WHERE span_id = ? AND deleted_at IS NULL LIMIT 1"

	row := r.db.QueryRow(ctx, query, spanID)
	return ScanSpanRow(row)
}

// GetSpanByProject retrieves a span with project ownership validation.
// Returns error if span doesn't exist or doesn't belong to the specified project.
// This prevents cross-project data access when importing spans into datasets.
func (r *traceRepository) GetSpanByProject(ctx context.Context, spanID string, projectID uuid.UUID) (*observability.Span, error) {
	query := "SELECT " + observability.SpanSelectFields + " FROM otel_traces WHERE span_id = ? AND project_id = ? AND deleted_at IS NULL LIMIT 1"

	row := r.db.QueryRow(ctx, query, spanID, projectID)
	return ScanSpanRow(row)
}

func (r *traceRepository) GetSpansByTraceID(ctx context.Context, traceID string) ([]*observability.Span, error) {
	query := "SELECT " + observability.SpanSelectFields + " FROM otel_traces WHERE trace_id = ? AND deleted_at IS NULL ORDER BY start_time ASC"

	rows, err := r.db.Query(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("query spans by trace: %w", err)
	}
	defer rows.Close()

	return r.scanSpans(rows)
}

func (r *traceRepository) GetSpanChildren(ctx context.Context, parentSpanID string) ([]*observability.Span, error) {
	query := "SELECT " + observability.SpanSelectFields + " FROM otel_traces WHERE parent_span_id = ? AND deleted_at IS NULL ORDER BY start_time ASC"
	rows, err := r.db.Query(ctx, query, parentSpanID)
	if err != nil {
		return nil, fmt.Errorf("query child spans: %w", err)
	}
	defer rows.Close()

	return r.scanSpans(rows)
}

func (r *traceRepository) GetSpanTree(ctx context.Context, traceID string) ([]*observability.Span, error) {
	return r.GetSpansByTraceID(ctx, traceID)
}

func (r *traceRepository) GetSpansByFilter(ctx context.Context, filter *observability.SpanFilter) ([]*observability.Span, error) {
	query := `
		SELECT ` + observability.SpanSelectFields + `
		FROM otel_traces
		WHERE 1=1
			AND deleted_at IS NULL
	`

	args := []interface{}{}

	if filter != nil {
		if filter.ProjectID != uuid.Nil {
			query += " AND project_id = ?"
			args = append(args, filter.ProjectID)
		}
		if filter.TraceID != nil {
			query += " AND trace_id = ?"
			args = append(args, *filter.TraceID)
		}
		if filter.ParentID != nil {
			query += " AND parent_span_id = ?"
			args = append(args, *filter.ParentID)
		}
		if filter.Type != nil {
			query += " AND span_type = ?" // Use materialized column
			args = append(args, *filter.Type)
		}
		if filter.SpanKind != nil {
			query += " AND span_kind = ?"
			args = append(args, *filter.SpanKind)
		}
		if filter.Model != nil {
			query += " AND model_name = ?"
			args = append(args, *filter.Model)
		}
		if filter.ServiceName != nil {
			query += " AND service_name = ?"
			args = append(args, *filter.ServiceName)
		}
		if filter.Level != nil {
			query += " AND span_level = ?"
			args = append(args, *filter.Level)
		}
		if filter.StartTime != nil {
			query += " AND start_time >= ?"
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			query += " AND start_time <= ?"
			args = append(args, *filter.EndTime)
		}
		if filter.MinLatencyMs != nil {
			query += " AND duration_nano >= ?"
			args = append(args, uint64(*filter.MinLatencyMs)*1000000)
		}
		if filter.MaxLatencyMs != nil {
			query += " AND duration_nano <= ?"
			args = append(args, uint64(*filter.MaxLatencyMs)*1000000)
		}
		if filter.MinCost != nil {
			query += " AND total_cost >= ?"
			args = append(args, *filter.MinCost)
		}
		if filter.MaxCost != nil {
			query += " AND total_cost <= ?"
			args = append(args, *filter.MaxCost)
		}
		if filter.IsCompleted != nil {
			if *filter.IsCompleted {
				query += " AND end_time IS NOT NULL"
			} else {
				query += " AND end_time IS NULL"
			}
		}
		if len(filter.SpanNames) > 0 {
			placeholders := make([]string, len(filter.SpanNames))
			for i := range filter.SpanNames {
				placeholders[i] = "?"
				args = append(args, filter.SpanNames[i])
			}
			query += " AND span_name IN (" + strings.Join(placeholders, ",") + ")"
		}

	}

	allowedSortFields := []string{"start_time", "end_time", "duration_nano", "span_name", "span_level", "status_code", "span_id"}
	sortField := "start_time" // default
	sortDir := "DESC"

	if filter != nil {
		if filter.Params.SortBy != "" {
			validated, err := pagination.ValidateSortField(filter.Params.SortBy, allowedSortFields)
			if err != nil {
				return nil, fmt.Errorf("invalid sort field: %w", err)
			}
			if validated != "" {
				sortField = validated
			}
		}
		if filter.Params.SortDir == "asc" {
			sortDir = "ASC"
		}
	}

	query += fmt.Sprintf(" ORDER BY %s %s, span_id %s", sortField, sortDir, sortDir)

	limit := pagination.DefaultPageSize
	offset := 0
	if filter != nil {
		if filter.Params.Limit > 0 {
			limit = filter.Params.Limit
		}
		offset = filter.Params.GetOffset()
	}
	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query spans by filter: %w", err)
	}
	defer rows.Close()

	return r.scanSpans(rows)
}

func (r *traceRepository) CountSpansByFilter(ctx context.Context, filter *observability.SpanFilter) (int64, error) {
	query := "SELECT count() FROM otel_traces WHERE 1=1 AND deleted_at IS NULL"
	args := []interface{}{}

	if filter != nil {
		if filter.ProjectID != uuid.Nil {
			query += " AND project_id = ?"
			args = append(args, filter.ProjectID)
		}
		if filter.TraceID != nil {
			query += " AND trace_id = ?"
			args = append(args, *filter.TraceID)
		}
		if filter.Type != nil {
			query += " AND span_type = ?" // Use materialized column
			args = append(args, *filter.Type)
		}
		if filter.Level != nil {
			query += " AND span_level = ?" // Use materialized column
			args = append(args, *filter.Level)
		}
		if filter.StartTime != nil {
			query += " AND start_time >= ?"
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			query += " AND start_time <= ?"
			args = append(args, *filter.EndTime)
		}
		if len(filter.SpanNames) > 0 {
			placeholders := make([]string, len(filter.SpanNames))
			for i := range filter.SpanNames {
				placeholders[i] = "?"
				args = append(args, filter.SpanNames[i])
			}
			query += " AND span_name IN (" + strings.Join(placeholders, ",") + ")"
		}
	}

	var count uint64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	return int64(count), err
}

func (r *traceRepository) GetRootSpan(ctx context.Context, traceID string) (*observability.Span, error) {
	query := `
		SELECT ` + observability.SpanSelectFields + `
		FROM otel_traces
		WHERE trace_id = ?
		  AND parent_span_id IS NULL
		  AND deleted_at IS NULL
		LIMIT 1
	`

	row := r.db.QueryRow(ctx, query, traceID)
	return ScanSpanRow(row)
}

// GetRootSpanByProject retrieves the root span of a trace with project ownership validation.
// Returns error if trace doesn't exist or doesn't belong to the specified project.
// This prevents cross-project data access when importing traces into datasets.
func (r *traceRepository) GetRootSpanByProject(ctx context.Context, traceID string, projectID uuid.UUID) (*observability.Span, error) {
	query := `
		SELECT ` + observability.SpanSelectFields + `
		FROM otel_traces
		WHERE trace_id = ?
		  AND project_id = ?
		  AND parent_span_id IS NULL
		  AND deleted_at IS NULL
		LIMIT 1
	`

	row := r.db.QueryRow(ctx, query, traceID, projectID)
	return ScanSpanRow(row)
}

func (r *traceRepository) GetTraceSummary(ctx context.Context, traceID string) (*observability.TraceSummary, error) {
	query := `
		SELECT
			trace_id,
			anyIf(span_id, parent_span_id IS NULL) as root_span_id,
			anyIf(project_id, parent_span_id IS NULL) as root_project_id,
			anyIf(span_name, parent_span_id IS NULL) as root_span_name,
			min(start_time) as trace_start,
			maxOrNull(end_time) as trace_end,
			anyIf(duration_nano, parent_span_id IS NULL) as trace_duration_nano,

			-- Cost and usage aggregations
			toFloat64(sum(total_cost)) as total_cost,
			sum(usage_details['input']) as total_input_tokens,
			sum(usage_details['output']) as total_output_tokens,
			sum(usage_details['total']) as total_tokens,

			-- Span metrics
			toInt64(count()) as span_count,
			toInt64(countIf(has_error = true)) as error_span_count,
			max(has_error) as trace_has_error,
			anyIf(status_code, parent_span_id IS NULL) as root_status_code,

			-- Root span metadata (materialized columns for fast access)
			anyIf(service_name, parent_span_id IS NULL) as root_service_name,
			anyIf(model_name, parent_span_id IS NULL) as root_model_name,
			anyIf(provider_name, parent_span_id IS NULL) as root_provider_name,
			anyIf(span_attributes['user.id'], parent_span_id IS NULL) as root_user_id,
			anyIf(span_attributes['session.id'], parent_span_id IS NULL) as root_session_id,
			anyIf(tags, parent_span_id IS NULL) as tags,
			anyIf(bookmarked, parent_span_id IS NULL) as bookmarked
		FROM otel_traces
		WHERE trace_id = ?
		  AND deleted_at IS NULL
		GROUP BY trace_id
	`

	row := r.db.QueryRow(ctx, query, traceID)

	var summary observability.TraceSummary
	var totalCostFloat float64

	err := row.Scan(
		&summary.TraceID,
		&summary.RootSpanID,
		&summary.ProjectID,
		&summary.Name,
		&summary.StartTime,
		&summary.EndTime,
		&summary.Duration,
		&totalCostFloat,
		&summary.InputTokens,
		&summary.OutputTokens,
		&summary.TotalTokens,
		&summary.SpanCount,
		&summary.ErrorSpanCount,
		&summary.HasError,
		&summary.StatusCode,
		&summary.ServiceName,
		&summary.ModelName,
		&summary.ProviderName,
		&summary.UserID,
		&summary.SessionID,
		&summary.Tags,
		&summary.Bookmarked,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan trace summary: %w", err)
	}

	summary.TotalCost = decimal.NewFromFloat(totalCostFloat)

	return &summary, nil
}

// Attribute filters (user_id, session_id, service_name) use HAVING clause to preserve full trace metrics
func (r *traceRepository) ListTraces(ctx context.Context, filter *observability.TraceFilter) ([]*observability.TraceSummary, error) {
	query := `
		SELECT
			trace_id,
			anyIf(span_id, parent_span_id IS NULL) as root_span_id,
			anyIf(project_id, parent_span_id IS NULL) as root_project_id,
			anyIf(span_name, parent_span_id IS NULL) as root_span_name,
			min(start_time) as trace_start,
			maxOrNull(end_time) as trace_end,
			anyIf(duration_nano, parent_span_id IS NULL) as trace_duration_nano,

			-- Aggregated cost and usage across all spans
			toFloat64(sum(total_cost)) as total_cost,
			sum(usage_details['input']) as input_tokens,
			sum(usage_details['output']) as output_tokens,
			sum(usage_details['total']) as total_tokens,

			-- Aggregated span metrics
			toInt64(count()) as span_count,
			toInt64(countIf(has_error = true)) as error_span_count,
			max(has_error) as trace_has_error,
			anyIf(status_code, parent_span_id IS NULL) as root_status_code,

			-- Root span metadata (use anyIf to get from root span)
			anyIf(service_name, parent_span_id IS NULL) as root_service_name,
			anyIf(model_name, parent_span_id IS NULL) as root_model_name,
			anyIf(provider_name, parent_span_id IS NULL) as root_provider_name,
			anyIf(span_attributes['user.id'], parent_span_id IS NULL) as root_user_id,
			anyIf(span_attributes['session.id'], parent_span_id IS NULL) as root_session_id,
			anyIf(tags, parent_span_id IS NULL) as tags,
			anyIf(bookmarked, parent_span_id IS NULL) as bookmarked
		FROM otel_traces
		WHERE deleted_at IS NULL
	`

	args := []interface{}{}
	havingClauses := []string{}
	havingArgs := []interface{}{}

	if filter != nil {
		if filter.ProjectID != uuid.Nil {
			query += " AND project_id = ?"
			args = append(args, filter.ProjectID)
		}
		if filter.StartTime != nil {
			query += " AND start_time >= ?"
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			query += " AND start_time <= ?"
			args = append(args, *filter.EndTime)
		}

		if filter.UserID != nil {
			havingClauses = append(havingClauses, "root_user_id = ?")
			havingArgs = append(havingArgs, *filter.UserID)
		}
		if filter.SessionID != nil {
			havingClauses = append(havingClauses, "root_session_id = ?")
			havingArgs = append(havingArgs, *filter.SessionID)
		}
		if filter.ServiceName != nil {
			havingClauses = append(havingClauses, "root_service_name = ?")
			havingArgs = append(havingArgs, *filter.ServiceName)
		}
		if filter.StatusCode != nil {
			havingClauses = append(havingClauses, "anyIf(status_code, parent_span_id IS NULL) = ?")
			havingArgs = append(havingArgs, *filter.StatusCode)
		}

		// Advanced filters
		if filter.ModelName != nil {
			havingClauses = append(havingClauses, "root_model_name = ?")
			havingArgs = append(havingArgs, *filter.ModelName)
		}
		if filter.ProviderName != nil {
			havingClauses = append(havingClauses, "root_provider_name = ?")
			havingArgs = append(havingArgs, *filter.ProviderName)
		}
		if filter.MinCost != nil {
			havingClauses = append(havingClauses, "total_cost >= ?")
			havingArgs = append(havingArgs, *filter.MinCost)
		}
		if filter.MaxCost != nil {
			havingClauses = append(havingClauses, "total_cost <= ?")
			havingArgs = append(havingArgs, *filter.MaxCost)
		}
		if filter.MinTokens != nil {
			havingClauses = append(havingClauses, "total_tokens >= ?")
			havingArgs = append(havingArgs, *filter.MinTokens)
		}
		if filter.MaxTokens != nil {
			havingClauses = append(havingClauses, "total_tokens <= ?")
			havingArgs = append(havingArgs, *filter.MaxTokens)
		}
		if filter.MinDuration != nil {
			havingClauses = append(havingClauses, "trace_duration_nano >= ?")
			havingArgs = append(havingArgs, *filter.MinDuration)
		}
		if filter.MaxDuration != nil {
			havingClauses = append(havingClauses, "trace_duration_nano <= ?")
			havingArgs = append(havingArgs, *filter.MaxDuration)
		}
		if filter.HasError != nil && *filter.HasError {
			havingClauses = append(havingClauses, "trace_has_error = ?")
			havingArgs = append(havingArgs, true)
		}

		if condition, searchArgs := textSearchCondition(filter); condition != "" {
			query += condition
			args = append(args, searchArgs...)
		}

		if statusClauses, statusArgs := statusHavingClauses(filter); len(statusClauses) > 0 {
			havingClauses = append(havingClauses, statusClauses...)
			havingArgs = append(havingArgs, statusArgs...)
		}
	}

	query += " GROUP BY trace_id"

	if len(havingClauses) > 0 {
		query += " HAVING " + strings.Join(havingClauses, " AND ")
		args = append(args, havingArgs...)
	}

	allowedSortFields := []string{
		"trace_start", "trace_end", "trace_duration_nano",
		"total_cost", "input_tokens", "output_tokens", "total_tokens",
		"span_count", "error_span_count", "service_name", "model_name",
	}
	sortField := "trace_start"
	sortDir := "DESC"

	if filter != nil {
		if filter.Params.SortBy != "" {
			validated, err := pagination.ValidateSortField(filter.Params.SortBy, allowedSortFields)
			if err != nil {
				return nil, fmt.Errorf("invalid sort field: %w", err)
			}
			if validated != "" {
				sortField = validated
			}
		}
		if filter.Params.SortDir == "asc" {
			sortDir = "ASC"
		}
	}

	// Map API sort field names to SQL column aliases for aggregated columns
	sortField = observability.GetSortFieldAlias(sortField)

	query += fmt.Sprintf(" ORDER BY %s %s, trace_id %s", sortField, sortDir, sortDir)

	if filter != nil && filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)

		offset := filter.GetOffset()
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	} else {
		query += " LIMIT 100" // Default limit
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list traces: %w", err)
	}
	defer rows.Close()

	var traces []*observability.TraceSummary
	for rows.Next() {
		var trace observability.TraceSummary
		var totalCostFloat float64

		err := rows.Scan(
			&trace.TraceID,
			&trace.RootSpanID,
			&trace.ProjectID,
			&trace.Name,
			&trace.StartTime,
			&trace.EndTime,
			&trace.Duration,
			&totalCostFloat,
			&trace.InputTokens,
			&trace.OutputTokens,
			&trace.TotalTokens,
			&trace.SpanCount,
			&trace.ErrorSpanCount,
			&trace.HasError,
			&trace.StatusCode,
			&trace.ServiceName,
			&trace.ModelName,
			&trace.ProviderName,
			&trace.UserID,
			&trace.SessionID,
			&trace.Tags,
			&trace.Bookmarked,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trace: %w", err)
		}

		trace.TotalCost = decimal.NewFromFloat(totalCostFloat)
		traces = append(traces, &trace)
	}

	return traces, nil
}

func (r *traceRepository) CountTraces(ctx context.Context, filter *observability.TraceFilter) (int64, error) {
	innerQuery := `
		SELECT
			trace_id,
			toFloat64(sum(total_cost)) as total_cost,
			sum(usage_details['total']) as total_tokens,
			if(maxOrNull(end_time) IS NOT NULL, toUInt64(maxOrNull(end_time) - min(start_time)), NULL) as trace_duration_nano,
			max(has_error) as trace_has_error,
			anyIf(service_name, parent_span_id IS NULL) as root_service_name,
			anyIf(model_name, parent_span_id IS NULL) as root_model_name,
			anyIf(provider_name, parent_span_id IS NULL) as root_provider_name,
			anyIf(span_attributes['user.id'], parent_span_id IS NULL) as root_user_id,
			anyIf(span_attributes['session.id'], parent_span_id IS NULL) as root_session_id,
			anyIf(status_code, parent_span_id IS NULL) as root_status_code
		FROM otel_traces
		WHERE deleted_at IS NULL
	`

	args := []interface{}{}
	havingClauses := []string{}
	havingArgs := []interface{}{}

	if filter != nil {
		if filter.ProjectID != uuid.Nil {
			innerQuery += " AND project_id = ?"
			args = append(args, filter.ProjectID)
		}
		if filter.StartTime != nil {
			innerQuery += " AND start_time >= ?"
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			innerQuery += " AND start_time <= ?"
			args = append(args, *filter.EndTime)
		}

		if filter.UserID != nil {
			havingClauses = append(havingClauses, "root_user_id = ?")
			havingArgs = append(havingArgs, *filter.UserID)
		}
		if filter.SessionID != nil {
			havingClauses = append(havingClauses, "root_session_id = ?")
			havingArgs = append(havingArgs, *filter.SessionID)
		}
		if filter.ServiceName != nil {
			havingClauses = append(havingClauses, "root_service_name = ?")
			havingArgs = append(havingArgs, *filter.ServiceName)
		}
		if filter.StatusCode != nil {
			havingClauses = append(havingClauses, "anyIf(status_code, parent_span_id IS NULL) = ?")
			havingArgs = append(havingArgs, *filter.StatusCode)
		}

		// Advanced filters
		if filter.ModelName != nil {
			havingClauses = append(havingClauses, "root_model_name = ?")
			havingArgs = append(havingArgs, *filter.ModelName)
		}
		if filter.ProviderName != nil {
			havingClauses = append(havingClauses, "root_provider_name = ?")
			havingArgs = append(havingArgs, *filter.ProviderName)
		}
		if filter.MinCost != nil {
			havingClauses = append(havingClauses, "total_cost >= ?")
			havingArgs = append(havingArgs, *filter.MinCost)
		}
		if filter.MaxCost != nil {
			havingClauses = append(havingClauses, "total_cost <= ?")
			havingArgs = append(havingArgs, *filter.MaxCost)
		}
		if filter.MinTokens != nil {
			havingClauses = append(havingClauses, "total_tokens >= ?")
			havingArgs = append(havingArgs, *filter.MinTokens)
		}
		if filter.MaxTokens != nil {
			havingClauses = append(havingClauses, "total_tokens <= ?")
			havingArgs = append(havingArgs, *filter.MaxTokens)
		}
		if filter.MinDuration != nil {
			havingClauses = append(havingClauses, "trace_duration_nano >= ?")
			havingArgs = append(havingArgs, *filter.MinDuration)
		}
		if filter.MaxDuration != nil {
			havingClauses = append(havingClauses, "trace_duration_nano <= ?")
			havingArgs = append(havingArgs, *filter.MaxDuration)
		}
		if filter.HasError != nil && *filter.HasError {
			havingClauses = append(havingClauses, "trace_has_error = ?")
			havingArgs = append(havingArgs, true)
		}

		if condition, searchArgs := textSearchCondition(filter); condition != "" {
			innerQuery += condition
			args = append(args, searchArgs...)
		}

		if statusClauses, statusArgs := statusHavingClauses(filter); len(statusClauses) > 0 {
			havingClauses = append(havingClauses, statusClauses...)
			havingArgs = append(havingArgs, statusArgs...)
		}
	}

	innerQuery += " GROUP BY trace_id"

	if len(havingClauses) > 0 {
		innerQuery += " HAVING " + strings.Join(havingClauses, " AND ")
		args = append(args, havingArgs...)
	}

	query := "SELECT toInt64(count()) FROM (" + innerQuery + ")"

	var count int64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count traces: %w", err)
	}

	return count, nil
}

func (r *traceRepository) CountSpansInTrace(ctx context.Context, traceID string) (int64, error) {
	query := `
		SELECT count() as span_count
		FROM otel_traces
		WHERE trace_id = ?
		  AND deleted_at IS NULL
	`

	var count int64
	err := r.db.QueryRow(ctx, query, traceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count spans: %w", err)
	}

	return count, nil
}

func (r *traceRepository) DeleteTrace(ctx context.Context, traceID string) error {
	query := `ALTER TABLE otel_traces DELETE WHERE trace_id = ?`
	return r.db.Exec(ctx, query, traceID)
}

// UpdateTraceTags updates the tags for a trace (all spans in the trace).
// Uses ALTER TABLE UPDATE which is ClickHouse's mutation mechanism for updates.
// Tags are normalized (lowercase, trimmed, unique, sorted) before storage.
func (r *traceRepository) UpdateTraceTags(ctx context.Context, projectID uuid.UUID, traceID string, tags []string) error {
	// Normalize tags before storage
	normalized := observability.NormalizeTags(tags)

	query := `
		ALTER TABLE otel_traces
		UPDATE tags = ?
		WHERE project_id = ?
		  AND trace_id = ?
		  AND deleted_at IS NULL
	`
	return r.db.Exec(ctx, query, normalized, projectID, traceID)
}

// UpdateTraceBookmark updates the bookmark status for a trace.
// Uses ALTER TABLE UPDATE which is ClickHouse's mutation mechanism for updates.
func (r *traceRepository) UpdateTraceBookmark(ctx context.Context, projectID uuid.UUID, traceID string, bookmarked bool) error {
	query := `
		ALTER TABLE otel_traces
		UPDATE bookmarked = ?
		WHERE project_id = ?
		  AND trace_id = ?
		  AND deleted_at IS NULL
	`
	return r.db.Exec(ctx, query, bookmarked, projectID, traceID)
}

func (r *traceRepository) GetTracesBySessionID(ctx context.Context, sessionID string) ([]*observability.TraceSummary, error) {
	filter := &observability.TraceFilter{
		SessionID: &sessionID,
	}
	filter.Limit = 1000 // Higher limit for session analytics
	return r.ListTraces(ctx, filter)
}

func (r *traceRepository) GetTracesByUserID(ctx context.Context, userID string, filter *observability.TraceFilter) ([]*observability.TraceSummary, error) {
	if filter == nil {
		filter = &observability.TraceFilter{}
	}
	filter.UserID = &userID
	return r.ListTraces(ctx, filter)
}

func (r *traceRepository) CalculateTotalCost(ctx context.Context, traceID string) (float64, error) {
	query := `
		SELECT sum(total_cost) as total
		FROM otel_traces
		WHERE trace_id = ?
		  AND deleted_at IS NULL
	`

	var total float64
	err := r.db.QueryRow(ctx, query, traceID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate total cost: %w", err)
	}

	return total, nil
}

func (r *traceRepository) CalculateTotalTokens(ctx context.Context, traceID string) (uint64, error) {
	query := `
		SELECT sum(usage_details['total']) as total
		FROM otel_traces
		WHERE trace_id = ?
		  AND deleted_at IS NULL
	`

	var total uint64
	err := r.db.QueryRow(ctx, query, traceID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate total tokens: %w", err)
	}

	return total, nil
}

func (r *traceRepository) GetFilterOptions(ctx context.Context, projectID uuid.UUID) (*observability.TraceFilterOptions, error) {
	query := `
		SELECT
			arrayDistinct(groupArray(root_model_name)) as models,
			arrayDistinct(groupArray(root_provider_name)) as providers,
			arrayDistinct(groupArray(root_service_name)) as services,
			arrayDistinct(groupArray(root_deployment_environment)) as environments,
			arrayDistinct(groupArray(root_user_id)) as users,
			arrayDistinct(groupArray(root_session_id)) as sessions,
			minOrNull(total_cost) as min_cost,
			maxOrNull(total_cost) as max_cost,
			minOrNull(total_tokens) as min_tokens,
			maxOrNull(total_tokens) as max_tokens,
			minOrNull(trace_duration_nano) as min_duration,
			maxOrNull(trace_duration_nano) as max_duration
		FROM (
			SELECT
				trace_id,
				anyIf(model_name, parent_span_id IS NULL) as root_model_name,
				anyIf(provider_name, parent_span_id IS NULL) as root_provider_name,
				anyIf(service_name, parent_span_id IS NULL) as root_service_name,
				anyIf(deployment_environment, parent_span_id IS NULL) as root_deployment_environment,
				anyIf(span_attributes['user.id'], parent_span_id IS NULL) as root_user_id,
				anyIf(span_attributes['session.id'], parent_span_id IS NULL) as root_session_id,
				toFloat64(sum(total_cost)) as total_cost,
				sum(usage_details['total']) as total_tokens,
				if(maxOrNull(end_time) IS NOT NULL, toUInt64(maxOrNull(end_time) - min(start_time)), NULL) as trace_duration_nano
			FROM otel_traces
			WHERE project_id = ? AND deleted_at IS NULL
			GROUP BY trace_id
		)
	`

	var (
		models       []string
		providers    []string
		services     []string
		environments []string
		users        []string
		sessions     []string
		minCost      *float64
		maxCost      *float64
		minTokens    *uint64
		maxTokens    *uint64
		minDuration  *uint64
		maxDuration  *uint64
	)

	err := r.db.QueryRow(ctx, query, projectID).Scan(
		&models,
		&providers,
		&services,
		&environments,
		&users,
		&sessions,
		&minCost,
		&maxCost,
		&minTokens,
		&maxTokens,
		&minDuration,
		&maxDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get filter options: %w", err)
	}

	spanQuery := `
		SELECT
			arrayDistinct(groupArray(span_name)) as span_names,
			arrayDistinct(groupArray(span_type)) as span_types,
			arrayDistinct(groupArray(status_code)) as status_codes
		FROM otel_traces
		WHERE project_id = ? AND deleted_at IS NULL
		LIMIT 1000
	`

	var (
		spanNames   []string
		spanTypes   []string
		statusCodes []int32
	)

	err = r.db.QueryRow(ctx, spanQuery, projectID).Scan(
		&spanNames,
		&spanTypes,
		&statusCodes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get span filter options: %w", err)
	}

	models = filterEmptyStrings(models)
	providers = filterEmptyStrings(providers)
	services = filterEmptyStrings(services)
	environments = filterEmptyStrings(environments)
	users = filterEmptyStrings(users)
	sessions = filterEmptyStrings(sessions)
	spanNames = filterEmptyStrings(spanNames)
	spanTypes = filterEmptyStrings(spanTypes)

	statusCodesInt := make([]int, len(statusCodes))
	for i, code := range statusCodes {
		statusCodesInt[i] = int(code)
	}

	options := &observability.TraceFilterOptions{
		Models:       models,
		Providers:    providers,
		Services:     services,
		Environments: environments,
		Users:        users,
		Sessions:     sessions,
		SpanNames:    spanNames,
		SpanTypes:    spanTypes,
		StatusCodes:  statusCodesInt,
	}

	if minCost != nil && maxCost != nil {
		options.CostRange = &observability.Range{
			Min: *minCost,
			Max: *maxCost,
		}
	}

	if minTokens != nil && maxTokens != nil {
		options.TokenRange = &observability.Range{
			Min: float64(*minTokens),
			Max: float64(*maxTokens),
		}
	}

	if minDuration != nil && maxDuration != nil {
		options.DurationRange = &observability.Range{
			Min: float64(*minDuration),
			Max: float64(*maxDuration),
		}
	}

	return options, nil
}

func filterEmptyStrings(slice []string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

func (r *traceRepository) QuerySpansByExpression(ctx context.Context, query string, args []interface{}) ([]*observability.Span, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query spans by expression: %w", err)
	}
	defer rows.Close()

	return r.scanSpans(rows)
}

func (r *traceRepository) CountSpansByExpression(ctx context.Context, query string, args []interface{}) (int64, error) {
	var count uint64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count spans by expression: %w", err)
	}
	return int64(count), nil
}

func (r *traceRepository) DiscoverAttributes(ctx context.Context, req *observability.AttributeDiscoveryRequest) (*observability.AttributeDiscoveryResponse, error) {
	// Normalize request with defaults
	observability.NormalizeAttributeDiscoveryRequest(req)

	var allAttributes []observability.AttributeKey

	// Query each source separately for cleaner results
	for _, source := range req.Sources {
		attrs, err := r.discoverAttributesFromSource(ctx, req.ProjectID, source, req.Prefix, req.Limit)
		if err != nil {
			return nil, fmt.Errorf("discover attributes from %s: %w", source, err)
		}
		allAttributes = append(allAttributes, attrs...)
	}

	// Get accurate total count across all sources (separate query without LIMIT)
	var totalCount int64
	for _, source := range req.Sources {
		count, err := r.countAttributeKeysFromSource(ctx, req.ProjectID, source, req.Prefix)
		if err != nil {
			return nil, fmt.Errorf("count attributes from %s: %w", source, err)
		}
		totalCount += count
	}

	// Sort by count descending and limit
	allAttributes = sortAndLimitAttributes(allAttributes, req.Limit)

	return &observability.AttributeDiscoveryResponse{
		Attributes: allAttributes,
		TotalCount: totalCount,
	}, nil
}

// discoverAttributesFromSource extracts attribute keys from a single source (span_attributes or resource_attributes).
func (r *traceRepository) discoverAttributesFromSource(ctx context.Context, projectID uuid.UUID, source observability.AttributeSource, prefix string, limit int) ([]observability.AttributeKey, error) {
	columnName := string(source)

	// Build query using mapKeys() and arrayJoin() for efficient key extraction
	// Sample values to infer type - use assumeNotNull to handle empty maps
	query := fmt.Sprintf(`
		SELECT
			key,
			count() as occurrence_count,
			anyIf(%s[key], %s[key] != '') as sample_value
		FROM otel_traces
		ARRAY JOIN mapKeys(%s) AS key
		WHERE project_id = ?
			AND deleted_at IS NULL
	`, columnName, columnName, columnName)

	args := []interface{}{projectID}

	// Add prefix filter if specified
	if prefix != "" {
		query += " AND key LIKE ?"
		args = append(args, prefix+"%")
	}

	query += fmt.Sprintf(`
		GROUP BY key
		ORDER BY occurrence_count DESC
		LIMIT %d
	`, limit)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query attribute keys: %w", err)
	}
	defer rows.Close()

	var attributes []observability.AttributeKey
	for rows.Next() {
		var key string
		var count uint64
		var sampleValue string

		if err := rows.Scan(&key, &count, &sampleValue); err != nil {
			return nil, fmt.Errorf("scan attribute key: %w", err)
		}

		// Skip materialized columns (already have direct column access)
		if _, isMaterialized := observability.MaterializedColumns[key]; isMaterialized {
			continue
		}

		attributes = append(attributes, observability.AttributeKey{
			Key:       key,
			ValueType: inferValueType(sampleValue),
			Source:    source,
			Count:     int64(count),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attribute keys: %w", err)
	}

	return attributes, nil
}

// countAttributeKeysFromSource counts unique attribute keys from a source without LIMIT.
// Used to provide accurate TotalCount for pagination.
// Excludes materialized columns to match the filtering in discoverAttributesFromSource.
func (r *traceRepository) countAttributeKeysFromSource(ctx context.Context, projectID uuid.UUID, source observability.AttributeSource, prefix string) (int64, error) {
	columnName := string(source)

	// Build exclusion list for materialized columns
	excludeKeys := make([]string, 0, len(observability.MaterializedColumns))
	for key := range observability.MaterializedColumns {
		excludeKeys = append(excludeKeys, key)
	}

	query := fmt.Sprintf(`
		SELECT count(DISTINCT key) as total
		FROM otel_traces
		ARRAY JOIN mapKeys(%s) AS key
		WHERE project_id = ?
			AND deleted_at IS NULL
	`, columnName)

	args := []interface{}{projectID}

	// Add NOT IN clause only if there are keys to exclude
	if len(excludeKeys) > 0 {
		placeholders := make([]string, len(excludeKeys))
		for i := range excludeKeys {
			placeholders[i] = "?"
		}
		query += fmt.Sprintf(" AND key NOT IN (%s)", strings.Join(placeholders, ", "))
		for _, key := range excludeKeys {
			args = append(args, key)
		}
	}

	if prefix != "" {
		query += " AND key LIKE ?"
		args = append(args, prefix+"%")
	}

	var total uint64
	if err := r.db.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count attribute keys: %w", err)
	}

	return int64(total), nil
}

// inferValueType attempts to determine the value type from a sample value.
func inferValueType(value string) observability.AttributeValueType {
	if value == "" {
		return observability.AttributeValueTypeString
	}

	// Check for boolean
	if value == "true" || value == "false" {
		return observability.AttributeValueTypeBoolean
	}

	// Check for number (integer or float)
	isNumber := true
	hasDecimal := false
	for i, c := range value {
		if c == '.' && !hasDecimal {
			hasDecimal = true
			continue
		}
		if c == '-' && i == 0 {
			continue
		}
		if c < '0' || c > '9' {
			isNumber = false
			break
		}
	}
	if isNumber && len(value) > 0 {
		return observability.AttributeValueTypeNumber
	}

	// Check for array (JSON array syntax)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return observability.AttributeValueTypeArray
	}

	return observability.AttributeValueTypeString
}

// sortAndLimitAttributes sorts attributes by count descending and limits the result.
func sortAndLimitAttributes(attrs []observability.AttributeKey, limit int) []observability.AttributeKey {
	// Sort by count descending
	for i := 0; i < len(attrs); i++ {
		for j := i + 1; j < len(attrs); j++ {
			if attrs[j].Count > attrs[i].Count {
				attrs[i], attrs[j] = attrs[j], attrs[i]
			}
		}
	}

	// Apply limit
	if len(attrs) > limit {
		attrs = attrs[:limit]
	}

	return attrs
}

// ListSessions returns paginated sessions aggregated from traces.
// Uses ClickHouse GROUP BY for server-side aggregation of traces by session_id.
func (r *traceRepository) ListSessions(ctx context.Context, filter *observability.SessionFilter) ([]*observability.SessionSummary, error) {
	if filter == nil {
		return nil, fmt.Errorf("filter is required")
	}
	if filter.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("project_id is required")
	}

	filter.SetDefaults("last_trace")

	query := `
		SELECT
			session_id,
			toInt64(count(DISTINCT trace_id)) as trace_count,
			min(start_time) as first_trace,
			max(start_time) as last_trace,
			sum(duration_nano) as total_duration,
			sum(usage_details['total']) as total_tokens,
			toFloat64(sum(total_cost)) as total_cost,
			toInt64(countIf(has_error = true)) as error_count,
			arrayDistinct(groupArray(span_attributes['user.id'])) as user_ids
		FROM otel_traces
		WHERE project_id = ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
			AND session_id != ''
	`

	args := []interface{}{filter.ProjectID}

	if filter.StartTime != nil {
		query += " AND start_time >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND start_time <= ?"
		args = append(args, *filter.EndTime)
	}
	if filter.Search != nil && *filter.Search != "" {
		query += " AND session_id ILIKE ?"
		args = append(args, "%"+*filter.Search+"%")
	}
	if filter.UserID != nil && *filter.UserID != "" {
		query += " AND span_attributes['user.id'] = ?"
		args = append(args, *filter.UserID)
	}

	query += " GROUP BY session_id"

	// Sorting
	allowedSortFields := []string{"last_trace", "first_trace", "trace_count", "total_tokens", "total_cost", "total_duration", "error_count"}
	sortField := "last_trace"
	sortDir := "DESC"

	if filter.SortBy != "" {
		validated, err := pagination.ValidateSortField(filter.SortBy, allowedSortFields)
		if err != nil {
			return nil, fmt.Errorf("invalid sort field: %w", err)
		}
		if validated != "" {
			sortField = validated
		}
	}
	if filter.SortDir == "asc" {
		sortDir = "ASC"
	}

	query += fmt.Sprintf(" ORDER BY %s %s, session_id %s", sortField, sortDir, sortDir)

	// Pagination
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.GetOffset()

	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*observability.SessionSummary
	for rows.Next() {
		var session observability.SessionSummary
		var totalCostFloat float64

		err := rows.Scan(
			&session.SessionID,
			&session.TraceCount,
			&session.FirstTrace,
			&session.LastTrace,
			&session.TotalDuration,
			&session.TotalTokens,
			&totalCostFloat,
			&session.ErrorCount,
			&session.UserIDs,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.TotalCost = decimal.NewFromFloat(totalCostFloat)

		// Filter empty user IDs
		session.UserIDs = filterEmptyStrings(session.UserIDs)

		sessions = append(sessions, &session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate sessions: %w", err)
	}

	return sessions, nil
}

// CountSessions returns the total number of sessions matching the filter.
func (r *traceRepository) CountSessions(ctx context.Context, filter *observability.SessionFilter) (int64, error) {
	if filter == nil {
		return 0, fmt.Errorf("filter is required")
	}
	if filter.ProjectID == uuid.Nil {
		return 0, fmt.Errorf("project_id is required")
	}

	innerQuery := `
		SELECT session_id
		FROM otel_traces
		WHERE project_id = ?
			AND parent_span_id IS NULL
			AND deleted_at IS NULL
			AND session_id != ''
	`

	args := []interface{}{filter.ProjectID}

	if filter.StartTime != nil {
		innerQuery += " AND start_time >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		innerQuery += " AND start_time <= ?"
		args = append(args, *filter.EndTime)
	}
	if filter.Search != nil && *filter.Search != "" {
		innerQuery += " AND session_id ILIKE ?"
		args = append(args, "%"+*filter.Search+"%")
	}
	if filter.UserID != nil && *filter.UserID != "" {
		innerQuery += " AND span_attributes['user.id'] = ?"
		args = append(args, *filter.UserID)
	}

	innerQuery += " GROUP BY session_id"

	query := "SELECT toInt64(count()) FROM (" + innerQuery + ")"

	var count int64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count sessions: %w", err)
	}

	return count, nil
}
