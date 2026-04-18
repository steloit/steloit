package dashboard

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	dashboardDomain "brokle/internal/core/domain/dashboard"
)

// validFieldNamePattern enforces that field names only contain safe characters.
// Defense in depth against SQL injection via field names.
var validFieldNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

const maxFieldNameLength = 200

// WidgetQueryBuilder builds ClickHouse SQL queries from widget query definitions.
// Uses PREWHERE optimization for indexed columns to improve query performance.
type WidgetQueryBuilder struct {
	paramCount int
}

func NewWidgetQueryBuilder() *WidgetQueryBuilder {
	return &WidgetQueryBuilder{}
}

type QueryResult struct {
	Query string
	Args  []any
}

type TimeBucket struct {
	Function   string
	Interval   string
	ColumnName string
}

func (b *WidgetQueryBuilder) BuildWidgetQuery(
	query *dashboardDomain.WidgetQuery,
	projectID string,
	startTime, endTime *time.Time,
) (*QueryResult, error) {
	b.paramCount = 0

	viewDef := dashboardDomain.GetViewDefinition(query.View)
	if viewDef == nil {
		return nil, fmt.Errorf("unknown view type: %s", query.View)
	}

	if len(query.Measures) == 0 {
		return nil, errors.New("at least one measure is required")
	}
	for _, measure := range query.Measures {
		if _, ok := viewDef.Measures[measure]; !ok {
			return nil, fmt.Errorf("unknown measure: %s", measure)
		}
	}

	for _, dim := range query.Dimensions {
		if _, ok := viewDef.Dimensions[dim]; !ok {
			return nil, fmt.Errorf("unknown dimension: %s", dim)
		}
	}

	selectParts := make([]string, 0, len(query.Measures)+len(query.Dimensions))

	// Add time bucket if time dimension is present
	hasTimeDimension := false
	timeBucket := b.determineTimeBucket(startTime, endTime)
	for _, dim := range query.Dimensions {
		dimDef := viewDef.Dimensions[dim]
		if dim == "time" && timeBucket != nil {
			selectParts = append(selectParts, fmt.Sprintf("%s(%s) AS %s", timeBucket.Function, dimDef.SQL, dim))
			hasTimeDimension = true
		} else {
			selectParts = append(selectParts, fmt.Sprintf("%s AS %s", dimDef.SQL, dim))
		}
	}

	for _, measure := range query.Measures {
		measureDef := viewDef.Measures[measure]
		selectParts = append(selectParts, fmt.Sprintf("%s AS %s", measureDef.SQL, measure))
	}

	selectClause := strings.Join(selectParts, ", ")

	// Build PREWHERE conditions (indexed columns)
	prewhereConditions := []string{"project_id = ?"}
	prewhereArgs := []any{projectID}

	if startTime != nil {
		prewhereConditions = append(prewhereConditions, viewDef.TimeColumn+" >= ?")
		prewhereArgs = append(prewhereArgs, *startTime)
	}
	if endTime != nil {
		prewhereConditions = append(prewhereConditions, viewDef.TimeColumn+" <= ?")
		prewhereArgs = append(prewhereArgs, *endTime)
	}

	if viewDef.BaseFilter != "" {
		prewhereConditions = append(prewhereConditions, viewDef.BaseFilter)
	}

	whereConditions := []string{}
	whereArgs := []any{}
	for _, filter := range query.Filters {
		cond, args, err := b.buildFilterCondition(filter, viewDef)
		if err != nil {
			return nil, err
		}
		whereConditions = append(whereConditions, cond)
		whereArgs = append(whereArgs, args...)
	}

	var groupByClause string
	if len(query.Dimensions) > 0 {
		groupByParts := make([]string, len(query.Dimensions))
		for i, dim := range query.Dimensions {
			dimDef := viewDef.Dimensions[dim]
			if dim == "time" && timeBucket != nil {
				groupByParts[i] = fmt.Sprintf("%s(%s)", timeBucket.Function, dimDef.SQL)
			} else {
				groupByParts[i] = dimDef.SQL
			}
		}
		groupByClause = "GROUP BY " + strings.Join(groupByParts, ", ")
	}

	orderByClause, err := b.buildOrderByClause(query, viewDef, hasTimeDimension)
	if err != nil {
		return nil, err
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 1000 // default limit
	}
	if limit > 10000 {
		limit = 10000 // max limit
	}

	allArgs := append(prewhereArgs, whereArgs...)
	allArgs = append(allArgs, limit)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT ")
	queryBuilder.WriteString(selectClause)
	queryBuilder.WriteString("\nFROM ")
	queryBuilder.WriteString(viewDef.Table)
	queryBuilder.WriteString("\nPREWHERE ")
	queryBuilder.WriteString(strings.Join(prewhereConditions, " AND "))

	if len(whereConditions) > 0 {
		queryBuilder.WriteString("\nWHERE ")
		queryBuilder.WriteString(strings.Join(whereConditions, " AND "))
	}

	if groupByClause != "" {
		queryBuilder.WriteString("\n")
		queryBuilder.WriteString(groupByClause)
	}

	if orderByClause != "" {
		queryBuilder.WriteString("\n")
		queryBuilder.WriteString(orderByClause)
	}

	queryBuilder.WriteString("\nLIMIT ?")

	return &QueryResult{
		Query: queryBuilder.String(),
		Args:  allArgs,
	}, nil
}

