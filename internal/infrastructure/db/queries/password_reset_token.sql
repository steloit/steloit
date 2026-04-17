-- Static queries for password_reset_tokens. No filter listings required;
-- the repository exposes fixed read paths only.

-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (
    id,
    user_id,
    token,
    expires_at,
    used_at,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
);

-- name: GetPasswordResetTokenByID :one
SELECT * FROM password_reset_tokens
WHERE id = $1
LIMIT 1;

-- name: GetPasswordResetTokenByToken :one
SELECT * FROM password_reset_tokens
WHERE token = $1
LIMIT 1;

-- name: ListPasswordResetTokensByUser :many
SELECT * FROM password_reset_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: UpdatePasswordResetToken :exec
UPDATE password_reset_tokens
SET token      = $2,
    expires_at = $3,
    used_at    = $4,
    updated_at = NOW()
WHERE id = $1;

-- name: DeletePasswordResetToken :exec
DELETE FROM password_reset_tokens
WHERE id = $1;

-- name: MarkPasswordResetTokenAsUsed :exec
-- Sets both used_at and updated_at atomically to the current DB time so the
-- audit trail is consistent regardless of clock skew on the caller.
UPDATE password_reset_tokens
SET used_at    = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: IsPasswordResetTokenUsed :one
SELECT EXISTS (
    SELECT 1 FROM password_reset_tokens
    WHERE id = $1 AND used_at IS NOT NULL
);

-- name: IsPasswordResetTokenValid :one
-- "Valid" = row exists, not yet consumed, not yet expired.
SELECT EXISTS (
    SELECT 1 FROM password_reset_tokens
    WHERE id = $1
      AND used_at IS NULL
      AND expires_at > NOW()
);

-- name: GetValidPasswordResetTokenByUser :one
SELECT * FROM password_reset_tokens
WHERE user_id = $1
  AND used_at IS NULL
  AND expires_at > NOW()
ORDER BY created_at DESC
LIMIT 1;

-- name: CleanupExpiredPasswordResetTokens :execrows
DELETE FROM password_reset_tokens
WHERE expires_at < NOW();

-- name: CleanupUsedPasswordResetTokens :execrows
-- @older_than is typed through sqlc's default timestamptz → time.Time
-- override; the nullable `used_at` column would otherwise coerce the
-- parameter to *time.Time.
DELETE FROM password_reset_tokens
WHERE used_at IS NOT NULL AND used_at < sqlc.arg('older_than')::timestamp with time zone;

-- name: InvalidateUserPasswordResetTokens :execrows
-- Marks every outstanding (unused) token for a user as used. Used by the
-- auth service whenever a password is changed so old reset links cannot
-- still be redeemed.
UPDATE password_reset_tokens
SET used_at    = NOW(),
    updated_at = NOW()
WHERE user_id = $1 AND used_at IS NULL;
