package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test extractGenericInput with LLM messages (highest priority)
func TestExtractGenericInput_GenAIMessages_Array(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.input.messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
		},
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Contains(t, value, `"role":"user"`)
	assert.Contains(t, value, `"content":"Hello"`)
	assert.Equal(t, "application/json", mimeType)
}

func TestExtractGenericInput_GenAIMessages_String(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.input.messages": `[{"role":"user","content":"Hello"}]`,
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Equal(t, `[{"role":"user","content":"Hello"}]`, value)
	assert.Equal(t, "application/json", mimeType)
}

// Test extractGenericInput with input.value (fallback)
func TestExtractGenericInput_InputValue_String(t *testing.T) {
	attrs := map[string]any{
		"input.value": `{"location":"Bangalore","units":"celsius"}`,
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Equal(t, `{"location":"Bangalore","units":"celsius"}`, value)
	assert.Equal(t, "application/json", mimeType) // Auto-detected
}

func TestExtractGenericInput_InputValue_StringWithMimeType(t *testing.T) {
	attrs := map[string]any{
		"input.value":     "Hello, World!",
		"input.mime_type": "text/plain",
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Equal(t, "Hello, World!", value)
	assert.Equal(t, "text/plain", mimeType)
}

// Test extractGenericInput with object input (new capability)
func TestExtractGenericInput_InputValue_Object(t *testing.T) {
	attrs := map[string]any{
		"input.value": map[string]any{
			"location": "Bangalore",
			"units":    "celsius",
		},
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Contains(t, value, `"location":"Bangalore"`)
	assert.Contains(t, value, `"units":"celsius"`)
	assert.Equal(t, "application/json", mimeType)
}

// Test extractGenericInput with array input (new capability)
func TestExtractGenericInput_InputValue_Array(t *testing.T) {
	attrs := map[string]any{
		"input.value": []any{"arg1", "arg2", 123},
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Equal(t, `["arg1","arg2",123]`, value)
	assert.Equal(t, "application/json", mimeType)
}

// Test priority: gen_ai.input.messages takes precedence over input.value
func TestExtractGenericInput_PriorityOrder(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.input.messages": `[{"role":"user","content":"high priority"}]`,
		"input.value":           `{"generic":"low priority"}`,
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Equal(t, `[{"role":"user","content":"high priority"}]`, value)
	assert.Equal(t, "application/json", mimeType)
}

// Test extractGenericInput returns empty when no input
func TestExtractGenericInput_Empty(t *testing.T) {
	attrs := map[string]any{
		"other.attribute": "value",
	}

	value, mimeType := extractGenericInput(attrs)

	assert.Equal(t, "", value)
	assert.Equal(t, "", mimeType)
}

// Test extractGenericOutput with LLM messages (highest priority)
func TestExtractGenericOutput_GenAIMessages_Array(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.output.messages": []any{
			map[string]any{"role": "assistant", "content": "Hello back!"},
		},
	}

	value, mimeType := extractGenericOutput(attrs)

	assert.Contains(t, value, `"role":"assistant"`)
	assert.Contains(t, value, `"content":"Hello back!"`)
	assert.Equal(t, "application/json", mimeType)
}

func TestExtractGenericOutput_GenAIMessages_String(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.output.messages": `[{"role":"assistant","content":"Response"}]`,
	}

	value, mimeType := extractGenericOutput(attrs)

	assert.Equal(t, `[{"role":"assistant","content":"Response"}]`, value)
	assert.Equal(t, "application/json", mimeType)
}

// Test extractGenericOutput with output.value (fallback)
func TestExtractGenericOutput_OutputValue_String(t *testing.T) {
	attrs := map[string]any{
		"output.value": `{"temperature":25,"conditions":"sunny"}`,
	}

	value, mimeType := extractGenericOutput(attrs)

	assert.Equal(t, `{"temperature":25,"conditions":"sunny"}`, value)
	assert.Equal(t, "application/json", mimeType)
}

// Test extractGenericOutput with object output (new capability)
func TestExtractGenericOutput_OutputValue_Object(t *testing.T) {
	attrs := map[string]any{
		"output.value": map[string]any{
			"temperature": 25,
			"conditions":  "sunny",
		},
	}

	value, mimeType := extractGenericOutput(attrs)

	assert.Contains(t, value, `"temperature":25`)
	assert.Contains(t, value, `"conditions":"sunny"`)
	assert.Equal(t, "application/json", mimeType)
}

// Test extractGenericOutput with array output (new capability)
func TestExtractGenericOutput_OutputValue_Array(t *testing.T) {
	attrs := map[string]any{
		"output.value": []any{"result1", "result2"},
	}

	value, mimeType := extractGenericOutput(attrs)

	assert.Equal(t, `["result1","result2"]`, value)
	assert.Equal(t, "application/json", mimeType)
}

// Test priority: gen_ai.output.messages takes precedence
func TestExtractGenericOutput_PriorityOrder(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.output.messages": `[{"role":"assistant","content":"high priority"}]`,
		"output.value":           `{"generic":"low priority"}`,
	}

	value, mimeType := extractGenericOutput(attrs)

	assert.Equal(t, `[{"role":"assistant","content":"high priority"}]`, value)
	assert.Equal(t, "application/json", mimeType)
}

