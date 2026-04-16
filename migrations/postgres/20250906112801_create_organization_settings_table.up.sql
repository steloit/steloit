-- Migration: create_organization_settings_table
-- Created: 2025-09-06T11:28:01+05:30

-- Create organization_settings table for key-value settings storage
CREATE TABLE organization_settings (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    value JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(organization_id, key)
);

-- Create indexes for better performance
CREATE INDEX idx_organization_settings_organization_id ON organization_settings(organization_id);
CREATE INDEX idx_organization_settings_key ON organization_settings(key);

-- Add updated_at trigger
CREATE TRIGGER update_organization_settings_updated_at BEFORE UPDATE ON organization_settings 
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
