-- Static queries for trace_comments and comment_reactions.
-- trace_comments supports threaded replies (parent_id) and soft-delete
-- with a tombstone pattern: a deleted parent stays visible as long as
-- it has any active replies (services rely on this for thread cohesion).

-- name: CreateComment :exec
INSERT INTO trace_comments (
    id, entity_type, entity_id, project_id, content,
    parent_id, created_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8
);

-- name: GetCommentByID :one
SELECT * FROM trace_comments
WHERE id = $1
LIMIT 1;

-- name: UpdateComment :execrows
UPDATE trace_comments
SET content    = $2,
    updated_by = $3,
    updated_at = NOW()
WHERE id = $1;

-- name: SoftDeleteComment :execrows
UPDATE trace_comments
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: HasActiveReplies :one
SELECT EXISTS (
    SELECT 1 FROM trace_comments
    WHERE parent_id = $1 AND deleted_at IS NULL
);

-- name: ListCommentsByEntity :many
-- Tombstone-aware: returns top-level comments that are either not
-- deleted, or deleted-but-still-referenced by an active reply. The
-- service layer treats deleted-with-replies rows as [deleted] placeholders.
SELECT tc.* FROM trace_comments tc
WHERE tc.entity_type = $1
  AND tc.entity_id   = $2
  AND tc.project_id  = $3
  AND tc.parent_id IS NULL
  AND (
      tc.deleted_at IS NULL
      OR tc.id IN (
          SELECT DISTINCT r.parent_id FROM trace_comments r
          WHERE r.entity_type = $1 AND r.entity_id = $2 AND r.project_id = $3
            AND r.parent_id IS NOT NULL AND r.deleted_at IS NULL
      )
  )
ORDER BY tc.created_at ASC;

-- name: CountCommentsByEntity :one
SELECT COUNT(*)::bigint FROM trace_comments
WHERE entity_type = $1
  AND entity_id   = $2
  AND project_id  = $3
  AND deleted_at IS NULL;

-- name: ListRepliesByParents :many
SELECT * FROM trace_comments
WHERE parent_id = ANY($1::uuid[]) AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: CountRepliesByParents :many
SELECT parent_id, COUNT(*)::bigint AS count
FROM trace_comments
WHERE parent_id = ANY($1::uuid[]) AND deleted_at IS NULL
GROUP BY parent_id;

-- ----- comment_reactions --------------------------------------------

-- name: CreateCommentReaction :exec
INSERT INTO comment_reactions (
    id, comment_id, user_id, emoji, created_at
) VALUES (
    $1, $2, $3, $4, NOW()
);

-- name: DeleteCommentReaction :execrows
DELETE FROM comment_reactions
WHERE comment_id = $1 AND user_id = $2 AND emoji = $3;

-- name: GetCommentReactionByUserEmoji :one
SELECT * FROM comment_reactions
WHERE comment_id = $1 AND user_id = $2 AND emoji = $3
LIMIT 1;

-- name: ListCommentReactionsByComments :many
SELECT * FROM comment_reactions
WHERE comment_id = ANY($1::uuid[])
ORDER BY created_at ASC;

-- name: CountDistinctEmojisOnComment :one
SELECT COUNT(DISTINCT emoji)::bigint FROM comment_reactions
WHERE comment_id = $1;

-- name: CommentReactionExists :one
SELECT EXISTS (
    SELECT 1 FROM comment_reactions
    WHERE comment_id = $1 AND user_id = $2 AND emoji = $3
);

-- ----- user lookup for comment author enrichment --------------------

-- name: ListUsersForCommentEnrichment :many
-- Returns (id, first_name, last_name, email, avatar_url) for comment
-- author/editor display. avatar_url comes from user_profiles; users
-- without profiles get NULL.
SELECT
    u.id,
    u.first_name,
    u.last_name,
    u.email,
    p.avatar_url
FROM users u
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE u.id = ANY($1::uuid[]) AND u.deleted_at IS NULL;