// Test extractToolMetadata
func TestExtractToolMetadata_ToolName(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.tool.name": "get_weather",
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Equal(t, "get_weather", payload["tool_name"])
}

func TestExtractToolMetadata_ToolParameters_String(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.tool.name":       "get_weather",
		"gen_ai.tool.parameters": `{"location":"Bangalore"}`,
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Equal(t, "get_weather", payload["tool_name"])
	assert.Equal(t, `{"location":"Bangalore"}`, payload["tool_parameters"])
}

func TestExtractToolMetadata_ToolParameters_Object(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.tool.name": "get_weather",
		"gen_ai.tool.parameters": map[string]any{
			"location": "Bangalore",
			"units":    "celsius",
		},
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Equal(t, "get_weather", payload["tool_name"])
	assert.Contains(t, payload["tool_parameters"].(string), `"location":"Bangalore"`)
}

func TestExtractToolMetadata_ToolResult_String(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.tool.name":   "get_weather",
		"gen_ai.tool.result": `{"temp":25}`,
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Equal(t, "get_weather", payload["tool_name"])
	assert.Equal(t, `{"temp":25}`, payload["tool_result"])
}

func TestExtractToolMetadata_ToolResult_Object(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.tool.name": "get_weather",
		"gen_ai.tool.result": map[string]any{
			"temp":       25,
			"conditions": "sunny",
		},
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Equal(t, "get_weather", payload["tool_name"])
	assert.Contains(t, payload["tool_result"].(string), `"temp":25`)
}

func TestExtractToolMetadata_ToolCallID(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.tool.name":    "get_weather",
		"gen_ai.tool.call.id": "call_abc123",
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Equal(t, "get_weather", payload["tool_name"])
	assert.Equal(t, "call_abc123", payload["tool_call_id"])
}

func TestExtractToolMetadata_AllFields(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.tool.name":       "search_database",
		"gen_ai.tool.parameters": `{"query":"SELECT * FROM users"}`,
		"gen_ai.tool.result":     `{"rows":10}`,
		"gen_ai.tool.call.id":    "call_xyz789",
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Equal(t, "search_database", payload["tool_name"])
	assert.Equal(t, `{"query":"SELECT * FROM users"}`, payload["tool_parameters"])
	assert.Equal(t, `{"rows":10}`, payload["tool_result"])
	assert.Equal(t, "call_xyz789", payload["tool_call_id"])
}

func TestExtractToolMetadata_Empty(t *testing.T) {
	attrs := map[string]any{
		"other.attribute": "value",
	}
	payload := make(map[string]any)

	extractToolMetadata(attrs, payload)

	assert.Nil(t, payload["tool_name"])
	assert.Nil(t, payload["tool_parameters"])
	assert.Nil(t, payload["tool_result"])
	assert.Nil(t, payload["tool_call_id"])
}

// Test validateMimeType
func TestValidateMimeType_AutoDetectJSON(t *testing.T) {
	result := validateMimeType(`{"key":"value"}`, "")
	assert.Equal(t, "application/json", result)
}

