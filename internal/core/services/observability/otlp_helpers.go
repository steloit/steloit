package observability

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// Shared helper functions for OTLP converters (traces, metrics, logs)

// convertKeyValuesToMap converts OTLP KeyValue array to map[string]string
// All values are converted to strings for ClickHouse Map(String, String) columns
func convertKeyValuesToMap(kvs []*commonpb.KeyValue) map[string]string {
	result := make(map[string]string)
	for _, kv := range kvs {
		key := kv.GetKey()
		value := attributeValueToString(kv.GetValue())
		result[key] = value
	}
	return result
}

// attributeValueToString converts OTLP AnyValue to string
// Handles all AnyValue types: string, int, double, bool, bytes, array, kvlist
func attributeValueToString(value *commonpb.AnyValue) string {
	if value == nil {
		return ""
	}

	switch v := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return v.StringValue
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", v.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%f", v.DoubleValue)
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", v.BoolValue)
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(v.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		// JSON-encode arrays
		jsonBytes, _ := json.Marshal(anyValueArrayToStrings(v.ArrayValue))
		return string(jsonBytes)
	case *commonpb.AnyValue_KvlistValue:
		// JSON-encode key-value lists
		jsonBytes, _ := json.Marshal(convertKeyValuesToMap(v.KvlistValue.GetValues()))
		return string(jsonBytes)
	default:
		return ""
	}
}

// anyValueArrayToStrings converts OTLP ArrayValue to Go []string
func anyValueArrayToStrings(arrayValue *commonpb.ArrayValue) []string {
	if arrayValue == nil {
		return []string{}
	}

	values := arrayValue.GetValues()
	result := make([]string, len(values))
	for i, v := range values {
		result[i] = attributeValueToString(v)
	}
	return result
}

// convertEntityToPayload converts a typed domain entity to map[string]any for TelemetryEventRequest
// This preserves type safety during conversion while matching the expected payload format
func convertEntityToPayload(entity any) map[string]any {
	// Serialize to JSON then deserialize to map (simple, reliable)
	jsonBytes, err := json.Marshal(entity)
	if err != nil {
		return make(map[string]any)
	}

	var payload map[string]any
	if err := json.Unmarshal(jsonBytes, &payload); err != nil {
		return make(map[string]any)
	}

	return payload
}

// extractResourceAttributes converts OTLP Resource to map[string]string
// Handles both metrics (resourcepb.Resource) and logs (any) resource types
func extractResourceAttributes(resource any) map[string]string {
	// Handle nil resource
	if resource == nil {
		return make(map[string]string)
	}

	// Try metrics resource type first (resourcepb.Resource)
	if r, ok := resource.(*resourcepb.Resource); ok {
		return convertKeyValuesToMap(r.GetAttributes())
	}

	// Try logs resource type (generic interface with GetAttributes method)
	type resourceInterface interface {
		GetAttributes() []*commonpb.KeyValue
	}

	if r, ok := resource.(resourceInterface); ok {
		return convertKeyValuesToMap(r.GetAttributes())
	}

	return make(map[string]string)
}

// extractScopeAttributes converts OTLP InstrumentationScope attributes to map[string]string
func extractScopeAttributes(scope *commonpb.InstrumentationScope) map[string]string {
	if scope == nil {
		return make(map[string]string)
	}
	return convertKeyValuesToMap(scope.GetAttributes())
}
