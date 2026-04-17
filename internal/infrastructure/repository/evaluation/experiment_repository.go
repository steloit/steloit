package evaluation

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	evalDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type ExperimentRepository struct {
	tm *db.TxManager
}

func NewExperimentRepository(tm *db.TxManager) *ExperimentRepository {
	return &ExperimentRepository{tm: tm}
}

func (r *ExperimentRepository) Create(ctx context.Context, e *evalDomain.Experiment) error {
	meta, err := marshalEvalJSON(e.Metadata)
	if err != nil {
		return err
	}
	return r.tm.Queries(ctx).CreateExperiment(ctx, gen.CreateExperimentParams{
		ID:             e.ID,
		ProjectID:      e.ProjectID,
		DatasetID:      e.DatasetID,
		Name:           e.Name,
		Description:    e.Description,
		Status:         string(e.Status),
		Metadata:       meta,
		StartedAt:      e.StartedAt,
		CompletedAt:    e.CompletedAt,
		ConfigID:       e.ConfigID,
		Source:         string(e.Source),
		TotalItems:     int32(e.TotalItems),
		CompletedItems: int32(e.CompletedItems),
		FailedItems:    int32(e.FailedItems),
	})
}

func (r *ExperimentRepository) GetByID(ctx context.Context, id, projectID uuid.UUID) (*evalDomain.Experiment, error) {
	row, err := r.tm.Queries(ctx).GetExperimentByID(ctx, gen.GetExperimentByIDParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrExperimentNotFound
		}
		return nil, err
	}
	return experimentFromRow(&row)
}