func TestValidateMimeType_AutoDetectPlainText(t *testing.T) {
	result := validateMimeType("Hello World", "")
	assert.Equal(t, "text/plain", result)
}

func TestValidateMimeType_DeclaredValid(t *testing.T) {
	result := validateMimeType(`{"key":"value"}`, "application/json")
	assert.Equal(t, "application/json", result)
}

func TestValidateMimeType_DeclaredInvalid(t *testing.T) {
	// Declared JSON but content is not valid JSON
	result := validateMimeType("not valid json", "application/json")
	assert.Equal(t, "text/plain", result)
}

// Test truncateWithIndicator
func TestTruncateWithIndicator_NoTruncation(t *testing.T) {
	value := "short string"
	result, truncated := truncateWithIndicator(value, 100)

	assert.Equal(t, value, result)
	assert.False(t, truncated)
}

func TestTruncateWithIndicator_Truncation(t *testing.T) {
	value := "this is a longer string that needs truncation"
	result, truncated := truncateWithIndicator(value, 20)

	assert.True(t, truncated)
	assert.Contains(t, result, "...[truncated]")
	assert.True(t, len(result) < len(value)+15) // Original truncated + indicator
}

// Regression test: extractGenAIFields should NOT overwrite payload["input"]/["output"]
// These are already set by createSpanEvent with proper truncation and MIME type handling.
func TestExtractGenAIFields_DoesNotOverwriteExistingInputOutput(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.input.messages":  `[{"role":"user","content":"from attrs"}]`,
		"gen_ai.output.messages": `[{"role":"assistant","content":"from attrs"}]`,
		"gen_ai.provider.name":   "openai",
		"gen_ai.request.model":   "gpt-4",
	}

	payload := map[string]any{
		"input":           "already set with truncation",
		"input_mime_type": "application/json",
		"output":          "already set with truncation",
	}

	extractGenAIFields(attrs, payload)

	// Input/output should NOT be overwritten by extractGenAIFields
	assert.Equal(t, "already set with truncation", payload["input"])
	assert.Equal(t, "already set with truncation", payload["output"])
	assert.Equal(t, "application/json", payload["input_mime_type"])

	// Other fields should still be extracted
	assert.Equal(t, "openai", payload["provider"])
	assert.Equal(t, "gpt-4", payload["model_name"])
}

// ============================================================================
// Tests for extractInputOutput (unified framework-aware I/O extraction)
// ============================================================================

// Test Vercel AI SDK - Basic Input (ai.prompt.messages)
func TestExtractInputOutput_VercelAISDK_PromptMessages(t *testing.T) {
	attrs := map[string]any{
		AttrVercelPromptMessages: `[{"role":"user","content":"Hello from Vercel"}]`,
	}
	input, _, inputMime, _, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Equal(t, `[{"role":"user","content":"Hello from Vercel"}]`, input)
	assert.Equal(t, "application/json", inputMime)
}

// Test Vercel AI SDK - ai.prompt (fallback)
func TestExtractInputOutput_VercelAISDK_Prompt(t *testing.T) {
	attrs := map[string]any{
		AttrVercelPrompt: "What is the weather?",
	}
	input, _, inputMime, _, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Equal(t, "What is the weather?", input)
	assert.Equal(t, "text/plain", inputMime)
}

// Test Vercel AI SDK - ai.toolCall.args
func TestExtractInputOutput_VercelAISDK_ToolCallArgs(t *testing.T) {
	attrs := map[string]any{
		AttrVercelToolCallArgs: `{"location":"Bangalore","units":"celsius"}`,
	}
	input, _, inputMime, _, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Equal(t, `{"location":"Bangalore","units":"celsius"}`, input)
	assert.Equal(t, "application/json", inputMime)
}

// Test Vercel AI SDK - Basic Output (ai.response.text)
func TestExtractInputOutput_VercelAISDK_ResponseText(t *testing.T) {
	attrs := map[string]any{
		AttrVercelResponseText: "The weather is sunny.",
	}
	_, output, _, outputMime, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Equal(t, "The weather is sunny.", output)
	assert.Equal(t, "text/plain", outputMime)
}

