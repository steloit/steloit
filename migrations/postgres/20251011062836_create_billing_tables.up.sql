-- Create billing tables for usage tracking and billing management
-- Migration: 000004_create_billing_tables.up.sql

-- Usage records table for individual billing entries
-- Note: provider_id and model_id are stored as text (no foreign keys to removed gateway tables)
-- These reference the ClickHouse spans table for cost calculation
CREATE TABLE IF NOT EXISTS usage_records (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id UUID NOT NULL,
    provider_id UUID NOT NULL,
    provider_name VARCHAR(255),
    model_id UUID NOT NULL,
    model_name VARCHAR(255),
    request_type VARCHAR(50) NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    cost DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    billing_tier VARCHAR(50) NOT NULL DEFAULT 'free',
    discounts DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    net_cost DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for usage_records
CREATE INDEX idx_usage_records_org_created ON usage_records(organization_id, created_at DESC);
CREATE INDEX idx_usage_records_org_tier ON usage_records(organization_id, billing_tier);
CREATE INDEX idx_usage_records_request ON usage_records(request_id);
CREATE INDEX idx_usage_records_provider ON usage_records(provider_id);
CREATE INDEX idx_usage_records_model ON usage_records(model_id);
CREATE INDEX idx_usage_records_processed ON usage_records(processed_at) WHERE processed_at IS NOT NULL;

-- Usage quotas table for organization limits
CREATE TABLE IF NOT EXISTS usage_quotas (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    billing_tier VARCHAR(50) NOT NULL DEFAULT 'free',
    monthly_request_limit BIGINT NOT NULL DEFAULT 0,
    monthly_token_limit BIGINT NOT NULL DEFAULT 0,
    monthly_cost_limit DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    current_requests BIGINT NOT NULL DEFAULT 0,
    current_tokens BIGINT NOT NULL DEFAULT 0,
    current_cost DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    reset_date TIMESTAMP WITH TIME ZONE NOT NULL,
    last_updated TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Check constraints for limits
    CONSTRAINT chk_monthly_request_limit CHECK (monthly_request_limit >= 0),
    CONSTRAINT chk_monthly_token_limit CHECK (monthly_token_limit >= 0),
    CONSTRAINT chk_monthly_cost_limit CHECK (monthly_cost_limit >= 0),
    CONSTRAINT chk_current_requests CHECK (current_requests >= 0),
    CONSTRAINT chk_current_tokens CHECK (current_tokens >= 0),
    CONSTRAINT chk_current_cost CHECK (current_cost >= 0)
);

-- Billing records table for payment transactions
CREATE TABLE IF NOT EXISTS billing_records (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    period VARCHAR(50) NOT NULL,
    amount DECIMAL(12, 6) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    transaction_id VARCHAR(255),
    payment_method VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE,

    -- Constraints
    CONSTRAINT chk_billing_amount CHECK (amount >= 0),
    CONSTRAINT chk_billing_status CHECK (status IN ('pending', 'paid', 'failed', 'cancelled', 'refunded'))
);

-- Indexes for billing_records
CREATE INDEX idx_billing_records_org_period ON billing_records(organization_id, period);
CREATE INDEX idx_billing_records_status ON billing_records(status);
CREATE INDEX idx_billing_records_created ON billing_records(created_at DESC);
CREATE INDEX idx_billing_records_transaction ON billing_records(transaction_id) WHERE transaction_id IS NOT NULL;

-- Billing summaries table for period summaries
CREATE TABLE IF NOT EXISTS billing_summaries (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    period VARCHAR(50) NOT NULL,
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    total_requests BIGINT NOT NULL DEFAULT 0,
    total_tokens BIGINT NOT NULL DEFAULT 0,
    total_cost DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    provider_breakdown JSONB NOT NULL DEFAULT '{}',
    model_breakdown JSONB NOT NULL DEFAULT '{}',
    discounts DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    net_cost DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    generated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Unique constraint to prevent duplicate summaries
    CONSTRAINT uq_billing_summaries_org_period_start UNIQUE (organization_id, period, period_start),

    -- Constraints
    CONSTRAINT chk_billing_summary_cost CHECK (total_cost >= 0),
    CONSTRAINT chk_billing_summary_net_cost CHECK (net_cost >= 0),
    CONSTRAINT chk_billing_summary_dates CHECK (period_end > period_start),
    CONSTRAINT chk_billing_summary_status CHECK (status IN ('pending', 'finalized', 'invoiced', 'paid', 'cancelled'))
);

-- Indexes for billing_summaries
CREATE INDEX idx_billing_summaries_org_period ON billing_summaries(organization_id, period);
CREATE INDEX idx_billing_summaries_period_start ON billing_summaries(period_start DESC);
CREATE INDEX idx_billing_summaries_status ON billing_summaries(status);

-- Discount rules table for promotional pricing
CREATE TABLE IF NOT EXISTS discount_rules (
    id UUID PRIMARY KEY,
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL,
    value DECIMAL(12, 6) NOT NULL,
    minimum_amount DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    maximum_discount DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    conditions JSONB NOT NULL DEFAULT '{}',
    valid_from TIMESTAMP WITH TIME ZONE NOT NULL,
    valid_until TIMESTAMP WITH TIME ZONE,
    usage_limit INTEGER,
    usage_count INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT chk_discount_value CHECK (value >= 0),
    CONSTRAINT chk_discount_minimum_amount CHECK (minimum_amount >= 0),
    CONSTRAINT chk_discount_maximum_discount CHECK (maximum_discount >= 0),
    CONSTRAINT chk_discount_usage_limit CHECK (usage_limit IS NULL OR usage_limit > 0),
    CONSTRAINT chk_discount_usage_count CHECK (usage_count >= 0),
    CONSTRAINT chk_discount_type CHECK (type IN ('percentage', 'fixed', 'tiered')),
    CONSTRAINT chk_discount_dates CHECK (valid_until IS NULL OR valid_until > valid_from)
);

-- Indexes for discount_rules
CREATE INDEX idx_discount_rules_org ON discount_rules(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_discount_rules_active_priority ON discount_rules(is_active, priority DESC) WHERE is_active = true;
CREATE INDEX idx_discount_rules_valid_from ON discount_rules(valid_from);
CREATE INDEX idx_discount_rules_valid_until ON discount_rules(valid_until) WHERE valid_until IS NOT NULL;
CREATE INDEX idx_discount_rules_type ON discount_rules(type);

-- Invoices table for invoice management
CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY,
    invoice_number VARCHAR(255) NOT NULL UNIQUE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    organization_name VARCHAR(255) NOT NULL,
    billing_address JSONB,
    period VARCHAR(50) NOT NULL,
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    issue_date TIMESTAMP WITH TIME ZONE NOT NULL,
    due_date TIMESTAMP WITH TIME ZONE NOT NULL,
    subtotal DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    tax_amount DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    discount_amount DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    total_amount DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    payment_terms VARCHAR(255),
    notes TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    paid_at TIMESTAMP WITH TIME ZONE,

    -- Constraints
    CONSTRAINT chk_invoice_subtotal CHECK (subtotal >= 0),
    CONSTRAINT chk_invoice_tax_amount CHECK (tax_amount >= 0),
    CONSTRAINT chk_invoice_discount_amount CHECK (discount_amount >= 0),
    CONSTRAINT chk_invoice_total_amount CHECK (total_amount >= 0),
    CONSTRAINT chk_invoice_dates CHECK (period_end > period_start AND due_date >= issue_date),
    CONSTRAINT chk_invoice_status CHECK (status IN ('draft', 'sent', 'paid', 'overdue', 'cancelled', 'refunded'))
);

-- Indexes for invoices
CREATE INDEX idx_invoices_org_period ON invoices(organization_id, period);
CREATE INDEX idx_invoices_number ON invoices(invoice_number);
CREATE INDEX idx_invoices_status ON invoices(status);
CREATE INDEX idx_invoices_due_date ON invoices(due_date);
CREATE INDEX idx_invoices_issue_date ON invoices(issue_date DESC);
CREATE INDEX idx_invoices_paid_at ON invoices(paid_at) WHERE paid_at IS NOT NULL;

-- Invoice line items table for detailed billing breakdown
-- Note: provider_id and model_id are stored as text (no foreign keys to removed gateway tables)
CREATE TABLE IF NOT EXISTS invoice_line_items (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    quantity DECIMAL(12, 6) NOT NULL DEFAULT 1.0,
    unit_price DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    amount DECIMAL(12, 6) NOT NULL DEFAULT 0.0,
    provider_id UUID,
    provider_name VARCHAR(255),
    model_id UUID,
    model_name VARCHAR(255),
    request_type VARCHAR(50),
    tokens BIGINT,
    requests BIGINT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT chk_line_item_quantity CHECK (quantity > 0),
    CONSTRAINT chk_line_item_unit_price CHECK (unit_price >= 0),
    CONSTRAINT chk_line_item_amount CHECK (amount >= 0),
    CONSTRAINT chk_line_item_tokens CHECK (tokens IS NULL OR tokens >= 0),
    CONSTRAINT chk_line_item_requests CHECK (requests IS NULL OR requests >= 0)
);

-- Indexes for invoice_line_items
CREATE INDEX idx_invoice_line_items_invoice ON invoice_line_items(invoice_id);
CREATE INDEX idx_invoice_line_items_provider ON invoice_line_items(provider_id) WHERE provider_id IS NOT NULL;
CREATE INDEX idx_invoice_line_items_model ON invoice_line_items(model_id) WHERE model_id IS NOT NULL;

-- Payment methods table for organization payment information
CREATE TABLE IF NOT EXISTS payment_methods (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL DEFAULT 'card',
    provider VARCHAR(100) NOT NULL DEFAULT 'stripe',
    external_id VARCHAR(255) NOT NULL,
    last_4 VARCHAR(4),
    expiry_month INTEGER,
    expiry_year INTEGER,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT chk_payment_method_type CHECK (type IN ('card', 'bank_transfer', 'paypal', 'other')),
    CONSTRAINT chk_payment_method_last_4 CHECK (last_4 IS NULL OR length(last_4) = 4),
    CONSTRAINT chk_payment_method_expiry_month CHECK (expiry_month IS NULL OR (expiry_month >= 1 AND expiry_month <= 12)),
    CONSTRAINT chk_payment_method_expiry_year CHECK (expiry_year IS NULL OR expiry_year >= date_part('year', NOW()))
);

-- Indexes for payment_methods
CREATE INDEX idx_payment_methods_org ON payment_methods(organization_id);
CREATE INDEX idx_payment_methods_external ON payment_methods(provider, external_id);

-- Partial unique index to prevent multiple defaults per organization (only enforce when is_default = true)
CREATE UNIQUE INDEX idx_payment_methods_org_default_unique ON payment_methods(organization_id) WHERE is_default = true;

-- Add trigger to automatically update updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_usage_quotas_updated_at
    BEFORE UPDATE ON usage_quotas
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_discount_rules_updated_at
    BEFORE UPDATE ON discount_rules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_invoices_updated_at
    BEFORE UPDATE ON invoices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_payment_methods_updated_at
    BEFORE UPDATE ON payment_methods
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE usage_records IS 'Individual usage records for billing and analytics';
COMMENT ON TABLE usage_quotas IS 'Organization usage quotas and limits';
COMMENT ON TABLE billing_records IS 'Payment transaction records';
COMMENT ON TABLE billing_summaries IS 'Billing period summaries';
COMMENT ON TABLE discount_rules IS 'Promotional discount rules and conditions';
COMMENT ON TABLE invoices IS 'Generated invoices for organizations';
COMMENT ON TABLE invoice_line_items IS 'Detailed line items for invoices';
COMMENT ON TABLE payment_methods IS 'Organization payment method information';

COMMENT ON COLUMN usage_records.request_id IS 'Reference to the original gateway request';
COMMENT ON COLUMN usage_records.billing_tier IS 'Billing tier at time of request (free, pro, business, enterprise)';
COMMENT ON COLUMN usage_quotas.reset_date IS 'When monthly quotas reset (typically first of month)';
COMMENT ON COLUMN discount_rules.conditions IS 'JSON conditions for discount application';
COMMENT ON COLUMN invoices.billing_address IS 'JSON billing address information';
COMMENT ON COLUMN invoices.metadata IS 'Additional invoice metadata (usage stats, etc.)';
