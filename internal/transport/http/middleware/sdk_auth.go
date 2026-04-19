package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"brokle/internal/core/domain/auth"
	"brokle/internal/transport/http/httpctx"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// SDKAuthDeps groups the services and logger every SDK auth middleware
// needs. Same constructor-with-Deps pattern as AuthDeps; populated once
// at server start and threaded into RequireSDKAuth.
type SDKAuthDeps struct {
	APIKey auth.APIKeyService
	Logger *slog.Logger
}

// RequireSDKAuth validates the self-contained SDK API key on every
// request to /v1/* (the SDK ingest plane). On success it writes the
// SDK auth context, API key ID, project ID, and organization ID into
// the request context via httpctx so handlers can read them with
// MustGetSDKAuthContext / MustGetProjectID / MustGetOrganizationID.
//
// Header source order:
//  1. X-API-Key — canonical SDK contract
//  2. Authorization: Bearer <key> — fallback for HTTP clients that
//     can't set custom headers (some browser fetch harnesses, CLIs)
//
// Failure paths:
//   - missing header             → 401 authentication_error
//   - APIKeyService validate err → status forwarded from the AppError
//     the service returns (typically 401 invalid key, 403 disabled,
//     500 infra failure).
func RequireSDKAuth(d SDKAuthDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := apiKeyFromRequest(r)
			if apiKey == "" {
				d.Logger.WarnContext(r.Context(), "sdk auth: missing API key")
				response.WriteError(w, appErrors.NewUnauthorizedError("API key required"))
				return
			}

			result, err := d.APIKey.ValidateAPIKey(r.Context(), apiKey)
			if err != nil {
				d.Logger.WarnContext(r.Context(), "sdk auth: API key validation failed", "error", err)
				response.WriteError(w, err) // forward AppError status verbatim
				return
			}

			ctx := r.Context()
			ctx = httpctx.WithSDKAuthContext(ctx, result.AuthContext)
			ctx = httpctx.WithAPIKeyID(ctx, result.AuthContext.APIKeyID)
			ctx = httpctx.WithProjectID(ctx, result.ProjectID)
			ctx = httpctx.WithOrganizationID(ctx, result.OrganizationID)

			d.Logger.DebugContext(r.Context(), "sdk auth: validated",
				"api_key_id", result.AuthContext.APIKeyID,
				"project_id", result.ProjectID,
				"organization_id", result.OrganizationID,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// apiKeyFromRequest extracts the SDK API key, preferring X-API-Key
// over the Authorization: Bearer fallback. Returns "" when neither is
// supplied or both are empty.
func apiKeyFromRequest(r *http.Request) string {
	if v := r.Header.Get("X-API-Key"); v != "" {
		return v
	}
	if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return v
	}
	return ""
}
