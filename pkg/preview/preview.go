package preview

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Preview size constants (adaptive based on content type)
const (
	PREVIEW_CHARS_MIN     = 300 // Minimum preview size (binary/unknown)
	PREVIEW_CHARS_DEFAULT = 500 // Default preview size (text, markdown)
	PREVIEW_CHARS_MAX     = 800 // Maximum preview size (JSON, errors)
)

// ContentType represents the detected content type
type ContentType string

const (
	ContentTypeJSON     ContentType = "json"
	ContentTypeText     ContentType = "text"
	ContentTypeMarkdown ContentType = "markdown"
	ContentTypeError    ContentType = "error"
	ContentTypeUnknown  ContentType = "unknown"
)

// GeneratePreview generates a type-aware preview of content with adaptive sizing
// Returns format: [{type} - {size}]\n{content_preview}
//
// Size Guide:
//   - JSON: Up to 800 chars (structure analysis + content)
//   - Text: 500 chars (sentence-boundary aware)
//   - Markdown: 500 chars (header + paragraph extraction)
//   - Errors: 800 chars (extended for stack traces)
//   - Unknown: 300 chars (simple truncation)
//
// Fallback: On any error, returns simple truncation at word boundary
func GeneratePreview(content string) string {
	if content == "" {
		return ""
	}

	// Detect content type
	contentType := detectContentType(content)

	// Generate header with type and size
	header := fmt.Sprintf("[%s - %s]\n", contentType, formatBytes(len(content)))

	// Route to type-specific preview generator
	var preview string
	var err error

	switch contentType {
	case ContentTypeJSON:
		preview, err = generateJSONPreview(content, PREVIEW_CHARS_MAX)
	case ContentTypeMarkdown:
		preview, err = generateMarkdownPreview(content, PREVIEW_CHARS_DEFAULT)
	case ContentTypeError:
		preview, err = generateErrorPreview(content, PREVIEW_CHARS_MAX)
	case ContentTypeText:
		preview, err = generateTextPreview(content, PREVIEW_CHARS_DEFAULT)
	default:
		preview, err = generateFallbackPreview(content, PREVIEW_CHARS_MIN)
	}

	// Fallback to simple truncation if type-specific formatter fails
	if err != nil || preview == "" {
		preview, _ = generateFallbackPreview(content, PREVIEW_CHARS_DEFAULT)
	}

	return header + preview
}

// detectContentType analyzes content and detects its type
func detectContentType(content string) ContentType {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ContentTypeUnknown
	}

	// Check for JSON (starts with { or [)
	if (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
		return ContentTypeJSON
	}

	// Check for error/stack trace patterns
	if isErrorContent(trimmed) {
		return ContentTypeError
	}

	// Check for Markdown patterns (headers, lists, code blocks)
	if isMarkdownContent(trimmed) {
		return ContentTypeMarkdown
	}

	// Check for text content (printable characters, sentences)
	if isTextContent(trimmed) {
		return ContentTypeText
	}

	return ContentTypeUnknown
}

// isErrorContent checks if content looks like an error or stack trace
func isErrorContent(content string) bool {
	errorPatterns := []string{
		"Error:",
		"Exception:",
		"Traceback",
		"at \\S+\\.\\S+\\(",       // Stack frame pattern
		"\\w+Error: ",             // Python/JS errors
		"java\\.\\w+\\.\\w+Error", // Java errors
	}

	for _, pattern := range errorPatterns {
		if matched, _ := regexp.MatchString(pattern, content); matched {
			return true
		}
	}
	return false
}

// isMarkdownContent checks if content has Markdown patterns
func isMarkdownContent(content string) bool {
	markdownPatterns := []string{
		"^#{1,6} ",         // Headers
		"^\\* ",            // Unordered lists
		"^\\d+\\. ",        // Ordered lists
		"```",              // Code blocks
		"\\[.*\\]\\(.*\\)", // Links
	}

	for _, pattern := range markdownPatterns {
		if matched, _ := regexp.MatchString(pattern, content); matched {
			return true
		}
	}
	return false
}

// isTextContent checks if content is primarily text
func isTextContent(content string) bool {
	// Count printable characters
	printable := 0
	total := 0
	for _, r := range content {
		total++
		if unicode.IsPrint(r) || unicode.IsSpace(r) {
			printable++
		}
	}

	// At least 95% printable characters
	return float64(printable)/float64(total) >= 0.95
}

