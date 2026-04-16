package observability

import (
	"context"
	"log/slog"

	"brokle/internal/core/domain/observability"
)

// GenAIEventsService implements business logic for OTLP GenAI events management
type GenAIEventsService struct {
	genaiEventsRepo observability.GenAIEventsRepository
	logger          *slog.Logger
}

// NewGenAIEventsService creates a new GenAI events service instance
func NewGenAIEventsService(
	genaiEventsRepo observability.GenAIEventsRepository,
	logger *slog.Logger,
) *GenAIEventsService {
	return &GenAIEventsService{
		genaiEventsRepo: genaiEventsRepo,
		logger:          logger,
	}
}

// CreateGenAIEventBatch creates multiple GenAI events in a single batch
// Used by workers for efficient OTLP GenAI events processing
func (s *GenAIEventsService) CreateGenAIEventBatch(ctx context.Context, events []*observability.GenAIEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Delegate to repository (no validation needed - converter already validated)
	return s.genaiEventsRepo.CreateGenAIEventBatch(ctx, events)
}
