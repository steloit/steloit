-- Scores table for storing evaluation scores (OTLP-linked)
-- Uses Enum8 for data_type and source to ensure data integrity

CREATE TABLE IF NOT EXISTS scores (
    -- Identity
    score_id UUID CODEC(ZSTD(1)),

    -- Links
    project_id UUID CODEC(ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),

    -- Score Data
    name String CODEC(ZSTD(1)),
    value Nullable(Float64) CODEC(ZSTD(1)),
    string_value Nullable(String) CODEC(ZSTD(1)),
    data_type Enum8('NUMERIC' = 1, 'CATEGORICAL' = 2, 'BOOLEAN' = 3) CODEC(ZSTD(1)),
    source Enum8('code' = 1, 'llm' = 2, 'human' = 3) CODEC(ZSTD(1)),

    -- Additional fields
    reason Nullable(String) CODEC(ZSTD(1)),
    metadata String DEFAULT '{}' CODEC(ZSTD(1)),

    -- Experiment tracking
    experiment_id Nullable(UUID) CODEC(ZSTD(1)),
    experiment_item_id Nullable(String) CODEC(ZSTD(1)),

    -- Timestamp
    timestamp DateTime64(3) DEFAULT now64(),

    -- Indexes
    INDEX idx_trace_id trace_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_span_id span_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_project_id project_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_score_name name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_experiment experiment_id TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY (toYYYYMM(timestamp), project_id)
ORDER BY (project_id, timestamp, score_id)
TTL timestamp + INTERVAL 365 DAY
SETTINGS index_granularity = 8192;
