package observability

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	observabilityDomain "brokle/internal/core/domain/observability"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type filterPresetRepository struct {
	tm *db.TxManager
}

func NewFilterPresetRepository(tm *db.TxManager) observabilityDomain.FilterPresetRepository {
	return &filterPresetRepository{tm: tm}
}

func (r *filterPresetRepository) Create(ctx context.Context, p *observabilityDomain.FilterPreset) error {
	if err := r.tm.Queries(ctx).CreateFilterPreset(ctx, gen.CreateFilterPresetParams{
		ID:               p.ID,
		ProjectID:        p.ProjectID,
		Name:             p.Name,
		Description:      p.Description,
		TableName:        p.TargetTable,
		Filters:          p.Filters,
		ColumnOrder:      db.JSONOr(p.ColumnOrder, "[]"),
		ColumnVisibility: db.JSONOr(p.ColumnVisibility, "{}"),
		SearchQuery:      p.SearchQuery,
		SearchTypes:      db.NonNilStrings(p.SearchTypes),
		IsPublic:         p.IsPublic,
		CreatedBy:        p.CreatedBy,
	}); err != nil {
		return fmt.Errorf("create filter preset: %w", err)
	}
	return nil
}

func (r *filterPresetRepository) GetByID(ctx context.Context, id uuid.UUID) (*observabilityDomain.FilterPreset, error) {
	row, err := r.tm.Queries(ctx).GetFilterPresetByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, observabilityDomain.ErrFilterPresetNotFound
		}
		return nil, fmt.Errorf("get filter preset by id: %w", err)
	}
	return filterPresetFromRow(&row), nil
}

func (r *filterPresetRepository) Update(ctx context.Context, p *observabilityDomain.FilterPreset) error {
	n, err := r.tm.Queries(ctx).UpdateFilterPreset(ctx, gen.UpdateFilterPresetParams{
		ID:               p.ID,
		Name:             p.Name,
		Description:      p.Description,
		Filters:          p.Filters,
		ColumnOrder:      db.JSONOr(p.ColumnOrder, "[]"),
		ColumnVisibility: db.JSONOr(p.ColumnVisibility, "{}"),
		SearchQuery:      p.SearchQuery,
		SearchTypes:      db.NonNilStrings(p.SearchTypes),
		IsPublic:         p.IsPublic,
	})
	if err != nil {
		return fmt.Errorf("update filter preset: %w", err)
	}
	if n == 0 {
		return observabilityDomain.ErrFilterPresetNotFound
	}
	return nil
}

func (r *filterPresetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteFilterPreset(ctx, id)
	if err != nil {
		return fmt.Errorf("delete filter preset: %w", err)
	}
	if n == 0 {
		return observabilityDomain.ErrFilterPresetNotFound
	}
	return nil
}

// List is built with squirrel — visibility filtering has 4 branches
// (owned / owned+public / by created_by / by is_public) that don't
// compose cleanly as sqlc parameters.
func (r *filterPresetRepository) List(ctx context.Context, f *observabilityDomain.FilterPresetFilter) ([]*observabilityDomain.FilterPreset, error) {
	b := r.buildList(f).
		Columns(
			"id", "project_id", "name", "description",
			"table_name", "filters", "column_order", "column_visibility",
			"search_query", "search_types",
			"is_public", "created_by", "created_at", "updated_at",
		).
		OrderBy("updated_at DESC")
	if f.Limit > 0 {
		b = b.Limit(uint64(f.Limit))
	}
	if f.Offset > 0 {
		b = b.Offset(uint64(f.Offset))
	}
	sqlStr, args, err := b.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build filter preset list: %w", err)
	}
	rows, err := r.tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list filter presets: %w", err)
	}
	defer rows.Close()
	out := make([]*observabilityDomain.FilterPreset, 0)
	for rows.Next() {
		p := &observabilityDomain.FilterPreset{}
		var (
			description, searchQuery *string
			columnOrder, columnVis   []byte
			searchTypes              []string
		)
		if err := rows.Scan(
			&p.ID, &p.ProjectID, &p.Name, &description,
			&p.TargetTable, &p.Filters, &columnOrder, &columnVis,
			&searchQuery, &searchTypes,
			&p.IsPublic, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan filter preset row: %w", err)
		}
		p.Description = description
		p.ColumnOrder = columnOrder
		p.ColumnVisibility = columnVis
		p.SearchQuery = searchQuery
		p.SearchTypes = searchTypes
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *filterPresetRepository) Count(ctx context.Context, f *observabilityDomain.FilterPresetFilter) (int64, error) {
	sqlStr, args, err := r.buildList(f).Columns("COUNT(*)").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build filter preset count: %w", err)
	}
	var n int64
	if err := r.tm.DB(ctx).QueryRow(ctx, sqlStr, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count filter presets: %w", err)
	}
	return n, nil
}

// buildList assembles the shared WHERE clause for List/Count so both
// queries see the same predicate.
func (r *filterPresetRepository) buildList(f *observabilityDomain.FilterPresetFilter) sq.SelectBuilder {
	b := sq.Select().From("filter_presets").Where(sq.Eq{"project_id": f.ProjectID})
	if f.TargetTable != nil {
		b = b.Where(sq.Eq{"table_name": *f.TargetTable})
	}
	switch {
	case f.UserID != nil && f.IncludeAll:
		b = b.Where("(created_by = ? OR is_public = true)", *f.UserID)
	case f.UserID != nil:
		b = b.Where(sq.Eq{"created_by": *f.UserID})
	case f.CreatedBy != nil:
		b = b.Where(sq.Eq{"created_by": *f.CreatedBy})
	case f.IsPublic != nil:
		b = b.Where(sq.Eq{"is_public": *f.IsPublic})
	}
	return b
}

func (r *filterPresetRepository) ExistsByName(ctx context.Context, projectID uuid.UUID, name string, excludeID *uuid.UUID) (bool, error) {
	b := sq.Select("COUNT(*)").From("filter_presets").
		Where(sq.Eq{"project_id": projectID, "name": name})
	if excludeID != nil {
		b = b.Where(sq.NotEq{"id": *excludeID})
	}
	sqlStr, args, err := b.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return false, fmt.Errorf("build exists query: %w", err)
	}
	var n int64
	if err := r.tm.DB(ctx).QueryRow(ctx, sqlStr, args...).Scan(&n); err != nil {
		return false, fmt.Errorf("check filter preset exists: %w", err)
	}
	return n > 0, nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func filterPresetFromRow(row *gen.FilterPreset) *observabilityDomain.FilterPreset {
	return &observabilityDomain.FilterPreset{
		ID:               row.ID,
		ProjectID:        row.ProjectID,
		Name:             row.Name,
		Description:      row.Description,
		TargetTable:      row.TableName,
		Filters:          row.Filters,
		ColumnOrder:      row.ColumnOrder,
		ColumnVisibility: row.ColumnVisibility,
		SearchQuery:      row.SearchQuery,
		SearchTypes:      row.SearchTypes,
		IsPublic:         row.IsPublic,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
