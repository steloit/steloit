CREATE TABLE IF NOT EXISTS otel_logs (
    timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    observed_timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),
    trace_flags UInt32 CODEC(ZSTD(1)),
    severity_text LowCardinality(String) CODEC(ZSTD(1)),
    severity_number Int32 CODEC(ZSTD(1)),
    body String CODEC(ZSTD(1)),
    resource_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    service_name LowCardinality(String) MATERIALIZED resource_attributes['service.name'] CODEC(ZSTD(1)),
    scope_name String CODEC(ZSTD(1)),
    scope_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    log_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    project_id UUID CODEC(ZSTD(1)),
    INDEX idx_trace_id trace_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_span_id span_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_service_name service_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_severity severity_text TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_project_id project_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_body body TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY (toYYYYMM(timestamp), project_id)
ORDER BY (project_id, service_name, severity_text, timestamp)
TTL timestamp + toIntervalDay(90)
SETTINGS index_granularity = 8192
