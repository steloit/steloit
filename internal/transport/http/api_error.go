package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// init replaces Huma's default RFC 9457 problem+json error builder with a
// constructor that emits Brokle's `APIResponse{success, error, meta}`
// envelope. This keeps the response shape consistent across success and
// error paths, which is the established codebase convention (and matches
// Stripe / GitHub / Twilio's "one envelope" approach).
//
// The function pointer huma.NewError is the documented extension point —
// see https://huma.rocks/features/response-errors/. Setting it in init keeps
// the override applied for the entire process lifetime, including tests.
func init() {
	huma.NewError = newHumaError
}

// newHumaError is the huma.NewError replacement. Huma calls it from two
// paths:
//
//  1. A Huma operation handler returns a non-nil error. Huma checks whether
//     the error implements huma.StatusError (our *AppError does — see
//     pkg/errors.AppError.GetStatus); if so, it pulls the status from
//     there and calls newHumaError(status, err.Error(), err) so the
//     original error is preserved in errs[0].
//  2. Huma's own pipeline rejects a request before the handler runs (e.g.
//     422 validation failure, 415 unsupported media type). It calls
//     newHumaError with a synthetic message and a slice of *huma.ErrorDetail
//     wrapped as errors. We surface those as the `details` field.
//
// In either case the returned value is rendered via the apiResponseError
// MarshalJSON to produce the canonical envelope shape.
func newHumaError(status int, msg string, errs ...error) huma.StatusError {
	apiErr := &response.APIError{
		Code:    codeFromStatus(status),
		Message: msg,
		Type:    typeFromStatus(status),
	}

	// If Huma forwarded our domain *AppError as the originating cause,
	// prefer its richer Code / Type / Details over the status-derived
	// fallbacks above. This is what preserves
	// appErrors.NewValidationError("Invalid project ID", "...") all the way
	// out to the SDK consumer.
	if appErr := firstAppError(errs); appErr != nil {
		apiErr.Code = string(appErr.Type)
		apiErr.Type = string(appErr.Type)
		if appErr.Message != "" {
			apiErr.Message = appErr.Message
		}
		if appErr.Details != "" {
			apiErr.Details = appErr.Details
		}
	} else if details := joinErrorDetails(errs); details != "" {
		// Native Huma validation/decoding errors — surface their messages
		// as a joined details string so SDK consumers get actionable info
		// (which field failed, expected vs. actual, etc.).
		apiErr.Details = details
	}

	return &apiResponseError{
		status: status,
		body: response.APIResponse{
			Success: false,
			Error:   apiErr,
		},
	}
}

// apiResponseError is the concrete huma.StatusError implementation that
// renders as an APIResponse envelope. status is kept out of the JSON body
// (it's already on the HTTP response line); the body field holds whatever
// envelope newHumaError assembled.
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

// GetStatus satisfies huma.StatusError so Huma writes the right HTTP code.
func (e *apiResponseError) GetStatus() int {
	return e.status
}

// MarshalJSON makes Huma's serializer emit the envelope shape rather than
// the wrapper struct's fields.
func (e *apiResponseError) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.body)
}

// firstAppError returns the first *AppError found in errs (using errors.As
// so wrapped errors are unwrapped transparently). nil if none.
func firstAppError(errs []error) *appErrors.AppError {
	for _, err := range errs {
		if err == nil {
			continue
		}
		var appErr *appErrors.AppError
		if errors.As(err, &appErr) {
			return appErr
		}
	}
	return nil
}

// joinErrorDetails concatenates the Error() strings of all non-nil
// elements of errs with "; ". Returns "" if errs is empty or all-nil.
// Used for native Huma validation errors which arrive as a slice of
// *huma.ErrorDetail values without an AppError to lift them from.
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

// codeFromStatus maps an HTTP status to one of the AppErrorType string
// constants. Used when Huma raises a framework error (e.g. validation 422)
// before the handler runs and there is no domain *AppError to lift fields
// from. The AppError type catalogue covers every status Huma synthesises;
// fall back to InternalError for anything unexpected so the envelope still
// has a non-empty machine-readable code.
func codeFromStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return string(appErrors.BadRequestError)
	case http.StatusUnauthorized:
		return string(appErrors.UnauthorizedError)
	case http.StatusPaymentRequired:
		return string(appErrors.PaymentRequiredError)
	case http.StatusForbidden:
		return string(appErrors.ForbiddenError)
	case http.StatusNotFound:
		return string(appErrors.NotFoundError)
	case http.StatusConflict:
		return string(appErrors.ConflictError)
	case http.StatusUnprocessableEntity:
		return string(appErrors.ValidationError)
	case http.StatusTooManyRequests:
		return string(appErrors.RateLimitError)
	case http.StatusNotImplemented:
		return string(appErrors.NotImplementedError)
	case http.StatusBadGateway:
		return string(appErrors.AIProviderError)
	case http.StatusServiceUnavailable:
		return string(appErrors.ServiceUnavailable)
	default:
		return string(appErrors.InternalError)
	}
}

// typeFromStatus mirrors codeFromStatus today; kept as a separate function
// so the response.APIError {Code, Type} fields can diverge later (Code as
// the stable machine identifier, Type as a higher-level taxonomy bucket)
// without touching every call site.
func typeFromStatus(status int) string {
	return codeFromStatus(status)
}
