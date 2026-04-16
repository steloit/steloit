-- Migration: enhance_invitations_security
-- Created: 2026-01-10T08:03:51+05:30
-- Purpose: Add token hashing, personal messages, and resend tracking to user_invitations
-- Part of: Invite Users feature enhancement

-- Add token hash column (will replace plaintext token)
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS token_hash VARCHAR(64);
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS token_preview VARCHAR(16);

-- Add personal message support
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS message TEXT;

-- Add resend tracking
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS resent_count INT NOT NULL DEFAULT 0;
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS resent_at TIMESTAMPTZ;

-- Add audit fields for acceptance and revocation
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS accepted_by_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;
ALTER TABLE user_invitations ADD COLUMN IF NOT EXISTS revoked_by_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- Create index for token hash lookup (O(1) lookup)
CREATE UNIQUE INDEX IF NOT EXISTS idx_invitations_token_hash
    ON user_invitations(token_hash)
    WHERE deleted_at IS NULL AND token_hash IS NOT NULL;

-- Create unique index to prevent duplicate pending invitations for same email/org
CREATE UNIQUE INDEX IF NOT EXISTS idx_invitations_email_org_pending
    ON user_invitations(LOWER(email), organization_id)
    WHERE deleted_at IS NULL AND status = 'pending';

-- Drop the unique constraint on token (this also drops the associated index)
-- Note: UNIQUE keyword creates a constraint, not just an index
ALTER TABLE user_invitations DROP CONSTRAINT IF EXISTS user_invitations_token_key;

-- Drop the legacy plaintext token column (no backward compatibility needed - invitations not yet released)
ALTER TABLE user_invitations DROP COLUMN IF EXISTS token;

-- Make token_hash NOT NULL (required for security)
ALTER TABLE user_invitations ALTER COLUMN token_hash SET NOT NULL;
