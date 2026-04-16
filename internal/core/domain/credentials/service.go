package credentials

import (
	"context"

	"github.com/google/uuid"
)

type CreateCredentialRequest struct {
	// OrganizationID is set from the URL path parameter
	OrganizationID uuid.UUID `json:"-"`

	// Name is the user-defined unique identifier for this configuration
	// e.g., "OpenAI Production", "Claude Development"
	Name string `json:"name" validate:"required,min=1,max=100"`

	// Adapter type (openai, anthropic, azure, gemini, openrouter, custom)
	Adapter Provider `json:"adapter" validate:"required"`

	// APIKey is the plaintext API key (only sent during create/update, never returned)
	APIKey string `json:"api_key" validate:"required,min=10"`

	// BaseURL is an optional custom endpoint (Azure OpenAI, proxy, etc.)
	// Required for Azure and Custom providers
	BaseURL *string `json:"base_url,omitempty" validate:"omitempty,url"`

	// Config is provider-specific configuration
	// Azure: {"deployment_id": "...", "api_version": "..."}
	// Gemini: {"location": "..."}
	Config map[string]any `json:"config,omitempty"`

	// CustomModels are user-defined model IDs
	// For standard providers: optional fine-tuned models (e.g., "ft:gpt-4o:my-org")
	// For custom provider: required list of available models (e.g., "llama-3.1", "mistral-7b")
	CustomModels []string `json:"custom_models,omitempty"`

	// Headers are custom HTTP headers (encrypted at rest, never returned)
	// Used for proxy authentication or custom endpoints
	Headers map[string]string `json:"headers,omitempty"`

	// CreatedBy is set from the auth context
	CreatedBy *uuid.UUID `json:"-"`
}

type UpdateCredentialRequest struct {
	// Name can be updated (unique within organization)
	Name *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`

	// APIKey is optional during update (only update if provided)
	APIKey *string `json:"api_key,omitempty" validate:"omitempty,min=10"`

	// BaseURL is an optional custom endpoint
	BaseURL *string `json:"base_url,omitempty" validate:"omitempty,url"`

	// Config is provider-specific configuration
	Config map[string]any `json:"config,omitempty"`

	// CustomModels are user-defined model IDs
	CustomModels []string `json:"custom_models,omitempty"`

	// Headers are custom HTTP headers
	// Pointer type allows distinguishing between:
	// - nil: don't change headers (omitted from request)
	// - empty map: clear headers (explicitly set to {})
	// - non-empty map: set new headers
	Headers *map[string]string `json:"headers,omitempty"`
}

type TestConnectionRequest struct {
	Adapter Provider          `json:"adapter" validate:"required"`
	APIKey  string            `json:"api_key" validate:"required"`
	BaseURL *string           `json:"base_url,omitempty"`
	Config  map[string]any    `json:"config,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type TestConnectionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type ProviderCredentialService interface {
	// Create creates a new credential for the organization.
	// Returns error if a credential with the same name already exists.
	Create(ctx context.Context, req *CreateCredentialRequest) (*ProviderCredentialResponse, error)

	// Update updates an existing credential by ID within a specific organization.
	// Returns error if the credential doesn't exist or belongs to different organization.
	Update(ctx context.Context, id uuid.UUID, orgID uuid.UUID, req *UpdateCredentialRequest) (*ProviderCredentialResponse, error)

	// GetByID retrieves a credential by ID within a specific organization.
	// Returns the safe response (no encrypted data, only masked key preview).
	// Returns error if the credential doesn't exist or belongs to different organization.
	GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*ProviderCredentialResponse, error)

	// GetByName retrieves a credential by organization and name.
	// Returns the safe response (no encrypted data, only masked key preview).
	GetByName(ctx context.Context, orgID uuid.UUID, name string) (*ProviderCredentialResponse, error)

	// List retrieves all credentials for an organization.
	// Returns safe responses (no encrypted data).
	List(ctx context.Context, orgID uuid.UUID) ([]*ProviderCredentialResponse, error)

	// Delete removes a credential by ID within a specific organization.
	// Returns error if the credential doesn't exist or belongs to different organization.
	Delete(ctx context.Context, id uuid.UUID, orgID uuid.UUID) error

	// GetDecryptedByID retrieves the decrypted key configuration by credential ID within a specific organization.
	// This is ONLY for internal use during prompt execution.
	// Returns ErrCredentialNotFound if no credential exists or belongs to different organization.
	GetDecryptedByID(ctx context.Context, credentialID uuid.UUID, orgID uuid.UUID) (*DecryptedKeyConfig, error)

	// GetExecutionConfig returns the key configuration for AI execution.
	// Requires credential_id and validates that the credential's adapter matches.
	// Returns ErrAdapterMismatch if the credential's adapter doesn't match the expected adapter.
	// Returns ErrCredentialNotFound if the credential doesn't exist.
	GetExecutionConfig(ctx context.Context, orgID uuid.UUID, credentialID uuid.UUID, adapter Provider) (*DecryptedKeyConfig, error)

	// ValidateKey validates an API key with the provider without storing it.
	// Makes a lightweight API call to verify the key works.
	ValidateKey(ctx context.Context, adapter Provider, apiKey string, baseURL *string, config map[string]any) error

	// TestConnection tests a provider configuration without saving.
	// Returns a response indicating success or failure with error message.
	TestConnection(ctx context.Context, req *TestConnectionRequest) *TestConnectionResponse
}
