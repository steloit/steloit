// Package errors implements Brokle's domain HTTP error type. The shape
// mirrors Stripe / OpenAI / Anthropic — a closed coarse Type (used by SDK
// consumers for retry/alert/error-class branching) plus an open fine Code
// (used for observability and SDK-side specific subclasses):
//
//	{
//	  "type":    "validation_error",   // closed enum, ≤12 values
//	  "code":    "project_not_found",   // open snake_case enum
//	  "message": "...",
//	  "details": "...",
//	  "param":   "projectId"            // optional input field hint
//	}
//
// The HTTP status is a pure function of Type via ErrorType.HTTPStatus —
// never derive status from a stored field, and never let callers
// override it. Framework-level errors (Huma's pre-handler validation,
// content-type negotiation, method routing) are categorised via
// FromHTTPStatus, which uses an explicit map for documented statuses
// plus an RFC 9110 §15.5/§15.6 class fallback so no 4xx is ever
// miscategorised as a 5xx-flavoured Type.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorType is the closed coarse classification of an AppError. The set
// is exhaustive by construction (every AppError carries one) and stable
// (clients can switch on it without a default arm). Adding a value is a
// deliberate API change — update typeToStatus and FromHTTPStatus in the
// same commit.
type ErrorType string

const (
	// TypeInvalidRequest — the request was syntactically valid but cannot
	// be honoured (bad params, unsupported content type, method not
	// allowed). HTTP 400.
	TypeInvalidRequest ErrorType = "invalid_request_error"

	// TypeValidation — the request body parsed but failed declarative
	// validation (struct-tag constraints, custom resolvers). HTTP 422.
	TypeValidation ErrorType = "validation_error"

	// TypeAuthentication — credentials missing, malformed, or rejected.
	// HTTP 401.
	TypeAuthentication ErrorType = "authentication_error"

	// TypePermission — the caller is authenticated but lacks the required
	// permission/role. HTTP 403.
	TypePermission ErrorType = "permission_error"

	// TypeNotFound — the addressed resource does not exist (or is hidden
	// from the caller for tenant-isolation reasons). HTTP 404.
	TypeNotFound ErrorType = "not_found_error"

	// TypeConflict — the requested state change conflicts with current
	// state (duplicate create, optimistic-concurrency mismatch). HTTP 409.
	TypeConflict ErrorType = "conflict_error"

	// TypePaymentRequired — the action requires a billable plan or has
	// exhausted included credits. HTTP 402.
	TypePaymentRequired ErrorType = "payment_required"

	// TypeRateLimit — request rate or quota exceeded. HTTP 429. SDK
	// consumers branch on this to apply Retry-After backoff.
	TypeRateLimit ErrorType = "rate_limit_error"

	// TypeUpstreamProvider — an upstream LLM provider returned an error
	// or failed to respond. HTTP 502. Distinct from TypeServiceUnavailable
	// because the failure is on the provider, not on us.
	TypeUpstreamProvider ErrorType = "upstream_provider_error"

	// TypeServiceUnavailable — the service is temporarily unable to
	// handle the request (maintenance, overload, dependency outage).
	// HTTP 503. SDK consumers retry with exponential backoff.
	TypeServiceUnavailable ErrorType = "service_unavailable"

	// TypeNotImplemented — the operation exists in the schema but has no
	// implementation in this build. HTTP 501.
	TypeNotImplemented ErrorType = "not_implemented"

	// TypeAPIError — catch-all 5xx for unexpected server faults. HTTP
	// 500. Reserve for invariant violations and bugs; prefer a more
	// specific Type when one fits.
	TypeAPIError ErrorType = "api_error"
)

