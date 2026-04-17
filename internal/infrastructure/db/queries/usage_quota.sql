-- Static queries for usage_quotas (per-org request/token/cost ceilings).
-- organization_id is the PK; a single row per org.

-- name: GetUsageQuota :one
SELECT * FROM usage_quotas
WHERE organization_id = $1
LIMIT 1;

-- name: UpsertUsageQuota :exec
INSERT INTO usage_quotas (
    organization_id, billing_tier,
    monthly_request_limit, monthly_token_limit, monthly_cost_limit,
    current_requests, current_tokens, current_cost,
    currency, reset_date, last_updated
) VALUES (
    $1, $2,
    $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11
)
ON CONFLICT (organization_id) DO UPDATE SET
    billing_tier          = EXCLUDED.billing_tier,
    monthly_request_limit = EXCLUDED.monthly_request_limit,
    monthly_token_limit   = EXCLUDED.monthly_token_limit,
    monthly_cost_limit    = EXCLUDED.monthly_cost_limit,
    current_requests      = EXCLUDED.current_requests,
    current_tokens        = EXCLUDED.current_tokens,
    current_cost          = EXCLUDED.current_cost,
    currency              = EXCLUDED.currency,
    reset_date            = EXCLUDED.reset_date,
    last_updated          = EXCLUDED.last_updated;
