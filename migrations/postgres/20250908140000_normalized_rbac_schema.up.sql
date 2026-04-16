-- Clean Normalized RBAC Schema
-- Drop existing RBAC tables (clean slate)
DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
DROP TABLE IF EXISTS permissions CASCADE;
DROP TABLE IF EXISTS organization_members CASCADE;

-- 1. Template Roles (Global, Reusable)
CREATE TABLE roles (
    id UUID PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    scope_type VARCHAR(20) NOT NULL, -- 'organization', 'project', 'environment'
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(name, scope_type)
);

-- 2. Individual Permissions (Normalized)
CREATE TABLE permissions (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE, -- 'users:create', 'projects:read'
    description TEXT,
    resource VARCHAR(50) NOT NULL,     -- 'users', 'projects', 'billing'
    action VARCHAR(50) NOT NULL,       -- 'create', 'read', 'update', 'delete'
    created_at TIMESTAMP DEFAULT NOW()
);

-- Add indexes for permissions
CREATE INDEX idx_permissions_resource ON permissions(resource);
CREATE INDEX idx_permissions_action ON permissions(action);
CREATE INDEX idx_permissions_resource_action ON permissions(resource, action);

-- 3. Role-Permission Assignments (Many-to-Many)
CREATE TABLE role_permissions (
    role_id UUID NOT NULL,
    permission_id UUID NOT NULL,
    granted_at TIMESTAMP DEFAULT NOW(),
    granted_by UUID, -- audit trail
    PRIMARY KEY (role_id, permission_id),
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

-- 4. Organization Membership (Single Role per Member)
CREATE TABLE organization_members (
    user_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    role_id UUID NOT NULL,
    status VARCHAR(20) DEFAULT 'active', -- 'active', 'invited', 'suspended'
    joined_at TIMESTAMP DEFAULT NOW(),
    invited_by UUID,
    PRIMARY KEY (user_id, organization_id),
    FOREIGN KEY (role_id) REFERENCES roles(id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

-- Add indexes for organization_members
CREATE INDEX idx_org_members_org ON organization_members(organization_id);
CREATE INDEX idx_org_members_user ON organization_members(user_id);
CREATE INDEX idx_org_members_role ON organization_members(role_id);
CREATE INDEX idx_org_members_status ON organization_members(status);

-- 5. Future: Project Membership (Extensible Pattern)
CREATE TABLE project_members (
    user_id UUID NOT NULL,
    project_id UUID NOT NULL,
    role_id UUID NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    joined_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (user_id, project_id),
    FOREIGN KEY (role_id) REFERENCES roles(id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
    -- project_id FK will be added when projects table is created
);

-- Add indexes for project_members
CREATE INDEX idx_project_members_project ON project_members(project_id);
CREATE INDEX idx_project_members_user ON project_members(user_id);
CREATE INDEX idx_project_members_role ON project_members(role_id);

-- 6. Future: Environment Membership (Extensible Pattern)
CREATE TABLE environment_members (
    user_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    role_id UUID NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    joined_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (user_id, environment_id),
    FOREIGN KEY (role_id) REFERENCES roles(id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
    -- environment_id FK will be added when environments table is created
);

-- Add indexes for environment_members
CREATE INDEX idx_environment_members_env ON environment_members(environment_id);
CREATE INDEX idx_environment_members_user ON environment_members(user_id);
CREATE INDEX idx_environment_members_role ON environment_members(role_id);

-- Comments for future reference
COMMENT ON TABLE roles IS 'Template roles available to all organizations, scoped by type';
COMMENT ON TABLE permissions IS 'Individual permissions with resource:action pattern';
COMMENT ON TABLE role_permissions IS 'Many-to-many mapping between roles and permissions';
COMMENT ON TABLE organization_members IS 'Single role per user per organization';
COMMENT ON TABLE project_members IS 'Single role per user per project (future)';
COMMENT ON TABLE environment_members IS 'Single role per user per environment (future)';