// typeToStatus is the source-of-truth Type → HTTP status mapping. Keep
// this map and the const block above in lockstep — adding a Type without
// a status is a build-time bug we catch via the test in errors_test.go.
var typeToStatus = map[ErrorType]int{
	TypeInvalidRequest:     http.StatusBadRequest,
	TypeValidation:         http.StatusUnprocessableEntity,
	TypeAuthentication:     http.StatusUnauthorized,
	TypePermission:         http.StatusForbidden,
	TypeNotFound:           http.StatusNotFound,
	TypeConflict:           http.StatusConflict,
	TypePaymentRequired:    http.StatusPaymentRequired,
	TypeRateLimit:          http.StatusTooManyRequests,
	TypeUpstreamProvider:   http.StatusBadGateway,
	TypeServiceUnavailable: http.StatusServiceUnavailable,
	TypeNotImplemented:     http.StatusNotImplemented,
	TypeAPIError:           http.StatusInternalServerError,
}

// HTTPStatus returns the canonical HTTP status code for the type.
// Unknown types fall back to 500 rather than panic so a future
// renamed-but-not-mapped value still produces a syntactically valid
// response — the missing entry should be caught by tests.
func (t ErrorType) HTTPStatus() int {
	if s, ok := typeToStatus[t]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// AppError is the canonical HTTP-aware domain error. Construct with one
// of the typed New* helpers below or as a struct literal in tests; Type
// and Message are required, Code defaults to string(Type) on the wire
// when unset.
//
// The struct exists at the package boundary intentionally — domain
// services return *AppError, the response renderer reads its fields
// directly, and tests construct literals. There is no setter API; use
// the variadic Option args on the constructors for the optional fields.
type AppError struct {
	Type    ErrorType
	Code    string
	Message string
	Details string
	Param   string
	Err     error
}

// Error formats the error for log output. Includes Type, Message, and
// the wrapped cause when present. Never written to clients — the
// response renderer pulls Message and Details into the wire envelope
// instead.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s - %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap exposes the underlying cause for errors.Is / errors.As chains.
func (e *AppError) Unwrap() error { return e.Err }

// HTTPStatus returns the canonical HTTP status for the error's Type.
func (e *AppError) HTTPStatus() int { return e.Type.HTTPStatus() }

// GetStatus satisfies huma.StatusError so AppError values returned from
// Huma operation handlers map to the right HTTP status without an
// explicit conversion.
func (e *AppError) GetStatus() int { return e.HTTPStatus() }

// CodeOrType returns the explicit Code, falling back to the Type's
// string form when Code is empty. Used by the wire renderer to ensure
// the on-the-wire `code` field is never empty.
func (e *AppError) CodeOrType() string {
	if e.Code != "" {
		return e.Code
	}
	return string(e.Type)
}

// Is implements errors.Is matching: a target *AppError matches when its
// Type equals the receiver's Type and (when the target's Code is set)
// its Code equals the receiver's Code. This lets callers write either
// of:
//
//	errors.Is(err, &AppError{Type: TypeNotFound})                      // type-only
//	errors.Is(err, &AppError{Type: TypeNotFound, Code: CodeProjectNotFound}) // type + code
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	if t.Type != "" && t.Type != e.Type {
		return false
	}
	if t.Code != "" && t.Code != e.Code {
		return false
	}
	return true
}

// Option modifies an AppError under construction. Functional-options
// pattern — keeps the typed constructor signatures stable as new
// optional fields are added without growing the constructor matrix.
type Option func(*AppError)

// WithCode sets the open-enum domain code (see codes.go for the
// catalogue).
func WithCode(code string) Option {
	return func(e *AppError) { e.Code = code }
}

// WithDetails sets a free-form details string supplementing Message.
// Use lowercase per the established convention (CLAUDE.md gotcha #4):
// `WithDetails("projectId must be a valid UUID")`.
func WithDetails(details string) Option {
	return func(e *AppError) { e.Details = details }
}

// WithParam annotates the error with the input field that triggered
// it. SDK clients use this to render per-field error messages and to
// map failures back to form inputs. Matches OpenAI's `error.param`.
func WithParam(param string) Option {
	return func(e *AppError) { e.Param = param }
}

