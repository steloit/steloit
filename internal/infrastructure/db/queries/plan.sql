-- Static queries for the plans lookup table (pricing tiers: free/pro/enterprise).
-- Renamed from pricing_configs in migration 20260106153135.

-- name: CreatePlan :exec
INSERT INTO plans (
    id, name,
    free_spans, price_per_100k_spans,
    free_gb,    price_per_gb,
    free_scores, price_per_1k_scores,
    is_active, is_default,
    created_at, updated_at
) VALUES (
    $1, $2,
    $3, $4,
    $5, $6,
    $7, $8,
    $9, $10,
    $11, $12
);

-- name: GetPlanByID :one
SELECT * FROM plans
WHERE id = $1
LIMIT 1;

-- name: GetPlanByName :one
SELECT * FROM plans
WHERE name = $1
LIMIT 1;

-- name: GetDefaultPlan :one
SELECT * FROM plans
WHERE is_default = TRUE AND is_active = TRUE
LIMIT 1;

-- name: ListActivePlans :many
SELECT * FROM plans
WHERE is_active = TRUE
ORDER BY name ASC;

-- name: UpdatePlan :exec
UPDATE plans
SET name                 = $2,
    free_spans           = $3,
    price_per_100k_spans = $4,
    free_gb              = $5,
    price_per_gb         = $6,
    free_scores          = $7,
    price_per_1k_scores  = $8,
    is_active            = $9,
    is_default           = $10,
    updated_at           = NOW()
WHERE id = $1;