// Test Vercel AI SDK - Composite Output (text + toolCalls)
func TestExtractInputOutput_VercelAISDK_CompositeToolCalls(t *testing.T) {
	attrs := map[string]any{
		AttrVercelResponseText:      "Let me check the weather.",
		AttrVercelResponseToolCalls: `[{"name":"get_weather","args":{"location":"Bangalore"}}]`,
	}
	_, output, _, outputMime, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Contains(t, output, `"role":"assistant"`)
	assert.Contains(t, output, `"tool_calls"`)
	assert.Contains(t, output, `get_weather`)
	assert.Equal(t, "application/json", outputMime)
}

// Test Vercel AI SDK - Legacy result attributes (<4.0)
func TestExtractInputOutput_VercelAISDK_LegacyResult(t *testing.T) {
	attrs := map[string]any{
		AttrVercelResultText: "Legacy response text",
	}
	_, output, _, outputMime, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Equal(t, "Legacy response text", output)
	assert.Equal(t, "text/plain", outputMime)
}

// Test Vercel AI SDK - ai.response.object
func TestExtractInputOutput_VercelAISDK_ResponseObject(t *testing.T) {
	attrs := map[string]any{
		AttrVercelResponseObject: `{"weather":"sunny","temperature":25}`,
	}
	_, output, _, outputMime, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Equal(t, `{"weather":"sunny","temperature":25}`, output)
	assert.Equal(t, "application/json", outputMime)
}

// Test OTEL GenAI Messages (Priority 3 - existing implementation)
func TestExtractInputOutput_OTELGenAI_Messages(t *testing.T) {
	attrs := map[string]any{
		AttrGenAIInputMessages:  `[{"role":"user","content":"OTEL message"}]`,
		AttrGenAIOutputMessages: `[{"role":"assistant","content":"OTEL response"}]`,
	}
	input, output, inputMime, outputMime, _ := extractInputOutput(ExtractIOParams{
		Attributes: attrs,
	})
	assert.Equal(t, `[{"role":"user","content":"OTEL message"}]`, input)
	assert.Equal(t, `[{"role":"assistant","content":"OTEL response"}]`, output)
	assert.Equal(t, "application/json", inputMime)
	assert.Equal(t, "application/json", outputMime)
}

// Test OpenInference (Priority 4 - fallback)
func TestExtractInputOutput_OpenInference(t *testing.T) {
	attrs := map[string]any{
		AttrInputValue:  `{"query":"test query"}`,
		AttrOutputValue: `{"result":"test result"}`,
	}
	input, output, inputMime, outputMime, _ := extractInputOutput(ExtractIOParams{
		Attributes: attrs,
	})
	assert.Equal(t, `{"query":"test query"}`, input)
	assert.Equal(t, `{"result":"test result"}`, output)
	assert.Equal(t, "application/json", inputMime)
	assert.Equal(t, "application/json", outputMime)
}

// Test OpenInference with declared MIME type
func TestExtractInputOutput_OpenInference_WithMimeType(t *testing.T) {
	attrs := map[string]any{
		AttrInputValue:     "Plain text input",
		AttrInputMimeType:  "text/plain",
		AttrOutputValue:    "Plain text output",
		AttrOutputMimeType: "text/plain",
	}
	input, output, inputMime, outputMime, _ := extractInputOutput(ExtractIOParams{
		Attributes: attrs,
	})
	assert.Equal(t, "Plain text input", input)
	assert.Equal(t, "Plain text output", output)
	assert.Equal(t, "text/plain", inputMime)
	assert.Equal(t, "text/plain", outputMime)
}

// Test Priority Order: Vercel AI SDK > OTEL GenAI > OpenInference
func TestExtractInputOutput_PriorityOrder_VercelOverOTEL(t *testing.T) {
	attrs := map[string]any{
		AttrVercelPromptMessages: `[{"role":"user","content":"vercel"}]`,
		AttrGenAIInputMessages:   `[{"role":"user","content":"otel"}]`,
		AttrInputValue:           `{"data":"openinference"}`,
	}
	input, _, _, _, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "ai",
	})
	assert.Contains(t, input, "vercel")
	assert.NotContains(t, input, "otel")
	assert.NotContains(t, input, "openinference")
}

