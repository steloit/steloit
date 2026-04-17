package prompt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type versionRepository struct {
	tm *db.TxManager
}

func NewVersionRepository(tm *db.TxManager) promptDomain.VersionRepository {
	return &versionRepository{tm: tm}
}

func (r *versionRepository) Create(ctx context.Context, v *promptDomain.Version) error {
	var cfg json.RawMessage
	if v.Config != nil {
		b, err := json.Marshal(v.Config)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		cfg = b
	}
	if err := r.tm.Queries(ctx).CreatePromptVersion(ctx, gen.CreatePromptVersionParams{
		ID:            v.ID,
		PromptID:      v.PromptID,
		Version:       int32(v.Version),
		Template:      json.RawMessage(v.Template),
		Variables:     db.NonNilStrings(v.Variables),
		CommitMessage: nilIfEmptyPrompt(v.CommitMessage),
		CreatedBy:     v.CreatedBy,
		Config:        cfg,
	}); err != nil {
		return fmt.Errorf("create prompt version: %w", err)
	}
	return nil
}

func (r *versionRepository) GetByID(ctx context.Context, id uuid.UUID) (*promptDomain.Version, error) {
	row, err := r.tm.Queries(ctx).GetPromptVersionByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get version by ID %s: %w", id, promptDomain.ErrVersionNotFound)
		}
		return nil, err
	}
	return versionFromRow(&row)
}

func (r *versionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.tm.Queries(ctx).DeletePromptVersion(ctx, id)
}

func (r *versionRepository) GetByPromptAndVersion(ctx context.Context, promptID uuid.UUID, version int) (*promptDomain.Version, error) {
	row, err := r.tm.Queries(ctx).GetPromptVersionByPromptAndVersion(ctx, gen.GetPromptVersionByPromptAndVersionParams{
		PromptID: promptID,
		Version:  int32(version),
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get version %d: %w", version, promptDomain.ErrVersionNotFound)
		}
		return nil, err
	}
	return versionFromRow(&row)
}

func (r *versionRepository) GetLatestByPrompt(ctx context.Context, promptID uuid.UUID) (*promptDomain.Version, error) {
	row, err := r.tm.Queries(ctx).GetLatestPromptVersion(ctx, promptID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get latest version: %w", promptDomain.ErrVersionNotFound)
		}
		return nil, err
	}
	return versionFromRow(&row)
}

func (r *versionRepository) ListByPrompt(ctx context.Context, promptID uuid.UUID) ([]*promptDomain.Version, error) {
	rows, err := r.tm.Queries(ctx).ListPromptVersions(ctx, promptID)
	if err != nil {
		return nil, err
	}
	return versionsFromRows(rows)
}

// GetNextVersionNumber acquires FOR UPDATE locks on existing versions
// for the prompt, computes MAX(version)+1, and returns it. Safe for
// concurrent callers when invoked inside a transaction: the locks are
// held until commit, serializing concurrent inserts through the
// (prompt_id, version) unique constraint.
func (r *versionRepository) GetNextVersionNumber(ctx context.Context, promptID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).GetNextPromptVersionNumber(ctx, promptID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *versionRepository) CountByPrompt(ctx context.Context, promptID uuid.UUID) (int64, error) {
	return r.tm.Queries(ctx).CountPromptVersions(ctx, promptID)
}

func (r *versionRepository) GetLatestByPrompts(ctx context.Context, promptIDs []uuid.UUID) ([]*promptDomain.Version, error) {
	if len(promptIDs) == 0 {
		return []*promptDomain.Version{}, nil
	}
	rows, err := r.tm.Queries(ctx).GetLatestPromptVersionsForPrompts(ctx, promptIDs)
	if err != nil {
		return nil, err
	}
	return versionsFromRows(rows)
}

func (r *versionRepository) GetByIDs(ctx context.Context, versionIDs []uuid.UUID) ([]*promptDomain.Version, error) {
	if len(versionIDs) == 0 {
		return []*promptDomain.Version{}, nil
	}
	rows, err := r.tm.Queries(ctx).ListPromptVersionsByIDs(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	return versionsFromRows(rows)
}

func versionFromRow(row *gen.PromptVersion) (*promptDomain.Version, error) {
	v := &promptDomain.Version{
		ID:        row.ID,
		PromptID:  row.PromptID,
		Version:   int(row.Version),
		Template:  promptDomain.JSON(row.Template),
		Variables: row.Variables,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt,
	}
	if row.CommitMessage != nil {
		v.CommitMessage = *row.CommitMessage
	}
	if len(row.Config) > 0 {
		v.Config = &promptDomain.ModelConfig{}
		if err := json.Unmarshal(row.Config, v.Config); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	return v, nil
}

func versionsFromRows(rows []gen.PromptVersion) ([]*promptDomain.Version, error) {
	out := make([]*promptDomain.Version, 0, len(rows))
	for i := range rows {
		v, err := versionFromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
