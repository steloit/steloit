package middleware

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
	orgDomain "brokle/internal/core/domain/organization"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// AuthMiddleware handles JWT token authentication and authorization
type AuthMiddleware struct {
	jwtService        auth.JWTService
	blacklistedTokens auth.BlacklistedTokenService
	orgMemberService  auth.OrganizationMemberService
	projectService    orgDomain.ProjectService
	logger            *slog.Logger
}

// NewAuthMiddleware creates a new stateless authentication middleware
func NewAuthMiddleware(
	jwtService auth.JWTService,
	blacklistedTokens auth.BlacklistedTokenService,
	orgMemberService auth.OrganizationMemberService,
	projectService orgDomain.ProjectService,
	logger *slog.Logger,
) *AuthMiddleware {
	return &AuthMiddleware{
		jwtService:        jwtService,
		blacklistedTokens: blacklistedTokens,
		orgMemberService:  orgMemberService,
		projectService:    projectService,
		logger:            logger,
	}
}

// Context keys for storing authentication data in Gin context
const (
	AuthContextKey = "auth_context"
	UserIDKey      = "user_id"
	TokenClaimsKey = "token_claims"
)

// RequireAuth middleware ensures valid JWT token with stateless authentication
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Extract token from Authorization header
		token, err := m.extractToken(c)
		if err != nil {
			m.logger.Warn("Failed to extract authentication token", "error", err)
			response.Unauthorized(c, "Authentication token required")
			c.Abort()
			return
		}

		// Validate JWT token structure and signature
		claims, err := m.jwtService.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			m.logger.Warn("Invalid JWT token", "error", err, "token_prefix", token[:min(len(token), 10)])
			response.Unauthorized(c, "Invalid authentication token")
			c.Abort()
			return
		}

		// Check if token is blacklisted (immediate revocation check)
		isBlacklisted, err := m.blacklistedTokens.IsTokenBlacklisted(c.Request.Context(), claims.JWTID)
		if err != nil {
			m.logger.Error("Failed to check token blacklist status", "error", err, "jti", claims.JWTID)
			response.InternalServerError(c, "Authentication verification failed")
			c.Abort()
			return
		}

		if isBlacklisted {
			m.logger.Warn("Blacklisted token attempted access", "jti", claims.JWTID, "user_id", claims.UserID)
			response.Unauthorized(c, "Authentication token has been revoked")
			c.Abort()
			return
		}

		// GDPR/SOC2 Compliance: Check user-wide timestamp blacklisting
		// This ensures ALL tokens issued before user revocation are blocked
		isUserBlacklisted, err := m.blacklistedTokens.IsUserBlacklistedAfterTimestamp(
			c.Request.Context(), claims.UserID, claims.IssuedAt)
		if err != nil {
			m.logger.Error("Failed to check user timestamp blacklist status", "error", err, "user_id", claims.UserID, "iat", claims.IssuedAt)
			response.InternalServerError(c, "Authentication verification failed")
			c.Abort()
			return
		}

		if isUserBlacklisted {
			m.logger.Warn("User token revoked - all sessions were revoked", "user_id", claims.UserID, "jti", claims.JWTID, "iat", claims.IssuedAt)
			response.Unauthorized(c, "All user sessions have been revoked")
			c.Abort()
			return
		}

		// Store clean authentication data in Gin context
		authContext := claims.GetUserContext()
		c.Set(AuthContextKey, authContext)
		c.Set(UserIDKey, claims.UserID)
		c.Set(TokenClaimsKey, claims)

		// Log successful authentication
		m.logger.Debug("Authentication successful", "user_id", claims.UserID, "jti", claims.JWTID)

		c.Next()
	})
}

