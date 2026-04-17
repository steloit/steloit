-- Static queries for role_permissions (join table). Every mutation goes
-- through named sqlc queries; bulk operations are composed in Go using
-- TxManager.WithinTransaction so atomicity is still guaranteed without
-- GORM's .Transaction() shortcut.

-- name: CreateRolePermission :exec
-- granted_at is optional — when the caller passes NULL we fall back to the
-- column's DEFAULT NOW(). Explicit COALESCE rather than relying on the
-- default clause so sqlc sees the parameter as nullable (*time.Time).
INSERT INTO role_permissions (
    role_id,
    permission_id,
    granted_at,
    granted_by
) VALUES (
    @role_id,
    @permission_id,
    COALESCE(sqlc.narg('granted_at')::timestamp, NOW()),
    sqlc.narg('granted_by')::uuid
);

-- name: ListRolePermissionsByRoleID :many
SELECT * FROM role_permissions
WHERE role_id = $1;

-- name: ListRolePermissionsByPermissionID :many
SELECT * FROM role_permissions
WHERE permission_id = $1;

-- name: DeleteRolePermission :exec
DELETE FROM role_permissions
WHERE role_id = $1 AND permission_id = $2;

-- name: DeleteRolePermissionsByRoleID :execrows
DELETE FROM role_permissions
WHERE role_id = $1;

-- name: DeleteRolePermissionsByPermissionID :execrows
DELETE FROM role_permissions
WHERE permission_id = $1;

-- name: RolePermissionExists :one
SELECT EXISTS (
    SELECT 1 FROM role_permissions
    WHERE role_id = $1 AND permission_id = $2
);

-- name: DeleteRolePermissionsForRoleIn :execrows
-- Revokes a specific set of permissions from a role in one statement.
-- Used by RevokePermissions so we avoid a query-per-permission round trip.
DELETE FROM role_permissions
WHERE role_id = @role_id
  AND permission_id = ANY(@permission_ids::uuid[]);

-- name: RoleHasResourceAction :one
-- True iff the role has a permission matching the given resource:action pair.
-- Composed against the separate (resource, action) columns on permissions —
-- there is no stored resource_action aggregate column. Input parsing is
-- done in Go so callers keep their single-string public API.
SELECT EXISTS (
    SELECT 1
    FROM role_permissions rp
    JOIN permissions p ON p.id = rp.permission_id
    WHERE rp.role_id = @role_id
      AND p.resource = @resource
      AND p.action = @action
);

-- name: ListResourceActionsForRole :many
-- Returns every resource_action the role has, composed from the split
-- columns. Callers intersect with their "wanted" list in Go — cheaper than
-- passing variadic resource/action pairs into SQL. The ::text cast gives
-- sqlc a concrete return type instead of interface{}.
SELECT (p.resource || ':' || p.action)::text AS resource_action
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE rp.role_id = $1;

-- name: CountRolePermissionsByRole :one
SELECT COUNT(*)::bigint AS count FROM role_permissions
WHERE role_id = $1;

-- name: CountRolePermissionsByPermission :one
SELECT COUNT(*)::bigint AS count FROM role_permissions
WHERE permission_id = $1;
