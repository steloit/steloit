-- Static queries for usage_budgets (per-org/project spend ceilings).
-- SpanLimit/BytesLimit/ScoreLimit/CostLimit are legitimately nullable
-- (NULL = no limit on that dimension).

-- name: CreateUsageBudget :exec
INSERT INTO usage_budgets (
    id, organization_id, project_id, name, budget_type,
    span_limit, bytes_limit, score_limit, cost_limit,
    current_spans, current_bytes, current_scores, current_cost,
    alert_thresholds, is_active,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13,
    $14, $15,
    $16, $17
);

-- name: GetUsageBudgetByID :one
SELECT * FROM usage_budgets
WHERE id = $1
LIMIT 1;

-- name: ListUsageBudgetsByOrg :many
SELECT * FROM usage_budgets
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: ListUsageBudgetsByProject :many
SELECT * FROM usage_budgets
WHERE project_id = $1
ORDER BY created_at DESC;

-- name: ListActiveUsageBudgetsByOrg :many
SELECT * FROM usage_budgets
WHERE organization_id = $1 AND is_active = TRUE
ORDER BY created_at DESC;

-- name: UpdateUsageBudget :exec
UPDATE usage_budgets
SET organization_id  = $2,
    project_id       = $3,
    name             = $4,
    budget_type      = $5,
    span_limit       = $6,
    bytes_limit      = $7,
    score_limit      = $8,
    cost_limit       = $9,
    current_spans    = $10,
    current_bytes    = $11,
    current_scores   = $12,
    current_cost     = $13,
    alert_thresholds = $14,
    is_active        = $15,
    updated_at       = NOW()
WHERE id = $1;

-- name: SetUsageBudgetUsage :exec
-- Idempotent cumulative-usage write (see organization_billings.SetUsage).
UPDATE usage_budgets
SET current_spans  = $2,
    current_bytes  = $3,
    current_scores = $4,
    current_cost   = $5,
    updated_at     = NOW()
WHERE id = $1;

-- name: DeactivateUsageBudget :exec
-- Logical delete: flip is_active instead of removing the row, so alert
-- history + audit continue to reference the budget by ID.
UPDATE usage_budgets
SET is_active  = FALSE,
    updated_at = NOW()
WHERE id = $1;
