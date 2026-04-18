package seeder

import (
	"encoding/json"

	"github.com/google/uuid"
)

type PermissionsFile struct {
	Permissions []PermissionSeed `yaml:"permissions"`
}

type RolesFile struct {
	Roles []RoleSeed `yaml:"roles"`
}

type SeedData struct {
	Permissions []PermissionSeed
	Roles       []RoleSeed
}

type PermissionSeed struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type RoleSeed struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	ScopeType   string   `yaml:"scope_type"` // 'organization' | 'project'
	Permissions []string `yaml:"permissions"`
}

type Options struct {
	Reset   bool
	DryRun  bool
	Verbose bool
}

type EntityMaps struct {
	Permissions map[string]uuid.UUID // permission name -> permission ID
	Roles       map[string]uuid.UUID // role_name -> role ID
}

func NewEntityMaps() *EntityMaps {
	return &EntityMaps{
		Permissions: make(map[string]uuid.UUID),
		Roles:       make(map[string]uuid.UUID),
	}
}

type ProviderPricingSeedData struct {
	Version        string              `yaml:"version"`
	ProviderModels []ProviderModelSeed `yaml:"provider_models"`
}

type ProviderModelSeed struct {
	ModelName       string                 `yaml:"model_name"`
	Provider        string                 `yaml:"provider"`               // "openai", "anthropic", "gemini"
	DisplayName     string                 `yaml:"display_name,omitempty"` // User-friendly name for UI
	MatchPattern    string                 `yaml:"match_pattern"`
	StartDate       string                 `yaml:"start_date"` // Format: "2024-05-13"
	Unit            string                 `yaml:"unit"`       // Default: "TOKENS"
	TokenizerID     string                 `yaml:"tokenizer_id,omitempty"`
	TokenizerConfig map[string]any `yaml:"tokenizer_config,omitempty"`
	Prices          []PriceSeed            `yaml:"prices"`
}

type PriceSeed struct {
	UsageType string  `yaml:"usage_type"` // "input", "output", "cache_read_input_tokens", etc.
	Price     float64 `yaml:"price"`      // Price per 1M tokens
}

type RBACStatistics struct {
	TotalRoles        int            `json:"total_roles"`
	TotalPermissions  int            `json:"total_permissions"`
	ScopeDistribution map[string]int `json:"scope_distribution"`
	RoleDistribution  map[string]int `json:"role_distribution"`
}

func (s *RBACStatistics) String() string {
	data, _ := json.MarshalIndent(s, "", "  ")
	return string(data)
}

type PricingStatistics struct {
	TotalModels          int            `json:"total_models"`
	TotalPrices          int            `json:"total_prices"`
	ProviderDistribution map[string]int `json:"provider_distribution"`
}

func (s *PricingStatistics) String() string {
	data, _ := json.MarshalIndent(s, "", "  ")
	return string(data)
}

func InferProvider(modelName string) string {
	switch {
	case len(modelName) >= 3 && modelName[:3] == "gpt":
		return "openai"
	case len(modelName) >= 6 && modelName[:6] == "claude":
		return "anthropic"
	case len(modelName) >= 6 && modelName[:6] == "gemini":
		return "google"
	default:
		return "unknown"
	}
}

// Dashboard Template Seed Types

type TemplatesFile struct {
	Version   string         `yaml:"version"`
	Templates []TemplateSeed `yaml:"templates"`
}

type TemplateSeed struct {
	Name        string               `yaml:"name"`
	Description string               `yaml:"description"`
	Category    string               `yaml:"category"`
	Config      TemplateConfigSeed   `yaml:"config"`
	Layout      []TemplateLayoutSeed `yaml:"layout"`
}

type TemplateConfigSeed struct {
	RefreshRate int                  `yaml:"refresh_rate"`
	Widgets     []TemplateWidgetSeed `yaml:"widgets"`
}

type TemplateWidgetSeed struct {
	ID          string                 `yaml:"id"`
	Type        string                 `yaml:"type"`
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	Query       TemplateQuerySeed      `yaml:"query"`
	Config      map[string]any `yaml:"config,omitempty"`
}

type TemplateQuerySeed struct {
	View       string               `yaml:"view"`
	Measures   []string             `yaml:"measures"`
	Dimensions []string             `yaml:"dimensions"`
	Filters    []TemplateFilterSeed `yaml:"filters"`
	Limit      int                  `yaml:"limit,omitempty"`
	OrderBy    string               `yaml:"order_by,omitempty"`
	OrderDir   string               `yaml:"order_dir,omitempty"`
}

type TemplateFilterSeed struct {
	Field    string      `yaml:"field"`
	Operator string      `yaml:"operator"`
	Value    any `yaml:"value"`
}

type TemplateLayoutSeed struct {
	WidgetID string `yaml:"widget_id"`
	X        int    `yaml:"x"`
	Y        int    `yaml:"y"`
	W        int    `yaml:"w"`
	H        int    `yaml:"h"`
}

type TemplateStatistics struct {
	TotalTemplates       int            `json:"total_templates"`
	CategoryDistribution map[string]int `json:"category_distribution"`
}

func (s *TemplateStatistics) String() string {
	data, _ := json.MarshalIndent(s, "", "  ")
	return string(data)
}

// Billing Config Seed Types

type BillingConfigsFile struct {
	Version string     `yaml:"version"`
	Plans   []PlanSeed `yaml:"plans"`
}

type PlanSeed struct {
	Name              string   `yaml:"name"`
	IsDefault         bool     `yaml:"is_default"`
	FreeSpans         int64    `yaml:"free_spans"`
	FreeGB            float64  `yaml:"free_gb"`
	FreeScores        int64    `yaml:"free_scores"`
	PricePer100KSpans *float64 `yaml:"price_per_100k_spans"`
	PricePerGB        *float64 `yaml:"price_per_gb"`
	PricePer1KScores  *float64 `yaml:"price_per_1k_scores"`
}

type BillingStatistics struct {
	TotalPlans        int    `json:"total_plans"`
	DefaultConfigName string `json:"default_config_name"`
}

func (s *BillingStatistics) String() string {
	data, _ := json.MarshalIndent(s, "", "  ")
	return string(data)
}
