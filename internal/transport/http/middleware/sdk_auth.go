package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
	"brokle/pkg/response"
)

// SDKAuthMiddleware handles API key authentication for SDK routes.
type SDKAuthMiddleware struct {
	apiKeyService auth.APIKeyService
	logger        *slog.Logger
}

// NewSDKAuthMiddleware creates a new SDK authentication middleware.
func NewSDKAuthMiddleware(
	apiKeyService auth.APIKeyService,
	logger *slog.Logger,
) *SDKAuthMiddleware {
	return &SDKAuthMiddleware{
		apiKeyService: apiKeyService,
		logger:        logger,
	}
}

// Context keys for SDK authentication.
const (
	SDKAuthContextKey = "sdk_auth_context"
	APIKeyIDKey       = "api_key_id"
	ProjectIDKey      = "project_id"
	OrganizationIDKey = "organization_id"
	EnvironmentKey    = "environment"
)

// RequireSDKAuth validates self-contained API keys for SDK routes.
// On success it sets SDKAuthContextKey, APIKeyIDKey, ProjectIDKey, and
// OrganizationIDKey in the Gin context. Handlers protected by this middleware
// should use MustGetSDKAuthContext / MustGetProjectID / MustGetOrganizationID
// to read those values — the invariant is guaranteed once the middleware runs.
func (m *SDKAuthMiddleware) RequireSDKAuth() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Extract API key from X-API-Key header
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			// Fallback to Authorization header with Bearer format
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				apiKey = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if apiKey == "" {
			m.logger.Warn("SDK request missing API key")
			response.Unauthorized(c, "API key required")
			c.Abort()
			return
		}

		// Validate API key (self-contained, extracts project ID automatically)
		validateResp, err := m.apiKeyService.ValidateAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			m.logger.Warn("API key validation failed", "error", err)
			response.Error(c, err) // Properly propagate AppError status codes (401, 403, etc.)
			c.Abort()
			return
		}

		// Store SDK authentication context by value. APIKeyID stays as *uuid.UUID
		// because AuthContext.APIKeyID is nullable (session-based contexts have no
		// API key); ProjectID / OrganizationID are always present after RequireSDKAuth
		// and are stored as uuid.UUID values.
		c.Set(SDKAuthContextKey, validateResp.AuthContext)
		c.Set(APIKeyIDKey, validateResp.AuthContext.APIKeyID)
		c.Set(ProjectIDKey, validateResp.ProjectID)
		c.Set(OrganizationIDKey, validateResp.OrganizationID)

		m.logger.Debug("SDK authentication successful",
			"api_key_id", validateResp.AuthContext.APIKeyID,
			"project_id", validateResp.ProjectID,
			"organization_id", validateResp.OrganizationID,
		)

		c.Next()
	})
}

// Helper functions to read SDK auth data from Gin context.
//
// Each helper has a Get* (returns `(value, ok)`) and a Must* variant. Use Must*
// in handlers protected by RequireSDKAuth where the value is a guaranteed
// invariant; use Get* only when the route legitimately runs without SDK auth.

// GetSDKAuthContext returns the SDK auth context if present.
func GetSDKAuthContext(c *gin.Context) (*auth.AuthContext, bool) {
	v, exists := c.Get(SDKAuthContextKey)
	if !exists {
		return nil, false
	}
	ctx, ok := v.(*auth.AuthContext)
	return ctx, ok
}

// MustGetSDKAuthContext returns the SDK auth context. Panics if missing or
// the wrong type — signals that a handler was attached outside RequireSDKAuth.
// The panic is caught by the Recovery middleware and returned as HTTP 500.
func MustGetSDKAuthContext(c *gin.Context) *auth.AuthContext {
	ctx, ok := GetSDKAuthContext(c)
	if !ok {
		panic("middleware.MustGetSDKAuthContext: SDK auth context not found — route is missing RequireSDKAuth middleware")
	}
	return ctx
}

// GetAPIKeyID returns the API key ID if present. Returns a pointer because
// the domain type (AuthContext.APIKeyID) is nullable — session-based auth
// contexts have no API key even when an auth context is present.
func GetAPIKeyID(c *gin.Context) (*uuid.UUID, bool) {
	v, exists := c.Get(APIKeyIDKey)
	if !exists {
		return nil, false
	}
	id, ok := v.(*uuid.UUID)
	return id, ok
}

// GetProjectID returns the SDK-context project ID if present.
func GetProjectID(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get(ProjectIDKey)
	if !exists {
		return uuid.UUID{}, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// MustGetProjectID returns the SDK-context project ID. Panics if missing —
// signals a handler attached outside RequireSDKAuth. Caught by Recovery → 500.
func MustGetProjectID(c *gin.Context) uuid.UUID {
	id, ok := GetProjectID(c)
	if !ok {
		panic("middleware.MustGetProjectID: project ID not found in context — SDK route is missing RequireSDKAuth middleware")
	}
	return id
}

// GetOrganizationID returns the SDK-context organization ID if present.
func GetOrganizationID(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get(OrganizationIDKey)
	if !exists {
		return uuid.UUID{}, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// MustGetOrganizationID returns the SDK-context organization ID. Panics if
// missing — signals a handler attached outside RequireSDKAuth. Caught by
// Recovery → 500.
func MustGetOrganizationID(c *gin.Context) uuid.UUID {
	id, ok := GetOrganizationID(c)
	if !ok {
		panic("middleware.MustGetOrganizationID: organization ID not found in context — SDK route is missing RequireSDKAuth middleware")
	}
	return id
}

// GetEnvironment returns the SDK-context environment tag if present.
func GetEnvironment(c *gin.Context) (string, bool) {
	v, exists := c.Get(EnvironmentKey)
	if !exists {
		return "", false
	}
	env, ok := v.(string)
	return env, ok
}
