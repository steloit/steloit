package middleware

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
	"brokle/internal/transport/http/httpctx"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// ScopeDeps groups the services and logger every scope-check middleware
// needs. Same constructor-with-Deps pattern as AuthDeps.
type ScopeDeps struct {
	Scope  auth.ScopeService
	Logger *slog.Logger
}

// RequireScope enforces that the authenticated user holds a specific
// scope in the resolved organization/project context.
//
// Context resolution: the dashboard plane (/api/v1/*) carries org/
// project IDs in the X-Org-ID / X-Project-ID headers. The SDK plane
// (/v1/*) derives them from the API key via RequireSDKAuth and
// reaches the request context via httpctx; this middleware only
// applies to the dashboard plane and reads from headers.
//
// Mounted downstream of RequireAuth — user-ID invariant via
// httpctx.MustGetUserID, mismatched routing panics into the recoverer.
//
// Failure paths:
//   - missing/invalid org or project header → 422 validation_error
//   - scope service infra failure           → 500 api_error
//   - scope not held                        → 403 permission_error
//
// Compose with chi's r.With:
//
//	r.With(mw.RequireAuth(authDeps), mw.RequireScope(scopeDeps, "members:invite")).Post("/", inviteMember)
func RequireScope(d ScopeDeps, scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := httpctx.MustGetUserID(r.Context())
			orgID, projectID, err := scopeContextFromHeaders(r)
			if err != nil {
				response.WriteError(w, err)
				return
			}

			ok, err := d.Scope.HasScope(r.Context(), userID, scope, orgID, projectID)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "scope check failed", "error", err, "user_id", userID, "scope", scope, "org_id", orgID, "project_id", projectID)
				response.WriteError(w, appErrors.NewInternalError("Scope verification failed", err))
				return
			}
			if !ok {
				d.Logger.WarnContext(r.Context(), "scope denied", "user_id", userID, "scope", scope, "org_id", orgID, "project_id", projectID)
				response.WriteError(w, appErrors.NewForbiddenError("Insufficient permissions"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyScope enforces that the user holds at least one of the
// supplied scopes. Variadic for inline composition.
func RequireAnyScope(d ScopeDeps, scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := httpctx.MustGetUserID(r.Context())
			orgID, projectID, err := scopeContextFromHeaders(r)
			if err != nil {
				response.WriteError(w, err)
				return
			}

			ok, err := d.Scope.HasAnyScope(r.Context(), userID, scopes, orgID, projectID)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "any-scope check failed", "error", err, "user_id", userID, "scopes", scopes, "org_id", orgID, "project_id", projectID)
				response.WriteError(w, appErrors.NewInternalError("Scope verification failed", err))
				return
			}
			if !ok {
				d.Logger.WarnContext(r.Context(), "any-scope denied", "user_id", userID, "scopes", scopes, "org_id", orgID, "project_id", projectID)
				response.WriteError(w, appErrors.NewForbiddenError("Insufficient permissions"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAllScopes enforces that the user holds every supplied scope.
// Variadic for symmetry with RequireAnyScope.
func RequireAllScopes(d ScopeDeps, scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := httpctx.MustGetUserID(r.Context())
			orgID, projectID, err := scopeContextFromHeaders(r)
			if err != nil {
				response.WriteError(w, err)
				return
			}

			ok, err := d.Scope.HasAllScopes(r.Context(), userID, scopes, orgID, projectID)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "all-scopes check failed", "error", err, "user_id", userID, "scopes", scopes, "org_id", orgID, "project_id", projectID)
				response.WriteError(w, appErrors.NewInternalError("Scope verification failed", err))
				return
			}
			if !ok {
				d.Logger.WarnContext(r.Context(), "all-scopes denied: missing one or more", "user_id", userID, "scopes", scopes, "org_id", orgID, "project_id", projectID)
				response.WriteError(w, appErrors.NewForbiddenError("Insufficient permissions"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// scopeContextFromHeaders resolves the org and project context for a
// dashboard request. Both are optional pointers — a scope check can
// be org-level (project nil) or project-level (both set). Header
// values that are present but malformed return an AppError so the
// caller can surface a 422 with a useful message.
//
// Headers: X-Org-ID, X-Project-ID. The route-param resolution
// (`/orgs/:orgId/...`) used in the legacy ContextResolver is dropped
// — Brokle's dashboard routes derive context from headers, not URL
// segments, so the dual-source heuristic was Brokle-specific cruft
// the chi migration removes.
func scopeContextFromHeaders(r *http.Request) (orgID, projectID *uuid.UUID, err error) {
	if v := r.Header.Get("X-Org-ID"); v != "" {
		id, parseErr := uuid.Parse(v)
		if parseErr != nil {
			return nil, nil, appErrors.NewValidationError("Invalid X-Org-ID", "X-Org-ID must be a valid UUID")
		}
		orgID = &id
	}
	if v := r.Header.Get("X-Project-ID"); v != "" {
		id, parseErr := uuid.Parse(v)
		if parseErr != nil {
			return nil, nil, appErrors.NewValidationError("Invalid X-Project-ID", "X-Project-ID must be a valid UUID")
		}
		projectID = &id
	}
	return orgID, projectID, nil
}
