-- Migration: create_usage_billing_tables
-- Created: 2026-01-05T01:13:12+05:30
-- Purpose: Billing state and pricing configuration (Spans + GB + Scores)

-- pricing_configs: Three-dimension pricing tiers
-- NOTE: Seed data is loaded via seeder, not migration (ULIDs generated at runtime)
CREATE TABLE pricing_configs (
    id UUID PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,

    -- Span pricing (per 100K)
    free_spans BIGINT NOT NULL DEFAULT 0,
    price_per_100k_spans NUMERIC(10, 4),

    -- Data volume pricing (per GB)
    free_gb NUMERIC(10, 4) NOT NULL DEFAULT 0,
    price_per_gb NUMERIC(10, 4),

    -- Score pricing (per 1K)
    free_scores BIGINT NOT NULL DEFAULT 0,
    price_per_1k_scores NUMERIC(10, 4),

    is_active BOOLEAN DEFAULT true,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Ensure only one default pricing config at a time
CREATE UNIQUE INDEX idx_pricing_configs_default ON pricing_configs(is_default) WHERE is_default = true;

-- organization_billings: Org-level billing state (three dimensions)
CREATE TABLE organization_billings (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    pricing_config_id UUID NOT NULL REFERENCES pricing_configs(id),
    billing_cycle_start TIMESTAMP WITH TIME ZONE NOT NULL,
    billing_cycle_anchor_day INTEGER DEFAULT 1 CHECK (billing_cycle_anchor_day >= 1 AND billing_cycle_anchor_day <= 28),

    -- Current period usage (three dimensions)
    current_period_spans BIGINT NOT NULL DEFAULT 0,
    current_period_bytes BIGINT NOT NULL DEFAULT 0,
    current_period_scores BIGINT NOT NULL DEFAULT 0,

    -- Calculated cost this period
    current_period_cost NUMERIC(18, 6) DEFAULT 0,

    -- Free tier remaining (three dimensions)
    free_spans_remaining BIGINT DEFAULT 1000000,
    free_bytes_remaining BIGINT DEFAULT 1073741824,
    free_scores_remaining BIGINT DEFAULT 10000,

    last_synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for organization_billings
CREATE INDEX idx_organization_billings_pricing ON organization_billings(pricing_config_id);
CREATE INDEX idx_organization_billings_cycle_start ON organization_billings(billing_cycle_start);

-- usage_budgets: Budget limits and alerts (multi-dimension)
CREATE TABLE usage_budgets (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    budget_type VARCHAR(20) NOT NULL DEFAULT 'monthly' CHECK (budget_type IN ('monthly', 'weekly')),

    -- Limits (any can be set, NULL = no limit)
    span_limit BIGINT,
    bytes_limit BIGINT,
    score_limit BIGINT,
    cost_limit NUMERIC(18, 6),

    -- Current usage
    current_spans BIGINT NOT NULL DEFAULT 0,
    current_bytes BIGINT NOT NULL DEFAULT 0,
    current_scores BIGINT NOT NULL DEFAULT 0,
    current_cost NUMERIC(18, 6) NOT NULL DEFAULT 0,

    -- Alert thresholds (flexible array of percentages, e.g., {50, 80, 100})
    alert_thresholds INTEGER[] NOT NULL DEFAULT '{50,80,100}',

    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for usage_budgets
CREATE INDEX idx_usage_budgets_organization ON usage_budgets(organization_id);
CREATE INDEX idx_usage_budgets_project ON usage_budgets(project_id);
CREATE INDEX idx_usage_budgets_active ON usage_budgets(is_active) WHERE is_active = true;

-- usage_alerts: Alert history
CREATE TABLE usage_alerts (
    id UUID PRIMARY KEY,
    budget_id UUID REFERENCES usage_budgets(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    alert_threshold INTEGER NOT NULL CHECK (alert_threshold >= 1 AND alert_threshold <= 100),
    dimension VARCHAR(20) NOT NULL CHECK (dimension IN ('spans', 'bytes', 'scores', 'cost')),
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    threshold_value BIGINT NOT NULL,
    actual_value BIGINT NOT NULL,
    percent_used NUMERIC(5, 2) NOT NULL,
    status VARCHAR(20) DEFAULT 'triggered' CHECK (status IN ('triggered', 'acknowledged', 'resolved')),
    triggered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    acknowledged_at TIMESTAMP WITH TIME ZONE,
    resolved_at TIMESTAMP WITH TIME ZONE,
    notification_sent BOOLEAN DEFAULT false
);

-- Indexes for usage_alerts
CREATE INDEX idx_usage_alerts_budget ON usage_alerts(budget_id);
CREATE INDEX idx_usage_alerts_organization ON usage_alerts(organization_id);
CREATE INDEX idx_usage_alerts_status ON usage_alerts(status);
CREATE INDEX idx_usage_alerts_triggered ON usage_alerts(triggered_at DESC);
