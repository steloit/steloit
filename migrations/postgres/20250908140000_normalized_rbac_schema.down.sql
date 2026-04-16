-- ===================================
-- ROLLBACK NORMALIZED RBAC SCHEMA
-- ===================================
-- Restore state after 20250908130000_clean_rbac_schema was applied
-- (scope_type + scope_id design without the normalized tables)

-- Drop tables created by this migration
DROP TABLE IF EXISTS environment_members CASCADE;
DROP TABLE IF EXISTS project_members CASCADE;
DROP TABLE IF EXISTS organization_members CASCADE;
DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS permissions CASCADE;
DROP TABLE IF EXISTS roles CASCADE;

-- Recreate roles table with exact schema from 20250908130000
-- (which kept display_name and description from initial schema)
CREATE TABLE roles (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    scope_type VARCHAR(50) NOT NULL,
    scope_id UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Recreate constraints and indexes from 20250908130000
ALTER TABLE roles ADD CONSTRAINT chk_scope_consistency CHECK (
    (scope_type = 'system' AND scope_id IS NULL) OR
    (scope_type != 'system' AND scope_id IS NOT NULL)
);

CREATE UNIQUE INDEX idx_roles_system_name
    ON roles (name)
    WHERE scope_type = 'system' AND deleted_at IS NULL;

CREATE UNIQUE INDEX idx_roles_scoped_name
    ON roles (scope_type, scope_id, name)
    WHERE scope_type != 'system' AND deleted_at IS NULL;

CREATE INDEX idx_roles_scope ON roles(scope_type, scope_id);
CREATE INDEX idx_roles_deleted_at ON roles(deleted_at);

-- Recreate user_roles table (from 20250908130000)
CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);

-- Recreate permissions table with exact schema from 20250908130000
-- (which has resource/action from 20250907172224 plus original fields)
CREATE TABLE permissions (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(100) NOT NULL,
    resource VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT unique_resource_action UNIQUE(resource, action)
);

-- Recreate indexes for permissions (from 20250907172224)
CREATE INDEX idx_permissions_resource ON permissions(resource);
CREATE INDEX idx_permissions_resource_action ON permissions(resource, action);

-- Recreate role_permissions table (from 20250908130000)
CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX idx_role_permissions_role_id ON role_permissions(role_id);
CREATE INDEX idx_role_permissions_permission_id ON role_permissions(permission_id);