func (r *ExperimentRepository) List(ctx context.Context, projectID uuid.UUID, filter *evalDomain.ExperimentFilter, offset, limit int) ([]*evalDomain.Experiment, int64, error) {
	base := sq.Select().From("experiments").Where(sq.Eq{"project_id": projectID})
	if filter != nil {
		if filter.DatasetID != nil {
			base = base.Where(sq.Eq{"dataset_id": *filter.DatasetID})
		}
		if filter.Status != nil {
			base = base.Where(sq.Eq{"status": string(*filter.Status)})
		}
		if filter.Search != nil && *filter.Search != "" {
			p := "%" + strings.ToLower(*filter.Search) + "%"
			base = base.Where(sq.Or{
				sq.Expr("LOWER(name) LIKE ?", p),
				sq.Expr("LOWER(description) LIKE ?", p),
			})
		}
		if len(filter.IDs) > 0 {
			base = base.Where(sq.Eq{"id": filter.IDs})
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

	selSQL, selArgs, err := base.Columns(experimentColumns...).
		OrderBy("created_at DESC").Offset(uint64(offset)).Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.tm.DB(ctx).Query(ctx, selSQL, selArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*evalDomain.Experiment, 0)
	for rows.Next() {
		e, err := scanExperiment(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *ExperimentRepository) Update(ctx context.Context, e *evalDomain.Experiment, projectID uuid.UUID) error {
	meta, err := marshalEvalJSON(e.Metadata)
	if err != nil {
		return err
	}
	n, err := r.tm.Queries(ctx).UpdateExperiment(ctx, gen.UpdateExperimentParams{
		ID:             e.ID,
		ProjectID:      projectID,
		DatasetID:      e.DatasetID,
		Name:           e.Name,
		Description:    e.Description,
		Status:         string(e.Status),
		Metadata:       meta,
		StartedAt:      e.StartedAt,
		CompletedAt:    e.CompletedAt,
		ConfigID:       e.ConfigID,
		Source:         string(e.Source),
		TotalItems:     int32(e.TotalItems),
		CompletedItems: int32(e.CompletedItems),
		FailedItems:    int32(e.FailedItems),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrExperimentNotFound
	}
	return nil
}

func (r *ExperimentRepository) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteExperiment(ctx, gen.DeleteExperimentParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrExperimentNotFound
	}
	return nil
}

func (r *ExperimentRepository) SetTotalItems(ctx context.Context, id, projectID uuid.UUID, total int) error {
	n, err := r.tm.Queries(ctx).SetExperimentTotalItems(ctx, gen.SetExperimentTotalItemsParams{
		ID:         id,
		ProjectID:  projectID,
		TotalItems: int32(total),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrExperimentNotFound
	}
	return nil
}

func (r *ExperimentRepository) IncrementCounters(ctx context.Context, id, projectID uuid.UUID, completed, failed int) error {
	n, err := r.tm.Queries(ctx).IncrementExperimentCounters(ctx, gen.IncrementExperimentCountersParams{
		ID:             id,
		ProjectID:      projectID,
		CompletedItems: int32(completed),
		FailedItems:    int32(failed),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrExperimentNotFound
	}
	return nil
}

// IncrementCountersAndUpdateStatus locks the experiment row, applies
// the delta, and flips status to completed/failed/partial once all
// items are processed. Returns true when the experiment just finished.
func (r *ExperimentRepository) IncrementCountersAndUpdateStatus(ctx context.Context, id, projectID uuid.UUID, completed, failed int) (bool, error) {
	var isComplete bool
	err := r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		row, err := r.tm.Queries(ctx).LockExperimentForUpdate(ctx, gen.LockExperimentForUpdateParams{
			ID:        id,
			ProjectID: projectID,
		})
		if err != nil {
			if db.IsNoRows(err) {
				return evalDomain.ErrExperimentNotFound
			}
			return err
		}
		row.CompletedItems += int32(completed)
		row.FailedItems += int32(failed)
		processed := row.CompletedItems + row.FailedItems
		if processed >= row.TotalItems && row.TotalItems > 0 {
			isComplete = true
			now := time.Now()
			row.CompletedAt = &now
			switch {
			case row.FailedItems == 0:
				row.Status = string(evalDomain.ExperimentStatusCompleted)
			case row.CompletedItems == 0:
				row.Status = string(evalDomain.ExperimentStatusFailed)
			default:
				row.Status = string(evalDomain.ExperimentStatusPartial)
			}
		}
		_, err = r.tm.Queries(ctx).UpdateExperiment(ctx, gen.UpdateExperimentParams{
			ID:             row.ID,
			ProjectID:      row.ProjectID,
			DatasetID:      row.DatasetID,
			Name:           row.Name,
			Description:    row.Description,
			Status:         row.Status,
			Metadata:       row.Metadata,
			StartedAt:      row.StartedAt,
			CompletedAt:    row.CompletedAt,
			ConfigID:       row.ConfigID,
			Source:         row.Source,
			TotalItems:     row.TotalItems,
			CompletedItems: row.CompletedItems,
			FailedItems:    row.FailedItems,
		})
		return err
	})
	return isComplete, err
}

func (r *ExperimentRepository) GetProgress(ctx context.Context, id, projectID uuid.UUID) (*evalDomain.Experiment, error) {
	row, err := r.tm.Queries(ctx).GetExperimentProgress(ctx, gen.GetExperimentProgressParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrExperimentNotFound
		}
		return nil, err
	}
	return &evalDomain.Experiment{
		ID:             row.ID,
		Status:         evalDomain.ExperimentStatus(row.Status),
		TotalItems:     int(row.TotalItems),
		CompletedItems: int(row.CompletedItems),
		FailedItems:    int(row.FailedItems),
		StartedAt:      row.StartedAt,
		CompletedAt:    row.CompletedAt,
	}, nil
}

var experimentColumns = []string{
	"id", "project_id", "dataset_id", "name", "description",
	"status", "metadata", "started_at", "completed_at",
	"created_at", "updated_at", "config_id", "source",
	"total_items", "completed_items", "failed_items",
}

func scanExperiment(row interface {
	Scan(dest ...any) error
}) (*evalDomain.Experiment, error) {
	var (
		e    evalDomain.Experiment
		meta []byte
		total, completed, failed int32
	)
	if err := row.Scan(
		&e.ID, &e.ProjectID, &e.DatasetID, &e.Name, &e.Description,
		&e.Status, &meta, &e.StartedAt, &e.CompletedAt,
		&e.CreatedAt, &e.UpdatedAt, &e.ConfigID, &e.Source,
		&total, &completed, &failed,
	); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(meta, &e.Metadata); err != nil {
		return nil, err
	}
	e.TotalItems = int(total)
	e.CompletedItems = int(completed)
	e.FailedItems = int(failed)
	return &e, nil
}

func experimentFromRow(row *gen.Experiment) (*evalDomain.Experiment, error) {
	e := &evalDomain.Experiment{
		ID:             row.ID,
		ProjectID:      row.ProjectID,
		DatasetID:      row.DatasetID,
		Name:           row.Name,
		Description:    row.Description,
		Status:         evalDomain.ExperimentStatus(row.Status),
		StartedAt:      row.StartedAt,
		CompletedAt:    row.CompletedAt,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		ConfigID:       row.ConfigID,
		Source:         evalDomain.ExperimentSource(row.Source),
		TotalItems:     int(row.TotalItems),
		CompletedItems: int(row.CompletedItems),
		FailedItems:    int(row.FailedItems),
	}
	if err := unmarshalEvalJSON(row.Metadata, &e.Metadata); err != nil {
		return nil, err
	}
	return e, nil
}
