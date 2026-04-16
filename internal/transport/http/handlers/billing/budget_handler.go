package billing

import (
	"log/slog"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"brokle/internal/config"
	"brokle/internal/core/domain/billing"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type BudgetHandler struct {
	config        *config.Config
	logger        *slog.Logger
	budgetService billing.BudgetService
}

func NewBudgetHandler(
	config *config.Config,
	logger *slog.Logger,
	budgetService billing.BudgetService,
) *BudgetHandler {
	return &BudgetHandler{
		config:        config,
		logger:        logger,
		budgetService: budgetService,
	}
}

// CreateBudgetRequest represents the request body for creating a budget
type CreateBudgetRequest struct {
	Name            string   `json:"name" binding:"required,min=1,max=100"`
	ProjectID       *string  `json:"project_id,omitempty"`
	BudgetType      string   `json:"budget_type" binding:"required,oneof=monthly weekly"`
	SpanLimit       *int64   `json:"span_limit,omitempty"`
	BytesLimit      *int64   `json:"bytes_limit,omitempty"`
	ScoreLimit      *int64   `json:"score_limit,omitempty"`
	CostLimit       *float64 `json:"cost_limit,omitempty"`
	AlertThresholds []int64  `json:"alert_thresholds"` // e.g., [50, 80, 100]
}

// UpdateBudgetRequest represents the request body for updating a budget
type UpdateBudgetRequest struct {
	Name            *string  `json:"name,omitempty" binding:"omitempty,min=1,max=100"`
	SpanLimit       *int64   `json:"span_limit,omitempty"`
	BytesLimit      *int64   `json:"bytes_limit,omitempty"`
	ScoreLimit      *int64   `json:"score_limit,omitempty"`
	CostLimit       *float64 `json:"cost_limit,omitempty"`
	AlertThresholds []int64  `json:"alert_thresholds,omitempty"` // e.g., [50, 80, 100]
	IsActive        *bool    `json:"is_active,omitempty"`
}

// ListBudgets handles GET /api/v1/organizations/:orgId/budgets
// @Summary List budgets
// @Description List all budgets for an organization
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Success 200 {object} response.SuccessResponse{data=[]billing.UsageBudget}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/budgets [get]
func (h *BudgetHandler) ListBudgets(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	budgets, err := h.budgetService.GetBudgetsByOrg(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to list budgets",
			"error", err,
			"organization_id", orgID,
		)
		response.Error(c, appErrors.NewInternalError("Failed to list budgets", err))
		return
	}

	response.Success(c, budgets)
}

// GetBudget handles GET /api/v1/organizations/:orgId/budgets/:budgetId
// @Summary Get budget
// @Description Get a specific budget by ID
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param budgetId path string true "Budget ID"
// @Success 200 {object} response.SuccessResponse{data=billing.UsageBudget}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/budgets/{budgetId} [get]
func (h *BudgetHandler) GetBudget(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	budgetID, ok := h.parseBudgetID(c)
	if !ok {
		return
	}

	budget, err := h.budgetService.GetBudget(c.Request.Context(), budgetID)
	if err != nil {
		h.logger.Error("failed to get budget",
			"error", err,
			"budget_id", budgetID,
		)
		response.Error(c, appErrors.NewNotFoundError("Budget not found"))
		return
	}

	// Verify budget belongs to org
	if budget.OrganizationID != orgID {
		response.Error(c, appErrors.NewForbiddenError("Access denied to this budget"))
		return
	}

	response.Success(c, budget)
}

