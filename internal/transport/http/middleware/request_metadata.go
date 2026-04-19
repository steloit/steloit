package middleware

import (
	"net/http"

	"brokle/internal/transport/http/clientip"
	"brokle/internal/transport/http/httpctx"
)

// RequestMetadata resolves the originating client IP via the
// supplied clientip.Resolver and captures the User-Agent header,
// stuffing both into the request context via httpctx. Huma
// operation handlers receive only context.Context; this is the
// bridge that lets them record audit-log metadata without reaching
// for an http.ResponseWriter.
//
// Mount in the global middleware chain AFTER chi/middleware.RealIP
// (which normalises r.RemoteAddr) and BEFORE any middleware that
// reads the values (auth middleware that records an audit row,
// rate limiters that key by IP, …).
//
// Resolver is optional — pass nil to use r.RemoteAddr verbatim
// (useful in tests and dev where no proxy fronts the service).
// The clientip.Resolver's trust-proxy boundary enforcement means
// X-Forwarded-For headers from untrusted peers are ignored, so
// "pass resolver" + "don't pass resolver" are both safe defaults
// depending on deployment shape.
func RequestMetadata(resolver *clientip.Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ""
			if resolver != nil {
				ip = resolver.From(r)
			} else {
				ip = r.RemoteAddr
			}
			ctx := httpctx.WithClientIP(r.Context(), ip)
			ctx = httpctx.WithUserAgent(ctx, r.UserAgent())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
