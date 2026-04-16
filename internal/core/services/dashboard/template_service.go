package dashboard

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	dashboardDomain "brokle/internal/core/domain/dashboard"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type templateService struct {
	templateRepo  dashboardDomain.TemplateRepository
	dashboardRepo dashboardDomain.DashboardRepository
	logger        *slog.Logger
}

func NewTemplateService(
	templateRepo dashboardDomain.TemplateRepository,
	dashboardRepo dashboardDomain.DashboardRepository,
	logger *slog.Logger,
) dashboardDomain.TemplateService {
	return &templateService{
		templateRepo:  templateRepo,
		dashboardRepo: dashboardRepo,
		logger:        logger,
	}
}

func (s *templateService) ListTemplates(ctx context.Context) ([]*dashboardDomain.Template, error) {
	templates, err := s.templateRepo.List(ctx, nil)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list templates", err)
	}
	return templates, nil
}

func (s *templateService) GetTemplate(ctx context.Context, id uuid.UUID) (*dashboardDomain.Template, error) {
	template, err := s.templateRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, dashboardDomain.ErrTemplateNotFound) {
			return nil, appErrors.NewNotFoundError("template")
		}
		return nil, appErrors.NewInternalError("failed to get template", err)
	}
	return template, nil
}

func (s *templateService) CreateFromTemplate(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *dashboardDomain.CreateFromTemplateRequest) (*dashboardDomain.Dashboard, error) {
	if req.Name == "" {
		return nil, appErrors.NewValidationError("name", "dashboard name is required")
	}

	template, err := s.templateRepo.GetByID(ctx, req.TemplateID)
	if err != nil {
		if errors.Is(err, dashboardDomain.ErrTemplateNotFound) {
			return nil, appErrors.NewNotFoundError("template")
		}
		return nil, appErrors.NewInternalError("failed to get template", err)
	}

	existing, err := s.dashboardRepo.GetByNameAndProject(ctx, projectID, req.Name)
	if err != nil && !errors.Is(err, dashboardDomain.ErrDashboardNotFound) {
		return nil, appErrors.NewInternalError("failed to check existing dashboard", err)
	}
	if existing != nil {
		return nil, appErrors.NewConflictError("dashboard with this name already exists")
	}

	config := s.copyConfig(template.Config)
	layout := s.copyLayout(template.Layout)

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
		Name:        req.Name,
		Description: template.Description,
		Config:      config,
		Layout:      layout,
		CreatedBy:   userID,
	}

	if err := s.dashboardRepo.Create(ctx, dashboard); err != nil {
		return nil, appErrors.NewInternalError("failed to create dashboard from template", err)
	}

	s.logger.Info("dashboard created from template",
		"dashboard_id", dashboard.ID,
		"project_id", projectID,
		"template_id", template.ID,
		"template_name", template.Name,
	)

	return dashboard, nil
}

func (s *templateService) copyConfig(src dashboardDomain.DashboardConfig) dashboardDomain.DashboardConfig {
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

func (s *templateService) copyQuery(src dashboardDomain.WidgetQuery) dashboardDomain.WidgetQuery {
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

func (s *templateService) copyLayout(src []dashboardDomain.LayoutItem) []dashboardDomain.LayoutItem {
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
