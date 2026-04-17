-- Restore obsolete playground_sessions columns

-- Re-add columns
ALTER TABLE playground_sessions ADD COLUMN template JSONB;
ALTER TABLE playground_sessions ADD COLUMN template_type VARCHAR(10) DEFAULT 'chat';
ALTER TABLE playground_sessions ADD COLUMN is_saved BOOLEAN DEFAULT TRUE;
ALTER TABLE playground_sessions ADD COLUMN expires_at TIMESTAMP WITH TIME ZONE;

-- Populate template from first window if exists
UPDATE playground_sessions
SET template = (windows::jsonb->0->'template')::jsonb
WHERE windows IS NOT NULL AND jsonb_array_length(windows::jsonb) > 0;

-- Set default empty template where still null
UPDATE playground_sessions SET template = '{"messages":[]}'::jsonb WHERE template IS NULL;

-- Make template NOT NULL and template_type NOT NULL
ALTER TABLE playground_sessions ALTER COLUMN template SET NOT NULL;
ALTER TABLE playground_sessions ALTER COLUMN template_type SET NOT NULL;

-- Add constraint for template_type
ALTER TABLE playground_sessions ADD CONSTRAINT chk_template_type CHECK (template_type IN ('chat', 'text'));

-- Re-create old indexes
DROP INDEX IF EXISTS idx_playground_sessions_project_used;
DROP INDEX IF EXISTS idx_playground_sessions_tags;

CREATE INDEX idx_playground_sessions_project_saved
    ON playground_sessions(project_id, is_saved, last_used_at DESC);

CREATE INDEX idx_playground_sessions_expires
    ON playground_sessions(expires_at)
    WHERE expires_at IS NOT NULL;

CREATE INDEX idx_playground_sessions_tags
    ON playground_sessions USING GIN(tags)
    WHERE is_saved = TRUE;
