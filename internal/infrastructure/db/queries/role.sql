-- Static queries for roles (RBAC template and custom). The GORM-era
-- code Preloaded Permissions on every read; no service actually read
-- that field, so the new repository does not bundle permissions into
-- role reads. Callers fetch permissions explicitly via the permission
-- repository when they need them.

-- name: CreateRole :exec
INSERT INTO roles (
    id, name, scope_type, scope_id, description, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
);

-- name: GetRoleByID :one
SELECT * FROM roles WHERE id = $1 LIMIT 1;

-- name: GetRoleByNameAndScopeType :one
-- Template (system) roles have no scope_id; the query matches on that.
SELECT * FROM roles
WHERE name = $1
  AND scope_type = $2
  AND scope_id IS NULL
LIMIT 1;

-- name: GetRoleByNameScopeAndID :one
-- Custom-scope role lookup: same (name, scope_type) combo as above but
-- with an explicit scope_id.
SELECT * FROM roles
WHERE name = $1
  AND scope_type = $2
  AND scope_id = $3
LIMIT 1;

-- name: UpdateRole :exec
UPDATE roles
SET name        = $2,
    scope_type  = $3,
    scope_id    = $4,
    description = $5,
    updated_at  = NOW()
WHERE id = $1;

-- name: DeleteRole :exec
DELETE FROM roles WHERE id = $1;

-- name: ListRolesByScopeType :many
SELECT * FROM roles
WHERE scope_type = $1
ORDER BY name ASC;

-- name: ListAllRoles :many
SELECT * FROM roles
ORDER BY scope_type ASC, name ASC;

-- name: ListSystemRoles :many
SELECT * FROM roles
WHERE scope_type = 'system' AND scope_id IS NULL
ORDER BY name ASC;

-- name: ListCustomRolesByScopeID :many
-- Custom roles attached to a specific organization / project.
SELECT * FROM roles
WHERE scope_type = $1 AND scope_id = $2
ORDER BY name ASC;

-- name: ListCustomRolesByOrganization :many
SELECT * FROM roles
WHERE scope_type = 'organization' AND scope_id = $1
ORDER BY name ASC;

-- name: CountRoles :one
SELECT COUNT(*)::bigint AS count FROM roles;

-- name: CountRolesByScopeType :many
SELECT scope_type, COUNT(*)::bigint AS count FROM roles
GROUP BY scope_type;

-- name: CountMembersByRoleName :many
-- Returns role-name → member-count mapping used by GetRoleStatistics.
SELECT r.name AS role_name, COUNT(*)::bigint AS count
FROM organization_members om
JOIN roles r ON r.id = om.role_id
GROUP BY r.name;
