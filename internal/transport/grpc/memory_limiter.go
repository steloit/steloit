package grpc

import (
	"context"
	"runtime"

	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MemoryLimiterConfig holds memory limiter configuration
// Follows OTEL Collector memory_limiter processor semantics:
// - LimitMiB: Soft limit where graceful rejection starts
// - SpikeLimitMiB: Additional headroom ABOVE soft limit for traffic spikes
// - Hard limit = LimitMiB + SpikeLimitMiB (e.g., 1500 + 512 = 2012 MiB)
type MemoryLimiterConfig struct {
	LimitMiB      int64 // Soft limit (start rejecting requests)
	SpikeLimitMiB int64 // Headroom above soft limit for spikes (additive, not absolute)
}

// DefaultMemoryLimiterConfig returns OTEL Collector-compatible defaults
func DefaultMemoryLimiterConfig() *MemoryLimiterConfig {
	return &MemoryLimiterConfig{
		LimitMiB:      1500, // 1.5GB soft limit
		SpikeLimitMiB: 512,  // 512MB spike limit
	}
}

// MemoryLimiterInterceptor prevents OOM during traffic spikes
// Follows OTEL Collector memory_limiter processor pattern
// Returns ResourceExhausted (429) when memory exceeds limits
func MemoryLimiterInterceptor(cfg *MemoryLimiterConfig, logger *slog.Logger) grpc.UnaryServerInterceptor {
	if cfg == nil {
		cfg = DefaultMemoryLimiterConfig()
	}

	// Calculate hard limit (soft + spike headroom) per OTEL Collector semantics
	hardLimitMiB := cfg.LimitMiB + cfg.SpikeLimitMiB

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Check memory before processing request
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		usedMiB := int64(memStats.Alloc / 1024 / 1024)

		// Hard limit: Immediate rejection if total capacity exceeded (soft + spike)
		if usedMiB > hardLimitMiB {
			logger.Warn("Memory hard limit exceeded, rejecting request",
				"used_mib", usedMiB,
				"hard_limit_mib", hardLimitMiB,
				"soft_limit_mib", cfg.LimitMiB,
				"method", info.FullMethod,
			)
			return nil, status.Error(
				codes.ResourceExhausted,
				"server memory limit exceeded, try again later",
			)
		}

		// Soft limit: Graceful rejection when in spike zone (between soft and hard)
		if usedMiB > cfg.LimitMiB {
			logger.Warn("Memory soft limit exceeded, rejecting request",
				"used_mib", usedMiB,
				"soft_limit_mib", cfg.LimitMiB,
				"hard_limit_mib", hardLimitMiB,
				"method", info.FullMethod,
			)
			return nil, status.Error(
				codes.ResourceExhausted,
				"server memory limit exceeded, try again later",
			)
		}

		// Memory OK, proceed with request
		logger.Debug("Memory check passed",
			"used_mib", usedMiB,
			"soft_limit_mib", cfg.LimitMiB,
			"hard_limit_mib", hardLimitMiB,
		)

		return handler(ctx, req)
	}
}
