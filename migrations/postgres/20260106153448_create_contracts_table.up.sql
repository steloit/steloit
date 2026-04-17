-- Migration: create_contracts_table
-- Created: 2026-01-06T15:34:48+05:30

CREATE TABLE contracts (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    -- Contract metadata
    contract_name VARCHAR(255) NOT NULL,
    contract_number VARCHAR(100) UNIQUE NOT NULL,

    -- Dates
    start_date TIMESTAMP WITH TIME ZONE NOT NULL,
    end_date TIMESTAMP WITH TIME ZONE,  -- NULL = no end date

    -- Financial terms
    minimum_commit_amount DECIMAL(18, 6),
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',

    -- Account management
    account_owner VARCHAR(255),
    sales_rep_email VARCHAR(255),

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'active'
      CHECK (status IN ('draft', 'active', 'expired', 'cancelled')),

    -- Custom pricing overrides (NULL = use plan default)
    -- Spans dimension
    custom_free_spans BIGINT,
    custom_price_per_100k_spans DECIMAL(10, 4),

    -- Bytes dimension
    custom_free_gb DECIMAL(10, 4),
    custom_price_per_gb DECIMAL(10, 4),

    -- Scores dimension
    custom_free_scores BIGINT,
    custom_price_per_1k_scores DECIMAL(10, 4),

    -- Audit trail
    created_by UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    notes TEXT,

    CONSTRAINT chk_contract_dates CHECK (end_date IS NULL OR end_date > start_date),
    CONSTRAINT chk_minimum_commit CHECK (minimum_commit_amount IS NULL OR minimum_commit_amount >= 0)
);

-- Indexes
CREATE INDEX idx_contracts_organization ON contracts(organization_id);
CREATE INDEX idx_contracts_status ON contracts(status);
CREATE INDEX idx_contracts_dates ON contracts(start_date, end_date);

-- Only one active contract per organization
CREATE UNIQUE INDEX idx_contracts_active_org
  ON contracts(organization_id) WHERE status = 'active';

-- Auto-update updated_at
CREATE TRIGGER update_contracts_updated_at
    BEFORE UPDATE ON contracts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
