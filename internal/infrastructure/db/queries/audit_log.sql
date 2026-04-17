-- Static queries for audit_logs. The filter-based search is dynamic and
-- lives in repository/auth/audit_log_filter.go (squirrel).

-- name: CreateAuditLog :exec
INSERT INTO audit_logs (
    id,
    user_id,
    organization_id,
    action,
    resource,
    resource_id,
    metadata,
    ip_address,
    user_agent,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
);

-- name: GetAuditLogByID :one
SELECT * FROM audit_logs
WHERE id = $1
LIMIT 1;

-- name: ListAuditLogsByUser :many
SELECT * FROM audit_logs
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsByOrganization :many
SELECT * FROM audit_logs
WHERE organization_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsByResource :many
SELECT * FROM audit_logs
WHERE resource = $1 AND resource_id = $2
ORDER BY created_at DESC, id DESC
LIMIT $3 OFFSET $4;

-- name: ListAuditLogsByAction :many
SELECT * FROM audit_logs
WHERE action = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsByDateRange :many
SELECT * FROM audit_logs
WHERE created_at BETWEEN $1 AND $2
ORDER BY created_at DESC, id DESC
LIMIT $3 OFFSET $4;

-- name: CleanupAuditLogsOlderThan :execrows
DELETE FROM audit_logs WHERE created_at < $1;

-- name: CountAuditLogs :one
SELECT COUNT(*)::bigint AS count FROM audit_logs;

-- name: CountAuditLogsByUser :one
SELECT COUNT(*)::bigint AS count FROM audit_logs WHERE user_id = $1;

-- name: CountAuditLogsByOrganization :one
SELECT COUNT(*)::bigint AS count FROM audit_logs WHERE organization_id = $1;

-- name: CountAuditLogsByActionGroup :many
-- Aggregate action histograms for GetAuditLogStats. Applies an optional
-- user_id filter (NULL = platform-wide).
SELECT action, COUNT(*)::bigint AS count
FROM audit_logs
WHERE (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('organization_id')::uuid IS NULL OR organization_id = sqlc.narg('organization_id'))
GROUP BY action;

-- name: CountAuditLogsByResourceGroup :many
-- Mirror of the action histogram — only over rows with a resource value
-- (the legacy GORM query skipped NULL/''; both are filtered here).
SELECT resource, COUNT(*)::bigint AS count
FROM audit_logs
WHERE resource <> ''
  AND (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('organization_id')::uuid IS NULL OR organization_id = sqlc.narg('organization_id'))
GROUP BY resource;

-- name: GetLatestAuditLogTime :one
SELECT created_at FROM audit_logs
ORDER BY created_at DESC
LIMIT 1;
