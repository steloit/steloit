package uid

import (
	"time"

	"github.com/google/uuid"
)

// New generates a new UUIDv7 with the current timestamp.
func New() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

// TimeFromID extracts the creation timestamp from a UUIDv7.
// Returns zero time for non-v7 UUIDs.
func TimeFromID(id uuid.UUID) time.Time {
	if id.Version() != 7 {
		return time.Time{}
	}
	sec, nsec := id.Time().UnixTime()
	return time.Unix(sec, nsec)
}
