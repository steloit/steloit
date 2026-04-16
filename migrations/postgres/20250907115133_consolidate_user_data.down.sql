-- ===================================
-- ROLLBACK CONSOLIDATE USER DATA MIGRATION
-- ===================================
-- 
-- This rollback migration:
-- 1. Recreates the user_preferences table
-- 2. Moves avatar_url and phone back to users table
-- 3. Migrates notification preferences back to user_preferences
-- 4. Removes the consolidated columns from user_profiles

-- Recreate user_preferences table with original structure
CREATE TABLE user_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    email_notifications BOOLEAN DEFAULT true,
    push_notifications BOOLEAN DEFAULT true,
    marketing_emails BOOLEAN DEFAULT false,
    weekly_reports BOOLEAN DEFAULT true,
    monthly_reports BOOLEAN DEFAULT true,
    security_alerts BOOLEAN DEFAULT true,
    billing_alerts BOOLEAN DEFAULT true,
    usage_threshold_percent INTEGER DEFAULT 80,
    theme VARCHAR(20) DEFAULT 'light',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Add back columns to users table
ALTER TABLE users ADD COLUMN avatar_url VARCHAR(500);
ALTER TABLE users ADD COLUMN phone VARCHAR(50);

-- Migrate data back from user_profiles to users table
UPDATE users SET 
    avatar_url = p.avatar_url,
    phone = p.phone
FROM user_profiles p 
WHERE users.id = p.user_id 
AND (p.avatar_url IS NOT NULL OR p.phone IS NOT NULL);

-- Migrate notification preferences back to user_preferences table
INSERT INTO user_preferences (
    user_id, email_notifications, push_notifications, marketing_emails,
    weekly_reports, monthly_reports, security_alerts, billing_alerts,
    usage_threshold_percent, theme, created_at, updated_at
)
SELECT 
    user_id,
    COALESCE(email_notifications, true),
    COALESCE(push_notifications, true),
    COALESCE(marketing_emails, false),
    COALESCE(weekly_reports, true),
    COALESCE(monthly_reports, true),
    COALESCE(security_alerts, true),
    COALESCE(billing_alerts, true),
    COALESCE(usage_threshold_percent, 80),
    COALESCE(theme, 'light'),
    COALESCE(created_at, NOW()),
    COALESCE(updated_at, NOW())
FROM user_profiles;

-- Remove consolidated columns from user_profiles
ALTER TABLE user_profiles DROP COLUMN IF EXISTS avatar_url;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS phone;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS email_notifications;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS push_notifications;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS marketing_emails;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS weekly_reports;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS monthly_reports;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS security_alerts;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS billing_alerts;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS usage_threshold_percent;

-- Recreate original indexes for user_preferences
CREATE INDEX idx_user_preferences_user_id ON user_preferences(user_id);
CREATE INDEX idx_user_preferences_theme ON user_preferences(theme);
CREATE INDEX idx_user_preferences_email_notifications ON user_preferences(email_notifications);

-- Remove indexes created in the up migration
DROP INDEX IF EXISTS idx_user_profiles_email_notifications;
DROP INDEX IF EXISTS idx_user_profiles_security_alerts;
DROP INDEX IF EXISTS idx_user_profiles_avatar_url;
DROP INDEX IF EXISTS idx_user_profiles_phone;

-- Add back updated_at trigger for user_preferences
CREATE TRIGGER update_user_preferences_updated_at 
    BEFORE UPDATE ON user_preferences 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();