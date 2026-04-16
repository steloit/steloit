package middleware

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testUUID is a stable UUIDv7 used across tests for equality checks.
var testUUID = uuid.MustParse("01909f20-0000-7000-8000-000000000001")

// expectPanic runs fn and fails the test unless it panics.
func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got nil")
		}
	}()
	fn()
}

func TestGetUserIDFromContext(t *testing.T) {
	tests := []struct {
		name     string
		setKey   bool
		setValue any
		wantID   uuid.UUID
		wantOK   bool
	}{
		{"valid uuid returns id and true", true, testUUID, testUUID, true},
		{"missing key returns zero uuid and false", false, nil, uuid.UUID{}, false},
		{"wrong type returns zero uuid and false", true, "01909f20-0000-7000-8000-000000000001", uuid.UUID{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			if tc.setKey {
				c.Set(UserIDKey, tc.setValue)
			}
			gotID, gotOK := GetUserIDFromContext(c)
			if gotID != tc.wantID || gotOK != tc.wantOK {
				t.Errorf("got (%v, %v), want (%v, %v)", gotID, gotOK, tc.wantID, tc.wantOK)
			}
		})
	}
}

func TestMustGetUserID(t *testing.T) {
	t.Run("returns id when present", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(UserIDKey, testUUID)
		if got := MustGetUserID(c); got != testUUID {
			t.Errorf("got %v, want %v", got, testUUID)
		}
	})

	t.Run("panics when key missing", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		expectPanic(t, func() { _ = MustGetUserID(c) })
	})

	t.Run("panics when value has wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(UserIDKey, "not-a-uuid")
		expectPanic(t, func() { _ = MustGetUserID(c) })
	})
}

func TestGetAuthContext(t *testing.T) {
	t.Run("returns context when present", func(t *testing.T) {
		want := &auth.AuthContext{UserID: testUUID}
		c, _ := gin.CreateTestContext(nil)
		c.Set(AuthContextKey, want)
		got, ok := GetAuthContext(c)
		if !ok || got != want {
			t.Errorf("got (%v, %v), want (%v, true)", got, ok, want)
		}
	})

	t.Run("missing key returns nil and false", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		got, ok := GetAuthContext(c)
		if ok || got != nil {
			t.Errorf("got (%v, %v), want (nil, false)", got, ok)
		}
	})
}

func TestMustGetAuthContext(t *testing.T) {
	t.Run("returns context when present", func(t *testing.T) {
		want := &auth.AuthContext{UserID: testUUID}
		c, _ := gin.CreateTestContext(nil)
		c.Set(AuthContextKey, want)
		if got := MustGetAuthContext(c); got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("panics when missing", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		expectPanic(t, func() { _ = MustGetAuthContext(c) })
	})
}

func TestGetTokenClaims(t *testing.T) {
	t.Run("returns claims when present", func(t *testing.T) {
		want := &auth.JWTClaims{UserID: testUUID, JWTID: "jti-123"}
		c, _ := gin.CreateTestContext(nil)
		c.Set(TokenClaimsKey, want)
		got, ok := GetTokenClaims(c)
		if !ok || got != want {
			t.Errorf("got (%v, %v), want (%v, true)", got, ok, want)
		}
	})

	t.Run("missing key returns nil and false", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		got, ok := GetTokenClaims(c)
		if ok || got != nil {
			t.Errorf("got (%v, %v), want (nil, false)", got, ok)
		}
	})
}

func TestMustGetTokenClaims(t *testing.T) {
	t.Run("returns claims when present", func(t *testing.T) {
		want := &auth.JWTClaims{UserID: testUUID, JWTID: "jti-123"}
		c, _ := gin.CreateTestContext(nil)
		c.Set(TokenClaimsKey, want)
		if got := MustGetTokenClaims(c); got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("panics when missing", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		expectPanic(t, func() { _ = MustGetTokenClaims(c) })
	})
}
