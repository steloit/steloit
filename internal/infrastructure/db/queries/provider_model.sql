-- Static queries for provider_models and provider_prices. Complex
-- coalescing queries (project-override semantics) live in squirrel.

-- name: CreateProviderModel :exec
INSERT INTO provider_models (
    id, project_id, model_name, match_pattern,
    provider, display_name,
    start_date, unit, tokenizer_id, tokenizer_config,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9, $10,
    NOW(), NOW()
);

-- name: GetProviderModelByID :one
SELECT * FROM provider_models
WHERE id = $1
LIMIT 1;

-- name: ListProviderModelsGlobal :many
SELECT * FROM provider_models
WHERE project_id IS NULL
ORDER BY model_name ASC, start_date DESC;

-- name: ListProviderModelsByProject :many
SELECT * FROM provider_models
WHERE project_id = $1
ORDER BY model_name ASC, start_date DESC;

-- name: ListProviderModelsByProviders :many
-- Global (project_id IS NULL) models only, filtered to the requested
-- providers — used to populate the model catalog in the UI.
SELECT * FROM provider_models
WHERE provider = ANY($1::text[])
  AND project_id IS NULL
ORDER BY provider ASC, model_name ASC;

-- name: UpdateProviderModel :exec
UPDATE provider_models
SET project_id       = $2,
    model_name       = $3,
    match_pattern    = $4,
    provider         = $5,
    display_name     = $6,
    start_date       = $7,
    unit             = $8,
    tokenizer_id     = $9,
    tokenizer_config = $10,
    updated_at       = NOW()
WHERE id = $1;

-- name: DeleteProviderModel :exec
DELETE FROM provider_models
WHERE id = $1;

-- name: CreateProviderPrice :exec
INSERT INTO provider_prices (
    id, provider_model_id, project_id,
    usage_type, price,
    created_at, updated_at
) VALUES (
    $1, $2, $3,
    $4, $5,
    NOW(), NOW()
);

-- name: ListProviderPricesByModelGlobal :many
SELECT * FROM provider_prices
WHERE provider_model_id = $1
  AND project_id IS NULL;

-- name: ListProviderPricesByModelAndProject :many
-- Returns both project-specific and global prices for a model; the
-- repo layer deduplicates so project-specific overrides global per
-- usage_type.
SELECT * FROM provider_prices
WHERE provider_model_id = $1
  AND (project_id = $2 OR project_id IS NULL);

-- name: UpdateProviderPrice :exec
UPDATE provider_prices
SET provider_model_id = $2,
    project_id        = $3,
    usage_type        = $4,
    price             = $5,
    updated_at        = NOW()
WHERE id = $1;

-- name: DeleteProviderPrice :exec
DELETE FROM provider_prices
WHERE id = $1;
