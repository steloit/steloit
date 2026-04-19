package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"brokle/internal/core/domain/auth"
	orgDomain "brokle/internal/core/domain/organization"
	"brokle/internal/transport/http/httpctx"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// AuthDeps groups the services and logger every dashboard auth
// middleware needs. The struct is populated once at server startup
// (server.go) and threaded into each constructor — keeps the
// constructor signatures tight and lets tests pass a single argument.
//
// Constructor functions (RequireAuth, OptionalAuth, RequirePermission,
// …) capture an AuthDeps by value via closure and return the chi
// middleware. This is the canonical chi shape — see go-chi/jwtauth's
// Verifier(*JWTAuth), go-chi/oauth's NewBearerAuthentication.
type AuthDeps struct {
	JWT       auth.JWTService
	Blacklist auth.BlacklistedTokenService
	OrgMember auth.OrganizationMemberService
	Project   orgDomain.ProjectService
	Logger    *slog.Logger
}

// RequireAuth enforces a valid dashboard JWT (delivered via the
// access_token httpOnly cookie) for the wrapped handler. On success
// it writes the auth context, user ID, and JWT claims into the
// request context via httpctx so handlers can read them with
// MustGetAuthContext / MustGetUserID / MustGetTokenClaims.
//
// Failure paths:
//   - missing/empty cookie         → 401 authentication_error
//   - JWT decode/signature failure → 401 authentication_error
//   - blacklisted JWT              → 401 (revocation check)
//   - user-wide token revocation   → 401 (GDPR/SOC2 compliance check)
//   - blacklist lookup failure     → 500 api_error (infra failure)
//
// All failures emit a structured slog record with the request_id
// (correlated by middleware.RequestID) so traces are easy to find.
func RequireAuth(d AuthDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := tokenFromCookie(r)
			if err != nil {
				d.Logger.WarnContext(r.Context(), "authentication: missing access_token cookie", "error", err)
				response.WriteError(w, appErrors.NewUnauthorizedError("Authentication token required"))
				return
			}

			claims, err := d.JWT.ValidateAccessToken(r.Context(), token)
			if err != nil {
				d.Logger.WarnContext(r.Context(), "authentication: invalid JWT", "error", err)
				response.WriteError(w, appErrors.NewUnauthorizedError("Invalid authentication token"))
				return
			}

			// Per-token blacklist (immediate revocation by jti).
			revoked, err := d.Blacklist.IsTokenBlacklisted(r.Context(), claims.JWTID)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "authentication: blacklist lookup failed", "error", err, "jti", claims.JWTID)
				response.WriteError(w, appErrors.NewInternalError("Authentication verification failed", err))
				return
			}
			if revoked {
				d.Logger.WarnContext(r.Context(), "authentication: blacklisted JWT", "jti", claims.JWTID, "user_id", claims.UserID)
				response.WriteError(w, appErrors.NewUnauthorizedError("Authentication token has been revoked"))
				return
			}

			// User-wide timestamp blacklist (revoke-all-sessions, GDPR/SOC2).
			revokedByUser, err := d.Blacklist.IsUserBlacklistedAfterTimestamp(r.Context(), claims.UserID, claims.IssuedAt)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "authentication: user-wide blacklist lookup failed", "error", err, "user_id", claims.UserID)
				response.WriteError(w, appErrors.NewInternalError("Authentication verification failed", err))
				return
			}
			if revokedByUser {
				d.Logger.WarnContext(r.Context(), "authentication: user sessions revoked", "user_id", claims.UserID, "iat", claims.IssuedAt)
				response.WriteError(w, appErrors.NewUnauthorizedError("All user sessions have been revoked"))
				return
			}

			ctx := r.Context()
			ctx = httpctx.WithAuthContext(ctx, claims.GetUserContext())
			ctx = httpctx.WithUserID(ctx, claims.UserID)
			ctx = httpctx.WithTokenClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth populates the auth context if a valid JWT is present
