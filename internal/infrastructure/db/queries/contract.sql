-- Static queries for contracts. Volume tiers are fetched separately
-- via volume_discount_tier.sql ListVolumeDiscountTiersByContract.

-- name: CreateContract :exec
INSERT INTO contracts (
    id, organization_id,
    contract_name, contract_number,
    start_date, end_date,
    minimum_commit_amount, currency,
    account_owner, sales_rep_email,
    status,
    custom_free_spans, custom_price_per_100k_spans,
    custom_free_gb, custom_price_per_gb,
    custom_free_scores, custom_price_per_1k_scores,
    created_by, created_at, updated_at, notes
) VALUES (
    $1, $2,
    $3, $4,
    $5, $6,
    $7, $8,
    $9, $10,
    $11,
    $12, $13,
    $14, $15,
    $16, $17,
    $18, $19, $20, $21
);

-- name: GetContractByID :one
SELECT * FROM contracts
WHERE id = $1
LIMIT 1;

-- name: GetActiveContractByOrg :one
SELECT * FROM contracts
WHERE organization_id = $1 AND status = 'active'
LIMIT 1;

-- name: ListContractsByOrg :many
SELECT * FROM contracts
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: UpdateContract :exec
UPDATE contracts
SET contract_name              = $2,
    contract_number             = $3,
    start_date                  = $4,
    end_date                    = $5,
    minimum_commit_amount       = $6,
    currency                    = $7,
    account_owner               = $8,
    sales_rep_email             = $9,
    status                      = $10,
    custom_free_spans           = $11,
    custom_price_per_100k_spans = $12,
    custom_free_gb              = $13,
    custom_price_per_gb         = $14,
    custom_free_scores          = $15,
    custom_price_per_1k_scores  = $16,
    notes                       = $17,
    updated_at                  = NOW()
WHERE id = $1;

-- name: ExpireContract :execrows
UPDATE contracts
SET status     = 'expired',
    updated_at = NOW()
WHERE id = $1;

-- name: CancelContract :execrows
UPDATE contracts
SET status     = 'cancelled',
    updated_at = NOW()
WHERE id = $1;

-- name: ListExpiringContracts :many
-- Active contracts whose end_date is on or before the target time. The
-- service passes now + N*24h — zero or positive N surfaces contracts
-- about to expire, negative N surfaces already-past ones.
SELECT * FROM contracts
WHERE status = 'active'
  AND end_date IS NOT NULL
  AND end_date <= $1
ORDER BY end_date ASC;
