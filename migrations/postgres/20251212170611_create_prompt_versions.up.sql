-- ===================================
-- PROMPT VERSIONS TABLE (PROMPT MANAGEMENT DOMAIN)
-- ===================================
-- Immutable version snapshots: versions cannot be edited once created
-- New changes require creating a new version

CREATE TABLE prompt_versions (
    id UUID PRIMARY KEY,
    prompt_id UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    template JSONB NOT NULL,
    config JSONB,
    variables TEXT[] DEFAULT '{}',
    commit_message TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Unique constraint: version numbers must be unique per prompt
CREATE UNIQUE INDEX idx_prompt_versions_prompt_version ON prompt_versions(prompt_id, version);

-- Index for getting latest version quickly
CREATE INDEX idx_prompt_versions_prompt_created ON prompt_versions(prompt_id, created_at DESC);

-- Index for looking up versions by creator
CREATE INDEX idx_prompt_versions_created_by ON prompt_versions(created_by) WHERE created_by IS NOT NULL;

-- Comment on table and columns
COMMENT ON TABLE prompt_versions IS 'Immutable version snapshots of prompts - versions cannot be edited once created';
COMMENT ON COLUMN prompt_versions.template IS 'Template content: {"content": "..."} for text, {"messages": [...]} for chat';
COMMENT ON COLUMN prompt_versions.config IS 'Optional model configuration: {"model": "gpt-4", "temperature": 0.7, ...}';
COMMENT ON COLUMN prompt_versions.variables IS 'Extracted variable names from template (e.g., ["name", "product"])';
COMMENT ON COLUMN prompt_versions.commit_message IS 'Optional description of changes in this version';
