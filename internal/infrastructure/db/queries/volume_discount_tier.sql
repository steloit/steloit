-- Static queries for volume_discount_tiers.
-- tier_max is legitimately nullable: NULL = unlimited upper bound.

-- name: CreateVolumeDiscountTier :exec
INSERT INTO volume_discount_tiers (
    id, contract_id, dimension,
    tier_min, tier_max,
    price_per_unit, priority,
    created_at
) VALUES (
    $1, $2, $3,
    $4, $5,
    $6, $7,
    $8
);

-- name: ListVolumeDiscountTiersByContract :many
SELECT * FROM volume_discount_tiers
WHERE contract_id = $1
ORDER BY dimension ASC, priority ASC, tier_min ASC;

-- name: DeleteVolumeDiscountTiersByContract :execrows
DELETE FROM volume_discount_tiers
WHERE contract_id = $1;
