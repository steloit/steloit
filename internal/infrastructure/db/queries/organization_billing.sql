-- Static queries for organization_billings (per-org billing state).

-- name: CreateOrganizationBilling :exec
INSERT INTO organization_billings (
    organization_id, plan_id,
    billing_cycle_start, billing_cycle_anchor_day,
    current_period_spans, current_period_bytes, current_period_scores,
    current_period_cost,
    free_spans_remaining, free_bytes_remaining, free_scores_remaining,
    last_synced_at, created_at, updated_at
) VALUES (
    $1, $2,
    $3, $4,
    $5, $6, $7,
    $8,
    $9, $10, $11,
    $12, $13, $14
);

-- name: GetOrganizationBillingByOrgID :one
SELECT * FROM organization_billings
WHERE organization_id = $1
LIMIT 1;

-- name: UpdateOrganizationBilling :exec
UPDATE organization_billings
SET plan_id                  = $2,
    billing_cycle_start      = $3,
    billing_cycle_anchor_day = $4,
    current_period_spans     = $5,
    current_period_bytes     = $6,
    current_period_scores    = $7,
    current_period_cost      = $8,
    free_spans_remaining     = $9,
    free_bytes_remaining     = $10,
    free_scores_remaining    = $11,
    last_synced_at           = $12,
    updated_at               = NOW()
WHERE organization_id = $1;

-- name: SetOrganizationBillingUsage :exec
-- Idempotent cumulative-usage write: replaces counters rather than
-- incrementing, so a retried call with the same numbers yields the
-- same row (prevents double-counting on worker retries).
UPDATE organization_billings
SET current_period_spans  = $2,
    current_period_bytes  = $3,
    current_period_scores = $4,
    current_period_cost   = $5,
    free_spans_remaining  = $6,
    free_bytes_remaining  = $7,
    free_scores_remaining = $8,
    last_synced_at        = NOW(),
    updated_at            = NOW()
WHERE organization_id = $1;

-- name: ResetOrganizationBillingPeriod :exec
-- Resets current-period counters to zero and restores free-tier
-- allowances from the org's current plan. Runs at cycle rollover.
UPDATE organization_billings ob
SET billing_cycle_start   = $2,
    current_period_spans  = 0,
    current_period_bytes  = 0,
    current_period_scores = 0,
    current_period_cost   = 0,
    free_spans_remaining  = p.free_spans,
    free_bytes_remaining  = CAST(p.free_gb * 1073741824 AS BIGINT),
    free_scores_remaining = p.free_scores,
    last_synced_at        = NOW(),
    updated_at            = NOW()
FROM plans p
WHERE ob.organization_id = $1
  AND p.id = ob.plan_id;
