package observability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"

	"brokle/internal/core/domain/observability"
	"brokle/internal/infrastructure/database"
)

// telemetryDeduplicationRepository implements Redis-based deduplication for OTLP telemetry
// Uses Redis SET NX for atomic claims and TTL for auto-expiry (no PostgreSQL dependency)
type telemetryDeduplicationRepository struct {
	redis *database.RedisDB
}

// NewTelemetryDeduplicationRepository creates a new Redis-based deduplication repository
func NewTelemetryDeduplicationRepository(redis *database.RedisDB) *telemetryDeduplicationRepository {
	return &telemetryDeduplicationRepository{
		redis: redis,
	}
}

// ClaimEvents atomically claims dedup IDs using Redis SET NX (set if not exists)
// Returns: (claimedIDs, duplicateIDs, error)
func (r *telemetryDeduplicationRepository) ClaimEvents(ctx context.Context, projectID uuid.UUID, batchID uuid.UUID, dedupIDs []string, ttl time.Duration) ([]string, []string, error) {
	if len(dedupIDs) == 0 {
		return nil, nil, nil
	}

	if projectID == uuid.Nil {
		return nil, nil, errors.New("project ID cannot be zero")
	}

	if batchID == uuid.Nil {
		return nil, nil, errors.New("batch ID cannot be zero")
	}

	// Use Redis pipeline with SetNX for atomic claim
	pipe := r.redis.Client.Pipeline()
	cmds := make([]*redis.BoolCmd, len(dedupIDs))

	for i, dedupID := range dedupIDs {
		redisKey := r.buildRedisKey(dedupID)
		// SetNX returns true if key was set (didn't exist), false if key already exists
		// Store batch ID as string value
		cmds[i] = pipe.SetNX(ctx, redisKey, batchID.String(), ttl)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, nil, fmt.Errorf("failed to claim events: %w", err)
	}

	// Collect results - claimed vs duplicates
	claimed := make([]string, 0, len(dedupIDs))
	duplicates := make([]string, 0)

	for i, cmd := range cmds {
		success, cmdErr := cmd.Result()
		if cmdErr != nil {
			// On error, treat as not claimed (safe default)
			duplicates = append(duplicates, dedupIDs[i])
			continue
		}

		if success {
			// SetNX succeeded - we claimed this event
			claimed = append(claimed, dedupIDs[i])
		} else {
			// SetNX failed - event already exists (duplicate)
			duplicates = append(duplicates, dedupIDs[i])
		}
	}

	return claimed, duplicates, nil
}

// ReleaseEvents removes claimed dedup IDs (for rollback on publish failure)
func (r *telemetryDeduplicationRepository) ReleaseEvents(ctx context.Context, dedupIDs []string) error {
	if len(dedupIDs) == 0 {
		return nil
	}

	// Use Redis pipeline for batch deletion
	pipe := r.redis.Client.Pipeline()

	for _, dedupID := range dedupIDs {
		redisKey := r.buildRedisKey(dedupID)
		pipe.Del(ctx, redisKey)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to release events: %w", err)
	}

	return nil
}

// Create creates a new telemetry event deduplication entry in Redis with auto-expiry
func (r *telemetryDeduplicationRepository) Create(ctx context.Context, dedup *observability.TelemetryEventDeduplication) error {
	if dedup == nil {
		return errors.New("dedup entry cannot be nil")
	}

	if dedup.EventID == "" {
		return errors.New("event ID is required")
	}

	if dedup.BatchID == uuid.Nil {
		return errors.New("batch ID is required")
	}

	// Calculate TTL
	now := time.Now()
	ttl := dedup.ExpiresAt.Sub(now)

	if ttl <= 0 {
		return errors.New("deduplication entry already expired")
	}

	// Store in Redis with auto-expiry (SETEX)
	// Store batch ID as string value
	redisKey := r.buildRedisKey(dedup.EventID)
	err := r.redis.Set(ctx, redisKey, dedup.BatchID.String(), ttl)
	if err != nil {
		return fmt.Errorf("failed to create deduplication entry: %w", err)
	}

	return nil
}

// Exists checks if a deduplication entry exists for the given event ID
func (r *telemetryDeduplicationRepository) Exists(ctx context.Context, eventID string) (bool, error) {
	redisKey := r.buildRedisKey(eventID)

	exists, err := r.redis.Exists(ctx, redisKey)
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return exists > 0, nil
}

