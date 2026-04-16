package dashboard

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	dashboardDomain "brokle/internal/core/domain/dashboard"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type dashboardService struct {
	repo   dashboardDomain.DashboardRepository
	logger *slog.Logger
}

func NewDashboardService(
	repo dashboardDomain.DashboardRepository,
	logger *slog.Logger,
) dashboardDomain.DashboardService {
	return &dashboardService{
		repo:   repo,
		logger: logger,
	}
}

func (s *dashboardService) CreateDashboard(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *dashboardDomain.CreateDashboardRequest) (*dashboardDomain.Dashboard, error) {
	if req.Name == "" {
		return nil, appErrors.NewValidationError("name", "dashboard name is required")
	}

	if req.Config.Widgets != nil {
		if err := s.ValidateDashboardConfig(&req.Config); err != nil {
			return nil, err
		}
	}

	existing, err := s.repo.GetByNameAndProject(ctx, projectID, req.Name)
	if err != nil && !errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
		return nil, appErrors.NewInternalError("failed to check existing dashboard", err)
	}
	if existing != nil {
		return nil, appErrors.NewConflictError("dashboard with this name already exists")
	}

	config := req.Config
	if config.Widgets == nil {
		config.Widgets = []dashboardDomain.Widget{}
	}

	layout := req.Layout
	if layout == nil {
		layout = []dashboardDomain.LayoutItem{}
	}

	dashboard := &dashboardDomain.Dashboard{
		ID:          uid.New(),
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		Config:      config,
		Layout:      layout,
		CreatedBy:   userID,
	}

	if err := s.repo.Create(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to create dashboard", err)
	}

	s.logger.Info("dashboard created",
		"dashboard_id", dashboard.ID,
		"project_id", projectID,
		"name", dashboard.Name,
	)

	return dashboard, nil
}

func (s *dashboardService) GetDashboard(ctx context.Context, id uuid.UUID) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
			return nil, appErrors.NewNotFoundError("dashboard")
		}
		return nil, appErrors.NewInternalError("failed to get dashboard", err)
	}
	return dashboard, nil
}

func (s *dashboardService) GetDashboardByProject(ctx context.Context, projectID, dashboardID uuid.UUID) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.repo.GetByID(ctx, dashboardID)
	if err != nil {
		if errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
			return nil, appErrors.NewNotFoundError("dashboard")
		}
		return nil, appErrors.NewInternalError("failed to get dashboard", err)
	}

	if dashboard.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError("dashboard")
	}

	return dashboard, nil
}

func (s *dashboardService) UpdateDashboard(ctx context.Context, projectID, dashboardID uuid.UUID, req *dashboardDomain.UpdateDashboardRequest) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	if dashboard.IsLocked {
		return nil, appErrors.NewValidationError("is_locked", "cannot update a locked dashboard; unlock it first")
	}

	if req.Config != nil {
		if err := s.ValidateDashboardConfig(req.Config); err != nil {
			return nil, err
		}
	}

	if req.Name != nil && *req.Name != dashboard.Name {
		existing, err := s.repo.GetByNameAndProject(ctx, projectID, *req.Name)
		if err != nil && !errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
			return nil, appErrors.NewInternalError("failed to check existing dashboard", err)
		}
		if existing != nil {
			return nil, appErrors.NewConflictError("dashboard with this name already exists")
		}
		dashboard.Name = *req.Name
	}

	if req.Description != nil {
		dashboard.Description = *req.Description
	}

	if req.Config != nil {
		dashboard.Config = *req.Config
	}

	if req.Layout != nil {
		dashboard.Layout = req.Layout
	}

	if err := s.repo.Update(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to update dashboard", err)
	}

	s.logger.Info("dashboard updated",
		"dashboard_id", dashboardID,
		"project_id", projectID,
	)

	return dashboard, nil
}

func (s *dashboardService) DeleteDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) error {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return err
	}

	if dashboard.IsLocked {
		return appErrors.NewValidationError("is_locked", "cannot delete a locked dashboard; unlock it first")
	}

	if err := s.repo.SoftDelete(ctx, dashboardID); err != nil {
		return appErrors.NewInternalError("failed to delete dashboard", err)
	}

	s.logger.Info("dashboard deleted",
		"dashboard_id", dashboardID,
		"project_id", projectID,
	)

	return nil
}

