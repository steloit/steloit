-- PostgreSQL Migration: create_filter_presets
-- Created: 2026-01-01
-- Purpose: Add saved filter presets for traces/spans with sharing capabilities

CREATE TABLE IF NOT EXISTS filter_presets (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    table_name VARCHAR(50) NOT NULL DEFAULT 'traces', -- 'traces' or 'spans'

    -- Filter configuration stored as JSONB for flexibility
    filters JSONB NOT NULL DEFAULT '[]',          -- Array of filter conditions
    column_order JSONB DEFAULT '[]',              -- Ordered array of column IDs
    column_visibility JSONB DEFAULT '{}',         -- Map of column ID -> visible boolean
    search_query TEXT,                            -- Full-text search query
    search_types TEXT[] DEFAULT ARRAY['id'],      -- Array of search types (id, content, all)

    -- Sharing
    is_public BOOLEAN NOT NULL DEFAULT FALSE,     -- Visible to all project members

    -- Audit fields
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Unique constraint: one name per project
    CONSTRAINT filter_presets_project_name_unique UNIQUE(project_id, name)
);

-- Indexes for common queries
CREATE INDEX idx_filter_presets_project_id ON filter_presets(project_id);
CREATE INDEX idx_filter_presets_created_by ON filter_presets(created_by);
CREATE INDEX idx_filter_presets_is_public ON filter_presets(project_id, is_public);
CREATE INDEX idx_filter_presets_table_name ON filter_presets(project_id, table_name);

-- Add trigger for updated_at
CREATE OR REPLACE FUNCTION update_filter_presets_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_filter_presets_updated_at
    BEFORE UPDATE ON filter_presets
    FOR EACH ROW
    EXECUTE FUNCTION update_filter_presets_updated_at();

-- Add comment for documentation
COMMENT ON TABLE filter_presets IS 'Saved filter configurations for traces and spans tables with sharing';
