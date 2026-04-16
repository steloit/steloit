-- Migration: add_dashboard_templates
-- Created: 2026-01-03T16:36:01+05:30

-- Dashboard templates for pre-defined dashboard configurations
CREATE TABLE dashboard_templates (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(100) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    layout JSONB NOT NULL DEFAULT '[]',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique index on name for template lookup
CREATE UNIQUE INDEX idx_dashboard_templates_name ON dashboard_templates(name);

-- Index on category for filtering
CREATE INDEX idx_dashboard_templates_category ON dashboard_templates(category);

-- Index on is_active for filtering active templates
CREATE INDEX idx_dashboard_templates_is_active ON dashboard_templates(is_active);
