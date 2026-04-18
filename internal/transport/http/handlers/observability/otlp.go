package observability

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"brokle/internal/core/domain/observability"
	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/infrastructure/streams"
	"brokle/internal/transport/http/middleware"
	"brokle/pkg/response"
	"brokle/pkg/uid"
)

// OTLPHandler handles OTLP HTTP requests
type OTLPHandler struct {
	streamProducer       *streams.TelemetryStreamProducer
	deduplicationService observability.TelemetryDeduplicationService
	otlpConverter        *obsServices.OTLPConverterService
	logger               *slog.Logger
}

// NewOTLPHandler creates a new OTLP handler
func NewOTLPHandler(
	streamProducer *streams.TelemetryStreamProducer,
	deduplicationService observability.TelemetryDeduplicationService,
	otlpConverter *obsServices.OTLPConverterService,
	logger *slog.Logger,
) *OTLPHandler {
	return &OTLPHandler{
		streamProducer:       streamProducer,
		deduplicationService: deduplicationService,
		otlpConverter:        otlpConverter,
		logger:               logger,
	}
}

// HandleTraces handles POST /v1/traces
// @Summary OTLP trace ingestion endpoint (OpenTelemetry spec compliant)
// @Description Accepts OpenTelemetry Protocol (OTLP) traces in JSON or Protobuf format
// @Tags SDK - OTLP
// @Accept json
// @Accept application/x-protobuf
// @Produce json
// @Security ApiKeyAuth
// @Param request body observability.OTLPRequest true "OTLP trace export request"
// @Success 200 {object} response.APIResponse{data=map[string]any} "Traces accepted"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid OTLP request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Invalid or missing API key"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /v1/traces [post]
func (h *OTLPHandler) HandleTraces(c *gin.Context) {
	ctx := c.Request.Context()

	projectUUID := middleware.MustGetProjectID(c)
	projectID := projectUUID.String()
	organizationUUID := middleware.MustGetOrganizationID(c)

	// Validate Content-Type header (OTLP specification requires explicit Content-Type)
	contentType := c.GetHeader("Content-Type")
	validContentType := strings.Contains(contentType, "application/x-protobuf") ||
		strings.Contains(contentType, "application/json")

	if !validContentType {
		h.logger.Warn("Unsupported Content-Type for OTLP endpoint", "content_type", contentType)
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
			h.logger.Warn("OTLP request exceeds maximum size limit", "max_size", maxRequestSize, "error", err.Error())
			response.ErrorWithStatus(c, 413, "payload_too_large",
				fmt.Sprintf("Request body exceeds maximum size of %d bytes", maxRequestSize), "")
			return
		}

		h.logger.Error("Failed to read OTLP request body", "error", err)
		response.BadRequest(c, "invalid request", "Failed to read request body")
		return
	}

	// Decompress if Content-Encoding is gzip
	contentEncoding := c.GetHeader("Content-Encoding")
	originalSize := len(body)

	if strings.Contains(contentEncoding, "gzip") {
		h.logger.Debug("Decompressing gzip-encoded OTLP request")

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
	var otlpReq observability.OTLPRequest

	if strings.Contains(contentType, "application/x-protobuf") {
		// Parse Protobuf format (more efficient)
		h.logger.Debug("Parsing OTLP Protobuf request")

		var protoReq coltracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &protoReq); err != nil {
			h.logger.Error("Failed to unmarshal OTLP protobuf", "error", err)
			response.ValidationError(c, "invalid OTLP protobuf", err.Error())
			return
		}

		// Convert protobuf to internal format
		otlpReq, err = convertProtoToInternal(&protoReq)
		if err != nil {
			h.logger.Error("Failed to convert protobuf to internal format", "error", err)
			response.InternalServerError(c, "Failed to process OTLP protobuf")
			return
		}
	} else {
		// Parse JSON format (default, for debugging)
		h.logger.Debug("Parsing OTLP JSON request")

		var protoReq coltracepb.ExportTraceServiceRequest
		if err := protojson.Unmarshal(body, &protoReq); err != nil {
			h.logger.Error("Failed to parse OTLP JSON", "error", err)
			response.ValidationError(c, "invalid OTLP JSON", err.Error())
			return
		}

		// Convert protobuf to internal format (same as Protobuf path)
		otlpReq, err = convertProtoToInternal(&protoReq)
		if err != nil {
			h.logger.Error("Failed to convert JSON to internal format", "error", err)
			response.InternalServerError(c, "Failed to process OTLP JSON")
			return
		}
	}

	// Validate request has resource spans
	if len(otlpReq.ResourceSpans) == 0 {
		response.ValidationError(c, "empty request", "OTLP request must contain at least one resource span")
		return
	}

	h.logger.Debug("Received OTLP trace request", "project_id", projectID, "resource_spans", len(otlpReq.ResourceSpans))

	// Convert OTLP spans to Brokle telemetry events using converter service (with cost calculation)
	brokleEvents, err := h.otlpConverter.ConvertOTLPToBrokleEvents(c.Request.Context(), &otlpReq, projectID)
	if err != nil {
		h.logger.Error("Failed to convert OTLP to Brokle events", "error", err)
		response.InternalServerError(c, "Failed to process OTLP traces")
		return
	}

	h.logger.Debug("Converted OTLP spans to Brokle events", "project_id", projectID, "otlp_spans", countSpans(&otlpReq), "brokle_events", len(brokleEvents))

	// OTLP-native processing: deduplication + Redis Streams publishing

	// 1. Extract composite dedup IDs for spans (trace_id:span_id)
	dedupIDs := make([]string, 0, len(brokleEvents))
	dedupIDToFirstIndex := make(map[string]int) // Track first occurrence index for intra-batch deduplication

	for i, event := range brokleEvents {
		// Only deduplicate spans (spans have unique span_id)
		if event.EventType == observability.TelemetryEventTypeSpan {
			if event.SpanID == "" {
				h.logger.Error("Span missing span_id, skipping deduplication", "event_id", event.EventID.String(), "trace_id", event.TraceID, "event_type", event.EventType)
				continue
			}

			// Build composite key: trace_id:span_id (prevents cross-trace collisions)
			dedupID := fmt.Sprintf("%s:%s", event.TraceID, event.SpanID)
			dedupIDs = append(dedupIDs, dedupID)

			// Track first occurrence index within this batch (for intra-batch deduplication)
			if _, exists := dedupIDToFirstIndex[dedupID]; !exists {
				dedupIDToFirstIndex[dedupID] = i
			}
		}
	}

	// 2. Claim spans atomically (24h TTL, prevents duplicates)
	batchID := uid.New()
	var claimedIDs, duplicateIDs []string

	if len(dedupIDs) > 0 {
		claimedIDs, duplicateIDs, err = h.deduplicationService.ClaimEvents(
			ctx, projectUUID, batchID, dedupIDs, 24*time.Hour,
		)
		if err != nil {
			h.logger.Error("Failed to claim OTLP spans for deduplication", "error", err)
			response.InternalServerError(c, "Failed to claim events for deduplication")
			return
		}
	}

	// 3. Skip if all spans were duplicates and no traces
	hasTraces := false
	for _, event := range brokleEvents {
		if event.EventType == observability.TelemetryEventTypeTrace {
			hasTraces = true
			break
		}
	}

	if len(claimedIDs) == 0 && !hasTraces {
		h.logger.Info("All OTLP spans were duplicates, skipping", "project_id", projectID, "duplicates", len(duplicateIDs))

		response.Success(c, map[string]any{
			"status":          "all_duplicates",
			"duplicate_spans": len(duplicateIDs),
		})
		return
	}

	// 4. Filter to claimed spans + all traces
	claimedSet := make(map[string]bool, len(claimedIDs))
	for _, id := range claimedIDs {
		claimedSet[id] = true
	}

	claimedEventData := make([]streams.TelemetryEventData, 0, len(brokleEvents))
	for i, event := range brokleEvents {
		// Always include traces (no dedup)
		if event.EventType == observability.TelemetryEventTypeTrace {
			claimedEventData = append(claimedEventData, streams.TelemetryEventData{
				EventID:      event.EventID,
				SpanID:       event.SpanID,
				TraceID:      event.TraceID,
				EventType:    string(event.EventType),
				EventPayload: event.Payload,
			})
			continue
		}

		// Spans: include ONLY if (1) first occurrence in batch AND (2) claimed
		if event.EventType == observability.TelemetryEventTypeSpan {
			dedupID := fmt.Sprintf("%s:%s", event.TraceID, event.SpanID)
			firstIndex := dedupIDToFirstIndex[dedupID]
			isFirstOccurrence := (i == firstIndex)

			// Two-level deduplication:
			// 1. Intra-batch: only process first occurrence within this batch
			// 2. Inter-batch: only process if claimed by Redis (not a global duplicate)
			if isFirstOccurrence && claimedSet[dedupID] {
				claimedEventData = append(claimedEventData, streams.TelemetryEventData{
					EventID:      event.EventID,
					SpanID:       event.SpanID,
					TraceID:      event.TraceID,
					EventType:    string(event.EventType),
					EventPayload: event.Payload,
				})
			}
		}
	}

	// 5. Publish to Redis Streams for async processing
	streamMsg := &streams.TelemetryStreamMessage{
		BatchID:          batchID,
		ProjectID:        projectUUID,
		OrganizationID:   organizationUUID,
		Events:           claimedEventData,
		ClaimedSpanIDs:   claimedIDs,
		DuplicateSpanIDs: duplicateIDs,
		Metadata: map[string]any{
			"source":         "otlp",
			"content_type":   contentType,
			"resource_spans": len(otlpReq.ResourceSpans),
			"total_spans":    countSpans(&otlpReq),
		},
		Timestamp: time.Now(),
	}

	streamID, err := h.streamProducer.PublishBatch(ctx, streamMsg)
	if err != nil {
		// CRITICAL: Rollback claimed events on publish failure
		if rollbackErr := h.deduplicationService.ReleaseEvents(ctx, claimedIDs); rollbackErr != nil {
			h.logger.Error("CRITICAL: Failed to rollback OTLP deduplication claims after publish failure", "rollback_error", rollbackErr.Error(), "original_error", err.Error(), "batch_id", batchID.String())
		}
		response.InternalServerError(c, "Failed to publish events to stream")
		return
	}

	h.logger.Info("OTLP traces published to stream successfully", "batch_id", batchID.String(), "stream_id", streamID, "claimed_events", len(claimedIDs), "duplicates", len(duplicateIDs), "project_id", projectID)

	// 6. Return OTLP-compatible success response (using standard APIResponse envelope)
	response.Success(c, map[string]any{
		"status":          "accepted",
		"batch_id":        batchID.String(),
		"stream_id":       streamID,
		"processed_spans": len(claimedIDs),
		"duplicate_spans": len(duplicateIDs),
	})
}