// generateJSONPreview generates a preview for JSON content with structure analysis
func generateJSONPreview(content string, maxChars int) (string, error) {
	// Parse JSON to analyze structure
	var data any
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return "", err
	}

	// Analyze structure
	structure := analyzeJSONStructure(data)
	structureLine := "Structure: " + structure + "\n"

	// Calculate remaining space for content
	remainingChars := maxChars - len(structureLine)
	if remainingChars < 100 {
		remainingChars = 100 // Minimum content preview
	}

	// Truncate JSON content at valid position
	contentPreview := content
	if len(content) > remainingChars {
		contentPreview = content[:remainingChars]

		// Try to end at a valid JSON boundary (after }, ], ")
		lastValid := strings.LastIndexAny(contentPreview, "}]\",")
		if lastValid > remainingChars/2 {
			contentPreview = contentPreview[:lastValid+1]
		}
		contentPreview += "..."
	}

	return structureLine + contentPreview, nil
}

// analyzeJSONStructure analyzes JSON structure and returns a concise description
func analyzeJSONStructure(data any) string {
	switch v := data.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key, val := range v {
			valueType := getJSONValueType(val)
			keys = append(keys, fmt.Sprintf("%s:%s", key, valueType))
			if len(keys) >= 8 { // Limit to first 8 keys
				keys = append(keys, "...")
				break
			}
		}
		return "{" + strings.Join(keys, ", ") + "}"
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		// Analyze first element to infer array type
		firstType := getJSONValueType(v[0])
		return fmt.Sprintf("[%s (length:%d)]", firstType, len(v))
	default:
		return getJSONValueType(data)
	}
}

// getJSONValueType returns the type of a JSON value
func getJSONValueType(val any) string {
	switch v := val.(type) {
	case map[string]any:
		return "object"
	case []any:
		if len(v) == 0 {
			return "array"
		}
		return "array<" + getJSONValueType(v[0]) + ">"
	case string:
		return "string"
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

// generateTextPreview generates a sentence-aware preview for text content
func generateTextPreview(content string, maxChars int) (string, error) {
	if len(content) <= maxChars {
		return content, nil
	}

	// Try to truncate at sentence boundary
	truncated := content[:maxChars]

	// Look for sentence endings (. ! ?)
	sentenceEndings := []string{". ", ".\n", "! ", "!\n", "? ", "?\n"}
	lastSentenceEnd := -1
	for _, ending := range sentenceEndings {
		if idx := strings.LastIndex(truncated, ending); idx > maxChars/2 && idx > lastSentenceEnd {
			lastSentenceEnd = idx + len(ending) - 1
		}
	}

	if lastSentenceEnd > 0 {
		return content[:lastSentenceEnd] + "...", nil
	}

	// Fallback to word boundary
	return truncateAtWordBoundary(content, maxChars), nil
}

// generateMarkdownPreview generates a preview for Markdown content
func generateMarkdownPreview(content string, maxChars int) (string, error) {
	if len(content) <= maxChars {
		return content, nil
	}

	lines := strings.Split(content, "\n")
	var preview strings.Builder
	headerSeen := false

	for _, line := range lines {
		// Include first header if found
		if strings.HasPrefix(line, "#") && !headerSeen {
			preview.WriteString(line)
			preview.WriteString("\n")
			headerSeen = true
			continue
		}

		// Stop if we've exceeded the limit
		if preview.Len()+len(line) > maxChars {
			// Add partial line if there's room
			remaining := maxChars - preview.Len()
			if remaining > 20 {
				preview.WriteString(truncateAtWordBoundary(line, remaining))
			}
			preview.WriteString("...")
			break
		}

		preview.WriteString(line)
		preview.WriteString("\n")
	}

	return preview.String(), nil
}

// generateErrorPreview generates an extended preview for error/stack trace content
func generateErrorPreview(content string, maxChars int) (string, error) {
	if len(content) <= maxChars {
		return content, nil
	}

	// Try to capture first few stack frames
	lines := strings.Split(content, "\n")
	var preview strings.Builder
	frameCount := 0
	maxFrames := 10 // Capture first 10 stack frames

	for _, line := range lines {
		if preview.Len()+len(line) > maxChars {
			preview.WriteString("...")
			break
		}

		preview.WriteString(line)
		preview.WriteString("\n")

		// Count stack frames
		if strings.Contains(line, "at ") || strings.Contains(line, "File ") {
			frameCount++
			if frameCount >= maxFrames {
				preview.WriteString("...[additional frames omitted]")
				break
			}
		}
	}

	return preview.String(), nil
}

// generateFallbackPreview generates a simple truncated preview
func generateFallbackPreview(content string, maxChars int) (string, error) {
	if len(content) <= maxChars {
		return content, nil
	}

	return truncateAtWordBoundary(content, maxChars), nil
}

// truncateAtWordBoundary truncates content at a word boundary
func truncateAtWordBoundary(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}

	truncated := content[:maxChars]

	// Find last space
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxChars/2 {
		return content[:lastSpace] + "..."
	}

	return truncated + "..."
}

// formatBytes formats byte count to human-readable size
func formatBytes(bytes int) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