func (b *WidgetQueryBuilder) buildFilterCondition(
	filter dashboardDomain.QueryFilter,
	viewDef *dashboardDomain.ViewDefinition,
) (string, []any, error) {
	if err := b.validateFieldName(filter.Field); err != nil {
		return "", nil, err
	}

	fieldSQL := filter.Field
	if dim, ok := viewDef.Dimensions[filter.Field]; ok {
		fieldSQL = dim.SQL
	} else if measure, ok := viewDef.Measures[filter.Field]; ok {
		fieldSQL = measure.SQL
	}

	var condition string
	var args []any

	switch filter.Operator {
	case dashboardDomain.FilterOpEqual:
		condition = fieldSQL + " = ?"
		args = []any{filter.Value}
	case dashboardDomain.FilterOpNotEqual:
		condition = fieldSQL + " != ?"
		args = []any{filter.Value}
	case dashboardDomain.FilterOpGreaterThan:
		condition = fieldSQL + " > ?"
		args = []any{filter.Value}
	case dashboardDomain.FilterOpLessThan:
		condition = fieldSQL + " < ?"
		args = []any{filter.Value}
	case dashboardDomain.FilterOpGTE:
		condition = fieldSQL + " >= ?"
		args = []any{filter.Value}
	case dashboardDomain.FilterOpLTE:
		condition = fieldSQL + " <= ?"
		args = []any{filter.Value}
	case dashboardDomain.FilterOpContains:
		condition = fieldSQL + " LIKE ?"
		args = []any{fmt.Sprintf("%%%v%%", filter.Value)}
	case dashboardDomain.FilterOpIn:
		values, ok := filter.Value.([]any)
		if !ok {
			return "", nil, errors.New("IN operator requires an array value")
		}
		placeholders := make([]string, len(values))
		for i := range values {
			placeholders[i] = "?"
		}
		condition = fmt.Sprintf("%s IN (%s)", fieldSQL, strings.Join(placeholders, ", "))
		args = values
	default:
		return "", nil, fmt.Errorf("unsupported operator: %s", filter.Operator)
	}

	return condition, args, nil
}

// buildOrderByClause builds the ORDER BY clause with validation against allowed fields.
// OrderBy must be a valid measure or dimension from the view definition to prevent SQL injection.
func (b *WidgetQueryBuilder) buildOrderByClause(
	query *dashboardDomain.WidgetQuery,
	viewDef *dashboardDomain.ViewDefinition,
	hasTimeDimension bool,
) (string, error) {
	if query.OrderBy != "" {
		// Validate OrderBy is a known measure or dimension
		_, isMeasure := viewDef.Measures[query.OrderBy]
		_, isDimension := viewDef.Dimensions[query.OrderBy]
		if !isMeasure && !isDimension {
			return "", fmt.Errorf("invalid order_by field: %s", query.OrderBy)
		}

		dir := "ASC"
		if query.OrderDir == "desc" {
			dir = "DESC"
		}
		return fmt.Sprintf("ORDER BY %s %s", query.OrderBy, dir), nil
	}

	if hasTimeDimension {
		return "ORDER BY time ASC", nil
	}

	if len(query.Measures) > 0 {
		return fmt.Sprintf("ORDER BY %s DESC", query.Measures[0]), nil
	}

	return "", nil
}

