-- Static queries for the user aggregate (users + user_profiles).
-- Dynamic filter/search queries are built via squirrel in user_filter.go.

-- name: CreateUser :exec
INSERT INTO users (
    id, email, first_name, last_name, password,
    is_active, is_email_verified, email_verified_at,
    timezone, language, last_login_at, login_count,
    default_organization_id, role, referral_source,
    auth_method, oauth_provider, oauth_provider_id,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $15,
    $16, $17, $18,
    $19, $20
);

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateUser :exec
UPDATE users
SET email                   = $2,
    first_name              = $3,
    last_name               = $4,
    password                = $5,
    is_active               = $6,
    is_email_verified       = $7,
    email_verified_at       = $8,
    timezone                = $9,
    language                = $10,
    last_login_at           = $11,
    login_count             = $12,
    default_organization_id = $13,
    role                    = $14,
    referral_source         = $15,
    auth_method             = $16,
    oauth_provider          = $17,
    oauth_provider_id       = $18,
    updated_at              = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteUser :exec
UPDATE users
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserPassword :exec
UPDATE users
SET password   = $2,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login_at = NOW(),
    login_count   = login_count + 1,
    updated_at    = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: MarkUserEmailVerified :exec
UPDATE users
SET is_email_verified = TRUE,
    email_verified_at = NOW(),
    updated_at        = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetUserDefaultOrganization :exec
UPDATE users
SET default_organization_id = $2,
    updated_at              = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserDefaultOrganization :one
SELECT default_organization_id FROM users
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: SetUserActive :exec
UPDATE users
SET is_active  = $2,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsersByIDs :many
SELECT * FROM users
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;

-- name: CountActiveUsers :one
SELECT COUNT(*)::bigint AS count FROM users
WHERE deleted_at IS NULL;

-- name: CountUsersLoggedInSince :one
SELECT COUNT(*)::bigint AS count FROM users
WHERE deleted_at IS NULL
  AND last_login_at IS NOT NULL
  AND last_login_at > $1;

-- name: CountVerifiedUsers :one
SELECT COUNT(*)::bigint AS count FROM users
WHERE deleted_at IS NULL
  AND is_email_verified = TRUE;

-- name: CountUsersCreatedSince :one
SELECT COUNT(*)::bigint AS count FROM users
WHERE deleted_at IS NULL
  AND created_at >= $1;

-- name: ListUsersByOrganization :many
SELECT u.* FROM users u
JOIN organization_members om
  ON om.user_id = u.id
 AND om.deleted_at IS NULL
WHERE om.organization_id = $1
  AND u.deleted_at IS NULL
ORDER BY u.created_at DESC;

-- ----- user_profiles -------------------------------------------------

-- name: CreateUserProfile :exec
INSERT INTO user_profiles (
    user_id, bio, location, website,
    twitter_url, linkedin_url, github_url, avatar_url,
    phone, timezone, language, theme,
    email_notifications, push_notifications, marketing_emails,
    weekly_reports, monthly_reports, security_alerts, billing_alerts,
    usage_threshold_percent, created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $15,
    $16, $17, $18, $19,
    $20, $21, $22
);

-- name: GetUserProfile :one
SELECT * FROM user_profiles
WHERE user_id = $1
LIMIT 1;

-- name: UpdateUserProfile :exec
UPDATE user_profiles
SET bio                     = $2,
    location                = $3,
    website                 = $4,
    twitter_url             = $5,
    linkedin_url            = $6,
    github_url              = $7,
    avatar_url              = $8,
    phone                   = $9,
    timezone                = $10,
    language                = $11,
    theme                   = $12,
    email_notifications     = $13,
    push_notifications      = $14,
    marketing_emails        = $15,
    weekly_reports          = $16,
    monthly_reports         = $17,
    security_alerts         = $18,
    billing_alerts          = $19,
    usage_threshold_percent = $20,
    updated_at              = NOW()
WHERE user_id = $1;
