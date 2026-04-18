package grpc

import (
	"context"
	"fmt"
	"time"

	"encoding/hex"
	"log/slog"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"brokle/internal/core/domain/observability"
	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/infrastructure/streams"
	"brokle/pkg/uid"
)

// TODO: Extract convertProtoToInternal to shared package internal/transport/otlp/converter.go
// This function is currently duplicated from HTTP handler (lines 378-476)
// Needs to be shared between HTTP and gRPC transports

// OTLPHandler implements OTLP TraceService gRPC server
type OTLPHandler struct {
	coltracepb.UnimplementedTraceServiceServer

	streamProducer       *streams.TelemetryStreamProducer
	deduplicationService observability.TelemetryDeduplicationService
	otlpConverter        *obsServices.OTLPConverterService
	logger               *slog.Logger
}

// NewOTLPHandler creates a new gRPC OTLP handler
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

// Export implements TraceService.Export (standard OTLP gRPC method)
func (h *OTLPHandler) Export(
	ctx context.Context,
	req *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	// Extract project ID from authenticated context (set by auth interceptor)
	projectIDPtr, err := extractProjectIDFromContext(ctx)
	if err != nil {
		h.logger.Error("Project ID not found in gRPC context", "error", err)
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	projectID := projectIDPtr.String()

	// Validate request has resource spans
	if len(req.ResourceSpans) == 0 {
		return nil, status.Error(codes.InvalidArgument, "OTLP request must contain at least one resource span")
	}

	spanCount := countSpans(req)
	h.logger.Debug("Received gRPC OTLP trace request",
		"project_id", projectID,
		"resource_spans", len(req.ResourceSpans),
		"total_spans", spanCount,
	)

	// Convert official OTLP protobuf to internal format
	// This will be moved to shared package: internal/transport/otlp/converter.go
	otlpReq, err := convertProtoToInternal(req)
	if err != nil {
		h.logger.Error("Failed to convert gRPC OTLP to internal format", "error", err)
		return nil, status.Error(codes.Internal, "failed to convert OTLP request")
	}

	// Convert OTLP spans to Brokle telemetry events (SAME as HTTP path)
	brokleEvents, err := h.otlpConverter.ConvertOTLPToBrokleEvents(ctx, &otlpReq, projectID)
	if err != nil {
		h.logger.Error("Failed to convert OTLP to Brokle events", "error", err)
		return nil, status.Error(codes.Internal, "failed to process OTLP traces")
	}

	h.logger.Debug("Converted gRPC OTLP to Brokle events",
		"project_id", projectID,
		"otlp_spans", spanCount,
		"brokle_events", len(brokleEvents),
	)

	// Deduplication: Extract composite IDs (trace_id:span_id)
	dedupIDs := make([]string, 0, len(brokleEvents))
	dedupIDToFirstIndex := make(map[string]int)

	for i, event := range brokleEvents {
		// Only deduplicate spans (spans have unique span_id)
		if event.EventType == observability.TelemetryEventTypeSpan {
			if event.SpanID == "" {
				h.logger.Error("Span missing span_id, skipping deduplication",
					"event_id", event.EventID.String(),
					"trace_id", event.TraceID,
				)
				continue
			}

			// Build composite key: trace_id:span_id (prevents cross-trace collisions)
			dedupID := fmt.Sprintf("%s:%s", event.TraceID, event.SpanID)
			dedupIDs = append(dedupIDs, dedupID)

			// Track first occurrence index within this batch
			if _, exists := dedupIDToFirstIndex[dedupID]; !exists {
				dedupIDToFirstIndex[dedupID] = i
			}
		}
	}

	// Claim spans atomically (24h TTL)
	batchID := uid.New()
	var claimedIDs, duplicateIDs []string

	if len(dedupIDs) > 0 {
		claimedIDs, duplicateIDs, err = h.deduplicationService.ClaimEvents(
			ctx, *projectIDPtr, batchID, dedupIDs, 24*time.Hour,
		)
		if err != nil {
			h.logger.Error("Failed to claim gRPC OTLP spans for deduplication", "error", err)
			return nil, status.Error(codes.Internal, "failed to claim events for deduplication")
		}
	}

	// Skip if all spans were duplicates and no traces
	hasTraces := false
	for _, event := range brokleEvents {
		if event.EventType == observability.TelemetryEventTypeTrace {
			hasTraces = true
			break
		}
	}

	if len(claimedIDs) == 0 && !hasTraces {
		h.logger.Info("All gRPC spans were duplicates, skipping Redis publish",
			"batch_id", batchID.String(),
			"duplicates", len(duplicateIDs),
		)
		// Return success (OTLP spec: accept duplicates silently)
		return &coltracepb.ExportTraceServiceResponse{}, nil
	}

	// Filter to claimed events only
	claimedEventData := filterToClaimedEvents(brokleEvents, claimedIDs, dedupIDToFirstIndex)

	h.logger.Info("Publishing gRPC OTLP batch to Redis streams",
		"batch_id", batchID.String(),
		"claimed_events", len(claimedIDs),
		"duplicates", len(duplicateIDs),
	)

	// Publish to Redis Streams for async worker processing
	streamMsg := &streams.TelemetryStreamMessage{
		BatchID:          batchID,
		ProjectID:        *projectIDPtr,
		Events:           claimedEventData,
		ClaimedSpanIDs:   claimedIDs,
		DuplicateSpanIDs: duplicateIDs,
		Metadata: map[string]any{
			"source":         "otlp-grpc",
			"resource_spans": len(req.ResourceSpans),
			"total_spans":    spanCount,
		},
		Timestamp: time.Now(),
	}

	streamID, err := h.streamProducer.PublishBatch(ctx, streamMsg)
	if err != nil {
		// Rollback deduplication on failure
		if releaseErr := h.deduplicationService.ReleaseEvents(ctx, claimedIDs); releaseErr != nil {
			h.logger.Error("Failed to release claimed events after publish failure",
				"error", releaseErr,
				"claimed_ids_count", len(claimedIDs),
			)
		}

		h.logger.Error("Failed to publish gRPC OTLP batch to Redis streams", "error", err)
		return nil, status.Error(codes.Internal, "failed to publish events")
	}

	h.logger.Info("gRPC OTLP traces published successfully",
		"batch_id", batchID.String(),
		"stream_id", streamID,
		"claimed_events", len(claimedIDs),
		"duplicates", len(duplicateIDs),
		"project_id", projectID,
	)

	// Return standard OTLP success response
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

// Helper functions (will be moved to shared package)

// countSpans counts total spans across all resource spans
func countSpans(req *coltracepb.ExportTraceServiceRequest) int {
	count := 0
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			count += len(ss.Spans)
		}
	}
	return count
}

