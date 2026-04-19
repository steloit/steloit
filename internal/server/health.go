package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"brokle/internal/version"
)

// Health endpoints follow the Kubernetes API server convention —
// /livez, /readyz, /healthz. Each returns a flat JSON shape that
// kubelet, Prometheus blackbox exporters, and operator dashboards
// consume directly. HUMA-EXEMPT by design: the responses are
// envelope-free (probe consumers expect a known shape) and the
// readiness status code varies (200 vs 503), which would conflict
// with the APIResponse `success: true` invariant.
//
// Mounted on the chi router BEFORE global middleware (RequestID,
// Logger, Metrics) so that the high-frequency probe traffic
// (kubelet probes ~6/min × pod count) doesn't dominate the
// structured logs or pollute Prometheus label cardinality. The
// recoverer is the only middleware that wraps health routes (it
// applies via http.Server's outermost handler).

// readyState tracks whether the server should accept new requests.
// Set to true by Server.Start once routes are wired; flipped to
// false by Server.Shutdown's two-phase drain so kubelet observes
// 503 on /readyz BEFORE srv.Shutdown blocks new connections. The
// load balancer then de-registers the pod within one readiness
// interval (typically 5–10s) so in-flight requests can drain
// without a 502 burst.
type readyState struct{ ready atomic.Bool }

func newReadyState() *readyState { return &readyState{} }

func (r *readyState) MarkReady()       { r.ready.Store(true) }
func (r *readyState) MarkNotReady()    { r.ready.Store(false) }
func (r *readyState) IsReady() bool    { return r.ready.Load() }

// processStart is the moment the server package was first imported
// — close enough to "process start" for uptime reporting. Captured
// at package init so it survives Server reconstruction in tests.
var processStart = time.Now()

// healthBody is the flat (envelope-free) shape returned by every
// health endpoint. Field names match kubelet / Prometheus probe
// conventions.
type healthBody struct {
	Status    string                 `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Version   string                 `json:"version,omitempty"`
	Uptime    string                 `json:"uptime"`
	Checks    map[string]healthCheck `json:"checks,omitempty"`
}

// healthCheck is the per-component sub-status carried inside a
// readiness response. Duration is always reported so probe
// consumers can graph dependency-health latency.
type healthCheck struct {
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	LastChecked string `json:"last_checked"`
	Duration    string `json:"duration,omitempty"`
}

// healthDeps groups the dependencies the readyz handler pings. All
// pointers may be nil in tests — a nil pointer skips that check.
type healthDeps struct {
	DB         *pgxpool.Pool
	Redis      *redis.Client
	ClickHouse driver.Conn
	Logger     *slog.Logger
}

// handleLivez returns 200 unconditionally as long as the goroutine
// can serve a request. The kubelet liveness probe restarts the pod
// when this fails, so we never let it depend on external state —
// /readyz is the dependency-aware probe.
func handleLivez() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, http.StatusOK, healthBody{
			Status:    "alive",
			Timestamp: nowRFC3339(),
			Version:   version.Get(),
			Uptime:    time.Since(processStart).String(),
		})
	}
}

// handleHealthz aliases /livez. Kept for backward compat with
// generic monitoring tools that probe /healthz by default
// (Prometheus blackbox, AWS ALB target groups).
func handleHealthz() http.HandlerFunc { return handleLivez() }

// handleReadyz pings every dependency in parallel with a 2-second
// per-dep timeout. Returns 200 only when every check is healthy
// AND the server is in the "ready" state. The atomic ready flag is
// what the two-phase drain flips during shutdown so the LB
// de-registers before in-flight requests are interrupted.
func handleReadyz(state *readyState, d healthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !state.IsReady() {
			writeHealth(w, http.StatusServiceUnavailable, healthBody{
				Status:    "shutting_down",
				Timestamp: nowRFC3339(),
				Version:   version.Get(),
				Uptime:    time.Since(processStart).String(),
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := map[string]healthCheck{}
		if d.DB != nil {
			checks["database"] = pingPostgres(ctx, d.DB)
		}
		if d.Redis != nil {
			checks["redis"] = pingRedis(ctx, d.Redis)
		}
		if d.ClickHouse != nil {
			checks["clickhouse"] = pingClickHouse(ctx, d.ClickHouse)
		}

		overall := "healthy"
		status := http.StatusOK
		for _, c := range checks {
			if c.Status != "healthy" {
				overall = "unhealthy"
				status = http.StatusServiceUnavailable
				break
			}
		}

		writeHealth(w, status, healthBody{
			Status:    overall,
			Timestamp: nowRFC3339(),
			Version:   version.Get(),
			Uptime:    time.Since(processStart).String(),
			Checks:    checks,
		})
	}
}

// pingPostgres runs pgxpool.Ping with the supplied context. Pings
// are cheap (single round-trip SELECT 1) and bounded by the
// caller's 2-second timeout.
func pingPostgres(ctx context.Context, db *pgxpool.Pool) healthCheck {
	start := time.Now()
	if err := db.Ping(ctx); err != nil {
		return healthCheck{
			Status:      "unhealthy",
			Message:     err.Error(),
			LastChecked: nowRFC3339(),
			Duration:    time.Since(start).String(),
		}
	}
	return healthCheck{
		Status:      "healthy",
		Message:     "Postgres connection is healthy",
		LastChecked: nowRFC3339(),
		Duration:    time.Since(start).String(),
	}
}

// pingRedis runs PING with the supplied context. The 2-second
// caller timeout doubles as the connection-attempt deadline if the
// pool is empty.
func pingRedis(ctx context.Context, r *redis.Client) healthCheck {
	start := time.Now()
	if err := r.Ping(ctx).Err(); err != nil {
		return healthCheck{
			Status:      "unhealthy",
			Message:     err.Error(),
			LastChecked: nowRFC3339(),
			Duration:    time.Since(start).String(),
		}
	}
	return healthCheck{
		Status:      "healthy",
		Message:     "Redis connection is healthy",
		LastChecked: nowRFC3339(),
		Duration:    time.Since(start).String(),
	}
}

// pingClickHouse runs the native driver Ping with the supplied
// context.
func pingClickHouse(ctx context.Context, c driver.Conn) healthCheck {
	start := time.Now()
	if err := c.Ping(ctx); err != nil {
		return healthCheck{
			Status:      "unhealthy",
			Message:     err.Error(),
			LastChecked: nowRFC3339(),
			Duration:    time.Since(start).String(),
		}
	}
	return healthCheck{
		Status:      "healthy",
		Message:     "ClickHouse connection is healthy",
		LastChecked: nowRFC3339(),
		Duration:    time.Since(start).String(),
	}
}

// writeHealth writes a JSON-encoded health body with the given
// status. Encoder errors are intentionally swallowed — the response
// is already committed and there is nothing useful to do at that
// point.
func writeHealth(w http.ResponseWriter, status int, body healthBody) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
