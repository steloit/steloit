-- Static queries for playground_sessions. Hybrid saved/unsaved model:
-- name IS NULL means unsaved (ephemeral UX); name IS NOT NULL means
-- saved (shows in sidebar).

-- name: CreatePlaygroundSession :exec
INSERT INTO playground_sessions (
    id, project_id,
    name, description, tags,
    variables, config, windows, last_run,
    created_by, created_at, updated_at, last_used_at
) VALUES (
    $1, $2,
    $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13
);

-- name: GetPlaygroundSessionByID :one
SELECT * FROM playground_sessions
WHERE id = $1
LIMIT 1;

-- name: ListPlaygroundSessionsByProject :many
SELECT * FROM playground_sessions
WHERE project_id = $1
ORDER BY last_used_at DESC
LIMIT $2;

-- name: ListPlaygroundSessionsByProjectAndTags :many
SELECT * FROM playground_sessions
WHERE project_id = $1
  AND tags && $2::text[]
ORDER BY last_used_at DESC
LIMIT $3;

-- name: UpdatePlaygroundSession :execrows
UPDATE playground_sessions
SET name         = $2,
    description  = $3,
    tags         = $4,
    variables    = $5,
    config       = $6,
    windows      = $7,
    last_run     = $8,
    last_used_at = $9,
    updated_at   = NOW()
WHERE id = $1;

-- name: UpdatePlaygroundSessionLastRun :execrows
UPDATE playground_sessions
SET last_run     = $2,
    last_used_at = NOW(),
    updated_at   = NOW()
WHERE id = $1;

-- name: UpdatePlaygroundSessionWindows :execrows
UPDATE playground_sessions
SET windows    = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: DeletePlaygroundSession :execrows
DELETE FROM playground_sessions
WHERE id = $1;

-- name: PlaygroundSessionExists :one
SELECT EXISTS (SELECT 1 FROM playground_sessions WHERE id = $1);

-- name: PlaygroundSessionExistsInProject :one
SELECT EXISTS (
    SELECT 1 FROM playground_sessions
    WHERE id = $1 AND project_id = $2
);
