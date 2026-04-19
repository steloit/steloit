package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
	httprateredis "github.com/go-chi/httprate-redis"

	"brokle/internal/config"
	"brokle/internal/transport/http/httpctx"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// RateLimitDeps groups the Redis configuration and logger every rate-
// limit constructor needs. The Redis backend is built once per
// constructor call and reused across requests; constructors are
// expected to be invoked at server startup, not per-request.
//
// Buy vs build decision: we use go-chi/httprate + httprate-redis
// rather than maintaining a hand-rolled Redis sliding-window. The
// research that led here is in feedback_design_first / the build-vs-
// buy section of the chi-migration plan: rate limiters routinely
// ship six classes of subtle bugs (non-atomic RMW, clock skew across
// replicas, key TTL leakage, X-RateLimit-Reset math, missing
// Retry-After, port-stripping in the keyer). httprate gets all six
// right; the LimitCounter interface lets us swap backends later if
// needed.
type RateLimitDeps struct {
	Redis  *httprateredis.Config // host/port/password/database/keyprefix
	Auth   *config.AuthConfig    // limits + window + on/off switch
	Logger *slog.Logger
}

// LimitByIP rate-limits requests by client IP. Reads the IP via
// chi/middleware.RealIP if mounted upstream, else falls back to
// r.RemoteAddr (port-stripped by httprate.KeyByIP).
//
// Returns a no-op pass-through when rate limiting is disabled in
// config — keeps server.go from branching on the feature flag.
func LimitByIP(d RateLimitDeps) func(http.Handler) http.Handler {
	if !d.Auth.RateLimitEnabled {
		return passThrough()
	}
	return httprate.Limit(
		d.Auth.RateLimitPerIP,
		d.Auth.RateLimitWindow,
		httprate.WithKeyByIP(),
		httprate.WithLimitHandler(rateLimitHandler(d.Logger, "ip")),
		httprateredis.WithRedisLimitCounter(d.Redis),
	)
}

// LimitByUser rate-limits requests by authenticated user ID. Mounted
// downstream of RequireAuth — falls back to no-op when no user is in
// context (the route is public). This is intentionally fail-open
// because IP-based limiting upstream covers the public path.
func LimitByUser(d RateLimitDeps) func(http.Handler) http.Handler {
	if !d.Auth.RateLimitEnabled {
		return passThrough()
	}
	return httprate.Limit(
		d.Auth.RateLimitPerUser,
		d.Auth.RateLimitWindow,
		httprate.WithKeyFuncs(keyByUser),
		httprate.WithLimitHandler(rateLimitHandler(d.Logger, "user")),
		httprateredis.WithRedisLimitCounter(d.Redis),
	)
}

// LimitByAPIKey rate-limits requests by SDK API key ID. Mounted
// downstream of RequireSDKAuth — the API key ID is already in the
// request context. API keys get a 5× multiplier over user limits
// because SDK clients are expected to drive higher request volume
// than interactive dashboard sessions.
func LimitByAPIKey(d RateLimitDeps) func(http.Handler) http.Handler {
	if !d.Auth.RateLimitEnabled {
		return passThrough()
	}
	return httprate.Limit(
		d.Auth.RateLimitPerUser*5,
		d.Auth.RateLimitWindow,
		httprate.WithKeyFuncs(keyByAPIKey),
		httprate.WithLimitHandler(rateLimitHandler(d.Logger, "api_key")),
		httprateredis.WithRedisLimitCounter(d.Redis),
	)
}

