-- ===================================
-- PROMPT LABELS TABLE (PROMPT MANAGEMENT DOMAIN)
-- ===================================
-- Mutable version pointers: labels point to specific versions
-- Moving a label enables instant rollback without code changes
-- "latest" is auto-managed and always points to the highest version number

CREATE TABLE prompt_labels (
    id UUID PRIMARY KEY,
    prompt_id UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    version_id UUID NOT NULL REFERENCES prompt_versions(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Unique constraint: each label name can only exist once per prompt
CREATE UNIQUE INDEX idx_prompt_labels_prompt_name ON prompt_labels(prompt_id, name);

-- Index for looking up labels by version (to get all labels for a version)
CREATE INDEX idx_prompt_labels_version_id ON prompt_labels(version_id);

-- Index for label name searches
CREATE INDEX idx_prompt_labels_name ON prompt_labels(name);

-- Comment on table and columns
COMMENT ON TABLE prompt_labels IS 'Mutable pointers from label names to specific prompt versions';
COMMENT ON COLUMN prompt_labels.name IS 'Label name (e.g., "production", "staging", "latest") - lowercase alphanumeric with dots, dashes, underscores';
COMMENT ON COLUMN prompt_labels.version_id IS 'The version this label currently points to - can be changed atomically';
