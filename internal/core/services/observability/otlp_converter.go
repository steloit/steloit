package observability

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/analytics"
	"brokle/internal/core/domain/observability"
	"brokle/pkg/uid"
)

// MaxAttributeValueSize defines the maximum size for input/output attribute values
// Matches common OTEL collector limits (1MB) to prevent oversized spans
const MaxAttributeValueSize = 1024 * 1024 // 1MB

// Framework-specific attribute keys (Priority 1 frameworks)
const (
	// Vercel AI SDK
	AttrVercelPromptMessages    = "ai.prompt.messages"
	AttrVercelPrompt            = "ai.prompt"
	AttrVercelToolCallArgs      = "ai.toolCall.args"
	AttrVercelResponseText      = "ai.response.text"
	AttrVercelResponseToolCalls = "ai.response.toolCalls"
	AttrVercelResponseObject    = "ai.response.object"
	AttrVercelResultText        = "ai.result.text"   // Legacy <4.0
	AttrVercelResultObject      = "ai.result.object" // Legacy <4.0
	AttrVercelResultToolCalls   = "ai.result.toolCalls"

	// OTEL GenAI
	AttrGenAIInputMessages      = "gen_ai.input.messages"
	AttrGenAIOutputMessages     = "gen_ai.output.messages"
	AttrGenAISystemInstructions = "gen_ai.system_instructions"

	// OpenInference
	AttrInputValue     = "input.value"
	AttrInputMimeType  = "input.mime_type"
	AttrOutputValue    = "output.value"
	AttrOutputMimeType = "output.mime_type"
)

// frameworkIOKeys lists attribute keys that should be filtered from metadata
// to prevent duplicate I/O data in metadata attributes
var frameworkIOKeys = map[string]bool{
	// Vercel AI SDK
	AttrVercelPromptMessages:    true,
	AttrVercelPrompt:            true,
	AttrVercelToolCallArgs:      true,
	AttrVercelResponseText:      true,
	AttrVercelResultText:        true,
	AttrVercelResponseObject:    true,
	AttrVercelResultObject:      true,
	AttrVercelResponseToolCalls: true,
	AttrVercelResultToolCalls:   true,
	// OTEL GenAI
	AttrGenAIInputMessages:      true,
	AttrGenAIOutputMessages:     true,
	AttrGenAISystemInstructions: true,
	// OpenInference
	AttrInputValue:     true,
	AttrOutputValue:    true,
	AttrInputMimeType:  true,
	AttrOutputMimeType: true,
}

// ExtractIOParams contains parameters for input/output extraction
type ExtractIOParams struct {
	Attributes               map[string]any
	Events                   []map[string]any
	InstrumentationScopeName string
	MaxSize                  int
}

// InputOutputTruncated tracks whether input/output were truncated
type InputOutputTruncated struct {
	Input  bool
	Output bool
}

// OTLPConverterService handles conversion of OTLP traces to Brokle telemetry events
type OTLPConverterService struct {
	logger                 *slog.Logger
	providerPricingService analytics.ProviderPricingService
	config                 *config.ObservabilityConfig
}

// NewOTLPConverterService creates a new OTLP converter service
func NewOTLPConverterService(
	logger *slog.Logger,
	providerPricingService analytics.ProviderPricingService,
	observabilityConfig *config.ObservabilityConfig,
) *OTLPConverterService {
	return &OTLPConverterService{
		logger:                 logger,
		providerPricingService: providerPricingService,
		config:                 observabilityConfig,
	}
}

// brokleEvent represents an internal converted event (before domain conversion)
type brokleEvent struct {
	Payload   map[string]any `json:"payload"`
	Timestamp *int64                 `json:"timestamp,omitempty"`
	EventID   string                 `json:"event_id"`
	SpanID    string                 `json:"span_id"`
	TraceID   string                 `json:"trace_id"`
	EventType string                 `json:"event_type"`
}

// isRootSpanCheck determines if a span is a root span by checking if parent ID is nil, empty, or zero.
// Handles: nil, empty string, "0000000000000000", zero bytes array, and {data: Buffer} format.
func isRootSpanCheck(parentSpanID any) bool {
	if parentSpanID == nil {
		return true
	}

	if str, ok := parentSpanID.(string); ok {
		if str == "" || str == "0000000000000000" {
			return true
		}
	}

	if mapVal, ok := parentSpanID.(map[string]any); ok {
		if data, ok := mapVal["data"].([]any); ok {
			allZero := true
			for _, b := range data {
				if intVal, ok := b.(float64); ok && intVal != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				return true
			}
		}
	}

	if bytes, ok := parentSpanID.([]byte); ok {
		allZero := true
		for _, b := range bytes {
			if b != 0 {
				allZero = false
				break
			}
		}
		return allZero
	}

	return false
}

// ConvertOTLPToBrokleEvents converts OTLP resourceSpans to Brokle span events.
// Traces are derived asynchronously from root spans (parent_span_id IS NULL).
func (s *OTLPConverterService) ConvertOTLPToBrokleEvents(ctx context.Context, otlpReq *observability.OTLPRequest, projectID string) ([]*observability.TelemetryEventRequest, error) {
	var internalEvents []*brokleEvent

	for _, resourceSpan := range otlpReq.ResourceSpans {
		resourceAttrs := extractAttributes(resourceSpan.Resource)

		for _, scopeSpan := range resourceSpan.ScopeSpans {
			scopeAttrs := extractAttributes(scopeSpan.Scope)

			for _, span := range scopeSpan.Spans {
				obsEvent, err := s.createSpanEvent(ctx, span, resourceAttrs, scopeAttrs, resourceSpan.Resource, scopeSpan.Scope, projectID)
				if err != nil {
					return nil, fmt.Errorf("failed to create span event: %w", err)
				}
				internalEvents = append(internalEvents, obsEvent)
			}
		}
	}

	return convertToDomainEvents(internalEvents), nil
}

