-- ClickHouse Migration: create_otel_genai_events
-- Created: 2025-11-28T11:30:40+05:30

CREATE TABLE IF NOT EXISTS otel_genai_events (
    timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    event_name LowCardinality(String) CODEC(ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),
    operation_name LowCardinality(String) CODEC(ZSTD(1)),
    model_name LowCardinality(String) CODEC(ZSTD(1)),
    provider_name LowCardinality(String) CODEC(ZSTD(1)),
    input_messages String CODEC(ZSTD(3)),
    output_messages String CODEC(ZSTD(3)),
    input_tokens UInt32 CODEC(ZSTD(1)),
    output_tokens UInt32 CODEC(ZSTD(1)),
    temperature Nullable(Float32) CODEC(ZSTD(1)),
    top_p Nullable(Float32) CODEC(ZSTD(1)),
    max_tokens Nullable(UInt32) CODEC(ZSTD(1)),
    finish_reasons Array(String) CODEC(ZSTD(1)),
    response_id String CODEC(ZSTD(1)),
    evaluation_name Nullable(String) CODEC(ZSTD(1)),
    evaluation_score Nullable(Float32) CODEC(ZSTD(1)),
    evaluation_label Nullable(String) CODEC(ZSTD(1)),
    evaluation_explanation Nullable(String) CODEC(ZSTD(3)),
    project_id UUID CODEC(ZSTD(1)),
    user_id LowCardinality(String) CODEC(ZSTD(1)),
    session_id LowCardinality(String) CODEC(ZSTD(1)),
    INDEX idx_event_name event_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_model_name model_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_project_id project_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_trace_id trace_id TYPE bloom_filter(0.001) GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY (toYYYYMM(timestamp), project_id)
ORDER BY (project_id, event_name, model_name, timestamp)
TTL timestamp + toIntervalDay(365)
SETTINGS index_granularity = 8192;