// CheckDuplicate checks if an event ID is a duplicate (alias for Exists for interface compatibility)
func (r *telemetryDeduplicationRepository) CheckDuplicate(ctx context.Context, eventID string) (bool, error) {
	return r.Exists(ctx, eventID)
}

// RegisterEvent registers a new event for deduplication (alias for Create with simplified params)
func (r *telemetryDeduplicationRepository) RegisterEvent(ctx context.Context, dedupID string, batchID uuid.UUID, projectID uuid.UUID, ttl time.Duration) error {
	dedup := &observability.TelemetryEventDeduplication{
		EventID:     dedupID,
		BatchID:     batchID,
		ProjectID:   projectID,
		FirstSeenAt: time.Now(),
		ExpiresAt:   time.Now().Add(ttl),
	}
	return r.Create(ctx, dedup)
}

// CheckBatchDuplicates checks for duplicate dedup IDs in a batch using Redis pipeline
func (r *telemetryDeduplicationRepository) CheckBatchDuplicates(ctx context.Context, dedupIDs []string) ([]string, error) {
	if len(dedupIDs) == 0 {
		return nil, nil
	}

	// Use Redis pipeline for efficient batch checking
	pipe := r.redis.Client.Pipeline()

	// Create EXISTS commands for each dedup ID
	cmds := make([]*redis.IntCmd, len(dedupIDs))
	for i, dedupID := range dedupIDs {
		redisKey := r.buildRedisKey(dedupID)
		cmds[i] = pipe.Exists(ctx, redisKey)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to check batch duplicates: %w", err)
	}

	// Collect duplicates
	var duplicates []string
	for i, cmd := range cmds {
		exists, err := cmd.Result()
		if err != nil {
			// Skip on error, treat as not found
			continue
		}

		if exists > 0 {
			duplicates = append(duplicates, dedupIDs[i])
		}
	}

	return duplicates, nil
}

// CreateBatch creates multiple deduplication entries in Redis using pipeline
func (r *telemetryDeduplicationRepository) CreateBatch(ctx context.Context, entries []*observability.TelemetryEventDeduplication) error {
	if len(entries) == 0 {
		return nil
	}

	// Use Redis pipeline for efficient batch creation
	pipe := r.redis.Client.Pipeline()
	now := time.Now()

	for _, entry := range entries {
		if entry.IsExpired() {
			continue // Skip expired entries
		}

		ttl := entry.ExpiresAt.Sub(now)
		if ttl <= 0 {
			continue // Skip entries that will expire immediately
		}

		redisKey := r.buildRedisKey(entry.EventID)
		pipe.Set(ctx, redisKey, entry.BatchID.String(), ttl)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to batch create deduplication entries: %w", err)
	}

	return nil
}

// Delete deletes a deduplication entry from Redis
func (r *telemetryDeduplicationRepository) Delete(ctx context.Context, dedupID string) error {
	redisKey := r.buildRedisKey(dedupID)

	err := r.redis.Delete(ctx, redisKey)
	if err != nil {
		return fmt.Errorf("failed to delete deduplication entry: %w", err)
	}

	return nil
}

// GetByEventID retrieves batch ID for a dedup ID from Redis
func (r *telemetryDeduplicationRepository) GetByEventID(ctx context.Context, dedupID string) (*observability.TelemetryEventDeduplication, error) {
	redisKey := r.buildRedisKey(dedupID)

	batchIDStr, err := r.redis.Get(ctx, redisKey)
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("deduplication entry not found for dedup ID %s", dedupID)
		}
		return nil, fmt.Errorf("failed to get deduplication entry: %w", err)
	}

	// Get TTL to reconstruct expires_at
	ttl, err := r.redis.Client.TTL(ctx, redisKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get TTL: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	// Parse batch ID from string
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid batch ID stored in Redis: %w", err)
	}

	return &observability.TelemetryEventDeduplication{
		EventID:     dedupID,
		BatchID:     batchID,
		ProjectID:   uuid.UUID{},         // Not stored in Redis (optimization)
		FirstSeenAt: now.Add(-time.Hour), // Approximate (not critical for dedup)
		ExpiresAt:   expiresAt,
	}, nil
}

// CountByProjectID returns count (not supported in Redis-only, returns 0)
// This method is kept for interface compatibility but returns 0 as Redis doesn't store project-level counts
func (r *telemetryDeduplicationRepository) CountByProjectID(ctx context.Context, projectID uuid.UUID) (int64, error) {
	// Not supported in Redis-only mode (would require scanning all keys)
	// Return 0 for compatibility
	return 0, nil
}

