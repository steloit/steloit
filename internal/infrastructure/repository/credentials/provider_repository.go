package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	credentialsDomain "brokle/internal/core/domain/credentials"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	appErrors "brokle/pkg/errors"
)

type providerCredentialRepository struct {
	tm *db.TxManager
}

func NewProviderCredentialRepository(tm *db.TxManager) credentialsDomain.ProviderCredentialRepository {
	return &providerCredentialRepository{tm: tm}
}

func (r *providerCredentialRepository) Create(ctx context.Context, c *credentialsDomain.ProviderCredential) error {
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	cfg, err := marshalConfig(c.Config)
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	if err := r.tm.Queries(ctx).CreateProviderCredential(ctx, gen.CreateProviderCredentialParams{
		ID:             c.ID,
		OrganizationID: c.OrganizationID,
		Name:           c.Name,
		Adapter:        gen.Provider(c.Adapter),
		EncryptedKey:   c.EncryptedKey,
		KeyPreview:     c.KeyPreview,
		BaseUrl:        c.BaseURL,
		Config:         cfg,
		Headers:        c.Headers,
		CustomModels:   c.CustomModels,
		CreatedBy:      c.CreatedBy,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}); err != nil {
		if appErrors.IsUniqueViolation(err) {
			return fmt.Errorf("create credential: %w", credentialsDomain.ErrCredentialExists)
		}
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

func (r *providerCredentialRepository) GetByID(ctx context.Context, id, orgID uuid.UUID) (*credentialsDomain.ProviderCredential, error) {
	row, err := r.tm.Queries(ctx).GetProviderCredentialByID(ctx, gen.GetProviderCredentialByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get credential by ID %s: %w", id, credentialsDomain.ErrCredentialNotFound)
		}
		return nil, fmt.Errorf("get credential by ID: %w", err)
	}
	return credentialFromRow(&row)
}

// GetByOrgAndName returns (nil, nil) when no credential matches —
// preserves the "uniqueness check" contract the service layer relies on.
func (r *providerCredentialRepository) GetByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (*credentialsDomain.ProviderCredential, error) {
	row, err := r.tm.Queries(ctx).GetProviderCredentialByOrgAndName(ctx, gen.GetProviderCredentialByOrgAndNameParams{
		OrganizationID: orgID,
		Name:           name,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get credential by name: %w", err)
	}
	return credentialFromRow(&row)
}

func (r *providerCredentialRepository) GetByOrgAndAdapter(ctx context.Context, orgID uuid.UUID, adapter credentialsDomain.Provider) ([]*credentialsDomain.ProviderCredential, error) {
	rows, err := r.tm.Queries(ctx).ListProviderCredentialsByOrgAndAdapter(ctx, gen.ListProviderCredentialsByOrgAndAdapterParams{
		OrganizationID: orgID,
		Adapter:        gen.Provider(adapter),
	})
	if err != nil {
		return nil, fmt.Errorf("get credentials by adapter: %w", err)
	}
	return credentialsFromRows(rows)
}

func (r *providerCredentialRepository) ListByOrganization(ctx context.Context, orgID uuid.UUID) ([]*credentialsDomain.ProviderCredential, error) {
	rows, err := r.tm.Queries(ctx).ListProviderCredentialsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list credentials for organization %s: %w", orgID, err)
	}
	return credentialsFromRows(rows)
}

func (r *providerCredentialRepository) Update(ctx context.Context, c *credentialsDomain.ProviderCredential, orgID uuid.UUID) error {
	c.UpdatedAt = time.Now()
	cfg, err := marshalConfig(c.Config)
	if err != nil {
		return fmt.Errorf("update credential: %w", err)
	}
	n, err := r.tm.Queries(ctx).UpdateProviderCredential(ctx, gen.UpdateProviderCredentialParams{
		ID:             c.ID,
		OrganizationID: orgID,
		Name:           c.Name,
		EncryptedKey:   c.EncryptedKey,
		KeyPreview:     c.KeyPreview,
		BaseUrl:        c.BaseURL,
		Config:         cfg,
		CustomModels:   c.CustomModels,
		Headers:        c.Headers,
	})
	if err != nil {
		if appErrors.IsUniqueViolation(err) {
			return fmt.Errorf("update credential: %w", credentialsDomain.ErrCredentialExists)
		}
		return fmt.Errorf("update credential: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("update credential: %w", credentialsDomain.ErrCredentialNotFound)
	}
	return nil
}

func (r *providerCredentialRepository) Delete(ctx context.Context, id, orgID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteProviderCredential(ctx, gen.DeleteProviderCredentialParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("delete credential %s: %w", id, credentialsDomain.ErrCredentialNotFound)
	}
	return nil
}

func (r *providerCredentialRepository) ExistsByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (bool, error) {
	ok, err := r.tm.Queries(ctx).ProviderCredentialExistsByOrgAndName(ctx, gen.ProviderCredentialExistsByOrgAndNameParams{
		OrganizationID: orgID,
		Name:           name,
	})
	if err != nil {
		return false, fmt.Errorf("check credential exists: %w", err)
	}
	return ok, nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func credentialFromRow(row *gen.ProviderCredential) (*credentialsDomain.ProviderCredential, error) {
	cfg, err := unmarshalConfig(row.Config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &credentialsDomain.ProviderCredential{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		Adapter:        credentialsDomain.Provider(row.Adapter),
		EncryptedKey:   row.EncryptedKey,
		KeyPreview:     row.KeyPreview,
		BaseURL:        row.BaseUrl,
		Config:         cfg,
		Headers:        row.Headers,
		CustomModels:   row.CustomModels,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

func credentialsFromRows(rows []gen.ProviderCredential) ([]*credentialsDomain.ProviderCredential, error) {
	out := make([]*credentialsDomain.ProviderCredential, 0, len(rows))
	for i := range rows {
		c, err := credentialFromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Config round-trips through JSONB as a map[string]any. Empty maps
// become `{}` (schema has DEFAULT '{}' NOT NULL semantics in practice).
func marshalConfig(m map[string]any) (json.RawMessage, error) {
	if len(m) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return json.Marshal(m)
}

func unmarshalConfig(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

