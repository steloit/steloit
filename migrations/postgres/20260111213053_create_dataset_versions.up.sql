-- Migration: create_dataset_versions
-- Created: 2026-01-11T21:30:53+05:30

-- ============================================================================
-- Dataset Versioning Migration
-- ============================================================================
-- Implements version control for datasets to support:
-- - Auto-increment version on edits (add/remove items)
-- - Pin to specific version for reproducibility
-- - Version comparison and history tracking
-- ============================================================================

-- Dataset versions table
-- Each version represents a snapshot of the dataset at a point in time
CREATE TABLE IF NOT EXISTS dataset_versions (
    id UUID PRIMARY KEY,
    dataset_id UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    version INT NOT NULL,
    item_count INT NOT NULL DEFAULT 0,
    description TEXT,
    metadata JSONB DEFAULT '{}',
    created_by UUID,  -- User who created this version (nullable for auto-created)
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Ensure unique version numbers per dataset
    CONSTRAINT unique_dataset_version UNIQUE (dataset_id, version)
);

-- Index for listing versions of a dataset
CREATE INDEX idx_dataset_versions_dataset_id ON dataset_versions(dataset_id);
CREATE INDEX idx_dataset_versions_created_at ON dataset_versions(created_at);

-- Dataset item versions table (many-to-many join)
-- Links dataset items to specific versions they belong to
CREATE TABLE IF NOT EXISTS dataset_item_versions (
    dataset_version_id UUID NOT NULL REFERENCES dataset_versions(id) ON DELETE CASCADE,
    dataset_item_id UUID NOT NULL REFERENCES dataset_items(id) ON DELETE CASCADE,

    -- Composite primary key
    PRIMARY KEY (dataset_version_id, dataset_item_id)
);

-- Index for looking up items in a version
CREATE INDEX idx_dataset_item_versions_version_id ON dataset_item_versions(dataset_version_id);
-- Index for finding versions that contain an item
CREATE INDEX idx_dataset_item_versions_item_id ON dataset_item_versions(dataset_item_id);

-- Add current_version_id to datasets table
-- This tracks which version the dataset is currently pinned to (nullable = use latest)
ALTER TABLE datasets ADD COLUMN IF NOT EXISTS current_version_id UUID REFERENCES dataset_versions(id);

-- Index for quick lookup of pinned version
CREATE INDEX IF NOT EXISTS idx_datasets_current_version_id ON datasets(current_version_id) WHERE current_version_id IS NOT NULL;

-- ============================================================================
-- Comment explaining the versioning model
-- ============================================================================
-- Version Model:
--
-- 1. AUTOMATIC VERSIONING: When items are added/removed from a dataset,
--    a new version is created automatically capturing the current state.
--
-- 2. PINNING: A dataset can be "pinned" to a specific version via
--    current_version_id. When pinned, the dataset returns items from
--    that specific version instead of the latest.
--
-- 3. REPRODUCIBILITY: Experiments can reference a specific dataset version
--    to ensure reproducible results even if the dataset evolves.
--
-- 4. STORAGE EFFICIENCY: Items are not duplicated per version.
--    The dataset_item_versions join table tracks which items belong
--    to which versions.
-- ============================================================================
