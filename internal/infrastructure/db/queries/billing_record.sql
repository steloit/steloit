-- Static queries for billing_records and billing_summaries.
-- billing_records = payment transactions; billing_summaries = aggregated
-- period rollups (one per org × period × period_start).

-- name: CreateBillingRecord :exec
INSERT INTO billing_records (
    id, organization_id, period, amount, currency,
    status, transaction_id, payment_method,
    created_at, processed_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10
);

-- name: GetBillingRecordByID :one
SELECT * FROM billing_records
WHERE id = $1
LIMIT 1;

-- name: ListBillingRecordsByOrg :many
SELECT * FROM billing_records
WHERE organization_id = $1
  AND created_at >= $2
  AND created_at <  $3
ORDER BY created_at DESC;

-- name: UpdateBillingRecord :execrows
UPDATE billing_records
SET period         = $2,
    amount         = $3,
    currency       = $4,
    status         = $5,
    transaction_id = $6,
    payment_method = $7,
    processed_at   = $8
WHERE id = $1;

-- name: UpsertBillingSummary :exec
-- One row per (organization_id, period, period_start). Replays with the
-- same key update the aggregate counters idempotently.
INSERT INTO billing_summaries (
    id, organization_id, period, period_start, period_end,
    total_requests, total_tokens, total_cost, currency,
    provider_breakdown, model_breakdown,
    discounts, net_cost, status, generated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11,
    $12, $13, $14, $15
)
ON CONFLICT (organization_id, period, period_start) DO UPDATE SET
    period_end         = EXCLUDED.period_end,
    total_requests     = EXCLUDED.total_requests,
    total_tokens       = EXCLUDED.total_tokens,
    total_cost         = EXCLUDED.total_cost,
    currency           = EXCLUDED.currency,
    provider_breakdown = EXCLUDED.provider_breakdown,
    model_breakdown    = EXCLUDED.model_breakdown,
    discounts          = EXCLUDED.discounts,
    net_cost           = EXCLUDED.net_cost,
    status             = EXCLUDED.status,
    generated_at       = EXCLUDED.generated_at;

-- name: GetLatestBillingSummary :one
SELECT * FROM billing_summaries
WHERE organization_id = $1 AND period = $2
ORDER BY period_start DESC
LIMIT 1;

-- name: ListBillingSummariesByOrg :many
SELECT * FROM billing_summaries
WHERE organization_id = $1
  AND period_start >= $2
  AND period_start <  $3
ORDER BY period_start DESC;
