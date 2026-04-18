package observability

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	obsDomain "brokle/internal/core/domain/observability"
)

// validFieldNamePattern enforces that field names only contain safe characters.
// This provides defense in depth against SQL injection via field names.
// Pattern: must start with letter or underscore, followed by letters, digits, underscores, or dots.
var validFieldNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

const maxFieldNameLength = 200

// validateFieldName validates a field name to prevent SQL injection.
// Even though the lexer restricts characters, this provides defense in depth.
func validateFieldName(field string) error {
	if field == "" {
		return obsDomain.ErrInvalidFieldName
	}
	if len(field) > maxFieldNameLength {
		return fmt.Errorf("%w: field name too long (max %d characters)", obsDomain.ErrInvalidFieldName, maxFieldNameLength)
	}
	if !validFieldNamePattern.MatchString(field) {
		return obsDomain.ErrInvalidFieldName
	}
	return nil
}

// escapeAttributeKey escapes single quotes in attribute keys for SQL safety.
// This provides defense in depth even though validateFieldName should reject such input.
func escapeAttributeKey(key string) string {
	return strings.ReplaceAll(key, "'", "''")
}

// SpanQueryBuilder converts filter AST to ClickHouse SQL with parameterized queries.
// Uses PREWHERE optimization for indexed columns to improve query performance.
type SpanQueryBuilder struct {
	paramCount int
}

// NewSpanQueryBuilder creates a new query builder.
func NewSpanQueryBuilder() *SpanQueryBuilder {
	return &SpanQueryBuilder{}
}

// QueryResult contains the built query and its arguments.
type QueryResult struct {
	Query string
	Args  []any
	Count int // number of conditions
}

// BuildQuery generates a parameterized ClickHouse query from a filter AST.
// Uses PREWHERE for indexed columns (project_id, start_time, deleted_at) to improve performance.
func (b *SpanQueryBuilder) BuildQuery(
	node obsDomain.FilterNode,
	projectID string,
	startTime, endTime *time.Time,
	limit, offset int,
) (*QueryResult, error) {
	b.paramCount = 0

	whereClause, args, err := b.buildNode(node)
	if err != nil {
		return nil, err
	}

	// PREWHERE conditions: indexed columns that benefit from early filtering
	prewhereConditions := []string{"project_id = ?", "deleted_at IS NULL"}
	prewhereArgs := []any{projectID}

	if startTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time >= ?")
		prewhereArgs = append(prewhereArgs, *startTime)
	}
	if endTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time <= ?")
		prewhereArgs = append(prewhereArgs, *endTime)
	}

	// WHERE conditions: user filter conditions
	var whereConditions []string
	if whereClause != "" {
		whereConditions = append(whereConditions, "("+whereClause+")")
	}

	allArgs := append(prewhereArgs, args...)
	allArgs = append(allArgs, limit, offset)

	var query string
	if len(whereConditions) > 0 {
		query = fmt.Sprintf(`
			SELECT %s
			FROM otel_traces
			PREWHERE %s
			WHERE %s
			ORDER BY start_time DESC
			LIMIT ? OFFSET ?
		`, obsDomain.SpanSelectFields, strings.Join(prewhereConditions, " AND "), strings.Join(whereConditions, " AND "))
	} else {
		query = fmt.Sprintf(`
			SELECT %s
			FROM otel_traces
			PREWHERE %s
			ORDER BY start_time DESC
			LIMIT ? OFFSET ?
		`, obsDomain.SpanSelectFields, strings.Join(prewhereConditions, " AND "))
	}

	return &QueryResult{
		Query: query,
		Args:  allArgs,
		Count: b.paramCount,
	}, nil
}

// BuildCountQuery generates a COUNT query for pagination.
// Uses PREWHERE for indexed columns (project_id, start_time, deleted_at) to improve performance.
func (b *SpanQueryBuilder) BuildCountQuery(
	node obsDomain.FilterNode,
	projectID string,
	startTime, endTime *time.Time,
) (*QueryResult, error) {
	b.paramCount = 0

	whereClause, args, err := b.buildNode(node)
	if err != nil {
		return nil, err
	}

	// PREWHERE conditions: indexed columns that benefit from early filtering
	prewhereConditions := []string{"project_id = ?", "deleted_at IS NULL"}
	prewhereArgs := []any{projectID}

	if startTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time >= ?")
		prewhereArgs = append(prewhereArgs, *startTime)
	}
	if endTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time <= ?")
		prewhereArgs = append(prewhereArgs, *endTime)
	}

	// WHERE conditions: user filter conditions
	var whereConditions []string
	if whereClause != "" {
		whereConditions = append(whereConditions, "("+whereClause+")")
	}

	allArgs := append(prewhereArgs, args...)

	var query string
	if len(whereConditions) > 0 {
		query = fmt.Sprintf(`
			SELECT count(*) as total
			FROM otel_traces
			PREWHERE %s
			WHERE %s
		`, strings.Join(prewhereConditions, " AND "), strings.Join(whereConditions, " AND "))
	} else {
		query = fmt.Sprintf(`
			SELECT count(*) as total
			FROM otel_traces
			PREWHERE %s
		`, strings.Join(prewhereConditions, " AND "))
	}

	return &QueryResult{
		Query: query,
		Args:  allArgs,
		Count: b.paramCount,
	}, nil
}

