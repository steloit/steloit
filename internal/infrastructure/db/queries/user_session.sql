-- Static queries for user_sessions. No dynamic filter listings — every
-- read path is a fixed lookup (by id, JTI, refresh hash, user_id).

-- name: CreateUserSession :exec
INSERT INTO user_sessions (
    id,
    user_id,
    refresh_token_hash,
    refresh_token_version,
    current_jti,
    expires_at,
    refresh_expires_at,
    ip_address,
    user_agent,
    device_info,
    is_active,
    last_used_at,
    revoked_at,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
);

-- name: GetUserSessionByID :one
SELECT * FROM user_sessions
WHERE id = $1
LIMIT 1;

-- name: GetUserSessionByJTI :one
-- Only returns live sessions: request auth looks up the current JWT's JTI
-- here on every authenticated call, so the filter is critical for cost.
SELECT * FROM user_sessions
WHERE current_jti = $1
  AND is_active = TRUE
  AND revoked_at IS NULL
LIMIT 1;

-- name: GetUserSessionByRefreshTokenHash :one
SELECT * FROM user_sessions
WHERE refresh_token_hash = $1
  AND is_active = TRUE
  AND revoked_at IS NULL
LIMIT 1;

-- name: ListUserSessionsByUser :many
SELECT * FROM user_sessions
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListActiveUserSessionsByUser :many
SELECT * FROM user_sessions
WHERE user_id = $1
  AND is_active = TRUE
  AND revoked_at IS NULL
  AND expires_at > NOW()
ORDER BY last_used_at DESC NULLS LAST, created_at DESC;

-- name: UpdateUserSession :exec
-- Full replace — matches the GORM-era Save() semantics. Token rotation
-- flows use this; everyday last_used_at touches go through MarkUserSessionUsed.
UPDATE user_sessions
SET refresh_token_hash    = $2,
    refresh_token_version = $3,
    current_jti           = $4,
    expires_at            = $5,
    refresh_expires_at    = $6,
    ip_address            = $7,
    user_agent            = $8,
    device_info           = $9,
    is_active             = $10,
    last_used_at          = $11,
    revoked_at            = $12,
    updated_at            = NOW()
WHERE id = $1;

-- name: DeleteUserSession :exec
DELETE FROM user_sessions
WHERE id = $1;

-- name: DeactivateUserSession :exec
UPDATE user_sessions
SET is_active  = FALSE,
    updated_at = NOW()
WHERE id = $1;

-- name: DeactivateUserSessionsForUser :execrows
UPDATE user_sessions
SET is_active  = FALSE,
    updated_at = NOW()
WHERE user_id = $1;

-- name: RevokeUserSession :exec
-- Revocation is the stronger lifecycle event than deactivation: blacklist
-- check trusts revoked_at, and CleanupRevokedUserSessions sweeps by it.
UPDATE user_sessions
SET revoked_at = NOW(),
    is_active  = FALSE,
    updated_at = NOW()
WHERE id = $1;

-- name: RevokeUserSessionsForUser :execrows
UPDATE user_sessions
SET revoked_at = NOW(),
    is_active  = FALSE,
    updated_at = NOW()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: CleanupExpiredUserSessions :execrows
DELETE FROM user_sessions
WHERE expires_at < NOW();

-- name: CleanupRevokedUserSessions :execrows
-- Keep revoked rows for a 30-day audit window. Older rows are swept by
-- the session lifecycle cleanup worker.
DELETE FROM user_sessions
WHERE revoked_at IS NOT NULL
  AND revoked_at < NOW() - INTERVAL '30 days';

-- name: MarkUserSessionUsed :exec
UPDATE user_sessions
SET last_used_at = NOW(),
    updated_at   = NOW()
WHERE id = $1;

-- name: ListUserSessionsByDeviceInfo :many
-- Device match is exact-JSON equality. Callers stringify the device info
-- in Go so sqlc doesn't need to know about its shape.
SELECT * FROM user_sessions
WHERE user_id    = $1
  AND device_info = @device_info::jsonb;

-- name: CountActiveUserSessions :one
SELECT COUNT(*)::bigint AS count FROM user_sessions
WHERE user_id = $1
  AND is_active = TRUE
  AND revoked_at IS NULL
  AND expires_at > NOW();
