-- Static queries for usage_alerts (alert history triggered by usage_budgets).
-- AlertThreshold is INTEGER (1-100 percent); sqlc widens to int32 while
-- the domain still uses int64 until P2.B.11 unwinds the pq dependency.

-- name: CreateUsageAlert :exec
INSERT INTO usage_alerts (
    id, budget_id, organization_id, project_id,
    alert_threshold, dimension, severity,
    threshold_value, actual_value, percent_used,
    status, triggered_at,
    acknowledged_at, resolved_at, notification_sent
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9, $10,
    $11, $12,
    $13, $14, $15
);

-- name: GetUsageAlertByID :one
SELECT * FROM usage_alerts
WHERE id = $1
LIMIT 1;

-- name: ListUsageAlertsByOrg :many
SELECT * FROM usage_alerts
WHERE organization_id = $1
ORDER BY triggered_at DESC
LIMIT $2;

-- name: ListUsageAlertsByBudget :many
SELECT * FROM usage_alerts
WHERE budget_id = $1
ORDER BY triggered_at DESC;

-- name: ListUnacknowledgedUsageAlertsByOrg :many
SELECT * FROM usage_alerts
WHERE organization_id = $1 AND status = 'triggered'
ORDER BY triggered_at DESC;

-- name: AcknowledgeUsageAlert :exec
UPDATE usage_alerts
SET status          = 'acknowledged',
    acknowledged_at = NOW()
WHERE id = $1;

-- name: ResolveUsageAlert :exec
UPDATE usage_alerts
SET status      = 'resolved',
    resolved_at = NOW()
WHERE id = $1;

-- name: MarkUsageAlertNotified :exec
UPDATE usage_alerts
SET notification_sent = TRUE
WHERE id = $1;
