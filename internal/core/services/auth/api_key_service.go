package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	orgDomain "brokle/internal/core/domain/organization"
	appErrors "brokle/pkg/errors"
)

// apiKeyService implements the authDomain.APIKeyService interface
type apiKeyService struct {
	apiKeyRepo             authDomain.APIKeyRepository
	organizationMemberRepo authDomain.OrganizationMemberRepository
	projectRepo            orgDomain.ProjectRepository
}

// NewAPIKeyService creates a new API key service instance
func NewAPIKeyService(
	apiKeyRepo authDomain.APIKeyRepository,
	organizationMemberRepo authDomain.OrganizationMemberRepository,
	projectRepo orgDomain.ProjectRepository,
) authDomain.APIKeyService {
	return &apiKeyService{
		apiKeyRepo:             apiKeyRepo,
		organizationMemberRepo: organizationMemberRepo,
		projectRepo:            projectRepo,
	}
}

// CreateAPIKey creates a new industry-standard API key with pure random secret
func (s *apiKeyService) CreateAPIKey(ctx context.Context, userID uuid.UUID, req *authDomain.CreateAPIKeyRequest) (*authDomain.CreateAPIKeyResponse, error) {
	// TODO: Validate user has permission to create keys in the project
	// For now, skip membership validation - will be implemented when organization service is ready

	// Generate industry-standard pure random API key (bk_{40_char_random})
	fullKey, err := authDomain.GenerateAPIKey()
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to generate API key", err)
	}

	// Hash the full key for secure storage using SHA-256 (industry standard for API keys)
	// Note: SHA-256 is deterministic (same input = same output), enabling O(1) lookup
	// This is different from bcrypt (used for passwords) which is non-deterministic
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Create key preview for display (bk_...xyz)
	keyPreview := authDomain.CreateKeyPreview(fullKey)

	// Create API key entity (project_id stored in database, not in key)
	apiKeyEntity := authDomain.NewAPIKey(
		userID,
		req.ProjectID,
		req.Name,
		keyHash, // SHA-256 hash of full key (deterministic, enables O(1) lookup)
		keyPreview,
		req.ExpiresAt,
	)

	// Save to database
	if err := s.apiKeyRepo.Create(ctx, apiKeyEntity); err != nil {
		return nil, appErrors.NewInternalError("Failed to save API key", err)
	}

	// Return response with the full key (only shown once)
	return &authDomain.CreateAPIKeyResponse{
		ID:         apiKeyEntity.ID.String(),
		Name:       apiKeyEntity.Name,
		Key:        fullKey, // Full key - only returned once
		KeyPreview: apiKeyEntity.KeyPreview,
		ProjectID:  apiKeyEntity.ProjectID.String(),
		CreatedAt:  apiKeyEntity.CreatedAt,
		ExpiresAt:  apiKeyEntity.ExpiresAt,
	}, nil
}

// GetAPIKey retrieves an API key by ID
func (s *apiKeyService) GetAPIKey(ctx context.Context, keyID uuid.UUID) (*authDomain.APIKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}
	return apiKey, nil
}

// GetAPIKeys retrieves API keys based on filters
func (s *apiKeyService) GetAPIKeys(ctx context.Context, filters *authDomain.APIKeyFilters) ([]*authDomain.APIKey, error) {
	// Use existing repository methods based on filters
	if filters.ProjectID != nil {
		return s.apiKeyRepo.GetByProjectID(ctx, *filters.ProjectID)
	}
	if filters.OrganizationID != nil {
		return s.apiKeyRepo.GetByOrganizationID(ctx, *filters.OrganizationID)
	}
	if filters.UserID != nil {
		return s.apiKeyRepo.GetByUserID(ctx, *filters.UserID)
	}

	// Use GetByFilters for comprehensive filtering with pagination
	return s.apiKeyRepo.GetByFilters(ctx, filters)
}

// CountAPIKeys returns the total count of API keys matching the filters
func (s *apiKeyService) CountAPIKeys(ctx context.Context, filters *authDomain.APIKeyFilters) (int64, error) {
	return s.apiKeyRepo.CountByFilters(ctx, filters)
}

// DeleteAPIKey deletes (soft deletes) an API key with project ownership verification
func (s *apiKeyService) DeleteAPIKey(ctx context.Context, keyID uuid.UUID, projectID uuid.UUID) error {
	// Verify API key exists (filters out already-deleted keys)
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		if errors.Is(err, authDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("API key not found")
		}
		return appErrors.NewInternalError("Failed to get API key", err)
	}

	// Verify API key belongs to specified project (security check)
	if apiKey.ProjectID != projectID {
		return appErrors.NewNotFoundError("API key not found in this project")
	}

	// Perform soft delete
	if err := s.apiKeyRepo.Delete(ctx, keyID); err != nil {
		return appErrors.NewInternalError("Failed to delete API key", err)
	}

	return nil
}

