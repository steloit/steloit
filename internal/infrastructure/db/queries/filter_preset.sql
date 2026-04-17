-- Static queries for filter_presets (saved UI filters for traces/spans).
-- Dynamic list/count uses squirrel due to variable visibility predicates.

-- name: CreateFilterPreset :exec
INSERT INTO filter_presets (
    id, project_id, name, description,
    table_name, filters, column_order, column_visibility,
    search_query, search_types,
    is_public, created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10,
    $11, $12, NOW(), NOW()
);

-- name: GetFilterPresetByID :one
SELECT * FROM filter_presets
WHERE id = $1
LIMIT 1;

-- name: UpdateFilterPreset :execrows
UPDATE filter_presets
SET name              = $2,
    description       = $3,
    filters           = $4,
    column_order      = $5,
    column_visibility = $6,
    search_query      = $7,
    search_types      = $8,
    is_public         = $9,
    updated_at        = NOW()
WHERE id = $1;

-- name: DeleteFilterPreset :execrows
DELETE FROM filter_presets
WHERE id = $1;
