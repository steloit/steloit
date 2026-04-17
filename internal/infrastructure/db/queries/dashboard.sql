-- Static queries for dashboards and dashboard_templates.
-- Dashboards soft-delete via deleted_at.

-- name: CreateDashboard :exec
INSERT INTO dashboards (
    id, project_id, name, description,
    config, layout, is_locked,
    created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, NOW(), NOW()
);

-- name: GetDashboardByID :one
SELECT * FROM dashboards
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetDashboardByNameAndProject :one
SELECT * FROM dashboards
WHERE project_id = $1 AND name = $2 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateDashboard :exec
UPDATE dashboards
SET name        = $2,
    description = $3,
    config      = $4,
    layout      = $5,
    is_locked   = $6,
    updated_at  = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: HardDeleteDashboard :exec
DELETE FROM dashboards WHERE id = $1;

-- name: SoftDeleteDashboard :exec
UPDATE dashboards
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: CountDashboardsByProject :one
SELECT COUNT(*)::bigint AS count FROM dashboards
WHERE project_id = $1 AND deleted_at IS NULL;

-- ----- dashboard_templates ------------------------------------------

-- name: CreateDashboardTemplate :exec
INSERT INTO dashboard_templates (
    id, name, description, category,
    config, layout, is_active,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    NOW(), NOW()
);

-- name: GetDashboardTemplateByID :one
SELECT * FROM dashboard_templates
WHERE id = $1
LIMIT 1;

-- name: GetDashboardTemplateByName :one
SELECT * FROM dashboard_templates
WHERE name = $1
LIMIT 1;

-- name: GetActiveDashboardTemplateByCategory :one
SELECT * FROM dashboard_templates
WHERE category = $1 AND is_active = TRUE
LIMIT 1;

-- name: UpdateDashboardTemplate :exec
UPDATE dashboard_templates
SET name        = $2,
    description = $3,
    category    = $4,
    config      = $5,
    layout      = $6,
    is_active   = $7,
    updated_at  = NOW()
WHERE id = $1;

-- name: DeleteDashboardTemplate :execrows
DELETE FROM dashboard_templates
WHERE id = $1;

-- name: UpsertDashboardTemplateByName :exec
-- Used by the seeder: ON CONFLICT (name) overwrites everything so a
-- re-run with updated config replaces the row.
INSERT INTO dashboard_templates (
    id, name, description, category,
    config, layout, is_active,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    NOW(), NOW()
)
ON CONFLICT (name) DO UPDATE SET
    description = EXCLUDED.description,
    category    = EXCLUDED.category,
    config      = EXCLUDED.config,
    layout      = EXCLUDED.layout,
    is_active   = EXCLUDED.is_active,
    updated_at  = NOW();
