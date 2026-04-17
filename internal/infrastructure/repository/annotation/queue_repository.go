package annotation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	annotationDomain "brokle/internal/core/domain/annotation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	appErrors "brokle/pkg/errors"
)

type QueueRepository struct {
	tm *db.TxManager
}

func NewQueueRepository(tm *db.TxManager) *QueueRepository {
	return &QueueRepository{tm: tm}
}

func (r *QueueRepository) Create(ctx context.Context, q *annotationDomain.AnnotationQueue) error {
	scoreIDs, err := json.Marshal(q.ScoreConfigIDs)
	if err != nil {
		return fmt.Errorf("marshal score_config_ids: %w", err)
	}
	settings, err := json.Marshal(q.Settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := r.tm.Queries(ctx).CreateAnnotationQueue(ctx, gen.CreateAnnotationQueueParams{
		ID:             q.ID,
		ProjectID:      q.ProjectID,
		Name:           q.Name,
		Description:    q.Description,
		Instructions:   q.Instructions,
		ScoreConfigIds: scoreIDs,
		Status:         string(q.Status),
		Settings:       settings,
		CreatedBy:      q.CreatedBy,
	}); err != nil {
		if appErrors.IsUniqueViolation(err) {
			return annotationDomain.ErrQueueExists
		}
		return err
	}
	return nil
}

func (r *QueueRepository) GetByID(ctx context.Context, id, projectID uuid.UUID) (*annotationDomain.AnnotationQueue, error) {
	row, err := r.tm.Queries(ctx).GetAnnotationQueueByIDForProject(ctx, gen.GetAnnotationQueueByIDForProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, annotationDomain.ErrQueueNotFound
		}
		return nil, err
	}
	return queueFromRow(&row)
}

func (r *QueueRepository) GetByName(ctx context.Context, name string, projectID uuid.UUID) (*annotationDomain.AnnotationQueue, error) {
	row, err := r.tm.Queries(ctx).GetAnnotationQueueByName(ctx, gen.GetAnnotationQueueByNameParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return queueFromRow(&row)
}

func (r *QueueRepository) List(ctx context.Context, projectID uuid.UUID, filter *annotationDomain.QueueFilter, offset, limit int) ([]*annotationDomain.AnnotationQueue, int64, error) {
	base := sq.Select().From("annotation_queues").Where(sq.Eq{"project_id": projectID})
	if filter != nil {
		if filter.Status != nil {
			base = base.Where(sq.Eq{"status": string(*filter.Status)})
		}
		if filter.Search != nil && *filter.Search != "" {
			pattern := "%" + strings.ToLower(*filter.Search) + "%"
			base = base.Where(sq.Or{
				sq.Expr("LOWER(name) LIKE ?", pattern),
				sq.Expr("LOWER(description) LIKE ?", pattern),
			})
		}
	}

	// Count
	cntSQL, cntArgs, err := base.Columns("COUNT(*)").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build queue count: %w", err)
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, cntSQL, cntArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count queues: %w", err)
	}

	// List
	selSQL, selArgs, err := base.Columns(queueColumns...).
		OrderBy("created_at DESC").Offset(uint64(offset)).Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build queue list: %w", err)
	}
	rows, err := r.tm.DB(ctx).Query(ctx, selSQL, selArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list queues: %w", err)
	}
	defer rows.Close()
	out := make([]*annotationDomain.AnnotationQueue, 0)
	for rows.Next() {
		q, err := scanQueue(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, q)
	}
	return out, total, rows.Err()
}

func (r *QueueRepository) Update(ctx context.Context, q *annotationDomain.AnnotationQueue) error {
	scoreIDs, err := json.Marshal(q.ScoreConfigIDs)
	if err != nil {
		return fmt.Errorf("marshal score_config_ids: %w", err)
	}
	settings, err := json.Marshal(q.Settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	n, err := r.tm.Queries(ctx).UpdateAnnotationQueue(ctx, gen.UpdateAnnotationQueueParams{
		ID:             q.ID,
		Name:           q.Name,
		Description:    q.Description,
		Instructions:   q.Instructions,
		ScoreConfigIds: scoreIDs,
		Status:         string(q.Status),
		Settings:       settings,
	})
	if err != nil {
		if appErrors.IsUniqueViolation(err) {
			return annotationDomain.ErrQueueExists
		}
		return err
	}
	if n == 0 {
		return annotationDomain.ErrQueueNotFound
	}
	return nil
}

func (r *QueueRepository) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteAnnotationQueue(ctx, gen.DeleteAnnotationQueueParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return annotationDomain.ErrQueueNotFound
	}
	return nil
}

func (r *QueueRepository) ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error) {
	return r.tm.Queries(ctx).AnnotationQueueExistsByName(ctx, gen.AnnotationQueueExistsByNameParams{
		ProjectID: projectID,
		Name:      name,
	})
}

func (r *QueueRepository) ListAllActive(ctx context.Context) ([]*annotationDomain.AnnotationQueue, error) {
	rows, err := r.tm.Queries(ctx).ListAllActiveAnnotationQueues(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*annotationDomain.AnnotationQueue, 0, len(rows))
	for i := range rows {
		q, err := queueFromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

// ----- helpers --------------------------------------------------------

var queueColumns = []string{
	"id", "project_id", "name", "description", "instructions",
	"score_config_ids", "status", "settings",
	"created_by", "created_at", "updated_at",
}

func scanQueue(row interface {
	Scan(dest ...any) error
}) (*annotationDomain.AnnotationQueue, error) {
	var (
		q        annotationDomain.AnnotationQueue
		scoreIDs []byte
		settings []byte
	)
	if err := row.Scan(
		&q.ID, &q.ProjectID, &q.Name, &q.Description, &q.Instructions,
		&scoreIDs, &q.Status, &settings,
		&q.CreatedBy, &q.CreatedAt, &q.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(scoreIDs) > 0 {
		if err := json.Unmarshal(scoreIDs, &q.ScoreConfigIDs); err != nil {
			return nil, fmt.Errorf("unmarshal score_config_ids: %w", err)
		}
	}
	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &q.Settings); err != nil {
			return nil, fmt.Errorf("unmarshal settings: %w", err)
		}
	}
	return &q, nil
}

func queueFromRow(row *gen.AnnotationQueue) (*annotationDomain.AnnotationQueue, error) {
	q := &annotationDomain.AnnotationQueue{
		ID:           row.ID,
		ProjectID:    row.ProjectID,
		Name:         row.Name,
		Description:  row.Description,
		Instructions: row.Instructions,
		Status:       annotationDomain.QueueStatus(row.Status),
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
	if len(row.ScoreConfigIds) > 0 {
		if err := json.Unmarshal(row.ScoreConfigIds, &q.ScoreConfigIDs); err != nil {
			return nil, fmt.Errorf("unmarshal score_config_ids: %w", err)
		}
	}
	if len(row.Settings) > 0 {
		if err := json.Unmarshal(row.Settings, &q.Settings); err != nil {
			return nil, fmt.Errorf("unmarshal settings: %w", err)
		}
	}
	return q, nil
}
