package common

import (
	"context"
	"time"
)

// RedisClient defines the interface for Redis operations needed by repositories.
//
// This interface prevents import cycles by allowing repositories to depend on
// an abstraction rather than the concrete database.RedisDB type.
//
// Following the Dependency Inversion Principle:
// - High-level modules (repositories) should not depend on low-level modules (database)
// - Both should depend on abstractions (this interface)
type RedisClient interface {
	// Get retrieves a value by key
	Get(ctx context.Context, key string) (string, error)

	// Set stores a value with TTL
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete removes one or more keys
	Delete(ctx context.Context, keys ...string) error

	// Expire sets TTL on an existing key
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// Scan iterates keys matching a pattern (cursor-based iteration)
	Scan(ctx context.Context, cursor uint64, pattern string, count int64) (keys []string, nextCursor uint64, err error)
}
