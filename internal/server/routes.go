package server

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jub0bs/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"brokle/internal/transport/http/middleware"
	websiteHandler "brokle/internal/transport/http/handlers/website"
)

// addRoutes wires the full HTTP surface area onto a chi router. Run
// once at server start; never per-request. The function is a single
// authoritative dependency map for the service — when a route 404s,
// this is the file to grep.
//
// Layout (top-to-bottom matches request flow):
//
//  1. Health + metrics endpoints — mounted BEFORE global middleware
//     so probe traffic doesn't dominate logs/metrics. The recoverer
//     still wraps them via http.Server's outermost handler. K8s API
//     server convention (/livez, /readyz, /healthz).
//  2. Global middleware — RequestID, RealIP, RequestLogger,
//     Recoverer, Metrics. Order matters; see middleware/recoverer.go
//     for the http.ErrAbortHandler caveat.
//  3. CORS — jub0bs/cors, applied to /v1 and /api/v1 separately so
//     each surface declares its own allowlist (SDK vs dashboard
//     origins differ in cookie-credentials posture).
//  4. SDK plane (/v1/*) — RequireSDKAuth + LimitByAPIKey + apiPublic
//     Huma operations. The validate-key endpoint is the one
//     exception that runs without SDK auth (it accepts a raw key for
//     introspection); IP + key-prefix rate limit defends against
//     brute force.
//  5. Dashboard plane (/api/v1/*) — LimitByIP envelope; CSRF-style
//     protection via stdlib http.CrossOriginProtection (Go 1.25);
//     public auth routes (login/signup) followed by an authed group
//     (RequireAuth + LimitByUser) for everything else.
//  6. Per-domain RegisterRoutes calls — each handler domain exposes
//     RegisterRoutes(api huma.API, services...) and self-registers
//     against the appropriate Huma instance. New domains get added
//     as Step 4 converts handlers from gin-shape to Huma operations.
func addRoutes(r chi.Router, apiPublic, apiAdmin huma.API, d Deps, ready *readyState) {
	// 1. Health + metrics — mounted before middleware so probes are silent.
	healthD := d.healthDeps()
	r.Get("/livez", handleLivez())
	r.Get("/readyz", handleReadyz(ready, healthD))
	r.Get("/healthz", handleHealthz())
	r.Method(http.MethodGet, "/metrics", promhttp.Handler())

	// 2. Global middleware — order is intentional.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger(d.Logger))
	r.Use(middleware.Recoverer(d.Logger))
	r.Use(middleware.Metrics())

	// 3. CORS at the route-group level (each plane declares its own
	//    allowlist). Centralised cors.Middleware is constructed from
	//    config so misconfig fails fast at boot — jub0bs/cors panics
	//    on insecure combinations (credentials + wildcard, etc.).
	corsAdmin := mustCORS(d, "dashboard")

	// 4. SDK plane: /v1/* — apiPublic Huma operations.
	rateLimitD := d.rateLimitMiddlewareDeps()
	sdkAuthD := d.sdkAuthMiddlewareDeps()

	// /v1/auth/validate-key is the exception that runs WITHOUT
	// RequireSDKAuth (it accepts a raw key for introspection). Defend
	// against distributed brute force with IP + hashed-key-prefix
	// limits.
	r.With(
		middleware.LimitByIP(rateLimitD),
		middleware.LimitByKeyPrefix(rateLimitD),
	).Post(publicAPIPrefix+"/auth/validate-key", todoNotImplemented("validate-key conversion pending Step 4"))

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireSDKAuth(sdkAuthD))
		r.Use(middleware.LimitByAPIKey(rateLimitD))
		// Per-domain SDK route registrations land here as Step 4
		// converts handlers. apiPublic is the Huma instance carrying
		// the /v1/openapi.json spec.
		_ = apiPublic // Suppress "declared and not used" until first SDK domain registers.
	})

	// 5. Dashboard plane: /api/v1/* — apiAdmin Huma operations.
	r.Route(adminAPIPrefix, func(r chi.Router) {
		r.Use(corsAdmin.Wrap)
		r.Use(middleware.LimitByIP(rateLimitD))
		// CSRF protection — Go 1.25 stdlib. CrossOriginProtection
		// returns a *CrossOriginProtection whose .Handler wraps the
		// downstream handler with Sec-Fetch-Site / Origin checks.
		// Same-origin POSTs and any GET/HEAD/OPTIONS pass through
		// transparently; cross-origin POSTs without a trusted Origin
		// return 403.
		r.Use(crossOriginProtection().Handler)

		// Public dashboard routes — login, signup, password reset,
		// website contact form. No auth required; rate limit + CSRF
		// still apply.
		websiteHandler.RegisterRoutes(apiAdmin, d.Website, d.Logger)
		// auth.RegisterPublicRoutes(apiAdmin, d.AuthService)  // Step 4

		// Authed dashboard routes — everything else.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(d.authMiddlewareDeps()))
			r.Use(middleware.LimitByUser(rateLimitD))
			// Per-domain authed dashboard registrations land here.
			// organization.RegisterRoutes(apiAdmin, d.Organization, d.OrgMember)  // Step 4
			// project.RegisterRoutes(apiAdmin, d.Project)                          // Step 4
			// ... 22 more domains
		})
	})
}

// mustCORS constructs a jub0bs/cors middleware from config or panics
// at boot if the config is invalid. The panic is intentional:
// jub0bs/cors's whole value proposition is config validation, and a
// CORS misconfig at runtime is exactly the silent-vulnerability
// failure mode we adopted the library to prevent.
//
// "plane" labels which surface (sdk / dashboard) the CORS instance
// belongs to so the panic message points at the right config knob.
func mustCORS(d Deps, plane string) *cors.Middleware {
	mw, err := cors.NewMiddleware(cors.Config{
		Origins:        d.Config.Server.CORSAllowedOrigins,
		Methods:        d.Config.Server.CORSAllowedMethods,
		RequestHeaders: append(d.Config.Server.CORSAllowedHeaders, "X-CSRF-Token"),
		Credentialed:   true,
		MaxAgeInSeconds: 300, // 5 minutes — matches existing config
	})
	if err != nil {
		panic("server: invalid CORS config for " + plane + " plane: " + err.Error())
	}
	return mw
}

// crossOriginProtection returns a configured Go 1.25
// http.CrossOriginProtection enforcing the Sec-Fetch-Site +
// Origin header check on every non-idempotent request to /api/v1/*.
//
// The dashboard frontend at web/ runs on the same origin as the API
// in production, so no AddTrustedOrigin calls are needed for normal
// operation. Bypass patterns (OAuth callbacks, webhook receivers)
// are added with AddInsecureBypassPattern when the corresponding
// domain registers — for now there are none.
//
// Returns the *CrossOriginProtection rather than its .Handler so
// the route registrar can choose whether to apply it conditionally
// later (e.g. for an /api/v1/webhooks/* group).
func crossOriginProtection() *http.CrossOriginProtection {
	return http.NewCrossOriginProtection()
}

// todoNotImplemented is a placeholder handler used while Step 4 is
// in progress — returns 501 with a stable JSON payload so a probe
// against an un-converted route fails loudly instead of returning
// a misleading 404. Removed once every gin handler has been
// converted to a Huma operation.
func todoNotImplemented(reason string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`{"success":false,"error":{"type":"not_implemented","code":"not_implemented","message":"` + reason + `"}}`))
	}
}
