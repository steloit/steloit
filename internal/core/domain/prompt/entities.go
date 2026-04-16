// Package prompt provides the prompt management domain model.
//
// The prompt domain handles LLM prompt templates with versioning,
// label-based deployment, variable interpolation, and playground execution.
package prompt

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/lib/pq"

	"github.com/google/uuid"

	"brokle/pkg/uid"

	"gorm.io/gorm"
)

// PromptType represents the type of prompt template
type PromptType string

const (
	PromptTypeText PromptType = "text"
	PromptTypeChat PromptType = "chat"
)

// Prompt represents a prompt template with version management.
type Prompt struct {
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
	Name        string         `json:"name" gorm:"size:100;not null"`
	Description string         `json:"description,omitempty" gorm:"type:text"`
	Type        PromptType     `json:"type" gorm:"size:10;not null;default:'text'"`
	Tags        pq.StringArray `json:"tags" gorm:"type:text[];default:'{}'"`
	Versions    []Version      `json:"versions,omitempty" gorm:"foreignKey:PromptID"`
	Labels      []Label        `json:"labels,omitempty" gorm:"foreignKey:PromptID"`
	ID          uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	ProjectID   uuid.UUID      `json:"project_id" gorm:"type:uuid;not null"`
}

// Version represents an immutable version snapshot of a prompt.
type Version struct {
	CreatedAt     time.Time      `json:"created_at"`
	Template      JSON           `json:"template" gorm:"type:jsonb;not null"`
	Config        *ModelConfig   `json:"config,omitempty" gorm:"type:jsonb"`
	Variables     pq.StringArray `json:"variables" gorm:"type:text[];default:'{}'"`
	CommitMessage string         `json:"commit_message,omitempty" gorm:"type:text"`
	Labels        []Label        `json:"labels,omitempty" gorm:"foreignKey:VersionID"`
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	PromptID      uuid.UUID      `json:"prompt_id" gorm:"type:uuid;not null"`
	CreatedBy     *uuid.UUID     `json:"created_by,omitempty" gorm:"type:uuid"`
	Version       int            `json:"version" gorm:"not null"`
}

// Label represents a mutable pointer from a label name to a specific version.
type Label struct {
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Name      string     `json:"name" gorm:"size:50;not null"`
	ID        uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	PromptID  uuid.UUID  `json:"prompt_id" gorm:"type:uuid;not null"`
	VersionID uuid.UUID  `json:"version_id" gorm:"type:uuid;not null"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
}

// ProtectedLabel represents a project-level protected label configuration.
type ProtectedLabel struct {
	CreatedAt time.Time  `json:"created_at"`
	LabelName string     `json:"label_name" gorm:"size:50;not null"`
	ID        uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID  `json:"project_id" gorm:"type:uuid;not null"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
}

// ChatMessage represents a single message in a chat prompt template.
type ChatMessage struct {
	Type    string `json:"type"` // "message" or "placeholder"
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"` // For placeholders
}

// ModelConfig represents optional model configuration for a prompt version.
type ModelConfig struct {
	Model            string   `json:"model,omitempty"`
	Provider         string   `json:"provider,omitempty"`      // Adapter type (openai, anthropic, azure, gemini, openrouter, custom)
	CredentialID     *string  `json:"credential_id,omitempty"` // Specific credential config ID (optional, falls back to adapter-based lookup)
	Temperature      *float64 `json:"temperature,omitempty"`
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	Stop             []string `json:"stop,omitempty"`

	// Tools defines function calling tools for the LLM (OpenAI format).
	// Each tool is stored as raw JSON to preserve provider-specific fields.
	Tools []json.RawMessage `json:"tools,omitempty" swaggertype:"array,object"`

	// ToolChoice controls how the model uses tools ("auto", "none", "required", or specific function).
	// Can be a string or an object like {"type": "function", "function": {"name": "..."}}
	ToolChoice json.RawMessage `json:"tool_choice,omitempty" swaggertype:"object"`

	// ResponseFormat specifies output format for structured outputs.
	// Examples: {"type": "text"}, {"type": "json_object"}, {"type": "json_schema", "json_schema": {...}}
	// Note: Anthropic doesn't support response_format natively.
	ResponseFormat json.RawMessage `json:"response_format,omitempty" swaggertype:"object"`

	// APIKey is the resolved API key for execution (not persisted).
	// Set by handler after credential resolution. Excluded from JSON serialization.
	APIKey string `json:"-"`

	// ResolvedBaseURL is an optional custom endpoint (Azure OpenAI, proxy, etc.).
	// Set by handler after credential resolution. Excluded from JSON serialization.
	ResolvedBaseURL *string `json:"-"`

	// ProviderConfig holds provider-specific configuration (not persisted).
	// Azure: deployment_id, api_version; Gemini: location.
	// Set by handler after credential resolution. Excluded from JSON serialization.
	ProviderConfig map[string]any `json:"-"`

	// CustomHeaders holds custom HTTP headers for proxies/custom providers (not persisted).
	// Set by handler after credential resolution. Excluded from JSON serialization.
	CustomHeaders map[string]string `json:"-"`
}

