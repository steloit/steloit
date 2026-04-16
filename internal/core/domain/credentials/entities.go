package credentials

import (
	"time"

	"github.com/google/uuid"

	"github.com/lib/pq"
)

// Provider represents supported AI/LLM providers
type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderAzure      Provider = "azure"
	ProviderGemini     Provider = "gemini"
	ProviderOpenRouter Provider = "openrouter"
	ProviderCustom     Provider = "custom"
)

// ValidProviders returns all valid provider values
func ValidProviders() []Provider {
	return []Provider{
		ProviderOpenAI,
		ProviderAnthropic,
		ProviderAzure,
		ProviderGemini,
		ProviderOpenRouter,
		ProviderCustom,
	}
}

// IsValid checks if the provider is a valid value
func (p Provider) IsValid() bool {
	switch p {
	case ProviderOpenAI, ProviderAnthropic, ProviderAzure, ProviderGemini, ProviderOpenRouter, ProviderCustom:
		return true
	default:
		return false
	}
}

func (p Provider) String() string {
	return string(p)
}

// ProviderCredential represents an encrypted AI provider API key for an organization.
// The actual API key is stored encrypted with AES-256-GCM and is never
// returned to the frontend.
//
// Multiple configurations per adapter (provider type) are supported.
// Each configuration has a unique Name within an organization.
type ProviderCredential struct {
	ID             uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `json:"organization_id" gorm:"type:uuid;not null;index"`

	// User-defined name for this configuration (unique per organization)
	// e.g., "OpenAI Production", "Claude Development", "My Custom Provider"
	Name string `json:"name" gorm:"type:varchar(100);not null"`

	// Adapter type - the API protocol/provider (openai, anthropic, azure, gemini, openrouter, custom)
	Adapter Provider `json:"adapter" gorm:"column:adapter;type:provider;not null"`

	// Encrypted API key (AES-256-GCM: nonce + ciphertext + tag, base64 encoded)
	// Never serialized to JSON, never returned to frontend
	EncryptedKey string `json:"-" gorm:"column:encrypted_key;not null"`

	// Masked preview for safe display (e.g., "sk-***abcd")
	KeyPreview string `json:"key_preview" gorm:"size:20;not null"`

	// Optional custom base URL for Azure OpenAI, proxies, etc.
	BaseURL *string `json:"base_url,omitempty" gorm:"type:text"`

	// Provider-specific configuration (JSONB)
	// Azure: deployment_id, api_version
	// Gemini: location
	// Custom: models list
	Config map[string]any `json:"config,omitempty" gorm:"type:jsonb;serializer:json;default:'{}'"`

	// Encrypted custom HTTP headers (JSON string)
	// Used for proxy authentication or custom endpoints
	// Never serialized to JSON, never returned to frontend
	Headers string `json:"-" gorm:"type:text"`

	// Custom models defined by user (fine-tuned, private deployments, custom provider models)
	// For standard providers: optional fine-tuned models (e.g., "ft:gpt-4o:my-org")
	// For custom provider: required list of available models (e.g., "llama-3.1", "mistral-7b")
	CustomModels pq.StringArray `json:"custom_models" gorm:"type:text[];default:'{}'"`

	// Audit fields
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (ProviderCredential) TableName() string {
	return "provider_credentials"
}

// ProviderCredentialResponse is the safe response DTO (no encrypted data).
// This is what gets returned to the frontend.
type ProviderCredentialResponse struct {
	ID             uuid.UUID         `json:"id"`
	OrganizationID uuid.UUID         `json:"organization_id"`
	Name           string            `json:"name"`
	Adapter        Provider          `json:"adapter"`
	KeyPreview     string            `json:"key_preview"` // e.g., "sk-***abcd"
	BaseURL        *string           `json:"base_url,omitempty"`
	Config         map[string]any    `json:"config,omitempty"`
	CustomModels   []string          `json:"custom_models"`
	Headers        map[string]string `json:"headers,omitempty"` // Decrypted for editing
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// AzureConfig holds Azure OpenAI specific configuration
type AzureConfig struct {
	DeploymentID string `json:"deployment_id"`
	APIVersion   string `json:"api_version,omitempty"`
}

// GeminiConfig holds Google Gemini specific configuration
type GeminiConfig struct {
	Location string `json:"location,omitempty"`
}

// CustomConfig holds custom provider specific configuration
type CustomConfig struct {
	Models []string `json:"models,omitempty"`
}

// DecryptedKeyConfig holds the decrypted key configuration for AI execution.
// This is ONLY used internally during prompt execution and is NEVER persisted
// or returned via API.
type DecryptedKeyConfig struct {
	Provider Provider
	APIKey   string            // Decrypted API key - handle with care
	BaseURL  string            // Custom base URL (if configured)
	Config   map[string]any    // Provider-specific config (Azure: deployment_id, api_version)
	Headers  map[string]string // Custom headers (decrypted) for proxies/custom providers
}

func (c *ProviderCredential) ToResponse() *ProviderCredentialResponse {
	customModels := []string{}
	if c.CustomModels != nil {
		customModels = c.CustomModels
	}
	return &ProviderCredentialResponse{
		ID:             c.ID,
		OrganizationID: c.OrganizationID,
		Name:           c.Name,
		Adapter:        c.Adapter,
		KeyPreview:     c.KeyPreview,
		BaseURL:        c.BaseURL,
		Config:         c.Config,
		CustomModels:   customModels,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

// MaskAPIKey creates a masked preview of an API key.
// Format: first 3 chars + "***" + last 4 chars (e.g., "sk-***abcd")
// For short keys (< 8 chars), returns "***" for security.
func MaskAPIKey(key string) string {
	if len(key) < 8 {
		return "***"
	}
	return key[:3] + "***" + key[len(key)-4:]
}

func (p Provider) RequiresBaseURL() bool {
	switch p {
	case ProviderAzure, ProviderCustom:
		return true
	default:
		return false
	}
}

func (p Provider) DisplayName() string {
	switch p {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Anthropic"
	case ProviderAzure:
		return "Azure OpenAI"
	case ProviderGemini:
		return "Google Gemini"
	case ProviderOpenRouter:
		return "OpenRouter"
	case ProviderCustom:
		return "Custom"
	default:
		return string(p)
	}
}
