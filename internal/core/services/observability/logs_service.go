package observability

import (
	"context"
	"log/slog"

	"brokle/internal/core/domain/observability"
)

// LogsService implements business logic for OTLP logs management
type LogsService struct {
	logsRepo observability.LogsRepository
	logger   *slog.Logger
}

// NewLogsService creates a new logs service instance
func NewLogsService(
	logsRepo observability.LogsRepository,
	logger *slog.Logger,
) *LogsService {
	return &LogsService{
		logsRepo: logsRepo,
		logger:   logger,
	}
}

// CreateLogBatch creates multiple logs in a single batch
// Used by workers for efficient OTLP logs processing
func (s *LogsService) CreateLogBatch(ctx context.Context, logs []*observability.Log) error {
	if len(logs) == 0 {
		return nil
	}

	// Delegate to repository (no validation needed - converter already validated)
	return s.logsRepo.CreateLogBatch(ctx, logs)
}
