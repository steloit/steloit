package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// routePattern returns the chi-resolved route template for r (e.g.
// "/api/v1/projects/{id}") or "" when no route matched. Used by the
// request logger and metrics middleware to keep label cardinality
// bounded — emitting per-ID literal paths into Prometheus is the
// classic series-explosion footgun.
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return rctx.RoutePattern()
	}
	return ""
}
