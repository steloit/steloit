-- Create dashboards table for custom dashboard configurations
CREATE TABLE IF NOT EXISTS dashboards (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    config JSONB NOT NULL DEFAULT '{}',
    layout JSONB NOT NULL DEFAULT '[]',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Index for listing dashboards by project (soft delete aware)
CREATE INDEX idx_dashboards_project_id ON dashboards(project_id) WHERE deleted_at IS NULL;

-- Index for created_by user lookups
CREATE INDEX idx_dashboards_created_by ON dashboards(created_by) WHERE deleted_at IS NULL;

COMMENT ON TABLE dashboards IS 'Custom dashboards for projects with widget configurations';
COMMENT ON COLUMN dashboards.config IS 'JSONB containing widgets array and dashboard-level settings';
COMMENT ON COLUMN dashboards.layout IS 'JSONB array of widget positions in grid layout';
