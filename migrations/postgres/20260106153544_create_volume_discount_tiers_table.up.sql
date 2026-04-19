-- Migration: create_volume_discount_tiers_table
-- Created: 2026-01-06T15:35:44+05:30

CREATE TABLE volume_discount_tiers (
    id UUID PRIMARY KEY,
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,

    -- Which dimension
    dimension VARCHAR(20) NOT NULL CHECK (dimension IN ('spans', 'bytes', 'scores')),

    -- Tier range
    tier_min BIGINT NOT NULL DEFAULT 0,
    tier_max BIGINT,  -- NULL = unlimited

    -- Pricing
    price_per_unit NUMERIC(10, 4) NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT chk_tier_range CHECK (tier_max IS NULL OR tier_max > tier_min),
    CONSTRAINT chk_price_positive CHECK (price_per_unit >= 0)
);

-- Indexes
CREATE INDEX idx_volume_tiers_contract ON volume_discount_tiers(contract_id);
CREATE INDEX idx_volume_tiers_dimension ON volume_discount_tiers(dimension);

-- Prevent overlapping tiers
CREATE UNIQUE INDEX idx_volume_tiers_no_overlap
  ON volume_discount_tiers(contract_id, dimension, tier_min, tier_max);

COMMENT ON TABLE volume_discount_tiers IS 'Progressive pricing tiers (e.g., 0-1M at $0.30, 1M+ at $0.25)';
