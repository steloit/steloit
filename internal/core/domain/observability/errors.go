package observability

import (
	"errors"
	"fmt"
)

// Domain errors for observability operations
var (
	// Trace errors
	ErrTraceNotFound         = errors.New("trace not found")
	ErrTraceAlreadyExists    = errors.New("trace already exists")
	ErrInvalidTraceID        = errors.New("invalid trace ID")
	ErrExternalTraceIDExists = errors.New("external trace ID already exists")

	// Span errors
	ErrSpanNotFound         = errors.New("span not found")
	ErrSpanAlreadyExists    = errors.New("span already exists")
	ErrInvalidSpanID        = errors.New("invalid span ID")
	ErrSpanTraceNotFound    = errors.New("span trace not found")
	ErrInvalidSpanType      = errors.New("invalid span type")
	ErrSpanAlreadyCompleted = errors.New("span already completed")

	// Quality score errors
	ErrQualityScoreNotFound  = errors.New("quality score not found")
	ErrInvalidQualityScoreID = errors.New("invalid quality score ID")
	ErrInvalidScoreValue     = errors.New("invalid score value")
	ErrInvalidScoreType      = errors.New("invalid score type")
	ErrEvaluatorNotFound     = errors.New("evaluator not found")
	ErrDuplicateQualityScore = errors.New("duplicate quality score for the same trace/span and score name")

	// Model pricing errors
	ErrModelNotFound         = errors.New("model not found")
	ErrInvalidPricingPattern = errors.New("invalid pricing pattern")
	ErrPricingDataIncomplete = errors.New("incomplete pricing data")
	ErrPricingExpired        = errors.New("pricing has expired")
	ErrInvalidPricingData    = errors.New("invalid pricing data")

	// General validation errors
	ErrValidationFailed        = errors.New("validation failed")
	ErrInvalidProjectID        = errors.New("invalid project ID")
	ErrInvalidUserID           = errors.New("invalid user ID")
	ErrInvalidSessionID        = errors.New("invalid session ID")
	ErrUnauthorizedAccess      = errors.New("unauthorized access")
	ErrInsufficientPermissions = errors.New("insufficient permissions")

	// Operation errors
	ErrBatchOperationFailed   = errors.New("batch operation failed")
	ErrConcurrentModification = errors.New("concurrent modification detected")
	ErrResourceLimitExceeded  = errors.New("resource limit exceeded")
	ErrInvalidFilter          = errors.New("invalid filter parameters")
	ErrInvalidPagination      = errors.New("invalid pagination parameters")
)