// RequirePermission middleware ensures user has specific permission with effective permissions
func (m *AuthMiddleware) RequirePermission(permission string) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		userID := MustGetUserID(c)

		hasPermission, err := m.orgMemberService.CheckUserPermissions(
			c.Request.Context(),
			userID,
			[]string{permission},
		)
		if err != nil {
			m.logger.Error("Failed to check user permissions", "error", err, "user_id", userID, "permission", permission)
			response.InternalServerError(c, "Permission verification failed")
			c.Abort()
			return
		}

		if !hasPermission[permission] {
			m.logger.Warn("Insufficient permissions", "user_id", userID, "permission", permission)
			response.Forbidden(c, "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	})
}

// RequireProjectAccess ensures the authenticated user is a member of the
// organization that owns the project identified by either the ":projectId"
// path parameter (preferred) or the "project_id" query string.
//
// Must be mounted downstream of RequireAuth; the user-ID invariant is
// enforced via MustGetUserID, so a misconfigured route panics → Recovery → 500.
//
// Responses:
//   - 400 if the project identifier is missing or malformed;
//   - 403 if the caller is authenticated but has no org-level membership;
//   - 404 if the project does not exist;
//   - 500 on infrastructure errors or invariant violations (missing RequireAuth).
//
// On success, the resolved project UUID is pinned to the Gin context under
// ProjectIDKey so downstream handlers can read it via MustGetProjectID without
// re-parsing.
func (m *AuthMiddleware) RequireProjectAccess() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		userID := MustGetUserID(c)

		raw := c.Param("projectId")
		if raw == "" {
			raw = c.Query("project_id")
		}
		if raw == "" {
			response.Error(c, appErrors.NewValidationError(
				"Missing project ID", "project_id is required"))
			c.Abort()
			return
		}
		projectID, err := uuid.Parse(raw)
		if err != nil {
			response.Error(c, appErrors.NewValidationError(
				"Invalid project ID", "project_id must be a valid UUID"))
			c.Abort()
			return
		}

		canAccess, err := m.projectService.CanUserAccessProject(c.Request.Context(), userID, projectID)
		if err != nil {
			// projectService returns AppError constructors: forward the mapped
			// status (404 / 500) rather than dropping it to a generic 500.
			m.logger.Warn("project access check failed",
				"error", err, "user_id", userID, "project_id", projectID)
			response.Error(c, err)
			c.Abort()
			return
		}
		if !canAccess {
			m.logger.Warn("project access denied",
				"user_id", userID, "project_id", projectID)
			response.Error(c, appErrors.NewForbiddenError("Access denied to project"))
			c.Abort()
			return
		}

		c.Set(ProjectIDKey, projectID)
		c.Next()
	})
}

// RequireAnyPermission middleware ensures user has at least one of the specified permissions
func (m *AuthMiddleware) RequireAnyPermission(permissions []string) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		userID := MustGetUserID(c)

		hasPermission, err := m.orgMemberService.CheckUserPermissions(
			c.Request.Context(),
			userID,
			permissions,
		)
		if err != nil {
			m.logger.Error("Failed to check user permissions", "error", err, "user_id", userID, "permissions", permissions)
			response.InternalServerError(c, "Permission verification failed")
			c.Abort()
			return
		}

		hasAnyPermission := false
		for _, permission := range permissions {
			if hasPermission[permission] {
				hasAnyPermission = true
				break
			}
		}

		if !hasAnyPermission {
			m.logger.Warn("Insufficient permissions - none of the required permissions found", "user_id", userID, "permissions", permissions)
			response.Forbidden(c, "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	})
}

// RequireAllPermissions middleware ensures user has ALL specified permissions
func (m *AuthMiddleware) RequireAllPermissions(permissions []string) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		userID := MustGetUserID(c)

		hasPermission, err := m.orgMemberService.CheckUserPermissions(
			c.Request.Context(),
			userID,
			permissions,
		)
		if err != nil {
			m.logger.Error("Failed to check user permissions", "error", err, "user_id", userID, "permissions", permissions)
			response.InternalServerError(c, "Permission verification failed")
			c.Abort()
			return
		}

		for _, permission := range permissions {
			if !hasPermission[permission] {
				m.logger.Warn("Insufficient permissions - missing required permission", "user_id", userID, "permissions", permissions, "failed_on", permission)
				response.Forbidden(c, "Insufficient permissions")
				c.Abort()
				return
			}
		}

		c.Next()
	})
}

