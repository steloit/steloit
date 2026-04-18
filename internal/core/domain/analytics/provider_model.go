package analytics

import (
	"time"

	"github.com/google/uuid"

	"github.com/shopspring/decimal"
)

// ============================================================================
// AI Provider Pricing Entities
// ============================================================================
// Purpose: Track AI provider pricing (OpenAI, Anthropic, Google) for cost analytics
// NOT FOR: User billing - Brokle doesn't charge based on these prices
// FOR: Cost visibility - "You spent $50 with OpenAI this month"
// ============================================================================

// ProviderModel represents an AI provider's LLM model definition (OpenAI, Anthropic, Google)
// Used to track provider pricing for cost analytics, NOT for billing users
type ProviderModel struct {
	ID              uuid.UUID              `json:"id"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	ProjectID       *uuid.UUID             `json:"project_id,omitempty"`
	ModelName       string                 `json:"model_name"`
	MatchPattern    string                 `json:"match_pattern"`
	Provider        string                 `json:"provider"`
	DisplayName     *string                `json:"display_name,omitempty"`
	StartDate       time.Time              `json:"start_date"`
	Unit            string                 `json:"unit"`
	TokenizerID     *string                `json:"tokenizer_id,omitempty"`
	TokenizerConfig map[string]any `json:"tokenizer_config,omitempty"`
}


// ProviderPrice represents AI provider pricing per usage type
// Examples: OpenAI charges $2.50/1M input tokens, Anthropic charges $3.00/1M
// Supports: input, output, cache_read_input_tokens, audio_input, batch_input, etc.
type ProviderPrice struct {
	ID              uuid.UUID       `json:"id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	ProviderModelID uuid.UUID       `json:"provider_model_id"`
	ProjectID       *uuid.UUID      `json:"project_id,omitempty"`
	UsageType       string          `json:"usage_type"`
	Price           decimal.Decimal `json:"price"`
}


// ProviderPricingSnapshot represents provider pricing snapshot captured at ingestion time
// Purpose: Audit trail for "What was OpenAI's pricing on Nov 22, 2025?"
// Used for historical cost analysis and billing dispute resolution
type ProviderPricingSnapshot struct {
	ModelName    string
	Pricing      map[string]decimal.Decimal // usage_type → provider_price_per_million
	SnapshotTime time.Time
}

// AvailableModel represents a model available for selection in the UI
// Combines default models from provider_models table with custom user-defined models
type AvailableModel struct {
	ID             string  `json:"id"`                        // model_name or custom model ID
	Name           string  `json:"name"`                      // display_name for UI
	Provider       string  `json:"provider"`                  // provider type (openai, anthropic, etc.)
	CredentialID   *string `json:"credential_id,omitempty"`   // credential ID when multiple configs exist
	CredentialName *string `json:"credential_name,omitempty"` // credential name for display
	IsCustom       bool    `json:"is_custom,omitempty"`       // true for user-defined custom models
}
