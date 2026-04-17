package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
	"brokle/pkg/response"
)

// ScopeMiddleware handles scope-based authorization
// This is the NEW middleware that replaces RequirePermission*
type ScopeMiddleware struct {
	scopeService auth.ScopeService
	logger       *slog.Logger
}

// NewScopeMiddleware creates a new scope-based authorization middleware
func NewScopeMiddleware(
	scopeService auth.ScopeService,
	logger *slog.Logger,
) *ScopeMiddleware {
	return &ScopeMiddleware{
		scopeService: scopeService,
		logger:       logger,
	}
}

// RequireScope middleware ensures user has a specific scope in the current context
//
// This middleware automatically resolves organization and project context from:
// - Headers: X-Org-ID, X-Project-ID
// - URL params: orgId, projectId
//
// Scope Resolution:
// - Organization-level scope (e.g., "members:invite") → requires org context
// - Project-level scope (e.g., "traces:delete") → requires org + project context
//
// Usage:
//
//	router.POST("/members", authMiddleware.RequireAuth(), scopeMiddleware.RequireScope("members:invite"), handler.InviteMember)
//	router.DELETE("/traces/:id", authMiddleware.RequireAuth(), scopeMiddleware.RequireScope("traces:delete"), handler.DeleteTrace)
func (m *ScopeMiddleware) RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Must be mounted downstream of RequireAuth; misconfiguration panics
		// via Recovery → 500, instead of masquerading as a 401.
		userID := MustGetUserID(c)

		// Resolve organization and project context from request
		resolver := ResolveContext(c, ContextOrg, ContextProject)

		// Check if user has the required scope
		hasScope, err := m.scopeService.HasScope(
			c.Request.Context(),
			userID,
			scope,
			resolver.OrganizationID,
			resolver.ProjectID,
		)

		if err != nil {
			m.logger.Error("Failed to check user scope", "error", err, "user_id", userID, "scope", scope, "org_id", resolver.OrganizationID, "project_id", resolver.ProjectID)
			response.InternalServerError(c, "Scope verification failed")
			c.Abort()
			return
		}

		if !hasScope {
			m.logger.Warn("Insufficient scopes", "user_id", userID, "scope", scope, "org_id", resolver.OrganizationID, "project_id", resolver.ProjectID)
			response.Forbidden(c, "Insufficient permissions")
			c.Abort()
			return
		}

		// Store scope context in Gin context for handlers
		scopeContext := &ScopeContext{
			UserID:         userID,
			OrganizationID: resolver.OrganizationID,
			ProjectID:      resolver.ProjectID,
			Scopes:         []string{scope}, // At least this scope is guaranteed
		}
		c.Set(ScopeContextKey, scopeContext)

		m.logger.Debug("Scope check passed", "user_id", userID, "scope", scope, "org_id", resolver.OrganizationID, "project_id", resolver.ProjectID)

		c.Next()
	}
}

// RequireAnyScope middleware ensures user has at least one of the specified scopes
//
// Useful for endpoints that accept multiple permission levels, e.g.:
//
//	scopeMiddleware.RequireAnyScope([]string{"billing:manage", "billing:admin"})
func (m *ScopeMiddleware) RequireAnyScope(scopes []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := MustGetUserID(c)

		resolver := ResolveContext(c, ContextOrg, ContextProject)

		hasAny, err := m.scopeService.HasAnyScope(
			c.Request.Context(),
			userID,
			scopes,
			resolver.OrganizationID,
			resolver.ProjectID,
		)

		if err != nil {
			m.logger.Error("Failed to check user scopes", "error", err, "user_id", userID, "scopes", scopes, "org_id", resolver.OrganizationID, "project_id", resolver.ProjectID)
			response.InternalServerError(c, "Scope verification failed")
			c.Abort()
			return
		}

		if !hasAny {
			m.logger.Warn("Insufficient scopes - none of the required scopes found", "user_id", userID, "scopes", scopes, "org_id", resolver.OrganizationID, "project_id", resolver.ProjectID)
			response.Forbidden(c, "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAllScopes middleware ensures user has ALL of the specified scopes
//
// Useful for endpoints that require multiple permissions, e.g.:
//
//	scopeMiddleware.RequireAllScopes([]string{"traces:read", "analytics:export"})
func (m *ScopeMiddleware) RequireAllScopes(scopes []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := MustGetUserID(c)

		resolver := ResolveContext(c, ContextOrg, ContextProject)

		hasAll, err := m.scopeService.HasAllScopes(
			c.Request.Context(),
			userID,
			scopes,
			resolver.OrganizationID,
			resolver.ProjectID,
		)

		if err != nil {
			m.logger.Error("Failed to check user scopes", "error", err, "user_id", userID, "scopes", scopes, "org_id", resolver.OrganizationID, "project_id", resolver.ProjectID)
			response.InternalServerError(c, "Scope verification failed")
			c.Abort()
			return
		}

		if !hasAll {
			m.logger.Warn("Insufficient scopes - missing required scopes", "user_id", userID, "scopes", scopes, "org_id", resolver.OrganizationID, "project_id", resolver.ProjectID)
			response.Forbidden(c, "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// ScopeContext holds resolved scope context for a request
type ScopeContext struct {
	OrganizationID *uuid.UUID
	ProjectID      *uuid.UUID
	Scopes         []string
	UserID         uuid.UUID
}

// Context key for storing scope context
const ScopeContextKey = "scope_context"

// GetScopeContext retrieves scope context from Gin context (for handlers)
func GetScopeContext(c *gin.Context) (*ScopeContext, bool) {
	scopeCtx, exists := c.Get(ScopeContextKey)
	if !exists {
		return nil, false
	}

	ctx, ok := scopeCtx.(*ScopeContext)
	return ctx, ok
}
