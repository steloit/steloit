package grpc

import (
	"context"
	"time"

	"log/slog"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/infrastructure/streams"
	"brokle/pkg/uid"
)

// OTLPLogsHandler implements OTLP LogsService gRPC server
type OTLPLogsHandler struct {
	collogspb.UnimplementedLogsServiceServer

	streamProducer  *streams.TelemetryStreamProducer
	logsConverter   *obsServices.OTLPLogsConverterService
	eventsConverter *obsServices.OTLPEventsConverterService
	logger          *slog.Logger
}

// NewOTLPLogsHandler creates a new gRPC OTLP logs handler
func NewOTLPLogsHandler(
	streamProducer *streams.TelemetryStreamProducer,
	logsConverter *obsServices.OTLPLogsConverterService,
	eventsConverter *obsServices.OTLPEventsConverterService,
	logger *slog.Logger,
) *OTLPLogsHandler {
	return &OTLPLogsHandler{
		streamProducer:  streamProducer,
		logsConverter:   logsConverter,
		eventsConverter: eventsConverter,
		logger:          logger,
	}
}

// Export implements LogsService.Export (standard OTLP gRPC method)
func (h *OTLPLogsHandler) Export(
	ctx context.Context,
	req *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	// Extract project ID from authenticated context (set by auth interceptor)
	projectIDPtr, err := extractProjectIDFromContext(ctx)
	if err != nil {
		h.logger.Error("Project ID not found in gRPC context", "error", err)
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	projectID := projectIDPtr.String()

	// Validate request has resource logs
	if len(req.ResourceLogs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "OTLP request must contain at least one resource log")
	}

	logCount := countLogs(req)
	h.logger.Debug("Received gRPC OTLP logs request",
		"project_id", projectID,
		"resource_logs", len(req.ResourceLogs),
		"total_logs", logCount,
	)

	// Convert OTLP logs to Brokle telemetry events
	logsData := &logspb.LogsData{
		ResourceLogs: req.ResourceLogs,
	}

	// Convert logs (regular log records)
	logEvents, err := h.logsConverter.ConvertLogsRequest(ctx, logsData, *projectIDPtr)
	if err != nil {
		h.logger.Error("Failed to convert OTLP logs to Brokle events", "error", err)
		return nil, status.Error(codes.Internal, "failed to process OTLP logs")
	}

	// Convert GenAI events (structured log records with GenAI event names)
	genaiEvents, err := h.eventsConverter.ConvertGenAIEventsRequest(ctx, logsData, *projectIDPtr)
	if err != nil {
		h.logger.Error("Failed to convert GenAI events to Brokle events", "error", err)
		return nil, status.Error(codes.Internal, "failed to process GenAI events")
	}

	// Combine all events
	brokleEvents := append(logEvents, genaiEvents...)

	h.logger.Debug("Converted gRPC OTLP logs to Brokle events",
		"project_id", projectID,
		"otlp_logs", logCount,
		"log_events", len(logEvents),
		"genai_events", len(genaiEvents),
		"total_events", len(brokleEvents),
	)

	// Logs and events don't require deduplication (no unique ID constraints)
	// Convert events to stream format
	eventData := make([]streams.TelemetryEventData, 0, len(brokleEvents))
	for _, event := range brokleEvents {
		eventData = append(eventData, streams.TelemetryEventData{
			EventID:      event.EventID,
			SpanID:       event.SpanID,
			TraceID:      event.TraceID,
			EventType:    string(event.EventType),
			EventPayload: event.Payload,
		})
	}

	batchID := uid.New()
	h.logger.Info("Publishing gRPC OTLP logs batch to Redis streams",
		"batch_id", batchID.String(),
		"events", len(eventData),
	)

	// Publish to Redis Streams for async worker processing
	streamMsg := &streams.TelemetryStreamMessage{
		BatchID:   batchID,
		ProjectID: *projectIDPtr,
		Events:    eventData,
		Metadata: map[string]any{
			"source":        "otlp-grpc-logs",
			"resource_logs": len(req.ResourceLogs),
			"total_logs":    logCount,
			"log_events":    len(logEvents),
			"genai_events":  len(genaiEvents),
		},
		Timestamp: time.Now(),
	}

	streamID, err := h.streamProducer.PublishBatch(ctx, streamMsg)
	if err != nil {
		h.logger.Error("Failed to publish gRPC OTLP logs batch to Redis streams", "error", err)
		return nil, status.Error(codes.Internal, "failed to publish logs events")
	}

	h.logger.Info("gRPC OTLP logs published successfully",
		"batch_id", batchID.String(),
		"stream_id", streamID,
		"events", len(eventData),
		"project_id", projectID,
	)

	// Return standard OTLP success response
	return &collogspb.ExportLogsServiceResponse{}, nil
}

// countLogs counts total logs across all resource logs
func countLogs(req *collogspb.ExportLogsServiceRequest) int {
	count := 0
	for _, rl := range req.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			count += len(sl.LogRecords)
		}
	}
	return count
}

// RegisterOTLPLogsService registers the OTLP logs handler with gRPC server
func RegisterOTLPLogsService(server *grpc.Server, handler *OTLPLogsHandler) {
	collogspb.RegisterLogsServiceServer(server, handler)
}