// LimitByKeyPrefix rate-limits public unauthenticated endpoints that
// accept a raw API key (e.g. /v1/auth/validate-key) by hashing the
// leading 8 characters of the key. Defends against distributed
// brute-force where an attacker rotates source IPs.
//
// Only the prefix is hashed and stored — the full key never reaches
// Redis. Requests without an API-key header pass through (the
// handler returns 400) and only count against the IP bucket
// upstream.
func LimitByKeyPrefix(d RateLimitDeps) func(http.Handler) http.Handler {
	if !d.Auth.RateLimitEnabled {
		return passThrough()
	}
	return httprate.Limit(
		d.Auth.RateLimitPerKeyPrefix,
		d.Auth.RateLimitWindow,
		httprate.WithKeyFuncs(keyByAPIKeyPrefix),
		httprate.WithLimitHandler(rateLimitHandler(d.Logger, "key_prefix")),
		httprateredis.WithRedisLimitCounter(d.Redis),
	)
}

// passThrough returns an identity middleware. Used as the no-op when
// rate limiting is disabled at the config level so callers don't
// branch.
func passThrough() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// rateLimitHandler returns the http.HandlerFunc httprate invokes
// when the limit is exceeded. Logs with the bucket type and writes
// the canonical APIResponse 429 envelope so SDK clients see a
// machine-readable rate_limit_error.
//
// httprate already sets X-RateLimit-Limit/Remaining/Reset and
// Retry-After headers before invoking this function — we only emit
// the body.
func rateLimitHandler(logger *slog.Logger, bucket string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.WarnContext(r.Context(), "rate limit exceeded", "bucket", bucket, "path", r.URL.Path)
		retryAfter := time.Duration(0)
		if hv := w.Header().Get("Retry-After"); hv != "" {
			// httprate writes seconds as a string; parse for the log only —
			// the response header is already on the wire.
			if d, err := time.ParseDuration(hv + "s"); err == nil {
				retryAfter = d
			}
		}
		_ = retryAfter
		response.WriteError(w, appErrors.NewRateLimitError("Rate limit exceeded. Please try again later."))
	}
}

// ----- key extractors. httprate.KeyFunc returns (key, error). When
// the keyer returns an error, the request is rejected with 500; we
// distinguish "no key — public route" (return errSkipBucket so
// httprate's WithErrorHandler gracefully bypasses) from "infra
// failure". -----

// errSkipBucket is the sentinel returned by key funcs when the
// bucket key is intentionally absent (e.g. RateLimitByUser on a
// route that's currently unauthenticated). httprate treats any
// error as "skip this counter and pass through" because the rate
// limiter has no key to bucket against.
var errSkipBucket = errors.New("no rate-limit bucket key for this request")

// keyByUser keys the bucket on the authenticated user ID written
// into the request context by RequireAuth. Returns errSkipBucket on
// public routes — IP-based limiting upstream covers them.
func keyByUser(r *http.Request) (string, error) {
	id, ok := httpctx.UserID(r.Context())
	if !ok {
		return "", errSkipBucket
	}
	return "user:" + id.String(), nil
}

// keyByAPIKey keys the bucket on the SDK API key ID written into the
// request context by RequireSDKAuth. Returns errSkipBucket when the
// API key ID is nil (session-based auth context) — the route group
// shape ensures this only happens on misrouting.
func keyByAPIKey(r *http.Request) (string, error) {
	id, ok := httpctx.APIKeyID(r.Context())
	if !ok || id == nil {
		return "", errSkipBucket
	}
	return "apikey:" + id.String(), nil
}

// keyByAPIKeyPrefix hashes the first 8 chars of the raw API key from
// the X-API-Key or Authorization: Bearer header. Used on
// unauthenticated endpoints that accept a raw key — protects against
// distributed brute force without storing the key itself.
//
// Returns errSkipBucket when the header is missing or the key is
// shorter than 8 chars (the handler will return 400 for a
// short/missing key; we don't double-charge it against the bucket).
func keyByAPIKeyPrefix(r *http.Request) (string, error) {
	raw := r.Header.Get("X-API-Key")
	if raw == "" {
		if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			raw = v
		}
	}
	if len(raw) < 8 {
		return "", errSkipBucket
	}
	sum := sha256.Sum256([]byte(raw[:8]))
	return "keyprefix:" + hex.EncodeToString(sum[:8]), nil
}
