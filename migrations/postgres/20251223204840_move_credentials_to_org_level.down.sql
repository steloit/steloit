-- Rollback: move_credentials_to_org_level
-- Created: 2025-12-23T20:48:40+05:30
-- Purpose: Revert AI provider credentials back to project-level scope

-- ============================================
-- Phase 1: Add project_id column back
-- ============================================
ALTER TABLE provider_credentials ADD COLUMN project_id UUID;

-- ============================================
-- Phase 2: Migrate data back
-- ============================================
-- Set project_id from first project in organization (best effort)
-- Note: This may not perfectly restore original project assignment
UPDATE provider_credentials pc
SET project_id = (
    SELECT p.id FROM projects p
    WHERE p.organization_id = pc.organization_id
    ORDER BY p.created_at ASC
    LIMIT 1
);

-- ============================================
-- Phase 3: Make project_id required
-- ============================================
ALTER TABLE provider_credentials ALTER COLUMN project_id SET NOT NULL;

-- ============================================
-- Phase 4: Add project foreign key
-- ============================================
ALTER TABLE provider_credentials
ADD CONSTRAINT fk_provider_credentials_project
FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- ============================================
-- Phase 5: Drop organization constraints and column
-- ============================================
ALTER TABLE provider_credentials DROP CONSTRAINT provider_credentials_org_name_key;
DROP INDEX IF EXISTS idx_provider_credentials_organization_id;
ALTER TABLE provider_credentials DROP CONSTRAINT fk_provider_credentials_organization;
ALTER TABLE provider_credentials DROP COLUMN organization_id;

-- ============================================
-- Phase 6: Restore project-level constraints
-- ============================================
ALTER TABLE provider_credentials
ADD CONSTRAINT provider_credentials_project_id_name_key UNIQUE(project_id, name);

CREATE INDEX idx_provider_credentials_project_id ON provider_credentials(project_id);

