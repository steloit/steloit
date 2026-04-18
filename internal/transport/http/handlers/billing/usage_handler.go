package billing

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/analytics"
	"brokle/internal/core/domain/billing"
	"brokle/internal/transport/http/handlers/shared"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/units"

	"github.com/gin-gonic/gin"
)

type UsageHandler struct {
	config       *config.Config
	logger       *slog.Logger
	usageService billing.BillableUsageService
}

func NewUsageHandler(
	config *config.Config,
	logger *slog.Logger,
	usageService billing.BillableUsageService,
) *UsageHandler {
	return &UsageHandler{
		config:       config,
		logger:       logger,
		usageService: usageService,
	}
}

// UsageTimeSeriesRequest represents query params for time series
type UsageTimeSeriesRequest struct {
	TimeRange   string `form:"time_range" binding:"omitempty,oneof=15m 30m 1h 3h 6h 12h 24h 7d 14d 30d"`
	From        string `form:"from" binding:"omitempty"`                           // ISO 8601 (RFC3339) for custom range
	To          string `form:"to" binding:"omitempty"`                             // ISO 8601 (RFC3339) for custom range
	Granularity string `form:"granularity" binding:"omitempty,oneof=hourly daily"` // hourly or daily
}

// GetUsageOverview handles GET /api/v1/organizations/:orgId/usage/overview
// @Summary Get usage overview
// @Description Get current billing period usage overview with 3 dimensions (spans, bytes, scores)
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Success 200 {object} response.SuccessResponse{data=billing.UsageOverview}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/usage/overview [get]
func (h *UsageHandler) GetUsageOverview(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	overview, err := h.usageService.GetUsageOverview(c.Request.Context(), orgID)
	if err != nil {
		response.Error(c, appErrors.NewInternalError("Failed to get usage overview", err))
		return
	}

	response.Success(c, overview)
}

// GetUsageTimeSeries handles GET /api/v1/organizations/:orgId/usage/timeseries
// @Summary Get usage time series
// @Description Get usage data over time for charts
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param time_range query string false "Relative time range preset" default("30d") Enums(15m,30m,1h,3h,6h,12h,24h,7d,14d,30d)
// @Param from query string false "Custom range start (ISO 8601/RFC3339)"
// @Param to query string false "Custom range end (ISO 8601/RFC3339)"
// @Param granularity query string false "Data granularity" Enums(hourly,daily)
// @Success 200 {object} response.SuccessResponse{data=[]billing.BillableUsage}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/usage/timeseries [get]
func (h *UsageHandler) GetUsageTimeSeries(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	var req UsageTimeSeriesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid query parameters", err.Error()))
		return
	}

	from, to, err := shared.ParseTimeRange(req.From, req.To, req.TimeRange, analytics.TimeRange30Days)
	if err != nil {
		response.Error(c, err)
		return
	}

	granularity := req.Granularity
	if granularity == "" {
		if to.Sub(from) > 7*24*time.Hour {
			granularity = "daily"
		} else {
			granularity = "hourly"
		}
	}

	usage, err := h.usageService.GetUsageTimeSeries(c.Request.Context(), orgID, from, to, granularity)
	if err != nil {
		response.Error(c, appErrors.NewInternalError("Failed to get usage time series", err))
		return
	}

	response.Success(c, usage)
}

// GetUsageByProject handles GET /api/v1/organizations/:orgId/usage/by-project
// @Summary Get usage by project
// @Description Get usage breakdown by project
// @Tags Billing
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Param time_range query string false "Relative time range preset" default("30d") Enums(15m,30m,1h,3h,6h,12h,24h,7d,14d,30d)
// @Param from query string false "Custom range start (ISO 8601/RFC3339)"
// @Param to query string false "Custom range end (ISO 8601/RFC3339)"
// @Success 200 {object} response.SuccessResponse{data=[]billing.BillableUsageSummary}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/usage/by-project [get]
func (h *UsageHandler) GetUsageByProject(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	var req UsageTimeSeriesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid query parameters", err.Error()))
		return
	}

	from, to, err := shared.ParseTimeRange(req.From, req.To, req.TimeRange, analytics.TimeRange30Days)
	if err != nil {
		response.Error(c, err)
		return
	}

	summaries, err := h.usageService.GetUsageByProject(c.Request.Context(), orgID, from, to)
	if err != nil {
		response.Error(c, appErrors.NewInternalError("Failed to get usage by project", err))
		return
	}

	response.Success(c, summaries)
}

// ExportUsageRequest represents query params for export
type ExportUsageRequest struct {
	TimeRange   string `form:"time_range" binding:"omitempty,oneof=15m 30m 1h 3h 6h 12h 24h 7d 14d 30d"`
	From        string `form:"from" binding:"omitempty"`                           // ISO 8601 (RFC3339) for custom range
	To          string `form:"to" binding:"omitempty"`                             // ISO 8601 (RFC3339) for custom range
	Format      string `form:"format" binding:"omitempty,oneof=csv json"`          // csv or json
	Granularity string `form:"granularity" binding:"omitempty,oneof=hourly daily"` // hourly or daily
}

