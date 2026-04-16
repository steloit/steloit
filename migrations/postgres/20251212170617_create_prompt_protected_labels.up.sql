-- ===================================
-- PROMPT PROTECTED LABELS TABLE (PROMPT MANAGEMENT DOMAIN)
-- ===================================
-- Project-level configuration for label protection
-- Protected labels require admin permissions to modify
-- Provides governance for sensitive deployments (e.g., "production")

CREATE TABLE prompt_protected_labels (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    label_name VARCHAR(50) NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Unique constraint: each label name can only be protected once per project
CREATE UNIQUE INDEX idx_prompt_protected_labels_project_label ON prompt_protected_labels(project_id, label_name);

-- Index for listing protected labels by project
CREATE INDEX idx_prompt_protected_labels_project_id ON prompt_protected_labels(project_id);

-- Comment on table and columns
COMMENT ON TABLE prompt_protected_labels IS 'Project-level configuration for protected labels requiring admin permissions';
COMMENT ON COLUMN prompt_protected_labels.label_name IS 'The label name that is protected (e.g., "production", "staging")';
