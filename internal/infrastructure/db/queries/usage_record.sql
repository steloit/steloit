-- Static queries for usage_records. Each row = one gateway request
-- priced at the time of the call; ClickHouse spans are the source-of-
-- truth for analytics, these rows are the billing-side mirror.

-- name: CreateUsageRecord :exec
INSERT INTO usage_records (
    id, organization_id, request_id,
    provider_id, provider_name,
    model_id, model_name,
    request_type,
    input_tokens, output_tokens, total_tokens,
    cost, currency, billing_tier,
    discounts, net_cost,
    created_at, processed_at
) VALUES (
    $1, $2, $3,
    $4, $5,
    $6, $7,
    $8,
    $9, $10, $11,
    $12, $13, $14,
    $15, $16,
    $17, $18
);

-- name: ListUsageRecordsByOrg :many
SELECT * FROM usage_records
WHERE organization_id = $1
  AND created_at >= $2
  AND created_at <  $3
ORDER BY created_at DESC;

-- name: UpdateUsageRecord :execrows
UPDATE usage_records
SET request_type  = $2,
    input_tokens  = $3,
    output_tokens = $4,
    total_tokens  = $5,
    cost          = $6,
    currency      = $7,
    billing_tier  = $8,
    discounts     = $9,
    net_cost      = $10,
    processed_at  = $11
WHERE id = $1;
