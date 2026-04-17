-- Static queries for annotation_queues, annotation_queue_items, and
-- annotation_queue_assignments. Dynamic filters live in squirrel.
-- FetchAndLockNext uses SELECT ... FOR UPDATE SKIP LOCKED for safe
-- concurrent item dispatch across multiple reviewers.

-- name: CreateAnnotationQueue :exec
INSERT INTO annotation_queues (
    id, project_id, name, description, instructions,
    score_config_ids, status, settings,
    created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, NOW(), NOW()
);

-- name: GetAnnotationQueueByIDForProject :one
SELECT * FROM annotation_queues
WHERE id = $1 AND project_id = $2
LIMIT 1;

-- name: GetAnnotationQueueByName :one
SELECT * FROM annotation_queues
WHERE project_id = $1 AND name = $2
LIMIT 1;

-- name: UpdateAnnotationQueue :execrows
UPDATE annotation_queues
SET name             = $2,
    description      = $3,
    instructions     = $4,
    score_config_ids = $5,
    status           = $6,
    settings         = $7,
    updated_at       = NOW()
WHERE id = $1;

-- name: DeleteAnnotationQueue :execrows
DELETE FROM annotation_queues
WHERE id = $1 AND project_id = $2;

-- name: AnnotationQueueExistsByName :one
SELECT EXISTS (
    SELECT 1 FROM annotation_queues
    WHERE project_id = $1 AND name = $2
);

-- name: ListAllActiveAnnotationQueues :many
SELECT * FROM annotation_queues
WHERE status = 'active'
ORDER BY created_at DESC;

-- ----- annotation_queue_items ---------------------------------------

-- name: CreateAnnotationQueueItem :exec
INSERT INTO annotation_queue_items (
    id, queue_id, object_id, object_type,
    status, priority,
    locked_at, locked_by_user_id, annotator_user_id, completed_at,
    metadata, created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9, $10,
    $11, NOW(), NOW()
);

-- name: CreateAnnotationQueueItemsBatch :execrows
-- Idempotent batch insert: ON CONFLICT (queue_id, object_id, object_type)
-- skips duplicates. Returns rows actually inserted.
INSERT INTO annotation_queue_items (
    id, queue_id, object_id, object_type,
    status, priority,
    metadata, created_at, updated_at
)
SELECT
    UNNEST($1::uuid[]),
    UNNEST($2::uuid[]),
    UNNEST($3::text[]),
    UNNEST($4::text[]),
    UNNEST($5::text[]),
    UNNEST($6::int[]),
    UNNEST($7::jsonb[]),
    NOW(), NOW()
ON CONFLICT (queue_id, object_id, object_type) DO NOTHING;

-- name: GetAnnotationQueueItemByID :one
SELECT * FROM annotation_queue_items
WHERE id = $1
LIMIT 1;

-- name: GetAnnotationQueueItemByIDForQueue :one
SELECT * FROM annotation_queue_items
WHERE id = $1 AND queue_id = $2
LIMIT 1;

-- name: UpdateAnnotationQueueItem :execrows
UPDATE annotation_queue_items
SET status            = $2,
    priority          = $3,
    locked_at         = $4,
    locked_by_user_id = $5,
    annotator_user_id = $6,
    completed_at      = $7,
    metadata          = $8,
    updated_at        = NOW()
WHERE id = $1;

-- name: DeleteAnnotationQueueItem :execrows
DELETE FROM annotation_queue_items
WHERE id = $1 AND queue_id = $2;

-- name: AnnotationQueueItemExistsByObject :one
SELECT EXISTS (
    SELECT 1 FROM annotation_queue_items
    WHERE queue_id = $1 AND object_id = $2 AND object_type = $3
);

-- name: CompleteAnnotationQueueItem :execrows
UPDATE annotation_queue_items
SET status            = 'completed',
    annotator_user_id = $2,
    completed_at      = NOW(),
    updated_at        = NOW()
WHERE id = $1;

-- name: SkipAnnotationQueueItem :execrows
UPDATE annotation_queue_items
SET status            = 'skipped',
    annotator_user_id = $2,
    completed_at      = NOW(),
    updated_at        = NOW()
WHERE id = $1;

-- name: ReleaseAnnotationQueueItemLock :execrows
UPDATE annotation_queue_items
SET locked_at         = NULL,
    locked_by_user_id = NULL,
    updated_at        = NOW()
WHERE id = $1;

-- name: ReleaseExpiredAnnotationQueueLocks :execrows
UPDATE annotation_queue_items
SET locked_at         = NULL,
    locked_by_user_id = NULL,
    updated_at        = NOW()
WHERE queue_id  = $1
  AND status    = 'pending'
  AND locked_at IS NOT NULL
  AND locked_at < $2;

-- name: CountAnnotationQueueItemsByStatus :many
SELECT status, COUNT(*)::bigint AS count
FROM annotation_queue_items
WHERE queue_id = $1
GROUP BY status;

-- name: CountAnnotationQueueItemsInProgress :one
SELECT COUNT(*)::bigint FROM annotation_queue_items
WHERE queue_id  = $1
  AND status    = 'pending'
  AND locked_at IS NOT NULL
  AND locked_at >= $2;

-- ----- annotation_queue_assignments ---------------------------------

-- name: CreateAnnotationQueueAssignment :exec
INSERT INTO annotation_queue_assignments (
    id, queue_id, user_id, role, assigned_at, assigned_by
) VALUES (
    $1, $2, $3, $4, NOW(), $5
);

-- name: DeleteAnnotationQueueAssignment :execrows
DELETE FROM annotation_queue_assignments
WHERE queue_id = $1 AND user_id = $2;

-- name: GetAnnotationQueueAssignmentByQueueAndUser :one
SELECT * FROM annotation_queue_assignments
WHERE queue_id = $1 AND user_id = $2
LIMIT 1;

-- name: ListAnnotationQueueAssignmentsByQueue :many
SELECT * FROM annotation_queue_assignments
WHERE queue_id = $1
ORDER BY assigned_at ASC;

-- name: ListAnnotationQueueAssignmentsByUser :many
SELECT * FROM annotation_queue_assignments
WHERE user_id = $1
ORDER BY assigned_at DESC;

-- name: AnnotationQueueAssignmentExists :one
SELECT EXISTS (
    SELECT 1 FROM annotation_queue_assignments
    WHERE queue_id = $1 AND user_id = $2
);
