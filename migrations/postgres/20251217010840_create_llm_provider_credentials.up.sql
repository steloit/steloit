-- LLM Provider Credentials for Playground Execution
-- Per-project storage with AES-256-GCM encryption at rest

-- Supported providers enum (extensible for future providers)
CREATE TYPE llm_provider AS ENUM ('openai', 'anthropic');

CREATE TABLE llm_provider_credentials (
    -- Primary key using ULID (26 chars)
    id                UUID PRIMARY KEY,

    -- Project scope (one key per provider per project)
    project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Provider type
    provider          llm_provider NOT NULL,

    -- Encrypted API key (AES-256-GCM: nonce + ciphertext + tag, base64 encoded)
    -- Never stored in plaintext, never returned to frontend
    encrypted_key     TEXT NOT NULL,

    -- Masked preview for display (e.g., "sk-***abcd")
    -- First 3 chars + *** + last 4 chars (or masked entirely if short)
    key_preview       VARCHAR(20) NOT NULL,

    -- Optional custom base URL (for Azure OpenAI, proxies, self-hosted endpoints)
    base_url          TEXT,

    -- Audit fields
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Business constraint: one key per provider per project
    -- Users can update/replace but not have multiple keys for same provider
    CONSTRAINT unique_project_provider UNIQUE(project_id, provider)
);

-- Index for efficient project lookups
CREATE INDEX idx_llm_provider_credentials_project_id ON llm_provider_credentials(project_id);

-- Table and column documentation
COMMENT ON TABLE llm_provider_credentials IS 'User-provided LLM API keys for playground execution. Keys are encrypted at rest with AES-256-GCM.';
COMMENT ON COLUMN llm_provider_credentials.encrypted_key IS 'AES-256-GCM encrypted API key (base64 encoded: nonce || ciphertext || tag). Never returned to clients.';
COMMENT ON COLUMN llm_provider_credentials.key_preview IS 'Masked preview for safe display (e.g., sk-***abcd). Used in UI to show configured keys.';
COMMENT ON COLUMN llm_provider_credentials.base_url IS 'Optional custom endpoint URL for Azure OpenAI, proxy servers, or self-hosted LLM endpoints.';
