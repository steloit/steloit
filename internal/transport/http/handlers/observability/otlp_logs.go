package observability

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/infrastructure/streams"
	"brokle/internal/transport/http/middleware"
	"brokle/pkg/response"
	"brokle/pkg/uid"
)

// OTLPLogsHandler handles OTLP logs HTTP requests
type OTLPLogsHandler struct {
	streamProducer  *streams.TelemetryStreamProducer
	logsConverter   *obsServices.OTLPLogsConverterService
	eventsConverter *obsServices.OTLPEventsConverterService
	logger          *slog.Logger
}

// NewOTLPLogsHandler creates a new OTLP logs handler
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

// HandleLogs handles POST /v1/logs
// @Summary OTLP logs ingestion endpoint (OpenTelemetry spec compliant)
// @Description Accepts OpenTelemetry Protocol (OTLP) logs in JSON or Protobuf format
// @Tags SDK - OTLP
// @Accept json
// @Accept application/x-protobuf
// @Produce json
// @Security ApiKeyAuth
// @Param request body observability.OTLPLogsRequest true "OTLP logs export request"
// @Success 200 {object} response.APIResponse{data=map[string]any} "Logs accepted"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid OTLP request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Invalid or missing API key"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /v1/logs [post]
func (h *OTLPLogsHandler) HandleLogs(c *gin.Context) {
	ctx := c.Request.Context()

	projectID := middleware.MustGetProjectID(c)

	// Validate Content-Type header (OTLP specification requires explicit Content-Type)
	contentType := c.GetHeader("Content-Type")
	validContentType := strings.Contains(contentType, "application/x-protobuf") ||
		strings.Contains(contentType, "application/json")

	if !validContentType {
		h.logger.Warn("Unsupported Content-Type for OTLP logs endpoint", "content_type", contentType)
		response.ErrorWithStatus(c, 415, "unsupported_media_type",
			"Content-Type must be 'application/x-protobuf' or 'application/json'", "")
		return
	}

	// Enforce 10MB request size limit (OTEL Collector default, prevents DoS attacks)
	const maxRequestSize = 10 * 1024 * 1024 // 10MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestSize)

	// Read raw request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		// Check if error is due to size limit
		if err.Error() == "http: request body too large" {
			h.logger.Warn("OTLP logs request exceeds maximum size limit", "max_size", maxRequestSize, "error", err.Error())
			response.ErrorWithStatus(c, 413, "payload_too_large",
				fmt.Sprintf("Request body exceeds maximum size of %d bytes", maxRequestSize), "")
			return
		}

		h.logger.Error("Failed to read OTLP logs request body", "error", err)
		response.BadRequest(c, "invalid request", "Failed to read request body")
		return
	}

	// Decompress if Content-Encoding is gzip
	contentEncoding := c.GetHeader("Content-Encoding")
	originalSize := len(body)

	if strings.Contains(contentEncoding, "gzip") {
		h.logger.Debug("Decompressing gzip-encoded OTLP logs request")

		gzipReader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			h.logger.Error("Failed to create gzip reader", "error", err)
			response.BadRequest(c, "invalid encoding", "Failed to decompress gzip data")
			return
		}
		defer func() {
			if err := gzipReader.Close(); err != nil {
				h.logger.Warn("Failed to close gzip reader", "error", err)
			}
		}()

		body, err = io.ReadAll(gzipReader)
		if err != nil {
			h.logger.Error("Failed to decompress gzip data", "error", err)
			response.BadRequest(c, "invalid encoding", "Failed to read decompressed data")
			return
		}

		h.logger.Info("Gzip decompression successful", "original_size", originalSize, "decompressed_size", len(body), "compression_ratio", float64(originalSize)/float64(len(body)))
	}

	// Parse request based on content type (already validated above)
	var protoReq collogspb.ExportLogsServiceRequest

	if strings.Contains(contentType, "application/x-protobuf") {
		// Parse Protobuf format (more efficient)
		h.logger.Debug("Parsing OTLP logs Protobuf request")

		if err := proto.Unmarshal(body, &protoReq); err != nil {
			h.logger.Error("Failed to unmarshal OTLP logs protobuf", "error", err)
			response.ValidationError(c, "invalid OTLP protobuf", err.Error())
			return
		}
	} else {
		// Parse JSON format (default, for debugging)
		h.logger.Debug("Parsing OTLP logs JSON request")

		if err := protojson.Unmarshal(body, &protoReq); err != nil {
			h.logger.Error("Failed to parse OTLP logs JSON", "error", err)
			response.ValidationError(c, "invalid OTLP JSON", err.Error())
			return
		}
	}

	// Validate request has resource logs
	if len(protoReq.GetResourceLogs()) == 0 {
		response.ValidationError(c, "empty request", "OTLP request must contain at least one resource logs")
		return
	}

	h.logger.Debug("Received OTLP logs request", "project_id", projectID.String(), "resource_logs", len(protoReq.GetResourceLogs()))

	// Wrap ResourceLogs in LogsData for converter (OTLP spec structure)
	logsData := &logspb.LogsData{
		ResourceLogs: protoReq.GetResourceLogs(),
	}

	// Convert OTLP logs to Brokle telemetry events using converter service
	logEvents, err := h.logsConverter.ConvertLogsRequest(ctx, logsData, projectID)
	if err != nil {
		h.logger.Error("Failed to convert OTLP logs to Brokle events", "error", err)
		response.InternalServerError(c, "Failed to process OTLP logs")
		return
	}

	// Convert GenAI events (structured log records with GenAI event names)
	genaiEvents, err := h.eventsConverter.ConvertGenAIEventsRequest(ctx, logsData, projectID)
	if err != nil {
		h.logger.Error("Failed to convert GenAI events to Brokle events", "error", err)
		response.InternalServerError(c, "Failed to process GenAI events")
		return
	}

	// Combine all events (logs + GenAI events)
	brokleEvents := append(logEvents, genaiEvents...)

	h.logger.Debug("Converted OTLP logs to Brokle events", "project_id", projectID.String(), "log_events", len(logEvents), "genai_events", len(genaiEvents), "total_events", len(brokleEvents))

	// NO DEDUPLICATION for logs (logs are timeseries data - idempotent inserts)
	// Publish directly to Redis Streams for async processing by worker

	// Convert to stream event data format
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

	// Create batch for stream publishing
	batchID := uid.New()
	streamMessage := &streams.TelemetryStreamMessage{
		BatchID:   batchID,
		ProjectID: projectID,
		Events:    eventData,
		Timestamp: uid.TimeFromID(batchID), // Use batch ID timestamp (monotonic)
	}

	// Publish batch to Redis Streams (single stream per project with event_type routing)
	streamID, err := h.streamProducer.PublishBatch(ctx, streamMessage)
	if err != nil {
		h.logger.Error("Failed to publish logs batch to Redis Streams", "error", err)
		response.InternalServerError(c, "Failed to process logs batch")
		return
	}

	h.logger.Info("Successfully published OTLP logs batch to Redis Streams", "project_id", projectID.String(), "batch_id", batchID.String(), "stream_id", streamID, "event_count", len(brokleEvents))

	// OTLP spec: Return success response
	response.Success(c, gin.H{
		"batch_id":    batchID.String(),
		"event_count": len(brokleEvents),
		"status":      "accepted",
	})
}
