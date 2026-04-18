package streams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"

	"brokle/internal/infrastructure/database"
)

// TelemetryStreamMessage represents a telemetry batch message in Redis Stream
type TelemetryStreamMessage struct {
	Timestamp        time.Time              `json:"timestamp"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Events           []TelemetryEventData   `json:"events"`
	ClaimedSpanIDs   []string               `json:"claimed_span_ids"`
	DuplicateSpanIDs []string               `json:"duplicate_span_ids,omitempty"`
	BatchID          uuid.UUID              `json:"batch_id"`
	ProjectID        uuid.UUID              `json:"project_id"`
	OrganizationID   uuid.UUID              `json:"organization_id"`
}

// TelemetryEventData represents individual event data in the stream message
type TelemetryEventData struct {
	EventPayload map[string]any `json:"event_payload"`
	SpanID       string                 `json:"span_id"`
	TraceID      string                 `json:"trace_id"`
	EventType    string                 `json:"event_type"`
	EventID      uuid.UUID              `json:"event_id"`
}

// TelemetryStreamProducer handles publishing telemetry data to Redis Streams
type TelemetryStreamProducer struct {
	redis  *database.RedisDB
	logger *slog.Logger
}

// NewTelemetryStreamProducer creates a new telemetry stream producer
func NewTelemetryStreamProducer(redis *database.RedisDB, logger *slog.Logger) *TelemetryStreamProducer {
	return &TelemetryStreamProducer{
		redis:  redis,
		logger: logger,
	}
}

// PublishBatch publishes a telemetry batch to Redis Stream
// Returns the stream message ID for tracking
func (p *TelemetryStreamProducer) PublishBatch(ctx context.Context, batch *TelemetryStreamMessage) (string, error) {
	if batch == nil {
		return "", errors.New("batch cannot be nil")
	}

	if batch.BatchID == uuid.Nil {
		return "", errors.New("batch ID is required")
	}

	if batch.ProjectID == uuid.Nil {
		return "", errors.New("project ID is required")
	}

	// Use project-specific stream for better distribution and scalability
	streamKey := "telemetry:batches:" + batch.ProjectID.String()

	// Serialize batch data to JSON
	eventData, err := json.Marshal(batch)
	if err != nil {
		return "", fmt.Errorf("failed to marshal batch data: %w", err)
	}

	// Add to Redis Stream without MaxLen to prevent data loss
	// Stream cleanup is handled by TTL (30 days) which safely expires after all messages processed
	// MaxLen would trim unprocessed pending messages during high load or consumer outages
	result, err := p.redis.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]any{
			"batch_id":        batch.BatchID.String(),
			"project_id":      batch.ProjectID.String(),
			"organization_id": batch.OrganizationID.String(),
			"event_count":     len(batch.Events),
			"data":            string(eventData),
			"timestamp":       batch.Timestamp.Unix(),
		},
	}).Result()

	if err != nil {
		return "", fmt.Errorf("failed to add batch to stream: %w", err)
	}

	p.logger.Debug("Published telemetry batch to Redis Stream", "stream_id", result, "batch_id", batch.BatchID.String(), "project_id", batch.ProjectID.String(), "event_count", len(batch.Events), "stream_key", streamKey)

	// Set stream TTL for GDPR compliance (30 days)
	if err := p.SetStreamTTL(ctx, batch.ProjectID, 30*24*time.Hour); err != nil {
		p.logger.Warn("Failed to set stream TTL (GDPR compliance)", "error", err)
		// Don't return error - TTL is best-effort for compliance
	}

	return result, nil
}

// GetStreamInfo retrieves information about a stream
func (p *TelemetryStreamProducer) GetStreamInfo(ctx context.Context, projectID uuid.UUID) (*redis.XInfoStream, error) {
	streamKey := "telemetry:batches:" + projectID.String()

	info, err := p.redis.Client.XInfoStream(ctx, streamKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("stream not found for project %s", projectID)
		}
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	return info, nil
}

// GetStreamLength returns the number of messages in a stream
func (p *TelemetryStreamProducer) GetStreamLength(ctx context.Context, projectID uuid.UUID) (int64, error) {
	streamKey := "telemetry:batches:" + projectID.String()

	length, err := p.redis.Client.XLen(ctx, streamKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get stream length: %w", err)
	}

	return length, nil
}

// SetStreamTTL sets a TTL on the stream for automatic expiration (GDPR compliance)
// Default: 30 days (720 hours) for GDPR Article 17 compliance (right to erasure)
func (p *TelemetryStreamProducer) SetStreamTTL(ctx context.Context, projectID uuid.UUID, ttl time.Duration) error {
	streamKey := "telemetry:batches:" + projectID.String()

	// Set TTL on stream key
	err := p.redis.Client.Expire(ctx, streamKey, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set stream TTL: %w", err)
	}

	p.logger.Debug("Set stream TTL for GDPR compliance", "project_id", projectID, "stream_key", streamKey, "ttl_hours", ttl.Hours())

	return nil
}

// DeleteStream removes a stream (use with caution)
func (p *TelemetryStreamProducer) DeleteStream(ctx context.Context, projectID uuid.UUID) error {
	streamKey := "telemetry:batches:" + projectID.String()

	err := p.redis.Client.Del(ctx, streamKey).Err()
	if err != nil {
		return fmt.Errorf("failed to delete stream: %w", err)
	}

	p.logger.Warn("Deleted telemetry stream", "project_id", projectID.String(), "stream_key", streamKey)

	return nil
}