// buildNode recursively builds SQL from a filter AST node.
func (b *SpanQueryBuilder) buildNode(node obsDomain.FilterNode) (string, []any, error) {
	if node == nil {
		return "", nil, nil
	}

	switch n := node.(type) {
	case *obsDomain.BinaryNode:
		return b.buildBinaryNode(n)
	case *obsDomain.ConditionNode:
		return b.buildConditionNode(n)
	default:
		return "", nil, fmt.Errorf("unknown node type: %T", node)
	}
}

// buildBinaryNode handles AND/OR binary expressions.
func (b *SpanQueryBuilder) buildBinaryNode(node *obsDomain.BinaryNode) (string, []any, error) {
	// Validate operator to prevent injection via directly created AST nodes
	if node.Operator != obsDomain.LogicAnd && node.Operator != obsDomain.LogicOr {
		return "", nil, fmt.Errorf("invalid logic operator: %s", node.Operator)
	}

	leftSQL, leftArgs, err := b.buildNode(node.Left)
	if err != nil {
		return "", nil, err
	}

	rightSQL, rightArgs, err := b.buildNode(node.Right)
	if err != nil {
		return "", nil, err
	}

	sql := fmt.Sprintf("(%s %s %s)", leftSQL, node.Operator, rightSQL)
	args := append(leftArgs, rightArgs...)

	return sql, args, nil
}

// buildConditionNode converts a single condition to SQL.
func (b *SpanQueryBuilder) buildConditionNode(node *obsDomain.ConditionNode) (string, []any, error) {
	column, err := b.getColumn(node.Field)
	if err != nil {
		return "", nil, err
	}

	switch node.Operator {
	case obsDomain.FilterOpEqual:
		return b.buildComparison(column, "=", node.Value)

	case obsDomain.FilterOpNotEqual:
		return b.buildComparison(column, "!=", node.Value)

	case obsDomain.FilterOpGreaterThan:
		return b.buildNumericComparison(column, node.Field, ">", node.Value)

	case obsDomain.FilterOpLessThan:
		return b.buildNumericComparison(column, node.Field, "<", node.Value)

	case obsDomain.FilterOpGreaterOrEqual:
		return b.buildNumericComparison(column, node.Field, ">=", node.Value)

	case obsDomain.FilterOpLessOrEqual:
		return b.buildNumericComparison(column, node.Field, "<=", node.Value)

	case obsDomain.FilterOpContains:
		return b.buildContains(column, node.Value, false)

	case obsDomain.FilterOpNotContains:
		return b.buildContains(column, node.Value, true)

	case obsDomain.FilterOpIn:
		return b.buildInClause(column, node.Value, false)

	case obsDomain.FilterOpNotIn:
		return b.buildInClause(column, node.Value, true)

	case obsDomain.FilterOpExists:
		return b.buildExists(node.Field, false)

	case obsDomain.FilterOpNotExists:
		return b.buildExists(node.Field, true)

	case obsDomain.FilterOpStartsWith:
		return b.buildStartsWith(column, node.Value)

	case obsDomain.FilterOpEndsWith:
		return b.buildEndsWith(column, node.Value)

	case obsDomain.FilterOpRegex:
		return b.buildRegex(column, node.Value, false)

	case obsDomain.FilterOpNotRegex:
		return b.buildRegex(column, node.Value, true)

	case obsDomain.FilterOpIsEmpty:
		return b.buildIsEmpty(column, false)

	case obsDomain.FilterOpIsNotEmpty:
		return b.buildIsEmpty(column, true)

	case obsDomain.FilterOpSearch:
		return b.buildSearch(column, node.Value)

	default:
		return "", nil, obsDomain.NewUnsupportedOperatorError(string(node.Operator))
	}
}