// Value implements driver.Valuer for GORM JSONB storage
func (mc ModelConfig) Value() (driver.Value, error) {
	return json.Marshal(mc)
}

// Scan implements sql.Scanner for GORM JSONB retrieval
func (mc *ModelConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, mc)
	case string:
		return json.Unmarshal([]byte(v), mc)
	}
	return nil
}

// TextTemplate represents the template structure for text prompts.
type TextTemplate struct {
	Content string `json:"content"`
}

// ChatTemplate represents the template structure for chat prompts.
type ChatTemplate struct {
	Messages []ChatMessage `json:"messages"`
}

// JSON is a custom type for JSONB columns
type JSON json.RawMessage

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return []byte(j), nil
}

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append((*j)[0:0], v...)
	case string:
		*j = append((*j)[0:0], []byte(v)...)
	}
	return nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return nil
	}
	*j = append((*j)[0:0], data...)
	return nil
}

// CreatePromptRequest is the request for creating a new prompt with initial version.
type CreatePromptRequest struct {
	Name          string       `json:"name" validate:"required,min=1,max=100"`
	Type          PromptType   `json:"type,omitempty"`
	Description   string       `json:"description,omitempty"`
	Tags          []string     `json:"tags,omitempty"`
	Template      interface{}  `json:"template" validate:"required"`
	Config        *ModelConfig `json:"config,omitempty"`
	Labels        []string     `json:"labels,omitempty"`
	CommitMessage string       `json:"commit_message,omitempty"`
}

type UpdatePromptRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type CreateVersionRequest struct {
	Template      interface{}  `json:"template" validate:"required"`
	Config        *ModelConfig `json:"config,omitempty"`
	Labels        []string     `json:"labels,omitempty"`
	CommitMessage string       `json:"commit_message,omitempty"`
}

// UpsertPromptRequest is the SDK request for creating or updating a prompt.
type UpsertPromptRequest struct {
	Name          string       `json:"name" validate:"required,min=1,max=100"`
	Type          PromptType   `json:"type,omitempty"`
	Description   string       `json:"description,omitempty"`
	Tags          []string     `json:"tags,omitempty"`
	Template      interface{}  `json:"template" validate:"required"`
	Config        *ModelConfig `json:"config,omitempty"`
	Labels        []string     `json:"labels,omitempty"`
	CommitMessage string       `json:"commit_message,omitempty"`
}

type SetLabelsRequest struct {
	Labels []string `json:"labels" validate:"required"`
}

type GetPromptOptions struct {
	Label       string `form:"label"`
	Version     *int   `form:"version"`
	CacheTTL    *int   `form:"cache_ttl"`
	BypassCache bool   `form:"-"`
}

type ProtectedLabelsRequest struct {
	ProtectedLabels []string `json:"protected_labels" validate:"required"`
}

// PromptResponse is the response for a prompt with version info.
type PromptResponse struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	Name          string          `json:"name"`
	Type          PromptType      `json:"type"`
	Description   string          `json:"description,omitempty"`
	Tags          []string        `json:"tags"`
	Version       int             `json:"version"`
	VersionID     string          `json:"version_id"` // UUID of the specific version (for linking)
	Labels        []string        `json:"labels"`
	Template      interface{}     `json:"template"`
	Config        *ModelConfig    `json:"config,omitempty"`
	Variables     []string        `json:"variables"`
	Dialect       TemplateDialect `json:"dialect,omitempty"` // Template dialect (simple, mustache, jinja2)
	CommitMessage string          `json:"commit_message,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CreatedBy     string          `json:"created_by,omitempty"`
	IsFallback    bool            `json:"is_fallback,omitempty"`
}

// PromptListItem is a summary item for prompt listing.
type PromptListItem struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Type          PromptType            `json:"type"`
	Description   string                `json:"description,omitempty"`
	Tags          []string              `json:"tags"`
	LatestVersion int                   `json:"latest_version"`
	Labels        []PromptListLabelInfo `json:"labels"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

// PromptListLabelInfo shows label-to-version mapping in list view.
type PromptListLabelInfo struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

type VersionResponse struct {
	ID            string          `json:"id"`
	Version       int             `json:"version"`
	Template      interface{}     `json:"template"`
	Config        *ModelConfig    `json:"config,omitempty"`
	Variables     []string        `json:"variables"`
	Dialect       TemplateDialect `json:"dialect,omitempty"` // Template dialect (simple, mustache, jinja2)
	CommitMessage string          `json:"commit_message,omitempty"`
	Labels        []string        `json:"labels"`
	CreatedAt     time.Time       `json:"created_at"`
	CreatedBy     string          `json:"created_by,omitempty"`
}

type VersionDiffResponse struct {
	FromVersion      int         `json:"from_version"`
	ToVersion        int         `json:"to_version"`
	TemplateFrom     interface{} `json:"template_from"`
	TemplateTo       interface{} `json:"template_to"`
	VariablesAdded   []string    `json:"variables_added"`
	VariablesRemoved []string    `json:"variables_removed"`
}

