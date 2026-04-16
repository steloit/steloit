-- ===================================
-- ROLLBACK SECURE USER SESSIONS SCHEMA
-- ===================================
-- This rollback drops the secure user_sessions table and recreates
-- the previous user_sessions structure (after rename migration).

-- Drop the secure user_sessions table
DROP TABLE IF EXISTS user_sessions CASCADE;

-- Recreate the user_sessions table as it was after the rename migration
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    token VARCHAR(255) NOT NULL,
    refresh_token VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN DEFAULT TRUE,
    ip_address VARCHAR(45),
    user_agent TEXT,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    refresh_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE,
    device_info JSONB,                                       -- Added in rename migration
    revoked_at TIMESTAMP WITH TIME ZONE,                     -- Added in rename migration
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
    -- Note: deleted_at was dropped in rename migration
);

-- Add foreign key constraint
ALTER TABLE user_sessions 
ADD CONSTRAINT fk_user_sessions_user_id 
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Recreate indexes as they were after rename migration
CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_token ON user_sessions(token);
CREATE INDEX idx_user_sessions_refresh_token ON user_sessions(refresh_token);
CREATE INDEX idx_user_sessions_is_active ON user_sessions(is_active);
CREATE INDEX idx_user_sessions_expires_at ON user_sessions(expires_at);
CREATE INDEX idx_user_sessions_revoked_at ON user_sessions(revoked_at);

-- Recreate trigger as it was after rename migration
CREATE TRIGGER update_user_sessions_updated_at BEFORE UPDATE ON user_sessions 
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();