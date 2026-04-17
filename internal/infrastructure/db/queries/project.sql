-- Static queries for projects. No dynamic-filter reads exist in the
-- domain; every list path is a fixed filter (org_id).

-- name: CreateProject :exec
INSERT INTO projects (
    id, organization_id, name, description, status,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
);

-- name: GetProjectByID :one
SELECT * FROM projects
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateProject :exec
UPDATE projects
SET name        = $2,
    description = $3,
    status      = $4,
    updated_at  = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteProject :exec
UPDATE projects
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: ListProjectsByOrganization :many
SELECT * FROM projects
WHERE organization_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC, id ASC;

-- name: CountProjectsByOrganization :one
SELECT COUNT(*)::bigint AS count FROM projects
WHERE organization_id = $1 AND deleted_at IS NULL;

-- name: UserCanAccessProject :one
-- A user has access to a project iff they are a member of the project's
-- organization. Project-level membership (project_members) is orthogonal
-- and not required by this check — callers that need it should compose
-- project_members.user_id separately.
SELECT EXISTS (
    SELECT 1
    FROM projects p
    JOIN organization_members om ON om.organization_id = p.organization_id
    WHERE p.id = $1
      AND om.user_id = $2
      AND p.deleted_at IS NULL
);
