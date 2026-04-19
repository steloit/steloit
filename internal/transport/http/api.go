package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// APIConfig describes the public face of the OpenAPI document Huma emits.
//
// Title and Version are required. Description and Servers are optional —
// Servers is what populates the "Try it out" base-URL menu in the rendered
// docs UI; supply both the local and the production base URLs in production
// builds and just localhost in dev.
type APIConfig struct {
	Title       string
	Version     string
	Description string
	Servers     []*huma.Server
}

// NewAPI constructs a Huma v2 API mounted on the supplied chi router. The
// router retains ownership: plain chi handlers (SSE, WebSocket, /metrics,
// /health) live alongside Huma operations on the same mux, and middleware
// declared via router.Use wraps both uniformly.
//
// The function configures:
//   - OpenAPI 3.1 emission (Huma's default).
//   - bearerAuth (HTTP Bearer / JWT) — used by dashboard routes; the JWT is
//     normally delivered via the access_token httpOnly cookie. The Bearer
//     form is documented for OpenAPI consumers that don't model cookies.
//   - apiKeyAuth (X-API-Key header) — used by SDK routes; the
//     "Authorization: Bearer <key>" form is also accepted by the runtime
//     middleware but apiKey-in-header is the canonical SDK contract.
//
// Operations declare which scheme they require via huma.Operation.Security;
// enforcement still runs at the chi middleware layer (RequireAuth /
// RequireSDKAuth). Security on the operation is documentation only — the
// SDK consumers see it; the server doesn't gate on it.
func NewAPI(router chi.Router, cfg APIConfig) huma.API {
	config := huma.DefaultConfig(cfg.Title, cfg.Version)
	if cfg.Description != "" {
		config.Info.Description = cfg.Description
	}
	if len(cfg.Servers) > 0 {
		config.Servers = cfg.Servers
	}

	if config.Components == nil {
		config.Components = &huma.Components{}
	}
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description: "Dashboard JWT issued by /api/v1/auth/login. Production " +
				"clients receive it as the `access_token` httpOnly cookie; the " +
				"Bearer form is documented for OpenAPI consumers that do not " +
				"model cookie auth.",
		},
		"apiKeyAuth": {
			Type:        "apiKey",
			In:          "header",
			Name:        "X-API-Key",
			Description: "SDK API key. The runtime middleware also accepts the " +
				"`Authorization: Bearer <key>` form for compatibility with HTTP " +
				"clients that lack custom-header support.",
		},
	}

	return humachi.New(router, config)
}
