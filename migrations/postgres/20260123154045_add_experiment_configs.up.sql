-- Migration: add_experiment_configs
-- Created: 2026-01-23
-- Purpose: Add experiment_configs table for wizard-created experiments
--          and extend experiments table with source tracking

-- ============================================================================
-- ExperimentConfig: Stores configuration for dashboard-created experiments
-- ============================================================================
CREATE TABLE experiment_configs (
    id                   UUID PRIMARY KEY,
    experiment_id        UUID NOT NULL UNIQUE REFERENCES experiments(id) ON DELETE CASCADE,

    -- Step 1: Prompt Configuration
    prompt_id            UUID NOT NULL,
    prompt_version_id    UUID NOT NULL,
    model_config         JSONB,  -- Override model settings or null to use prompt's config

    -- Step 2: Dataset Configuration
    dataset_id           UUID NOT NULL,
    dataset_version_id   UUID,
    variable_mapping     JSONB NOT NULL DEFAULT '[]',  -- [{variable_name, source, field_path, is_auto_mapped}]

    -- Step 3: Evaluators Configuration
    evaluators           JSONB NOT NULL DEFAULT '[]',  -- [{name, scorer_type, scorer_config, variable_mapping}]

    created_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for efficient lookups
CREATE INDEX idx_experiment_configs_experiment ON experiment_configs(experiment_id);
CREATE INDEX idx_experiment_configs_prompt ON experiment_configs(prompt_id);
CREATE INDEX idx_experiment_configs_dataset ON experiment_configs(dataset_id);

-- ============================================================================
-- Extend experiments table with source tracking
-- ============================================================================
ALTER TABLE experiments
ADD COLUMN config_id UUID REFERENCES experiment_configs(id) ON DELETE SET NULL,
ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'sdk' CHECK (source IN ('sdk', 'dashboard'));

-- Index for filtering by source
CREATE INDEX idx_experiments_source ON experiments(source);

-- ============================================================================
-- Comments for documentation
-- ============================================================================
COMMENT ON TABLE experiment_configs IS 'Configuration for experiments created via the dashboard wizard';
COMMENT ON COLUMN experiment_configs.variable_mapping IS 'Maps prompt template variables to dataset fields: [{variable_name, source, field_path, is_auto_mapped}]';
COMMENT ON COLUMN experiment_configs.evaluators IS 'Evaluator configurations: [{name, scorer_type, scorer_config, variable_mapping}]';
COMMENT ON COLUMN experiments.source IS 'Origin of the experiment: sdk (created via SDK) or dashboard (created via wizard)';