// Test Priority Order: OTEL GenAI > OpenInference (when not Vercel)
func TestExtractInputOutput_PriorityOrder_OTELOverOpenInference(t *testing.T) {
	attrs := map[string]any{
		AttrGenAIInputMessages: `[{"role":"user","content":"otel"}]`,
		AttrInputValue:         `{"data":"openinference"}`,
	}
	input, _, _, _, _ := extractInputOutput(ExtractIOParams{
		Attributes: attrs,
	})
	assert.Contains(t, input, "otel")
	assert.NotContains(t, input, "openinference")
}

// Test that Vercel attributes are ignored when scope is not "ai"
func TestExtractInputOutput_VercelIgnoredWithoutScope(t *testing.T) {
	attrs := map[string]any{
		AttrVercelPromptMessages: `[{"role":"user","content":"vercel"}]`,
		AttrGenAIInputMessages:   `[{"role":"user","content":"otel"}]`,
	}
	input, _, _, _, _ := extractInputOutput(ExtractIOParams{
		Attributes:               attrs,
		InstrumentationScopeName: "other-sdk", // Not "ai"
	})
	assert.Contains(t, input, "otel")
	assert.NotContains(t, input, "vercel")
}

// Test OTEL GenAI Span Events - User message
func TestExtractInputOutput_SpanEvents_UserMessage(t *testing.T) {
	events := []map[string]any{
		{
			"name":       "gen_ai.user.message",
			"attributes": map[string]string{"content": "Hello from user"},
		},
	}
	input, _, inputMime, _, _ := extractInputOutput(ExtractIOParams{
		Events: events,
	})
	assert.Contains(t, input, `"role":"user"`)
	assert.Contains(t, input, `"content":"Hello from user"`)
	assert.Equal(t, "application/json", inputMime)
}

// Test OTEL GenAI Span Events - System + User messages
func TestExtractInputOutput_SpanEvents_MultipleMessages(t *testing.T) {
	events := []map[string]any{
		{
			"name":       "gen_ai.system.message",
			"attributes": map[string]string{"content": "You are helpful"},
		},
		{
			"name":       "gen_ai.user.message",
			"attributes": map[string]string{"content": "Hello"},
		},
	}
	input, _, _, _, _ := extractInputOutput(ExtractIOParams{
		Events: events,
	})
	assert.Contains(t, input, `"role":"system"`)
	assert.Contains(t, input, `"role":"user"`)
	assert.Contains(t, input, `"content":"You are helpful"`)
	assert.Contains(t, input, `"content":"Hello"`)
}

// Test OTEL GenAI Span Events - Choice output
func TestExtractInputOutput_SpanEvents_Choice(t *testing.T) {
	events := []map[string]any{
		{
			"name":       "gen_ai.choice",
			"attributes": map[string]string{"content": "AI response", "index": "0"},
		},
	}
	_, output, _, outputMime, _ := extractInputOutput(ExtractIOParams{
		Events: events,
	})
	assert.Contains(t, output, `"content":"AI response"`)
	assert.Equal(t, "application/json", outputMime)
}

// Test Truncation
func TestExtractInputOutput_Truncation(t *testing.T) {
	longValue := ""
	for i := 0; i < 100; i++ {
		longValue += "This is a long string that needs truncation. "
	}
	attrs := map[string]any{
		AttrInputValue: longValue,
	}
	input, _, _, _, truncated := extractInputOutput(ExtractIOParams{
		Attributes: attrs,
		MaxSize:    100,
	})
	assert.True(t, truncated.Input)
	assert.Contains(t, input, "...[truncated]")
	assert.LessOrEqual(t, len(input), 120) // 100 + truncation indicator
}

// Test Empty attributes
func TestExtractInputOutput_Empty(t *testing.T) {
	attrs := map[string]any{
		"other.attribute": "value",
	}
	input, output, inputMime, outputMime, truncated := extractInputOutput(ExtractIOParams{
		Attributes: attrs,
	})
	assert.Equal(t, "", input)
	assert.Equal(t, "", output)
	assert.Equal(t, "", inputMime)
	assert.Equal(t, "", outputMime)
	assert.False(t, truncated.Input)
	assert.False(t, truncated.Output)
}

// ============================================================================
// Tests for extractVercelAISDK
// ============================================================================

