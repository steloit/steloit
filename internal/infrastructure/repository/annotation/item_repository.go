package annotation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	annotationDomain "brokle/internal/core/domain/annotation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	appErrors "brokle/pkg/errors"
)

type ItemRepository struct {
	tm *db.TxManager
}

func NewItemRepository(tm *db.TxManager) *ItemRepository {
	return &ItemRepository{tm: tm}
}

func (r *ItemRepository) Create(ctx context.Context, item *annotationDomain.QueueItem) error {
	meta, err := marshalItemMetadata(item.Metadata)
	if err != nil {
		return err
	}
	if err := r.tm.Queries(ctx).CreateAnnotationQueueItem(ctx, gen.CreateAnnotationQueueItemParams{
		ID:              item.ID,
		QueueID:         item.QueueID,
		ObjectID:        item.ObjectID,
		ObjectType:      string(item.ObjectType),
		Status:          string(item.Status),
		Priority:        int32(item.Priority),
		LockedAt:        item.LockedAt,
		LockedByUserID:  item.LockedByUserID,
		AnnotatorUserID: item.AnnotatorUserID,
		CompletedAt:     item.CompletedAt,
		Metadata:        meta,
	}); err != nil {
		if appErrors.IsUniqueViolation(err) {
			return annotationDomain.ErrItemExists
		}
		return err
	}
	return nil
}

// CreateBatch inserts many items in one round-trip using parallel UNNEST arrays.
// ON CONFLICT (queue_id, object_id, object_type) DO NOTHING makes this
// idempotent; the returned count reflects actual insertions.
func (r *ItemRepository) CreateBatch(ctx context.Context, items []*annotationDomain.QueueItem) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}
	ids := make([]uuid.UUID, len(items))
	queueIDs := make([]uuid.UUID, len(items))
	objectIDs := make([]string, len(items))
	objectTypes := make([]string, len(items))
	statuses := make([]string, len(items))
	priorities := make([]int32, len(items))
	metas := make([]json.RawMessage, len(items))
	for i, it := range items {
		ids[i] = it.ID
		queueIDs[i] = it.QueueID
		objectIDs[i] = it.ObjectID
		objectTypes[i] = string(it.ObjectType)
		statuses[i] = string(it.Status)
		priorities[i] = int32(it.Priority)
		meta, err := marshalItemMetadata(it.Metadata)
		if err != nil {
			return 0, err
		}
		if len(meta) == 0 {
			metas[i] = json.RawMessage("{}")
		} else {
			metas[i] = meta
		}
	}
	n, err := r.tm.Queries(ctx).CreateAnnotationQueueItemsBatch(ctx, gen.CreateAnnotationQueueItemsBatchParams{
		Column1: ids,
		Column2: queueIDs,
		Column3: objectIDs,
		Column4: objectTypes,
		Column5: statuses,
		Column6: priorities,
		Column7: metas,
	})
	if err != nil {
		return 0, fmt.Errorf("batch insert annotation items: %w", err)
	}
	return n, nil
}

func (r *ItemRepository) GetByID(ctx context.Context, id uuid.UUID) (*annotationDomain.QueueItem, error) {
	row, err := r.tm.Queries(ctx).GetAnnotationQueueItemByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, annotationDomain.ErrItemNotFound
		}
		return nil, err
	}
	return itemFromRow(&row)
}

func (r *ItemRepository) GetByIDForQueue(ctx context.Context, id, queueID uuid.UUID) (*annotationDomain.QueueItem, error) {
	row, err := r.tm.Queries(ctx).GetAnnotationQueueItemByIDForQueue(ctx, gen.GetAnnotationQueueItemByIDForQueueParams{
		ID:      id,
		QueueID: queueID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, annotationDomain.ErrItemNotFound
		}
		return nil, err
	}
	return itemFromRow(&row)
}