func (s *dashboardService) ListDashboards(ctx context.Context, projectID uuid.UUID, filter *dashboardDomain.DashboardFilter) (*dashboardDomain.DashboardListResponse, error) {
	if filter == nil {
		filter = &dashboardDomain.DashboardFilter{}
	}
	filter.ProjectID = projectID

	resp, err := s.repo.GetByProjectID(ctx, projectID, filter)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list dashboards", err)
	}

	return resp, nil
}

func (s *dashboardService) AddWidget(ctx context.Context, projectID, dashboardID uuid.UUID, widget *dashboardDomain.Widget) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	if !widget.Type.IsValid() {
		return nil, appErrors.NewValidationError("widget.type", "invalid widget type")
	}

	if err := s.ValidateWidgetQuery(&widget.Query); err != nil {
		return nil, err
	}

	if widget.ID == "" {
		widget.ID = uid.New().String()
	}

	for _, w := range dashboard.Config.Widgets {
		if w.ID == widget.ID {
			return nil, appErrors.NewConflictError("widget with this ID already exists")
		}
	}

	dashboard.Config.Widgets = append(dashboard.Config.Widgets, *widget)

	if err := s.repo.Update(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to add widget", err)
	}

	return dashboard, nil
}

func (s *dashboardService) UpdateWidget(ctx context.Context, projectID, dashboardID uuid.UUID, widgetID string, widget *dashboardDomain.Widget) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	found := false
	for i, w := range dashboard.Config.Widgets {
		if w.ID == widgetID {
			widget.ID = widgetID // Preserve the ID
			dashboard.Config.Widgets[i] = *widget
			found = true
			break
		}
	}

	if !found {
		return nil, appErrors.NewNotFoundError("widget")
	}

	if !widget.Type.IsValid() {
		return nil, appErrors.NewValidationError("widget.type", "invalid widget type")
	}

	if err := s.ValidateWidgetQuery(&widget.Query); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to update widget", err)
	}

	return dashboard, nil
}

func (s *dashboardService) RemoveWidget(ctx context.Context, projectID, dashboardID uuid.UUID, widgetID string) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	found := false
	widgets := make([]dashboardDomain.Widget, 0, len(dashboard.Config.Widgets)-1)
	for _, w := range dashboard.Config.Widgets {
		if w.ID == widgetID {
			found = true
			continue
		}
		widgets = append(widgets, w)
	}

	if !found {
		return nil, appErrors.NewNotFoundError("widget")
	}

	dashboard.Config.Widgets = widgets

	layout := make([]dashboardDomain.LayoutItem, 0, len(dashboard.Layout))
	for _, item := range dashboard.Layout {
		if item.WidgetID != widgetID {
			layout = append(layout, item)
		}
	}
	dashboard.Layout = layout

	if err := s.repo.Update(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to remove widget", err)
	}

	return dashboard, nil
}

func (s *dashboardService) UpdateLayout(ctx context.Context, projectID, dashboardID uuid.UUID, layout []dashboardDomain.LayoutItem) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	widgetIDs := make(map[string]bool)
	for _, w := range dashboard.Config.Widgets {
		widgetIDs[w.ID] = true
	}

	for _, item := range layout {
		if !widgetIDs[item.WidgetID] {
			return nil, appErrors.NewValidationError("layout", "layout references non-existent widget: "+item.WidgetID)
		}
	}

	dashboard.Layout = layout

	if err := s.repo.Update(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to update layout", err)
	}

	return dashboard, nil
}

