package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"brokle/internal/infrastructure/database"

	"github.com/redis/go-redis/v9"
)

// CacheRepository implements caching operations using Redis
type CacheRepository struct {
	db *database.RedisDB
}

// NewCacheRepository creates a new cache repository
func NewCacheRepository(db *database.RedisDB) *CacheRepository {
	return &CacheRepository{
		db: db,
	}
}

// Set stores a value in cache with expiration
func (r *CacheRepository) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	if err := r.db.Set(ctx, key, data, expiration); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// Get retrieves a value from cache
func (r *CacheRepository) Get(ctx context.Context, key string, dest any) error {
	data, err := r.db.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get cache: %w", err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal cache value: %w", err)
	}

	return nil
}

// Delete removes keys from cache
func (r *CacheRepository) Delete(ctx context.Context, keys ...string) error {
	return r.db.Delete(ctx, keys...)
}

// Exists checks if key exists in cache
func (r *CacheRepository) Exists(ctx context.Context, key string) (bool, error) {
	count, err := r.db.Exists(ctx, key)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SetHash stores hash fields
func (r *CacheRepository) SetHash(ctx context.Context, key string, fields map[string]any) error {
	values := make([]any, 0, len(fields)*2)
	for field, value := range fields {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal hash field %s: %w", field, err)
		}
		values = append(values, field, string(data))
	}

	return r.db.HSet(ctx, key, values...)
}

// GetHash retrieves hash field
func (r *CacheRepository) GetHash(ctx context.Context, key, field string, dest any) error {
	data, err := r.db.HGet(ctx, key, field)
	if err != nil {
		return fmt.Errorf("failed to get hash field: %w", err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal hash value: %w", err)
	}

	return nil
}

// GetAllHash retrieves all hash fields
func (r *CacheRepository) GetAllHash(ctx context.Context, key string) (map[string]any, error) {
	data, err := r.db.HGetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get all hash fields: %w", err)
	}

	result := make(map[string]any)
	for field, value := range data {
		var obj any
		if err := json.Unmarshal([]byte(value), &obj); err != nil {
			// If unmarshal fails, store as string
			result[field] = value
		} else {
			result[field] = obj
		}
	}

	return result, nil
}

// Increment atomically increments a counter
func (r *CacheRepository) Increment(ctx context.Context, key string) (int64, error) {
	return r.db.Increment(ctx, key)
}

// IncrementBy atomically increments a counter by value
func (r *CacheRepository) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	return r.db.IncrementBy(ctx, key, value)
}

// SetExpire sets expiration for a key
func (r *CacheRepository) SetExpire(ctx context.Context, key string, expiration time.Duration) error {
	return r.db.Expire(ctx, key, expiration)
}

// AddToSortedSet adds members to sorted set (for rankings, leaderboards)
func (r *CacheRepository) AddToSortedSet(ctx context.Context, key string, score float64, member string) error {
	return r.db.ZAdd(ctx, key, redis.Z{Score: score, Member: member})
}

// GetSortedSetRange gets members from sorted set
func (r *CacheRepository) GetSortedSetRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.db.ZRange(ctx, key, start, stop)
}

// Session management methods

// CreateSession stores session data
func (r *CacheRepository) CreateSession(ctx context.Context, sessionID string, data any, expiration time.Duration) error {
	key := r.sessionKey(sessionID)
	return r.Set(ctx, key, data, expiration)
}

// GetSession retrieves session data
func (r *CacheRepository) GetSession(ctx context.Context, sessionID string, dest any) error {
	key := r.sessionKey(sessionID)
	return r.Get(ctx, key, dest)
}

// DeleteSession removes session
func (r *CacheRepository) DeleteSession(ctx context.Context, sessionID string) error {
	key := r.sessionKey(sessionID)
	return r.Delete(ctx, key)
}

// RefreshSession extends session expiration
func (r *CacheRepository) RefreshSession(ctx context.Context, sessionID string, expiration time.Duration) error {
	key := r.sessionKey(sessionID)
	return r.SetExpire(ctx, key, expiration)
}

// Rate limiting methods

// CheckRateLimit checks if request is within rate limit
func (r *CacheRepository) CheckRateLimit(ctx context.Context, key string, limit int64, window time.Duration) (bool, int64, error) {
	rateLimitKey := r.rateLimitKey(key)

	current, err := r.db.Increment(ctx, rateLimitKey)
	if err != nil {
		return false, 0, fmt.Errorf("failed to increment rate limit counter: %w", err)
	}

	// Set expiration on first request
	if current == 1 {
		if err := r.db.Expire(ctx, rateLimitKey, window); err != nil {
			return false, current, fmt.Errorf("failed to set rate limit expiration: %w", err)
		}
	}

	allowed := current <= limit
	return allowed, current, nil
}

// API Key caching methods

// CacheAPIKey caches API key validation result
func (r *CacheRepository) CacheAPIKey(ctx context.Context, apiKey string, keyData any, expiration time.Duration) error {
	key := r.apiKeyKey(apiKey)
	return r.Set(ctx, key, keyData, expiration)
}

// GetCachedAPIKey retrieves cached API key data
func (r *CacheRepository) GetCachedAPIKey(ctx context.Context, apiKey string, dest any) error {
	key := r.apiKeyKey(apiKey)
	return r.Get(ctx, key, dest)
}

// InvalidateAPIKey removes API key from cache
func (r *CacheRepository) InvalidateAPIKey(ctx context.Context, apiKey string) error {
	key := r.apiKeyKey(apiKey)
	return r.Delete(ctx, key)
}

// Helper methods for key generation

func (r *CacheRepository) sessionKey(sessionID string) string {
	return "session:" + sessionID
}

func (r *CacheRepository) rateLimitKey(identifier string) string {
	return "rate_limit:" + identifier
}

func (r *CacheRepository) apiKeyKey(apiKey string) string {
	return "api_key:" + apiKey
}

func (r *CacheRepository) userKey(userID string) string {
	return "user:" + userID
}
