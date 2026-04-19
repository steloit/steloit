package middleware

import (
	"net/http"
	"strconv"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus collectors for HTTP request metrics. Registered against
// the default registerer at process start via promauto so they appear
// in the existing /metrics endpoint without extra wiring.
var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests handled by the chi router",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds, bucketed by route and method",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// Metrics records request count and duration into Prometheus, keyed
// by the chi route pattern (e.g. "/api/v1/projects/{id}") rather than
// the literal URL. Patterned paths keep label cardinality bounded —
// per-ID label explosion is a classic Prometheus pitfall.
//
// The wrapped writer is chi.middleware.NewWrapResponseWriter so SSE
// (http.Flusher), WebSocket (http.Hijacker), and HTTP/2 server-push
// (http.Pusher) interfaces survive — vanilla wrapping silently breaks
// streaming endpoints (chi/wrap_writer.go).
//
// Path resolution falls back to "unmatched" when chi's RouteContext
// has no pattern (404 from scanner traffic, malformed URLs). This
// caps label cardinality even under abuse.
//
// Mount AFTER the request logger so both middleware see the same
// wrapped writer and observe the same final status. Only one
// middleware should instantiate a wrapper — the wrapper-of-a-wrapper
// chain loses optional-interface capability ([Flusher], [Hijacker]).
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww, ok := w.(chimw.WrapResponseWriter)
			if !ok {
				ww = chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			}
			start := time.Now()

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			path := routePattern(r)
			if path == "" {
				path = "unmatched"
			}

			httpRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(status)).Inc()
			httpRequestDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
		})
	}
}