func (s *dashboardService) DuplicateDashboard(ctx context.Context, projectID, dashboardID uuid.UUID, req *dashboardDomain.DuplicateDashboardRequest) (*dashboardDomain.Dashboard, error) {
	if req.Name == "" {
		return nil, appErrors.NewValidationError("name", "dashboard name is required")
	}

	source, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByNameAndProject(ctx, projectID, req.Name)
	if err != nil && !errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
		return nil, appErrors.NewInternalError("failed to check existing dashboard", err)
	}
	if existing != nil {
		return nil, appErrors.NewConflictError("dashboard with this name already exists")
	}

	config := s.copyConfig(source.Config)
	layout := s.copyLayout(source.Layout)

	widgetIDMap := make(map[string]string)
	for i := range config.Widgets {
		oldID := config.Widgets[i].ID
		newID := uid.New().String()
		widgetIDMap[oldID] = newID
		config.Widgets[i].ID = newID
	}

	// Update layout widget references
	for i := range layout {
		if newID, ok := widgetIDMap[layout[i].WidgetID]; ok {
			layout[i].WidgetID = newID
		}
	}

	dashboard := &dashboardDomain.Dashboard{
		ID:          uid.New(),
		ProjectID:   projectID,
		Name:        req.Name,
		Description: source.Description,
		Config:      config,
		Layout:      layout,
		CreatedBy:   source.CreatedBy,
	}

	if err := s.repo.Create(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to duplicate dashboard", err)
	}

	s.logger.Info("dashboard duplicated",
		"source_id", dashboardID,
		"new_id", dashboard.ID,
		"project_id", projectID,
		"name", dashboard.Name,
	)

	return dashboard, nil
}

func (s *dashboardService) LockDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	if dashboard.IsLocked {
		return dashboard, nil // Already locked, return as-is
	}

	dashboard.IsLocked = true

	if err := s.repo.Update(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to lock dashboard", err)
	}

	s.logger.Info("dashboard locked",
		"dashboard_id", dashboardID,
		"project_id", projectID,
	)

	return dashboard, nil
}

func (s *dashboardService) UnlockDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) (*dashboardDomain.Dashboard, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	if !dashboard.IsLocked {
		return dashboard, nil // Already unlocked, return as-is
	}

	dashboard.IsLocked = false

	if err := s.repo.Update(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to unlock dashboard", err)
	}

	s.logger.Info("dashboard unlocked",
		"dashboard_id", dashboardID,
		"project_id", projectID,
	)

	return dashboard, nil
}

func (s *dashboardService) ExportDashboard(ctx context.Context, projectID, dashboardID uuid.UUID) (*dashboardDomain.DashboardExport, error) {
	dashboard, err := s.GetDashboardByProject(ctx, projectID, dashboardID)
	if err != nil {
		return nil, err
	}

	export := &dashboardDomain.DashboardExport{
		Version:     "1.0",
		ExportedAt:  time.Now().UTC(),
		Name:        dashboard.Name,
		Description: dashboard.Description,
		Config:      s.copyConfig(dashboard.Config),
		Layout:      s.copyLayout(dashboard.Layout),
	}

	s.logger.Info("dashboard exported",
		"dashboard_id", dashboardID,
		"project_id", projectID,
	)

	return export, nil
}

func (s *dashboardService) ImportDashboard(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *dashboardDomain.DashboardImportRequest) (*dashboardDomain.Dashboard, error) {
	name := req.Name
	if name == "" {
		name = req.Data.Name
	}
	if name == "" {
		return nil, appErrors.NewValidationError("name", "dashboard name is required")
	}

	if err := s.ValidateDashboardConfig(&req.Data.Config); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByNameAndProject(ctx, projectID, name)
	if err != nil && !errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
		return nil, appErrors.NewInternalError("failed to check existing dashboard", err)
	}
	if existing != nil {
		return nil, appErrors.NewConflictError("dashboard with this name already exists")
	}

	config := s.copyConfig(req.Data.Config)
	layout := s.copyLayout(req.Data.Layout)

	widgetIDMap := make(map[string]string)
	for i := range config.Widgets {
		oldID := config.Widgets[i].ID
		newID := uid.New().String()
		widgetIDMap[oldID] = newID
		config.Widgets[i].ID = newID
	}

	for i := range layout {
		if newID, ok := widgetIDMap[layout[i].WidgetID]; ok {
			layout[i].WidgetID = newID
		}
	}

	dashboard := &dashboardDomain.Dashboard{
		ID:          uid.New(),
		ProjectID:   projectID,
		Name:        name,
		Description: req.Data.Description,
		Config:      config,
		Layout:      layout,
		IsLocked:    false, // Imported dashboards are unlocked by default
		CreatedBy:   userID,
	}

	if err := s.repo.Create(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to import dashboard", err)
	}

	s.logger.Info("dashboard imported",
		"dashboard_id", dashboard.ID,
		"project_id", projectID,
		"name", dashboard.Name,
	)

	return dashboard, nil
}