// OptionalAuth middleware extracts auth info if present but doesn't require it
func (m *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Try to extract token
		token, err := m.extractToken(c)
		if err != nil {
			// No token present, continue without auth context
			c.Next()
			return
		}

		// Validate token if present
		claims, err := m.jwtService.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			// Invalid token, continue without auth context
			c.Next()
			return
		}

		// Check blacklist
		isBlacklisted, err := m.blacklistedTokens.IsTokenBlacklisted(c.Request.Context(), claims.JWTID)
		if err != nil || isBlacklisted {
			// Blacklisted or error, continue without auth context
			c.Next()
			return
		}

		// Store clean auth context for valid token
		authContext := claims.GetUserContext()
		c.Set(AuthContextKey, authContext)
		c.Set(UserIDKey, claims.UserID)
		c.Set(TokenClaimsKey, claims)

		c.Next()
	})
}

// extractToken extracts JWT token from httpOnly cookie
func (m *AuthMiddleware) extractToken(c *gin.Context) (string, error) {
	// Read token from httpOnly cookie only (secure, XSS-protected)
	token, err := c.Cookie("access_token")
	if err != nil || token == "" {
		return "", errors.New("authentication token required in cookie")
	}

	return token, nil
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper functions to get auth data from Gin context.
//
// Each context value has a Get* form (returns `(value, ok)`) and a Must* form
// (panics on missing/wrong type — caught by Recovery → 500). Use Must* in
// handlers protected by RequireAuth where the value is a guaranteed invariant;
// use Get* only on routes that legitimately run without auth (OptionalAuth).

// GetAuthContext returns the dashboard auth context if present.
func GetAuthContext(c *gin.Context) (*auth.AuthContext, bool) {
	v, exists := c.Get(AuthContextKey)
	if !exists {
		return nil, false
	}
	ctx, ok := v.(*auth.AuthContext)
	return ctx, ok
}

// MustGetAuthContext returns the dashboard auth context. Panics if missing —
// signals a handler attached outside RequireAuth. Caught by Recovery → 500.
func MustGetAuthContext(c *gin.Context) *auth.AuthContext {
	ctx, ok := GetAuthContext(c)
	if !ok {
		panic("middleware.MustGetAuthContext: auth context not found — protected route is missing RequireAuth middleware")
	}
	return ctx
}

// GetTokenClaims returns the JWT claims for the current request if present.
func GetTokenClaims(c *gin.Context) (*auth.JWTClaims, bool) {
	v, exists := c.Get(TokenClaimsKey)
	if !exists {
		return nil, false
	}
	claims, ok := v.(*auth.JWTClaims)
	return claims, ok
}

// MustGetTokenClaims returns the JWT claims for the current request. Panics if
// missing — signals a handler attached outside RequireAuth. Caught by
// Recovery → 500.
func MustGetTokenClaims(c *gin.Context) *auth.JWTClaims {
	claims, ok := GetTokenClaims(c)
	if !ok {
		panic("middleware.MustGetTokenClaims: token claims not found — protected route is missing RequireAuth middleware")
	}
	return claims
}

// GetUserIDFromContext retrieves the authenticated user's ID from Gin context.
// Returns the zero UUID and false if no user is present or the value has the
// wrong type. Use this when a handler legitimately supports both authenticated
// and unauthenticated callers (routes with OptionalAuth).
//
// For routes guaranteed to have run RequireAuth, prefer MustGetUserID — it
// removes boilerplate and surfaces misconfiguration loudly via panic/Recovery.
func GetUserIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	userID, exists := c.Get(UserIDKey)
	if !exists {
		return uuid.UUID{}, false
	}

	id, ok := userID.(uuid.UUID)
	return id, ok
}

