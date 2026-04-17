-- Migration: create_contract_history_table
-- Created: 2026-01-06T15:37:49+05:30

CREATE TABLE contract_history (
    id UUID PRIMARY KEY,
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,

    action VARCHAR(20) NOT NULL
      CHECK (action IN ('created', 'updated', 'cancelled', 'expired', 'pricing_changed')),

    changed_by UUID,
    changed_by_email VARCHAR(255),
    changed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    changes JSONB NOT NULL DEFAULT '{}',
    reason TEXT
);

CREATE INDEX idx_contract_history_contract ON contract_history(contract_id);
CREATE INDEX idx_contract_history_changed_at ON contract_history(changed_at DESC);

COMMENT ON TABLE contract_history IS 'Audit trail for all contract changes';
