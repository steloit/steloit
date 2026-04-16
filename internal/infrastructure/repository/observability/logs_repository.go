package observability

import (
	"context"
	"fmt"

	"brokle/internal/core/domain/observability"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// logsRepository implements ClickHouse persistence for OTLP logs
type logsRepository struct {
	db clickhouse.Conn
}

// NewLogsRepository creates a new logs repository instance
func NewLogsRepository(db clickhouse.Conn) observability.LogsRepository {
	return &logsRepository{db: db}
}

// CreateLogBatch inserts multiple logs in a single batch
func (r *logsRepository) CreateLogBatch(ctx context.Context, logs []*observability.Log) error {
	if len(logs) == 0 {
		return nil
	}

	batch, err := r.db.PrepareBatch(ctx, `
		INSERT INTO otel_logs (
			timestamp, observed_timestamp,
			trace_id, span_id, trace_flags,
			severity_text, severity_number,
			body,
			resource_attributes,
			scope_name, scope_attributes,
			log_attributes,
			project_id
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, log := range logs {
		err = batch.Append(
			log.Timestamp,
			log.ObservedTimestamp,
			log.TraceID, // Hex string (32 chars) - may be empty if no trace correlation
			log.SpanID,  // Hex string (16 chars) - may be empty if no span correlation
			log.TraceFlags,
			log.SeverityText,
			log.SeverityNumber,
			log.Body, // AnyValue → string (JSON-encoded for complex types)
			log.ResourceAttributes,
			log.ScopeName,
			log.ScopeAttributes,
			log.LogAttributes,
			log.ProjectID,
		)
		if err != nil {
			return fmt.Errorf("append to batch: %w", err)
		}
	}

	return batch.Send()
}
