-- Static queries for the blacklisted_tokens table.
-- Dynamic filter listings live in repository/auth/blacklisted_token_filter.go
-- (squirrel-built) because BlacklistedTokenFilter has >3 optional predicates.

-- name: CreateBlacklistedToken :exec
INSERT INTO blacklisted_tokens (
    jti,
    user_id,
    expires_at,
    revoked_at,
    reason,
    token_type,
    blacklist_timestamp,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: GetBlacklistedTokenByJTI :one
SELECT * FROM blacklisted_tokens
WHERE jti = $1
LIMIT 1;

-- name: IsTokenBlacklisted :one
-- Fast path for request-auth middleware. A row is considered "active" only
-- while expires_at is in the future; expired rows are cleanup candidates.
SELECT EXISTS (
    SELECT 1 FROM blacklisted_tokens
    WHERE jti = $1 AND expires_at > NOW()
);

-- name: CleanupExpiredBlacklistedTokens :execrows
DELETE FROM blacklisted_tokens
WHERE expires_at <= NOW();

-- name: CleanupBlacklistedTokensOlderThan :execrows
DELETE FROM blacklisted_tokens
WHERE created_at < $1;

-- name: IsUserBlacklistedAfterTimestamp :one
-- GDPR/SOC2 "revoke all sessions" primitive: any token issued before the most
-- recent user-wide timestamp blacklist for this user is invalid.
SELECT EXISTS (
    SELECT 1 FROM blacklisted_tokens
    WHERE user_id = @user_id
      AND token_type = 'user_wide_timestamp'
      AND blacklist_timestamp IS NOT NULL
      AND @token_issued_at::bigint < blacklist_timestamp
      AND expires_at > NOW()
);

-- name: GetUserBlacklistTimestamp :one
-- Latest user-wide blacklist timestamp, used to decide whether an incoming
-- JWT (with its iat claim) predates the user's most recent revoke-all call.
SELECT blacklist_timestamp
FROM blacklisted_tokens
WHERE user_id = $1
  AND token_type = 'user_wide_timestamp'
  AND blacklist_timestamp IS NOT NULL
  AND expires_at > NOW()
ORDER BY blacklist_timestamp DESC
LIMIT 1;

-- name: CountBlacklistedTokens :one
SELECT COUNT(*) FROM blacklisted_tokens;

-- name: ListBlacklistedTokensByReason :many
SELECT * FROM blacklisted_tokens
WHERE reason = $1
ORDER BY created_at DESC;
