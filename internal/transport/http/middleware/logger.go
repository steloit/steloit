package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestLogger emits a single slog record per HTTP request after the
// handler returns. Hand-rolled rather than imported because (a) the
// surface is ~40 LoC of stable stdlib-adjacent logic and (b) Brokle is
// an observability platform — outsourcing our own request log line is
// the wrong dogfood signal. This matches what Loki, Caddy, SigNoz, and
// the OpenTelemetry Collector do in their own server code.
//
// Fields emitted:
//
//   - method          — HTTP verb
//   - path            — request URL path (literal, not the route pattern)
//   - route           — chi route template (e.g. /api/v1/projects/{id}),
//     omitted when the request did not match any registered route
//   - status          — response status (defaults to 200 when the handler
//     never calls WriteHeader explicitly)
//   - dur_ms          — handler duration in milliseconds
//   - bytes           — response body size in bytes
//   - request_id      — chi/middleware.RequestID value, propagated to
//     the X-Request-ID response header
//   - remote_ip       — r.RemoteAddr, already normalised by
//     chi/middleware.RealIP if mounted
//   - user_agent      — request User-Agent header
//
// Trace and span IDs are picked up automatically when the application
// is wired with otelslog (the official OTel → slog bridge). No special
// handling here.
//
// Log level is derived from response status:
//
//   - 5xx → Error
//   - 4xx → Warn
//   - else → Info
//
// Mount this middleware after chi/middleware.RequestID + RealIP and
// after the Recoverer so panic recoveries still produce a request
// log line.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				// Handler never called WriteHeader — http.ResponseWriter
				// implicitly emits 200 on first Write. Reflect that so
				// the log line agrees with the wire response.
				status = http.StatusOK
			}

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int64("dur_ms", time.Since(start).Milliseconds()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.String("remote_ip", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			}
			if id := chimw.GetReqID(r.Context()); id != "" {
				attrs = append(attrs, slog.String("request_id", id))
			}
			if pattern := routePattern(r); pattern != "" {
				attrs = append(attrs, slog.String("route", pattern))
			}

			logger.LogAttrs(r.Context(), levelForStatus(status), "http_request", attrs...)
		})
	}
}

// levelForStatus picks a slog level based on response status. 5xx are
// errors (server fault), 4xx are warnings (client misuse / expected
// rejections), everything else is info. Matches the convention used
// by Caddy and most chi-ecosystem loggers.
func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
