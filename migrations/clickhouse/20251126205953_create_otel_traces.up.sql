CREATE TABLE IF NOT EXISTS otel_traces (
    -- OTLP Core Identity
    span_id String CODEC(ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    parent_span_id Nullable(String) CODEC(ZSTD(1)),
    trace_state Nullable(String) CODEC(ZSTD(1)),

    -- Multi-Tenancy
    project_id UUID CODEC(ZSTD(1)),

    -- OTLP Span Metadata
    span_name String CODEC(ZSTD(1)),
    span_kind UInt8 CODEC(ZSTD(1)),

    -- OTLP Timing (nanosecond precision)
    start_time DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    end_time Nullable(DateTime64(9)) CODEC(Delta(8), ZSTD(1)),
    duration_nano Nullable(UInt64) CODEC(ZSTD(1)),
    completion_start_time Nullable(DateTime64(9)) CODEC(Delta(8), ZSTD(1)),

    -- OTLP Status
    status_code UInt8 CODEC(ZSTD(1)),
    status_message Nullable(String) CODEC(ZSTD(1)),

    -- OTLP-STANDARD Attributes (Map type matching metrics/logs pattern)
    resource_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    span_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- OTLP-STANDARD Scope Fields (separate, not nested in metadata)
    scope_name Nullable(String) CODEC(ZSTD(1)),
    scope_version Nullable(String) CODEC(ZSTD(1)),
    scope_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- OTLP Schema URLs (OTLP 1.38+ schema versioning)
    resource_schema_url Nullable(String) CODEC(ZSTD(1)),
    scope_schema_url Nullable(String) CODEC(ZSTD(1)),

    -- OTLP Preservation (Lossless Export)
    otlp_span_raw Nullable(String) CODEC(ZSTD(3)),
    otlp_resource_attributes Nullable(String) CODEC(ZSTD(3)),
    otlp_scope_attributes Nullable(String) CODEC(ZSTD(3)),

    -- I/O Data
    input Nullable(String) CODEC(ZSTD(3)),
    output Nullable(String) CODEC(ZSTD(3)),

    -- Usage Details (typed Map - correct as-is)
    usage_details Map(LowCardinality(String), UInt64) CODEC(ZSTD(1)),

    -- Cost Tracking
    cost_details Map(LowCardinality(String), Decimal(18, 12)) CODEC(ZSTD(1)),
    pricing_snapshot Map(LowCardinality(String), Decimal(18, 12)) CODEC(ZSTD(1)),
    total_cost Nullable(Decimal(18, 12)) CODEC(ZSTD(1)),

    -- OTLP Events (arrays with Map attributes)
    events_timestamp Array(DateTime64(9)) CODEC(ZSTD(1)),
    events_name Array(LowCardinality(String)) CODEC(ZSTD(1)),
    events_attributes Array(Map(String, String)) CODEC(ZSTD(1)),

    -- OTLP Links (arrays with Map attributes)
    links_trace_id Array(String) CODEC(ZSTD(1)),
    links_span_id Array(String) CODEC(ZSTD(1)),
    links_trace_state Array(String) CODEC(ZSTD(1)),
    links_attributes Array(Map(String, String)) CODEC(ZSTD(1)),

    -- MATERIALIZED: OTEL Standard Attributes (Map subscript syntax)
    user_id LowCardinality(String) MATERIALIZED span_attributes['user.id'] CODEC(ZSTD(1)),
    session_id LowCardinality(String) MATERIALIZED span_attributes['session.id'] CODEC(ZSTD(1)),
    service_name LowCardinality(String) MATERIALIZED resource_attributes['service.name'] CODEC(ZSTD(1)),
    service_version LowCardinality(String) MATERIALIZED resource_attributes['service.version'] CODEC(ZSTD(1)),
    deployment_environment LowCardinality(String) MATERIALIZED resource_attributes['deployment.environment'] CODEC(ZSTD(1)),

    -- MATERIALIZED: OTEL GenAI 1.38 Attributes (dual support for gen_ai.system fallback)
    -- Note: Map returns empty string (not NULL) for missing keys, so use nullIf to handle fallback
    model_name LowCardinality(String) MATERIALIZED span_attributes['gen_ai.request.model'] CODEC(ZSTD(1)),
    provider_name LowCardinality(String) MATERIALIZED coalesce(
        nullIf(span_attributes['gen_ai.provider.name'], ''),
        span_attributes['gen_ai.system']
    ) CODEC(ZSTD(1)),

    -- MATERIALIZED: Brokle Custom Attributes
    brokle_version LowCardinality(String) MATERIALIZED span_attributes['brokle.version'] CODEC(ZSTD(1)),
    brokle_release LowCardinality(String) MATERIALIZED resource_attributes['brokle.release'] CODEC(ZSTD(1)),
    span_type LowCardinality(String) MATERIALIZED span_attributes['brokle.span.type'] CODEC(ZSTD(1)),
    span_level LowCardinality(String) MATERIALIZED span_attributes['brokle.span.level'] CODEC(ZSTD(1)),

    -- DERIVED: Helper Columns
    is_root_span Bool MATERIALIZED parent_span_id IS NULL CODEC(ZSTD(1)),
    has_error Bool MATERIALIZED status_code = 2 CODEC(ZSTD(1)),

    -- Soft Delete Support
    deleted_at Nullable(DateTime64(3)) CODEC(ZSTD(1)),

    -- INDEXES
    INDEX idx_span_id span_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_trace_id trace_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_parent_span_id parent_span_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_project_id project_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_user_id user_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_session_id session_id TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_service_name service_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_service_version service_version TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_deployment_env deployment_environment TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_model model_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_provider provider_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_span_type span_type TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_brokle_version brokle_version TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_brokle_release brokle_release TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_is_root_span is_root_span TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_has_error has_error TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_scope_name scope_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_total_cost total_cost TYPE minmax GRANULARITY 1,
    INDEX idx_duration duration_nano TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY (toYYYYMM(start_time), project_id)
ORDER BY (project_id, start_time, span_id)
TTL start_time + toIntervalDay(365)
SETTINGS index_granularity = 8192;
