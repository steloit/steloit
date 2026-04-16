package prompt

import (
	"context"

	"brokle/pkg/ulid"
)

// PromptService defines the prompt management service interface.
type PromptService interface {
	// Prompt CRUD operations
	CreatePrompt(ctx context.Context, projectID ulid.ULID, userID *ulid.ULID, req *CreatePromptRequest) (*Prompt, *Version, []string, error)
	GetPrompt(ctx context.Context, projectID ulid.ULID, name string, opts *GetPromptOptions) (*PromptResponse, error)
	GetPromptByID(ctx context.Context, projectID, promptID ulid.ULID) (*Prompt, error)
	UpdatePrompt(ctx context.Context, projectID, promptID ulid.ULID, req *UpdatePromptRequest) (*Prompt, error)
	DeletePrompt(ctx context.Context, projectID, promptID ulid.ULID) error
	ListPrompts(ctx context.Context, projectID ulid.ULID, filters *PromptFilters) ([]*PromptListItem, int64, error)

	// SDK upsert operation (create prompt or new version)
	UpsertPrompt(ctx context.Context, projectID ulid.ULID, userID *ulid.ULID, req *UpsertPromptRequest) (*UpsertResponse, error)

	// Version operations
	CreateVersion(ctx context.Context, projectID, promptID ulid.ULID, userID *ulid.ULID, req *CreateVersionRequest) (*Version, []string, error)
	GetVersion(ctx context.Context, projectID, promptID ulid.ULID, version int) (*VersionResponse, error)
	GetVersionEntity(ctx context.Context, projectID, promptID, versionID ulid.ULID) (*Version, error)
	GetVersionByID(ctx context.Context, projectID, promptID, versionID ulid.ULID) (*VersionResponse, error)
	ListVersions(ctx context.Context, projectID, promptID ulid.ULID) ([]*VersionResponse, error)
	GetVersionDiff(ctx context.Context, projectID, promptID ulid.ULID, fromVersion, toVersion int) (*VersionDiffResponse, error)

	// Label operations
	SetLabels(ctx context.Context, projectID, promptID, versionID ulid.ULID, userID *ulid.ULID, labels []string) ([]string, error)
	RemoveLabel(ctx context.Context, projectID, promptID ulid.ULID, userID *ulid.ULID, labelName string) error
	GetVersionByLabel(ctx context.Context, projectID, promptID ulid.ULID, label string) (*Version, error)

	// Protected labels
	GetProtectedLabels(ctx context.Context, projectID ulid.ULID) ([]string, error)
	SetProtectedLabels(ctx context.Context, projectID ulid.ULID, userID *ulid.ULID, labels []string) ([]string, error)
	IsLabelProtected(ctx context.Context, projectID ulid.ULID, labelName string) (bool, error)

	// Cache operations
	InvalidateCache(ctx context.Context, projectID ulid.ULID, promptName string) error
}

// CompilerService defines the template compilation service interface.
type CompilerService interface {
	// Variable extraction
	ExtractVariables(template interface{}, promptType PromptType) ([]string, error)

	// Template compilation
	Compile(template interface{}, promptType PromptType, variables map[string]string) (interface{}, error)
	CompileText(template string, variables map[string]string) (string, error)
	CompileChat(messages []ChatMessage, variables map[string]string) ([]ChatMessage, error)

	// Validation
	ValidateTemplate(template interface{}, promptType PromptType) error
	ValidateVariables(required []string, provided map[string]string) error

	// Dialect-aware operations (new)
	DetectDialect(template interface{}, promptType PromptType) (TemplateDialect, error)
	ValidateSyntax(template interface{}, promptType PromptType, dialect TemplateDialect) (*ValidationResult, error)
	CompileWithDialect(template interface{}, promptType PromptType, variables map[string]any, dialect TemplateDialect) (interface{}, error)
	ExtractVariablesWithDialect(template interface{}, promptType PromptType, dialect TemplateDialect) ([]string, error)

	// Registry access
	GetDialectRegistry() DialectRegistry
}

// ExecutionService defines the prompt execution service interface.
type ExecutionService interface {
	// Execute prompt with LLM (non-streaming)
	Execute(ctx context.Context, prompt *PromptResponse, variables map[string]string, configOverrides *ModelConfig) (*ExecutePromptResponse, error)

	// ExecuteStream executes a prompt with real-time streaming.
	// Returns two channels: one for events (content chunks) and one for the final result.
	// The caller must consume both channels until they close.
	ExecuteStream(ctx context.Context, prompt *PromptResponse, variables map[string]string, configOverrides *ModelConfig) (<-chan StreamEvent, <-chan *StreamResult, error)

	// Compile and preview without execution
	Preview(ctx context.Context, prompt *PromptResponse, variables map[string]string) (interface{}, error)
}
