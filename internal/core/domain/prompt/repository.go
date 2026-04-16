package prompt

import (
	"context"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/pagination"
)

// PromptRepository defines the interface for prompt data access.
type PromptRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, prompt *Prompt) error
	GetByID(ctx context.Context, id uuid.UUID) (*Prompt, error)
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (*Prompt, error)
	Update(ctx context.Context, prompt *Prompt) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Project-scoped queries
	ListByProject(ctx context.Context, projectID uuid.UUID, filters *PromptFilters) ([]*Prompt, int64, error)
	CountByProject(ctx context.Context, projectID uuid.UUID) (int64, error)

	// Soft delete operations
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Restore(ctx context.Context, id uuid.UUID) error
}

// VersionRepository defines the interface for prompt version data access.
type VersionRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, version *Version) error
	GetByID(ctx context.Context, id uuid.UUID) (*Version, error)
	Delete(ctx context.Context, id uuid.UUID) error

	// Version queries
	GetByPromptAndVersion(ctx context.Context, promptID uuid.UUID, version int) (*Version, error)
	GetLatestByPrompt(ctx context.Context, promptID uuid.UUID) (*Version, error)
	GetLatestByPrompts(ctx context.Context, promptIDs []uuid.UUID) ([]*Version, error)
	GetByIDs(ctx context.Context, versionIDs []uuid.UUID) ([]*Version, error)
	ListByPrompt(ctx context.Context, promptID uuid.UUID) ([]*Version, error)

	// Get next version number (atomic)
	GetNextVersionNumber(ctx context.Context, promptID uuid.UUID) (int, error)

	// Version count
	CountByPrompt(ctx context.Context, promptID uuid.UUID) (int64, error)
}

// LabelRepository defines the interface for prompt label data access.
type LabelRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, label *Label) error
	GetByID(ctx context.Context, id uuid.UUID) (*Label, error)
	Update(ctx context.Context, label *Label) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Label queries
	GetByPromptAndName(ctx context.Context, promptID uuid.UUID, name string) (*Label, error)
	ListByPrompt(ctx context.Context, promptID uuid.UUID) ([]*Label, error)
	ListByPrompts(ctx context.Context, promptIDs []uuid.UUID) ([]*Label, error)
	ListByVersion(ctx context.Context, versionID uuid.UUID) ([]*Label, error)
	ListByVersions(ctx context.Context, versionIDs []uuid.UUID) ([]*Label, error)

	// Atomic label operations
	SetLabel(ctx context.Context, promptID, versionID uuid.UUID, name string, createdBy *uuid.UUID) error
	RemoveLabel(ctx context.Context, promptID uuid.UUID, name string) error

	// Delete all labels for a prompt
	DeleteByPrompt(ctx context.Context, promptID uuid.UUID) error
}

// ProtectedLabelRepository defines the interface for protected label data access.
type ProtectedLabelRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, label *ProtectedLabel) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Project-scoped queries
	GetByProjectAndLabel(ctx context.Context, projectID uuid.UUID, labelName string) (*ProtectedLabel, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]*ProtectedLabel, error)
	IsProtected(ctx context.Context, projectID uuid.UUID, labelName string) (bool, error)

	// Bulk operations
	SetProtectedLabels(ctx context.Context, projectID uuid.UUID, labels []string, createdBy *uuid.UUID) error
	DeleteByProject(ctx context.Context, projectID uuid.UUID) error
}

// CacheRepository defines the interface for prompt caching.
type CacheRepository interface {
	// Cache operations
	Get(ctx context.Context, key string) (*CachedPrompt, error)
	Set(ctx context.Context, key string, prompt *CachedPrompt, ttl time.Duration) error
	Delete(ctx context.Context, key string) error

	// Pattern-based invalidation
	DeleteByPattern(ctx context.Context, pattern string) error

	// Key generation helpers
	BuildKey(projectID uuid.UUID, name string, labelOrVersion string) string
}

// CachedPrompt represents a prompt stored in the cache.
type CachedPrompt struct {
	PromptID      string       `json:"prompt_id"`
	ProjectID     string       `json:"project_id"`
	Name          string       `json:"name"`
	Type          PromptType   `json:"type"`
	Description   string       `json:"description"`
	Tags          []string     `json:"tags"`
	Version       int          `json:"version"`
	VersionID     string       `json:"version_id"`
	Labels        []string     `json:"labels"`
	Template      interface{}  `json:"template"`
	Config        *ModelConfig `json:"config,omitempty"`
	Variables     []string     `json:"variables"`
	CommitMessage string       `json:"commit_message"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	CreatedBy     string       `json:"created_by"`
	CachedAt      time.Time    `json:"cached_at"`
	ExpiresAt     time.Time    `json:"expires_at"`
}

// IsExpired checks if the cached prompt has expired.
func (c *CachedPrompt) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// IsStale checks if the cached prompt is stale but still usable.
func (c *CachedPrompt) IsStale(ttl time.Duration) bool {
	staleAt := c.CachedAt.Add(ttl)
	return time.Now().After(staleAt)
}

// PromptFilters represents filters for prompt queries.
type PromptFilters struct {
	// Domain filters
	Type   *PromptType
	Tags   []string
	Search *string

	// Pagination (embedded for DRY)
	pagination.Params
}

// VersionFilters represents filters for version queries.
type VersionFilters struct {
	// Domain filters
	PromptID *uuid.UUID

	// Pagination (embedded for DRY)
	pagination.Params
}

// Repository aggregates all prompt-related repositories.
type Repository interface {
	Prompts() PromptRepository
	Versions() VersionRepository
	Labels() LabelRepository
	ProtectedLabels() ProtectedLabelRepository
	Cache() CacheRepository
}
