-- Static queries for provider_models and provider_prices. Complex
-- coalescing queries (project-override semantics) live in squirrel.

-- name: CreateProviderModel :exec
INSERT INTO provider_models (
    id, project_id, model_name, match_pattern,
    provider, display_name,
    start_date, unit, tokenizer_id, tokenizer_config,
    created_at, updated_at
) VALUES (
    @id, @project_id, @model_name, @match_pattern,
    @provider, @display_name,
    @start_date, @unit, @tokenizer_id, @tokenizer_config,
    NOW(), NOW()
);

-- name: GetProviderModelByID :one
SELECT * FROM provider_models
WHERE id = @id
LIMIT 1;

-- name: ListProviderModelsGlobal :many
SELECT * FROM provider_models
WHERE project_id IS NULL
ORDER BY model_name ASC, start_date DESC;

-- name: ListProviderModelsByProject :many
SELECT * FROM provider_models
WHERE project_id = @project_id
ORDER BY model_name ASC, start_date DESC;

-- name: ListProviderModelsByProviders :many
-- Global (project_id IS NULL) models only, filtered to the requested
-- providers — used to populate the model catalog in the UI.
SELECT * FROM provider_models
WHERE provider = ANY(@providers::text[])
  AND project_id IS NULL
ORDER BY provider ASC, model_name ASC;

-- name: UpdateProviderModel :exec
UPDATE provider_models
SET project_id       = @project_id,
    model_name       = @model_name,
    match_pattern    = @match_pattern,
    provider         = @provider,
    display_name     = @display_name,
    start_date       = @start_date,
    unit             = @unit,
    tokenizer_id     = @tokenizer_id,
    tokenizer_config = @tokenizer_config,
    updated_at       = NOW()
WHERE id = @id;

-- name: DeleteProviderModel :exec
DELETE FROM provider_models
WHERE id = @id;

-- name: CreateProviderPrice :exec
INSERT INTO provider_prices (
    id, provider_model_id, project_id,
    usage_type, price,
    created_at, updated_at
) VALUES (
    @id, @provider_model_id, @project_id,
    @usage_type, @price,
    NOW(), NOW()
);

-- name: ListProviderPricesByModelGlobal :many
SELECT * FROM provider_prices
WHERE provider_model_id = @provider_model_id
  AND project_id IS NULL;

-- name: ListProviderPricesByModelAndProject :many
-- Returns both project-specific and global prices for a model; the
-- repo layer deduplicates so project-specific overrides global per
-- usage_type.
SELECT * FROM provider_prices
WHERE provider_model_id = @provider_model_id
  AND (project_id = @project_id OR project_id IS NULL);

-- name: UpdateProviderPrice :exec
UPDATE provider_prices
SET provider_model_id = @provider_model_id,
    project_id        = @project_id,
    usage_type        = @usage_type,
    price             = @price,
    updated_at        = NOW()
WHERE id = @id;

-- name: DeleteProviderPrice :exec
DELETE FROM provider_prices
WHERE id = @id;
