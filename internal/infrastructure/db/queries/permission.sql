-- Static queries for permissions. Dynamic-search (ILIKE) lives in
-- repository/auth/permission_filter.go (squirrel).
--
-- permissions.resource_action is a composed value (resource || ':' || action),
-- not a stored column. Callers that need the string form compose it in Go
-- or use the @resource + @action parameter pair below.

-- name: CreatePermission :exec
INSERT INTO permissions (
    id, name, resource, action, description, scope_level, category, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: GetPermissionByID :one
SELECT * FROM permissions WHERE id = $1 LIMIT 1;

-- name: GetPermissionByName :one
SELECT * FROM permissions WHERE name = $1 LIMIT 1;

-- name: GetPermissionByResourceAction :one
SELECT * FROM permissions
WHERE resource = $1 AND action = $2
LIMIT 1;

-- name: ListAllPermissions :many
SELECT * FROM permissions
ORDER BY category NULLS LAST, resource ASC, action ASC, id ASC;

-- name: ListPermissionsByNames :many
SELECT * FROM permissions
WHERE name = ANY($1::text[])
ORDER BY resource ASC, action ASC;

-- name: ListPermissionsByResource :many
SELECT * FROM permissions
WHERE resource = $1
ORDER BY action ASC;

-- name: ListPermissionsByResourceActions :many
-- Matches rows whose composed "resource:action" string appears in the
-- input. Callers stringify pairs in Go so there is no need to send two
-- parallel arrays.
SELECT *
FROM permissions
WHERE resource || ':' || action = ANY($1::text[])
ORDER BY resource ASC, action ASC;

-- name: ListPermissionsByScopeLevel :many
SELECT * FROM permissions
WHERE scope_level = $1
ORDER BY category NULLS LAST, resource ASC, action ASC;

-- name: ListPermissionsByCategory :many
SELECT * FROM permissions
WHERE category = $1
ORDER BY resource ASC, action ASC;

-- name: ListPermissionsByScopeAndCategory :many
SELECT * FROM permissions
WHERE scope_level = $1 AND category = $2
ORDER BY resource ASC, action ASC;

-- name: ListDistinctResources :many
SELECT DISTINCT resource FROM permissions
WHERE resource <> ''
ORDER BY resource ASC;

-- name: ListActionsForResource :many
SELECT action FROM permissions
WHERE resource = $1
ORDER BY action ASC;

-- name: ListDistinctCategories :many
SELECT DISTINCT category FROM permissions
WHERE category IS NOT NULL AND category <> ''
ORDER BY category ASC;

-- name: UpdatePermission :exec
UPDATE permissions
SET name        = $2,
    resource    = $3,
    action      = $4,
    description = $5,
    scope_level = $6,
    category    = $7
WHERE id = $1;

-- name: DeletePermission :exec
-- Hard delete. The schema has no deleted_at column; archival would need a
-- separate table.
DELETE FROM permissions WHERE id = $1;

-- name: DeletePermissionsIn :execrows
DELETE FROM permissions
WHERE id = ANY($1::uuid[]);

-- name: CountPermissions :one
SELECT COUNT(*)::bigint AS count FROM permissions;

-- name: ExistsPermissionByResourceAction :one
SELECT EXISTS (
    SELECT 1 FROM permissions
    WHERE resource = $1 AND action = $2
);

-- name: ListExistingResourceActions :many
-- Given an input set of "resource:action" strings, return those that exist.
-- Callers build a wanted → granted map in Go for O(1) lookup.
SELECT (resource || ':' || action)::text AS resource_action
FROM permissions
WHERE resource || ':' || action = ANY($1::text[]);

-- name: ListResourceActionsForRolePermissions :many
-- Every resource:action string the role holds. Used for RolePermissionMap.
SELECT (p.resource || ':' || p.action)::text AS resource_action
FROM permissions p
JOIN role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1;

-- name: ListPermissionsForRole :many
SELECT p.*
FROM permissions p
JOIN role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1
ORDER BY p.category NULLS LAST, p.resource ASC, p.action ASC;

-- name: ListUserPermissionsInOrg :many
-- All permissions granted to a user via their org-level role(s).
SELECT p.*
FROM permissions p
JOIN role_permissions rp       ON rp.permission_id  = p.id
JOIN organization_members om   ON om.role_id        = rp.role_id
WHERE om.user_id = $1
  AND om.organization_id = $2
ORDER BY p.category NULLS LAST, p.resource ASC, p.action ASC;

-- name: ListUserResourceActionsInOrg :many
SELECT (p.resource || ':' || p.action)::text AS resource_action
FROM permissions p
JOIN role_permissions rp       ON rp.permission_id  = p.id
JOIN organization_members om   ON om.role_id        = rp.role_id
WHERE om.user_id = $1
  AND om.organization_id = $2;

-- name: ListPermissionNamesForAPIKey :many
-- API keys inherit permissions through their user's role in the key's
-- project organisation. api_keys.organization_id was dropped in
-- 20251005150009, so we join through projects.organization_id instead.
SELECT p.name
FROM permissions p
JOIN role_permissions rp      ON rp.permission_id   = p.id
JOIN organization_members om  ON om.role_id         = rp.role_id
JOIN projects pr              ON pr.organization_id = om.organization_id
JOIN api_keys ak              ON ak.user_id         = om.user_id
                             AND ak.project_id      = pr.id
WHERE ak.id = $1
  AND ak.deleted_at IS NULL;
