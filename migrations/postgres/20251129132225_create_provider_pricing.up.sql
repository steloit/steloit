-- ============================================================================
-- AI Provider Models + Pricing Schema (Schema Only)
-- ============================================================================
-- Purpose: Track AI provider pricing (OpenAI, Anthropic, Google) for cost analytics
-- Design: ProviderModel (metadata) + ProviderPrice (flexible usage types)
-- Seed Data: Managed via seeds/pricing/*.yaml (not in migrations)
-- ============================================================================

-- Provider models table (AI model definitions)
CREATE TABLE IF NOT EXISTS provider_models (
    id UUID PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Project association (NULL = global, non-NULL = project-specific override)
    project_id UUID,

    -- Model identification
    model_name VARCHAR(255) NOT NULL,
    match_pattern VARCHAR(500) NOT NULL,

    -- Temporal versioning (pricing changes over time)
    start_date TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Unit specification
    unit VARCHAR(50) NOT NULL DEFAULT 'TOKENS',

    -- Tokenization (for accurate token counting)
    tokenizer_id VARCHAR(100),
    tokenizer_config JSONB,

    -- Constraints
    CONSTRAINT provider_models_unique_version UNIQUE(project_id, model_name, start_date, unit),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Provider prices table (flexible usage-based pricing)
CREATE TABLE IF NOT EXISTS provider_prices (
    id UUID PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    provider_model_id UUID NOT NULL,
    project_id UUID,

    -- Flexible usage type (arbitrary string - no schema changes needed)
    usage_type VARCHAR(100) NOT NULL,

    -- Price per 1 million units (what providers charge)
    price DECIMAL(20,12) NOT NULL,

    -- Constraints
    CONSTRAINT provider_prices_unique_usage UNIQUE(provider_model_id, usage_type),
    FOREIGN KEY (provider_model_id) REFERENCES provider_models(id) ON DELETE CASCADE,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Indexes for fast lookups
CREATE INDEX idx_provider_models_name ON provider_models(model_name);
CREATE INDEX idx_provider_models_lookup ON provider_models(project_id, model_name, start_date DESC);
CREATE INDEX idx_provider_models_pattern ON provider_models(match_pattern);
CREATE INDEX idx_provider_prices_lookup ON provider_prices(provider_model_id, usage_type);
CREATE INDEX idx_provider_prices_project ON provider_prices(project_id);

-- Add documentation comments
COMMENT ON TABLE provider_models IS 'AI provider model definitions - seed data managed via seeds/pricing/*.yaml';
COMMENT ON TABLE provider_prices IS 'AI provider pricing rates (per 1M tokens) - used for cost analytics dashboards';
COMMENT ON COLUMN provider_prices.usage_type IS 'Flexible usage types: input, output, cache_read_input_tokens, audio_input, batch_input, etc.';
COMMENT ON COLUMN provider_prices.price IS 'Provider rate per 1M units - used for cost visibility (not Brokle billing)';
