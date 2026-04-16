package credentials

import (
	"context"

	"github.com/google/uuid"
)

// ProviderCredentialRepository defines the repository interface for provider credentials.
type ProviderCredentialRepository interface {
	// Create creates a new provider credential.
	// Returns ErrCredentialExists if a credential with the same name already exists for this organization.
	Create(ctx context.Context, credential *ProviderCredential) error

	// GetByID retrieves a credential by its ID within a specific organization.
	// Returns ErrCredentialNotFound if not found or belongs to different organization.
	GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*ProviderCredential, error)

	// GetByOrgAndName retrieves the credential for a specific organization and name.
	// Returns nil if not found.
	GetByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (*ProviderCredential, error)

	// GetByOrgAndAdapter retrieves all credentials for a specific organization and adapter type.
	// Returns empty slice if none found.
	GetByOrgAndAdapter(ctx context.Context, orgID uuid.UUID, adapter Provider) ([]*ProviderCredential, error)

	// ListByOrganization retrieves all credentials for an organization.
	// Returns empty slice if no credentials configured.
	ListByOrganization(ctx context.Context, orgID uuid.UUID) ([]*ProviderCredential, error)

	// Update updates an existing credential within a specific organization.
	// The ID field must be set. Returns ErrCredentialNotFound if not found or belongs to different organization.
	Update(ctx context.Context, credential *ProviderCredential, orgID uuid.UUID) error

	// Delete removes a credential by ID within a specific organization.
	// Returns ErrCredentialNotFound if the credential doesn't exist or belongs to different organization.
	Delete(ctx context.Context, id uuid.UUID, orgID uuid.UUID) error

	// ExistsByOrgAndName checks if a credential exists for an organization/name combination.
	ExistsByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (bool, error)
}
