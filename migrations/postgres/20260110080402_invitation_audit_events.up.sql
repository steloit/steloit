-- Migration: invitation_audit_events
-- Purpose: Create audit logging table for invitation lifecycle events
-- Part of: Invite Users feature enhancement

-- Create invitation audit events table
CREATE TABLE IF NOT EXISTS invitation_audit_events (
    id UUID PRIMARY KEY,
    invitation_id UUID NOT NULL REFERENCES user_invitations(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_type VARCHAR(20) NOT NULL DEFAULT 'user',
    metadata JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Constraint to ensure valid event types
    CONSTRAINT valid_event_type CHECK (event_type IN ('created', 'resent', 'accepted', 'revoked', 'expired', 'declined'))
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_audit_events_invitation ON invitation_audit_events(invitation_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON invitation_audit_events(actor_id) WHERE actor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_events_created ON invitation_audit_events(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_events_type ON invitation_audit_events(event_type);

-- Add comments explaining the table
COMMENT ON TABLE invitation_audit_events IS 'Audit log for all invitation lifecycle events (SOC2/compliance)';
COMMENT ON COLUMN invitation_audit_events.event_type IS 'Type of event: created, resent, accepted, revoked, expired, declined';
COMMENT ON COLUMN invitation_audit_events.actor_type IS 'Who performed the action: user or system';
COMMENT ON COLUMN invitation_audit_events.metadata IS 'Additional context (e.g., old/new role, reason for revocation)';
