-- ClickHouse Migration: create_billable_usage_tables
-- Created: 2026-01-05T01:10:39+05:30
-- Purpose: Billable usage tracking (Spans + GB + Evaluations)

-- Billable Usage Hourly: Tracks spans, bytes, evaluations per hour
CREATE TABLE IF NOT EXISTS billable_usage_hourly (
    -- Dimensions
    organization_id UUID CODEC(ZSTD(1)),
    project_id UUID CODEC(ZSTD(1)),
    bucket_hour DateTime CODEC(Delta(4), ZSTD(1)),

    -- Billable Dimensions
    span_count UInt64 CODEC(ZSTD(1)),           -- All spans (traces + child spans)
    bytes_processed UInt64 CODEC(ZSTD(1)),      -- Total payload bytes (input + output)
    score_count UInt64 CODEC(ZSTD(1)),          -- Quality scores

    -- AI Provider Costs (informational, not billable by Brokle)
    ai_provider_cost Decimal(18, 12) CODEC(ZSTD(1)),

    -- Metadata
    last_updated DateTime64(3) CODEC(ZSTD(1))
)
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(bucket_hour)
ORDER BY (organization_id, project_id, bucket_hour)
TTL bucket_hour + toIntervalDay(90)
SETTINGS index_granularity = 8192;

-- Billable Usage Daily: Daily rollups for historical queries
CREATE TABLE IF NOT EXISTS billable_usage_daily (
    -- Dimensions
    organization_id UUID CODEC(ZSTD(1)),
    project_id UUID CODEC(ZSTD(1)),
    bucket_date Date CODEC(ZSTD(1)),

    -- Billable Dimensions
    span_count UInt64 CODEC(ZSTD(1)),
    bytes_processed UInt64 CODEC(ZSTD(1)),
    score_count UInt64 CODEC(ZSTD(1)),

    -- AI Provider Costs (informational)
    ai_provider_cost Decimal(18, 12) CODEC(ZSTD(1)),

    -- Metadata
    last_updated DateTime64(3) CODEC(ZSTD(1))
)
ENGINE = SummingMergeTree()
PARTITION BY toYear(bucket_date)
ORDER BY (organization_id, project_id, bucket_date)
TTL bucket_date + toIntervalDay(730)
SETTINGS index_granularity = 8192;

-- Materialized view to auto-populate hourly usage from otel_traces
-- Counts spans and bytes processed in real-time
CREATE MATERIALIZED VIEW IF NOT EXISTS billable_usage_hourly_mv
TO billable_usage_hourly AS
SELECT
    organization_id,
    project_id,
    toStartOfHour(start_time) AS bucket_hour,
    count() AS span_count,
    sum(length(coalesce(input, '')) + length(coalesce(output, ''))) AS bytes_processed,
    toUInt64(0) AS score_count,       -- Populated by separate MV from scores table
    sum(coalesce(total_cost, 0)) AS ai_provider_cost,
    now64(3) AS last_updated
FROM otel_traces
WHERE deleted_at IS NULL
GROUP BY organization_id, project_id, bucket_hour;

-- Materialized view to auto-populate hourly score count from scores table
CREATE MATERIALIZED VIEW IF NOT EXISTS billable_scores_hourly_mv
TO billable_usage_hourly AS
SELECT
    organization_id,
    project_id,
    toStartOfHour(timestamp) AS bucket_hour,
    toUInt64(0) AS span_count,
    toUInt64(0) AS bytes_processed,
    count() AS score_count,
    toDecimal128(0, 12) AS ai_provider_cost,
    now64(3) AS last_updated
FROM scores
GROUP BY organization_id, project_id, bucket_hour;

-- Materialized view to roll up hourly data to daily
CREATE MATERIALIZED VIEW IF NOT EXISTS billable_usage_daily_mv
TO billable_usage_daily AS
SELECT
    organization_id,
    project_id,
    toDate(bucket_hour) AS bucket_date,
    sum(span_count) AS span_count,
    sum(bytes_processed) AS bytes_processed,
    sum(score_count) AS score_count,
    sum(ai_provider_cost) AS ai_provider_cost,
    now64(3) AS last_updated
FROM billable_usage_hourly
GROUP BY organization_id, project_id, bucket_date;
