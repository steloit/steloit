package observability

import (
	"context"
	"fmt"

	"brokle/internal/core/domain/observability"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// genaiEventsRepository implements ClickHouse persistence for OTLP GenAI events
type genaiEventsRepository struct {
	db clickhouse.Conn
}

// NewGenAIEventsRepository creates a new GenAI events repository instance
func NewGenAIEventsRepository(db clickhouse.Conn) observability.GenAIEventsRepository {
	return &genaiEventsRepository{db: db}
}

// CreateGenAIEventBatch inserts multiple GenAI events in a single batch
// GenAI events capture detailed inference operations and evaluation results
func (r *genaiEventsRepository) CreateGenAIEventBatch(ctx context.Context, events []*observability.GenAIEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := r.db.PrepareBatch(ctx, `
		INSERT INTO otel_genai_events (
			timestamp,
			event_name,
			trace_id, span_id,
			operation_name, model_name, provider_name,
			input_messages, output_messages,
			input_tokens, output_tokens,
			temperature, top_p, max_tokens,
			finish_reasons, response_id,
			evaluation_name, evaluation_score, evaluation_label, evaluation_explanation,
			project_id, user_id, session_id
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, event := range events {
		err = batch.Append(
			event.Timestamp,
			event.EventName,
			event.TraceID,
			event.SpanID,
			event.OperationName,
			event.ModelName,
			event.ProviderName,
			event.InputMessages,
			event.OutputMessages,
			event.InputTokens,
			event.OutputTokens,
			event.Temperature, // Nullable(Float32) - pointer type
			event.TopP,        // Nullable(Float32) - pointer type
			event.MaxTokens,   // Nullable(UInt32) - pointer type
			event.FinishReasons,
			event.ResponseID,
			event.EvaluationName,        // Nullable(String) - pointer type
			event.EvaluationScore,       // Nullable(Float32) - pointer type
			event.EvaluationLabel,       // Nullable(String) - pointer type
			event.EvaluationExplanation, // Nullable(String) - pointer type
			event.ProjectID,
			event.UserID,
			event.SessionID,
		)
		if err != nil {
			return fmt.Errorf("append to batch: %w", err)
		}
	}

	return batch.Send()
}