// WithCause attaches the underlying Go error for log/wrapping. Exposed
// via Unwrap and serialised into Error() but never written to clients.
func WithCause(cause error) Option {
	return func(e *AppError) { e.Err = cause }
}

// New is the low-level constructor. The typed helpers below are
// preferred for the canonical patterns; reach for New only when no
// typed helper fits.
func New(t ErrorType, message string, opts ...Option) *AppError {
	e := &AppError{Type: t, Message: message}
	for _, o := range opts {
		o(e)
	}
	return e
}

// ----- Typed constructors. Existing 50+ call-site signatures preserved
// (CLAUDE.md gotcha #4) — message + (details when applicable) come
// first, optional fields supplied via Option args. -----

// NewValidationError: HTTP 422.
func NewValidationError(message, details string, opts ...Option) *AppError {
	return New(TypeValidation, message, append([]Option{WithDetails(details)}, opts...)...)
}

// NewNotFoundError: HTTP 404. The resource string is folded into the
// human message ("X not found").
func NewNotFoundError(resource string, opts ...Option) *AppError {
	return New(TypeNotFound, resource+" not found", opts...)
}

// NewConflictError: HTTP 409.
func NewConflictError(message string, opts ...Option) *AppError {
	return New(TypeConflict, message, opts...)
}

// NewUnauthorizedError: HTTP 401.
func NewUnauthorizedError(message string, opts ...Option) *AppError {
	return New(TypeAuthentication, message, opts...)
}

// NewForbiddenError: HTTP 403.
func NewForbiddenError(message string, opts ...Option) *AppError {
	return New(TypePermission, message, opts...)
}

// NewBadRequestError: HTTP 400.
func NewBadRequestError(message, details string, opts ...Option) *AppError {
	return New(TypeInvalidRequest, message, append([]Option{WithDetails(details)}, opts...)...)
}

// NewInternalError: HTTP 500. cause is attached via Unwrap so log
// processors and trace exporters see the original error.
func NewInternalError(message string, cause error, opts ...Option) *AppError {
	return New(TypeAPIError, message, append([]Option{WithCause(cause)}, opts...)...)
}

// NewServiceUnavailableError: HTTP 503.
func NewServiceUnavailableError(message string, opts ...Option) *AppError {
	return New(TypeServiceUnavailable, message, opts...)
}

// NewNotImplementedError: HTTP 501.
func NewNotImplementedError(message string, opts ...Option) *AppError {
	return New(TypeNotImplemented, message, opts...)
}

// NewRateLimitError: HTTP 429.
func NewRateLimitError(message string, opts ...Option) *AppError {
	return New(TypeRateLimit, message, opts...)
}

// NewPaymentRequiredError: HTTP 402.
func NewPaymentRequiredError(message string, opts ...Option) *AppError {
	return New(TypePaymentRequired, message, opts...)
}

// NewQuotaExceededError surfaces as TypeRateLimit (HTTP 429) — same
// retry semantics — but carries the distinct Code "quota_exceeded" so
// SDK consumers can render a "you've used your quota" message instead
// of "slow down".
func NewQuotaExceededError(message string, opts ...Option) *AppError {
	return New(TypeRateLimit, message, append([]Option{WithCode(CodeQuotaExceeded)}, opts...)...)
}

// NewUpstreamProviderError marks an error as originating in the
// upstream LLM provider (OpenAI, Anthropic, …) rather than in Brokle.
// HTTP 502.
func NewUpstreamProviderError(message string, cause error, opts ...Option) *AppError {
	return New(TypeUpstreamProvider, message, append([]Option{WithCause(cause)}, opts...)...)
}

// ----- Wrap variants attach an existing Go error as the cause and
// derive Details from cause.Error(). Useful for "we saw this DB error,
// surface it as a validation failure" patterns. -----

