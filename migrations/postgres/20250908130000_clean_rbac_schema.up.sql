-- ===================================
-- CLEAN RBAC SCHEMA MIGRATION
-- scope_type + scope_id Design
-- ===================================

-- Drop all legacy constraints and indexes first
ALTER TABLE roles DROP CONSTRAINT IF EXISTS fk_roles_organization_id;
ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_organization_id_name_key;
DROP INDEX IF EXISTS idx_roles_organization_id;
DROP INDEX IF EXISTS idx_roles_name;
DROP INDEX IF EXISTS idx_roles_is_system_role;
DROP INDEX IF EXISTS idx_roles_deleted_at;

-- Drop legacy tables that will be replaced
DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS organization_members CASCADE;

-- Clean roles table redesign
ALTER TABLE roles 
    DROP COLUMN IF EXISTS organization_id,
    DROP COLUMN IF EXISTS is_system_role;

-- Add clean scoped design columns
ALTER TABLE roles 
    ADD COLUMN scope_type VARCHAR(50) NOT NULL DEFAULT 'system',
    ADD COLUMN scope_id UUID;

-- Remove default after adding column
ALTER TABLE roles ALTER COLUMN scope_type DROP DEFAULT;

-- Add clean scope consistency constraint
ALTER TABLE roles ADD CONSTRAINT chk_scope_consistency CHECK (
    (scope_type = 'system' AND scope_id IS NULL) OR
    (scope_type != 'system' AND scope_id IS NOT NULL)
);

-- Create clean unique indexes
CREATE UNIQUE INDEX idx_roles_system_name 
    ON roles (name) 
    WHERE scope_type = 'system' AND deleted_at IS NULL;

CREATE UNIQUE INDEX idx_roles_scoped_name 
    ON roles (scope_type, scope_id, name) 
    WHERE scope_type != 'system' AND deleted_at IS NULL;

-- Create clean performance indexes
CREATE INDEX idx_roles_scope ON roles(scope_type, scope_id);
CREATE INDEX idx_roles_deleted_at ON roles(deleted_at);

-- Create clean user_roles table
CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

-- Add indexes for user_roles
CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);

-- Recreate role_permissions table with clean design
CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (role_id, permission_id)
);

-- Add indexes for role_permissions
CREATE INDEX idx_role_permissions_role_id ON role_permissions(role_id);
CREATE INDEX idx_role_permissions_permission_id ON role_permissions(permission_id);

-- Clean up any orphaned data that might exist
DELETE FROM roles WHERE deleted_at IS NOT NULL;

-- ===================================
-- CLEAN RBAC SCHEMA COMPLETE
-- ===================================