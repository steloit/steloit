package dashboard

import (
	"context"

	"github.com/google/uuid"
)

// DashboardService defines the dashboard management service interface.
type DashboardService interface {
	// Dashboard CRUD operations
	CreateDashboard(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *CreateDashboardRequest) (*Dashboard, error)
	GetDashboard(ctx context.Context, id uuid.UUID) (*Dashboard, error)
	GetDashboardByProject(ctx context.Context, projectID, dashboardID uuid.UUID) (*Dashboard, error)
	UpdateDashboard(ctx context.Context, projectID, dashboardID uuid.UUID, req *UpdateDashboardRequest) (*Dashboard, error)
	DeleteDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) error

	// List operations
	ListDashboards(ctx context.Context, projectID uuid.UUID, filter *DashboardFilter) (*DashboardListResponse, error)

	// Widget operations
	AddWidget(ctx context.Context, projectID, dashboardID uuid.UUID, widget *Widget) (*Dashboard, error)
	UpdateWidget(ctx context.Context, projectID, dashboardID uuid.UUID, widgetID string, widget *Widget) (*Dashboard, error)
	RemoveWidget(ctx context.Context, projectID, dashboardID uuid.UUID, widgetID string) (*Dashboard, error)

	// Layout operations
	UpdateLayout(ctx context.Context, projectID, dashboardID uuid.UUID, layout []LayoutItem) (*Dashboard, error)

	// Duplication
	DuplicateDashboard(ctx context.Context, projectID, dashboardID uuid.UUID, req *DuplicateDashboardRequest) (*Dashboard, error)

	// Lock operations
	LockDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) (*Dashboard, error)
	UnlockDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) (*Dashboard, error)

	// Export/Import operations
	ExportDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) (*DashboardExport, error)
	ImportDashboard(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *DashboardImportRequest) (*Dashboard, error)

	// Validation
	ValidateDashboardConfig(config *DashboardConfig) error
	ValidateWidgetQuery(query *WidgetQuery) error
}

// TemplateService defines the template management service interface.
type TemplateService interface {
	// ListTemplates retrieves all active templates.
	ListTemplates(ctx context.Context) ([]*Template, error)

	// GetTemplate retrieves a template by ID.
	GetTemplate(ctx context.Context, id uuid.UUID) (*Template, error)

	// CreateFromTemplate creates a new dashboard from a template.
	CreateFromTemplate(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *CreateFromTemplateRequest) (*Dashboard, error)
}
