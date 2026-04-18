package organization

import (
	"time"

	"github.com/google/uuid"
)

// Package-local conversion helpers used at the gen ↔ domain boundary.
// Nil-coalescing for fields the domain encodes as zero values while
// sqlc-generated row types use pointers.

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func emptyToNilString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefUUID(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return *p
}

func derefTime(p *time.Time) time.Time {
	if p == nil {
		return time.Time{}
	}
	return *p
}
