package utils

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const uuidLen = 36

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateCompositeSlug creates a URL-friendly slug from name and ID
// Format: "{name-slug}-{uuid}"
// Example: "acme-corp-018f6b6a-1234-7abc-8def-0123456789ab"
func GenerateCompositeSlug(name string, id uuid.UUID) string {
	slug := strings.ToLower(name)
	slug = slugRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	return slug + "-" + id.String()
}

// ExtractIDFromCompositeSlug extracts UUID from composite slug
// Input: "acme-corp-018f6b6a-1234-7abc-8def-0123456789ab"
// Output: uuid.UUID
func ExtractIDFromCompositeSlug(compositeSlug string) (uuid.UUID, error) {
	if len(compositeSlug) < uuidLen {
		return uuid.UUID{}, errors.New("invalid composite slug: too short")
	}

	idStr := compositeSlug[len(compositeSlug)-uuidLen:]
	return uuid.Parse(idStr)
}
