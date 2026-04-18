// Package dashboard provides the dashboard management domain model.
//
// The dashboard domain handles custom dashboards with configurable widgets,
// query-based data visualization, and grid-based layout management.
package dashboard

import (
	"time"

	"github.com/google/uuid"
)

// WidgetType defines the type of widget
type WidgetType string

const (
	WidgetTypeStat       WidgetType = "stat"
	WidgetTypeTimeSeries WidgetType = "time_series"
	WidgetTypeTable      WidgetType = "table"
	WidgetTypeBar        WidgetType = "bar"
	WidgetTypePie        WidgetType = "pie"
	WidgetTypeHeatmap    WidgetType = "heatmap"
	WidgetTypeHistogram  WidgetType = "histogram"
	WidgetTypeTraceList  WidgetType = "trace_list"
	WidgetTypeText       WidgetType = "text"
)

// IsValid checks if the widget type is valid
func (wt WidgetType) IsValid() bool {
	switch wt {
	case WidgetTypeStat, WidgetTypeTimeSeries, WidgetTypeTable,
		WidgetTypeBar, WidgetTypePie, WidgetTypeHeatmap,
		WidgetTypeHistogram, WidgetTypeTraceList, WidgetTypeText:
		return true
	default:
		return false
	}
}

// ViewType defines the data source view for widget queries
type ViewType string

const (
	ViewTypeTraces ViewType = "traces"
	ViewTypeSpans  ViewType = "spans"
	ViewTypeScores ViewType = "scores"
)

// IsValid checks if the view type is valid
func (vt ViewType) IsValid() bool {
	switch vt {
	case ViewTypeTraces, ViewTypeSpans, ViewTypeScores:
		return true
	default:
		return false
	}
}

// FilterOperator defines valid filter operators
type FilterOperator string

const (
	FilterOpEqual       FilterOperator = "eq"
	FilterOpNotEqual    FilterOperator = "neq"
	FilterOpGreaterThan FilterOperator = "gt"
	FilterOpLessThan    FilterOperator = "lt"
	FilterOpGTE         FilterOperator = "gte"
	FilterOpLTE         FilterOperator = "lte"
	FilterOpContains    FilterOperator = "contains"
	FilterOpIn          FilterOperator = "in"
)

// QueryFilter represents a filter condition for widget queries.
type QueryFilter struct {
	Field    string         `json:"field"`
	Operator FilterOperator `json:"operator"`
	Value    any    `json:"value"`
}

// TimeRange defines a time range for widget queries.
type TimeRange struct {
	From     *time.Time `json:"from,omitempty"`
	To       *time.Time `json:"to,omitempty"`
	Relative string     `json:"relative,omitempty"` // "1h", "24h", "7d", "30d"
}

// WidgetQuery defines the data query configuration for a widget.
type WidgetQuery struct {
	View       ViewType      `json:"view"`                 // "traces", "spans", "scores"
	Measures   []string      `json:"measures"`             // ["count", "latency_p50", "total_cost"]
	Dimensions []string      `json:"dimensions,omitempty"` // grouping fields
	Filters    []QueryFilter `json:"filters,omitempty"`    // filter conditions
	TimeRange  *TimeRange    `json:"time_range,omitempty"` // time range
	Limit      int           `json:"limit,omitempty"`      // result limit
	OrderBy    string        `json:"order_by,omitempty"`   // order by field
	OrderDir   string        `json:"order_dir,omitempty"`  // "asc" or "desc"
}

// Widget represents a dashboard widget configuration.
type Widget struct {
	ID          string                 `json:"id"`
	Type        WidgetType             `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Query       WidgetQuery            `json:"query"`
	Config      map[string]any `json:"config,omitempty"` // widget-specific config
}

// LayoutItem defines widget position in the dashboard grid.
type LayoutItem struct {
	WidgetID string `json:"widget_id"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	W        int    `json:"w"`
	H        int    `json:"h"`
}

// VariableQueryConfig defines how to fetch dynamic options for a "query" type variable.
type VariableQueryConfig struct {
	View      ViewType `json:"view"`            // data source view (traces, spans, scores)
	Dimension string   `json:"dimension"`       // dimension field to get distinct values from
	Limit     int      `json:"limit,omitempty"` // max options to fetch (default: 100)
}

