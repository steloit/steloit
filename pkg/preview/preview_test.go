package preview

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGeneratePreview_EmptyContent tests preview generation for empty content
func TestGeneratePreview_EmptyContent(t *testing.T) {
	result := GeneratePreview("")
	if result != "" {
		t.Errorf("Expected empty string for empty content, got: %s", result)
	}
}

// TestGeneratePreview_JSONContent tests preview generation for JSON content
func TestGeneratePreview_JSONContent(t *testing.T) {
	jsonContent := `{
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "What is the weather today?"}
		],
		"model": "gpt-4",
		"temperature": 0.7,
		"max_tokens": 1000
	}`

	result := GeneratePreview(jsonContent)

	// Check header format
	if !strings.HasPrefix(result, "[json - ") {
		t.Errorf("Expected JSON header, got: %s", result)
	}

	// Check structure line exists
	if !strings.Contains(result, "Structure:") {
		t.Errorf("Expected Structure line in JSON preview, got: %s", result)
	}

	// Check content is included
	if !strings.Contains(result, "messages") {
		t.Errorf("Expected JSON content in preview, got: %s", result)
	}

	// Verify it's valid JSON (at least the source)
	var data any
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		t.Errorf("Source JSON is invalid: %v", err)
	}
}

// TestGeneratePreview_TextContent tests preview generation for plain text
func TestGeneratePreview_TextContent(t *testing.T) {
	textContent := "This is a sample text document. It contains multiple sentences. Each sentence should be preserved. The preview should truncate at sentence boundaries when possible."

	result := GeneratePreview(textContent)

	// Check header format
	if !strings.HasPrefix(result, "[text - ") {
		t.Errorf("Expected text header, got: %s", result)
	}

	// Check content is included
	if !strings.Contains(result, "sample text") {
		t.Errorf("Expected text content in preview, got: %s", result)
	}

	// For short content, should not truncate
	if strings.Contains(result, "...") && len(textContent) < PREVIEW_CHARS_DEFAULT {
		t.Errorf("Short text should not be truncated, got: %s", result)
	}
}

// TestGeneratePreview_LongTextContent tests sentence-boundary truncation
func TestGeneratePreview_LongTextContent(t *testing.T) {
	// Create long text content (>500 chars)
	longText := strings.Repeat("This is a sentence. ", 50) // ~1000 chars

	result := GeneratePreview(longText)

	// Extract preview body (after header)
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Fatal("Preview should have header and body")
	}
	previewBody := strings.Join(lines[1:], "\n")

	// Should be truncated
	if len(previewBody) > PREVIEW_CHARS_DEFAULT+10 { // Small tolerance for "..."
		t.Errorf("Long text should be truncated to ~%d chars, got %d", PREVIEW_CHARS_DEFAULT, len(previewBody))
	}

	// Should end with sentence or "..."
	if !strings.HasSuffix(previewBody, ".") && !strings.HasSuffix(previewBody, "...") {
		t.Errorf("Truncated text should end with period or ellipsis, got: %s", previewBody[len(previewBody)-10:])
	}
}

// TestGeneratePreview_MarkdownContent tests Markdown preview generation
func TestGeneratePreview_MarkdownContent(t *testing.T) {
	markdownContent := `# Main Heading

This is the first paragraph with some content.

## Subheading

This is another paragraph with more details.

- List item 1
- List item 2

` + "```go\nfunc example() {}\n```"

	result := GeneratePreview(markdownContent)

	// Check header format
	if !strings.HasPrefix(result, "[markdown - ") {
		t.Errorf("Expected markdown header, got: %s", result)
	}

	// Check that main heading is preserved
	if !strings.Contains(result, "# Main Heading") {
		t.Errorf("Expected main heading in preview, got: %s", result)
	}

	// Check some content is included
	if !strings.Contains(result, "first paragraph") {
		t.Errorf("Expected paragraph content in preview, got: %s", result)
	}
}

// TestGeneratePreview_ErrorContent tests error/stack trace preview
func TestGeneratePreview_ErrorContent(t *testing.T) {
	errorContent := `Error: Something went wrong
    at Object.<anonymous> (/app/index.js:10:15)
    at Module._compile (internal/modules/cjs/loader.js:1063:30)
    at Object.Module._extensions..js (internal/modules/cjs/loader.js:1092:10)
    at Module.load (internal/modules/cjs/loader.js:928:32)
    at Function.Module._load (internal/modules/cjs/loader.js:769:14)`

	result := GeneratePreview(errorContent)

	// Check header format
	if !strings.HasPrefix(result, "[error - ") {
		t.Errorf("Expected error header, got: %s", result)
	}

	// Check that error message is preserved
	if !strings.Contains(result, "Error: Something went wrong") {
		t.Errorf("Expected error message in preview, got: %s", result)
	}

	// Check that stack frames are included
	if !strings.Contains(result, "at Object.<anonymous>") {
		t.Errorf("Expected stack frames in preview, got: %s", result)
	}
}

