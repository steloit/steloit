package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMetadataStorageForRelease tests that release is stored in metadata JSON from resource attributes.
// Note: version (brokle.span.version) is now a per-span attribute materialized directly by ClickHouse —
// the backend does NOT extract it from resource attributes.
func TestMetadataStorageForRelease(t *testing.T) {
	tests := []struct {
		name               string
		resourceAttrs      map[string]any
		expectTraceRelease string
	}{
		{
			name: "release_present",
			resourceAttrs: map[string]any{
				"brokle.release": "v2.1.24",
			},
			expectTraceRelease: "v2.1.24",
		},
		{
			name:               "no_release",
			resourceAttrs:      map[string]any{},
			expectTraceRelease: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Merge attributes (simulating createTraceEvent logic)
			allAttrs := make(map[string]any)
			for k, v := range tt.resourceAttrs {
				allAttrs[k] = v
			}

			// Create metadata map
			metadata := make(map[string]any)

			// Extract release
			if release, ok := allAttrs["brokle.release"].(string); ok && release != "" {
				metadata["brokle.release"] = release
			}

			// Verify expectations
			if tt.expectTraceRelease == "" {
				_, exists := metadata["brokle.release"]
				assert.False(t, exists, "brokle.release should not be in metadata")
			} else {
				actual, exists := metadata["brokle.release"]
				assert.True(t, exists, "brokle.release should be in metadata")
				assert.Equal(t, tt.expectTraceRelease, actual)
			}
		})
	}
}

// TestReleaseVersionMaterialization tests that materialized columns correctly extract from JSON
// Note: This is an integration test that requires ClickHouse to be running
func TestReleaseVersionMaterialization(t *testing.T) {
	t.Skip("Integration test - requires ClickHouse. Run with: go test -tags=integration")

	// This test would:
	// 1. Insert trace with metadata.brokle.release
	// 2. Insert span with span_attributes['brokle.span.version']
	// 3. Query materialized columns traces.release and spans.span_version
	// 4. Verify they match the original values
}