// Variable represents a dashboard-level variable for dynamic filtering.
type Variable struct {
	Name        string               `json:"name"`
	Type        string               `json:"type"`                   // "string", "number", "select", "query"
	Label       string               `json:"label,omitempty"`        // display label (defaults to name)
	Default     any                  `json:"default,omitempty"`      // default value
	Options     []string             `json:"options,omitempty"`      // for select type - static options
	QueryConfig *VariableQueryConfig `json:"query_config,omitempty"` // for query type - dynamic options
	Multi       bool                 `json:"multi,omitempty"`        // allow multiple values
}

// DashboardConfig holds dashboard-level configuration including widgets.
type DashboardConfig struct {
	Widgets     []Widget   `json:"widgets"`
	RefreshRate int        `json:"refresh_rate,omitempty"` // seconds
	TimeRange   *TimeRange `json:"time_range,omitempty"`   // dashboard-level time range
	Variables   []Variable `json:"variables,omitempty"`    // dashboard variables
}

// Dashboard represents a project dashboard with widget configurations.
type Dashboard struct {
	ID          uuid.UUID       `json:"id"`
	ProjectID   uuid.UUID       `json:"project_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Config      DashboardConfig `json:"config"`
	Layout      []LayoutItem    `json:"layout"`
	IsLocked    bool            `json:"is_locked"`
	CreatedBy   *uuid.UUID      `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty" swaggertype:"string"`
}

// TableName returns the database table name for Dashboard.
// CreateDashboardRequest represents the request to create a new dashboard.
type CreateDashboardRequest struct {
	Name        string          `json:"name" binding:"required,min=1,max=255"`
	Description string          `json:"description,omitempty"`
	Config      DashboardConfig `json:"config,omitempty"`
	Layout      []LayoutItem    `json:"layout,omitempty"`
}

// UpdateDashboardRequest represents the request to update a dashboard.
type UpdateDashboardRequest struct {
	Name        *string          `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Description *string          `json:"description,omitempty"`
	Config      *DashboardConfig `json:"config,omitempty"`
	Layout      []LayoutItem     `json:"layout,omitempty"`
}

// DashboardFilter represents filters for dashboard queries.
type DashboardFilter struct {
	ProjectID uuid.UUID
	Name      string
	Limit     int
	Offset    int
}

// DashboardListResponse represents a paginated list of dashboards.
type DashboardListResponse struct {
	Dashboards []*Dashboard `json:"dashboards"`
	Total      int64        `json:"total"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
}

// TemplateCategory defines the category of a dashboard template
type TemplateCategory string

const (
	TemplateCategoryLLMOverview   TemplateCategory = "llm-overview"
	TemplateCategoryCostAnalytics TemplateCategory = "cost-analytics"
	TemplateCategoryQualityScores TemplateCategory = "quality-scores"
)

// IsValid checks if the template category is valid
func (tc TemplateCategory) IsValid() bool {
	switch tc {
	case TemplateCategoryLLMOverview, TemplateCategoryCostAnalytics, TemplateCategoryQualityScores:
		return true
	default:
		return false
	}
}

// Template represents a pre-defined dashboard template.
type Template struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Category    TemplateCategory `json:"category"`
	Config      DashboardConfig  `json:"config"`
	Layout      []LayoutItem     `json:"layout"`
	IsActive    bool             `json:"is_active"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// TableName returns the database table name for Template.
// TemplateFilter represents filters for template queries.
type TemplateFilter struct {
	Category *TemplateCategory
	IsActive *bool
}

// CreateFromTemplateRequest represents the request to create a dashboard from a template.
type CreateFromTemplateRequest struct {
	TemplateID uuid.UUID `json:"template_id" binding:"required"`
	Name       string    `json:"name" binding:"required,min=1,max=255"`
}

// DuplicateDashboardRequest represents the request to duplicate a dashboard.
type DuplicateDashboardRequest struct {
	Name string `json:"name" binding:"required,min=1,max=255"`
}

// DashboardExport represents an exported dashboard for sharing or backup.
type DashboardExport struct {
	Version     string          `json:"version"`
	ExportedAt  time.Time       `json:"exported_at"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Config      DashboardConfig `json:"config"`
	Layout      []LayoutItem    `json:"layout"`
}

// DashboardImportRequest represents the request to import a dashboard.
type DashboardImportRequest struct {
	Data DashboardExport `json:"data" binding:"required"`
	Name string          `json:"name,omitempty"`
}