func truncateWithIndicator(value string, maxSize int) (string, bool) {
	if len(value) <= maxSize {
		return value, false
	}
	return value[:maxSize] + "...[truncated]", true
}

// validateMimeType validates MIME type against actual content, auto-detecting if missing.
func validateMimeType(value string, declaredMimeType string) string {
	if declaredMimeType == "" {
		if json.Valid([]byte(value)) {
			return "application/json"
		}
		return "text/plain"
	}

	if declaredMimeType == "application/json" && !json.Valid([]byte(value)) {
		return "text/plain"
	}

	return declaredMimeType
}

// extractGenericInput extracts input with extended priority chain.
// Priority: gen_ai.input.messages > input.value (string, []any, or map[string]any)
// This supports both LLM ChatML format and generic function arguments.
func extractGenericInput(allAttrs map[string]any) (value string, mimeType string) {
	// Priority 1: gen_ai.input.messages (LLM ChatML format)
	if messages, ok := allAttrs["gen_ai.input.messages"].([]any); ok {
		if messagesJSON, err := json.Marshal(messages); err == nil {
			return string(messagesJSON), "application/json"
		}
	} else if messages, ok := allAttrs["gen_ai.input.messages"].(string); ok && messages != "" {
		return messages, "application/json"
	}

	// Priority 2: input.value (generic input - supports string, array, or object)
	if inputVal, ok := allAttrs["input.value"]; ok && inputVal != nil {
		declaredMime, _ := allAttrs["input.mime_type"].(string)

		switch v := inputVal.(type) {
		case string:
			if v != "" {
				return v, validateMimeType(v, declaredMime)
			}
		case []any:
			// Array input (e.g., function positional arguments)
			if inputJSON, err := json.Marshal(v); err == nil {
				return string(inputJSON), "application/json"
			}
		case map[string]any:
			// Object input (e.g., function kwargs or structured data)
			if inputJSON, err := json.Marshal(v); err == nil {
				return string(inputJSON), "application/json"
			}
		}
	}

	return "", ""
}

// extractGenericOutput extracts output with extended priority chain.
// Priority: gen_ai.output.messages > output.value (string, []any, or map[string]any)
// This supports both LLM ChatML format and generic function return values.
func extractGenericOutput(allAttrs map[string]any) (value string, mimeType string) {
	// Priority 1: gen_ai.output.messages (LLM ChatML format)
	if messages, ok := allAttrs["gen_ai.output.messages"].([]any); ok {
		if messagesJSON, err := json.Marshal(messages); err == nil {
			return string(messagesJSON), "application/json"
		}
	} else if messages, ok := allAttrs["gen_ai.output.messages"].(string); ok && messages != "" {
		return messages, "application/json"
	}

	// Priority 2: output.value (generic output - supports string, array, or object)
	if outputVal, ok := allAttrs["output.value"]; ok && outputVal != nil {
		declaredMime, _ := allAttrs["output.mime_type"].(string)

		switch v := outputVal.(type) {
		case string:
			if v != "" {
				return v, validateMimeType(v, declaredMime)
			}
		case []any:
			// Array output (e.g., multiple return values)
			if outputJSON, err := json.Marshal(v); err == nil {
				return string(outputJSON), "application/json"
			}
		case map[string]any:
			// Object output (e.g., structured return value)
			if outputJSON, err := json.Marshal(v); err == nil {
				return string(outputJSON), "application/json"
			}
		}
	}

	return "", ""
}

// extractToolMetadata extracts tool/function-specific attributes for non-LLM operations.
// Supports gen_ai.tool.* attributes for tool calls and function instrumentation.
func extractToolMetadata(allAttrs map[string]any, payload map[string]any) {
	// Tool name from gen_ai.tool.name
	if toolName, ok := allAttrs["gen_ai.tool.name"].(string); ok && toolName != "" {
		payload["tool_name"] = toolName
	}

	// Tool parameters (function input arguments as JSON)
	if params, ok := allAttrs["gen_ai.tool.parameters"].(string); ok && params != "" {
		payload["tool_parameters"] = params
	} else if params, ok := allAttrs["gen_ai.tool.parameters"].(map[string]any); ok {
		if paramsJSON, err := json.Marshal(params); err == nil {
			payload["tool_parameters"] = string(paramsJSON)
		}
	}

	// Tool result (function return value)
	if result, ok := allAttrs["gen_ai.tool.result"].(string); ok && result != "" {
		payload["tool_result"] = result
	} else if result, ok := allAttrs["gen_ai.tool.result"].(map[string]any); ok {
		if resultJSON, err := json.Marshal(result); err == nil {
			payload["tool_result"] = string(resultJSON)
		}
	}

	// Tool call ID (for tracking tool invocations in agentic workflows)
	if toolCallID, ok := allAttrs["gen_ai.tool.call.id"].(string); ok && toolCallID != "" {
		payload["tool_call_id"] = toolCallID
	}
}

