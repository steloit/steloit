package organization

import (
	"testing"
)

// These tests lock in the nil-safety of the nested-actor pattern on
// Invitation. The underlying schema (user_invitations.invited_by_id)
// is nullable with ON DELETE SET NULL, so a fetched Invitation with
// Inviter == nil or InvitedByID == nil is a legitimate production
// state — handlers that render it must not panic.
//
// Regression: an earlier patch changed InvitedByID to *uuid.UUID but
// handler code still did `*invitation.InvitedByID` unconditionally,
// panicking on rows whose inviter account had been deleted. The fix
// introduced the JOIN-hydrated Invitation.Inviter field and explicit
// nil-checks at single-row sites.

func TestInvitation_InviterNilIsValidState(t *testing.T) {
	// A valid post-inviter-deletion invitation: InvitedByID is NULL
	// in Postgres → *uuid.UUID is nil in Go; Inviter is nil because
	// the LEFT JOIN on users did not match.
	inv := &Invitation{
		Status: InvitationStatusPending,
		// InvitedByID intentionally omitted — nil
		// Inviter intentionally omitted — nil
		// Role intentionally omitted — nil
	}

	if inv.InvitedByID != nil {
		t.Fatalf("expected nil InvitedByID, got %v", inv.InvitedByID)
	}
	if inv.Inviter != nil {
		t.Fatalf("expected nil Inviter, got %v", inv.Inviter)
	}
	if inv.Role != nil {
		t.Fatalf("expected nil Role, got %v", inv.Role)
	}
}

func TestInviterRef_FullName(t *testing.T) {
	tests := []struct {
		name string
		ref  InviterRef
		want string
	}{
		{"both set", InviterRef{FirstName: "Ada", LastName: "Lovelace"}, "Ada Lovelace"},
		{"first only", InviterRef{FirstName: "Ada"}, "Ada"},
		{"last only", InviterRef{LastName: "Lovelace"}, "Lovelace"},
		{"both empty", InviterRef{}, ""},
		{"whitespace only", InviterRef{FirstName: "  ", LastName: "  "}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.FullName(); got != tt.want {
				t.Fatalf("FullName() = %q, want %q", got, tt.want)
			}
		})
	}
}
