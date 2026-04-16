-- ClickHouse Migration: add_organization_id
-- Created: 2026-01-05T01:10:30+05:30
-- Purpose: Add organization_id column to otel_traces and scores tables
--          Required for billing aggregation by organization

-- Add organization_id to otel_traces table
ALTER TABLE otel_traces ADD COLUMN IF NOT EXISTS organization_id UUID AFTER project_id;

-- Add organization_id to scores table
ALTER TABLE scores ADD COLUMN IF NOT EXISTS organization_id UUID AFTER project_id;
