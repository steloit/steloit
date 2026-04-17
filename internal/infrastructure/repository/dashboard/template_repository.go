package dashboard

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	dashboardDomain "brokle/internal/core/domain/dashboard"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// templateRepository is the pgx+sqlc implementation of
// dashboardDomain.TemplateRepository. List uses squirrel because the
// is_active filter has a default ("active only when unspecified")
// branch that doesn't compose cleanly as a sqlc parameter.
type templateRepository struct {
	tm *db.TxManager
}

func NewTemplateRepository(tm *db.TxManager) dashboardDomain.TemplateRepository {
	return &templateRepository{tm: tm}
}

func (r *templateRepository) List(ctx context.Context, filter *dashboardDomain.TemplateFilter) ([]*dashboardDomain.Template, error) {
	b := sq.Select(
		"id", "name", "description", "category",
		"config", "layout", "is_active",
		"created_at", "updated_at",
	).From("dashboard_templates").OrderBy("name ASC")

	// Default visibility rule: active only unless the caller says otherwise.
	if filter == nil || filter.IsActive == nil {
		b = b.Where(sq.Eq{"is_active": true})
	} else {
		b = b.Where(sq.Eq{"is_active": *filter.IsActive})
	}
	if filter != nil && filter.Category != nil {
		b = b.Where(sq.Eq{"category": string(*filter.Category)})
	}

	sqlStr, args, err := b.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build template list query: %w", err)
	}
	rows, err := r.tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()
	out := make([]*dashboardDomain.Template, 0)
	for rows.Next() {
		t := &dashboardDomain.Template{}
		var (
			description *string
			cfgRaw      json.RawMessage
			layoutRaw   json.RawMessage
		)
		if err := rows.Scan(
			&t.ID, &t.Name, &description, &t.Category,
			&cfgRaw, &layoutRaw, &t.IsActive,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan template row: %w", err)
		}
		if description != nil {
			t.Description = *description
		}
		if err := unmarshalDashboardContent(cfgRaw, layoutRaw, &t.Config, &t.Layout); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *templateRepository) GetByID(ctx context.Context, id uuid.UUID) (*dashboardDomain.Template, error) {
	row, err := r.tm.Queries(ctx).GetDashboardTemplateByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get template by ID %s: %w", id, dashboardDomain.ErrTemplateNotFound)
		}
		return nil, fmt.Errorf("get template by ID %s: %w", id, err)
	}
	return templateFromRow(&row)
}

func (r *templateRepository) GetByName(ctx context.Context, name string) (*dashboardDomain.Template, error) {
	row, err := r.tm.Queries(ctx).GetDashboardTemplateByName(ctx, name)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get template by name %s: %w", name, dashboardDomain.ErrTemplateNotFound)
		}
		return nil, fmt.Errorf("get template by name %s: %w", name, err)
	}
	return templateFromRow(&row)
}

func (r *templateRepository) GetByCategory(ctx context.Context, category dashboardDomain.TemplateCategory) (*dashboardDomain.Template, error) {
	row, err := r.tm.Queries(ctx).GetActiveDashboardTemplateByCategory(ctx, string(category))
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get template by category %s: %w", category, dashboardDomain.ErrTemplateNotFound)
		}
		return nil, fmt.Errorf("get template by category %s: %w", category, err)
	}
	return templateFromRow(&row)
}

func (r *templateRepository) Create(ctx context.Context, t *dashboardDomain.Template) error {
	cfg, layout, err := marshalDashboardContent(&t.Config, t.Layout)
	if err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	if err := r.tm.Queries(ctx).CreateDashboardTemplate(ctx, gen.CreateDashboardTemplateParams{
		ID:          t.ID,
		Name:        t.Name,
		Description: nilIfEmptyDash(t.Description),
		Category:    string(t.Category),
		Config:      cfg,
		Layout:      layout,
		IsActive:    t.IsActive,
	}); err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	return nil
}

func (r *templateRepository) Update(ctx context.Context, t *dashboardDomain.Template) error {
	cfg, layout, err := marshalDashboardContent(&t.Config, t.Layout)
	if err != nil {
		return fmt.Errorf("update template: %w", err)
	}
	if err := r.tm.Queries(ctx).UpdateDashboardTemplate(ctx, gen.UpdateDashboardTemplateParams{
		ID:          t.ID,
		Name:        t.Name,
		Description: nilIfEmptyDash(t.Description),
		Category:    string(t.Category),
		Config:      cfg,
		Layout:      layout,
		IsActive:    t.IsActive,
	}); err != nil {
		return fmt.Errorf("update template: %w", err)
	}
	return nil
}

func (r *templateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteDashboardTemplate(ctx, id)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	if n == 0 {
		return dashboardDomain.ErrTemplateNotFound
	}
	return nil
}

func (r *templateRepository) Upsert(ctx context.Context, t *dashboardDomain.Template) error {
	cfg, layout, err := marshalDashboardContent(&t.Config, t.Layout)
	if err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}
	if err := r.tm.Queries(ctx).UpsertDashboardTemplateByName(ctx, gen.UpsertDashboardTemplateByNameParams{
		ID:          t.ID,
		Name:        t.Name,
		Description: nilIfEmptyDash(t.Description),
		Category:    string(t.Category),
		Config:      cfg,
		Layout:      layout,
		IsActive:    t.IsActive,
	}); err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}
	return nil
}

func templateFromRow(row *gen.DashboardTemplate) (*dashboardDomain.Template, error) {
	t := &dashboardDomain.Template{
		ID:        row.ID,
		Name:      row.Name,
		Category:  dashboardDomain.TemplateCategory(row.Category),
		IsActive:  row.IsActive,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.Description != nil {
		t.Description = *row.Description
	}
	if err := unmarshalDashboardContent(row.Config, row.Layout, &t.Config, &t.Layout); err != nil {
		return nil, err
	}
	return t, nil
}