// CreateBudget handles POST /api/v1/organizations/:orgId/budgets
// @Summary Create budget
// @Description Create a new usage budget
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param body body CreateBudgetRequest true "Budget data"
// @Success 201 {object} response.SuccessResponse{data=billing.UsageBudget}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/budgets [post]
func (h *BudgetHandler) CreateBudget(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	var req CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validate at least one limit is set
	if req.SpanLimit == nil && req.BytesLimit == nil && req.ScoreLimit == nil && req.CostLimit == nil {
		response.Error(c, appErrors.NewValidationError("At least one limit is required", "Set span_limit, bytes_limit, score_limit, or cost_limit"))
		return
	}

	// Use provided thresholds; default only if field omitted (nil)
	// Explicitly empty [] means user wants to disable alerts
	alertThresholds := req.AlertThresholds
	if req.AlertThresholds == nil {
		alertThresholds = []int64{50, 80, 100}
	}

	// Sort thresholds ascending for consistent evaluation
	sort.Slice(alertThresholds, func(i, j int) bool {
		return alertThresholds[i] < alertThresholds[j]
	})

	// Convert float64 cost limit to decimal if provided
	var costLimit *decimal.Decimal
	if req.CostLimit != nil {
		d := decimal.NewFromFloat(*req.CostLimit)
		costLimit = &d
	}

	budget := &billing.UsageBudget{
		OrganizationID:  orgID,
		Name:            req.Name,
		BudgetType:      billing.BudgetType(req.BudgetType),
		SpanLimit:       req.SpanLimit,
		BytesLimit:      req.BytesLimit,
		ScoreLimit:      req.ScoreLimit,
		CostLimit:       costLimit,
		AlertThresholds: alertThresholds,
	}

	if req.ProjectID != nil {
		projectID, err := ulid.Parse(*req.ProjectID)
		if err != nil {
			response.Error(c, appErrors.NewValidationError("Invalid project_id", "project_id must be a valid ULID"))
			return
		}
		budget.ProjectID = &projectID
	}

	if err := h.budgetService.CreateBudget(c.Request.Context(), budget); err != nil {
		h.logger.Error("failed to create budget",
			"error", err,
			"organization_id", orgID,
		)
		response.Error(c, err)
		return
	}

	response.Created(c, budget)
}

// UpdateBudget handles PUT /api/v1/organizations/:orgId/budgets/:budgetId
// @Summary Update budget
// @Description Update an existing budget
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param budgetId path string true "Budget ID"
// @Param body body UpdateBudgetRequest true "Budget updates"
// @Success 200 {object} response.SuccessResponse{data=billing.UsageBudget}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/budgets/{budgetId} [put]
func (h *BudgetHandler) UpdateBudget(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	budgetID, ok := h.parseBudgetID(c)
	if !ok {
		return
	}

	budget, err := h.budgetService.GetBudget(c.Request.Context(), budgetID)
	if err != nil {
		response.Error(c, appErrors.NewNotFoundError("Budget not found"))
		return
	}

	if budget.OrganizationID != orgID {
		response.Error(c, appErrors.NewForbiddenError("Access denied to this budget"))
		return
	}

	var req UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Name != nil {
		budget.Name = *req.Name
	}
	if req.SpanLimit != nil {
		budget.SpanLimit = req.SpanLimit
	}
	if req.BytesLimit != nil {
		budget.BytesLimit = req.BytesLimit
	}
	if req.ScoreLimit != nil {
		budget.ScoreLimit = req.ScoreLimit
	}
	if req.CostLimit != nil {
		d := decimal.NewFromFloat(*req.CostLimit)
		budget.CostLimit = &d
	}
	// AlertThresholds: apply if explicitly provided (including empty [] to disable alerts)
	if req.AlertThresholds != nil {
		// Sort thresholds ascending for consistent evaluation
		sort.Slice(req.AlertThresholds, func(i, j int) bool {
			return req.AlertThresholds[i] < req.AlertThresholds[j]
		})
		budget.AlertThresholds = req.AlertThresholds
	}
	if req.IsActive != nil {
		budget.IsActive = *req.IsActive
	}

	if err := h.budgetService.UpdateBudget(c.Request.Context(), budget); err != nil {
		h.logger.Error("failed to update budget",
			"error", err,
			"budget_id", budgetID,
		)
		response.Error(c, appErrors.NewInternalError("Failed to update budget", err))
		return
	}

	response.Success(c, budget)
}

