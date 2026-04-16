package dashboard

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/config"
	dashboardDomain "brokle/internal/core/domain/dashboard"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// TemplateHandler handles dashboard template HTTP requests
type TemplateHandler struct {
	config  *config.Config
	logger  *slog.Logger
	service dashboardDomain.TemplateService
}

// NewTemplateHandler creates a new template handler instance
func NewTemplateHandler(
	cfg *config.Config,
	logger *slog.Logger,
	service dashboardDomain.TemplateService,
) *TemplateHandler {
	return &TemplateHandler{
		config:  cfg,
		logger:  logger,
		service: service,
	}
}

// ListTemplates handles GET /api/v1/dashboard-templates
// @Summary List all active dashboard templates
// @Description Retrieve all active dashboard templates
// @Tags Dashboard Templates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=[]dashboard.Template} "List of templates"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/dashboard-templates [get]
func (h *TemplateHandler) ListTemplates(c *gin.Context) {
	templates, err := h.service.ListTemplates(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, templates)
}

// GetTemplate handles GET /api/v1/dashboard-templates/:templateId
// @Summary Get template by ID
// @Description Retrieve dashboard template details
// @Tags Dashboard Templates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param templateId path string true "Template ID"
// @Success 200 {object} response.APIResponse{data=dashboard.Template} "Template details"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Template not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/dashboard-templates/{templateId} [get]
func (h *TemplateHandler) GetTemplate(c *gin.Context) {
	templateID, err := uuid.Parse(c.Param("templateId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid template ID", "templateId must be a valid UUID"))
		return
	}

	template, err := h.service.GetTemplate(c.Request.Context(), templateID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, template)
}

// CreateFromTemplate handles POST /api/v1/projects/:projectId/dashboards/from-template
// @Summary Create dashboard from template
// @Description Create a new dashboard from an existing template
// @Tags Dashboard Templates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body dashboard.CreateFromTemplateRequest true "Create from template request"
// @Success 201 {object} response.APIResponse{data=dashboard.Dashboard} "Created dashboard"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Template not found"
// @Failure 409 {object} response.APIResponse{error=response.APIError} "Dashboard name already exists"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/dashboards/from-template [post]
func (h *TemplateHandler) CreateFromTemplate(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req dashboardDomain.CreateFromTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var userID *uuid.UUID
	if uid, ok := middleware.GetUserIDFromContext(c); ok {
		userID = &uid
	}

	dashboard, err := h.service.CreateFromTemplate(c.Request.Context(), projectID, userID, &req)
	if err != nil {
		h.logger.Error("failed to create dashboard from template",
			"template_id", req.TemplateID,
			"project_id", projectID,
			"error", err,
		)
		response.Error(c, err)
		return
	}

	h.logger.Info("dashboard created from template",
		"dashboard_id", dashboard.ID,
		"template_id", req.TemplateID,
		"project_id", projectID,
	)

	response.Created(c, dashboard)
}