func (b *WidgetQueryBuilder) determineTimeBucket(startTime, endTime *time.Time) *TimeBucket {
	if startTime == nil || endTime == nil {
		// Default to hourly if no time range specified
		return &TimeBucket{
			Function:   "toStartOfHour",
			Interval:   "1 hour",
			ColumnName: "time",
		}
	}

	duration := endTime.Sub(*startTime)

	switch {
	case duration < time.Hour:
		// <1h: 1-minute buckets
		return &TimeBucket{
			Function:   "toStartOfMinute",
			Interval:   "1 minute",
			ColumnName: "time",
		}
	case duration < 24*time.Hour:
		// 1h-24h: 5-minute buckets
		return &TimeBucket{
			Function:   "toStartOfFiveMinutes",
			Interval:   "5 minute",
			ColumnName: "time",
		}
	case duration < 7*24*time.Hour:
		// 1d-7d: 1-hour buckets
		return &TimeBucket{
			Function:   "toStartOfHour",
			Interval:   "1 hour",
			ColumnName: "time",
		}
	default:
		// >7d: 1-day buckets
		return &TimeBucket{
			Function:   "toStartOfDay",
			Interval:   "1 day",
			ColumnName: "time",
		}
	}
}

// validateFieldName validates a field name to prevent SQL injection.
func (b *WidgetQueryBuilder) validateFieldName(field string) error {
	if field == "" {
		return errors.New("field name cannot be empty")
	}
	if len(field) > maxFieldNameLength {
		return fmt.Errorf("field name too long (max %d characters)", maxFieldNameLength)
	}
	if !validFieldNamePattern.MatchString(field) {
		return fmt.Errorf("invalid field name: %s", field)
	}
	return nil
}

func (b *WidgetQueryBuilder) BuildTraceListQuery(
	query *dashboardDomain.WidgetQuery,
	projectID string,
	startTime, endTime *time.Time,
) (*QueryResult, error) {
	b.paramCount = 0
	viewDef := dashboardDomain.TracesViewDefinition()

	selectClause := `
		trace_id,
		span_name AS name,
		start_time,
		duration_nano,
		status_code,
		total_cost,
		model_name,
		provider_name,
		service_name
	`

	prewhereConditions := []string{
		"project_id = ?",
		"parent_span_id IS NULL",
		"deleted_at IS NULL",
	}
	prewhereArgs := []any{projectID}

	if startTime != nil {
		prewhereConditions = append(prewhereConditions, viewDef.TimeColumn+" >= ?")
		prewhereArgs = append(prewhereArgs, *startTime)
	}
	if endTime != nil {
		prewhereConditions = append(prewhereConditions, viewDef.TimeColumn+" <= ?")
		prewhereArgs = append(prewhereArgs, *endTime)
	}

	whereConditions := []string{}
	whereArgs := []any{}
	for _, filter := range query.Filters {
		cond, args, err := b.buildFilterCondition(filter, viewDef)
		if err != nil {
			return nil, err
		}
		whereConditions = append(whereConditions, cond)
		whereArgs = append(whereArgs, args...)
	}

	orderBy := "start_time DESC"
	if query.OrderBy != "" {
		var orderField string
		if measure, ok := viewDef.Measures[query.OrderBy]; ok {
			// For trace list queries (non-aggregated), use BaseColumn instead of aggregate SQL
			if measure.BaseColumn == "" {
				return nil, fmt.Errorf("cannot order trace list by computed measure: %s", query.OrderBy)
			}
			orderField = measure.BaseColumn
		} else if dim, ok := viewDef.Dimensions[query.OrderBy]; ok {
			orderField = dim.SQL
		} else {
			return nil, fmt.Errorf("invalid order_by field: %s", query.OrderBy)
		}
		dir := "DESC"
		if query.OrderDir == "asc" {
			dir = "ASC"
		}
		orderBy = fmt.Sprintf("%s %s", orderField, dir)
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 50 // default for trace list
	}
	if limit > 1000 {
		limit = 1000
	}

	allArgs := append(prewhereArgs, whereArgs...)
	allArgs = append(allArgs, limit)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT ")
	queryBuilder.WriteString(selectClause)
	queryBuilder.WriteString("\nFROM otel_traces")
	queryBuilder.WriteString("\nPREWHERE ")
	queryBuilder.WriteString(strings.Join(prewhereConditions, " AND "))

	if len(whereConditions) > 0 {
		queryBuilder.WriteString("\nWHERE ")
		queryBuilder.WriteString(strings.Join(whereConditions, " AND "))
	}

	queryBuilder.WriteString("\nORDER BY ")
	queryBuilder.WriteString(orderBy)
	queryBuilder.WriteString("\nLIMIT ?")

	return &QueryResult{
		Query: queryBuilder.String(),
		Args:  allArgs,
	}, nil
}