// extractLLMMetadata extracts LLM-specific metadata from ChatML formatted input.
func extractLLMMetadata(inputValue string) map[string]any {
	metadata := make(map[string]any)

	var messages []map[string]any
	if err := json.Unmarshal([]byte(inputValue), &messages); err != nil {
		return metadata
	}

	if len(messages) == 0 {
		return metadata
	}

	if _, hasRole := messages[0]["role"]; !hasRole {
		return metadata
	}

	metadata["brokle.llm.message_count"] = len(messages)

	var userCount, assistantCount, systemCount, toolCount int
	var firstRole, lastRole string
	hasToolCalls := false

	for i, msg := range messages {
		role, _ := msg["role"].(string)

		if i == 0 {
			firstRole = role
		}
		if i == len(messages)-1 {
			lastRole = role
		}

		switch role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
		case "system":
			systemCount++
		case "tool":
			toolCount++
		}

		if toolCalls, ok := msg["tool_calls"]; ok && toolCalls != nil {
			hasToolCalls = true
		}
	}

	if userCount > 0 {
		metadata["brokle.llm.user_message_count"] = userCount
	}
	if assistantCount > 0 {
		metadata["brokle.llm.assistant_message_count"] = assistantCount
	}
	if systemCount > 0 {
		metadata["brokle.llm.system_message_count"] = systemCount
	}
	if toolCount > 0 {
		metadata["brokle.llm.tool_message_count"] = toolCount
	}

	if firstRole != "" {
		metadata["brokle.llm.first_role"] = firstRole
	}
	if lastRole != "" {
		metadata["brokle.llm.last_role"] = lastRole
	}

	metadata["brokle.llm.has_tool_calls"] = hasToolCalls

	return metadata
}

func (s *OTLPConverterService) createSpanEvent(ctx context.Context, span observability.OTLPSpan, resourceAttrs, scopeAttrs map[string]any, resource *observability.Resource, scope *observability.Scope, projectID string) (*brokleEvent, error) {
	traceID, err := convertTraceID(span.TraceID)
	if err != nil {
		return nil, fmt.Errorf("invalid trace_id: %w", err)
	}

	spanID, err := convertSpanID(span.SpanID)
	if err != nil {
		return nil, fmt.Errorf("invalid span_id: %w", err)
	}

	var parentSpanID *string
	if !isRootSpanCheck(span.ParentSpanID) {
		parentID, err := convertSpanID(span.ParentSpanID)
		if err == nil {
			parentSpanID = &parentID
		}
	}

	spanAttrs := extractAttributesFromKeyValues(span.Attributes)
	allAttrs := mergeAttributes(resourceAttrs, scopeAttrs, spanAttrs)
	startTime := convertUnixNano(span.StartTimeUnixNano)
	endTime := convertUnixNano(span.EndTimeUnixNano)
	spanKind := convertSpanKind(span.Kind)
	statusCode := convertStatusCode(span.Status)

	payload := map[string]any{
		"span_id":        spanID,
		"trace_id":       traceID,
		"parent_span_id": parentSpanID,
		"span_name":      span.Name,
		"span_kind":      spanKind,
		"status_code":    statusCode,
	}

	if startTime != nil {
		payload["start_time"] = startTime.Format(time.RFC3339Nano)
	}

	if endTime != nil {
		payload["end_time"] = endTime.Format(time.RFC3339Nano)
	}
	if span.Status != nil && span.Status.Message != "" {
		payload["status_message"] = span.Status.Message
	}

	// Get instrumentation scope name for framework detection
	var scopeName string
	if scope != nil && scope.Name != "" {
		scopeName = scope.Name
	}

	// Store instrumentation scope name in payload for debugging and filtering
	if scopeName != "" {
		payload["instrumentation_scope_name"] = scopeName
	}

	// Convert span events to the format expected by extractInputOutput
	var spanEvents []map[string]any
	if len(span.Events) > 0 {
		spanEvents = make([]map[string]any, len(span.Events))
		for i, event := range span.Events {
			eventMap := make(map[string]any)
			if eventTime := convertUnixNano(event.TimeUnixNano); eventTime != nil {
				eventMap["timestamp"] = eventTime.Format(time.RFC3339Nano)
			}
			eventMap["name"] = event.Name
			eventMap["attributes"] = convertToStringMap(extractAttributesFromKeyValues(event.Attributes))
			eventMap["dropped_attributes_count"] = event.DroppedAttributesCount
			spanEvents[i] = eventMap
		}
	}

	// Extract input/output using unified framework-aware extractor
	inputValue, outputValue, inputMimeType, outputMimeType, ioTruncated := extractInputOutput(ExtractIOParams{
		Attributes:               allAttrs,
		Events:                   spanEvents,
		InstrumentationScopeName: scopeName,
		MaxSize:                  MaxAttributeValueSize,
	})

	if inputValue != "" {
		payload["input"] = inputValue
		if inputMimeType != "" {
			payload["input_mime_type"] = inputMimeType
		}
		if ioTruncated.Input {
			payload["input_truncated"] = true
		}

		// Extract LLM metadata from ChatML formatted input
		if inputMimeType == "application/json" {
			if llmMetadata := extractLLMMetadata(inputValue); len(llmMetadata) > 0 {
				for key, value := range llmMetadata {
					payload[key] = value
				}
			}
		}
	}

	if outputValue != "" {
		payload["output"] = outputValue
		if outputMimeType != "" {
			payload["output_mime_type"] = outputMimeType
		}
		if ioTruncated.Output {
			payload["output_truncated"] = true
		}
	}

	extractToolMetadata(allAttrs, payload)

	extractGenAIFields(allAttrs, payload)
	s.calculateProviderCostsAtIngestion(ctx, allAttrs, payload, projectID)

	payload["span_attributes"] = spanAttrs

	if scope != nil {
		if scope.Name != "" {
			payload["scope_name"] = scope.Name
		}
		if scope.Version != "" {
			payload["scope_version"] = scope.Version
		}
		if len(scopeAttrs) > 0 {
			payload["scope_attributes"] = scopeAttrs
		}
	}

	if traceState, ok := allAttrs["trace_state"].(string); ok && traceState != "" {
		payload["trace_state"] = traceState
	}

	// Reuse spanEvents that were already converted for I/O extraction
	if len(spanEvents) > 0 {
		payload["events"] = spanEvents
	}

	if len(span.Links) > 0 {
		links := make([]map[string]any, len(span.Links))
		for i, link := range span.Links {
			linkMap := make(map[string]any)
			if traceID, err := convertTraceID(link.TraceID); err == nil {
				linkMap["trace_id"] = traceID
			}
			if spanID, err := convertSpanID(link.SpanID); err == nil {
				linkMap["span_id"] = spanID
			}
			if link.TraceState != nil {
				if ts, ok := link.TraceState.(string); ok {
					linkMap["trace_state"] = ts
				}
			}
			linkMap["attributes"] = convertToStringMap(extractAttributesFromKeyValues(link.Attributes))
			linkMap["dropped_attributes_count"] = link.DroppedAttributesCount
			links[i] = linkMap
		}
		payload["links"] = links
	}

	payload["resource_attributes"] = convertToStringMap(resourceAttrs)
	payload["span_attributes"] = convertToStringMap(filterIOKeysFromMetadata(spanAttrs))
	payload["scope_attributes"] = convertToStringMap(scopeAttrs)

	if scope != nil {
		payload["scope_name"] = scope.Name
		payload["scope_version"] = scope.Version
	}

	if resource != nil && resource.SchemaUrl != "" {
		payload["resource_schema_url"] = resource.SchemaUrl
	}
	if scope != nil && scope.SchemaUrl != "" {
		payload["scope_schema_url"] = scope.SchemaUrl
	}

	if s.config.PreserveRawOTLP {
		rawOTLPJSON, err := json.Marshal(span)
		if err == nil {
			payload["otlp_span_raw"] = string(rawOTLPJSON)
		} else {
			s.logger.Warn("Failed to marshal raw OTLP span", "error", err)
		}

		if len(resourceAttrs) > 0 {
			resourceJSON, err := json.Marshal(resourceAttrs)
			if err == nil {
				payload["otlp_resource_attributes"] = string(resourceJSON)
			}
		}

		if len(scopeAttrs) > 0 {
			scopeJSON, err := json.Marshal(scopeAttrs)
			if err == nil {
				payload["otlp_scope_attributes"] = string(scopeJSON)
			}
		}
	}

	event := &brokleEvent{
		EventID:   uid.New().String(),
		SpanID:    spanID,
		TraceID:   traceID,
		EventType: "span",
		Payload:   payload,
		Timestamp: func() *int64 {
			if startTime != nil {
				ts := startTime.Unix()
				return &ts
			}
			return nil
		}(),
	}

	return event, nil
}

