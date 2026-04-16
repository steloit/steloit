-- ============================================================================
-- Blob Storage File Log
-- ============================================================================
-- Purpose: Track S3/blob storage references for archived telemetry data
-- Engine: MergeTree (append-only, no updates needed)
-- Pattern: Aligned with scores table design
-- ============================================================================

CREATE TABLE IF NOT EXISTS blob_storage_file_log (
    -- Identifiers
    id String,
    project_id UUID,

    -- Entity reference
    entity_type LowCardinality(String),
    entity_id String,
    event_id UUID,

    -- Storage location
    bucket_name String,
    bucket_path String,

    -- Metadata
    file_size_bytes Nullable(UInt64),
    content_type Nullable(String) DEFAULT 'text/plain',
    compression Nullable(String),

    -- Timestamp
    created_at DateTime64(3) DEFAULT now64(),

    -- Indexes
    INDEX idx_entity_id entity_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_project_id project_id TYPE bloom_filter(0.001) GRANULARITY 1

) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (project_id, entity_type, entity_id, created_at)
TTL toDateTime(created_at) + INTERVAL 365 DAY
SETTINGS index_granularity = 8192;
