-- ===================================
-- ROLLBACK: RESTORE ENVIRONMENTS & REVERT API KEYS
-- ===================================
-- This rollback migration restores the Environment entity and reverts
-- API keys to the original schema with environment_id.

-- Step 1: Recreate the environments table
CREATE TABLE environments (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(project_id, slug)
);

-- Step 2: Create default environments for existing projects
INSERT INTO environments (id, project_id, name, slug, created_at, updated_at)
SELECT
    md5(p.id::text || 'default-environment')::uuid,  -- Generate deterministic UUID from project ID
    p.id,
    'Default',
    'default',
    NOW(),
    NOW()
FROM projects p;

-- Step 3: Revert API keys schema
-- First, remove the constraint and index
DROP INDEX IF EXISTS idx_api_keys_default_environment;
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_environment_name;

-- Add environment_id column back
ALTER TABLE api_keys ADD COLUMN environment_id UUID;

-- Map API keys to the default environments based on their project
UPDATE api_keys
SET environment_id = e.id
FROM environments e
WHERE api_keys.project_id = e.project_id
AND e.slug = 'default';

-- Add foreign key constraint
ALTER TABLE api_keys ADD CONSTRAINT api_keys_environment_id_fkey
    FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE CASCADE;

-- Make project_id optional again (as it was before)
ALTER TABLE api_keys ALTER COLUMN project_id DROP NOT NULL;

-- Remove default_environment column
ALTER TABLE api_keys DROP COLUMN IF EXISTS default_environment;

-- Add comment to document the rollback
COMMENT ON TABLE environments IS 'Environment entity restored from migration rollback';