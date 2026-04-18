package validator

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Common validation rules for the Brokle platform

// ValidateUserData validates user registration/update data
func ValidateUserData(data map[string]any) error {
	v := New()

	// Name validation
	if name, ok := data["name"].(string); ok {
		v.Required("name", name).
			MinLength("name", name, 2, "name must be at least 2 characters").
			MaxLength("name", name, 100, "name must not exceed 100 characters")
	}

	// Email validation
	if email, ok := data["email"].(string); ok {
		v.Required("email", email).
			Email("email", email)
	}

	// Password validation (if provided)
	if password, ok := data["password"].(string); ok && password != "" {
		v.MinLength("password", password, 8, "password must be at least 8 characters").
			Custom("password", password, IsStrongPassword, "password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateOrganizationData validates organization data
func ValidateOrganizationData(data map[string]any) error {
	v := New()

	// Name validation
	if name, ok := data["name"].(string); ok {
		v.Required("name", name).
			MinLength("name", name, 2, "organization name must be at least 2 characters").
			MaxLength("name", name, 100, "organization name must not exceed 100 characters")
	}

	// Slug validation
	if slug, ok := data["slug"].(string); ok {
		v.Required("slug", slug).
			MinLength("slug", slug, 3, "slug must be at least 3 characters").
			MaxLength("slug", slug, 50, "slug must not exceed 50 characters").
			Pattern("slug", slug, `^[a-z0-9][a-z0-9-]*[a-z0-9]$`, "slug must contain only lowercase letters, numbers, and hyphens, and cannot start or end with a hyphen")
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateProjectData validates project data
func ValidateProjectData(data map[string]any) error {
	v := New()

	// Name validation
	if name, ok := data["name"].(string); ok {
		v.Required("name", name).
			MinLength("name", name, 2, "project name must be at least 2 characters").
			MaxLength("name", name, 100, "project name must not exceed 100 characters")
	}

	// Description validation (optional)
	if description, ok := data["description"].(string); ok && description != "" {
		v.MaxLength("description", description, 500, "description must not exceed 500 characters")
	}

	// Environment validation
	if environment, ok := data["environment"].(string); ok {
		v.Required("environment", environment).
			OneOf("environment", environment, []string{"development", "staging", "production"}, "environment must be development, staging, or production")
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateAPIKeyData validates API key data
func ValidateAPIKeyData(data map[string]any) error {
	v := New()

	// Name validation
	if name, ok := data["name"].(string); ok {
		v.Required("name", name).
			MinLength("name", name, 3, "API key name must be at least 3 characters").
			MaxLength("name", name, 100, "API key name must not exceed 100 characters")
	}

	// Permissions validation (optional)
	if permissions, ok := data["permissions"].([]any); ok && len(permissions) > 0 {
		for i, perm := range permissions {
			if permStr, ok := perm.(string); ok {
				v.Custom(
					"permissions["+strconv.Itoa(i)+"]",
					permStr,
					func(val any) bool {
						return IsValidPermission(val.(string))
					},
					"invalid permission: "+permStr,
				)
			}
		}
	}

	// Expiration date validation (optional)
	if expiresAt, ok := data["expires_at"].(string); ok && expiresAt != "" {
		v.Date("expires_at", expiresAt, time.RFC3339, "expires_at must be a valid RFC3339 date").
			Custom("expires_at", expiresAt, func(val any) bool {
				t, err := time.Parse(time.RFC3339, val.(string))
				if err != nil {
					return false
				}
				return t.After(time.Now())
			}, "expires_at must be in the future")
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateAIRoutingConfig validates AI routing configuration
func ValidateAIRoutingConfig(data map[string]any) error {
	v := New()

	// Provider validation
	if provider, ok := data["provider"].(string); ok {
		v.Required("provider", provider).
			OneOf("provider", provider, []string{"openai", "anthropic", "cohere", "google", "azure", "aws"}, "unsupported AI provider")
	}

	// Model validation
	if model, ok := data["model"].(string); ok {
		v.Required("model", model).
			MinLength("model", model, 1, "model is required").
			MaxLength("model", model, 100, "model name too long")
	}

	// Temperature validation (optional)
	if temperature, ok := data["temperature"]; ok {
		v.Range("temperature", temperature, 0.0, 2.0, "temperature must be between 0.0 and 2.0")
	}

	// Max tokens validation (optional)
	if maxTokens, ok := data["max_tokens"]; ok {
		v.Min("max_tokens", maxTokens, 1, "max_tokens must be at least 1").
			Max("max_tokens", maxTokens, 128000, "max_tokens cannot exceed 128000")
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateBillingData validates billing configuration
func ValidateBillingData(data map[string]any) error {
	v := New()

	// Plan validation
	if plan, ok := data["plan"].(string); ok {
		v.Required("plan", plan).
			OneOf("plan", plan, []string{"free", "pro", "business", "enterprise"}, "invalid billing plan")
	}

	// Usage limit validation (optional)
	if usageLimit, ok := data["usage_limit"]; ok {
		v.Min("usage_limit", usageLimit, 0, "usage_limit cannot be negative")
	}

	// Billing email validation (optional)
	if billingEmail, ok := data["billing_email"].(string); ok && billingEmail != "" {
		v.Email("billing_email", billingEmail)
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidatePaginationParams validates pagination parameters
func ValidatePaginationParams(data map[string]any) error {
	v := New()

	// Page validation
	if page, ok := data["page"]; ok {
		v.Min("page", page, 1, "page must be at least 1")
	}

	// Page size validation
	if pageSize, ok := data["page_size"]; ok {
		v.Range("page_size", pageSize, 1, 100, "page_size must be between 1 and 100")
	}

	// Sort field validation (optional)
	if sortBy, ok := data["sort_by"].(string); ok && sortBy != "" {
		v.Pattern("sort_by", sortBy, `^[a-zA-Z_][a-zA-Z0-9_]*$`, "sort_by must be a valid field name")
	}

	// Sort order validation (optional)
	if sortOrder, ok := data["sort_order"].(string); ok && sortOrder != "" {
		v.OneOf("sort_order", sortOrder, []string{"asc", "desc"}, "sort_order must be 'asc' or 'desc'")
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateWebSocketMessage validates WebSocket message format
func ValidateWebSocketMessage(data map[string]any) error {
	v := New()

	// Type validation
	if msgType, ok := data["type"].(string); ok {
		v.Required("type", msgType).
			OneOf("type", msgType, []string{"subscribe", "unsubscribe", "message", "ping", "pong"}, "invalid message type")
	}

	// Channel validation (for subscribe/unsubscribe)
	if msgType, ok := data["type"].(string); ok && (msgType == "subscribe" || msgType == "unsubscribe") {
		if channel, ok := data["channel"].(string); ok {
			v.Required("channel", channel).
				Pattern("channel", channel, `^[a-zA-Z0-9_.-]+$`, "channel name contains invalid characters")
		}
	}

	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// Custom validation functions

// IsStrongPassword validates password strength
func IsStrongPassword(password any) bool {
	pwd, ok := password.(string)
	if !ok {
		return false
	}

	if len(pwd) < 8 {
		return false
	}

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(pwd)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(pwd)
	hasNumber := regexp.MustCompile(`\d`).MatchString(pwd)
	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(pwd)

	return hasUpper && hasLower && hasNumber && hasSpecial
}

// IsValidPermission validates permission format
func IsValidPermission(permission string) bool {
	validPermissions := []string{
		"read", "write", "admin",
		"projects:read", "projects:write", "projects:admin",
		"analytics:read", "analytics:write",
		"billing:read", "billing:write", "billing:admin",
		"users:read", "users:write", "users:admin",
		"api_keys:read", "api_keys:write", "api_keys:admin",
		"routing:read", "routing:write", "routing:admin",
	}

	for _, valid := range validPermissions {
		if permission == valid {
			return true
		}
	}

	// Also allow resource-specific permissions like "project:123:read"
	resourcePattern := regexp.MustCompile(`^[a-z_]+:[a-zA-Z0-9_-]+:(read|write|admin)$`)
	return resourcePattern.MatchString(permission)
}

// IsValidSlug validates URL slug format
func IsValidSlug(slug string) bool {
	if len(slug) < 3 || len(slug) > 50 {
		return false
	}

	// Must start and end with alphanumeric, can contain hyphens in between
	pattern := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)
	return pattern.MatchString(slug)
}

// IsValidEnvironment validates environment name
func IsValidEnvironment(env string) bool {
	validEnvs := []string{"development", "staging", "production", "test"}
	for _, valid := range validEnvs {
		if env == valid {
			return true
		}
	}
	return false
}

// IsValidProviderModel validates AI provider and model combination
func IsValidProviderModel(provider, model string) bool {
	validCombinations := map[string][]string{
		"openai": {
			"gpt-4", "gpt-4-turbo", "gpt-4-turbo-preview",
			"gpt-3.5-turbo", "gpt-3.5-turbo-16k",
			"text-embedding-ada-002", "text-embedding-3-small", "text-embedding-3-large",
		},
		"anthropic": {
			"claude-3-opus-20240229", "claude-3-sonnet-20240229", "claude-3-haiku-20240307",
			"claude-2.1", "claude-2", "claude-instant-1.2",
		},
		"cohere": {
			"command", "command-light", "command-nightly",
			"embed-english-v3.0", "embed-multilingual-v3.0",
		},
		"google": {
			"gemini-pro", "gemini-pro-vision", "text-bison", "chat-bison",
		},
	}

	if models, exists := validCombinations[provider]; exists {
		for _, validModel := range models {
			if model == validModel {
				return true
			}
		}
	}

	return false
}

// IsValidTimezone validates timezone string
func IsValidTimezone(tz string) bool {
	_, err := time.LoadLocation(tz)
	return err == nil
}

// IsValidLanguageCode validates ISO 639-1 language code
func IsValidLanguageCode(code string) bool {
	validCodes := []string{
		"en", "es", "fr", "de", "it", "pt", "ru", "ja", "ko", "zh",
		"ar", "hi", "th", "vi", "nl", "sv", "da", "no", "fi", "pl",
	}

	for _, valid := range validCodes {
		if strings.ToLower(code) == valid {
			return true
		}
	}

	return false
}

// IsValidCurrency validates ISO 4217 currency code
func IsValidCurrency(code string) bool {
	validCurrencies := []string{
		"USD", "EUR", "GBP", "JPY", "AUD", "CAD", "CHF", "CNY", "SEK", "NZD",
		"MXN", "SGD", "HKD", "NOK", "KRW", "TRY", "RUB", "INR", "BRL", "ZAR",
	}

	for _, valid := range validCurrencies {
		if strings.ToUpper(code) == valid {
			return true
		}
	}

	return false
}

// IsValidCountryCode validates ISO 3166-1 alpha-2 country code
func IsValidCountryCode(code string) bool {
	// This is a simplified list - in production, you'd use a complete ISO 3166-1 list
	validCodes := []string{
		"US", "GB", "DE", "FR", "CA", "AU", "JP", "KR", "CN", "IN",
		"BR", "MX", "RU", "IT", "ES", "NL", "SE", "NO", "DK", "FI",
	}

	for _, valid := range validCodes {
		if strings.ToUpper(code) == valid {
			return true
		}
	}

	return false
}