// Error codes for different types of errors
const (
	// Trace error codes
	ErrCodeTraceNotFound         = "TRACE_NOT_FOUND"
	ErrCodeTraceAlreadyExists    = "TRACE_ALREADY_EXISTS"
	ErrCodeInvalidTraceID        = "INVALID_TRACE_ID"
	ErrCodeExternalTraceIDExists = "EXTERNAL_TRACE_ID_EXISTS"

	// Span error codes
	ErrCodeSpanNotFound         = "SPAN_NOT_FOUND"
	ErrCodeSpanAlreadyExists    = "SPAN_ALREADY_EXISTS"
	ErrCodeInvalidSpanID        = "INVALID_SPAN_ID"
	ErrCodeSpanTraceNotFound    = "SPAN_TRACE_NOT_FOUND"
	ErrCodeInvalidSpanType      = "INVALID_SPAN_TYPE"
	ErrCodeSpanAlreadyCompleted = "SPAN_ALREADY_COMPLETED"
	ErrCodeExternalSpanIDExists = "EXTERNAL_SPAN_ID_EXISTS"
	ErrCodeValidation           = "VALIDATION_ERROR"

	// Quality score error codes
	ErrCodeQualityScoreNotFound  = "QUALITY_SCORE_NOT_FOUND"
	ErrCodeInvalidQualityScoreID = "INVALID_QUALITY_SCORE_ID"
	ErrCodeInvalidScoreValue     = "INVALID_SCORE_VALUE"
	ErrCodeInvalidScoreType      = "INVALID_SCORE_TYPE"
	ErrCodeEvaluatorNotFound     = "EVALUATOR_NOT_FOUND"
	ErrCodeDuplicateQualityScore = "DUPLICATE_QUALITY_SCORE"

	// Model pricing error codes
	ErrCodeModelNotFound         = "MODEL_NOT_FOUND"
	ErrCodeInvalidPricingPattern = "INVALID_PRICING_PATTERN"
	ErrCodePricingDataIncomplete = "PRICING_DATA_INCOMPLETE"
	ErrCodePricingExpired        = "PRICING_EXPIRED"
	ErrCodeInvalidPricingData    = "INVALID_PRICING_DATA"

	// General validation error codes
	ErrCodeValidationFailed        = "VALIDATION_FAILED"
	ErrCodeInvalidProjectID        = "INVALID_PROJECT_ID"
	ErrCodeInvalidUserID           = "INVALID_USER_ID"
	ErrCodeInvalidSessionID        = "INVALID_SESSION_ID"
	ErrCodeUnauthorizedAccess      = "UNAUTHORIZED_ACCESS"
	ErrCodeInsufficientPermissions = "INSUFFICIENT_PERMISSIONS"

	// Operation error codes
	ErrCodeBatchOperationFailed   = "BATCH_OPERATION_FAILED"
	ErrCodeConcurrentModification = "CONCURRENT_MODIFICATION"
	ErrCodeResourceLimitExceeded  = "RESOURCE_LIMIT_EXCEEDED"
	ErrCodeInvalidFilter          = "INVALID_FILTER"
	ErrCodeInvalidPagination      = "INVALID_PAGINATION"
)

// Constructor functions for contextualized errors

func NewTraceNotFoundError(traceID string) error {
	return fmt.Errorf("%w: %s", ErrTraceNotFound, traceID)
}

func NewSpanNotFoundError(spanID string) error {
	return fmt.Errorf("%w: %s", ErrSpanNotFound, spanID)
}

func NewUnauthorizedError(resource string) error {
	return fmt.Errorf("%w: %s", ErrUnauthorizedAccess, resource)
}

func NewInsufficientPermissionsError(operation string) error {
	return fmt.Errorf("%w: %s", ErrInsufficientPermissions, operation)
}

func NewBatchOperationError(operation string, cause error) error {
	return fmt.Errorf("%w: %s: %w", ErrBatchOperationFailed, operation, cause)
}

func NewResourceLimitError(resource string, limit int) error {
	return fmt.Errorf("%w: %s (limit %d)", ErrResourceLimitExceeded, resource, limit)
}

// ValidationError represents a field validation error (used as a DTO by ValidateSpanQueryRequest)
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Classification helpers

func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrTraceNotFound) ||
		errors.Is(err, ErrSpanNotFound) ||
		errors.Is(err, ErrQualityScoreNotFound) ||
		errors.Is(err, ErrEvaluatorNotFound)
}

func IsValidationError(err error) bool {
	return errors.Is(err, ErrValidationFailed) ||
		errors.Is(err, ErrInvalidTraceID) ||
		errors.Is(err, ErrInvalidSpanID) ||
		errors.Is(err, ErrInvalidQualityScoreID) ||
		errors.Is(err, ErrInvalidSpanType) ||
		errors.Is(err, ErrInvalidScoreValue) ||
		errors.Is(err, ErrInvalidScoreType)
}

func IsUnauthorizedError(err error) bool {
	return errors.Is(err, ErrUnauthorizedAccess) ||
		errors.Is(err, ErrInsufficientPermissions)
}

func IsConflictError(err error) bool {
	return errors.Is(err, ErrTraceAlreadyExists) ||
		errors.Is(err, ErrSpanAlreadyExists) ||
		errors.Is(err, ErrExternalTraceIDExists) ||
		errors.Is(err, ErrDuplicateQualityScore) ||
		errors.Is(err, ErrConcurrentModification)
}
