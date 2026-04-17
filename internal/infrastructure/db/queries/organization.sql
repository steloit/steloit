-- Static queries for organizations. Dynamic-filter listing lives in
-- repository/organization/organization_filter.go (squirrel).

-- name: CreateOrganization :exec
INSERT INTO organizations (
    id, name, billing_email, plan, subscription_status, trial_ends_at,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: GetOrganizationByID :one
SELECT * FROM organizations
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateOrganization :exec
UPDATE organizations
SET name                = $2,
    billing_email       = $3,
    plan                = $4,
    subscription_status = $5,
    trial_ends_at       = $6,
    updated_at          = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteOrganization :exec
UPDATE organizations
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: ListOrganizationsByUser :many
-- Returns every organization the user belongs to, ordered newest first.
SELECT o.*
FROM organizations o
JOIN organization_members om ON om.organization_id = o.id
WHERE om.user_id = $1 AND o.deleted_at IS NULL
ORDER BY o.created_at DESC, o.id DESC;

-- name: ListUserOrganizationsWithProjects :many
-- Single-query fan-out of (org → role) with optional project. LEFT JOIN on
-- projects so an org with no projects still returns one row with NULL
-- project columns. Callers group by org_id in Go to hydrate the nested
-- structure.
SELECT
    o.id            AS org_id,
    o.name          AS org_name,
    o.plan          AS org_plan,
    o.created_at    AS org_created_at,
    o.updated_at    AS org_updated_at,
    r.name          AS role_name,
    p.id            AS project_id,
    p.name          AS project_name,
    p.description   AS project_description,
    p.organization_id AS project_organization_id,
    p.status        AS project_status,
    p.created_at    AS project_created_at,
    p.updated_at    AS project_updated_at
FROM organizations o
JOIN organization_members om ON om.organization_id = o.id
JOIN roles r                  ON r.id              = om.role_id
LEFT JOIN projects p          ON p.organization_id = o.id
                             AND p.deleted_at IS NULL
WHERE om.user_id = $1 AND o.deleted_at IS NULL
ORDER BY o.created_at DESC, p.created_at DESC;
