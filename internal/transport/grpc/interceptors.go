package grpc

import (
	"context"
	"time"

	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"brokle/internal/core/domain/auth"
)

// AuthInterceptor validates API keys from gRPC metadata
type AuthInterceptor struct {
	apiKeyService auth.APIKeyService
	logger        *slog.Logger
}

// NewAuthInterceptor creates a new gRPC auth interceptor
func NewAuthInterceptor(
	apiKeyService auth.APIKeyService,
	logger *slog.Logger,
) *AuthInterceptor {
	return &AuthInterceptor{
		apiKeyService: apiKeyService,
		logger:        logger,
	}
}

// Unary returns a gRPC unary interceptor for API key authentication
func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Extract API key from gRPC metadata
		apiKey, err := extractAPIKeyFromMetadata(ctx)
		if err != nil {
			i.logger.Error("Failed to extract API key from gRPC metadata",
				"error", err,
				"method", info.FullMethod,
			)
			return nil, status.Error(codes.Unauthenticated, "API key required in metadata (x-api-key or authorization header)")
		}

		// Validate API key and get project data
		keyData, err := i.apiKeyService.ValidateAPIKey(ctx, apiKey)
		if err != nil {
			i.logger.Error("Invalid API key in gRPC request",
				"error", err,
				"method", info.FullMethod,
			)
			return nil, status.Error(codes.Unauthenticated, "invalid API key")
		}

		i.logger.Debug("gRPC API key validated successfully",
			"project_id", keyData.ProjectID.String(),
			"api_key_id", keyData.APIKey.ID.String(),
			"method", info.FullMethod,
		)

		// Store authentication data in context for handler use
		ctx = storeAuthDataInContext(ctx, &keyData.ProjectID, &keyData.APIKey.ID)

		// Call handler with authenticated context
		return handler(ctx, req)
	}
}

// LoggingInterceptor logs gRPC requests with timing and errors
func LoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		// Call handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Log request with structured fields
		if err != nil {
			logger.Error("gRPC request failed",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
				"error", err,
			)
		} else {
			logger.Info("gRPC request completed",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return resp, err
	}
}
