package observability

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SpanQueryRequest represents an SDK request for querying spans with filter expressions.
// This enables users to query production telemetry using human-readable filter syntax.
type SpanQueryRequest struct {
	Filter    string     `json:"filter" validate:"required,max=2000"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Limit     int        `json:"limit,omitempty"` // default 100, max 10000
	Page      int        `json:"page,omitempty"`  // 1-indexed page number
}

// SpanQueryResponse represents the response containing queried spans.
type SpanQueryResponse struct {
	Spans      []*Span `json:"spans"`
	TotalCount int64   `json:"total_count"`
	HasMore    bool    `json:"has_more"`
}

// Query request defaults and limits
const (
	SpanQueryDefaultLimit = 100
	SpanQueryMaxLimit     = 10000
	SpanQueryMaxClauses   = 20
	SpanQueryMaxFilterLen = 2000
)

// SpanSelectFields defines the columns selected when querying spans.
// Used by both query builder and repository for consistency (single source of truth).
const SpanSelectFields = `
	span_id, trace_id, parent_span_id, trace_state, project_id,
	span_name, span_kind, start_time, end_time, duration_nano, completion_start_time,
	status_code, status_message, has_error,
	input, output,
	resource_attributes, span_attributes, scope_name, scope_version, scope_attributes,
	resource_schema_url, scope_schema_url,
	usage_details, cost_details, pricing_snapshot, total_cost,
	events_timestamp, events_name, events_attributes,
	links_trace_id, links_span_id, links_trace_state, links_attributes,
	span_version, deleted_at,
	model_name, provider_name, span_type, span_level,
	service_name
`

// FilterNode represents a node in the filter expression AST.
// This interface enables recursive tree structures for complex expressions.
type FilterNode interface {
	isFilterNode() // marker method for type safety
}

// BinaryNode represents a logical operation (AND/OR) between two expressions.
// Supports full parentheses grouping: (a=1 AND b=2) OR c=3
type BinaryNode struct {
	Left     FilterNode
	Right    FilterNode
	Operator LogicOperator
}

func (b *BinaryNode) isFilterNode() {}

// ConditionNode represents a leaf condition in the filter expression.
// Examples: service.name=chatbot, gen_ai.usage.total_tokens>1000
type ConditionNode struct {
	Field    string         // Attribute path: service.name, gen_ai.system
	Operator FilterOperator // Comparison operator
	Value    any    // string, float64, []string (for IN clause)
	Negated  bool           // For NOT EXISTS
}

func (c *ConditionNode) isFilterNode() {}

// LogicOperator represents logical operators for combining conditions.
type LogicOperator string

const (
	LogicAnd LogicOperator = "AND"
	LogicOr  LogicOperator = "OR"
)

// FilterOperator represents comparison operators for filter conditions.
type FilterOperator string

const (
	FilterOpEqual          FilterOperator = "="
	FilterOpNotEqual       FilterOperator = "!="
	FilterOpGreaterThan    FilterOperator = ">"
	FilterOpLessThan       FilterOperator = "<"
	FilterOpGreaterOrEqual FilterOperator = ">="
	FilterOpLessOrEqual    FilterOperator = "<="
	FilterOpContains       FilterOperator = "CONTAINS"
	FilterOpNotContains    FilterOperator = "NOT CONTAINS"
	FilterOpIn             FilterOperator = "IN"
	FilterOpNotIn          FilterOperator = "NOT IN"
	FilterOpExists         FilterOperator = "EXISTS"
	FilterOpNotExists      FilterOperator = "NOT EXISTS"
	FilterOpStartsWith     FilterOperator = "STARTS WITH"
	FilterOpEndsWith       FilterOperator = "ENDS WITH"
	FilterOpRegex          FilterOperator = "REGEX"
	FilterOpNotRegex       FilterOperator = "NOT REGEX"
	FilterOpIsEmpty        FilterOperator = "IS EMPTY"
	FilterOpIsNotEmpty     FilterOperator = "IS NOT EMPTY"
	FilterOpSearch         FilterOperator = "~" // Full-text search operator
)

// IsComparisonOperator returns true for numeric comparison operators.
func (op FilterOperator) IsComparisonOperator() bool {
	switch op {
	case FilterOpGreaterThan, FilterOpLessThan, FilterOpGreaterOrEqual, FilterOpLessOrEqual:
		return true
	}
	return false
}

// IsExistenceOperator returns true for EXISTS/NOT EXISTS operators.
func (op FilterOperator) IsExistenceOperator() bool {
	return op == FilterOpExists || op == FilterOpNotExists
}

// RequiresValue returns true if the operator requires a value operand.
func (op FilterOperator) RequiresValue() bool {
	return !op.IsExistenceOperator()
}

// AttributeKeyType indicates where an attribute is stored.
type AttributeKeyType string

const (
	AttributeKeyTypeSpan     AttributeKeyType = "span"     // span_attributes map
	AttributeKeyTypeResource AttributeKeyType = "resource" // resource_attributes map
	AttributeKeyTypeColumn   AttributeKeyType = "column"   // Materialized column
)

// MaterializedColumns maps semantic attribute names to ClickHouse column names.
// These columns are pre-computed for O(1) lookup performance.
var MaterializedColumns = map[string]string{
	"service.name":         "service_name",
	"gen_ai.request.model": "model_name",
	"gen_ai.system":        "provider_name",
	"gen_ai.provider.name": "provider_name",
	"brokle.span.type":     "span_type",
	"brokle.span.version":  "span_version",
	"user.id":              "user_id",
	"session.id":           "session_id",
	"span.name":            "span_name",
	"trace.id":             "trace_id",
	"span.id":              "span_id",
	"status.code":          "status_code",
}

// GetMaterializedColumn returns the ClickHouse column name if the attribute is materialized.
// Returns empty string if not materialized.
func GetMaterializedColumn(attrPath string) string {
	return MaterializedColumns[attrPath]
}

// IsMaterializedColumn returns true if the attribute path maps to a materialized column.
func IsMaterializedColumn(attrPath string) bool {
	_, ok := MaterializedColumns[attrPath]
	return ok
}

// SortFieldAliases maps API sort field names to SQL column aliases.
// Used for aggregated columns in GROUP BY queries where the SQL alias
// differs from the API field name (e.g., model_name -> root_model_name).
var SortFieldAliases = map[string]string{
	"model_name":   "root_model_name",
	"service_name": "root_service_name",
}

// GetSortFieldAlias returns the SQL column alias for a sort field.
// If no alias exists, returns the original field name unchanged.
func GetSortFieldAlias(field string) string {
	if alias, ok := SortFieldAliases[field]; ok {
		return alias
	}
	return field
}

var (
	// Parser errors
	ErrInvalidFilterSyntax   = errors.New("invalid filter syntax")
	ErrUnsupportedOperator   = errors.New("unsupported operator")
	ErrInvalidAttributePath  = errors.New("invalid attribute path")
	ErrFilterTooComplex      = errors.New("filter too complex")
	ErrUnexpectedToken       = errors.New("unexpected token")
	ErrUnclosedParenthesis   = errors.New("unclosed parenthesis")
	ErrUnexpectedEndOfInput  = errors.New("unexpected end of input")
	ErrInvalidValue          = errors.New("invalid value")
	ErrEmptyFilter           = errors.New("empty filter expression")
	ErrFilterTooLong         = errors.New("filter expression too long")
	ErrTooManyClauses        = errors.New("too many filter clauses")
	ErrInvalidNumericValue   = errors.New("invalid numeric value")
	ErrInvalidStringValue    = errors.New("invalid string value")
	ErrMissingOperator       = errors.New("missing operator")
	ErrMissingValue          = errors.New("missing value")
	ErrInvalidInClause       = errors.New("invalid IN clause syntax")
	ErrNestedParensTooDeep   = errors.New("nested parentheses too deep")
	ErrInvalidFieldName      = errors.New("invalid field name")
	ErrReservedKeywordAsName = errors.New("reserved keyword used as field name")

	// Query execution errors
	ErrQueryTimeout        = errors.New("query execution timeout")
	ErrResultLimitExceeded = errors.New("result limit exceeded")
)

// Error codes for span query operations
const (
	ErrCodeInvalidFilterSyntax  = "INVALID_FILTER_SYNTAX"
	ErrCodeUnsupportedOperator  = "UNSUPPORTED_OPERATOR"
	ErrCodeInvalidAttributePath = "INVALID_ATTRIBUTE_PATH"
	ErrCodeFilterTooComplex     = "FILTER_TOO_COMPLEX"
	ErrCodeQueryTimeout         = "QUERY_TIMEOUT"
	ErrCodeResultLimitExceeded  = "RESULT_LIMIT_EXCEEDED"
)

// NewInvalidFilterSyntaxError creates a detailed filter syntax error.
func NewInvalidFilterSyntaxError(position int, detail string) error {
	return fmt.Errorf("%w: position %d: %s", ErrInvalidFilterSyntax, position, detail)
}

// NewFilterTooComplexError creates a filter complexity error.
func NewFilterTooComplexError(clauseCount int) error {
	return fmt.Errorf("%w: %d clauses (max %d)", ErrFilterTooComplex, clauseCount, SpanQueryMaxClauses)
}

// NewUnsupportedOperatorError creates an unsupported operator error.
func NewUnsupportedOperatorError(operator string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedOperator, operator)
}

// ValidateSpanQueryRequest validates the span query request parameters.
func ValidateSpanQueryRequest(req *SpanQueryRequest) []ValidationError {
	var errs []ValidationError

	// Filter validation
	if req.Filter == "" {
		errs = append(errs, ValidationError{Field: "filter", Message: "filter is required"})
	} else if len(req.Filter) > SpanQueryMaxFilterLen {
		errs = append(errs, ValidationError{Field: "filter", Message: "filter expression too long"})
	}

	// Limit validation
	if req.Limit < 0 {
		errs = append(errs, ValidationError{Field: "limit", Message: "limit must be non-negative"})
	} else if req.Limit > SpanQueryMaxLimit {
		errs = append(errs, ValidationError{Field: "limit", Message: "limit exceeds maximum allowed"})
	}

	// Page validation
	if req.Page < 1 {
		errs = append(errs, ValidationError{Field: "page", Message: "page must be >= 1"})
	}

	// Time range validation
	if req.StartTime != nil && req.EndTime != nil {
		if req.EndTime.Before(*req.StartTime) {
			errs = append(errs, ValidationError{Field: "end_time", Message: "end_time must be after start_time"})
		}
	}

	return errs
}

// NormalizeSpanQueryRequest applies defaults to the request.
func NormalizeSpanQueryRequest(req *SpanQueryRequest) {
	if req.Limit == 0 {
		req.Limit = SpanQueryDefaultLimit
	}
	if req.Limit > SpanQueryMaxLimit {
		req.Limit = SpanQueryMaxLimit
	}
}

// SearchType specifies which fields to search in for full-text search.
type SearchType string

const (
	// SearchTypeID searches in trace_id, span_id, and span_name fields.
	SearchTypeID SearchType = "id"
	// SearchTypeContent searches in input/output preview fields using tokenized indexes.
	SearchTypeContent SearchType = "content"
	// SearchTypeAll searches in all searchable text fields.
	SearchTypeAll SearchType = "all"
)

// TextSearchRequest represents a full-text search request.
type TextSearchRequest struct {
	Query       string       `json:"query" validate:"required,min=1,max=500"`
	SearchTypes []SearchType `json:"search_types,omitempty"` // defaults to ["all"]
}

// ValidSearchTypes returns true if all search types are valid.
func ValidSearchTypes(types []SearchType) bool {
	validTypes := map[SearchType]bool{
		SearchTypeID:      true,
		SearchTypeContent: true,
		SearchTypeAll:     true,
	}
	for _, t := range types {
		if !validTypes[t] {
			return false
		}
	}
	return true
}

// NormalizeSearchTypes returns a normalized list of search types.
// If empty, defaults to SearchTypeAll.
func NormalizeSearchTypes(types []SearchType) []SearchType {
	if len(types) == 0 {
		return []SearchType{SearchTypeAll}
	}
	return types
}

// SearchableColumns maps search types to their corresponding column names.
var SearchableColumns = map[SearchType][]string{
	SearchTypeID: {
		"trace_id",
		"span_id",
		"span_name",
	},
	SearchTypeContent: {
		"input_preview",
		"output_preview",
	},
}

// AttributeValueType represents the data type of an attribute value.
type AttributeValueType string

const (
	AttributeValueTypeString  AttributeValueType = "string"
	AttributeValueTypeNumber  AttributeValueType = "number"
	AttributeValueTypeBoolean AttributeValueType = "boolean"
	AttributeValueTypeArray   AttributeValueType = "array"
)

// AttributeSource indicates where the attribute comes from.
type AttributeSource string

const (
	AttributeSourceSpan     AttributeSource = "span_attributes"
	AttributeSourceResource AttributeSource = "resource_attributes"
)

// AttributeKey represents a discovered attribute key with metadata.
type AttributeKey struct {
	Key       string             `json:"key"`
	ValueType AttributeValueType `json:"value_type"`
	Source    AttributeSource    `json:"source"`
	Count     int64              `json:"count"`
}

// AttributeDiscoveryRequest represents a request for discovering attribute keys.
type AttributeDiscoveryRequest struct {
	ProjectID uuid.UUID         `json:"project_id"`
	Sources   []AttributeSource `json:"sources,omitempty"` // defaults to all sources
	Prefix    string            `json:"prefix,omitempty"`  // filter by key prefix
	Limit     int               `json:"limit,omitempty"`   // default 100, max 500
}

// AttributeDiscoveryResponse represents the response containing discovered attributes.
type AttributeDiscoveryResponse struct {
	Attributes []AttributeKey `json:"attributes"`
	TotalCount int64          `json:"total_count"`
}

// Attribute discovery limits
const (
	AttributeDiscoveryDefaultLimit = 100
	AttributeDiscoveryMaxLimit     = 500
)

// NormalizeAttributeDiscoveryRequest applies defaults to the request.
func NormalizeAttributeDiscoveryRequest(req *AttributeDiscoveryRequest) {
	if req.Limit <= 0 {
		req.Limit = AttributeDiscoveryDefaultLimit
	}
	if req.Limit > AttributeDiscoveryMaxLimit {
		req.Limit = AttributeDiscoveryMaxLimit
	}
	if len(req.Sources) == 0 {
		req.Sources = []AttributeSource{AttributeSourceSpan, AttributeSourceResource}
	}
}