func (b *WidgetQueryBuilder) BuildHistogramQuery(
	query *dashboardDomain.WidgetQuery,
	projectID string,
	startTime, endTime *time.Time,
	bucketCount int,
) (*QueryResult, error) {
	b.paramCount = 0

	if bucketCount <= 0 {
		bucketCount = 20 // default bucket count
	}

	viewDef := dashboardDomain.GetViewDefinition(query.View)
	if viewDef == nil {
		return nil, fmt.Errorf("unknown view type: %s", query.View)
	}

	if len(query.Measures) != 1 {
		return nil, errors.New("histogram requires exactly one measure")
	}

	measure := query.Measures[0]
	measureConfig, ok := viewDef.Measures[measure]
	if !ok {
		return nil, fmt.Errorf("unknown measure: %s", measure)
	}

	histogramColumn := measureConfig.HistogramColumn
	if histogramColumn == "" {
		// Fallback for backward compatibility with measures that don't have HistogramColumn set
		histogramColumn = "duration_nano"
		if strings.Contains(measure, "cost") {
			histogramColumn = "total_cost"
		} else if strings.Contains(measure, "tokens") {
			histogramColumn = "usage_details['input_tokens'] + usage_details['output_tokens']"
		}
	}

	selectClause := fmt.Sprintf(`
		histogram(%d)(%s) AS buckets
	`, bucketCount, histogramColumn)

	prewhereConditions := []string{"project_id = ?"}
	prewhereArgs := []any{projectID}

	if startTime != nil {
		prewhereConditions = append(prewhereConditions, viewDef.TimeColumn+" >= ?")
		prewhereArgs = append(prewhereArgs, *startTime)
	}
	if endTime != nil {
		prewhereConditions = append(prewhereConditions, viewDef.TimeColumn+" <= ?")
		prewhereArgs = append(prewhereArgs, *endTime)
	}

	if viewDef.BaseFilter != "" {
		prewhereConditions = append(prewhereConditions, viewDef.BaseFilter)
	}

	whereConditions := []string{}
	whereArgs := []any{}
	for _, filter := range query.Filters {
		cond, args, err := b.buildFilterCondition(filter, viewDef)
		if err != nil {
			return nil, err
		}
		whereConditions = append(whereConditions, cond)
		whereArgs = append(whereArgs, args...)
	}

	allArgs := append(prewhereArgs, whereArgs...)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT ")
	queryBuilder.WriteString(selectClause)
	queryBuilder.WriteString("\nFROM ")
	queryBuilder.WriteString(viewDef.Table)
	queryBuilder.WriteString("\nPREWHERE ")
	queryBuilder.WriteString(strings.Join(prewhereConditions, " AND "))

	if len(whereConditions) > 0 {
		queryBuilder.WriteString("\nWHERE ")
		queryBuilder.WriteString(strings.Join(whereConditions, " AND "))
	}

	return &QueryResult{
		Query: queryBuilder.String(),
		Args:  allArgs,
	}, nil
}

func (b *WidgetQueryBuilder) BuildVariableOptionsQuery(
	view dashboardDomain.ViewType,
	columnSQL string,
	projectID string,
	limit int,
) *QueryResult {
	viewDef := dashboardDomain.GetViewDefinition(view)
	if viewDef == nil {
		return nil
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT DISTINCT ")
	queryBuilder.WriteString(columnSQL)
	queryBuilder.WriteString(" AS value")
	queryBuilder.WriteString("\nFROM ")
	queryBuilder.WriteString(viewDef.Table)
	queryBuilder.WriteString("\nPREWHERE project_id = ?")

	if viewDef.BaseFilter != "" {
		queryBuilder.WriteString(" AND ")
		queryBuilder.WriteString(viewDef.BaseFilter)
	}

	queryBuilder.WriteString("\nWHERE ")
	queryBuilder.WriteString(columnSQL)
	queryBuilder.WriteString(" IS NOT NULL AND ")
	queryBuilder.WriteString(columnSQL)
	queryBuilder.WriteString(" != ''")

	queryBuilder.WriteString("\nORDER BY value ASC")
	queryBuilder.WriteString("\nLIMIT ?")

	return &QueryResult{
		Query: queryBuilder.String(),
		Args:  []any{projectID, limit},
	}
}
