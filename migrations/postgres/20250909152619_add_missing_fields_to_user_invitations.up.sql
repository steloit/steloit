-- Add missing fields to user_invitations table to match domain model
-- This fixes the schema mismatch between database and Invitation struct

-- Add missing deleted_at column for soft delete support
ALTER TABLE user_invitations
ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;

-- Add missing invited_by_id column to track who sent the invitation
ALTER TABLE user_invitations
ADD COLUMN invited_by_id UUID;

-- Add foreign key constraint for invited_by_id
ALTER TABLE user_invitations
ADD CONSTRAINT fk_user_invitations_invited_by_id 
FOREIGN KEY (invited_by_id) REFERENCES users(id) ON DELETE SET NULL;

-- Add performance indexes
CREATE INDEX idx_user_invitations_deleted_at ON user_invitations(deleted_at);
CREATE INDEX idx_user_invitations_invited_by_id ON user_invitations(invited_by_id);