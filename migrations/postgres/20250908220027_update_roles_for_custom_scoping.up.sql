-- Update roles table to support scope-based custom roles
-- This enables organizations (and future projects/environments) to create custom roles
-- while maintaining existing template roles

-- Add scope_id column for custom role scoping
ALTER TABLE roles ADD COLUMN scope_id UUID;

-- Drop existing unique constraint that only considered name + scope_type
ALTER TABLE roles DROP CONSTRAINT IF EXISTS idx_role_name_scope;
ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_name_scope_type_key;

-- Keep existing organization template roles as organization scope
-- Only convert truly system-level roles (if any) to system scope
-- Organization template roles (owner, admin, developer, viewer) stay as organization scope with scope_id = NULL

-- Add new comprehensive unique constraint for scope-based roles
-- This prevents naming conflicts within the same scope
ALTER TABLE roles ADD CONSTRAINT unique_role_scope_name UNIQUE (scope_type, scope_id, name);

-- Add index for efficient querying of custom roles by organization
CREATE INDEX idx_roles_organization_scope ON roles (scope_type, scope_id) WHERE scope_id IS NOT NULL;

-- Add index for system template roles
CREATE INDEX idx_roles_system_scope ON roles (scope_type) WHERE scope_type = 'system';

-- Add check constraint to ensure valid scope_type values
ALTER TABLE roles ADD CONSTRAINT chk_roles_scope_type 
    CHECK (scope_type IN ('system', 'organization', 'project', 'environment'));

-- Add check constraint for scope consistency
-- System roles must have scope_id = NULL
-- Custom roles must have scope_id NOT NULL (except for organization templates)
ALTER TABLE roles ADD CONSTRAINT chk_roles_scope_consistency 
    CHECK (
        (scope_type = 'system' AND scope_id IS NULL) OR
        (scope_type IN ('organization', 'project', 'environment'))
    );