// getColumn returns the ClickHouse column for a field path.
// It validates the field name to prevent SQL injection and returns an error if invalid.
func (b *SpanQueryBuilder) getColumn(field string) (string, error) {
	if err := validateFieldName(field); err != nil {
		return "", err
	}

	if col := obsDomain.GetMaterializedColumn(field); col != "" {
		return col, nil
	}

	escapedField := escapeAttributeKey(field)

	if strings.HasPrefix(field, "resource.") || strings.HasPrefix(field, "deployment.") {
		return fmt.Sprintf("resource_attributes['%s']", escapedField), nil
	}

	return fmt.Sprintf("span_attributes['%s']", escapedField), nil
}

// buildComparison builds a simple comparison (=, !=).
func (b *SpanQueryBuilder) buildComparison(column, op string, value any) (string, []any, error) {
	b.paramCount++
	return fmt.Sprintf("%s %s ?", column, op), []any{value}, nil
}

// buildNumericComparison builds a numeric comparison with type coercion.
func (b *SpanQueryBuilder) buildNumericComparison(column, field, op string, value any) (string, []any, error) {
	b.paramCount++

	if obsDomain.IsMaterializedColumn(field) {
		return fmt.Sprintf("%s %s ?", column, op), []any{value}, nil
	}

	// toFloat64OrNull handles non-numeric values gracefully for map columns
	return fmt.Sprintf("toFloat64OrNull(%s) %s ?", column, op), []any{value}, nil
}

// buildContains builds a case-insensitive substring search.
// Uses positionCaseInsensitive for efficient ClickHouse substring search.
func (b *SpanQueryBuilder) buildContains(column string, value any, negated bool) (string, []any, error) {
	b.paramCount++

	if negated {
		return fmt.Sprintf("positionCaseInsensitive(%s, ?) = 0", column), []any{value}, nil
	}
	return fmt.Sprintf("positionCaseInsensitive(%s, ?) > 0", column), []any{value}, nil
}

// buildInClause builds an IN clause with array parameter.
func (b *SpanQueryBuilder) buildInClause(column string, value any, negated bool) (string, []any, error) {
	values, ok := value.([]string)
	if !ok {
		return "", nil, obsDomain.ErrInvalidValue
	}

	if len(values) == 0 {
		if negated {
			return "1=1", nil, nil // NOT IN empty set is always true
		}
		return "1=0", nil, nil // IN empty set is always false
	}

	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args[i] = v
		b.paramCount++
	}

	op := "IN"
	if negated {
		op = "NOT IN"
	}

	return fmt.Sprintf("%s %s (%s)", column, op, strings.Join(placeholders, ", ")), args, nil
}

// buildExists builds an EXISTS check using mapContains for efficient ClickHouse existence checks.
func (b *SpanQueryBuilder) buildExists(field string, negated bool) (string, []any, error) {
	if err := validateFieldName(field); err != nil {
		return "", nil, err
	}

	if obsDomain.IsMaterializedColumn(field) {
		col := obsDomain.GetMaterializedColumn(field)
		if negated {
			return fmt.Sprintf("(%s IS NULL OR %s = '')", col, col), nil, nil
		}
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", col, col), nil, nil
	}

	mapName := "span_attributes"
	attrKey := field
	if strings.HasPrefix(field, "resource.") {
		mapName = "resource_attributes"
		attrKey = field
	}

	escapedKey := escapeAttributeKey(attrKey)

	if negated {
		return fmt.Sprintf("NOT mapContains(%s, '%s')", mapName, escapedKey), nil, nil
	}
	return fmt.Sprintf("mapContains(%s, '%s')", mapName, escapedKey), nil, nil
}

// buildStartsWith builds a STARTS WITH condition using ClickHouse's startsWith function.
func (b *SpanQueryBuilder) buildStartsWith(column string, value any) (string, []any, error) {
	b.paramCount++
	return fmt.Sprintf("startsWith(%s, ?)", column), []any{value}, nil
}

// buildEndsWith builds an ENDS WITH condition using ClickHouse's endsWith function.
func (b *SpanQueryBuilder) buildEndsWith(column string, value any) (string, []any, error) {
	b.paramCount++
	return fmt.Sprintf("endsWith(%s, ?)", column), []any{value}, nil
}

// buildRegex builds a REGEX condition using ClickHouse's match function.
// Includes validation to prevent ReDoS attacks.
func (b *SpanQueryBuilder) buildRegex(column string, value any, negated bool) (string, []any, error) {
	pattern, ok := value.(string)
	if !ok {
		return "", nil, obsDomain.ErrInvalidValue
	}

	// Validate regex pattern for safety (prevent ReDoS)
	if err := validateRegexPattern(pattern); err != nil {
		return "", nil, err
	}

	b.paramCount++

	if negated {
		return fmt.Sprintf("NOT match(%s, ?)", column), []any{value}, nil
	}
	return fmt.Sprintf("match(%s, ?)", column), []any{value}, nil
}

