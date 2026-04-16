package middleware

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
)

var (
	testProjectID = uuid.MustParse("01909f20-0000-7000-8000-000000000010")
	testOrgID     = uuid.MustParse("01909f20-0000-7000-8000-000000000020")
	testAPIKeyID  = uuid.MustParse("01909f20-0000-7000-8000-000000000030")
)

func TestGetProjectID(t *testing.T) {
	t.Run("returns id when present", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(ProjectIDKey, testProjectID)
		got, ok := GetProjectID(c)
		if !ok || got != testProjectID {
			t.Errorf("got (%v, %v), want (%v, true)", got, ok, testProjectID)
		}
	})

	t.Run("missing key returns zero uuid and false", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		got, ok := GetProjectID(c)
		if ok || got != (uuid.UUID{}) {
			t.Errorf("got (%v, %v), want (zero, false)", got, ok)
		}
	})

	t.Run("wrong type returns zero uuid and false", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(ProjectIDKey, "not-a-uuid")
		got, ok := GetProjectID(c)
		if ok || got != (uuid.UUID{}) {
			t.Errorf("got (%v, %v), want (zero, false)", got, ok)
		}
	})
}

func TestMustGetProjectID(t *testing.T) {
	t.Run("returns id when present", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(ProjectIDKey, testProjectID)
		if got := MustGetProjectID(c); got != testProjectID {
			t.Errorf("got %v, want %v", got, testProjectID)
		}
	})

	t.Run("panics when missing", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		expectPanic(t, func() { _ = MustGetProjectID(c) })
	})

	t.Run("panics when value is *uuid.UUID (legacy pointer form)", func(t *testing.T) {
		// Regression: SDK middleware used to store &uuid. Helper now expects value.
		c, _ := gin.CreateTestContext(nil)
		c.Set(ProjectIDKey, &testProjectID)
		expectPanic(t, func() { _ = MustGetProjectID(c) })
	})
}

func TestGetOrganizationID(t *testing.T) {
	t.Run("returns id when present", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(OrganizationIDKey, testOrgID)
		got, ok := GetOrganizationID(c)
		if !ok || got != testOrgID {
			t.Errorf("got (%v, %v), want (%v, true)", got, ok, testOrgID)
		}
	})

	t.Run("missing key returns zero uuid and false", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		got, ok := GetOrganizationID(c)
		if ok || got != (uuid.UUID{}) {
			t.Errorf("got (%v, %v), want (zero, false)", got, ok)
		}
	})
}

func TestMustGetOrganizationID(t *testing.T) {
	t.Run("returns id when present", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(OrganizationIDKey, testOrgID)
		if got := MustGetOrganizationID(c); got != testOrgID {
			t.Errorf("got %v, want %v", got, testOrgID)
		}
	})

	t.Run("panics when missing", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		expectPanic(t, func() { _ = MustGetOrganizationID(c) })
	})
}

func TestGetAPIKeyID_NullablePointerSemantic(t *testing.T) {
	// APIKeyID stays as *uuid.UUID because AuthContext.APIKeyID is nullable
	// (session-based auth contexts have no API key). Test it.
	t.Run("returns pointer when present", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(APIKeyIDKey, &testAPIKeyID)
		got, ok := GetAPIKeyID(c)
		if !ok || got == nil || *got != testAPIKeyID {
			t.Errorf("got (%v, %v), want (ptr to %v, true)", got, ok, testAPIKeyID)
		}
	})

	t.Run("missing key returns nil and false", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		got, ok := GetAPIKeyID(c)
		if ok || got != nil {
			t.Errorf("got (%v, %v), want (nil, false)", got, ok)
		}
	})
}

func TestGetSDKAuthContext(t *testing.T) {
	t.Run("returns context when present", func(t *testing.T) {
		want := &auth.AuthContext{UserID: testUUID, APIKeyID: &testAPIKeyID}
		c, _ := gin.CreateTestContext(nil)
		c.Set(SDKAuthContextKey, want)
		got, ok := GetSDKAuthContext(c)
		if !ok || got != want {
			t.Errorf("got (%v, %v), want (%v, true)", got, ok, want)
		}
	})
}

func TestMustGetSDKAuthContext(t *testing.T) {
	t.Run("returns context when present", func(t *testing.T) {
		want := &auth.AuthContext{UserID: testUUID, APIKeyID: &testAPIKeyID}
		c, _ := gin.CreateTestContext(nil)
		c.Set(SDKAuthContextKey, want)
		if got := MustGetSDKAuthContext(c); got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("panics when missing", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		expectPanic(t, func() { _ = MustGetSDKAuthContext(c) })
	})
}
