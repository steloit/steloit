package utils

import (
	"testing"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

func TestGenerateCompositeSlug(t *testing.T) {
	id := uid.New()
	tests := []struct {
		name     string
		orgName  string
		expected string
		id       uuid.UUID
	}{
		{
			name:     "simple name",
			orgName:  "Acme Corp",
			id:       id,
			expected: "acme-corp-" + id.String(),
		},
		{
			name:     "name with special characters",
			orgName:  "Acme & Co. Inc!",
			id:       id,
			expected: "acme-co-inc-" + id.String(),
		},
		{
			name:     "name with multiple spaces",
			orgName:  "The   Big   Company",
			id:       id,
			expected: "the-big-company-" + id.String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateCompositeSlug(tt.orgName, tt.id)
			if result != tt.expected {
				t.Errorf("GenerateCompositeSlug() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractIDFromCompositeSlug(t *testing.T) {
	id := uid.New()
	slug := GenerateCompositeSlug("Acme Corp", id)

	tests := []struct {
		name          string
		compositeSlug string
		expectedID    uuid.UUID
		wantErr       bool
	}{
		{
			name:          "valid composite slug",
			compositeSlug: slug,
			expectedID:    id,
			wantErr:       false,
		},
		{
			name:          "too short",
			compositeSlug: "short",
			wantErr:       true,
		},
		{
			name:          "just uuid",
			compositeSlug: id.String(),
			expectedID:    id,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractIDFromCompositeSlug(tt.compositeSlug)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractIDFromCompositeSlug() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expectedID {
				t.Errorf("ExtractIDFromCompositeSlug() = %v, want %v", result, tt.expectedID)
			}
		})
	}
}

func TestSlugRoundTrip(t *testing.T) {
	id := uid.New()
	slug := GenerateCompositeSlug("My Project", id)

	extracted, err := ExtractIDFromCompositeSlug(slug)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if extracted != id {
		t.Fatalf("round-trip mismatch: got %s, want %s", extracted, id)
	}
}
