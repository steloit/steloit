package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// init replaces Huma's default RFC 9457 problem+json error builder
// with a constructor that emits Brokle's APIResponse{success, error,
// meta} envelope. The override matches the success-path envelope so
// SDK consumers parse one shape, not two — the established codebase
// convention (Stripe / GitHub / OpenAI all converge here).
//
// huma.NewError is the documented extension point —
// https://huma.rocks/features/response-errors/. Setting it in init
// keeps the override applied for the entire process lifetime,
// including tests that import this package transitively.
func init() {
	huma.NewError = newHumaError
}

// newHumaError is the huma.NewError replacement. Huma calls it from
// two paths:
//
//  1. A Huma operation handler returns a non-nil error. Huma checks
//     whether the error implements huma.StatusError (our *AppError
//     does via AppError.GetStatus); if so it pulls the status from
//     there and calls newHumaError(status, err.Error(), err) so the
//     original error rides along in errs[0]. We extract the AppError
//     to preserve its Type / Code / Message / Details / Param fields
//     verbatim.
//  2. Huma's pipeline rejects a request before any handler runs
//     (request body too large, content type 415, validation 422,
//     method 405, …). There is no AppError to lift from; we
//     synthesise one via appErrors.FromHTTPStatus, which is class-
//     fallback safe — no 4xx ever leaks to a 5xx-flavoured Type.
//
// In either case the returned value is rendered via
// apiResponseError.MarshalJSON to produce the canonical envelope.
func newHumaError(status int, msg string, errs ...error) huma.StatusError {
	if appErr := firstAppError(errs); appErr != nil {
		return wrapAppError(appErr)
	}
	// Synthesise an AppError for framework errors. Direct field write
	// for Details is intentional — AppError has no setter API; the
	// fields are exported precisely so the framework boundary can fill
	// them without ceremony.
	appErr := appErrors.FromHTTPStatus(status, msg, errs...)
	appErr.Details = joinErrorDetails(errs)
	return wrapAppError(appErr)
}

// wrapAppError lifts an *AppError into the wire envelope. Type / Code
// / Message / Details / Param come from the AppError; Status comes
// from Type via the canonical Type.HTTPStatus mapping (never from a
// stored field).
func wrapAppError(e *appErrors.AppError) *apiResponseError {
	return &apiResponseError{
		status: e.HTTPStatus(),
		body: response.APIResponse{
			Success: false,
			Error: &response.APIError{
				Type:    string(e.Type),
				Code:    e.CodeOrType(),
				Message: e.Message,
				Details: e.Details,
				Param:   e.Param,
			},
		},
	}
}

// apiResponseError is the concrete huma.StatusError implementation
// that renders as an APIResponse envelope. status is kept out of the
// JSON body — it's already on the HTTP response line.
type apiResponseError struct {
	status int
	body   response.APIResponse
}

func (e *apiResponseError) Error() string {
	if e.body.Error != nil {
		return e.body.Error.Message
	}
	return http.StatusText(e.status)
}

// GetStatus satisfies huma.StatusError so Huma writes the right HTTP
// status line.
func (e *apiResponseError) GetStatus() int { return e.status }

// MarshalJSON makes Huma's serializer emit the envelope shape rather
// than the wrapper struct's fields.
func (e *apiResponseError) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.body)
}

// firstAppError returns the first *AppError found in errs, unwrapping
// transparently via errors.As. nil if none.
func firstAppError(errs []error) *appErrors.AppError {
	for _, err := range errs {
		if err == nil {
			continue
		}
		if appErr := appErrors.AsAppError(err); appErr != nil {
			return appErr
		}
	}
	return nil
}

// joinErrorDetails concatenates the Error() strings of all non-nil
// elements of errs with "; ". Returns "" if errs is empty or all-nil.
// Used for native Huma validation errors (a slice of *huma.ErrorDetail
// values) where there is no AppError to lift fields from.
func joinErrorDetails(errs []error) string {
	if len(errs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		s := err.Error()
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}