func WrapValidationError(cause error, message string, opts ...Option) *AppError {
	return New(TypeValidation, message,
		append([]Option{WithCause(cause), WithDetails(cause.Error())}, opts...)...)
}

func WrapInternalError(cause error, message string, opts ...Option) *AppError {
	return New(TypeAPIError, message, append([]Option{WithCause(cause)}, opts...)...)
}

func WrapUpstreamProviderError(cause error, message string, opts ...Option) *AppError {
	return New(TypeUpstreamProvider, message, append([]Option{WithCause(cause)}, opts...)...)
}

// ----- Inspection helpers, shaped after stdlib `errors`. -----

// AsAppError returns the wrapped *AppError if err is or wraps one,
// nil otherwise. Mirrors errors.As ergonomics for the common single-
// type case:
//
//	if appErr := AsAppError(err); appErr != nil {
//	    switch appErr.Type { … }
//	}
func AsAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return nil
}

// IsAppError preserved for back-compat with the existing handler /
// middleware call sites that use the (value, ok) idiom. New code should
// prefer AsAppError.
func IsAppError(err error) (*AppError, bool) {
	appErr := AsAppError(err)
	return appErr, appErr != nil
}

// HTTPStatus returns the HTTP status implied by err. Falls back to 500
// for non-AppError errors.
func HTTPStatus(err error) int {
	if appErr := AsAppError(err); appErr != nil {
		return appErr.HTTPStatus()
	}
	return http.StatusInternalServerError
}

// IsNotFound reports whether err wraps an AppError of TypeNotFound.
// Used by repository → service mapping to translate sql.ErrNoRows-style
// outcomes into 404 responses.
func IsNotFound(err error) bool {
	appErr := AsAppError(err)
	return appErr != nil && appErr.Type == TypeNotFound
}

// ----- Framework-boundary categorisation -----

// FromHTTPStatus synthesises an AppError from an HTTP status code,
// used by the Huma adapter for framework-level errors that arrive
// without an AppError context (pre-handler validation 422, content
// negotiation 406/415, method routing 405, body size 413, …).
//
// Class-fallback safe: any 4xx not in the explicit map becomes
// TypeInvalidRequest, any 5xx becomes TypeAPIError. This eliminates
// the regression where unmapped framework statuses leaked into the
// server-fault category.
//
// cause is variadic so callers can pass either zero or one originating
// error; only the first non-nil cause is recorded.
func FromHTTPStatus(status int, message string, cause ...error) *AppError {
	e := &AppError{
		Type:    typeFromStatus(status),
		Message: message,
	}
	for _, c := range cause {
		if c != nil {
			e.Err = c
			break
		}
	}
	return e
}

// typeFromStatus is the inverse of typeToStatus, used only at the
// framework boundary where a bare HTTP status arrives without
// AppError context.
func typeFromStatus(status int) ErrorType {
	switch status {
	case http.StatusBadRequest,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusRequestTimeout,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusMisdirectedRequest,
		http.StatusLocked,
		http.StatusFailedDependency,
		http.StatusTooEarly,
		http.StatusUpgradeRequired,
		http.StatusPreconditionRequired,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons:
		return TypeInvalidRequest
	case http.StatusUnauthorized:
		return TypeAuthentication
	case http.StatusPaymentRequired:
		return TypePaymentRequired
	case http.StatusForbidden:
		return TypePermission
	case http.StatusNotFound:
		return TypeNotFound
	case http.StatusConflict:
		return TypeConflict
	case http.StatusUnprocessableEntity:
		return TypeValidation
	case http.StatusTooManyRequests:
		return TypeRateLimit
	case http.StatusNotImplemented:
		return TypeNotImplemented
	case http.StatusBadGateway:
		return TypeUpstreamProvider
	case http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusInsufficientStorage,
		http.StatusLoopDetected:
		return TypeServiceUnavailable
	default:
		if status >= 400 && status < 500 {
			return TypeInvalidRequest
		}
		return TypeAPIError
	}
}
