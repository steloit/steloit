CREATE TABLE IF NOT EXISTS otel_resources (
    resource_fingerprint String CODEC(ZSTD(1)),
    project_id UUID CODEC(ZSTD(1)),
    service_name LowCardinality(String) CODEC(ZSTD(1)),
    service_namespace LowCardinality(String) CODEC(ZSTD(1)),
    service_version LowCardinality(String) CODEC(ZSTD(1)),
    service_instance_id String CODEC(ZSTD(1)),
    deployment_environment LowCardinality(String) CODEC(ZSTD(1)),
    container_name LowCardinality(String) CODEC(ZSTD(1)),
    container_id String CODEC(ZSTD(1)),
    container_image_name LowCardinality(String) CODEC(ZSTD(1)),
    k8s_pod_name LowCardinality(String) CODEC(ZSTD(1)),
    k8s_namespace_name LowCardinality(String) CODEC(ZSTD(1)),
    k8s_deployment_name LowCardinality(String) CODEC(ZSTD(1)),
    k8s_cluster_name LowCardinality(String) CODEC(ZSTD(1)),
    host_name LowCardinality(String) CODEC(ZSTD(1)),
    host_arch LowCardinality(String) CODEC(ZSTD(1)),
    resource_attributes Map(String, String) CODEC(ZSTD(1)),
    first_seen DateTime64(3) DEFAULT now64(),
    last_seen DateTime64(3) DEFAULT now64(),
    INDEX idx_resource_fingerprint resource_fingerprint TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_service_name service_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_project_id project_id TYPE bloom_filter(0.001) GRANULARITY 1
)
ENGINE = ReplacingMergeTree(last_seen)
ORDER BY (project_id, service_name, resource_fingerprint)
