-- Static queries for prompts, prompt_versions, prompt_labels, and
-- prompt_protected_labels. Dynamic list (tags, search) uses squirrel.

-- ----- prompts -----------------------------------------------------

-- name: CreatePrompt :exec
INSERT INTO prompts (
    id, project_id, name, description, type, tags,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    NOW(), NOW()
);

-- name: GetPromptByID :one
SELECT * FROM prompts
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetPromptByName :one
SELECT * FROM prompts
WHERE project_id = $1 AND name = $2 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdatePrompt :exec
UPDATE prompts
SET name        = $2,
    description = $3,
    type        = $4,
    tags        = $5,
    updated_at  = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: HardDeletePrompt :exec
DELETE FROM prompts WHERE id = $1;

-- name: SoftDeletePrompt :exec
UPDATE prompts
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: RestorePrompt :exec
UPDATE prompts
SET deleted_at = NULL,
    updated_at = NOW()
WHERE id = $1;

-- name: CountPromptsByProject :one
SELECT COUNT(*)::bigint FROM prompts
WHERE project_id = $1 AND deleted_at IS NULL;

-- ----- prompt_versions ---------------------------------------------

-- name: CreatePromptVersion :exec
INSERT INTO prompt_versions (
    id, prompt_id, version, template, variables,
    commit_message, created_by, config, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, NOW()
);

-- name: GetPromptVersionByID :one
SELECT * FROM prompt_versions
WHERE id = $1
LIMIT 1;

-- name: DeletePromptVersion :exec
DELETE FROM prompt_versions WHERE id = $1;

-- name: GetPromptVersionByPromptAndVersion :one
SELECT * FROM prompt_versions
WHERE prompt_id = $1 AND version = $2
LIMIT 1;

-- name: GetLatestPromptVersion :one
SELECT * FROM prompt_versions
WHERE prompt_id = $1
ORDER BY version DESC
LIMIT 1;

-- name: ListPromptVersions :many
SELECT * FROM prompt_versions
WHERE prompt_id = $1
ORDER BY version DESC;

-- name: GetNextPromptVersionNumber :one
-- Atomic next-version reservation. Subquery + FOR UPDATE locks the
-- prompt's version rows so concurrent Create() calls serialize to
-- successive integers instead of colliding on the unique constraint.
SELECT COALESCE(MAX(version), 0) + 1
FROM (
    SELECT version FROM prompt_versions
    WHERE prompt_id = $1
    FOR UPDATE
) locked;

-- name: CountPromptVersions :one
SELECT COUNT(*)::bigint FROM prompt_versions
WHERE prompt_id = $1;

-- name: GetLatestPromptVersionsForPrompts :many
-- DISTINCT ON (prompt_id) keeps the row with the highest version per
-- prompt — single pass, no correlated subquery.
SELECT DISTINCT ON (prompt_id) *
FROM prompt_versions
WHERE prompt_id = ANY($1::uuid[])
ORDER BY prompt_id, version DESC;

-- name: ListPromptVersionsByIDs :many
SELECT * FROM prompt_versions
WHERE id = ANY($1::uuid[]);

-- ----- prompt_labels -----------------------------------------------

-- name: CreatePromptLabel :exec
INSERT INTO prompt_labels (
    id, prompt_id, version_id, name, created_by,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    NOW(), NOW()
);

-- name: GetPromptLabelByID :one
SELECT * FROM prompt_labels
WHERE id = $1
LIMIT 1;

-- name: UpdatePromptLabel :exec
UPDATE prompt_labels
SET version_id = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: DeletePromptLabel :exec
DELETE FROM prompt_labels WHERE id = $1;

-- name: GetPromptLabelByPromptAndName :one
SELECT * FROM prompt_labels
WHERE prompt_id = $1 AND name = $2
LIMIT 1;

-- name: ListPromptLabelsByPrompt :many
SELECT * FROM prompt_labels
WHERE prompt_id = $1
ORDER BY name ASC;

-- name: ListPromptLabelsByPrompts :many
SELECT * FROM prompt_labels
WHERE prompt_id = ANY($1::uuid[])
ORDER BY prompt_id ASC, name ASC;

-- name: ListPromptLabelsByVersion :many
SELECT * FROM prompt_labels
WHERE version_id = $1
ORDER BY name ASC;

-- name: ListPromptLabelsByVersions :many
SELECT * FROM prompt_labels
WHERE version_id = ANY($1::uuid[])
ORDER BY version_id ASC, name ASC;

-- name: DeletePromptLabelByName :execrows
DELETE FROM prompt_labels
WHERE prompt_id = $1 AND name = $2;

-- name: DeletePromptLabelsByPrompt :exec
DELETE FROM prompt_labels
WHERE prompt_id = $1;

-- ----- prompt_protected_labels -------------------------------------

-- name: CreateProtectedPromptLabel :exec
INSERT INTO prompt_protected_labels (
    id, project_id, label_name, created_by, created_at
) VALUES (
    $1, $2, $3, $4, NOW()
);

-- name: DeleteProtectedPromptLabel :exec
DELETE FROM prompt_protected_labels WHERE id = $1;

-- name: GetProtectedPromptLabelByProjectAndLabel :one
SELECT * FROM prompt_protected_labels
WHERE project_id = $1 AND label_name = $2
LIMIT 1;

-- name: ListProtectedPromptLabelsByProject :many
SELECT * FROM prompt_protected_labels
WHERE project_id = $1
ORDER BY label_name ASC;

-- name: ProtectedPromptLabelExists :one
SELECT EXISTS (
    SELECT 1 FROM prompt_protected_labels
    WHERE project_id = $1 AND label_name = $2
);

-- name: DeleteProtectedPromptLabelsByProject :exec
DELETE FROM prompt_protected_labels
WHERE project_id = $1;
