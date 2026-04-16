-- ClickHouse Migration: create_otel_metrics_exponential_histogram
-- Created: 2025-11-28T11:29:43+05:30

CREATE TABLE IF NOT EXISTS otel_metrics_exponential_histogram (
    resource_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    service_name LowCardinality(String) MATERIALIZED resource_attributes['service.name'] CODEC(ZSTD(1)),
    scope_name String CODEC(ZSTD(1)),
    scope_version String CODEC(ZSTD(1)),
    scope_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    metric_name LowCardinality(String) CODEC(ZSTD(1)),
    metric_description String CODEC(ZSTD(1)),
    metric_unit String CODEC(ZSTD(1)),
    attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    start_time_unix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    time_unix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    count UInt64 CODEC(ZSTD(1)),
    sum Nullable(Float64) CODEC(ZSTD(1)),
    scale Int32 CODEC(ZSTD(1)),
    zero_count UInt64 CODEC(ZSTD(1)),
    positive_offset Int32 CODEC(ZSTD(1)),
    positive_bucket_counts Array(UInt64) CODEC(ZSTD(1)),
    negative_offset Int32 CODEC(ZSTD(1)),
    negative_bucket_counts Array(UInt64) CODEC(ZSTD(1)),
    min Nullable(Float64) CODEC(ZSTD(1)),
    max Nullable(Float64) CODEC(ZSTD(1)),
    aggregation_temporality Enum8('UNSPECIFIED'=0, 'DELTA'=1, 'CUMULATIVE'=2),
    exemplars_trace_id Array(String) CODEC(ZSTD(1)),
    exemplars_span_id Array(String) CODEC(ZSTD(1)),
    project_id UUID CODEC(ZSTD(1)),
    INDEX idx_metric_name metric_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_service_name service_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_project_id project_id TYPE bloom_filter(0.001) GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY (toYYYYMM(time_unix), project_id)
ORDER BY (project_id, service_name, metric_name, time_unix)
TTL time_unix + toIntervalDay(365)
SETTINGS index_granularity = 8192;
