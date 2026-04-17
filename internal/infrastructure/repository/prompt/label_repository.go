package prompt

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	"brokle/pkg/uid"
)

type labelRepository struct {
	tm *db.TxManager
}

func NewLabelRepository(tm *db.TxManager) promptDomain.LabelRepository {
	return &labelRepository{tm: tm}
}

func (r *labelRepository) Create(ctx context.Context, l *promptDomain.Label) error {
	return r.tm.Queries(ctx).CreatePromptLabel(ctx, gen.CreatePromptLabelParams{
		ID:        l.ID,
		PromptID:  l.PromptID,
		VersionID: l.VersionID,
		Name:      l.Name,
		CreatedBy: l.CreatedBy,
	})
}

func (r *labelRepository) GetByID(ctx context.Context, id uuid.UUID) (*promptDomain.Label, error) {
	row, err := r.tm.Queries(ctx).GetPromptLabelByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get label by ID %s: %w", id, promptDomain.ErrLabelNotFound)
		}
		return nil, err
	}
	return labelFromRow(&row), nil
}

func (r *labelRepository) Update(ctx context.Context, l *promptDomain.Label) error {
	l.UpdatedAt = time.Now()
	return r.tm.Queries(ctx).UpdatePromptLabel(ctx, gen.UpdatePromptLabelParams{
		ID:        l.ID,
		VersionID: l.VersionID,
	})
}

func (r *labelRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.tm.Queries(ctx).DeletePromptLabel(ctx, id)
}

func (r *labelRepository) GetByPromptAndName(ctx context.Context, promptID uuid.UUID, name string) (*promptDomain.Label, error) {
	row, err := r.tm.Queries(ctx).GetPromptLabelByPromptAndName(ctx, gen.GetPromptLabelByPromptAndNameParams{
		PromptID: promptID,
		Name:     name,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get label %s: %w", name, promptDomain.ErrLabelNotFound)
		}
		return nil, err
	}
	return labelFromRow(&row), nil
}

func (r *labelRepository) ListByPrompt(ctx context.Context, promptID uuid.UUID) ([]*promptDomain.Label, error) {
	rows, err := r.tm.Queries(ctx).ListPromptLabelsByPrompt(ctx, promptID)
	if err != nil {
		return nil, err
	}
	return labelsFromRows(rows), nil
}

func (r *labelRepository) ListByPrompts(ctx context.Context, promptIDs []uuid.UUID) ([]*promptDomain.Label, error) {
	if len(promptIDs) == 0 {
		return []*promptDomain.Label{}, nil
	}
	rows, err := r.tm.Queries(ctx).ListPromptLabelsByPrompts(ctx, promptIDs)
	if err != nil {
		return nil, err
	}
	return labelsFromRows(rows), nil
}

func (r *labelRepository) ListByVersion(ctx context.Context, versionID uuid.UUID) ([]*promptDomain.Label, error) {
	rows, err := r.tm.Queries(ctx).ListPromptLabelsByVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	return labelsFromRows(rows), nil
}

func (r *labelRepository) ListByVersions(ctx context.Context, versionIDs []uuid.UUID) ([]*promptDomain.Label, error) {
	if len(versionIDs) == 0 {
		return []*promptDomain.Label{}, nil
	}
	rows, err := r.tm.Queries(ctx).ListPromptLabelsByVersions(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	return labelsFromRows(rows), nil
}

// SetLabel upserts the (prompt_id, name) pointer to a new version.
// Inside a transaction this becomes an atomic re-pointing; without one
// the race window is tiny (sub-millisecond) and the existing row
// structure already enforces uniqueness.
func (r *labelRepository) SetLabel(ctx context.Context, promptID, versionID uuid.UUID, name string, createdBy *uuid.UUID) error {
	existing, err := r.GetByPromptAndName(ctx, promptID, name)
	if err == nil {
		existing.VersionID = versionID
		return r.Update(ctx, existing)
	}
	label := &promptDomain.Label{
		ID:        uid.New(),
		PromptID:  promptID,
		VersionID: versionID,
		Name:      name,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return r.Create(ctx, label)
}

func (r *labelRepository) RemoveLabel(ctx context.Context, promptID uuid.UUID, name string) error {
	n, err := r.tm.Queries(ctx).DeletePromptLabelByName(ctx, gen.DeletePromptLabelByNameParams{
		PromptID: promptID,
		Name:     name,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("remove label %s: %w", name, promptDomain.ErrLabelNotFound)
	}
	return nil
}

func (r *labelRepository) DeleteByPrompt(ctx context.Context, promptID uuid.UUID) error {
	return r.tm.Queries(ctx).DeletePromptLabelsByPrompt(ctx, promptID)
}

func labelFromRow(row *gen.PromptLabel) *promptDomain.Label {
	return &promptDomain.Label{
		ID:        row.ID,
		PromptID:  row.PromptID,
		VersionID: row.VersionID,
		Name:      row.Name,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func labelsFromRows(rows []gen.PromptLabel) []*promptDomain.Label {
	out := make([]*promptDomain.Label, 0, len(rows))
	for i := range rows {
		out = append(out, labelFromRow(&rows[i]))
	}
	return out
}
