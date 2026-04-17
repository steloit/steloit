-- Static queries for organization_settings. Multi-key reads use ANY($::text[])
-- so callers don't need to wrap individual keys.

-- name: CreateOrganizationSetting :exec
INSERT INTO organization_settings (
    id, organization_id, key, value, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6
);

-- name: GetOrganizationSettingByID :one
SELECT * FROM organization_settings
WHERE id = $1
LIMIT 1;

-- name: GetOrganizationSettingByKey :one
SELECT * FROM organization_settings
WHERE organization_id = $1 AND key = $2
LIMIT 1;

-- name: ListOrganizationSettings :many
SELECT * FROM organization_settings
WHERE organization_id = $1;

-- name: ListOrganizationSettingsByKeys :many
SELECT * FROM organization_settings
WHERE organization_id = $1
  AND key = ANY($2::text[]);

-- name: UpdateOrganizationSetting :exec
UPDATE organization_settings
SET key        = $2,
    value      = $3,
    updated_at = NOW()
WHERE id = $1;

-- name: DeleteOrganizationSetting :exec
DELETE FROM organization_settings WHERE id = $1;

-- name: DeleteOrganizationSettingByKey :execrows
DELETE FROM organization_settings
WHERE organization_id = $1 AND key = $2;

-- name: DeleteOrganizationSettingsByKeys :execrows
DELETE FROM organization_settings
WHERE organization_id = $1
  AND key = ANY($2::text[]);

-- name: UpsertOrganizationSetting :one
-- Single statement handles both create-if-absent and update-in-place,
-- preserving the (organization_id, key) unique constraint. Returns the
-- final row so callers can hand it back to the domain.
INSERT INTO organization_settings (
    id, organization_id, key, value, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, NOW(), NOW()
)
ON CONFLICT (organization_id, key) DO UPDATE
SET value      = EXCLUDED.value,
    updated_at = NOW()
RETURNING *;