// validateRegexPattern validates a regex pattern to prevent ReDoS attacks.
// ClickHouse uses RE2-compatible regex which is safe against catastrophic backtracking,
// but we still limit complexity for defense in depth.
func validateRegexPattern(pattern string) error {
	// Limit pattern length
	if len(pattern) > 500 {
		return fmt.Errorf("regex pattern too long (max 500 characters)")
	}

	// Reject patterns with excessive quantifiers that could indicate problematic patterns
	// Note: ClickHouse uses RE2 which is safe, but this provides defense in depth
	quantifierCount := 0
	for _, ch := range pattern {
		if ch == '*' || ch == '+' || ch == '?' {
			quantifierCount++
		}
	}
	if quantifierCount > 10 {
		return fmt.Errorf("regex pattern too complex (too many quantifiers)")
	}

	return nil
}

// buildIsEmpty builds an IS EMPTY / IS NOT EMPTY condition.
// Checks for NULL or empty string values.
func (b *SpanQueryBuilder) buildIsEmpty(column string, notEmpty bool) (string, []any, error) {
	if notEmpty {
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", column, column), nil, nil
	}
	return fmt.Sprintf("(%s IS NULL OR %s = '')", column, column), nil, nil
}

// buildSearch builds a full-text search condition using case-insensitive substring matching.
// Uses positionCaseInsensitive for efficient ClickHouse text search.
func (b *SpanQueryBuilder) buildSearch(column string, value any) (string, []any, error) {
	b.paramCount++
	// Use positionCaseInsensitive for case-insensitive substring matching
	// This is similar to CONTAINS but optimized for search use cases
	return fmt.Sprintf("positionCaseInsensitive(%s, ?) > 0", column), []any{value}, nil
}

// BuildTextSearchCondition builds a full-text search condition across multiple columns.
// Uses hasToken() for tokenized columns (input_preview, output_preview) which leverages bloom filter indexes.
// Uses positionCaseInsensitive() for ID columns (trace_id, span_id, span_name).
func (b *SpanQueryBuilder) BuildTextSearchCondition(query string, searchTypes []obsDomain.SearchType) (string, []any, error) {
	if query == "" {
		return "", nil, nil
	}

	types := obsDomain.NormalizeSearchTypes(searchTypes)

	var conditions []string
	var args []any

	for _, searchType := range types {
		switch searchType {
		case obsDomain.SearchTypeID:
			for _, col := range obsDomain.SearchableColumns[obsDomain.SearchTypeID] {
				conditions = append(conditions, fmt.Sprintf("positionCaseInsensitive(%s, ?) > 0", col))
				args = append(args, query)
				b.paramCount++
			}
		case obsDomain.SearchTypeContent:
			// Search in content columns using hasToken for tokenized indexes
			// hasToken works with tokenbf_v1 indexes for efficient text search
			for _, col := range obsDomain.SearchableColumns[obsDomain.SearchTypeContent] {
				conditions = append(conditions, fmt.Sprintf("hasToken(%s, ?)", col))
				args = append(args, strings.ToLower(query)) // hasToken is case-sensitive, so lowercase
				b.paramCount++
			}
		case obsDomain.SearchTypeAll:
			for _, col := range obsDomain.SearchableColumns[obsDomain.SearchTypeID] {
				conditions = append(conditions, fmt.Sprintf("positionCaseInsensitive(%s, ?) > 0", col))
				args = append(args, query)
				b.paramCount++
			}
			for _, col := range obsDomain.SearchableColumns[obsDomain.SearchTypeContent] {
				conditions = append(conditions, fmt.Sprintf("hasToken(%s, ?)", col))
				args = append(args, strings.ToLower(query))
				b.paramCount++
			}
		}
	}

	if len(conditions) == 0 {
		return "", nil, nil
	}

	return "(" + strings.Join(conditions, " OR ") + ")", args, nil
}

