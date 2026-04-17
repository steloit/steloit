package organization

import (
	"time"

	"github.com/google/uuid"
)

// Package-local conversion helpers used at the gen ↔ domain boundary.
// Domain entities use *time.Time for soft-delete (post-gorm strip);
// sqlc-generated row types also use *time.Time. These helpers survive
// only for the nil-coalescing cases (empty strings, nil UUIDs) that
// the domain still encodes as zero values instead of pointers.

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
