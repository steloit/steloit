package observability

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"brokle/internal/core/domain/observability"
	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// SpanQueryHandler handles SDK span query requests.
// This enables SDK users to query production telemetry using filter expressions.
type SpanQueryHandler struct {
	spanQueryService *obsServices.SpanQueryService
	logger           *slog.Logger
}

// NewSpanQueryHandler creates a new span query handler.
func NewSpanQueryHandler(
	spanQueryService *obsServices.SpanQueryService,
	logger *slog.Logger,
) *SpanQueryHandler {
	return &SpanQueryHandler{
		spanQueryService: spanQueryService,
		logger:           logger.With("handler", "span_query"),
	}
}

// SpanQueryHTTPRequest is the HTTP request body for span queries.
// @Description SDK request for querying spans with filter expressions
type SpanQueryHTTPRequest struct {
	Filter    string     `json:"filter" binding:"required,max=2000" example:"service.name=chatbot AND gen_ai.system=openai"`
	StartTime *time.Time `json:"start_time,omitempty" example:"2024-01-01T00:00:00Z"`
	EndTime   *time.Time `json:"end_time,omitempty" example:"2024-01-31T23:59:59Z"`
	Limit int `json:"limit,omitempty" example:"100"`
	Page  int `json:"page,omitempty" example:"1"`
}

// SpanQueryHTTPResponse is the HTTP response for span queries.
// @Description Response containing queried spans with pagination info
type SpanQueryHTTPResponse struct {
	Spans      []*observability.Span `json:"spans"`
	TotalCount int64                 `json:"total_count"`
	HasMore    bool                  `json:"has_more"`
}

// ValidateFilterRequest is the HTTP request body for filter validation.
// @Description Request to validate a filter expression
type ValidateFilterRequest struct {
	Filter string `json:"filter" binding:"required,max=2000" example:"service.name=chatbot"`
}

// HandleQuery handles POST /v1/spans/query
// @Summary Query spans using filter expressions
// @Description Query production telemetry data using human-readable filter syntax.
// @Description Supports operators: =, !=, >, <, >=, <=, CONTAINS, IN, EXISTS
// @Description Supports logical operators: AND, OR with parentheses grouping
// @Tags SDK - Span Query
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body SpanQueryHTTPRequest true "Span query request"
// @Success 200 {object} response.APIResponse{data=SpanQueryHTTPResponse} "Spans matching filter"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid filter syntax"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Invalid or missing API key"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /v1/spans/query [post]
func (h *SpanQueryHandler) HandleQuery(c *gin.Context) {
	ctx := c.Request.Context()

	projectIDPtr, exists := middleware.GetProjectID(c)
	if !exists || projectIDPtr == nil {
		h.logger.Error("Project ID not found in context")
		response.Unauthorized(c, "Authentication required")
		return
	}
	projectID := projectIDPtr.String()

	var req SpanQueryHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Set defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}

	domainReq := &observability.SpanQueryRequest{
		Filter:    req.Filter,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Limit:     limit,
		Page:      page,
	}

	result, err := h.spanQueryService.QuerySpans(ctx, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, SpanQueryHTTPResponse{
		Spans:      result.Spans,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	})
}

// HandleValidate handles POST /v1/spans/query/validate
// @Summary Validate a filter expression
// @Description Validates filter syntax without executing the query. Useful for SDK clients
// @Description to validate filters before submitting queries.
// @Tags SDK - Span Query
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body ValidateFilterRequest true "Filter validation request"
// @Success 200 {object} response.APIResponse{data=map[string]interface{}} "Filter is valid"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid filter syntax"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Invalid or missing API key"
// @Router /v1/spans/query/validate [post]
func (h *SpanQueryHandler) HandleValidate(c *gin.Context) {
	_, exists := middleware.GetProjectID(c)
	if !exists {
		response.Unauthorized(c, "Authentication required")
		return
	}

	var req ValidateFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if err := h.spanQueryService.ValidateFilter(req.Filter); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, map[string]interface{}{
		"valid":   true,
		"message": "Filter expression is valid",
	})
}
