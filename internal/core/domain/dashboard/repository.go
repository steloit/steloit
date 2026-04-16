package dashboard

import (
	"context"

	"github.com/google/uuid"
)

// DashboardRepository defines the interface for dashboard data access.
type DashboardRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, dashboard *Dashboard) error
	GetByID(ctx context.Context, id uuid.UUID) (*Dashboard, error)
	Update(ctx context.Context, dashboard *Dashboard) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Project-scoped queries
	GetByProjectID(ctx context.Context, projectID uuid.UUID, filter *DashboardFilter) (*DashboardListResponse, error)
	GetByNameAndProject(ctx context.Context, projectID uuid.UUID, name string) (*Dashboard, error)

	// Soft delete operations
	SoftDelete(ctx context.Context, id uuid.UUID) error

	// Count operations
	CountByProject(ctx context.Context, projectID uuid.UUID) (int64, error)
}

// TemplateRepository defines the interface for dashboard template data access.
type TemplateRepository interface {
	// List retrieves all active templates with optional filtering.
	List(ctx context.Context, filter *TemplateFilter) ([]*Template, error)

	// GetByID retrieves a template by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*Template, error)

	// GetByName retrieves a template by its name.
	GetByName(ctx context.Context, name string) (*Template, error)

	// GetByCategory retrieves a template by its category.
	GetByCategory(ctx context.Context, category TemplateCategory) (*Template, error)

	// Create creates a new template (used for seeding).
	Create(ctx context.Context, template *Template) error

	// Update updates an existing template.
	Update(ctx context.Context, template *Template) error

	// Delete removes a template by its ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// Upsert creates or updates a template by name (used for seeding).
	Upsert(ctx context.Context, template *Template) error
}