func (s *dashboardService) copyConfig(src dashboardDomain.DashboardConfig) dashboardDomain.DashboardConfig {
	dst := dashboardDomain.DashboardConfig{
		RefreshRate: src.RefreshRate,
	}

	if src.Widgets != nil {
		dst.Widgets = make([]dashboardDomain.Widget, len(src.Widgets))
		for i, w := range src.Widgets {
			dst.Widgets[i] = dashboardDomain.Widget{
				ID:          w.ID,
				Type:        w.Type,
				Title:       w.Title,
				Description: w.Description,
				Query:       s.copyQuery(w.Query),
				Config:      copyMap(w.Config),
			}
		}
	}

	if src.TimeRange != nil {
		dst.TimeRange = &dashboardDomain.TimeRange{
			From:     src.TimeRange.From,
			To:       src.TimeRange.To,
			Relative: src.TimeRange.Relative,
		}
	}

	if src.Variables != nil {
		dst.Variables = make([]dashboardDomain.Variable, len(src.Variables))
		for i, v := range src.Variables {
			dst.Variables[i] = dashboardDomain.Variable{
				Name:    v.Name,
				Type:    v.Type,
				Default: v.Default,
			}
			if v.Options != nil {
				dst.Variables[i].Options = make([]string, len(v.Options))
				copy(dst.Variables[i].Options, v.Options)
			}
		}
	}

	return dst
}

func (s *dashboardService) copyQuery(src dashboardDomain.WidgetQuery) dashboardDomain.WidgetQuery {
	dst := dashboardDomain.WidgetQuery{
		View:     src.View,
		Limit:    src.Limit,
		OrderBy:  src.OrderBy,
		OrderDir: src.OrderDir,
	}

	if src.Measures != nil {
		dst.Measures = make([]string, len(src.Measures))
		copy(dst.Measures, src.Measures)
	}

	if src.Dimensions != nil {
		dst.Dimensions = make([]string, len(src.Dimensions))
		copy(dst.Dimensions, src.Dimensions)
	}

	if src.Filters != nil {
		dst.Filters = make([]dashboardDomain.QueryFilter, len(src.Filters))
		for i, f := range src.Filters {
			dst.Filters[i] = dashboardDomain.QueryFilter{
				Field:    f.Field,
				Operator: f.Operator,
				Value:    f.Value,
			}
		}
	}

	if src.TimeRange != nil {
		dst.TimeRange = &dashboardDomain.TimeRange{
			From:     src.TimeRange.From,
			To:       src.TimeRange.To,
			Relative: src.TimeRange.Relative,
		}
	}

	return dst
}

func (s *dashboardService) copyLayout(src []dashboardDomain.LayoutItem) []dashboardDomain.LayoutItem {
	if src == nil {
		return nil
	}
	dst := make([]dashboardDomain.LayoutItem, len(src))
	for i, item := range src {
		dst[i] = dashboardDomain.LayoutItem{
			WidgetID: item.WidgetID,
			X:        item.X,
			Y:        item.Y,
			W:        item.W,
			H:        item.H,
		}
	}
	return dst
}

func (s *dashboardService) ValidateDashboardConfig(config *dashboardDomain.DashboardConfig) error {
	if config == nil {
		return nil
	}

	widgetIDs := make(map[string]bool)
	for _, widget := range config.Widgets {
		if widget.ID != "" {
			if widgetIDs[widget.ID] {
				return appErrors.NewValidationError("widgets", "duplicate widget ID: "+widget.ID)
			}
			widgetIDs[widget.ID] = true
		}

		if !widget.Type.IsValid() {
			return appErrors.NewValidationError("widgets", "invalid widget type: "+string(widget.Type))
		}

		if err := s.ValidateWidgetQuery(&widget.Query); err != nil {
			return err
		}
	}

	return nil
}

func (s *dashboardService) ValidateWidgetQuery(query *dashboardDomain.WidgetQuery) error {
	if query == nil {
		return appErrors.NewValidationError("query", "widget query is required")
	}

	if !query.View.IsValid() {
		return appErrors.NewValidationError("query.view", "invalid view type: "+string(query.View))
	}

	if len(query.Measures) == 0 {
		return appErrors.NewValidationError("query.measures", "at least one measure is required")
	}

	// TODO: Validate measures and dimensions against the view schema
	// This will be implemented when we add the query builder

	return nil
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
