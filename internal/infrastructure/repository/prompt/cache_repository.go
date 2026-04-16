package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"

	"brokle/internal/core/domain/common"
	promptDomain "brokle/internal/core/domain/prompt"
)

const (
	// Cache key prefix for prompts
	promptCachePrefix = "prompt"

	// Default cache TTL
	defaultCacheTTL = 60 * time.Second
)

// cacheRepository implements promptDomain.CacheRepository using Redis
type cacheRepository struct {
	db common.RedisClient
}

// NewCacheRepository creates a new prompt cache repository instance
func NewCacheRepository(db common.RedisClient) promptDomain.CacheRepository {
	return &cacheRepository{
		db: db,
	}
}

// Get retrieves a cached prompt
func (r *cacheRepository) Get(ctx context.Context, key string) (*promptDomain.CachedPrompt, error) {
	data, err := r.db.Get(ctx, key)
	if err != nil {
		if err == redis.Nil {
			return nil, promptDomain.ErrCacheNotFound
		}
		return nil, fmt.Errorf("failed to get prompt from cache: %w", err)
	}

	var cached promptDomain.CachedPrompt
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached prompt: %w", err)
	}

	if cached.IsExpired() {
		return nil, promptDomain.ErrCacheExpired
	}

	return &cached, nil
}

// Set stores a prompt in cache
func (r *cacheRepository) Set(ctx context.Context, key string, prompt *promptDomain.CachedPrompt, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}

	prompt.CachedAt = time.Now()
	prompt.ExpiresAt = prompt.CachedAt.Add(ttl)

	data, err := json.Marshal(prompt)
	if err != nil {
		return fmt.Errorf("failed to marshal prompt for cache: %w", err)
	}

	if err := r.db.Set(ctx, key, data, ttl); err != nil {
		return fmt.Errorf("failed to set prompt in cache: %w", err)
	}

	return nil
}

// Delete removes a prompt from cache
func (r *cacheRepository) Delete(ctx context.Context, key string) error {
	if err := r.db.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete prompt from cache: %w", err)
	}
	return nil
}

// DeleteByPattern removes all cache entries matching a pattern
func (r *cacheRepository) DeleteByPattern(ctx context.Context, pattern string) error {
	// Use SCAN to find matching keys (safer than KEYS for production)
	var cursor uint64
	var keysToDelete []string

	for {
		var keys []string
		var err error
		keys, cursor, err = r.db.Scan(ctx, cursor, pattern, 100)
		if err != nil {
			return fmt.Errorf("failed to scan cache keys: %w", err)
		}

		keysToDelete = append(keysToDelete, keys...)

		if cursor == 0 {
			break
		}
	}

	if len(keysToDelete) > 0 {
		if err := r.db.Delete(ctx, keysToDelete...); err != nil {
			return fmt.Errorf("failed to delete cache keys: %w", err)
		}
	}

	return nil
}

// BuildKey generates a cache key for a prompt.
// Format: prompt:{project_id}:{name}:{label_or_version}
// Example: prompt:01HQMYV7D1:greeting:production
// Example: prompt:01HQMYV7D1:greeting:v2
//
// INVARIANT: name and labelOrVersion cannot contain ':' - this is enforced by validation
// patterns in prompt_service.go (promptNamePattern: ^[a-zA-Z][a-zA-Z0-9_-]*$,
// labelPattern: ^[a-z0-9][a-z0-9_.-]*$). Key parsing in ParseCacheKey relies on
// exactly 4 colon-separated parts.
func (r *cacheRepository) BuildKey(projectID uuid.UUID, name string, labelOrVersion string) string {
	return fmt.Sprintf("%s:%s:%s:%s", promptCachePrefix, projectID.String(), name, labelOrVersion)
}

// BuildPatternForPrompt generates a cache pattern for all versions/labels of a prompt
func BuildPatternForPrompt(projectID uuid.UUID, name string) string {
	return fmt.Sprintf("%s:%s:%s:*", promptCachePrefix, projectID.String(), name)
}

// BuildPatternForLabel generates a cache pattern for a specific label across all prompts
func BuildPatternForLabel(projectID uuid.UUID, label string) string {
	return fmt.Sprintf("%s:%s:*:%s", promptCachePrefix, projectID.String(), label)
}

// BuildPatternForProject generates a cache pattern for all prompts in a project
func BuildPatternForProject(projectID uuid.UUID) string {
	return fmt.Sprintf("%s:%s:*", promptCachePrefix, projectID.String())
}

// ParseCacheKey extracts components from a cache key
func ParseCacheKey(key string) (projectID, name, labelOrVersion string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 4 || parts[0] != promptCachePrefix {
		return "", "", "", fmt.Errorf("invalid cache key format: %s", key)
	}
	return parts[1], parts[2], parts[3], nil
}

// GetOrSet is a convenience method that gets from cache or sets using a fetch function
func (r *cacheRepository) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetchFn func() (*promptDomain.CachedPrompt, error)) (*promptDomain.CachedPrompt, bool, error) {
	cached, err := r.Get(ctx, key)
	if err == nil {
		return cached, true, nil
	}

	prompt, err := fetchFn()
	if err != nil {
		return nil, false, err
	}

	_ = r.Set(ctx, key, prompt, ttl) // Don't fail if cache write fails
	return prompt, false, nil
}

// GetStale retrieves a cached prompt even if expired (for stale-while-revalidate)
func (r *cacheRepository) GetStale(ctx context.Context, key string) (*promptDomain.CachedPrompt, error) {
	data, err := r.db.Get(ctx, key)
	if err != nil {
		if err == redis.Nil {
			return nil, promptDomain.ErrCacheNotFound
		}
		return nil, fmt.Errorf("failed to get stale prompt from cache: %w", err)
	}

	var cached promptDomain.CachedPrompt
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stale cached prompt: %w", err)
	}

	return &cached, nil
}

// Refresh updates the TTL of an existing cache entry
func (r *cacheRepository) Refresh(ctx context.Context, key string, ttl time.Duration) error {
	return r.db.Expire(ctx, key, ttl)
}
