-- Migration: move_credentials_to_org_level
-- Created: 2025-12-23T20:48:40+05:30
-- Purpose: Move AI provider credentials from project-level to organization-level scope

-- ============================================
-- Phase 1: Add organization_id column
-- ============================================
ALTER TABLE provider_credentials ADD COLUMN organization_id UUID;

-- ============================================
-- Phase 2: Migrate existing data
-- ============================================
-- Set organization_id from project's organization
UPDATE provider_credentials pc
SET organization_id = p.organization_id
FROM projects p
WHERE pc.project_id = p.id;

-- ============================================
-- Phase 3: Make organization_id required
-- ============================================
ALTER TABLE provider_credentials ALTER COLUMN organization_id SET NOT NULL;

-- ============================================
-- Phase 4: Add foreign key constraint
-- ============================================
ALTER TABLE provider_credentials
ADD CONSTRAINT fk_provider_credentials_organization
FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;

-- ============================================
-- Phase 5: Drop project_id column and constraints
-- ============================================
-- Drop unique constraint on (project_id, name)
ALTER TABLE provider_credentials DROP CONSTRAINT provider_credentials_project_id_name_key;

-- Drop index on project_id
DROP INDEX IF EXISTS idx_provider_credentials_project_id;

-- Drop project_id column (also drops FK constraint automatically)
ALTER TABLE provider_credentials DROP COLUMN project_id;

-- ============================================
-- Phase 6: Add new constraints and indexes
-- ============================================
-- New unique constraint: one name per organization
ALTER TABLE provider_credentials
ADD CONSTRAINT provider_credentials_org_name_key UNIQUE(organization_id, name);

-- New index for organization lookups
CREATE INDEX idx_provider_credentials_organization_id ON provider_credentials(organization_id);