// ExportUsage handles GET /api/v1/organizations/:orgId/usage/export
// @Summary Export usage data
// @Description Export usage data as CSV or JSON
// @Tags Billing
// @Accept json
// @Produce application/octet-stream
// @Param orgId path string true "Organization ID"
// @Param time_range query string false "Relative time range preset" default("30d") Enums(15m,30m,1h,3h,6h,12h,24h,7d,14d,30d)
// @Param from query string false "Custom range start (ISO 8601/RFC3339)"
// @Param to query string false "Custom range end (ISO 8601/RFC3339)"
// @Param format query string false "Export format" Enums(csv,json)
// @Param granularity query string false "Data granularity" Enums(hourly,daily)
// @Success 200 {file} file "Export file"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/organizations/{orgId}/usage/export [get]
func (h *UsageHandler) ExportUsage(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	var req ExportUsageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid query parameters", err.Error()))
		return
	}

	from, to, err := shared.ParseTimeRange(req.From, req.To, req.TimeRange, analytics.TimeRange30Days)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := req.Format
	if format == "" {
		format = "csv"
	}

	granularity := req.Granularity
	if granularity == "" {
		if to.Sub(from) > 7*24*time.Hour {
			granularity = "daily"
		} else {
			granularity = "hourly"
		}
	}

	usage, err := h.usageService.GetUsageTimeSeries(c.Request.Context(), orgID, from, to, granularity)
	if err != nil {
		response.Error(c, appErrors.NewInternalError("Failed to get usage for export", err))
		return
	}

	// Get project breakdown for richer export
	projectUsage, err := h.usageService.GetUsageByProject(c.Request.Context(), orgID, from, to)
	if err != nil {
		// Continue without project breakdown
		projectUsage = nil
	}

	switch format {
	case "json":
		h.exportJSON(c, usage, projectUsage, from, to)
	default:
		h.exportCSV(c, usage, projectUsage, from, to)
	}
}

func (h *UsageHandler) exportCSV(c *gin.Context, usage []*billing.BillableUsage, projectUsage []*billing.BillableUsageSummary, from, to time.Time) {
	var buf []byte

	buf = append(buf, "date,organization_id,project_id,spans,bytes_processed,gb_processed,scores,ai_provider_cost\n"...)

	for _, u := range usage {
		line := formatCSVLine(
			u.BucketTime.Format("2006-01-02"),
			u.OrganizationID.String(),
			u.ProjectID.String(),
			formatInt64(u.SpanCount),
			formatInt64(u.BytesProcessed),
			formatFloat64(float64(u.BytesProcessed)/float64(units.BytesPerGB), 4),
			formatInt64(u.ScoreCount),
			u.AIProviderCost.StringFixed(2),
		)
		buf = append(buf, line...)
	}

	filename := "usage_export_" + from.Format("2006-01-02") + "_to_" + to.Format("2006-01-02") + ".csv"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "text/csv")
	c.Data(200, "text/csv", buf)
}

func (h *UsageHandler) exportJSON(c *gin.Context, usage []*billing.BillableUsage, projectUsage []*billing.BillableUsageSummary, from, to time.Time) {
	export := map[string]any{
		"period": map[string]string{
			"from": from.Format(time.RFC3339),
			"to":   to.Format(time.RFC3339),
		},
		"time_series":  usage,
		"by_project":   projectUsage,
		"generated_at": time.Now().Format(time.RFC3339),
	}

	filename := "usage_export_" + from.Format("2006-01-02") + "_to_" + to.Format("2006-01-02") + ".json"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	// Direct c.JSON: file download endpoint with Content-Disposition header.
	// The raw JSON is the downloadable file content — not wrapped in APIResponse.
	c.JSON(200, export)
}

func formatCSVLine(fields ...string) string {
	var result string
	for i, f := range fields {
		if i > 0 {
			result += ","
		}
		result += f
	}
	result += "\n"
	return result
}

func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	if negative {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}

func formatFloat64(f float64, decimals int) string {
	if f == 0 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', decimals, 64)
}

func (h *UsageHandler) parseOrgID(c *gin.Context) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "orgId must be a valid UUID"))
		return uuid.UUID{}, false
	}
	return orgID, true
}

func (h *UsageHandler) verifyOrgAccess(c *gin.Context, orgID uuid.UUID) error {
	userOrgID := middleware.ResolveOrganizationID(c)
	if userOrgID == nil || *userOrgID == uuid.Nil {
		return appErrors.NewUnauthorizedError("Organization context required")
	}

	if *userOrgID != orgID {
		return appErrors.NewForbiddenError("Access denied to this organization")
	}

	return nil
}