type ExecutePromptResponse struct {
	CompiledPrompt interface{}  `json:"compiled_prompt"`
	Response       *LLMResponse `json:"response,omitempty"`
	LatencyMs      int64        `json:"latency_ms"`
	Error          string       `json:"error,omitempty"`
}

type LLMResponse struct {
	Content      string            `json:"content"`
	Model        string            `json:"model"`
	Usage        *LLMUsage         `json:"usage,omitempty"`
	Cost         *float64          `json:"cost,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
	ToolCalls    []json.RawMessage `json:"tool_calls,omitempty" swaggertype:"array,object"`
}

type LLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type StreamEventType string

const (
	StreamEventStart   StreamEventType = "start"   // Stream started
	StreamEventContent StreamEventType = "content" // Content chunk received
	StreamEventEnd     StreamEventType = "end"     // Stream completed
	StreamEventError   StreamEventType = "error"   // Stream error occurred
)

type StreamEvent struct {
	Type         StreamEventType `json:"type"`
	Content      string          `json:"content,omitempty"`       // For content events
	Error        string          `json:"error,omitempty"`         // For error events
	FinishReason string          `json:"finish_reason,omitempty"` // For end events
}

// StreamResult contains the final metrics after streaming completes.
type StreamResult struct {
	Content       string            `json:"content"`                                         // Full accumulated content
	Model         string            `json:"model"`                                           // Model used
	Usage         *LLMUsage         `json:"usage,omitempty"`                                 // Token usage
	Cost          *float64          `json:"cost,omitempty"`                                  // Calculated cost
	FinishReason  string            `json:"finish_reason,omitempty"`                         // Completion reason
	TTFTMs        *float64          `json:"ttft_ms,omitempty"`                               // Time to first token (ms)
	TotalDuration int64             `json:"total_duration_ms,omitempty"`                     // Total execution time (ms)
	ToolCalls     []json.RawMessage `json:"tool_calls,omitempty" swaggertype:"array,object"` // Tool calls if finish_reason is "tool_calls"
}

// UpsertResponse is the response for the SDK upsert endpoint.
type UpsertResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        PromptType `json:"type"`
	Version     int        `json:"version"`
	IsNewPrompt bool       `json:"is_new_prompt"`
	Labels      []string   `json:"labels"`
	CreatedAt   time.Time  `json:"created_at"`
}

func NewPrompt(projectID uuid.UUID, name string, promptType PromptType, description string, tags []string) *Prompt {
	now := time.Now()
	return &Prompt{
		ID:          uid.New(),
		ProjectID:   projectID,
		Name:        name,
		Type:        promptType,
		Description: description,
		Tags:        pq.StringArray(tags),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func NewVersion(promptID uuid.UUID, version int, template JSON, config *ModelConfig, variables []string, commitMessage string, createdBy *uuid.UUID) *Version {
	return &Version{
		ID:            uid.New(),
		PromptID:      promptID,
		Version:       version,
		Template:      template,
		Config:        config,
		Variables:     pq.StringArray(variables),
		CommitMessage: commitMessage,
		CreatedBy:     createdBy,
		CreatedAt:     time.Now(),
	}
}

func NewLabel(promptID, versionID uuid.UUID, name string, createdBy *uuid.UUID) *Label {
	now := time.Now()
	return &Label{
		ID:        uid.New(),
		PromptID:  promptID,
		VersionID: versionID,
		Name:      name,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func NewProtectedLabel(projectID uuid.UUID, labelName string, createdBy *uuid.UUID) *ProtectedLabel {
	return &ProtectedLabel{
		ID:        uid.New(),
		ProjectID: projectID,
		LabelName: labelName,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}
}

func (Prompt) TableName() string         { return "prompts" }
func (Version) TableName() string        { return "prompt_versions" }
func (Label) TableName() string          { return "prompt_labels" }
func (ProtectedLabel) TableName() string { return "prompt_protected_labels" }

func (p *Prompt) IsDeleted() bool {
	return p.DeletedAt.Valid
}

func (p *Prompt) GetLatestVersion() int {
	if len(p.Versions) == 0 {
		return 0
	}
	max := 0
	for _, v := range p.Versions {
		if v.Version > max {
			max = v.Version
		}
	}
	return max
}

func (v *Version) GetTextTemplate() (*TextTemplate, error) {
	var t TextTemplate
	if err := json.Unmarshal(v.Template, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (v *Version) GetChatTemplate() (*ChatTemplate, error) {
	var t ChatTemplate
	if err := json.Unmarshal(v.Template, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (v *Version) SetTemplate(template interface{}) error {
	data, err := json.Marshal(template)
	if err != nil {
		return err
	}
	v.Template = data
	return nil
}

func (l *Label) IsLatestLabel() bool {
	return l.Name == "latest"
}

// LabelLatest is the reserved label name for the auto-managed latest version.
const LabelLatest = "latest"
