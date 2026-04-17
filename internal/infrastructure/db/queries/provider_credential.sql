-- Static queries for provider_credentials (encrypted LLM provider API keys).

-- name: CreateProviderCredential :exec
INSERT INTO provider_credentials (
    id, organization_id, name, adapter,
    encrypted_key, key_preview,
    base_url, config, headers, custom_models,
    created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13
);

-- name: GetProviderCredentialByID :one
SELECT * FROM provider_credentials
WHERE id = $1 AND organization_id = $2
LIMIT 1;

-- name: GetProviderCredentialByOrgAndName :one
SELECT * FROM provider_credentials
WHERE organization_id = $1 AND name = $2
LIMIT 1;

-- name: ListProviderCredentialsByOrgAndAdapter :many
SELECT * FROM provider_credentials
WHERE organization_id = $1 AND adapter = $2
ORDER BY created_at DESC;

-- name: ListProviderCredentialsByOrg :many
SELECT * FROM provider_credentials
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: UpdateProviderCredential :execrows
UPDATE provider_credentials
SET name          = $3,
    encrypted_key = $4,
    key_preview   = $5,
    base_url      = $6,
    config        = $7,
    custom_models = $8,
    headers       = $9,
    updated_at    = NOW()
WHERE id = $1 AND organization_id = $2;

-- name: DeleteProviderCredential :execrows
DELETE FROM provider_credentials
WHERE id = $1 AND organization_id = $2;

-- name: ProviderCredentialExistsByOrgAndName :one
SELECT EXISTS (
    SELECT 1 FROM provider_credentials
    WHERE organization_id = $1 AND name = $2
);