func TestExtractVercelAISDK_InputPriority(t *testing.T) {
	// Test that ai.prompt.messages takes precedence over ai.prompt
	attrs := map[string]any{
		AttrVercelPromptMessages: `[{"role":"user","content":"messages"}]`,
		AttrVercelPrompt:         "plain prompt",
	}
	input, _ := extractVercelAISDK(attrs)
	assert.Contains(t, input, "messages")
	assert.NotContains(t, input, "plain prompt")
}

func TestExtractVercelAISDK_OutputWithToolCallsOnly(t *testing.T) {
	attrs := map[string]any{
		AttrVercelResponseToolCalls: `[{"name":"search"}]`,
	}
	_, output := extractVercelAISDK(attrs)
	assert.Equal(t, `[{"name":"search"}]`, output)
}

// ============================================================================
// Tests for extractFromSpanEvents
// ============================================================================

func TestExtractFromSpanEvents_Empty(t *testing.T) {
	events := []map[string]any{}
	input, output := extractFromSpanEvents(events)
	assert.Equal(t, "", input)
	assert.Equal(t, "", output)
}

func TestExtractFromSpanEvents_MultipleChoices(t *testing.T) {
	events := []map[string]any{
		{
			"name":       "gen_ai.choice",
			"attributes": map[string]string{"content": "Choice 1", "index": "0"},
		},
		{
			"name":       "gen_ai.choice",
			"attributes": map[string]string{"content": "Choice 2", "index": "1"},
		},
	}
	_, output := extractFromSpanEvents(events)
	// Should return as array when multiple choices
	assert.Contains(t, output, "Choice 1")
	assert.Contains(t, output, "Choice 2")
}

// ============================================================================
// Tests for extractRoleFromEventName
// ============================================================================

