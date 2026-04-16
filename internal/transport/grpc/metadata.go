package grpc

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/metadata"

	"github.com/google/uuid"
)

// Context keys for storing authentication data
type contextKey string

const (
	contextKeyProjectID contextKey = "project_id"
	contextKeyAPIKeyID  contextKey = "api_key_id"
)

// extractAPIKeyFromMetadata extracts API key from gRPC metadata
// Supports both "authorization" and "x-api-key" headers (OTLP standard)
func extractAPIKeyFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("no metadata in gRPC context")
	}

	// Try Authorization header (Bearer token format)
	if auth := md.Get("authorization"); len(auth) > 0 {
		bearer := auth[0]

		// Support "Bearer bk_..." format
		if strings.HasPrefix(bearer, "Bearer ") {
			apiKey := strings.TrimPrefix(bearer, "Bearer ")
			if strings.HasPrefix(apiKey, "bk_") {
				return apiKey, nil
			}
		}

		// Support direct "bk_..." format (no Bearer prefix)
		if strings.HasPrefix(bearer, "bk_") {
			return bearer, nil
		}
	}

	// Try X-API-Key header (alternative OTLP convention)
	if apiKey := md.Get("x-api-key"); len(apiKey) > 0 {
		if strings.HasPrefix(apiKey[0], "bk_") {
			return apiKey[0], nil
		}
	}

	return "", fmt.Errorf("API key not found in gRPC metadata (tried 'authorization' and 'x-api-key' headers)")
}

// extractProjectIDFromContext retrieves project ID from authenticated context
// Set by auth interceptor after successful API key validation
func extractProjectIDFromContext(ctx context.Context) (*uuid.UUID, error) {
	val := ctx.Value(contextKeyProjectID)
	if val == nil {
		return nil, fmt.Errorf("project_id not found in context (authentication may have failed)")
	}

	projectID, ok := val.(*uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("project_id has invalid type in context")
	}

	return projectID, nil
}

// extractAPIKeyIDFromContext retrieves API key ID from authenticated context
func extractAPIKeyIDFromContext(ctx context.Context) (*uuid.UUID, error) {
	val := ctx.Value(contextKeyAPIKeyID)
	if val == nil {
		return nil, fmt.Errorf("api_key_id not found in context")
	}

	apiKeyID, ok := val.(*uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("api_key_id has invalid type in context")
	}

	return apiKeyID, nil
}

// storeAuthDataInContext stores authentication data in context
// Used by auth interceptor after successful validation
func storeAuthDataInContext(ctx context.Context, projectID, apiKeyID *uuid.UUID) context.Context {
	ctx = context.WithValue(ctx, contextKeyProjectID, projectID)
	ctx = context.WithValue(ctx, contextKeyAPIKeyID, apiKeyID)
	return ctx
}