// DeleteBudget handles DELETE /api/v1/organizations/:orgId/budgets/:budgetId
// @Summary Delete budget
// @Description Delete a budget (soft delete)
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param budgetId path string true "Budget ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/budgets/{budgetId} [delete]
func (h *BudgetHandler) DeleteBudget(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	budgetID, ok := h.parseBudgetID(c)
	if !ok {
		return
	}

	// Verify budget exists and belongs to org
	budget, err := h.budgetService.GetBudget(c.Request.Context(), budgetID)
	if err != nil {
		response.Error(c, appErrors.NewNotFoundError("Budget not found"))
		return
	}

	if budget.OrganizationID != orgID {
		response.Error(c, appErrors.NewForbiddenError("Access denied to this budget"))
		return
	}

	if err := h.budgetService.DeleteBudget(c.Request.Context(), budgetID); err != nil {
		h.logger.Error("failed to delete budget",
			"error", err,
			"budget_id", budgetID,
		)
		response.Error(c, appErrors.NewInternalError("Failed to delete budget", err))
		return
	}

	response.NoContent(c)
}

// GetAlerts handles GET /api/v1/organizations/:orgId/budgets/alerts
// @Summary Get budget alerts
// @Description Get recent budget alerts for an organization
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param limit query int false "Maximum number of alerts" default(50)
// @Success 200 {object} response.SuccessResponse{data=[]billing.UsageAlert}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/budgets/alerts [get]
func (h *BudgetHandler) GetAlerts(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := parseInt(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	alerts, err := h.budgetService.GetAlerts(c.Request.Context(), orgID, limit)
	if err != nil {
		h.logger.Error("failed to get alerts",
			"error", err,
			"organization_id", orgID,
		)
		response.Error(c, appErrors.NewInternalError("Failed to get alerts", err))
		return
	}

	response.Success(c, alerts)
}

// AcknowledgeAlert handles POST /api/v1/organizations/:orgId/budgets/alerts/:alertId/acknowledge
// @Summary Acknowledge alert
// @Description Mark a budget alert as acknowledged
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param alertId path string true "Alert ID"
// @Success 200 {object} response.SuccessResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/budgets/alerts/{alertId}/acknowledge [post]
func (h *BudgetHandler) AcknowledgeAlert(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	alertID, err := ulid.Parse(c.Param("alertId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid alert ID", "alertId must be a valid ULID"))
		return
	}

	if err := h.budgetService.AcknowledgeAlert(c.Request.Context(), orgID, alertID); err != nil {
		h.logger.Error("failed to acknowledge alert",
			"error", err,
			"alert_id", alertID,
		)
		// Check if this is a "not found" error (includes unauthorized access)
		if billing.IsNotFoundError(err) {
			response.Error(c, appErrors.NewNotFoundError("Alert not found"))
			return
		}
		response.Error(c, appErrors.NewInternalError("Failed to acknowledge alert", err))
		return
	}

	response.Success(c, gin.H{"acknowledged": true})
}

// Helper methods

func (h *BudgetHandler) parseOrgID(c *gin.Context) (ulid.ULID, bool) {
	orgID, err := ulid.Parse(c.Param("orgId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "orgId must be a valid ULID"))
		return ulid.ULID{}, false
	}
	return orgID, true
}

func (h *BudgetHandler) parseBudgetID(c *gin.Context) (ulid.ULID, bool) {
	budgetID, err := ulid.Parse(c.Param("budgetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid budget ID", "budgetId must be a valid ULID"))
		return ulid.ULID{}, false
	}
	return budgetID, true
}

func (h *BudgetHandler) verifyOrgAccess(c *gin.Context, orgID ulid.ULID) error {
	userOrgID := middleware.ResolveOrganizationID(c)
	if userOrgID == nil || userOrgID.IsZero() {
		return appErrors.NewUnauthorizedError("Organization context required")
	}

	if *userOrgID != orgID {
		return appErrors.NewForbiddenError("Access denied to this organization")
	}

	return nil
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
