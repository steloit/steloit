package errors

import (
	stderrors "errors"
	"net/http"
	"testing"
)

// TestEveryErrorTypeHasStatus locks in the invariant that every value
// listed in the ErrorType const block has a typeToStatus row. The
// const block IS the source of truth for the closed enum; missing a
// row would silently fall back to 500 in production. Catching that at
// build time is what keeps the closed-enum guarantee real.
func TestEveryErrorTypeHasStatus(t *testing.T) {
	declared := []ErrorType{
		TypeInvalidRequest,
		TypeValidation,
		TypeAuthentication,
		TypePermission,
		TypeNotFound,
		TypeConflict,
		TypePaymentRequired,
		TypeRateLimit,
		TypeUpstreamProvider,
		TypeServiceUnavailable,
		TypeNotImplemented,
		TypeAPIError,
	}
	for _, tt := range declared {
		if _, ok := typeToStatus[tt]; !ok {
			t.Errorf("ErrorType %q has no typeToStatus row — add it to the map in errors.go", tt)
		}
	}
	if len(declared) != len(typeToStatus) {
		t.Errorf("declared types (%d) != typeToStatus entries (%d) — exhaustive list out of sync",
			len(declared), len(typeToStatus))
	}
}

// TestFromHTTPStatusClassFallback is the regression test for the
// original bug: framework-emitted 4xx statuses (405, 406, 408, 413,
// 415, 416, 417, 451) used to fall through to INTERNAL_ERROR. The
// class fallback in typeFromStatus must keep them in TypeInvalidRequest
// (or a more specific 4xx Type), never let them leak to TypeAPIError.
func TestFromHTTPStatusClassFallback(t *testing.T) {
	// Statuses Huma documents emitting from its pre-handler pipeline,
	// plus chi/net-http 405 and a fronting CDN's 520 (Cloudflare).
	cases := map[int]ErrorType{
		http.StatusMethodNotAllowed:           TypeInvalidRequest,
		http.StatusNotAcceptable:              TypeInvalidRequest,
		http.StatusRequestTimeout:             TypeInvalidRequest,
		http.StatusRequestEntityTooLarge:      TypeInvalidRequest,
		http.StatusUnsupportedMediaType:       TypeInvalidRequest,
		http.StatusRequestedRangeNotSatisfiable: TypeInvalidRequest,
		http.StatusExpectationFailed:          TypeInvalidRequest,
		http.StatusUnavailableForLegalReasons: TypeInvalidRequest,
		418:                                   TypeInvalidRequest, // I'm a teapot — class fallback
		520:                                   TypeAPIError,       // Cloudflare unknown — 5xx fallback
		599:                                   TypeAPIError,       // far end of 5xx — fallback
		http.StatusGatewayTimeout:             TypeServiceUnavailable,
		http.StatusBadGateway:                 TypeUpstreamProvider,
	}
	for status, want := range cases {
		got := typeFromStatus(status)
		if got != want {
			t.Errorf("typeFromStatus(%d) = %q, want %q", status, got, want)
		}
		if want != TypeAPIError && got == TypeAPIError {
			t.Errorf("MISCLASSIFICATION REGRESSION: status %d (4xx) leaked to TypeAPIError", status)
		}
	}
}

// TestAppErrorIs locks in the (Type, Code) matching contract used by
// errors.Is — type-only matching is a common pattern and must not
// regress to require code equality.
func TestAppErrorIs(t *testing.T) {
	err := NewNotFoundError("project", WithCode("project_not_found"))

	if !stderrors.Is(err, &AppError{Type: TypeNotFound}) {
		t.Error("type-only matching against TypeNotFound should succeed")
	}
	if !stderrors.Is(err, &AppError{Type: TypeNotFound, Code: "project_not_found"}) {
		t.Error("type+code matching against same code should succeed")
	}
	if stderrors.Is(err, &AppError{Type: TypeNotFound, Code: "user_not_found"}) {
		t.Error("type+code matching against different code should fail")
	}
	if stderrors.Is(err, &AppError{Type: TypeValidation}) {
		t.Error("type-only matching against different type should fail")
	}
}

// TestCodeOrTypeFallback verifies the wire-format invariant: the
// `code` field is never empty in the rendered envelope. When Code is
// unset, CodeOrType falls back to string(Type).
func TestCodeOrTypeFallback(t *testing.T) {
	bare := New(TypeRateLimit, "slow down")
	if bare.CodeOrType() != string(TypeRateLimit) {
		t.Errorf("CodeOrType() with empty Code = %q, want %q", bare.CodeOrType(), TypeRateLimit)
	}

	withCode := New(TypeRateLimit, "slow down", WithCode("quota_exceeded"))
	if withCode.CodeOrType() != "quota_exceeded" {
		t.Errorf("CodeOrType() with Code = %q, want %q", withCode.CodeOrType(), "quota_exceeded")
	}
}

// TestFunctionalOptions sanity-checks the variadic-options pattern —
// each option must mutate only its own field, options must compose,
// and the typed constructors must default Type / Code consistently.
func TestFunctionalOptions(t *testing.T) {
	cause := stderrors.New("underlying boom")
	err := NewValidationError("Invalid project ID", "projectId must be a valid UUID",
		WithCode("invalid_project_id"),
		WithParam("projectId"),
		WithCause(cause),
	)

	if err.Type != TypeValidation {
		t.Errorf("Type = %q, want %q", err.Type, TypeValidation)
	}
	if err.Code != "invalid_project_id" {
		t.Errorf("Code = %q, want %q", err.Code, "invalid_project_id")
	}
	if err.Param != "projectId" {
		t.Errorf("Param = %q, want %q", err.Param, "projectId")
	}
	if err.Details != "projectId must be a valid UUID" {
		t.Errorf("Details = %q, want the supplied details string", err.Details)
	}
	if !stderrors.Is(err, cause) {
		t.Error("WithCause should make errors.Is(err, cause) succeed via Unwrap")
	}
	if err.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Errorf("HTTPStatus() = %d, want %d (TypeValidation → 422)",
			err.HTTPStatus(), http.StatusUnprocessableEntity)
	}
}