// ValidateAPIKey validates an industry-standard API key using direct SHA-256 hash lookup
// This is O(1) with unique index on key_hash column (GitHub/Stripe pattern)
func (s *apiKeyService) ValidateAPIKey(ctx context.Context, fullKey string) (*authDomain.ValidateAPIKeyResponse, error) {
	// Validate API key format (bk_{40_chars})
	if err := authDomain.ValidateAPIKeyFormat(fullKey); err != nil {
		return nil, appErrors.NewUnauthorizedError("Invalid API key format")
	}

	// Hash the incoming key using SHA-256 for O(1) lookup
	// SHA-256 is deterministic (same input = same hash), enabling direct database lookup
	// This is the industry standard for API keys (GitHub, Stripe, OpenAI all use this)
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Direct lookup by hash (O(1) with unique index on key_hash)
	apiKey, err := s.apiKeyRepo.GetByKeyHash(ctx, keyHash)
	if err != nil {
		// Distinguish between not-found (401) and infrastructure errors (500)
		if errors.Is(err, authDomain.ErrNotFound) {
			// Don't expose whether key exists or not (security best practice)
			return nil, appErrors.NewUnauthorizedError("Invalid API key")
		}
		// Infrastructure error (DB connection, migration issue, etc.) - return 500
		return nil, appErrors.NewInternalError("Failed to validate API key", err)
	}

	// Check expiration (deleted keys filtered by GORM soft delete)
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, appErrors.NewUnauthorizedError("API key has expired")
	}

	// Create auth context
	authContext := &authDomain.AuthContext{
		UserID:   apiKey.UserID,
		APIKeyID: &apiKey.ID,
	}

	// Update last used timestamp (async, don't block validation)
	go func() {
		ctx := context.Background()
		if err := s.apiKeyRepo.UpdateLastUsed(ctx, apiKey.ID); err != nil {
			// Log error but don't fail validation
			// TODO: Add proper logging when logger is available
		}
	}()

	// Look up project to get OrganizationID for billing aggregation
	project, err := s.projectRepo.GetByID(ctx, apiKey.ProjectID)
	if err != nil {
		// Project lookup failure is critical - shouldn't happen for valid API keys
		return nil, appErrors.NewInternalError("Failed to look up project for API key", err)
	}

	// Return validation response with project_id and organization_id from database
	return &authDomain.ValidateAPIKeyResponse{
		APIKey:         apiKey,
		ProjectID:      apiKey.ProjectID,       // Retrieved from database, not extracted from key
		OrganizationID: project.OrganizationID, // From project lookup for billing aggregation
		Valid:          true,
		AuthContext:    authContext,
	}, nil
}

// CheckRateLimit checks if the API key has exceeded rate limits
func (s *apiKeyService) CheckRateLimit(ctx context.Context, keyID uuid.UUID) (bool, error) {
	// TODO: Implement rate limiting logic with Redis
	// For now, always allow requests
	return true, nil
}

// GetAPIKeyContext creates an AuthContext from an API key
func (s *apiKeyService) GetAPIKeyContext(ctx context.Context, keyID uuid.UUID) (*authDomain.AuthContext, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	return &authDomain.AuthContext{
		UserID:   apiKey.UserID,
		APIKeyID: &apiKey.ID,
	}, nil
}

// CanAPIKeyAccessResource checks if an API key can access a specific resource
// Note: All non-deleted, non-expired API keys have full access to their project
// Access control should be handled at the organization RBAC level
func (s *apiKeyService) CanAPIKeyAccessResource(ctx context.Context, keyID uuid.UUID, resource string) (bool, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return false, fmt.Errorf("get API key: %w", err)
	}

	// All API keys have full access to their project (deleted keys filtered by GORM)
	// Fine-grained permissions handled by organization RBAC
	return !apiKey.IsExpired(), nil
}

// Scoped access methods
func (s *apiKeyService) GetAPIKeysByUser(ctx context.Context, userID uuid.UUID) ([]*authDomain.APIKey, error) {
	return s.apiKeyRepo.GetByUserID(ctx, userID)
}

func (s *apiKeyService) GetAPIKeysByOrganization(ctx context.Context, orgID uuid.UUID) ([]*authDomain.APIKey, error) {
	return s.apiKeyRepo.GetByOrganizationID(ctx, orgID)
}

func (s *apiKeyService) GetAPIKeysByProject(ctx context.Context, projectID uuid.UUID) ([]*authDomain.APIKey, error) {
	return s.apiKeyRepo.GetByProjectID(ctx, projectID)
}
