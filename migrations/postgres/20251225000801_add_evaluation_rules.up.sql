-- Evaluation Rules for automated span/trace scoring
CREATE TABLE evaluation_rules (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'inactive',
    trigger_type VARCHAR(30) NOT NULL DEFAULT 'on_span_complete',
    target_scope VARCHAR(20) NOT NULL DEFAULT 'span',
    filter JSONB NOT NULL DEFAULT '[]',
    span_names TEXT[] DEFAULT '{}',
    sampling_rate NUMERIC(5,4) NOT NULL DEFAULT 1.0,
    scorer_type VARCHAR(20) NOT NULL,
    scorer_config JSONB NOT NULL,
    variable_mapping JSONB NOT NULL DEFAULT '[]',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT evaluation_rules_status_check CHECK (status IN ('active', 'inactive', 'paused')),
    CONSTRAINT evaluation_rules_trigger_type_check CHECK (trigger_type IN ('on_span_complete')),
    CONSTRAINT evaluation_rules_target_scope_check CHECK (target_scope IN ('span', 'trace')),
    CONSTRAINT evaluation_rules_scorer_type_check CHECK (scorer_type IN ('llm', 'builtin', 'regex')),
    CONSTRAINT evaluation_rules_sampling_rate_check CHECK (sampling_rate >= 0.0 AND sampling_rate <= 1.0)
);

CREATE INDEX idx_evaluation_rules_project_id ON evaluation_rules(project_id);
CREATE INDEX idx_evaluation_rules_status ON evaluation_rules(status);
CREATE INDEX idx_evaluation_rules_project_status ON evaluation_rules(project_id, status);
CREATE UNIQUE INDEX idx_evaluation_rules_project_name ON evaluation_rules(project_id, name);

-- Trigger for updated_at
CREATE TRIGGER evaluation_rules_updated_at
    BEFORE UPDATE ON evaluation_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
