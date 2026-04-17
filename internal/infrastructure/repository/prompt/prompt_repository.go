package prompt

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type promptRepository struct {
	tm *db.TxManager
}

func NewPromptRepository(tm *db.TxManager) promptDomain.PromptRepository {
	return &promptRepository{tm: tm}
}

func (r *promptRepository) Create(ctx context.Context, p *promptDomain.Prompt) error {
	if err := r.tm.Queries(ctx).CreatePrompt(ctx, gen.CreatePromptParams{
		ID:          p.ID,
		ProjectID:   p.ProjectID,
		Name:        p.Name,
		Description: nilIfEmptyPrompt(p.Description),
		Type:        string(p.Type),
		Tags:        db.NonNilStrings(p.Tags),
	}); err != nil {
		return fmt.Errorf("create prompt: %w", err)
	}
	return nil
}

func (r *promptRepository) GetByID(ctx context.Context, id uuid.UUID) (*promptDomain.Prompt, error) {
	row, err := r.tm.Queries(ctx).GetPromptByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get prompt by ID %s: %w", id, promptDomain.ErrPromptNotFound)
		}
		return nil, err
	}
	return promptFromRow(&row), nil
}

func (r *promptRepository) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*promptDomain.Prompt, error) {
	row, err := r.tm.Queries(ctx).GetPromptByName(ctx, gen.GetPromptByNameParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get prompt by name %s: %w", name, promptDomain.ErrPromptNotFound)
		}
		return nil, err
	}
	return promptFromRow(&row), nil
}

func (r *promptRepository) Update(ctx context.Context, p *promptDomain.Prompt) error {
	p.UpdatedAt = time.Now()
	if err := r.tm.Queries(ctx).UpdatePrompt(ctx, gen.UpdatePromptParams{
		ID:          p.ID,
		Name:        p.Name,
		Description: nilIfEmptyPrompt(p.Description),
		Type:        string(p.Type),
		Tags:        db.NonNilStrings(p.Tags),
	}); err != nil {
		return fmt.Errorf("update prompt: %w", err)
	}
	return nil
}

func (r *promptRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).HardDeletePrompt(ctx, id); err != nil {
		return fmt.Errorf("delete prompt: %w", err)
	}
	return nil
}

func (r *promptRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeletePrompt(ctx, id); err != nil {
		return fmt.Errorf("soft-delete prompt: %w", err)
	}
	return nil
}

func (r *promptRepository) Restore(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).RestorePrompt(ctx, id); err != nil {
		return fmt.Errorf("restore prompt: %w", err)
	}
	return nil
}

func (r *promptRepository) CountByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return r.tm.Queries(ctx).CountPromptsByProject(ctx, projectID)
}

// ListByProject uses squirrel for optional type/tag/search filtering.
// `tags @> $1` matches prompts whose tags array contains all filter tags.
func (r *promptRepository) ListByProject(ctx context.Context, projectID uuid.UUID, filters *promptDomain.PromptFilters) ([]*promptDomain.Prompt, int64, error) {
	base := sq.Select().From("prompts").
		Where(sq.Eq{"project_id": projectID}).
		Where("deleted_at IS NULL")
	if filters != nil {
		if filters.Type != nil {
			base = base.Where(sq.Eq{"type": string(*filters.Type)})
		}
		if len(filters.Tags) > 0 {
			base = base.Where(sq.Expr("tags @> ?", []string(filters.Tags)))
		}
		if filters.Search != nil && *filters.Search != "" {
			p := "%" + *filters.Search + "%"
			base = base.Where(sq.Or{
				sq.Expr("name ILIKE ?", p),
				sq.Expr("description ILIKE ?", p),
			})
		}
	}

	cntSQL, cntArgs, err := base.Columns("COUNT(*)").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, cntSQL, cntArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	list := base.Columns(promptColumns...).OrderBy("created_at DESC")
	if filters != nil && filters.Limit > 0 {
		list = list.Limit(uint64(filters.Limit))
		if filters.Page > 1 {
			list = list.Offset(uint64((filters.Page - 1) * filters.Limit))
		}
	}
	selSQL, selArgs, err := list.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.tm.DB(ctx).Query(ctx, selSQL, selArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*promptDomain.Prompt, 0)
	for rows.Next() {
		p, err := scanPrompt(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

var promptColumns = []string{
	"id", "project_id", "name", "description", "type",
	"tags", "created_at", "updated_at", "deleted_at",
}

func scanPrompt(row interface {
	Scan(dest ...any) error
}) (*promptDomain.Prompt, error) {
	var (
		p           promptDomain.Prompt
		description *string
		typ         string
		tags        []string
		deletedAt   *time.Time
	)
	if err := row.Scan(
		&p.ID, &p.ProjectID, &p.Name, &description, &typ,
		&tags, &p.CreatedAt, &p.UpdatedAt, &deletedAt,
	); err != nil {
		return nil, err
	}
	if description != nil {
		p.Description = *description
	}
	p.Type = promptDomain.PromptType(typ)
	p.Tags = tags
	p.DeletedAt = deletedAt
	return &p, nil
}

func promptFromRow(row *gen.Prompt) *promptDomain.Prompt {
	p := &promptDomain.Prompt{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Name:      row.Name,
		Type:      promptDomain.PromptType(row.Type),
		Tags:      row.Tags,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		DeletedAt: row.DeletedAt,
	}
	if row.Description != nil {
		p.Description = *row.Description
	}
	return p
}

func nilIfEmptyPrompt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
