-- Static queries for contract_history (audit log of contract lifecycle changes).

-- name: CreateContractHistory :exec
INSERT INTO contract_history (
    id, contract_id, action,
    changed_by, changed_by_email,
    changed_at, changes, reason
) VALUES (
    $1, $2, $3,
    $4, $5,
    $6, $7, $8
);

-- name: ListContractHistoryByContract :many
SELECT * FROM contract_history
WHERE contract_id = $1
ORDER BY changed_at DESC;
