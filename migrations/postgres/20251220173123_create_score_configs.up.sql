-- ===================================
-- SCORE CONFIGS TABLE (EVALUATION DOMAIN)
-- ===================================
-- Reusable score definitions with validation rules for production scoring
-- Part of Phase 1: Production Scoring

CREATE TABLE IF NOT EXISTS score_configs (
    id              UUID PRIMARY KEY,  -- ULID
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    data_type       VARCHAR(20) NOT NULL DEFAULT 'NUMERIC'
                    CHECK (data_type IN ('NUMERIC', 'CATEGORICAL', 'BOOLEAN')),
    min_value       DECIMAL(10,4),         -- For NUMERIC type: minimum allowed value
    max_value       DECIMAL(10,4),         -- For NUMERIC type: maximum allowed value
    categories      JSONB,                 -- For CATEGORICAL type: ["positive", "negative", "neutral"]
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Unique constraint: one name per project
    CONSTRAINT score_configs_project_name_unique UNIQUE(project_id, name)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_score_configs_project_id ON score_configs(project_id);
CREATE INDEX IF NOT EXISTS idx_score_configs_name ON score_configs(name);

-- Documentation
COMMENT ON TABLE score_configs IS 'Reusable score definitions with validation rules. Scores reference these configs for type checking and value validation.';
COMMENT ON COLUMN score_configs.data_type IS 'Score data type: NUMERIC (float with optional min/max), CATEGORICAL (string from predefined list), BOOLEAN (0 or 1)';
COMMENT ON COLUMN score_configs.min_value IS 'Minimum allowed value for NUMERIC scores';
COMMENT ON COLUMN score_configs.max_value IS 'Maximum allowed value for NUMERIC scores';
COMMENT ON COLUMN score_configs.categories IS 'JSON array of allowed string values for CATEGORICAL scores';
