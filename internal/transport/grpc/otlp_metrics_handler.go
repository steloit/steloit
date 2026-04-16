package grpc

import (
	"context"
	"time"

	"log/slog"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/infrastructure/streams"
	"brokle/pkg/uid"
)

// OTLPMetricsHandler implements OTLP MetricsService gRPC server
type OTLPMetricsHandler struct {
	colmetricspb.UnimplementedMetricsServiceServer

	streamProducer   *streams.TelemetryStreamProducer
	metricsConverter *obsServices.OTLPMetricsConverterService
	logger           *slog.Logger
}

// NewOTLPMetricsHandler creates a new gRPC OTLP metrics handler
func NewOTLPMetricsHandler(
	streamProducer *streams.TelemetryStreamProducer,
	metricsConverter *obsServices.OTLPMetricsConverterService,
	logger *slog.Logger,
) *OTLPMetricsHandler {
	return &OTLPMetricsHandler{
		streamProducer:   streamProducer,
		metricsConverter: metricsConverter,
		logger:           logger,
	}
}

// Export implements MetricsService.Export (standard OTLP gRPC method)
func (h *OTLPMetricsHandler) Export(
	ctx context.Context,
	req *colmetricspb.ExportMetricsServiceRequest,
) (*colmetricspb.ExportMetricsServiceResponse, error) {
	// Extract project ID from authenticated context (set by auth interceptor)
	projectIDPtr, err := extractProjectIDFromContext(ctx)
	if err != nil {
		h.logger.Error("Project ID not found in gRPC context", "error", err)
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	projectID := projectIDPtr.String()

	// Validate request has resource metrics
	if len(req.ResourceMetrics) == 0 {
		return nil, status.Error(codes.InvalidArgument, "OTLP request must contain at least one resource metric")
	}

	metricCount := countMetrics(req)
	h.logger.Debug("Received gRPC OTLP metrics request",
		"project_id", projectID,
		"resource_metrics", len(req.ResourceMetrics),
		"total_metrics", metricCount,
	)

	// Convert OTLP metrics to Brokle telemetry events
	metricsData := &metricspb.MetricsData{
		ResourceMetrics: req.ResourceMetrics,
	}

	brokleEvents, err := h.metricsConverter.ConvertMetricsRequest(ctx, metricsData, *projectIDPtr)
	if err != nil {
		h.logger.Error("Failed to convert OTLP metrics to Brokle events", "error", err)
		return nil, status.Error(codes.Internal, "failed to process OTLP metrics")
	}

	h.logger.Debug("Converted gRPC OTLP metrics to Brokle events",
		"project_id", projectID,
		"otlp_metrics", metricCount,
		"brokle_events", len(brokleEvents),
	)

	// Metrics don't require deduplication (no unique ID constraints)
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
	h.logger.Info("Publishing gRPC OTLP metrics batch to Redis streams",
		"batch_id", batchID.String(),
		"events", len(eventData),
	)

	// Publish to Redis Streams for async worker processing
	streamMsg := &streams.TelemetryStreamMessage{
		BatchID:   batchID,
		ProjectID: *projectIDPtr,
		Events:    eventData,
		Metadata: map[string]interface{}{
			"source":           "otlp-grpc-metrics",
			"resource_metrics": len(req.ResourceMetrics),
			"total_metrics":    metricCount,
		},
		Timestamp: time.Now(),
	}

	streamID, err := h.streamProducer.PublishBatch(ctx, streamMsg)
	if err != nil {
		h.logger.Error("Failed to publish gRPC OTLP metrics batch to Redis streams", "error", err)
		return nil, status.Error(codes.Internal, "failed to publish metrics events")
	}

	h.logger.Info("gRPC OTLP metrics published successfully",
		"batch_id", batchID.String(),
		"stream_id", streamID,
		"events", len(eventData),
		"project_id", projectID,
	)

	// Return standard OTLP success response
	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}

// countMetrics counts total metrics across all resource metrics
func countMetrics(req *colmetricspb.ExportMetricsServiceRequest) int {
	count := 0
	for _, rm := range req.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			count += len(sm.Metrics)
		}
	}
	return count
}

// RegisterOTLPMetricsService registers the OTLP metrics handler with gRPC server
func RegisterOTLPMetricsService(server *grpc.Server, handler *OTLPMetricsHandler) {
	colmetricspb.RegisterMetricsServiceServer(server, handler)
}
