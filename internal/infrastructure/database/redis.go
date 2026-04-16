package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"brokle/internal/config"
)

// RedisDB represents Redis database connection
type RedisDB struct {
	Client *redis.Client
	config *config.Config
	logger *slog.Logger
}

// NewRedisDB creates a new Redis database connection
func NewRedisDB(cfg *config.Config, logger *slog.Logger) (*RedisDB, error) {
	// Parse Redis URL
	opt, err := redis.ParseURL(cfg.GetRedisURL())
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Configure connection settings
	opt.MaxRetries = 3
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second
	opt.PoolSize = 10
	opt.PoolTimeout = 30 * time.Second
	opt.MaxIdleConns = 5

	// Create Redis client
	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	logger.Info("Connected to Redis database")

	return &RedisDB{
		Client: client,
		config: cfg,
		logger: logger,
	}, nil
}

// Close closes the Redis connection
func (r *RedisDB) Close() error {
	r.logger.Info("Closing Redis connection")
	return r.Client.Close()
}

// Health checks Redis health
func (r *RedisDB) Health() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return r.Client.Ping(ctx).Err()
}

// Set sets a key-value pair with optional expiration
func (r *RedisDB) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.Client.Set(ctx, key, value, expiration).Err()
}

// Get gets a value by key
func (r *RedisDB) Get(ctx context.Context, key string) (string, error) {
	return r.Client.Get(ctx, key).Result()
}

// Delete deletes keys
func (r *RedisDB) Delete(ctx context.Context, keys ...string) error {
	return r.Client.Del(ctx, keys...).Err()
}

// Exists checks if key exists
func (r *RedisDB) Exists(ctx context.Context, keys ...string) (int64, error) {
	return r.Client.Exists(ctx, keys...).Result()
}

// Expire sets expiration for a key
func (r *RedisDB) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.Client.Expire(ctx, key, expiration).Err()
}

// Scan iterates keys matching a pattern (cursor-based iteration)
func (r *RedisDB) Scan(ctx context.Context, cursor uint64, pattern string, count int64) (keys []string, nextCursor uint64, err error) {
	return r.Client.Scan(ctx, cursor, pattern, count).Result()
}

// HSet sets hash field
func (r *RedisDB) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.Client.HSet(ctx, key, values...).Err()
}

// HGet gets hash field
func (r *RedisDB) HGet(ctx context.Context, key, field string) (string, error) {
	return r.Client.HGet(ctx, key, field).Result()
}

// HGetAll gets all hash fields
func (r *RedisDB) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.Client.HGetAll(ctx, key).Result()
}

// HDel deletes hash fields
func (r *RedisDB) HDel(ctx context.Context, key string, fields ...string) error {
	return r.Client.HDel(ctx, key, fields...).Err()
}

// ZAdd adds members to sorted set
func (r *RedisDB) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	return r.Client.ZAdd(ctx, key, members...).Err()
}

// ZRange gets sorted set members by range
func (r *RedisDB) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.Client.ZRange(ctx, key, start, stop).Result()
}

// ZRangeWithScores gets sorted set members with scores
func (r *RedisDB) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.Client.ZRangeWithScores(ctx, key, start, stop).Result()
}

// Increment increments a key
func (r *RedisDB) Increment(ctx context.Context, key string) (int64, error) {
	return r.Client.Incr(ctx, key).Result()
}

// IncrementBy increments a key by value
func (r *RedisDB) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	return r.Client.IncrBy(ctx, key, value).Result()
}

// GetStats returns Redis connection pool statistics
func (r *RedisDB) GetStats() *redis.PoolStats {
	return r.Client.PoolStats()
}

// Redis Streams Methods

// XAdd adds a message to a Redis Stream
func (r *RedisDB) XAdd(ctx context.Context, args *redis.XAddArgs) (string, error) {
	return r.Client.XAdd(ctx, args).Result()
}

// XReadGroup reads messages from a Redis Stream using consumer groups
func (r *RedisDB) XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error) {
	return r.Client.XReadGroup(ctx, args).Result()
}

// XAck acknowledges message processing in a consumer group
func (r *RedisDB) XAck(ctx context.Context, stream, group string, ids ...string) error {
	return r.Client.XAck(ctx, stream, group, ids...).Err()
}

// XGroupCreate creates a consumer group for a stream
func (r *RedisDB) XGroupCreate(ctx context.Context, stream, group, start string) error {
	return r.Client.XGroupCreate(ctx, stream, group, start).Err()
}

// XGroupCreateMkStream creates a consumer group and the stream if it doesn't exist
func (r *RedisDB) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	return r.Client.XGroupCreateMkStream(ctx, stream, group, start).Err()
}

// XLen returns the number of entries in a stream
func (r *RedisDB) XLen(ctx context.Context, stream string) (int64, error) {
	return r.Client.XLen(ctx, stream).Result()
}

// XInfoStream returns information about a stream
func (r *RedisDB) XInfoStream(ctx context.Context, stream string) (*redis.XInfoStream, error) {
	return r.Client.XInfoStream(ctx, stream).Result()
}

// XInfoGroups returns information about consumer groups for a stream
func (r *RedisDB) XInfoGroups(ctx context.Context, stream string) ([]redis.XInfoGroup, error) {
	return r.Client.XInfoGroups(ctx, stream).Result()
}

// XPending returns pending messages for a consumer group
func (r *RedisDB) XPending(ctx context.Context, stream, group string) (*redis.XPending, error) {
	return r.Client.XPending(ctx, stream, group).Result()
}