// MustGetUserID returns the authenticated user's ID from the Gin context.
//
// It assumes RequireAuth has run and set UserIDKey to a uuid.UUID. If the key
// is missing or has the wrong type, MustGetUserID panics with a descriptive
// message — this signals a programming bug (a protected route was registered
// without RequireAuth). The panic is caught by the Recovery middleware and
// returned to the client as an HTTP 500 with a request ID for log correlation.
//
// This follows the idiomatic Go Must* convention (regexp.MustCompile,
// template.Must, uuid.Must) and Gin's own BasicAuth pattern
// (c.MustGet(gin.AuthUserKey)): terminate loudly when an invariant the code
// structurally depends on is violated, rather than paper over a misconfigured
// route with a misleading 401.
//
// Use GetUserIDFromContext instead on routes protected by OptionalAuth.
func MustGetUserID(c *gin.Context) uuid.UUID {
	id, ok := GetUserIDFromContext(c)
	if !ok {
		panic("middleware.MustGetUserID: user not found in context — protected route is missing RequireAuth middleware")
	}
	return id
}

// Context Resolution Helper Functions

// ContextType represents the type of context to resolve
type ContextType string

const (
	ContextOrg     ContextType = "org"
	ContextProject ContextType = "project"
	ContextEnv     ContextType = "env"
)

// ContextResolver resolves context IDs from headers or URL parameters
type ContextResolver struct {
	OrganizationID *uuid.UUID
	ProjectID      *uuid.UUID
	Environment    string // Environment tag (e.g., "production", "staging")
}

// ResolveContext resolves specified context types (variadic - any combination is optional)
func ResolveContext(c *gin.Context, contextTypes ...ContextType) *ContextResolver {
	resolver := &ContextResolver{}

	// Build a set for faster lookup
	typeSet := make(map[ContextType]bool)
	for _, ctxType := range contextTypes {
		typeSet[ctxType] = true
	}

	// If no types specified, resolve all (backward compatibility)
	if len(contextTypes) == 0 {
		typeSet[ContextOrg] = true
		typeSet[ContextProject] = true
		typeSet[ContextEnv] = true
	}

	// Resolve organization ID if requested
	if typeSet[ContextOrg] {
		// Try X-Org-ID header first
		if orgIDHeader := c.GetHeader("X-Org-ID"); orgIDHeader != "" {
			if orgID, err := uuid.Parse(orgIDHeader); err == nil {
				resolver.OrganizationID = &orgID
			}
		}
		// Try orgId URL parameter if header failed
		if resolver.OrganizationID == nil {
			if orgIDParam := c.Param("orgId"); orgIDParam != "" {
				if orgID, err := uuid.Parse(orgIDParam); err == nil {
					resolver.OrganizationID = &orgID
				}
			}
		}
	}

	// Resolve project ID if requested
	if typeSet[ContextProject] {
		// Try X-Project-ID header first
		if projectIDHeader := c.GetHeader("X-Project-ID"); projectIDHeader != "" {
			if projectID, err := uuid.Parse(projectIDHeader); err == nil {
				resolver.ProjectID = &projectID
			}
		}
		// Try projectId URL parameter if header failed
		if resolver.ProjectID == nil {
			if projectIDParam := c.Param("projectId"); projectIDParam != "" {
				if projectID, err := uuid.Parse(projectIDParam); err == nil {
					resolver.ProjectID = &projectID
				}
			}
		}
	}

	// Resolve environment tag if requested
	if typeSet[ContextEnv] {
		// Try environment query parameter
		if envParam := c.Query("environment"); envParam != "" {
			resolver.Environment = envParam
		}
		// Default to "default" if no environment specified
		if resolver.Environment == "" {
			resolver.Environment = "default"
		}
	}

	return resolver
}

// Convenience functions for single context resolution
func ResolveOrganizationID(c *gin.Context) *uuid.UUID {
	return ResolveContext(c, ContextOrg).OrganizationID
}

func ResolveProjectID(c *gin.Context) *uuid.UUID {
	return ResolveContext(c, ContextProject).ProjectID
}

func ResolveEnvironment(c *gin.Context) string {
	return ResolveContext(c, ContextEnv).Environment
}
