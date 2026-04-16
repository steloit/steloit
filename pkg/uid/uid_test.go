package uid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNew(t *testing.T) {
	id := New()

	if id == uuid.Nil {
		t.Fatal("New() returned nil UUID")
	}
	if id.Version() != 7 {
		t.Fatalf("New() returned version %d, want 7", id.Version())
	}
}

func TestNew_Sequential(t *testing.T) {
	id1 := New()
	time.Sleep(time.Millisecond)
	id2 := New()

	if id1.String() >= id2.String() {
		t.Fatalf("sequential UUIDv7s should be ordered: %s >= %s", id1, id2)
	}
}

func TestTimeFromID(t *testing.T) {
	before := time.Now()
	id := New()
	after := time.Now()

	extracted := TimeFromID(id)
	if extracted.IsZero() {
		t.Fatal("TimeFromID returned zero time for UUIDv7")
	}
	if extracted.Before(before.Truncate(time.Millisecond)) {
		t.Fatalf("extracted time %v is before creation time %v", extracted, before)
	}
	if extracted.After(after.Add(time.Millisecond)) {
		t.Fatalf("extracted time %v is after creation time %v", extracted, after)
	}
}

func TestTimeFromID_NonV7(t *testing.T) {
	v4 := uuid.New() // UUIDv4
	extracted := TimeFromID(v4)
	if !extracted.IsZero() {
		t.Fatal("TimeFromID should return zero time for non-v7 UUID")
	}
}

func TestUUID_ScanValue_RoundTrip(t *testing.T) {
	original := New()

	val, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	str, ok := val.(string)
	if !ok {
		t.Fatalf("Value() returned %T, want string", val)
	}

	var scanned uuid.UUID
	if err := scanned.Scan(str); err != nil {
		t.Fatalf("Scan(string) error: %v", err)
	}
	if scanned != original {
		t.Fatalf("Scan round-trip failed: got %s, want %s", scanned, original)
	}
}

func TestUUID_JSON_RoundTrip(t *testing.T) {
	type entity struct {
		ID   uuid.UUID  `json:"id"`
		Name string     `json:"name"`
		Ref  *uuid.UUID `json:"ref,omitempty"`
	}

	id := New()
	ref := New()
	original := entity{ID: id, Name: "test", Ref: &ref}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded entity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if decoded.ID != original.ID {
		t.Fatalf("JSON round-trip ID: got %s, want %s", decoded.ID, original.ID)
	}
	if decoded.Ref == nil || *decoded.Ref != *original.Ref {
		t.Fatal("JSON round-trip Ref failed")
	}
}