func convertToStringMap(attrs map[string]any) map[string]string {
	if attrs == nil {
		return make(map[string]string)
	}

	result := make(map[string]string, len(attrs))
	for k, v := range attrs {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

func convertTraceID(traceID any) (string, error) {
	switch v := traceID.(type) {
	case string:
		if len(v) == 32 {
			return v, nil
		}
		return "", fmt.Errorf("invalid trace_id length: %d (expected 32)", len(v))
	case map[string]any:
		if data, ok := v["data"].([]any); ok {
			return bytesToHex(data), nil
		}
	case []byte:
		return hex.EncodeToString(v), nil
	}
	return "", fmt.Errorf("unsupported trace_id type: %T", traceID)
}

func convertSpanID(spanID any) (string, error) {
	switch v := spanID.(type) {
	case string:
		if len(v) == 16 {
			return v, nil
		}
		return "", fmt.Errorf("invalid span_id length: %d (expected 16)", len(v))
	case map[string]any:
		if data, ok := v["data"].([]any); ok {
			return bytesToHex(data), nil
		}
	case []byte:
		return hex.EncodeToString(v), nil
	}
	return "", fmt.Errorf("unsupported span_id type: %T", spanID)
}

func convertUnixNano(ts any) *time.Time {
	if ts == nil {
		return nil
	}

	var nanos int64
	switch v := ts.(type) {
	case int64:
		nanos = v
	case float64:
		nanos = int64(v)
	case map[string]any:
		low, lowOk := v["low"].(float64)
		high, highOk := v["high"].(float64)
		if !lowOk || !highOk {
			return nil
		}
		nanos = int64(high)*4294967296 + int64(low)
	default:
		return nil
	}

	if nanos == 0 {
		return nil
	}

	t := time.Unix(0, nanos)
	return &t
}

func convertSpanKind(kind int) uint8 {
	switch kind {
	case 0:
		return observability.SpanKindUnspecified
	case 1:
		return observability.SpanKindInternal
	case 2:
		return observability.SpanKindServer
	case 3:
		return observability.SpanKindClient
	case 4:
		return observability.SpanKindProducer
	case 5:
		return observability.SpanKindConsumer
	default:
		return observability.SpanKindInternal
	}
}

func convertStatusCode(status *observability.Status) uint8 {
	if status == nil {
		return observability.StatusCodeUnset
	}
	switch status.Code {
	case 0:
		return observability.StatusCodeUnset
	case 1:
		return observability.StatusCodeOK
	case 2:
		return observability.StatusCodeError
	default:
		return observability.StatusCodeUnset
	}
}

func extractAttributes(obj any) map[string]any {
	attrs := make(map[string]any)

	switch v := obj.(type) {
	case *observability.Resource:
		if v != nil {
			return extractAttributesFromKeyValues(v.Attributes)
		}
	case *observability.Scope:
		if v != nil {
			return extractAttributesFromKeyValues(v.Attributes)
		}
	}

	return attrs
}

func extractAttributesFromKeyValues(kvs []observability.KeyValue) map[string]any {
	attrs := make(map[string]any)

	for _, kv := range kvs {
		value := extractValue(kv.Value)
		if value != nil {
			attrs[kv.Key] = value
		}
	}

	return attrs
}

func extractValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		if sv, ok := val["stringValue"].(string); ok {
			return sv
		}
		if iv, ok := val["intValue"].(float64); ok {
			return int64(iv)
		}
		if bv, ok := val["boolValue"].(bool); ok {
			return bv
		}
		if dv, ok := val["doubleValue"].(float64); ok {
			return dv
		}
		if av, ok := val["arrayValue"].(map[string]any); ok {
			if values, ok := av["values"].([]any); ok {
				result := make([]any, len(values))
				for i, item := range values {
					result[i] = extractValue(item)
				}
				return result
			}
		}
		return val
	case string, int, int64, float64, bool:
		return val
	}

	return v
}