// Helper methods

// buildRedisKey builds a Redis key for deduplication (now accepts OTLP hex IDs)
func (r *telemetryDeduplicationRepository) buildRedisKey(dedupID string) string {
	// dedupID is now OTLP composite ID (trace_id:span_id)
	// Use single prefix for all OTLP IDs (globally unique)
	return "dedup:span:" + dedupID
}

// GetStats returns statistics about deduplication cache (approximate)
func (r *telemetryDeduplicationRepository) GetStats(ctx context.Context) (map[string]any, error) {
	// Get approximate count using SCAN (non-blocking)
	var cursor uint64
	var count int64

	// Sample first 100 keys matching the pattern
	keys, _, err := r.redis.Client.Scan(ctx, cursor, "dedup:span:*", 100).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to scan keys: %w", err)
	}

	count = int64(len(keys))

	return map[string]any{
		"approximate_count": count,
		"pattern":           "dedup:span:*",
		"storage":           "redis",
		"auto_expiry":       true,
	}, nil
}

// Interface compatibility methods (not supported in Redis-only mode)

// ExistsInBatch checks if event exists in specific batch (not supported in Redis-only)
func (r *telemetryDeduplicationRepository) ExistsInBatch(ctx context.Context, eventID string, batchID string) (bool, error) {
	// Not supported - would need to store batch_id in Redis value
	return false, errors.New("ExistsInBatch not supported in Redis-only mode")
}

// ExistsWithRedisCheck checks existence with Redis flag (Redis-only always uses Redis)
func (r *telemetryDeduplicationRepository) ExistsWithRedisCheck(ctx context.Context, dedupID string) (bool, bool, error) {
	exists, err := r.Exists(ctx, dedupID)
	// Second return value indicates "found in Redis" - always true for Redis-only
	return exists, exists, err
}

// StoreInRedis stores event in Redis (equivalent to Create)
func (r *telemetryDeduplicationRepository) StoreInRedis(ctx context.Context, dedupID string, batchID uuid.UUID, ttl time.Duration) error {
	dedup := &observability.TelemetryEventDeduplication{
		EventID:   dedupID,
		BatchID:   batchID,
		ProjectID: uuid.UUID{}, // Not required for Redis storage
		ExpiresAt: time.Now().Add(ttl),
	}
	return r.Create(ctx, dedup)
}

// GetFromRedis retrieves batch ID from Redis
func (r *telemetryDeduplicationRepository) GetFromRedis(ctx context.Context, dedupID string) (*uuid.UUID, error) {
	dedup, err := r.GetByEventID(ctx, dedupID)
	if err != nil {
		return nil, err
	}
	return &dedup.BatchID, nil
}

// CleanupExpired removes expired entries (not needed - Redis handles auto-expiry)
func (r *telemetryDeduplicationRepository) CleanupExpired(ctx context.Context) (int64, error) {
	// Redis handles auto-expiry via TTL
	return 0, nil
}

// GetExpiredEntries returns expired entries (not supported - Redis auto-expires)
func (r *telemetryDeduplicationRepository) GetExpiredEntries(ctx context.Context, limit int) ([]*observability.TelemetryEventDeduplication, error) {
	// Not supported - Redis auto-expires entries
	return nil, nil
}

// BatchCleanup batch cleanup (not needed - Redis handles auto-expiry)
func (r *telemetryDeduplicationRepository) BatchCleanup(ctx context.Context, olderThan time.Time, batchSize int) (int64, error) {
	// Redis handles auto-expiry via TTL
	return 0, nil
}

// GetByProjectID retrieves entries by project (not supported efficiently in Redis-only)
func (r *telemetryDeduplicationRepository) GetByProjectID(ctx context.Context, projectID string, limit, offset int) ([]*observability.TelemetryEventDeduplication, error) {
	// Not supported - would require scanning all keys
	return nil, errors.New("GetByProjectID not supported in Redis-only mode")
}

// CleanupByProjectID cleanup by project (not needed - Redis handles auto-expiry)
func (r *telemetryDeduplicationRepository) CleanupByProjectID(ctx context.Context, projectID string, olderThan time.Time) (int64, error) {
	// Redis handles auto-expiry via TTL
	return 0, nil
}
