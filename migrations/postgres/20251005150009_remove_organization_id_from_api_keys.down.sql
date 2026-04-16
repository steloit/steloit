-- ===================================
-- ROLLBACK: REMOVE ORGANIZATION_ID FROM API_KEYS
-- ===================================
-- Restore organization_id column

-- Add organization_id column back
ALTER TABLE api_keys ADD COLUMN organization_id UUID;

-- Populate organization_id from projects table
UPDATE api_keys
SET organization_id = projects.organization_id
FROM projects
WHERE api_keys.project_id = projects.id;

-- Make it NOT NULL after population
ALTER TABLE api_keys ALTER COLUMN organization_id SET NOT NULL;

-- Restore foreign key constraint
ALTER TABLE api_keys ADD CONSTRAINT api_keys_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;

-- Restore index
CREATE INDEX idx_api_keys_organization_id ON api_keys(organization_id);
