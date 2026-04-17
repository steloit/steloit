package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// apiKeyRepository is the pgx+sqlc implementation of
// authDomain.APIKeyRepository. Dynamic filter reads live alongside in
// api_key_filter.go (squirrel).
type apiKeyRepository struct {
	tm *db.TxManager
}

// NewAPIKeyRepository returns the pgx-backed repository.
func NewAPIKeyRepository(tm *db.TxManager) authDomain.APIKeyRepository {
	return &apiKeyRepository{tm: tm}
}

func (r *apiKeyRepository) Create(ctx context.Context, key *authDomain.APIKey) error {
	if err := r.tm.Queries(ctx).CreateAPIKey(ctx, gen.CreateAPIKeyParams{
		ID:         key.ID,
		UserID:     key.UserID,
		ProjectID:  key.ProjectID,
		Name:       key.Name,
		KeyHash:    key.KeyHash,
		KeyPreview: key.KeyPreview,
		ExpiresAt:  key.ExpiresAt,
		LastUsedAt: key.LastUsedAt,
		CreatedAt:  key.CreatedAt,
		UpdatedAt:  key.UpdatedAt,
		DeletedAt:  key.DeletedAt,
	}); err != nil {
		return fmt.Errorf("create api_key: %w", err)
	}
	return nil
}

func (r *apiKeyRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.APIKey, error) {
	row, err := r.tm.Queries(ctx).GetAPIKeyByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get api_key by ID %s: %w", id, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get api_key by ID %s: %w", id, err)
	}
	return apiKeyFromRow(&row), nil
}

func (r *apiKeyRepository) GetByKeyHash(ctx context.Context, keyHash string) (*authDomain.APIKey, error) {
	row, err := r.tm.Queries(ctx).GetAPIKeyByKeyHash(ctx, keyHash)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get api_key by hash: %w", authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get api_key by hash: %w", err)
	}
	return apiKeyFromRow(&row), nil
}

func (r *apiKeyRepository) Update(ctx context.Context, key *authDomain.APIKey) error {
	if err := r.tm.Queries(ctx).UpdateAPIKey(ctx, gen.UpdateAPIKeyParams{
		ID:         key.ID,
		Name:       key.Name,
		KeyHash:    key.KeyHash,
		KeyPreview: key.KeyPreview,
		ExpiresAt:  key.ExpiresAt,
		LastUsedAt: key.LastUsedAt,
	}); err != nil {
		return fmt.Errorf("update api_key %s: %w", key.ID, err)
	}
	return nil
}

func (r *apiKeyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteAPIKey(ctx, id); err != nil {
		return fmt.Errorf("soft-delete api_key %s: %w", id, err)
	}
	return nil
}

func (r *apiKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).UpdateAPIKeyLastUsed(ctx, id); err != nil {
		return fmt.Errorf("update api_key %s last_used_at: %w", id, err)
	}
	return nil
}

func (r *apiKeyRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.APIKey, error) {
	rows, err := r.tm.Queries(ctx).ListAPIKeysByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys for user %s: %w", userID, err)
	}
	return apiKeysFromRows(rows), nil
}

func (r *apiKeyRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*authDomain.APIKey, error) {
	rows, err := r.tm.Queries(ctx).ListAPIKeysByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys for org %s: %w", orgID, err)
	}
	return apiKeysFromRows(rows), nil
}

func (r *apiKeyRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID) ([]*authDomain.APIKey, error) {
	rows, err := r.tm.Queries(ctx).ListAPIKeysByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys for project %s: %w", projectID, err)
	}
	return apiKeysFromRows(rows), nil
}

func (r *apiKeyRepository) CleanupExpiredAPIKeys(ctx context.Context) error {
	if _, err := r.tm.Queries(ctx).CleanupExpiredAPIKeys(ctx); err != nil {
		return fmt.Errorf("cleanup expired api_keys: %w", err)
	}
	return nil
}

func (r *apiKeyRepository) GetAPIKeyCount(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountAPIKeysByUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count api_keys for user %s: %w", userID, err)
	}
	return int(n), nil
}

func (r *apiKeyRepository) GetActiveAPIKeyCount(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountActiveAPIKeysByUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count active api_keys for user %s: %w", userID, err)
	}
	return int(n), nil
}

// ----- gen ↔ domain boundary -----------------------------------------

// apiKeyFromRow adapts a sqlc-generated row to the domain type.
func apiKeyFromRow(row *gen.ApiKey) *authDomain.APIKey {
	return &authDomain.APIKey{
		ID:         row.ID,
		UserID:     row.UserID,
		ProjectID:  row.ProjectID,
		Name:       row.Name,
		KeyHash:    row.KeyHash,
		KeyPreview: row.KeyPreview,
		ExpiresAt:  row.ExpiresAt,
		LastUsedAt: row.LastUsedAt,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
		DeletedAt:  row.DeletedAt,
	}
}

func apiKeysFromRows(rows []gen.ApiKey) []*authDomain.APIKey {
	out := make([]*authDomain.APIKey, 0, len(rows))
	for i := range rows {
		out = append(out, apiKeyFromRow(&rows[i]))
	}
	return out
}
