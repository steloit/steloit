package prompt

import (
	"context"

	"github.com/google/uuid"
)

// PromptService defines the prompt management service interface.
type PromptService interface {
	// Prompt CRUD operations
	CreatePrompt(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *CreatePromptRequest) (*Prompt, *Version, []string, error)
	GetPrompt(ctx context.Context, projectID uuid.UUID, name string, opts *GetPromptOptions) (*PromptResponse, error)
	GetPromptByID(ctx context.Context, projectID, promptID uuid.UUID) (*Prompt, error)
	UpdatePrompt(ctx context.Context, projectID, promptID uuid.UUID, req *UpdatePromptRequest) (*Prompt, error)
	DeletePrompt(ctx context.Context, projectID, promptID uuid.UUID) error
	ListPrompts(ctx context.Context, projectID uuid.UUID, filters *PromptFilters) ([]*PromptListItem, int64, error)

	// SDK upsert operation (create prompt or new version)
	UpsertPrompt(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *UpsertPromptRequest) (*UpsertResponse, error)

	// Version operations
	CreateVersion(ctx context.Context, projectID, promptID uuid.UUID, userID *uuid.UUID, req *CreateVersionRequest) (*Version, []string, error)
	GetVersion(ctx context.Context, projectID, promptID uuid.UUID, version int) (*VersionResponse, error)
	GetVersionEntity(ctx context.Context, projectID, promptID, versionID uuid.UUID) (*Version, error)
	GetVersionByID(ctx context.Context, projectID, promptID, versionID uuid.UUID) (*VersionResponse, error)
	ListVersions(ctx context.Context, projectID, promptID uuid.UUID) ([]*VersionResponse, error)
	GetVersionDiff(ctx context.Context, projectID, promptID uuid.UUID, fromVersion, toVersion int) (*VersionDiffResponse, error)

	// Label operations
	SetLabels(ctx context.Context, projectID, promptID, versionID uuid.UUID, userID *uuid.UUID, labels []string) ([]string, error)
	RemoveLabel(ctx context.Context, projectID, promptID uuid.UUID, userID *uuid.UUID, labelName string) error
	GetVersionByLabel(ctx context.Context, projectID, promptID uuid.UUID, label string) (*Version, error)

	// Protected labels
	GetProtectedLabels(ctx context.Context, projectID uuid.UUID) ([]string, error)
	SetProtectedLabels(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, labels []string) ([]string, error)
	IsLabelProtected(ctx context.Context, projectID uuid.UUID, labelName string) (bool, error)

	// Cache operations
	InvalidateCache(ctx context.Context, projectID uuid.UUID, promptName string) error
}

// CompilerService defines the template compilation service interface.
type CompilerService interface {
	// Variable extraction
	ExtractVariables(template any, promptType PromptType) ([]string, error)

	// Template compilation
	Compile(template any, promptType PromptType, variables map[string]string) (any, error)
	CompileText(template string, variables map[string]string) (string, error)
	CompileChat(messages []ChatMessage, variables map[string]string) ([]ChatMessage, error)

	// Validation
	ValidateTemplate(template any, promptType PromptType) error
	ValidateVariables(required []string, provided map[string]string) error

	// Dialect-aware operations (new)
	DetectDialect(template any, promptType PromptType) (TemplateDialect, error)
	ValidateSyntax(template any, promptType PromptType, dialect TemplateDialect) (*ValidationResult, error)
	CompileWithDialect(template any, promptType PromptType, variables map[string]any, dialect TemplateDialect) (any, error)
	ExtractVariablesWithDialect(template any, promptType PromptType, dialect TemplateDialect) ([]string, error)

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
	Preview(ctx context.Context, prompt *PromptResponse, variables map[string]string) (any, error)
}