// TestGeneratePreview_LargeJSONContent tests JSON preview with size limits
func TestGeneratePreview_LargeJSONContent(t *testing.T) {
	// Create large JSON (>10KB)
	largeArray := make([]map[string]string, 0, 1000)
	for i := range 1000 {
		largeArray = append(largeArray, map[string]string{
			"id":          string(rune(i)),
			"name":        "Item " + string(rune(i)),
			"description": "This is a sample description for testing purposes.",
		})
	}
	largeJSON, _ := json.Marshal(map[string]any{
		"items": largeArray,
		"total": 1000,
	})

	result := GeneratePreview(string(largeJSON))

	// Check header includes size
	if !strings.Contains(result, "KB") && !strings.Contains(result, "MB") {
		t.Errorf("Expected size in KB/MB for large JSON, got: %s", strings.Split(result, "\n")[0])
	}

	// Extract preview body
	lines := strings.Split(result, "\n")
	if len(lines) < 3 { // Header + Structure + Content
		t.Fatal("Preview should have header, structure, and content lines")
	}

	// Combined preview should not exceed max chars significantly
	previewBody := strings.Join(lines[1:], "\n")
	if len(previewBody) > PREVIEW_CHARS_MAX+100 { // Generous tolerance
		t.Errorf("Large JSON preview should be truncated to ~%d chars, got %d", PREVIEW_CHARS_MAX, len(previewBody))
	}
}

// TestFormatBytes tests human-readable byte formatting
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		expected string
		bytes    int
	}{
		{"100 bytes", 100},
		{"1.0 KB", 1024},
		{"1.5 KB", 1536},
		{"1.0 MB", 1048576},
		{"1.5 MB", 1572864},
		{"1.0 GB", 1073741824},
		{"2.0 GB", 2147483648},
	}

	for _, test := range tests {
		result := formatBytes(test.bytes)
		if result != test.expected {
			t.Errorf("formatBytes(%d) = %s, expected %s", test.bytes, result, test.expected)
		}
	}
}

// TestDetectContentType tests content type detection
func TestDetectContentType(t *testing.T) {
	tests := []struct {
		content      string
		expectedType ContentType
	}{
		{`{"key": "value"}`, ContentTypeJSON},
		{`[1, 2, 3]`, ContentTypeJSON},
		{"Error: something failed", ContentTypeError},
		{"Traceback (most recent call last):", ContentTypeError},
		{"# Heading\n\nParagraph", ContentTypeMarkdown},
		{"* List item", ContentTypeMarkdown},
		{"Plain text content", ContentTypeText},
		{"", ContentTypeUnknown},
	}

	for _, test := range tests {
		result := detectContentType(test.content)
		if result != test.expectedType {
			t.Errorf("detectContentType(%q) = %v, expected %v", test.content, result, test.expectedType)
		}
	}
}

// TestTruncateAtWordBoundary tests word boundary truncation
func TestTruncateAtWordBoundary(t *testing.T) {
	content := "This is a long sentence that needs to be truncated at a word boundary."
	maxChars := 30

	result := truncateAtWordBoundary(content, maxChars)

	// Should be truncated
	if len(result) > maxChars+10 { // Tolerance for "..."
		t.Errorf("Expected truncation to ~%d chars, got %d", maxChars, len(result))
	}

	// Should end with "..."
	if !strings.HasSuffix(result, "...") {
		t.Errorf("Expected ellipsis at end, got: %s", result)
	}

	// Should not break mid-word (check for space before ...)
	withoutEllipsis := strings.TrimSuffix(result, "...")
	if len(withoutEllipsis) > 0 && withoutEllipsis[len(withoutEllipsis)-1] != ' ' {
		lastWord := withoutEllipsis[strings.LastIndex(withoutEllipsis, " ")+1:]
		// Check if last word is complete (appears in original)
		if !strings.Contains(content, lastWord+" ") && !strings.HasSuffix(content, lastWord) {
			t.Errorf("Truncation appears to break mid-word: %s", result)
		}
	}
}

// TestAnalyzeJSONStructure tests JSON structure analysis
func TestAnalyzeJSONStructure(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		expected string
	}{
		{
			name:     "Simple object",
			data:     map[string]any{"name": "John", "age": 30.0}, // JSON unmarshal converts to float64
			expected: "{name:string, age:number}",
		},
		{
			name:     "Empty array",
			data:     []any{},
			expected: "[]",
		},
		{
			name:     "Array of strings",
			data:     []any{"a", "b", "c"},
			expected: "[string (length:3)]",
		},
		{
			name:     "Simple string",
			data:     "hello",
			expected: "string",
		},
		{
			name:     "Number",
			data:     42.5,
			expected: "number",
		},
		{
			name:     "Boolean",
			data:     true,
			expected: "boolean",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := analyzeJSONStructure(test.data)
			// For objects, check that both keys are present (order may vary)
			if strings.HasPrefix(test.expected, "{") && strings.Contains(test.expected, ",") {
				// Check each key-value pair is present
				for _, part := range []string{"name:string", "age:number"} {
					if !strings.Contains(result, part) {
						t.Errorf("analyzeJSONStructure() = %s, should contain %s", result, part)
					}
				}
			} else if result != test.expected {
				t.Errorf("analyzeJSONStructure() = %s, expected %s", result, test.expected)
			}
		})
	}
}