func mergeAttributes(resource, scope, span map[string]any) map[string]any {
	merged := make(map[string]any)

	for k, v := range resource {
		merged[k] = v
	}

	for k, v := range scope {
		merged[k] = v
	}

	for k, v := range span {
		merged[k] = v
	}

	return merged
}

func marshalAttributes(attrs map[string]any) string {
	if len(attrs) == 0 {
		return "{}"
	}

	jsonBytes, err := json.Marshal(attrs)
	if err != nil {
		return "{}"
	}

	return string(jsonBytes)
}

// extractGenAIFields extracts Gen AI semantic conventions from attributes.
// Note: input/output extraction is handled in createSpanEvent() with proper
// truncation and MIME type handling. This function only extracts non-I/O fields.
func extractGenAIFields(attrs map[string]any, payload map[string]any) {
	if provider, ok := attrs["gen_ai.provider.name"].(string); ok {
		payload["provider"] = provider
	}

	if responseModel, ok := attrs["gen_ai.response.model"].(string); ok {
		payload["model_name"] = responseModel
	} else if requestModel, ok := attrs["gen_ai.request.model"].(string); ok {
		payload["model_name"] = requestModel
	}

	modelParams := make(map[string]any)
	if temp, ok := attrs["gen_ai.request.temperature"].(float64); ok {
		modelParams["temperature"] = temp
	}
	if maxTokens, ok := attrs["gen_ai.request.max_tokens"].(float64); ok {
		modelParams["max_tokens"] = int(maxTokens)
	} else if maxTokens, ok := attrs["gen_ai.request.max_tokens"].(int64); ok {
		modelParams["max_tokens"] = int(maxTokens)
	}
	if topP, ok := attrs["gen_ai.request.top_p"].(float64); ok {
		modelParams["top_p"] = topP
	}
	if topK, ok := attrs["gen_ai.request.top_k"].(float64); ok {
		modelParams["top_k"] = int(topK)
	} else if topK, ok := attrs["gen_ai.request.top_k"].(int64); ok {
		modelParams["top_k"] = int(topK)
	}
	if freqPenalty, ok := attrs["gen_ai.request.frequency_penalty"].(float64); ok {
		modelParams["frequency_penalty"] = freqPenalty
	}
	if presPenalty, ok := attrs["gen_ai.request.presence_penalty"].(float64); ok {
		modelParams["presence_penalty"] = presPenalty
	}

	if len(modelParams) > 0 {
		if paramsJSON, err := json.Marshal(modelParams); err == nil {
			payload["model_parameters"] = string(paramsJSON)
		}
	}

	usageDetails := make(map[string]uint64)
	if inputTokens, ok := attrs["gen_ai.usage.input_tokens"].(float64); ok {
		usageDetails["input"] = uint64(inputTokens)
	} else if inputTokens, ok := attrs["gen_ai.usage.input_tokens"].(int64); ok {
		usageDetails["input"] = uint64(inputTokens)
	}

	if outputTokens, ok := attrs["gen_ai.usage.output_tokens"].(float64); ok {
		usageDetails["output"] = uint64(outputTokens)
	} else if outputTokens, ok := attrs["gen_ai.usage.output_tokens"].(int64); ok {
		usageDetails["output"] = uint64(outputTokens)
	}

	if totalTokens, ok := attrs["brokle.usage.total_tokens"].(float64); ok {
		usageDetails["total"] = uint64(totalTokens)
	} else if totalTokens, ok := attrs["brokle.usage.total_tokens"].(int64); ok {
		usageDetails["total"] = uint64(totalTokens)
	}

	if len(usageDetails) > 0 {
		payload["usage_details"] = usageDetails
	}
}

