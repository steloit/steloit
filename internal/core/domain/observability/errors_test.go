package observability

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorWrapping(t *testing.T) {
	err := NewTraceNotFoundError("trace-abc")

	assert.ErrorIs(t, err, ErrTraceNotFound)
	assert.Contains(t, err.Error(), "trace-abc")
	assert.Contains(t, err.Error(), "trace not found")
}

func TestErrorChaining(t *testing.T) {
	dbErr := errors.New("connection timeout")
	batchErr := NewBatchOperationError("bulk insert", dbErr)

	// Must unwrap to both the sentinel AND the original cause
	assert.ErrorIs(t, batchErr, ErrBatchOperationFailed)
	assert.ErrorIs(t, batchErr, dbErr, "cause must be unwrappable via errors.Is")
	assert.Contains(t, batchErr.Error(), "bulk insert")
	assert.Contains(t, batchErr.Error(), "connection timeout")
}

func TestConstructors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		sentinel error
		contains string
	}{
		{"TraceNotFound", NewTraceNotFoundError("t1"), ErrTraceNotFound, "t1"},
		{"SpanNotFound", NewSpanNotFoundError("s1"), ErrSpanNotFound, "s1"},
		{"Unauthorized", NewUnauthorizedError("traces"), ErrUnauthorizedAccess, "traces"},
		{"InsufficientPermissions", NewInsufficientPermissionsError("delete"), ErrInsufficientPermissions, "delete"},
		{"ResourceLimit", NewResourceLimitError("spans", 1000), ErrResourceLimitExceeded, "1000"},
		{"FilterTooComplex", NewFilterTooComplexError(50), ErrFilterTooComplex, "50"},
		{"UnsupportedOperator", NewUnsupportedOperatorError("LIKE"), ErrUnsupportedOperator, "LIKE"},
		{"InvalidFilterSyntax", NewInvalidFilterSyntaxError(5, "unexpected token"), ErrInvalidFilterSyntax, "unexpected token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, tt.err, tt.sentinel)
			assert.Contains(t, tt.err.Error(), tt.contains)
		})
	}
}

func TestClassifiers(t *testing.T) {
	assert.True(t, IsNotFoundError(NewTraceNotFoundError("t1")))
	assert.True(t, IsNotFoundError(NewSpanNotFoundError("s1")))
	assert.False(t, IsNotFoundError(NewUnauthorizedError("x")))

	assert.True(t, IsUnauthorizedError(NewUnauthorizedError("x")))
	assert.True(t, IsUnauthorizedError(NewInsufficientPermissionsError("x")))
	assert.False(t, IsUnauthorizedError(NewTraceNotFoundError("x")))

	assert.True(t, IsConflictError(ErrTraceAlreadyExists))
	assert.True(t, IsConflictError(ErrDuplicateQualityScore))
	assert.False(t, IsConflictError(ErrTraceNotFound))

	assert.True(t, IsValidationError(ErrInvalidTraceID))
	assert.True(t, IsValidationError(ErrInvalidScoreType))
	assert.False(t, IsValidationError(ErrTraceNotFound))
}