func TestExtractRoleFromEventName(t *testing.T) {
	tests := []struct {
		eventName string
		expected  string
	}{
		{"gen_ai.user.message", "user"},
		{"gen_ai.system.message", "system"},
		{"gen_ai.assistant.message", "assistant"},
		{"gen_ai.tool.message", "tool"},
		{"unknown_format", "unknown"},
		{"gen_ai.choice", "unknown"}, // Not a message event
	}
	for _, tt := range tests {
		t.Run(tt.eventName, func(t *testing.T) {
			result := extractRoleFromEventName(tt.eventName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Tests for isFrameworkIOKey
// ============================================================================

func TestIsFrameworkIOKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"ai.prompt.messages", true},
		{"ai.response.text", true},
		{"gen_ai.input.messages", true},
		{"gen_ai.output.messages", true},
		{"input.value", true},
		{"output.value", true},
		{"other.attribute", false},
		{"gen_ai.provider.name", false},
		{"gen_ai.request.model", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := isFrameworkIOKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Tests for filterIOKeysFromMetadata
// ============================================================================

func TestFilterIOKeysFromMetadata(t *testing.T) {
	attrs := map[string]any{
		"ai.prompt.messages":    `[{"role":"user"}]`,
		"ai.response.text":      "response",
		"gen_ai.provider.name":  "openai",
		"gen_ai.request.model":  "gpt-4",
		"custom.attribute":      "value",
		"gen_ai.input.messages": `[{"role":"user"}]`,
	}
	filtered := filterIOKeysFromMetadata(attrs)

	// I/O keys should be filtered out
	assert.NotContains(t, filtered, "ai.prompt.messages")
	assert.NotContains(t, filtered, "ai.response.text")
	assert.NotContains(t, filtered, "gen_ai.input.messages")

	// Non-I/O keys should remain
	assert.Contains(t, filtered, "gen_ai.provider.name")
	assert.Contains(t, filtered, "gen_ai.request.model")
	assert.Contains(t, filtered, "custom.attribute")
}

// TestFilterIOKeysFromMetadata_IntegrationWithConvertToStringMap verifies that
// filterIOKeysFromMetadata works correctly when chained with convertToStringMap
func TestFilterIOKeysFromMetadata_IntegrationWithConvertToStringMap(t *testing.T) {
	// Simulate span attributes with both I/O keys and regular attributes
	spanAttrs := map[string]any{
		"input.value":           `{"query":"test"}`,
		"output.value":          `{"result":"done"}`,
		"gen_ai.input.messages": `[{"role":"user"}]`,
		"ai.prompt.messages":    `[{"role":"user"}]`,
		"gen_ai.provider.name":  "openai",
		"gen_ai.request.model":  "gpt-4",
		"custom.attribute":      "value",
	}

	// This is the exact call chain used in createSpanEvent()
	result := convertToStringMap(filterIOKeysFromMetadata(spanAttrs))

	// I/O keys should be filtered out
	assert.NotContains(t, result, "input.value")
	assert.NotContains(t, result, "output.value")
	assert.NotContains(t, result, "gen_ai.input.messages")
	assert.NotContains(t, result, "ai.prompt.messages")

	// Non-I/O keys should remain and be converted to strings
	assert.Equal(t, "openai", result["gen_ai.provider.name"])
	assert.Equal(t, "gpt-4", result["gen_ai.request.model"])
	assert.Equal(t, "value", result["custom.attribute"])
}

// TestExtractFromSpanEvents_TypeHandling verifies robust type handling for event attributes
func TestExtractFromSpanEvents_TypeHandling(t *testing.T) {
	t.Run("handles map[string]string attributes", func(t *testing.T) {
		// This is the actual type stored by createSpanEvent()
		events := []map[string]any{
			{
				"name": "gen_ai.user.message",
				"attributes": map[string]string{
					"content": "Hello from user",
				},
			},
		}
		input, _ := extractFromSpanEvents(events)
		assert.Contains(t, input, "Hello from user")
		assert.Contains(t, input, `"role":"user"`)
	})

	t.Run("handles map[string]any attributes", func(t *testing.T) {
		// Alternative type that might be used in tests or future code
		events := []map[string]any{
			{
				"name": "gen_ai.user.message",
				"attributes": map[string]any{
					"content": "Hello from interface",
				},
			},
		}
		input, _ := extractFromSpanEvents(events)
		assert.Contains(t, input, "Hello from interface")
		assert.Contains(t, input, `"role":"user"`)
	})

	t.Run("handles nil attributes gracefully", func(t *testing.T) {
		events := []map[string]any{
			{
				"name":       "gen_ai.user.message",
				"attributes": nil,
			},
		}
		input, _ := extractFromSpanEvents(events)
		// Should still create message with role, even without attributes
		assert.Contains(t, input, `"role":"user"`)
	})

	t.Run("handles missing attributes gracefully", func(t *testing.T) {
		events := []map[string]any{
			{
				"name": "gen_ai.user.message",
				// No attributes key at all
			},
		}
		input, _ := extractFromSpanEvents(events)
		// Should still create message with role
		assert.Contains(t, input, `"role":"user"`)
	})
}

// =============================================
// Tests for gen_ai.system_instructions support
// =============================================

// TestExtractGenAIMessages_WithSystemInstructions verifies system instructions extraction
func TestExtractGenAIMessages_WithSystemInstructions(t *testing.T) {
	t.Run("extracts all three components", func(t *testing.T) {
		attrs := map[string]any{
			AttrGenAISystemInstructions: []any{
				map[string]any{"role": "system", "content": "You are helpful"},
			},
			AttrGenAIInputMessages: []any{
				map[string]any{"role": "user", "content": "Hello"},
			},
			AttrGenAIOutputMessages: []any{
				map[string]any{"role": "assistant", "content": "Hi there!"},
			},
		}

		input, output, systemInstructions := extractGenAIMessages(attrs)

		assert.Contains(t, systemInstructions, "You are helpful")
		assert.Contains(t, input, "Hello")
		assert.Contains(t, output, "Hi there!")
	})

	t.Run("handles system instructions as string", func(t *testing.T) {
		attrs := map[string]any{
			AttrGenAISystemInstructions: `[{"role":"system","content":"Be concise"}]`,
		}

		_, _, systemInstructions := extractGenAIMessages(attrs)
		assert.Contains(t, systemInstructions, "Be concise")
	})

	t.Run("handles system instructions only", func(t *testing.T) {
		attrs := map[string]any{
			AttrGenAISystemInstructions: []any{
				map[string]any{"role": "system", "content": "System only"},
			},
		}

		input, output, systemInstructions := extractGenAIMessages(attrs)

		assert.Contains(t, systemInstructions, "System only")
		assert.Empty(t, input)
		assert.Empty(t, output)
	})
}

// TestCombineMessagesJSON verifies JSON array merging
func TestCombineMessagesJSON(t *testing.T) {
	t.Run("combines system and input messages", func(t *testing.T) {
		systemJSON := `[{"role":"system","content":"Be helpful"}]`
		inputJSON := `[{"role":"user","content":"Hello"}]`

		combined := combineMessagesJSON(systemJSON, inputJSON)

		assert.Contains(t, combined, "Be helpful")
		assert.Contains(t, combined, "Hello")
		// System should come first
		assert.True(t, len(combined) > 0)
	})

	t.Run("falls back to input on invalid system JSON", func(t *testing.T) {
		systemJSON := `invalid json`
		inputJSON := `[{"role":"user","content":"Hello"}]`

		combined := combineMessagesJSON(systemJSON, inputJSON)
		assert.Equal(t, inputJSON, combined)
	})

	t.Run("falls back to system on invalid input JSON", func(t *testing.T) {
		systemJSON := `[{"role":"system","content":"System"}]`
		inputJSON := `invalid json`

		combined := combineMessagesJSON(systemJSON, inputJSON)
		assert.Equal(t, systemJSON, combined)
	})
}

// TestExtractInputOutput_WithSystemInstructions verifies system instructions integration
func TestExtractInputOutput_WithSystemInstructions(t *testing.T) {
	t.Run("combines system instructions with input messages", func(t *testing.T) {
		attrs := map[string]any{
			AttrGenAISystemInstructions: []any{
				map[string]any{"role": "system", "content": "You are an AI assistant"},
			},
			AttrGenAIInputMessages: []any{
				map[string]any{"role": "user", "content": "What is 2+2?"},
			},
			AttrGenAIOutputMessages: []any{
				map[string]any{"role": "assistant", "content": "4"},
			},
		}

		params := ExtractIOParams{Attributes: attrs}
		input, output, _, _, _ := extractInputOutput(params)

		// Input should contain BOTH system instructions AND user message
		assert.Contains(t, input, "You are an AI assistant")
		assert.Contains(t, input, "What is 2+2?")
		assert.Contains(t, output, "4")
	})

	t.Run("system instructions only (no input messages)", func(t *testing.T) {
		attrs := map[string]any{
			AttrGenAISystemInstructions: []any{
				map[string]any{"role": "system", "content": "System prompt only"},
			},
		}

		params := ExtractIOParams{Attributes: attrs}
		input, _, _, _, _ := extractInputOutput(params)

		// Input should be the system instructions
		assert.Contains(t, input, "System prompt only")
	})

	t.Run("input messages only (no system instructions)", func(t *testing.T) {
		attrs := map[string]any{
			AttrGenAIInputMessages: []any{
				map[string]any{"role": "user", "content": "User message only"},
			},
		}

		params := ExtractIOParams{Attributes: attrs}
		input, _, _, _, _ := extractInputOutput(params)

		assert.Contains(t, input, "User message only")
	})
}

// TestFilterIOKeysFromMetadata_SystemInstructions verifies system instructions are filtered
func TestFilterIOKeysFromMetadata_SystemInstructions(t *testing.T) {
	attrs := map[string]any{
		AttrGenAISystemInstructions: `[{"role":"system","content":"secret system prompt"}]`,
		AttrGenAIInputMessages:      `[{"role":"user","content":"message"}]`,
		AttrGenAIOutputMessages:     `[{"role":"assistant","content":"response"}]`,
		"gen_ai.provider.name":      "openai",
		"gen_ai.request.model":      "gpt-4",
	}

	filtered := filterIOKeysFromMetadata(attrs)

	// All I/O keys should be filtered (including system instructions)
	assert.NotContains(t, filtered, AttrGenAISystemInstructions)
	assert.NotContains(t, filtered, AttrGenAIInputMessages)
	assert.NotContains(t, filtered, AttrGenAIOutputMessages)

	// Non-I/O keys should remain
	assert.Contains(t, filtered, "gen_ai.provider.name")
	assert.Contains(t, filtered, "gen_ai.request.model")
}
