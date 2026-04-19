-- Static queries for user_invitations and invitation_audit_events.
-- Soft-delete via deleted_at. Token lookup uses SHA-256 hash for security.

-- name: CreateInvitation :exec
INSERT INTO user_invitations (
    id, organization_id, role_id, email, status,
    expires_at, created_at, updated_at,
    invited_by_id, token_hash, token_preview, message,
    resent_count
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12,
    $13
);

-- name: GetInvitationByID :one
SELECT * FROM user_invitations
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetInvitationByTokenHash :one
SELECT * FROM user_invitations
WHERE token_hash = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateInvitation :exec
UPDATE user_invitations
SET organization_id = $2,
    role_id         = $3,
    email           = $4,
    status          = $5,
    expires_at      = $6,
    accepted_at     = $7,
    revoked_at      = $8,
    resent_at       = $9,
    accepted_by_id  = $10,
    revoked_by_id   = $11,
    resent_count    = $12,
    message         = $13,
    token_hash      = $14,
    token_preview   = $15,
    updated_at      = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteInvitation :exec
UPDATE user_invitations
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListInvitationsByOrganization :many
SELECT * FROM user_invitations
WHERE organization_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ListInvitationsByEmail :many
SELECT * FROM user_invitations
WHERE email = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: GetPendingInvitationByOrgAndEmail :one
SELECT * FROM user_invitations
WHERE organization_id = $1
  AND email = $2
  AND status = 'pending'
  AND deleted_at IS NULL
LIMIT 1;

-- name: ListPendingInvitationsWithDetailsByOrganization :many
-- List pending invitations for a single organization, hydrating inviter
-- and role display fields via LEFT JOIN. Eliminates the N+1 loop that
-- previously called GetUser / GetRoleByID per row in the handler.
--
-- LEFT JOIN on users is filtered by deleted_at IS NULL so a soft-deleted
-- inviter row does not leak through; combined with the FK ON DELETE SET
-- NULL on invited_by_id, the inviter_* columns are NULL whenever the
-- account is gone — the handler renders that as a nil Inviter ref.
SELECT
    i.id,
    i.organization_id,
    i.role_id,
    i.email,
    i.status,
    i.expires_at,
    i.accepted_at,
    i.revoked_at,
    i.resent_at,
    i.created_at,
    i.updated_at,
    i.deleted_at,
    i.invited_by_id,
    i.accepted_by_id,
    i.revoked_by_id,
    i.token_hash,
    i.token_preview,
    i.message,
    i.resent_count,
    u.id         AS inviter_id,
    u.email      AS inviter_email,
    u.first_name AS inviter_first_name,
    u.last_name  AS inviter_last_name,
    r.id         AS role_name_id,
    r.name       AS role_name
FROM user_invitations i
LEFT JOIN users u ON u.id = i.invited_by_id AND u.deleted_at IS NULL
LEFT JOIN roles r ON r.id = i.role_id
WHERE i.organization_id = @organization_id
  AND i.status = 'pending'
  AND i.deleted_at IS NULL
ORDER BY i.created_at DESC;

-- name: ListInvitationsWithDetailsByEmail :many
-- Same hydrated shape as ListPendingInvitationsWithDetailsByOrganization
-- but scoped by invitee email. Serves the "my invitations" endpoint for
-- authenticated users viewing invitations addressed to them.
SELECT
    i.id,
    i.organization_id,
    i.role_id,
    i.email,
    i.status,
    i.expires_at,
    i.accepted_at,
    i.revoked_at,
    i.resent_at,
    i.created_at,
    i.updated_at,
    i.deleted_at,
    i.invited_by_id,
    i.accepted_by_id,
    i.revoked_by_id,
    i.token_hash,
    i.token_preview,
    i.message,
    i.resent_count,
    u.id         AS inviter_id,
    u.email      AS inviter_email,
    u.first_name AS inviter_first_name,
    u.last_name  AS inviter_last_name,
    r.id         AS role_name_id,
    r.name       AS role_name
FROM user_invitations i
LEFT JOIN users u ON u.id = i.invited_by_id AND u.deleted_at IS NULL
LEFT JOIN roles r ON r.id = i.role_id
WHERE i.email = @email
  AND i.deleted_at IS NULL
ORDER BY i.created_at DESC;

-- name: MarkInvitationAccepted :exec
UPDATE user_invitations
SET status         = 'accepted',
    accepted_at    = NOW(),
    accepted_by_id = $2,
    updated_at     = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: MarkInvitationExpired :exec
UPDATE user_invitations
SET status     = 'expired',
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: RevokeInvitation :exec
UPDATE user_invitations
SET status        = 'revoked',
    revoked_at    = NOW(),
    revoked_by_id = $2,
    updated_at    = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: CleanupExpiredPendingInvitations :exec
DELETE FROM user_invitations
WHERE status = 'pending'
  AND expires_at < NOW();

-- name: IsEmailAlreadyInvited :one
SELECT EXISTS (
    SELECT 1 FROM user_invitations
    WHERE organization_id = $1
      AND email = $2
      AND status = 'pending'
      AND deleted_at IS NULL
);

-- name: UpdateInvitationTokenHash :exec
UPDATE user_invitations
SET token_hash    = $2,
    token_preview = $3,
    updated_at    = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: MarkInvitationResentIfAllowed :execrows
-- Atomically increment resent_count + refresh expires_at if within limits.
-- Zero rows affected means either limit reached or cooldown not elapsed;
-- callers disambiguate by re-reading the row.
UPDATE user_invitations
SET resent_at    = NOW(),
    resent_count = resent_count + 1,
    expires_at   = $2,
    updated_at   = NOW()
WHERE id = $1
  AND deleted_at IS NULL
  AND resent_count < $3::int
  AND (resent_at IS NULL OR resent_at < $4::timestamp with time zone);

-- name: CreateInvitationAuditEvent :exec
INSERT INTO invitation_audit_events (
    id, invitation_id, event_type, actor_id, actor_type,
    metadata, ip_address, user_agent, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9
);

-- name: ListInvitationAuditEventsByInvitation :many
SELECT * FROM invitation_audit_events
WHERE invitation_id = $1
ORDER BY created_at ASC;
