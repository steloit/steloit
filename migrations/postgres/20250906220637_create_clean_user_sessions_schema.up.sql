-- ===================================
-- SECURE USER SESSIONS SCHEMA TRANSFORMATION
-- ===================================
-- This migration transforms the existing user_sessions table to use
-- secure JWT token management without storing access tokens.
-- 
-- SECURITY IMPROVEMENTS:
-- - No access tokens stored (stateless JWT)
-- - Only hashed refresh tokens stored
-- - Optimized indexes for performance
-- - Token rotation tracking
-- - Immediate revocation capability

-- Since we have no production users, we can safely drop and recreate
-- the table with the new secure schema
DROP TABLE IF EXISTS user_sessions CASCADE;

-- Create clean user_sessions table with secure token management
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    
    -- Secure Token Management (NO ACCESS TOKENS STORED)
    refresh_token_hash CHAR(64) NOT NULL UNIQUE,              -- SHA-256 hash = 64 hex chars
    refresh_token_version INTEGER NOT NULL DEFAULT 1,         -- For rotation tracking
    current_jti UUID NOT NULL,                           -- Current access token JTI for blacklisting
    
    -- Session Metadata
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,            -- Access token expiry
    refresh_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,    -- Refresh token expiry
    ip_address INET,                                          -- PostgreSQL inet type
    user_agent TEXT,
    device_info JSONB,                                        -- Device information JSON
    
    -- Session State
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_used_at TIMESTAMP WITH TIME ZONE,
    revoked_at TIMESTAMP WITH TIME ZONE,
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Add foreign key constraint
ALTER TABLE user_sessions 
ADD CONSTRAINT fk_user_sessions_user_id 
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Optimized composite index for efficient session lookups
CREATE INDEX idx_user_sessions_user_active ON user_sessions(user_id, is_active);

-- Additional performance indexes
CREATE INDEX idx_user_sessions_current_jti ON user_sessions(current_jti);
CREATE INDEX idx_user_sessions_expires_at ON user_sessions(expires_at);
CREATE INDEX idx_user_sessions_refresh_expires_at ON user_sessions(refresh_expires_at);
CREATE INDEX idx_user_sessions_last_used_at ON user_sessions(last_used_at);
CREATE INDEX idx_user_sessions_revoked_at ON user_sessions(revoked_at);
CREATE INDEX idx_user_sessions_ip_address ON user_sessions(ip_address);

-- Comments for documentation
COMMENT ON TABLE user_sessions IS 'Secure user session management - access tokens NOT stored';
COMMENT ON COLUMN user_sessions.refresh_token_hash IS 'SHA-256 hash of refresh token (64 hex chars)';
COMMENT ON COLUMN user_sessions.refresh_token_version IS 'Version number for token rotation tracking';
COMMENT ON COLUMN user_sessions.current_jti IS 'Current access token JTI for immediate blacklisting';
COMMENT ON COLUMN user_sessions.expires_at IS 'Access token expiry time (typically 15 minutes)';
COMMENT ON COLUMN user_sessions.refresh_expires_at IS 'Refresh token expiry time (typically 7 days)';