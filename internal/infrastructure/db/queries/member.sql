-- Static queries for organization_members. Composite PK is
-- (user_id, organization_id). Soft-delete via deleted_at.

-- name: CreateMember :exec
INSERT INTO organization_members (
    user_id, organization_id, role_id, status, joined_at, invited_by,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: GetMemberByUserAndOrg :one
SELECT * FROM organization_members
WHERE user_id = $1 AND organization_id = $2 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateMember :exec
UPDATE organization_members
SET role_id    = $3,
    status     = $4,
    invited_by = $5,
    updated_at = NOW()
WHERE user_id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: UpdateMemberRole :exec
UPDATE organization_members
SET role_id    = $3,
    updated_at = NOW()
WHERE user_id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteMemberByUserAndOrg :exec
UPDATE organization_members
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE user_id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: ListMembersByOrganization :many
SELECT * FROM organization_members
WHERE organization_id = $1 AND deleted_at IS NULL
ORDER BY joined_at ASC, user_id ASC;

-- name: ListMembersByUser :many
SELECT * FROM organization_members
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY joined_at ASC, organization_id ASC;

-- name: ListMembersByOrganizationAndRole :many
SELECT * FROM organization_members
WHERE organization_id = $1 AND role_id = $2 AND deleted_at IS NULL
ORDER BY joined_at ASC;

-- name: IsMember :one
SELECT EXISTS (
    SELECT 1 FROM organization_members
    WHERE user_id = $1 AND organization_id = $2 AND deleted_at IS NULL
);

-- name: GetMemberRoleID :one
SELECT role_id FROM organization_members
WHERE user_id = $1 AND organization_id = $2 AND deleted_at IS NULL
LIMIT 1;

-- name: CountMembersByOrganization :one
SELECT COUNT(*)::bigint AS count FROM organization_members
WHERE organization_id = $1 AND deleted_at IS NULL;

-- name: CountMembersByOrganizationAndRole :one
SELECT COUNT(*)::bigint AS count FROM organization_members
WHERE organization_id = $1 AND role_id = $2 AND deleted_at IS NULL;

-- name: ListMembersByRole :many
SELECT * FROM organization_members
WHERE role_id = $1 AND deleted_at IS NULL
ORDER BY organization_id ASC, user_id ASC;

-- name: ListActiveMembersByOrganization :many
SELECT * FROM organization_members
WHERE organization_id = $1
  AND status = 'active'
  AND deleted_at IS NULL
ORDER BY joined_at ASC, user_id ASC;

-- name: UpdateMemberStatus :exec
UPDATE organization_members
SET status     = $3,
    updated_at = NOW()
WHERE user_id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: CountActiveMembersByOrganization :one
SELECT COUNT(*)::bigint AS count FROM organization_members
WHERE organization_id = $1
  AND status = 'active'
  AND deleted_at IS NULL;

-- name: CountActiveMembersByRoleName :many
SELECT r.name        AS role_name,
       COUNT(*)::bigint AS count
FROM organization_members om
JOIN roles r ON r.id = om.role_id
WHERE om.organization_id = $1
  AND om.status = 'active'
  AND om.deleted_at IS NULL
GROUP BY r.name;

-- name: ListUserEffectivePermissionsGlobal :many
SELECT DISTINCT (p.resource || ':' || p.action)::text AS name
FROM organization_members om
JOIN roles r             ON r.id  = om.role_id
JOIN role_permissions rp ON rp.role_id = r.id
JOIN permissions p       ON p.id  = rp.permission_id
WHERE om.user_id = $1
  AND om.status = 'active'
  AND om.deleted_at IS NULL;

-- name: ListUserEffectivePermissionsInOrg :many
SELECT DISTINCT (p.resource || ':' || p.action)::text AS name
FROM organization_members om
JOIN roles r             ON r.id  = om.role_id
JOIN role_permissions rp ON rp.role_id = r.id
JOIN permissions p       ON p.id  = rp.permission_id
WHERE om.user_id = $1
  AND om.organization_id = $2
  AND om.status = 'active'
  AND om.deleted_at IS NULL;

-- name: UserHasPermissionGlobal :one
SELECT EXISTS (
    SELECT 1
    FROM organization_members om
    JOIN roles r             ON r.id  = om.role_id
    JOIN role_permissions rp ON rp.role_id = r.id
    JOIN permissions p       ON p.id  = rp.permission_id
    WHERE om.user_id = $1
      AND om.status = 'active'
      AND om.deleted_at IS NULL
      AND (p.resource || ':' || p.action)::text = $2::text
);

-- name: BulkUpdateMemberRoles :exec
UPDATE organization_members AS om
SET role_id    = v.role_id,
    updated_at = NOW()
FROM (
    SELECT UNNEST($1::uuid[]) AS user_id,
           UNNEST($2::uuid[]) AS organization_id,
           UNNEST($3::uuid[]) AS role_id
) AS v
WHERE om.user_id = v.user_id
  AND om.organization_id = v.organization_id
  AND om.deleted_at IS NULL;
