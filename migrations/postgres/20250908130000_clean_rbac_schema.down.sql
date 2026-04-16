-- ===================================
-- CLEAN RBAC SCHEMA ROLLBACK
-- ===================================

-- Note: This migration's changes are superseded by 20250908140000
-- which completely recreates the RBAC tables.
--
-- During rollback, 20250908140000 drops all RBAC tables first,
-- so this migration only needs to restore the state BEFORE 20250908130000 was applied.
--
-- The previous state (from 20250907172224) had:
-- - roles table with organization_id and is_system_role columns
-- - role_permissions table
-- - organization_members table

-- ===================================
-- ROLLBACK CLEAN RBAC SCHEMA
-- ===================================
-- This restores the exact state after 20250907172224 was applied
-- by reversing the transformations done in the UP migration

-- Drop user_roles table (created in UP migration)
DROP TABLE IF EXISTS user_roles CASCADE;

-- Restore the exact schema from AFTER 20250907172224
-- Step 1: Add organization_id and is_system_role columns FIRST (before dropping scope fields)
ALTER TABLE roles ADD COLUMN organization_id UUID;
ALTER TABLE roles ADD COLUMN is_system_role BOOLEAN DEFAULT FALSE;

-- Step 2: Migrate data from scope_type/scope_id to organization_id/is_system_role
-- System roles: scope_type='system' → is_system_role=true, organization_id=NULL
-- Org roles: scope_type='organization' → is_system_role=false, organization_id=scope_id
UPDATE roles SET
    is_system_role = (scope_type = 'system'),
    organization_id = CASE
        WHEN scope_type = 'system' THEN NULL
        WHEN scope_type = 'organization' THEN scope_id
        ELSE scope_id
    END;

-- Step 3: Now drop scope_type and scope_id columns
ALTER TABLE roles DROP COLUMN IF EXISTS scope_type CASCADE;
ALTER TABLE roles DROP COLUMN IF EXISTS scope_id CASCADE;

-- Step 4: Drop indexes created in UP migration
DROP INDEX IF EXISTS idx_roles_system_name;
DROP INDEX IF EXISTS idx_roles_scoped_name;
DROP INDEX IF EXISTS idx_roles_scope;

-- Step 5: Drop constraint created in UP migration
ALTER TABLE roles DROP CONSTRAINT IF EXISTS chk_scope_consistency;

-- Step 6: Restore original indexes from before 20250908130000
-- Note: idx_roles_deleted_at was created in UP migration and still exists
CREATE INDEX idx_roles_organization_id ON roles(organization_id);
CREATE INDEX idx_roles_name ON roles(name);
CREATE INDEX idx_roles_is_system_role ON roles(is_system_role);

-- Step 7: Restore constraints from 20250907172224
ALTER TABLE roles ADD CONSTRAINT system_role_organization_check
CHECK (
    (is_system_role = true AND organization_id IS NULL) OR
    (is_system_role = false AND organization_id IS NOT NULL)
);

-- Step 8: Restore unique indexes from 20250907172224
CREATE UNIQUE INDEX unique_global_system_role
ON roles (name) WHERE (is_system_role = true AND organization_id IS NULL);

CREATE UNIQUE INDEX unique_org_role
ON roles (organization_id, name) WHERE (organization_id IS NOT NULL);

-- Step 9: Restore performance indexes from 20250907172224
CREATE INDEX idx_roles_system_global
ON roles (is_system_role, organization_id) WHERE is_system_role = true;

-- Step 10: Restore role_permissions indexes
CREATE INDEX idx_role_permissions_lookup ON role_permissions(role_id);

-- Step 11: Recreate organization_members table with EXACT schema from initial_schema
-- (includes id, created_at, updated_at, deleted_at, and correct unique constraint)
CREATE TABLE organization_members (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(organization_id, user_id)
);

-- Step 12: Add foreign key constraint from initial schema
ALTER TABLE organization_members ADD CONSTRAINT fk_organization_members_role_id
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE RESTRICT;