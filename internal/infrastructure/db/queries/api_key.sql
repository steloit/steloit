-- Static queries for api_keys. Filter listings with dynamic WHERE live in
-- internal/infrastructure/repository/auth/api_key_filter.go (squirrel).
--
-- Every non-cleanup read implicitly restricts to deleted_at IS NULL — the
-- soft-delete convention the domain still honours. Do not add a query
-- path here that reads deleted rows without an explicit reason.

-- name: CreateAPIKey :exec
INSERT INTO api_keys (
    id, user_id, project_id, name, key_hash, key_preview,
    expires_at, last_used_at,
    created_at, updated_at, deleted_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
);

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetAPIKeyByKeyHash :one
-- Hot path: request-auth middleware hits this on every SDK call. The UNIQUE
-- index on key_hash makes it O(1).
SELECT * FROM api_keys
WHERE key_hash = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateAPIKey :exec
UPDATE api_keys
SET name         = $2,
    key_hash     = $3,
    key_preview  = $4,
    expires_at   = $5,
    last_used_at = $6,
    updated_at   = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteAPIKey :exec
-- Preserves the row for audit/forensics; request-auth ignores deleted rows.
UPDATE api_keys
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys
SET last_used_at = NOW(),
    updated_at   = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListAPIKeysByUser :many
SELECT * FROM api_keys
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC;

-- name: ListAPIKeysByProject :many
SELECT * FROM api_keys
WHERE project_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC;

-- name: ListAPIKeysByOrganization :many
-- Joins through projects.organization_id; api_keys no longer carries the
-- org FK directly (dropped in 20251005150009).
SELECT api_keys.*
FROM api_keys
JOIN projects ON api_keys.project_id = projects.id
WHERE projects.organization_id = $1
  AND api_keys.deleted_at IS NULL
ORDER BY api_keys.created_at DESC, api_keys.id DESC;

-- name: CleanupExpiredAPIKeys :execrows
-- Hard-delete keys whose expires_at is set and in the past. Unexpired keys
-- are never touched here; expires_at NULL is "never expires".
DELETE FROM api_keys
WHERE expires_at IS NOT NULL AND expires_at < NOW();

-- name: CountAPIKeysByUser :one
SELECT COUNT(*)::bigint AS count FROM api_keys
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: CountActiveAPIKeysByUser :one
SELECT COUNT(*)::bigint AS count FROM api_keys
WHERE user_id = $1
  AND deleted_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW());
