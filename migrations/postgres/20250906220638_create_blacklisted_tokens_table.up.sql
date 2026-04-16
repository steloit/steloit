-- ===================================
-- BLACKLISTED TOKENS TABLE MIGRATION
-- ===================================
-- This migration creates the blacklisted_tokens table for 
-- immediate access token revocation capability.

-- Create blacklisted_tokens table for immediate token revocation
CREATE TABLE blacklisted_tokens (
    jti UUID PRIMARY KEY,                                -- JWT ID (ULID format)
    user_id UUID NOT NULL,                              -- Owner user
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,           -- Token expiry for cleanup
    revoked_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(), -- When revoked
    reason VARCHAR(100) NOT NULL,                           -- logout, suspicious_activity, etc.
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Add foreign key constraint with CASCADE delete for automatic cleanup
ALTER TABLE blacklisted_tokens 
ADD CONSTRAINT fk_blacklisted_tokens_user_id 
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Indexes for efficient cleanup and lookups
CREATE INDEX idx_blacklisted_tokens_expires_at ON blacklisted_tokens(expires_at);
CREATE INDEX idx_blacklisted_tokens_user_id ON blacklisted_tokens(user_id);
CREATE INDEX idx_blacklisted_tokens_revoked_at ON blacklisted_tokens(revoked_at);

-- Comments for documentation
COMMENT ON TABLE blacklisted_tokens IS 'Revoked access tokens for immediate revocation capability';
COMMENT ON COLUMN blacklisted_tokens.jti IS 'JWT ID from access token claims (ULID format)';
COMMENT ON COLUMN blacklisted_tokens.expires_at IS 'Original token expiry time for automatic cleanup';
COMMENT ON COLUMN blacklisted_tokens.reason IS 'Reason for revocation: logout, suspicious_activity, etc.';