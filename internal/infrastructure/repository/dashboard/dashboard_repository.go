package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	dashboardDomain "brokle/internal/core/domain/dashboard"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type dashboardRepository struct {
	tm *db.TxManager
}

func NewDashboardRepository(tm *db.TxManager) dashboardDomain.DashboardRepository {
	return &dashboardRepository{tm: tm}
}

func (r *dashboardRepository) Create(ctx context.Context, d *dashboardDomain.Dashboard) error {
	cfg, layout, err := marshalDashboardContent(&d.Config, d.Layout)
	if err != nil {
		return fmt.Errorf("create dashboard: %w", err)
	}
	if err := r.tm.Queries(ctx).CreateDashboard(ctx, gen.CreateDashboardParams{
		ID:          d.ID,
		ProjectID:   d.ProjectID,
		Name:        d.Name,
		Description: nilIfEmptyDash(d.Description),
		Config:      cfg,
		Layout:      layout,
		IsLocked:    d.IsLocked,
		CreatedBy:   d.CreatedBy,
	}); err != nil {
		return fmt.Errorf("create dashboard: %w", err)
	}
	return nil
}

func (r *dashboardRepository) GetByID(ctx context.Context, id uuid.UUID) (*dashboardDomain.Dashboard, error) {
	row, err := r.tm.Queries(ctx).GetDashboardByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get dashboard by ID %s: %w", id, dashboardDomain.ErrDashboardNotFound)
		}
		return nil, err
	}
	return dashboardFromRow(&row)
}

func (r *dashboardRepository) Update(ctx context.Context, d *dashboardDomain.Dashboard) error {
	d.UpdatedAt = time.Now()
	cfg, layout, err := marshalDashboardContent(&d.Config, d.Layout)
	if err != nil {
		return fmt.Errorf("update dashboard: %w", err)
	}
	if err := r.tm.Queries(ctx).UpdateDashboard(ctx, gen.UpdateDashboardParams{
		ID:          d.ID,
		Name:        d.Name,
		Description: nilIfEmptyDash(d.Description),
		Config:      cfg,
		Layout:      layout,
		IsLocked:    d.IsLocked,
	}); err != nil {
		return fmt.Errorf("update dashboard: %w", err)
	}
	return nil
}

// Delete preserves the original GORM semantics (GORM-style soft-delete
// — explicit SQL soft-delete via deleted_at.
func (r *dashboardRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteDashboard(ctx, id); err != nil {
		return fmt.Errorf("delete dashboard: %w", err)
	}
	return nil
}

func (r *dashboardRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.Delete(ctx, id)
}

func (r *dashboardRepository) GetByNameAndProject(ctx context.Context, projectID uuid.UUID, name string) (*dashboardDomain.Dashboard, error) {
	row, err := r.tm.Queries(ctx).GetDashboardByNameAndProject(ctx, gen.GetDashboardByNameAndProjectParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get dashboard by name %s: %w", name, dashboardDomain.ErrDashboardNotFound)
		}
		return nil, err
	}
	return dashboardFromRow(&row)
}

func (r *dashboardRepository) CountByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	n, err := r.tm.Queries(ctx).CountDashboardsByProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// GetByProjectID is the dynamic-filter list. Optional ILIKE on name,
// pagination; total uses the same predicate for accuracy.
func (r *dashboardRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID, filter *dashboardDomain.DashboardFilter) (*dashboardDomain.DashboardListResponse, error) {
	limit := 50
	offset := 0
	if filter != nil {
		if filter.Limit > 0 {
			limit = filter.Limit
		}
		if filter.Offset > 0 {
			offset = filter.Offset
		}
	}

	whereSQL, whereArgs := buildDashboardFilter(projectID, filter)

	cntSQL, cntArgs, err := sq.Select("COUNT(*)").From("dashboards").Where(whereSQL, whereArgs...).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build dashboard count query: %w", err)
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, cntSQL, cntArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count dashboards: %w", err)
	}

	selSQL, selArgs, err := sq.Select(
		"id", "project_id", "name", "description",
		"config", "layout", "created_by",
		"created_at", "updated_at", "deleted_at", "is_locked",
	).From("dashboards").Where(whereSQL, whereArgs...).
		OrderBy("created_at DESC").
		Limit(uint64(limit)).Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build dashboard list query: %w", err)
	}

	rows, err := r.tm.DB(ctx).Query(ctx, selSQL, selArgs...)
	if err != nil {
		return nil, fmt.Errorf("list dashboards: %w", err)
	}
	defer rows.Close()
	out := make([]*dashboardDomain.Dashboard, 0)
	for rows.Next() {
		var (
			d           dashboardDomain.Dashboard
			description *string
			cfgRaw      json.RawMessage
			layoutRaw   json.RawMessage
			deletedAt   *time.Time
		)
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.Name, &description,
			&cfgRaw, &layoutRaw, &d.CreatedBy,
			&d.CreatedAt, &d.UpdatedAt, &deletedAt, &d.IsLocked,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard row: %w", err)
		}
		if description != nil {
			d.Description = *description
		}
		if err := unmarshalDashboardContent(cfgRaw, layoutRaw, &d.Config, &d.Layout); err != nil {
			return nil, err
		}
		d.DeletedAt = deletedAt
		out = append(out, &d)
	}

	return &dashboardDomain.DashboardListResponse{
		Dashboards: out,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, rows.Err()
}

// buildDashboardFilter returns a sqlizer-ready WHERE clause + args.
// Kept as a helper because List and Count must share the same predicate
// or the pagination/total pair will disagree.
func buildDashboardFilter(projectID uuid.UUID, filter *dashboardDomain.DashboardFilter) (string, []any) {
	args := []any{projectID}
	clauses := []string{"project_id = ?", "deleted_at IS NULL"}
	if filter != nil && filter.Name != "" {
		clauses = append(clauses, "name ILIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}
	combined := clauses[0]
	for _, c := range clauses[1:] {
		combined += " AND " + c
	}
	return combined, args
}

// ----- gen ↔ domain boundary -----------------------------------------

func dashboardFromRow(row *gen.Dashboard) (*dashboardDomain.Dashboard, error) {
	d := &dashboardDomain.Dashboard{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Name:      row.Name,
		IsLocked:  row.IsLocked,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		DeletedAt: row.DeletedAt,
	}
	if row.Description != nil {
		d.Description = *row.Description
	}
	if err := unmarshalDashboardContent(row.Config, row.Layout, &d.Config, &d.Layout); err != nil {
		return nil, err
	}
	return d, nil
}

func marshalDashboardContent(cfg *dashboardDomain.DashboardConfig, layout []dashboardDomain.LayoutItem) (json.RawMessage, json.RawMessage, error) {
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal dashboard config: %w", err)
	}
	if layout == nil {
		layout = []dashboardDomain.LayoutItem{}
	}
	layoutRaw, err := json.Marshal(layout)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal dashboard layout: %w", err)
	}
	return cfgRaw, layoutRaw, nil
}

func unmarshalDashboardContent(cfgRaw, layoutRaw json.RawMessage, cfg *dashboardDomain.DashboardConfig, layout *[]dashboardDomain.LayoutItem) error {
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, cfg); err != nil {
			return fmt.Errorf("unmarshal dashboard config: %w", err)
		}
	}
	if len(layoutRaw) > 0 {
		if err := json.Unmarshal(layoutRaw, layout); err != nil {
			return fmt.Errorf("unmarshal dashboard layout: %w", err)
		}
	}
	return nil
}

func nilIfEmptyDash(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