// but never rejects the request — used for endpoints that have both
// public and authenticated behaviour (e.g. landing-page personalisation,
// audit-log entries that record actor when known).
//
// Distinct constructor rather than a flag on RequireAuth so the call
// site reads correctly: a route guarded by OptionalAuth is visibly
// different from one guarded by RequireAuth.
func OptionalAuth(d AuthDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := tokenFromCookie(r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := d.JWT.ValidateAccessToken(r.Context(), token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			revoked, err := d.Blacklist.IsTokenBlacklisted(r.Context(), claims.JWTID)
			if err != nil || revoked {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			ctx = httpctx.WithAuthContext(ctx, claims.GetUserContext())
			ctx = httpctx.WithUserID(ctx, claims.UserID)
			ctx = httpctx.WithTokenClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission enforces that the authenticated user has the
// supplied permission. Must be mounted downstream of RequireAuth —
// the user-ID invariant is enforced via httpctx.MustGetUserID, so a
// misconfigured route panics into the recoverer (HTTP 500 with stack)
// instead of silently returning a misleading 401. This mirrors
// regexp.MustCompile / template.Must (CLAUDE.md gotcha #6).
//
// Designed for chi's `r.With()` composition:
//
//	r.With(mw.RequirePermission(deps, "orgs:read")).Get("/", handler)
func RequirePermission(d AuthDeps, permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := httpctx.MustGetUserID(r.Context())
			if !checkPermissions(r.Context(), w, d, userID, []string{permission}, allRequired) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission enforces that the authenticated user has at
// least one of the supplied permissions. Variadic so callers can list
// permissions inline:
//
//	r.With(mw.RequireAnyPermission(deps, "orgs:read", "orgs:admin")).Get(...)
func RequireAnyPermission(d AuthDeps, permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := httpctx.MustGetUserID(r.Context())
			if !checkPermissions(r.Context(), w, d, userID, permissions, anyOf) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAllPermissions enforces that the authenticated user has
// every permission in the supplied list. Variadic for symmetry with
// RequireAnyPermission.
func RequireAllPermissions(d AuthDeps, permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := httpctx.MustGetUserID(r.Context())
			if !checkPermissions(r.Context(), w, d, userID, permissions, allRequired) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// permissionMode controls how a list of permissions is evaluated by
// checkPermissions. Single-permission RequirePermission uses
// allRequired with a single-element slice — semantically equivalent
// to "the one permission is required".
type permissionMode int

const (
	allRequired permissionMode = iota
	anyOf
)

// checkPermissions runs the RBAC service lookup for a permission
// list and writes a 403 envelope on failure. Returns true when the
// caller should continue, false when a response has already been
// written.
//
// Centralised so RequirePermission, RequireAnyPermission, and
// RequireAllPermissions share one error-response shape and one set of
// log fields.
func checkPermissions(ctx context.Context, w http.ResponseWriter, d AuthDeps, userID uuid.UUID, perms []string, mode permissionMode) bool {
	results, err := d.OrgMember.CheckUserPermissions(ctx, userID, perms)
	if err != nil {
		d.Logger.ErrorContext(ctx, "authorization: permission check failed", "error", err, "user_id", userID, "permissions", perms)
		response.WriteError(w, appErrors.NewInternalError("Permission verification failed", err))
		return false
	}

	switch mode {
	case anyOf:
		for _, p := range perms {
			if results[p] {
				return true
			}
		}
	case allRequired:
		missing := ""
		for _, p := range perms {
			if !results[p] {
				missing = p
				break
			}
		}
		if missing == "" {
			return true
		}
		d.Logger.WarnContext(ctx, "authorization: missing permission", "user_id", userID, "permissions", perms, "missing", missing)
		response.WriteError(w, appErrors.NewForbiddenError("Insufficient permissions"))
		return false
	}

	d.Logger.WarnContext(ctx, "authorization: none of required permissions held", "user_id", userID, "permissions", perms)
	response.WriteError(w, appErrors.NewForbiddenError("Insufficient permissions"))
	return false
}

// RequireProjectAccess ensures the authenticated user is a member of
// the organization that owns the project identified by either the
// `:projectId` path parameter (preferred) or the `project_id` query
// string. Mounted downstream of RequireAuth.
//
// Responses:
//   - 422 invalid_request_error if project ID is missing or malformed
//   - 403 permission_error      if the caller has no org-level membership
//   - 404 not_found_error       if the project does not exist
//   - 500 api_error             on infrastructure failures or invariant violations
//
// On success, the resolved project UUID is written into the request
// context via httpctx.WithProjectID so downstream handlers can read
// it with httpctx.MustGetProjectID without re-parsing.
func RequireProjectAccess(d AuthDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := httpctx.MustGetUserID(r.Context())

			raw := chi.URLParam(r, "projectId")
			if raw == "" {
				raw = r.URL.Query().Get("project_id")
			}
			if raw == "" {
				response.WriteError(w, appErrors.NewValidationError("Missing project ID", "project_id is required"))
				return
			}
			projectID, err := uuid.Parse(raw)
			if err != nil {
				response.WriteError(w, appErrors.NewValidationError("Invalid project ID", "project_id must be a valid UUID"))
				return
			}

			canAccess, err := d.Project.CanUserAccessProject(r.Context(), userID, projectID)
			if err != nil {
				// Project service returns AppError — preserve its mapped
				// status (404 / 500) rather than dropping to a generic 500.
				d.Logger.WarnContext(r.Context(), "authorization: project access check failed", "error", err, "user_id", userID, "project_id", projectID)
				response.WriteError(w, err)
				return
			}
			if !canAccess {
				d.Logger.WarnContext(r.Context(), "authorization: project access denied", "user_id", userID, "project_id", projectID)
				response.WriteError(w, appErrors.NewForbiddenError("Access denied to project"))
				return
			}

			next.ServeHTTP(w, r.WithContext(httpctx.WithProjectID(r.Context(), projectID)))
		})
	}
}

// tokenFromCookie reads the access_token httpOnly cookie. Returns
// errMissingCookie when the cookie is absent or empty so callers
// (RequireAuth vs OptionalAuth) can branch on a typed sentinel
// instead of string-comparing error messages.
func tokenFromCookie(r *http.Request) (string, error) {
	c, err := r.Cookie("access_token")
	if err != nil {
		return "", errMissingCookie
	}
	if c.Value == "" {
		return "", errMissingCookie
	}
	return c.Value, nil
}

var errMissingCookie = errors.New("authentication token required in cookie")
