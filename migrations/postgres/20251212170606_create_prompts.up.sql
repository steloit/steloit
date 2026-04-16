-- ===================================
-- PROMPTS TABLE (PROMPT MANAGEMENT DOMAIN)
-- ===================================
-- Core prompt entity: stores prompt metadata and links to versions
-- Uses label-based versioning for deployment management

CREATE TABLE prompts (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    type VARCHAR(10) NOT NULL DEFAULT 'text' CHECK (type IN ('text', 'chat')),
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Unique constraint: prompt names must be unique within a project (excluding soft-deleted)
CREATE UNIQUE INDEX idx_prompts_project_name ON prompts(project_id, name) WHERE deleted_at IS NULL;

-- Index for listing prompts by project
CREATE INDEX idx_prompts_project_id ON prompts(project_id) WHERE deleted_at IS NULL;

-- Index for filtering by tags (GIN for array containment queries)
CREATE INDEX idx_prompts_tags ON prompts USING GIN(tags) WHERE deleted_at IS NULL;

-- Index for ordering by creation time
CREATE INDEX idx_prompts_created_at ON prompts(created_at DESC) WHERE deleted_at IS NULL;

-- Comment on table
COMMENT ON TABLE prompts IS 'Stores LLM prompt templates with version management and label-based deployment';
COMMENT ON COLUMN prompts.type IS 'Prompt type: text (simple string template) or chat (array of messages)';
COMMENT ON COLUMN prompts.tags IS 'Array of tags for organizing and filtering prompts';
