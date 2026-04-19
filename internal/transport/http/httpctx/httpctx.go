// Package httpctx provides typed accessors for request-scoped values that
// authentication and routing middleware attach to context.Context.
//
// The keys themselves are unexported empty-struct singletons (the canonical
// Go idiom — see net/http's ServerContextKey, LocalAddrContextKey). Values
// are read and written through the With*/MustGet*/Get* helpers in this
// package; nothing outside the package can construct a key, which makes
// collisions structurally impossible and keeps the request-scoped data
// surface area auditable in one place.
//
// Two accessor styles are exposed for each value:
//
//   - MustGetX(ctx) — panics when the value is missing or has the wrong type.
//     Use on routes guaranteed to run the appropriate auth middleware
//     (RequireAuth / RequireSDKAuth). Panics are caught by the Recovery
//     middleware and surface as HTTP 500 with the request-id in the log,
//     making the misconfiguration loud instead of silently returning a
//     misleading 401. This mirrors regexp.MustCompile / template.Must and
//     matches Mat Ryer's "How I write HTTP services in Go" recommendation.
//
//   - X(ctx) (T, bool) — tuple form for routes wired with OptionalAuth or
//     audit-log paths where unauthenticated is a legitimate runtime case.
package httpctx

import (
	"context"

	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
)

// Unexported singleton keys — one per value type. context.WithValue compares
// keys with ==; empty structs are size-zero and compare by their address
// within the package, eliminating cross-package collision risk.
type (
	authContextKey    struct{}
	userIDKey         struct{}
	tokenClaimsKey    struct{}
	sdkAuthContextKey struct{}
	apiKeyIDKey       struct{}
	projectIDKey      struct{}
	organizationIDKey struct{}
	environmentKey    struct{}
	clientIPKey       struct{}
	userAgentKey      struct{}
)

// ----- Dashboard auth context (set by RequireAuth / OptionalAuth) -----

// WithAuthContext returns a derived context carrying the dashboard auth context.
func WithAuthContext(ctx context.Context, ac *auth.AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, ac)
}

// AuthContext returns the dashboard auth context if present.
func AuthContext(ctx context.Context) (*auth.AuthContext, bool) {
	ac, ok := ctx.Value(authContextKey{}).(*auth.AuthContext)
	return ac, ok
}

// MustGetAuthContext returns the dashboard auth context. Panics when missing —
// signals that a handler was attached outside RequireAuth.
func MustGetAuthContext(ctx context.Context) *auth.AuthContext {
	ac, ok := AuthContext(ctx)
	if !ok {
		panic("httpctx.MustGetAuthContext: auth context not found — protected route is missing RequireAuth middleware")
	}
	return ac
}

// ----- User ID (set by RequireAuth / OptionalAuth) -----

// WithUserID returns a derived context carrying the authenticated user ID.
func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserID returns the authenticated user ID if present.
func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	return id, ok
}

// MustGetUserID returns the authenticated user ID. Panics when missing —
// signals that a handler was attached outside RequireAuth.
func MustGetUserID(ctx context.Context) uuid.UUID {
	id, ok := UserID(ctx)
	if !ok {
		panic("httpctx.MustGetUserID: user ID not found — protected route is missing RequireAuth middleware")
	}
	return id
}

// ----- JWT claims (set by RequireAuth / OptionalAuth) -----

// WithTokenClaims returns a derived context carrying the JWT claims.
func WithTokenClaims(ctx context.Context, claims *auth.JWTClaims) context.Context {
	return context.WithValue(ctx, tokenClaimsKey{}, claims)
}

// TokenClaims returns the JWT claims if present.
func TokenClaims(ctx context.Context) (*auth.JWTClaims, bool) {
	c, ok := ctx.Value(tokenClaimsKey{}).(*auth.JWTClaims)
	return c, ok
}

// MustGetTokenClaims returns the JWT claims. Panics when missing — signals
// a handler attached outside RequireAuth.
func MustGetTokenClaims(ctx context.Context) *auth.JWTClaims {
	c, ok := TokenClaims(ctx)
	if !ok {
		panic("httpctx.MustGetTokenClaims: token claims not found — protected route is missing RequireAuth middleware")
	}
	return c
}

// ----- SDK auth context (set by RequireSDKAuth) -----

// WithSDKAuthContext returns a derived context carrying the SDK auth context.
func WithSDKAuthContext(ctx context.Context, ac *auth.AuthContext) context.Context {
	return context.WithValue(ctx, sdkAuthContextKey{}, ac)
}