// countSpans counts total spans in OTLP request
func countSpans(req *observability.OTLPRequest) int {
	count := 0
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			count += len(ss.Spans)
		}
	}
	return count
}

// convertProtoToInternal converts official OTLP protobuf to internal format
func convertProtoToInternal(protoReq *coltracepb.ExportTraceServiceRequest) (observability.OTLPRequest, error) {
	var internalReq observability.OTLPRequest

	for _, protoRS := range protoReq.ResourceSpans {
		internalRS := observability.ResourceSpan{}

		// Convert Resource
		if protoRS.Resource != nil {
			internalResource := &observability.Resource{}
			for _, attr := range protoRS.Resource.Attributes {
				internalResource.Attributes = append(internalResource.Attributes, observability.KeyValue{
					Key:   attr.Key,
					Value: convertProtoAnyValue(attr.Value),
				})
			}
			internalRS.Resource = internalResource
		}

		// Convert ScopeSpans
		for _, protoSS := range protoRS.ScopeSpans {
			internalSS := observability.ScopeSpan{}

			// Convert Scope
			if protoSS.Scope != nil {
				internalScope := &observability.Scope{
					Name:    protoSS.Scope.Name,
					Version: protoSS.Scope.Version,
				}
				for _, attr := range protoSS.Scope.Attributes {
					internalScope.Attributes = append(internalScope.Attributes, observability.KeyValue{
						Key:   attr.Key,
						Value: convertProtoAnyValue(attr.Value),
					})
				}
				internalSS.Scope = internalScope
			}

			// Convert Spans
			for _, protoSpan := range protoSS.Spans {
				// Convert byte arrays to hex strings for internal format
				traceIDHex := hex.EncodeToString(protoSpan.TraceId)
				spanIDHex := hex.EncodeToString(protoSpan.SpanId)
				var parentSpanIDHex any
				if len(protoSpan.ParentSpanId) > 0 {
					parentSpanIDHex = hex.EncodeToString(protoSpan.ParentSpanId)
				}

				internalSpan := observability.OTLPSpan{
					TraceID:           traceIDHex,
					SpanID:            spanIDHex,
					ParentSpanID:      parentSpanIDHex,
					Name:              protoSpan.Name,
					Kind:              int(protoSpan.Kind),
					StartTimeUnixNano: int64(protoSpan.StartTimeUnixNano),
					EndTimeUnixNano:   int64(protoSpan.EndTimeUnixNano),
				}

				// Convert Attributes
				for _, attr := range protoSpan.Attributes {
					internalSpan.Attributes = append(internalSpan.Attributes, observability.KeyValue{
						Key:   attr.Key,
						Value: convertProtoAnyValue(attr.Value),
					})
				}

				// Convert Status
				if protoSpan.Status != nil {
					internalSpan.Status = &observability.Status{
						Code:    int(protoSpan.Status.Code),
						Message: protoSpan.Status.Message,
					}
				}

				// Convert Events
				for _, protoEvent := range protoSpan.Events {
					internalEvent := observability.Event{
						TimeUnixNano: int64(protoEvent.TimeUnixNano),
						Name:         protoEvent.Name,
					}
					for _, attr := range protoEvent.Attributes {
						internalEvent.Attributes = append(internalEvent.Attributes, observability.KeyValue{
							Key:   attr.Key,
							Value: convertProtoAnyValue(attr.Value),
						})
					}
					internalSpan.Events = append(internalSpan.Events, internalEvent)
				}

				internalSS.Spans = append(internalSS.Spans, internalSpan)
			}

			internalRS.ScopeSpans = append(internalRS.ScopeSpans, internalSS)
		}

		internalReq.ResourceSpans = append(internalReq.ResourceSpans, internalRS)
	}

	return internalReq, nil
}

// convertProtoAnyValue converts protobuf AnyValue to any
func convertProtoAnyValue(value *commonpb.AnyValue) any {
	if value == nil {
		return nil
	}

	switch v := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return v.StringValue
	case *commonpb.AnyValue_BoolValue:
		return v.BoolValue
	case *commonpb.AnyValue_IntValue:
		return v.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return v.DoubleValue
	case *commonpb.AnyValue_ArrayValue:
		if v.ArrayValue == nil {
			return nil
		}
		arr := make([]any, len(v.ArrayValue.Values))
		for i, item := range v.ArrayValue.Values {
			arr[i] = convertProtoAnyValue(item)
		}
		return arr
	case *commonpb.AnyValue_KvlistValue:
		if v.KvlistValue == nil {
			return nil
		}
		m := make(map[string]any)
		for _, kv := range v.KvlistValue.Values {
			m[kv.Key] = convertProtoAnyValue(kv.Value)
		}
		return m
	case *commonpb.AnyValue_BytesValue:
		return v.BytesValue
	default:
		return nil
	}
}