// filterToClaimedEvents filters events to only claimed (non-duplicate) spans + all traces
// Converts to streams.TelemetryEventData format for Redis publishing
func filterToClaimedEvents(
	events []*observability.TelemetryEventRequest,
	claimedIDs []string,
	dedupIDToFirstIndex map[string]int,
) []streams.TelemetryEventData {
	// Convert claimedIDs to set for O(1) lookup
	claimedSet := make(map[string]bool, len(claimedIDs))
	for _, id := range claimedIDs {
		claimedSet[id] = true
	}

	// Filter and convert events
	filtered := make([]streams.TelemetryEventData, 0, len(events))
	for i, event := range events {
		// Always include traces (no deduplication for traces)
		if event.EventType == observability.TelemetryEventTypeTrace {
			filtered = append(filtered, streams.TelemetryEventData{
				EventID:      event.EventID,
				SpanID:       event.SpanID,
				TraceID:      event.TraceID,
				EventType:    string(event.EventType),
				EventPayload: event.Payload,
			})
			continue
		}

		// For spans: only include if claimed AND first occurrence in batch
		if event.EventType == observability.TelemetryEventTypeSpan {
			dedupID := fmt.Sprintf("%s:%s", event.TraceID, event.SpanID)

			// Include if: (1) claimed from Redis AND (2) first occurrence in this batch
			if claimedSet[dedupID] && dedupIDToFirstIndex[dedupID] == i {
				filtered = append(filtered, streams.TelemetryEventData{
					EventID:      event.EventID,
					SpanID:       event.SpanID,
					TraceID:      event.TraceID,
					EventType:    string(event.EventType),
					EventPayload: event.Payload,
				})
			}
		}
	}

	return filtered
}

// RegisterOTLPTraceService registers the OTLP handler with gRPC server
func RegisterOTLPTraceService(server *grpc.Server, handler *OTLPHandler) {
	coltracepb.RegisterTraceServiceServer(server, handler)
}

// convertProtoToInternal converts official OTLP protobuf to internal format
// TODO: Move to shared package internal/transport/otlp/converter.go
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
// TODO: Move to shared package internal/transport/otlp/converter.go
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
