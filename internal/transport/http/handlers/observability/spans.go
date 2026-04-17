package observability

import (
	"github.com/gin-gonic/gin"

	"brokle/internal/core/domain/observability"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// ListSpans handles GET /api/v1/spans
// @Summary List spans with filtering
// @Description Retrieve paginated list of spans
// @Tags Spans
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param trace_id query string false "Filter by trace ID"
// @Param type query string false "Filter by span type"
// @Param model query string false "Filter by model"
// @Param level query string false "Filter by level"
// @Param limit query int false "Limit (default 50, max 1000)"
// @Param offset query int false "Offset (default 0)"
// @Success 200 {object} response.APIResponse{data=[]observability.Span} "List of spans"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid parameters"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/spans [get]
func (h *Handler) ListSpans(c *gin.Context) {
	// Project scoping + membership check is enforced by RequireProjectAccess
	// middleware; SpanFilter.ProjectID is required so repository queries are
	// tenant-bounded.
	filter := &observability.SpanFilter{
		ProjectID: middleware.MustGetProjectID(c).String(),
	}

	if traceID := c.Query("trace_id"); traceID != "" {
		filter.TraceID = &traceID
	}
	if obsType := c.Query("type"); obsType != "" {
		filter.Type = &obsType
	}
	if model := c.Query("model"); model != "" {
		filter.Model = &model
	}
	if level := c.Query("level"); level != "" {
		filter.Level = &level
	}

	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)
	filter.Params = params

	spans, err := h.services.GetTraceService().GetSpansByFilter(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to list spans", "error", err)
		response.Error(c, err)
		return
	}

	totalCount, err := h.services.GetTraceService().CountSpans(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to count spans", "error", err)
		response.Error(c, err)
		return
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, totalCount)

	response.SuccessWithPagination(c, spans, paginationMeta)
}

// GetSpan handles GET /api/v1/spans/:id
// @Summary Get span by ID
// @Description Retrieve detailed span information
// @Tags Spans
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Span ID (OTEL 16-character hex)"
// @Success 200 {object} response.APIResponse{data=observability.Span} "Span details"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Span not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/spans/{id} [get]
func (h *Handler) GetSpan(c *gin.Context) {
	spanID := c.Param("id")
	if spanID == "" {
		response.Error(c, appErrors.NewValidationError("Missing span ID", "id is required"))
		return
	}

	span, err := h.services.GetTraceService().GetSpan(c.Request.Context(), spanID)
	if err != nil {
		h.logger.Error("Failed to get span", "error", err)
		response.Error(c, err)
		return
	}

	response.Success(c, span)
}

// DeleteSpan handles DELETE /api/v1/spans/:id
// @Summary Delete a span
// @Description Delete a span by its OTEL span_id
// @Tags Spans
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Span ID (OTEL 16-character hex)"
// @Success 204 "No Content"
// @Failure 404 {object} response.APIResponse{error=response.APIError} "Span not found"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/spans/{id} [delete]
func (h *Handler) DeleteSpan(c *gin.Context) {
	spanID := c.Param("id")
	if spanID == "" {
		response.Error(c, appErrors.NewValidationError("Missing span ID", "id is required"))
		return
	}

	if err := h.services.GetTraceService().DeleteSpan(c.Request.Context(), spanID); err != nil {
		h.logger.Error("Failed to delete span", "error", err)
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}
