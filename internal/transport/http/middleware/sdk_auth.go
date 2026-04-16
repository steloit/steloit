package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"brokle/internal/core/domain/auth"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

// SDKAuthMiddleware handles API key authentication for SDK routes
type SDKAuthMiddleware struct {
	apiKeyService auth.APIKeyService
	logger        *slog.Logger
}

// NewSDKAuthMiddleware creates a new SDK authentication middleware
func NewSDKAuthMiddleware(
	apiKeyService auth.APIKeyService,
	logger *slog.Logger,
) *SDKAuthMiddleware {
	return &SDKAuthMiddleware{
		apiKeyService: apiKeyService,
		logger:        logger,
	}
}

// Context keys for SDK authentication
const (
	SDKAuthContextKey  = "sdk_auth_context"
	APIKeyIDKey        = "api_key_id"
	ProjectIDKey       = "project_id"
	OrganizationIDKey  = "organization_id"
	EnvironmentKey     = "environment"
)

// RequireSDKAuth middleware validates self-contained API keys for SDK routes
// Extracts project ID automatically from the API key (no additional headers required)
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

		// Store SDK authentication context in Gin context
		c.Set(SDKAuthContextKey, validateResp.AuthContext)
		c.Set(APIKeyIDKey, validateResp.AuthContext.APIKeyID)
		// Store project ID and organization ID as pointers to match getter expectations
		projectID := validateResp.ProjectID
		c.Set(ProjectIDKey, &projectID)
		organizationID := validateResp.OrganizationID
		c.Set(OrganizationIDKey, &organizationID)

		// Log successful SDK authentication
		m.logger.Debug("SDK authentication successful", "api_key_id", validateResp.AuthContext.APIKeyID, "project_id", validateResp.ProjectID, "organization_id", validateResp.OrganizationID)

		c.Next()
	})
}

// Helper functions to get SDK auth data from Gin context

// GetSDKAuthContext retrieves SDK authentication context from Gin context
func GetSDKAuthContext(c *gin.Context) (*auth.AuthContext, bool) {
	authContext, exists := c.Get(SDKAuthContextKey)
	if !exists {
		return nil, false
	}

	ctx, ok := authContext.(*auth.AuthContext)
	return ctx, ok
}

// GetAPIKeyID retrieves API key ID from Gin context
func GetAPIKeyID(c *gin.Context) (*ulid.ULID, bool) {
	keyID, exists := c.Get(APIKeyIDKey)
	if !exists {
		return nil, false
	}

	id, ok := keyID.(*ulid.ULID)
	return id, ok
}

// GetProjectID retrieves project ID from Gin context
func GetProjectID(c *gin.Context) (*ulid.ULID, bool) {
	projectID, exists := c.Get(ProjectIDKey)
	if !exists {
		return nil, false
	}

	id, ok := projectID.(*ulid.ULID)
	return id, ok
}

// GetOrganizationID retrieves organization ID from Gin context
func GetOrganizationID(c *gin.Context) (*ulid.ULID, bool) {
	organizationID, exists := c.Get(OrganizationIDKey)
	if !exists {
		return nil, false
	}

	id, ok := organizationID.(*ulid.ULID)
	return id, ok
}

// GetEnvironment retrieves environment from Gin context
func GetEnvironment(c *gin.Context) (string, bool) {
	environment, exists := c.Get(EnvironmentKey)
	if !exists {
		return "", false
	}

	env, ok := environment.(string)
	return env, ok
}