// SDKAuthContext returns the SDK auth context if present.
func SDKAuthContext(ctx context.Context) (*auth.AuthContext, bool) {
	ac, ok := ctx.Value(sdkAuthContextKey{}).(*auth.AuthContext)
	return ac, ok
}

// MustGetSDKAuthContext returns the SDK auth context. Panics when missing —
// signals a handler attached outside RequireSDKAuth.
func MustGetSDKAuthContext(ctx context.Context) *auth.AuthContext {
	ac, ok := SDKAuthContext(ctx)
	if !ok {
		panic("httpctx.MustGetSDKAuthContext: SDK auth context not found — route is missing RequireSDKAuth middleware")
	}
	return ac
}

// ----- API key ID (set by RequireSDKAuth) -----
//
// Stored as *uuid.UUID because AuthContext.APIKeyID is legitimately nullable —
// session-based auth contexts have no API key.

// WithAPIKeyID returns a derived context carrying the API key ID (nullable).
func WithAPIKeyID(ctx context.Context, id *uuid.UUID) context.Context {
	return context.WithValue(ctx, apiKeyIDKey{}, id)
}

// APIKeyID returns the API key ID pointer if present.
func APIKeyID(ctx context.Context) (*uuid.UUID, bool) {
	id, ok := ctx.Value(apiKeyIDKey{}).(*uuid.UUID)
	return id, ok
}

// ----- Project ID (set by RequireSDKAuth and RequireProjectAccess) -----

// WithProjectID returns a derived context carrying the resolved project ID.
func WithProjectID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, projectIDKey{}, id)
}

// ProjectID returns the resolved project ID if present.
func ProjectID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(projectIDKey{}).(uuid.UUID)
	return id, ok
}

// MustGetProjectID returns the resolved project ID. Panics when missing —
// signals a handler attached outside RequireSDKAuth or RequireProjectAccess.
func MustGetProjectID(ctx context.Context) uuid.UUID {
	id, ok := ProjectID(ctx)
	if !ok {
		panic("httpctx.MustGetProjectID: project ID not found — route is missing RequireSDKAuth or RequireProjectAccess middleware")
	}
	return id
}

// ----- Organization ID (set by RequireSDKAuth) -----

// WithOrganizationID returns a derived context carrying the organization ID.
func WithOrganizationID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, organizationIDKey{}, id)
}

// OrganizationID returns the organization ID if present.
func OrganizationID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(organizationIDKey{}).(uuid.UUID)
	return id, ok
}

// MustGetOrganizationID returns the organization ID. Panics when missing —
// signals a handler attached outside RequireSDKAuth.
func MustGetOrganizationID(ctx context.Context) uuid.UUID {
	id, ok := OrganizationID(ctx)
	if !ok {
		panic("httpctx.MustGetOrganizationID: organization ID not found — SDK route is missing RequireSDKAuth middleware")
	}
	return id
}

// ----- Environment tag (set by SDK / scope middleware) -----

// WithEnvironment returns a derived context carrying the environment tag.
func WithEnvironment(ctx context.Context, env string) context.Context {
	return context.WithValue(ctx, environmentKey{}, env)
}

// Environment returns the environment tag if present.
func Environment(ctx context.Context) (string, bool) {
	env, ok := ctx.Value(environmentKey{}).(string)
	return env, ok
}

// ----- Client IP + User-Agent (set by RequestMetadata middleware) -----
//
// These carry the trusted-proxy-resolved client IP and the raw
// User-Agent header through the request context so Huma operation
// handlers (which receive only context.Context, not *http.Request)
// can emit audit-log rows with the real caller metadata without
// reaching for an http.ResponseWriter.
//
// Both values are always present once the global middleware chain
// has run (ClientIP falls back to r.RemoteAddr host, UserAgent
// falls back to "" — the empty string is a legitimate value, not a
// misconfiguration signal — so these values do NOT expose a Must*
// variant).

// WithClientIP returns a derived context carrying the resolved
// client IP.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// ClientIP returns the resolved client IP for the current request,
// or "" when the metadata middleware didn't run (tests calling
// handlers directly without a full HTTP chain).
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// WithUserAgent returns a derived context carrying the request's
// User-Agent header verbatim.
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, userAgentKey{}, ua)
}

// UserAgent returns the request's User-Agent header, or "" when
// absent or when the metadata middleware didn't run.
func UserAgent(ctx context.Context) string {
	ua, _ := ctx.Value(userAgentKey{}).(string)
	return ua
}