func bytesToHex(data []any) string {
	bytes := make([]byte, len(data))
	for i, v := range data {
		if f, ok := v.(float64); ok {
			bytes[i] = byte(f)
		}
	}
	return hex.EncodeToString(bytes)
}

func convertToDomainEvents(events []*brokleEvent) []*observability.TelemetryEventRequest {
	result := make([]*observability.TelemetryEventRequest, 0, len(events))
	for _, e := range events {
		eventID, err := uuid.Parse(e.EventID)
		if err != nil {
			continue
		}
		result = append(result, &observability.TelemetryEventRequest{
			EventID:   eventID,
			SpanID:    e.SpanID,
			TraceID:   e.TraceID,
			EventType: observability.TelemetryEventType(e.EventType),
			Payload:   e.Payload,
			Timestamp: func() *time.Time {
				if e.Timestamp != nil {
					t := time.Unix(*e.Timestamp, 0)
					return &t
				}
				return nil
			}(),
		})
	}
	return result
}

// calculateProviderCostsAtIngestion calculates provider costs for cost visibility.
func (s *OTLPConverterService) calculateProviderCostsAtIngestion(
	ctx context.Context,
	attrs map[string]any,
	payload map[string]any,
	projectID string,
) {
	modelName := extractStringFromInterface(attrs["gen_ai.request.model"])
	if modelName == "" {
		return
	}

	usage := make(map[string]uint64)

	if val := extractUint64FromInterface(attrs["gen_ai.usage.input_tokens"]); val > 0 {
		usage["input"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.output_tokens"]); val > 0 {
		usage["output"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.input_tokens.cache_read"]); val > 0 {
		usage["cache_read_input_tokens"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.input_tokens.cache_creation"]); val > 0 {
		usage["cache_creation_input_tokens"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.input_audio_tokens"]); val > 0 {
		usage["audio_input"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.output_audio_tokens"]); val > 0 {
		usage["audio_output"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.reasoning_tokens"]); val > 0 {
		usage["reasoning_tokens"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.image_tokens"]); val > 0 {
		usage["image_tokens"] = val
	}
	if val := extractUint64FromInterface(attrs["gen_ai.usage.video_tokens"]); val > 0 {
		usage["video_tokens"] = val
	}

	// Calculate total (excludes cache subsets which are already counted in input)
	var total uint64
	if input, ok := usage["input"]; ok {
		total += input
	}
	if output, ok := usage["output"]; ok {
		total += output
	}
	if reasoning, ok := usage["reasoning_tokens"]; ok {
		total += reasoning
	}
	if audioIn, ok := usage["audio_input"]; ok {
		total += audioIn
	}
	if audioOut, ok := usage["audio_output"]; ok {
		total += audioOut
	}
	if image, ok := usage["image_tokens"]; ok {
		total += image
	}
	if video, ok := usage["video_tokens"]; ok {
		total += video
	}

	if total > 0 {
		usage["total"] = total
	}

	if len(usage) == 0 {
		return
	}

	projectIDPtr := (*uuid.UUID)(nil)
	if projectID != "" {
		if pid, err := uuid.Parse(projectID); err == nil {
			projectIDPtr = &pid
		}
	}

	providerPricing, err := s.providerPricingService.GetProviderPricingSnapshot(ctx, projectIDPtr, modelName, time.Now())
	if err != nil {
		s.logger.Warn("Failed to get provider pricing - continuing without cost data", "model", modelName, "project_id", projectID, "error", err)
		payload["usage_details"] = usage
		return
	}

	providerCost := s.providerPricingService.CalculateProviderCost(usage, providerPricing)

	providerPricingSnapshot := make(map[string]decimal.Decimal)
	for usageType, price := range providerPricing.Pricing {
		key := fmt.Sprintf("%s_price_per_million", usageType)
		providerPricingSnapshot[key] = price
	}

	payload["usage_details"] = usage
	payload["cost_details"] = providerCost
	payload["pricing_snapshot"] = providerPricingSnapshot

	if totalCost, ok := providerCost["total"]; ok {
		payload["total_cost"] = totalCost
	}

	s.logger.Debug("Provider costs calculated successfully", "model", modelName, "usage_types", len(usage), "total_tokens", total, "provider_cost_usd", providerCost["total"], "provider_pricing_date", providerPricing.SnapshotTime)
}

func extractStringFromInterface(val any) string {
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

func extractUint64FromInterface(val any) uint64 {
	switch v := val.(type) {
	case float64:
		return uint64(v)
	case int64:
		return uint64(v)
	case int32:
		return uint64(v)
	case int:
		return uint64(v)
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case uint:
		return uint64(v)
	default:
		return 0
	}
}

func extractBoolFromInterface(val any) bool {
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// stringify converts various types to JSON string representation
func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []any, map[string]any:
		if jsonBytes, err := json.Marshal(val); err == nil {
			return string(jsonBytes)
		}
	}
	return fmt.Sprintf("%v", v)
}

// extractVercelAISDK extracts input/output from Vercel AI SDK attributes.
// Handles composite output (text + toolCalls) and legacy attributes (<4.0).
func extractVercelAISDK(attrs map[string]any) (input, output string) {
	// Input priority: ai.prompt.messages > ai.prompt > ai.toolCall.args
	if v, ok := attrs[AttrVercelPromptMessages]; ok && v != nil {
		input = stringify(v)
	} else if v, ok := attrs[AttrVercelPrompt]; ok && v != nil {
		input = stringify(v)
	} else if v, ok := attrs[AttrVercelToolCallArgs]; ok && v != nil {
		input = stringify(v)
	}

	// Output: Composite if both text and toolCalls exist
	responseText, hasText := attrs[AttrVercelResponseText]
	toolCalls, hasToolCalls := attrs[AttrVercelResponseToolCalls]

	// Also check legacy result attributes (<4.0)
	if !hasText {
		if v, ok := attrs[AttrVercelResultText]; ok && v != nil {
			responseText = v
			hasText = true
		}
	}
	if !hasToolCalls {
		if v, ok := attrs[AttrVercelResultToolCalls]; ok && v != nil {
			toolCalls = v
			hasToolCalls = true
		}
	}

	if hasText && hasToolCalls && responseText != nil && toolCalls != nil {
		// Combine into ChatML format with both content and tool_calls
		textStr := stringify(responseText)
		toolCallsStr := stringify(toolCalls)
		output = fmt.Sprintf(`{"role":"assistant","content":%s,"tool_calls":%s}`,
			quoteIfNotJSON(textStr), toolCallsStr)
	} else if hasText && responseText != nil {
		output = stringify(responseText)
	} else if v, ok := attrs[AttrVercelResponseObject]; ok && v != nil {
		output = stringify(v)
	} else if v, ok := attrs[AttrVercelResultObject]; ok && v != nil {
		output = stringify(v)
	} else if hasToolCalls && toolCalls != nil {
		output = stringify(toolCalls)
	}

	return input, output
}

// quoteIfNotJSON wraps a string in JSON quotes if it's not already valid JSON
func quoteIfNotJSON(s string) string {
	if json.Valid([]byte(s)) {
		return s
	}
	// Quote as JSON string
	if jsonBytes, err := json.Marshal(s); err == nil {
		return string(jsonBytes)
	}
	return `"` + s + `"`
}

// extractFromSpanEvents extracts input/output from OTEL GenAI span events.
// Handles gen_ai.user.message, gen_ai.system.message, gen_ai.assistant.message,
// gen_ai.tool.message (input) and gen_ai.choice (output).
func extractFromSpanEvents(events []map[string]any) (input, output string) {
	var inputMessages []map[string]any
	var outputChoices []map[string]any

	for _, event := range events {
		eventName, _ := event["name"].(string)

		// Handle both map[string]string and map[string]any for robustness
		// createSpanEvent() stores attributes as map[string]string via convertToStringMap(),
		// but we handle both types for flexibility and future-proofing
		var attrMap map[string]any
		switch attrs := event["attributes"].(type) {
		case map[string]string:
			attrMap = make(map[string]any, len(attrs))
			for k, v := range attrs {
				attrMap[k] = v
			}
		case map[string]any:
			attrMap = attrs
		default:
			attrMap = make(map[string]any)
		}

		switch eventName {
		case "gen_ai.system.message", "gen_ai.user.message",
			"gen_ai.assistant.message", "gen_ai.tool.message":
			// Extract role from event name
			role := extractRoleFromEventName(eventName)
			msg := map[string]any{"role": role}
			for k, v := range attrMap {
				msg[k] = v
			}
			inputMessages = append(inputMessages, msg)
		case "gen_ai.choice":
			outputChoices = append(outputChoices, attrMap)
		}
	}

	if len(inputMessages) > 0 {
		if jsonBytes, err := json.Marshal(inputMessages); err == nil {
			input = string(jsonBytes)
		}
	}
	if len(outputChoices) == 1 {
		if jsonBytes, err := json.Marshal(outputChoices[0]); err == nil {
			output = string(jsonBytes)
		}
	} else if len(outputChoices) > 1 {
		if jsonBytes, err := json.Marshal(outputChoices); err == nil {
			output = string(jsonBytes)
		}
	}
	return input, output
}

// extractRoleFromEventName extracts the role from an OTEL GenAI event name.
// e.g., "gen_ai.user.message" -> "user"
func extractRoleFromEventName(eventName string) string {
	// Pattern: gen_ai.<role>.message
	parts := strings.Split(eventName, ".")
	if len(parts) >= 3 && parts[0] == "gen_ai" && parts[2] == "message" {
		return parts[1]
	}
	return "unknown"
}

// extractInputOutput is the unified framework-aware I/O extraction function.
// It follows a priority chain: Vercel AI SDK > Span Events > OTEL GenAI > OpenInference
func extractInputOutput(params ExtractIOParams) (input, output, inputMime, outputMime string, truncated InputOutputTruncated) {
	attrs := params.Attributes
	events := params.Events
	scopeName := params.InstrumentationScopeName
	maxSize := params.MaxSize
	if maxSize == 0 {
		maxSize = MaxAttributeValueSize
	}

	// Priority 1: Vercel AI SDK (instrumentationScopeName === "ai")
	if scopeName == "ai" {
		input, output = extractVercelAISDK(attrs)
		if input != "" || output != "" {
			return applyTruncationAndMime(input, output, maxSize)
		}
	}

	// Priority 2: OTEL GenAI Span Events
	if len(events) > 0 {
		input, output = extractFromSpanEvents(events)
		if input != "" || output != "" {
			return applyTruncationAndMime(input, output, maxSize)
		}
	}

	// Priority 3: OTEL GenAI Messages (existing implementation)
	input, output, systemInstructions := extractGenAIMessages(attrs)

	// Combine system instructions with input for complete LLM context
	if input == "" && systemInstructions != "" {
		// System instructions only (no input messages)
		input = systemInstructions
	} else if input != "" && systemInstructions != "" {
		// Prepend system instructions to input messages
		input = combineMessagesJSON(systemInstructions, input)
	}

	if input != "" || output != "" {
		return applyTruncationAndMime(input, output, maxSize)
	}

	// Priority 4: OpenInference (existing fallback)
	return extractOpenInference(attrs, maxSize)
}

// extractGenAIMessages extracts I/O from gen_ai.input/output.messages and system_instructions attributes
// Returns: input, output, systemInstructions
func extractGenAIMessages(attrs map[string]any) (input, output, systemInstructions string) {
	// Input: gen_ai.input.messages
	if messages, ok := attrs[AttrGenAIInputMessages].([]any); ok {
		if jsonBytes, err := json.Marshal(messages); err == nil {
			input = string(jsonBytes)
		}
	} else if messages, ok := attrs[AttrGenAIInputMessages].(string); ok && messages != "" {
		input = messages
	}

	// Output: gen_ai.output.messages
	if messages, ok := attrs[AttrGenAIOutputMessages].([]any); ok {
		if jsonBytes, err := json.Marshal(messages); err == nil {
			output = string(jsonBytes)
		}
	} else if messages, ok := attrs[AttrGenAIOutputMessages].(string); ok && messages != "" {
		output = messages
	}

	// System Instructions: gen_ai.system_instructions (OTEL GenAI 1.28+)
	if instructions, ok := attrs[AttrGenAISystemInstructions].([]any); ok {
		if jsonBytes, err := json.Marshal(instructions); err == nil {
			systemInstructions = string(jsonBytes)
		}
	} else if instructions, ok := attrs[AttrGenAISystemInstructions].(string); ok && instructions != "" {
		systemInstructions = instructions
	}

	return input, output, systemInstructions
}

// combineMessagesJSON combines two JSON arrays of messages.
// Used to prepend system instructions to input messages for complete LLM context.
func combineMessagesJSON(systemJSON, inputJSON string) string {
	var systemMsgs, inputMsgs []any

	if err := json.Unmarshal([]byte(systemJSON), &systemMsgs); err != nil {
		return inputJSON // Fallback to input only
	}
	if err := json.Unmarshal([]byte(inputJSON), &inputMsgs); err != nil {
		return systemJSON // Fallback to system only
	}

	// Combine: system messages first, then input messages
	combined := append(systemMsgs, inputMsgs...)
	if jsonBytes, err := json.Marshal(combined); err == nil {
		return string(jsonBytes)
	}
	return inputJSON
}

// extractOpenInference extracts I/O from input.value/output.value attributes (OpenInference)
func extractOpenInference(attrs map[string]any, maxSize int) (input, output, inputMime, outputMime string, truncated InputOutputTruncated) {
	declaredInputMime, _ := attrs[AttrInputMimeType].(string)
	declaredOutputMime, _ := attrs[AttrOutputMimeType].(string)

	// Input: input.value
	if inputVal, ok := attrs[AttrInputValue]; ok && inputVal != nil {
		switch v := inputVal.(type) {
		case string:
			if v != "" {
				input = v
				inputMime = validateMimeType(v, declaredInputMime)
			}
		case []any, map[string]any:
			if jsonBytes, err := json.Marshal(v); err == nil {
				input = string(jsonBytes)
				inputMime = "application/json"
			}
		}
	}

	// Output: output.value
	if outputVal, ok := attrs[AttrOutputValue]; ok && outputVal != nil {
		switch v := outputVal.(type) {
		case string:
			if v != "" {
				output = v
				outputMime = validateMimeType(v, declaredOutputMime)
			}
		case []any, map[string]any:
			if jsonBytes, err := json.Marshal(v); err == nil {
				output = string(jsonBytes)
				outputMime = "application/json"
			}
		}
	}

	// Apply truncation
	if input != "" {
		input, truncated.Input = truncateWithIndicator(input, maxSize)
	}
	if output != "" {
		output, truncated.Output = truncateWithIndicator(output, maxSize)
	}

	return input, output, inputMime, outputMime, truncated
}

// applyTruncationAndMime applies truncation and MIME type detection to extracted I/O
func applyTruncationAndMime(input, output string, maxSize int) (string, string, string, string, InputOutputTruncated) {
	var truncated InputOutputTruncated
	var inputMime, outputMime string

	if input != "" {
		input, truncated.Input = truncateWithIndicator(input, maxSize)
		inputMime = validateMimeType(input, "")
	}
	if output != "" {
		output, truncated.Output = truncateWithIndicator(output, maxSize)
		outputMime = validateMimeType(output, "")
	}

	return input, output, inputMime, outputMime, truncated
}

// isFrameworkIOKey checks if a key is a framework I/O key that should be filtered from metadata
func isFrameworkIOKey(key string) bool {
	return frameworkIOKeys[key]
}

// filterIOKeysFromMetadata filters framework I/O keys from attributes to prevent duplication
func filterIOKeysFromMetadata(attrs map[string]any) map[string]any {
	filtered := make(map[string]any)
	for k, v := range attrs {
		if !isFrameworkIOKey(k) {
			filtered[k] = v
		}
	}
	return filtered
}