// TestGeneratePreview_AdaptiveSizing tests that different content types get appropriate sizes
func TestGeneratePreview_AdaptiveSizing(t *testing.T) {
	// Create content of different types with realistic structure
	jsonContent := `{
		"messages": [` + strings.Repeat(`{"role": "user", "content": "test"},`, 100) + `],
		"model": "gpt-4",
		"temperature": 0.7
	}`

	textContent := strings.Repeat("This is a sample sentence. ", 100)

	errorContent := "Error: Something went wrong\n" +
		strings.Repeat("    at Object.<anonymous> (/app/index.js:10:15)\n", 30)

	jsonPreview := GeneratePreview(jsonContent)
	textPreview := GeneratePreview(textContent)
	errorPreview := GeneratePreview(errorContent)

	// Extract body lengths (excluding header)
	getBodyLength := func(preview string) int {
		lines := strings.Split(preview, "\n")
		if len(lines) < 2 {
			return 0
		}
		return len(strings.Join(lines[1:], "\n"))
	}

	jsonBodyLen := getBodyLength(jsonPreview)
	textBodyLen := getBodyLength(textPreview)
	errorBodyLen := getBodyLength(errorPreview)

	// Log the actual lengths for debugging
	t.Logf("JSON preview length: %d", jsonBodyLen)
	t.Logf("Text preview length: %d", textBodyLen)
	t.Logf("Error preview length: %d", errorBodyLen)

	// JSON should have structure line + content, making it potentially longer
	if jsonBodyLen <= PREVIEW_CHARS_MIN {
		t.Errorf("JSON preview (%d) should be at least %d chars", jsonBodyLen, PREVIEW_CHARS_MIN)
	}

	// Text should be around default size
	if textBodyLen > PREVIEW_CHARS_DEFAULT+50 {
		t.Errorf("Text preview (%d) should be around %d chars", textBodyLen, PREVIEW_CHARS_DEFAULT)
	}

	// Error should allow extended preview
	if len(errorContent) > PREVIEW_CHARS_MAX && errorBodyLen < PREVIEW_CHARS_DEFAULT {
		t.Errorf("Error preview (%d) should use extended size for long errors", errorBodyLen)
	}
}

// TestGeneratePreview_FallbackOnError tests graceful fallback for invalid JSON
func TestGeneratePreview_FallbackOnError(t *testing.T) {
	// Invalid JSON that looks like JSON but isn't
	invalidJSON := `{"key": "value", "unclosed": `

	result := GeneratePreview(invalidJSON)

	// Should still generate a preview (fallback to text or unknown)
	if result == "" {
		t.Error("Expected fallback preview for invalid JSON, got empty string")
	}

	// Should have some header
	lines := strings.Split(result, "\n")
	if len(lines) < 1 {
		t.Error("Expected at least header line in fallback preview")
	}

	// Should contain the content
	if !strings.Contains(result, "key") {
		t.Error("Expected content to be included in fallback preview")
	}
}

// BenchmarkGeneratePreview_SmallJSON benchmarks preview generation for small JSON
func BenchmarkGeneratePreview_SmallJSON(b *testing.B) {
	content := `{"model": "gpt-4", "temperature": 0.7, "messages": [{"role": "user", "content": "Hello"}]}`
	b.ResetTimer()
	for range b.N {
		_ = GeneratePreview(content)
	}
}

// BenchmarkGeneratePreview_LargeJSON benchmarks preview generation for large JSON
func BenchmarkGeneratePreview_LargeJSON(b *testing.B) {
	largeArray := make([]map[string]string, 0, 100)
	for i := range 100 {
		largeArray = append(largeArray, map[string]string{
			"id":   string(rune(i)),
			"data": strings.Repeat("x", 100),
		})
	}
	content, _ := json.Marshal(map[string]any{"items": largeArray})
	b.ResetTimer()
	for range b.N {
		_ = GeneratePreview(string(content))
	}
}

// BenchmarkGeneratePreview_Text benchmarks preview generation for text
func BenchmarkGeneratePreview_Text(b *testing.B) {
	content := strings.Repeat("This is a sentence. ", 100)
	b.ResetTimer()
	for range b.N {
		_ = GeneratePreview(content)
	}
}
