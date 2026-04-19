// Package server holds the HTTP server lifecycle, routing skeleton,
// per-process middleware composition, and the Huma API instances. It
// is the bridge between the chi/Huma transport layer and the rest of
// the application — services, repositories, and workers are
// constructed elsewhere and injected via Deps.
package server

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// Brokle ships TWO OpenAPI surfaces, mounted on the same chi mux:
//
//   - apiPublic — /v1/* SDK ingestion + read endpoints. Spec at
//     /v1/openapi.json, docs at /v1/docs. Auth: X-API-Key.
//   - apiAdmin  — /api/v1/* dashboard endpoints. Spec at
//     /api/v1/openapi.json, docs at /api/v1/docs. Auth: cookie+JWT.
//
// Two surfaces (rather than one) because the auth schemes are
// distinct (API-key vs cookie+JWT), the consumers are distinct
// (third-party SDKs vs first-party Next.js dashboard), and a single
// spec would have to declare both securitySchemes on every operation
// or invent tag conventions that codegen toolchains do not honour
// (openapi-typescript issue #1922 still open as of 2026).
//
// The pattern follows Langfuse's split (server + client + admin) and
// Stripe's split (spec3.json + spec3.sdk.json). A third surface
// (apiInstance) is intentionally NOT pre-built — see CLAUDE.md
// gotcha on scaffolded-but-unreachable code. Add it when self-host
// admin endpoints land, not before.

// publicAPIPrefix and adminAPIPrefix are the path prefixes Huma's
// auto-served openapi/docs endpoints live under. Operation paths
// declare the same prefix in their Path field (e.g. Path:
// "/v1/projects/{id}") — Huma uses these for spec generation and
// chi for routing.
const (
	publicAPIPrefix = "/v1"
	adminAPIPrefix  = "/api/v1"
)

// newAPIPublic constructs the SDK-facing huma.API. Mounted on the
// shared chi router; serves /v1/openapi.json + /v1/openapi.yaml +
// /v1/docs + /v1/schemas/*.
//
// SecuritySchemes lists ONLY apiKeyAuth — operations registered on
// this API declare X-API-Key auth, never bearer/cookie. The runtime
// also accepts Authorization: Bearer <key> for HTTP clients that
// can't set custom headers; that is documented in the scheme's
// Description but not modelled as a separate scheme so SDK codegen
// produces one auth class.
func newAPIPublic(router chi.Router, version string) huma.API {
	cfg := huma.DefaultConfig("Brokle SDK API", version)
	cfg.Info.Description = "Brokle SDK ingestion, observability reads, and prompt management. " +
		"API-key authenticated — pass `X-API-Key: <key>` or `Authorization: Bearer <key>`."
	cfg.OpenAPIPath = publicAPIPrefix + "/openapi"
	cfg.DocsPath = publicAPIPrefix + "/docs"
	cfg.SchemasPath = publicAPIPrefix + "/schemas"

	if cfg.Components == nil {
		cfg.Components = &huma.Components{}
	}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"apiKeyAuth": {
			Type:        "apiKey",
			In:          "header",
			Name:        "X-API-Key",
			Description: "SDK API key. The runtime middleware also accepts the `Authorization: Bearer <key>` form for HTTP clients that lack custom-header support.",
		},
	}
	return humachi.New(router, cfg)
}

// newAPIAdmin constructs the dashboard-facing huma.API. Mounted on
// the same chi router; serves /api/v1/openapi.json + .yaml + /docs
// + /schemas/*.
//
// SecuritySchemes lists ONLY bearerAuth (HTTP Bearer / JWT). Real
// dashboard clients receive the JWT as the access_token httpOnly
// cookie set by /api/v1/auth/login; the Bearer form is documented
// for OpenAPI consumers (and curl) that don't model cookie auth.
//
// CSRF is enforced at the chi middleware layer (Go 1.25
// http.CrossOriginProtection); it is not modelled as an OpenAPI
// security scheme because the spec doesn't have a way to declare
// "Sec-Fetch-Site origin check required for non-idempotent
// methods" — the rule belongs in handler-side documentation.
func newAPIAdmin(router chi.Router, version string) huma.API {
	cfg := huma.DefaultConfig("Brokle Dashboard API", version)
	cfg.Info.Description = "Brokle dashboard API. JWT authenticated — production clients receive " +
		"the access_token cookie set by /api/v1/auth/login. The `Authorization: Bearer <token>` form " +
		"is documented for OpenAPI consumers that do not model cookie auth."
	cfg.OpenAPIPath = adminAPIPrefix + "/openapi"
	cfg.DocsPath = adminAPIPrefix + "/docs"
	cfg.SchemasPath = adminAPIPrefix + "/schemas"

	if cfg.Components == nil {
		cfg.Components = &huma.Components{}
	}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description: "Dashboard JWT issued by /api/v1/auth/login. Production clients receive it as the `access_token` httpOnly cookie; the Bearer form is documented for OpenAPI consumers that do not model cookie auth.",
		},
	}
	return humachi.New(router, cfg)
}