func (r *ItemRepository) List(ctx context.Context, queueID uuid.UUID, filter *annotationDomain.ItemFilter) ([]*annotationDomain.QueueItem, int64, error) {
	base := sq.Select().From("annotation_queue_items").Where(sq.Eq{"queue_id": queueID})
	if filter != nil && filter.Status != nil {
		base = base.Where(sq.Eq{"status": string(*filter.Status)})
	}

	cntSQL, cntArgs, err := base.Columns("COUNT(*)").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, cntSQL, cntArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	list := base.Columns(itemColumns...).OrderBy("priority DESC, created_at ASC")
	if filter != nil {
		if filter.Limit > 0 {
			list = list.Limit(uint64(filter.Limit))
		}
		if filter.Offset > 0 {
			list = list.Offset(uint64(filter.Offset))
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
	out := make([]*annotationDomain.QueueItem, 0)
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (r *ItemRepository) Update(ctx context.Context, item *annotationDomain.QueueItem) error {
	item.UpdatedAt = time.Now()
	meta, err := marshalItemMetadata(item.Metadata)
	if err != nil {
		return err
	}
	n, err := r.tm.Queries(ctx).UpdateAnnotationQueueItem(ctx, gen.UpdateAnnotationQueueItemParams{
		ID:              item.ID,
		Status:          string(item.Status),
		Priority:        int32(item.Priority),
		LockedAt:        item.LockedAt,
		LockedByUserID:  item.LockedByUserID,
		AnnotatorUserID: item.AnnotatorUserID,
		CompletedAt:     item.CompletedAt,
		Metadata:        meta,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return annotationDomain.ErrItemNotFound
	}
	return nil
}

func (r *ItemRepository) Delete(ctx context.Context, id, queueID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteAnnotationQueueItem(ctx, gen.DeleteAnnotationQueueItemParams{
		ID:      id,
		QueueID: queueID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return annotationDomain.ErrItemNotFound
	}
	return nil
}

func (r *ItemRepository) ExistsByObject(ctx context.Context, queueID uuid.UUID, objectID string, objectType annotationDomain.ObjectType) (bool, error) {
	return r.tm.Queries(ctx).AnnotationQueueItemExistsByObject(ctx, gen.AnnotationQueueItemExistsByObjectParams{
		QueueID:    queueID,
		ObjectID:   objectID,
		ObjectType: string(objectType),
	})
}

// FetchAndLockNext atomically claims the next available item using
// SELECT ... FOR UPDATE SKIP LOCKED + UPDATE. Eligibility: pending AND
// (never locked OR lock expired OR locked by same user). Kept as raw
// SQL because sqlc cannot express FOR UPDATE SKIP LOCKED typed params.
func (r *ItemRepository) FetchAndLockNext(ctx context.Context, queueID, userID uuid.UUID, lockTimeout int, seenItemIDs []uuid.UUID) (*annotationDomain.QueueItem, error) {
	now := time.Now()
	lockExpiry := now.Add(-time.Duration(lockTimeout) * time.Second)

	selector := sq.Select("id").From("annotation_queue_items").
		Where(sq.Eq{"queue_id": queueID}).
		Where(sq.Eq{"status": string(annotationDomain.ItemStatusPending)}).
		Where(sq.Or{
			sq.Expr("locked_at IS NULL"),
			sq.Lt{"locked_at": lockExpiry},
			sq.Eq{"locked_by_user_id": userID},
		}).
		OrderBy("priority DESC", "created_at ASC").
		Limit(1).
		Suffix("FOR UPDATE SKIP LOCKED")
	if len(seenItemIDs) > 0 {
		selector = selector.Where(sq.NotEq{"id": seenItemIDs})
	}
	selSQL, selArgs, err := selector.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build lock-next query: %w", err)
	}

	claim := fmt.Sprintf(`
		UPDATE annotation_queue_items
		SET locked_at         = $%d,
		    locked_by_user_id = $%d,
		    updated_at        = $%d
		WHERE id = (%s)
		RETURNING %s
	`, len(selArgs)+1, len(selArgs)+2, len(selArgs)+3, selSQL, itemColumnList())

	args := append(selArgs, now, userID, now)
	row := r.tm.DB(ctx).QueryRow(ctx, claim, args...)
	item, err := scanItem(row)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, annotationDomain.ErrNoItemsAvailable
		}
		return nil, fmt.Errorf("claim next annotation item: %w", err)
	}
	return item, nil
}

func (r *ItemRepository) Complete(ctx context.Context, id, userID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).CompleteAnnotationQueueItem(ctx, gen.CompleteAnnotationQueueItemParams{
		ID:              id,
		AnnotatorUserID: &userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return annotationDomain.ErrItemNotFound
	}
	return nil
}

func (r *ItemRepository) Skip(ctx context.Context, id, userID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).SkipAnnotationQueueItem(ctx, gen.SkipAnnotationQueueItemParams{
		ID:              id,
		AnnotatorUserID: &userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return annotationDomain.ErrItemNotFound
	}
	return nil
}

func (r *ItemRepository) ReleaseLock(ctx context.Context, id uuid.UUID) error {
	n, err := r.tm.Queries(ctx).ReleaseAnnotationQueueItemLock(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return annotationDomain.ErrItemNotFound
	}
	return nil
}

func (r *ItemRepository) ReleaseExpiredLocks(ctx context.Context, queueID uuid.UUID, lockTimeout int) (int64, error) {
	lockExpiry := time.Now().Add(-time.Duration(lockTimeout) * time.Second)
	return r.tm.Queries(ctx).ReleaseExpiredAnnotationQueueLocks(ctx, gen.ReleaseExpiredAnnotationQueueLocksParams{
		QueueID:  queueID,
		LockedAt: &lockExpiry,
	})
}

func (r *ItemRepository) GetStats(ctx context.Context, queueID uuid.UUID, lockTimeout int) (*annotationDomain.QueueStats, error) {
	stats := &annotationDomain.QueueStats{}
	statusRows, err := r.tm.Queries(ctx).CountAnnotationQueueItemsByStatus(ctx, queueID)
	if err != nil {
		return nil, err
	}
	for _, row := range statusRows {
		switch annotationDomain.ItemStatus(row.Status) {
		case annotationDomain.ItemStatusPending:
			stats.PendingItems = row.Count
		case annotationDomain.ItemStatusCompleted:
			stats.CompletedItems = row.Count
		case annotationDomain.ItemStatusSkipped:
			stats.SkippedItems = row.Count
		}
		stats.TotalItems += row.Count
	}
	lockExpiry := time.Now().Add(-time.Duration(lockTimeout) * time.Second)
	inProgress, err := r.tm.Queries(ctx).CountAnnotationQueueItemsInProgress(ctx, gen.CountAnnotationQueueItemsInProgressParams{
		QueueID:  queueID,
		LockedAt: &lockExpiry,
	})
	if err != nil {
		return nil, err
	}
	stats.InProgressItems = inProgress
	stats.PendingItems -= stats.InProgressItems
	return stats, nil
}

// ----- helpers --------------------------------------------------------

var itemColumns = []string{
	"id", "queue_id", "object_id", "object_type",
	"status", "priority",
	"locked_at", "locked_by_user_id", "annotator_user_id", "completed_at",
	"metadata", "created_at", "updated_at",
}

func itemColumnList() string {
	return "id, queue_id, object_id, object_type, status, priority, locked_at, locked_by_user_id, annotator_user_id, completed_at, metadata, created_at, updated_at"
}

func scanItem(row interface {
	Scan(dest ...any) error
}) (*annotationDomain.QueueItem, error) {
	var (
		it       annotationDomain.QueueItem
		priority int32
		meta     []byte
	)
	if err := row.Scan(
		&it.ID, &it.QueueID, &it.ObjectID, &it.ObjectType,
		&it.Status, &priority,
		&it.LockedAt, &it.LockedByUserID, &it.AnnotatorUserID, &it.CompletedAt,
		&meta, &it.CreatedAt, &it.UpdatedAt,
	); err != nil {
		return nil, err
	}
	it.Priority = int(priority)
	if len(meta) > 0 {
		if err := json.Unmarshal(meta, &it.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal item metadata: %w", err)
		}
	}
	return &it, nil
}

func itemFromRow(row *gen.AnnotationQueueItem) (*annotationDomain.QueueItem, error) {
	it := &annotationDomain.QueueItem{
		ID:              row.ID,
		QueueID:         row.QueueID,
		ObjectID:        row.ObjectID,
		ObjectType:      annotationDomain.ObjectType(row.ObjectType),
		Status:          annotationDomain.ItemStatus(row.Status),
		Priority:        int(row.Priority),
		LockedAt:        row.LockedAt,
		LockedByUserID:  row.LockedByUserID,
		AnnotatorUserID: row.AnnotatorUserID,
		CompletedAt:     row.CompletedAt,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &it.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal item metadata: %w", err)
		}
	}
	return it, nil
}

func marshalItemMetadata(m map[string]interface{}) (json.RawMessage, error) {
	if len(m) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return json.Marshal(m)
}
