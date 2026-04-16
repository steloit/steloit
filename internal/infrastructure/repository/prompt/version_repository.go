package prompt

import (
	"context"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/infrastructure/shared"
)

// versionRepository implements promptDomain.VersionRepository using GORM
type versionRepository struct {
	db *gorm.DB
}

// NewVersionRepository creates a new version repository instance
func NewVersionRepository(db *gorm.DB) promptDomain.VersionRepository {
	return &versionRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *versionRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new version
func (r *versionRepository) Create(ctx context.Context, version *promptDomain.Version) error {
	return r.getDB(ctx).WithContext(ctx).Create(version).Error
}

// GetByID retrieves a version by ID
func (r *versionRepository) GetByID(ctx context.Context, id uuid.UUID) (*promptDomain.Version, error) {
	var version promptDomain.Version
	err := r.getDB(ctx).WithContext(ctx).
		Where("id = ?", id).
		First(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get version by ID %s: %w", id, promptDomain.ErrVersionNotFound)
		}
		return nil, err
	}
	return &version, nil
}

// Delete deletes a version (versions should generally not be deleted)
func (r *versionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Where("id = ?", id).Delete(&promptDomain.Version{}).Error
}

// GetByPromptAndVersion retrieves a specific version of a prompt
func (r *versionRepository) GetByPromptAndVersion(ctx context.Context, promptID uuid.UUID, version int) (*promptDomain.Version, error) {
	var v promptDomain.Version
	err := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ? AND version = ?", promptID, version).
		First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get version %d: %w", version, promptDomain.ErrVersionNotFound)
		}
		return nil, err
	}
	return &v, nil
}

// GetLatestByPrompt retrieves the latest version of a prompt
func (r *versionRepository) GetLatestByPrompt(ctx context.Context, promptID uuid.UUID) (*promptDomain.Version, error) {
	var version promptDomain.Version
	err := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ?", promptID).
		Order("version DESC").
		First(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get latest version: %w", promptDomain.ErrVersionNotFound)
		}
		return nil, err
	}
	return &version, nil
}

// ListByPrompt retrieves all versions of a prompt
func (r *versionRepository) ListByPrompt(ctx context.Context, promptID uuid.UUID) ([]*promptDomain.Version, error) {
	var versions []*promptDomain.Version
	err := r.getDB(ctx).WithContext(ctx).
		Where("prompt_id = ?", promptID).
		Order("version DESC").
		Find(&versions).Error
	return versions, err
}

// GetNextVersionNumber atomically gets the next version number for a prompt.
// Uses FOR UPDATE locking to prevent race conditions when called within a transaction.
func (r *versionRepository) GetNextVersionNumber(ctx context.Context, promptID uuid.UUID) (int, error) {
	var nextVersion int
	// Subquery pattern: PostgreSQL disallows FOR UPDATE with aggregate functions
	err := r.getDB(ctx).WithContext(ctx).
		Raw(`
			SELECT COALESCE(MAX(version), 0) + 1
			FROM (
				SELECT version
				FROM prompt_versions
				WHERE prompt_id = ?
				FOR UPDATE
			) locked_rows
		`, promptID).
		Scan(&nextVersion).Error
	if err != nil {
		return 0, err
	}
	return nextVersion, nil
}

// CountByPrompt counts versions for a prompt
func (r *versionRepository) CountByPrompt(ctx context.Context, promptID uuid.UUID) (int64, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&promptDomain.Version{}).
		Where("prompt_id = ?", promptID).
		Count(&count).Error
	return count, err
}

// GetLatestByPrompts retrieves the latest version for multiple prompts in a single query
// This is a batch operation to avoid N+1 query problems
func (r *versionRepository) GetLatestByPrompts(ctx context.Context, promptIDs []uuid.UUID) ([]*promptDomain.Version, error) {
	if len(promptIDs) == 0 {
		return []*promptDomain.Version{}, nil
	}

	// pq.Array required: GORM Raw() doesn't expand slices for PostgreSQL arrays
	ids := make([]string, len(promptIDs))
	for i, id := range promptIDs {
		ids[i] = id.String()
	}

	var versions []*promptDomain.Version
	err := r.getDB(ctx).WithContext(ctx).
		Raw(`
			SELECT DISTINCT ON (prompt_id) *
			FROM prompt_versions
			WHERE prompt_id = ANY(?)
			ORDER BY prompt_id, version DESC
		`, pq.Array(ids)).
		Scan(&versions).Error

	if err != nil {
		return nil, err
	}

	return versions, nil
}

// GetByIDs retrieves multiple versions by their IDs in a single query
// This is a batch operation to avoid N+1 query problems
func (r *versionRepository) GetByIDs(ctx context.Context, versionIDs []uuid.UUID) ([]*promptDomain.Version, error) {
	if len(versionIDs) == 0 {
		return []*promptDomain.Version{}, nil
	}

	var versions []*promptDomain.Version
	err := r.getDB(ctx).WithContext(ctx).
		Where("id IN ?", versionIDs).
		Find(&versions).Error

	if err != nil {
		return nil, err
	}

	return versions, nil
}
