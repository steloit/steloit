// Package shared provides small, cross-domain primitives used by every
// domain package. Currently: nullable-field helpers that bridge the gap
// between zero-value ingress (empty-string from handlers, zero UUIDs
// from unset fields) and pointer-typed domain fields that mirror the
// nullable schema.
//
// Scope rules: if a helper is only used by one domain, it lives in that
// domain, not here. A helper earns a seat in `shared` only after two or
// more domains need it — and even then it must represent a
// domain-universal concept ("absence vs empty"), not an infrastructure
// concern (which belongs in `internal/infrastructure/db`).
package shared

// NilIfEmpty returns nil if s is "", otherwise a pointer to s. Use at
// ingress boundaries (handler → domain constructor) when the caller
// passes "" to mean "not provided" and the domain/schema models
// "not provided" as NULL.
//
// The mirror operation (pointer → string) is intentionally NOT provided
// — callers that read a nullable field should branch on nil explicitly
// rather than silently collapse nil to "" at the read site.
func NilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Ptr returns &v. Use when constructing a struct literal that needs a
// pointer field set from a value expression (e.g., `Description: shared.Ptr("foo")`).
// Equivalent to k8s.io/utils/ptr.To[T].
func Ptr[T any](v T) *T {
	return &v
}