// BuildQueryWithSearch generates a parameterized ClickHouse query with both filter AST and text search.
// Uses PREWHERE for indexed columns (project_id, start_time, deleted_at) to improve performance.
func (b *SpanQueryBuilder) BuildQueryWithSearch(
	node obsDomain.FilterNode,
	searchQuery string,
	searchTypes []obsDomain.SearchType,
	projectID string,
	startTime, endTime *time.Time,
	limit, offset int,
) (*QueryResult, error) {
	b.paramCount = 0

	// PREWHERE conditions: indexed columns that benefit from early filtering
	prewhereConditions := []string{"project_id = ?", "deleted_at IS NULL"}
	prewhereArgs := []any{projectID}

	// Add time range conditions to PREWHERE (uses partition key)
	if startTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time >= ?")
		prewhereArgs = append(prewhereArgs, *startTime)
	}
	if endTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time <= ?")
		prewhereArgs = append(prewhereArgs, *endTime)
	}

	var whereConditions []string
	var whereArgs []any

	if node != nil {
		whereClause, filterArgs, err := b.buildNode(node)
		if err != nil {
			return nil, err
		}
		if whereClause != "" {
			whereConditions = append(whereConditions, "("+whereClause+")")
			whereArgs = append(whereArgs, filterArgs...)
		}
	}

	if searchQuery != "" {
		searchClause, searchArgs, err := b.BuildTextSearchCondition(searchQuery, searchTypes)
		if err != nil {
			return nil, err
		}
		if searchClause != "" {
			whereConditions = append(whereConditions, searchClause)
			whereArgs = append(whereArgs, searchArgs...)
		}
	}

	allArgs := append(prewhereArgs, whereArgs...)
	allArgs = append(allArgs, limit, offset)

	var query string
	if len(whereConditions) > 0 {
		query = fmt.Sprintf(`
			SELECT %s
			FROM otel_traces
			PREWHERE %s
			WHERE %s
			ORDER BY start_time DESC
			LIMIT ? OFFSET ?
		`, obsDomain.SpanSelectFields, strings.Join(prewhereConditions, " AND "), strings.Join(whereConditions, " AND "))
	} else {
		query = fmt.Sprintf(`
			SELECT %s
			FROM otel_traces
			PREWHERE %s
			ORDER BY start_time DESC
			LIMIT ? OFFSET ?
		`, obsDomain.SpanSelectFields, strings.Join(prewhereConditions, " AND "))
	}

	return &QueryResult{
		Query: query,
		Args:  allArgs,
		Count: b.paramCount,
	}, nil
}

// BuildCountQueryWithSearch generates a COUNT query with both filter AST and text search.
// Uses PREWHERE for indexed columns (project_id, start_time, deleted_at) to improve performance.
func (b *SpanQueryBuilder) BuildCountQueryWithSearch(
	node obsDomain.FilterNode,
	searchQuery string,
	searchTypes []obsDomain.SearchType,
	projectID string,
	startTime, endTime *time.Time,
) (*QueryResult, error) {
	b.paramCount = 0

	// PREWHERE conditions: indexed columns that benefit from early filtering
	prewhereConditions := []string{"project_id = ?", "deleted_at IS NULL"}
	prewhereArgs := []any{projectID}

	// Add time range conditions to PREWHERE (uses partition key)
	if startTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time >= ?")
		prewhereArgs = append(prewhereArgs, *startTime)
	}
	if endTime != nil {
		prewhereConditions = append(prewhereConditions, "start_time <= ?")
		prewhereArgs = append(prewhereArgs, *endTime)
	}

	var whereConditions []string
	var whereArgs []any

	if node != nil {
		whereClause, filterArgs, err := b.buildNode(node)
		if err != nil {
			return nil, err
		}
		if whereClause != "" {
			whereConditions = append(whereConditions, "("+whereClause+")")
			whereArgs = append(whereArgs, filterArgs...)
		}
	}

	if searchQuery != "" {
		searchClause, searchArgs, err := b.BuildTextSearchCondition(searchQuery, searchTypes)
		if err != nil {
			return nil, err
		}
		if searchClause != "" {
			whereConditions = append(whereConditions, searchClause)
			whereArgs = append(whereArgs, searchArgs...)
		}
	}

	allArgs := append(prewhereArgs, whereArgs...)

	var query string
	if len(whereConditions) > 0 {
		query = fmt.Sprintf(`
			SELECT count(*) as total
			FROM otel_traces
			PREWHERE %s
			WHERE %s
		`, strings.Join(prewhereConditions, " AND "), strings.Join(whereConditions, " AND "))
	} else {
		query = fmt.Sprintf(`
			SELECT count(*) as total
			FROM otel_traces
			PREWHERE %s
		`, strings.Join(prewhereConditions, " AND "))
	}

	return &QueryResult{
		Query: query,
		Args:  allArgs,
		Count: b.paramCount,
	}, nil
}
