package response

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appErrors "brokle/pkg/errors"
	"brokle/pkg/pagination"
)

// All API endpoints should use the helpers in this package (Success, Error, NoContent, etc.)
// to return responses wrapped in the standard APIResponse envelope.
//
// Intentional exceptions that use c.JSON() directly:
//   - Health endpoints (/health, /health/ready, /health/live): infrastructure endpoints
//     consumed by K8s probes and monitoring tools that expect flat responses.
//   - Billing export JSON (usage_handler.go exportJSON): file download endpoint where
//     Content-Disposition is set and the raw JSON is the downloadable file content.

// APIResponse represents the standard API response format
// @Description Standard API response wrapper
type APIResponse struct {
	Data    interface{} `json:"data,omitempty" description:"Response data payload"`
	Error   *APIError   `json:"error,omitempty" description:"Error information if request failed"`
	Meta    *Meta       `json:"meta,omitempty" description:"Response metadata"`
	Success bool        `json:"success" example:"true" description:"Indicates if the request was successful"`
}

// APIError represents error information in API responses
// @Description Error details for failed API requests
type APIError struct {
	Code    string `json:"code" example:"validation_error" description:"Error code identifier"`
	Message string `json:"message" example:"Invalid request data" description:"Human-readable error message"`
	Details string `json:"details,omitempty" example:"Field 'email' is required" description:"Additional error details"`
	Type    string `json:"type,omitempty" example:"validation_error" description:"Error type category"`
}

// Pagination represents offset-based pagination metadata
// @Description Offset-based pagination information for list responses
type Pagination struct {
	Page       int   `json:"page" example:"1" description:"Current page number (1-indexed)"`
	Limit      int   `json:"limit" example:"50" description:"Items per page (10, 25, 50, 100)"`
	Total      int64 `json:"total" example:"1234" description:"Total number of items"`
	TotalPages int   `json:"total_pages" example:"25" description:"Total number of pages"`
	HasNext    bool  `json:"has_next" example:"true" description:"Whether there are more pages"`
	HasPrev    bool  `json:"has_prev" example:"false" description:"Whether there are previous pages"`
}

// Meta contains metadata about the API response
// @Description Response metadata including request tracking and offset pagination
type Meta struct {
	Pagination *Pagination `json:"pagination,omitempty" description:"Offset pagination information for list responses"`
	RequestID  string      `json:"request_id,omitempty" example:"req_01h2x3y4z5" description:"Unique request identifier"`
	Timestamp  string      `json:"timestamp,omitempty" example:"2023-12-01T10:30:00Z" description:"Response timestamp in ISO 8601 format"`
	Version    string      `json:"version,omitempty" example:"v1" description:"API version"`
}

// Success returns a successful response with data
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
		Meta:    getMeta(c),
	})
}

// SuccessWithPagination returns a successful response with offset pagination in meta
func SuccessWithPagination(c *gin.Context, data interface{}, pag *Pagination) {
	meta := getMeta(c)
	meta.Pagination = pag

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// SuccessWithStatus returns a successful response with custom status code
func SuccessWithStatus(c *gin.Context, statusCode int, data interface{}) {
	c.JSON(statusCode, APIResponse{
		Success: true,
		Data:    data,
		Meta:    getMeta(c),
	})
}

// SuccessWithMeta returns a successful response with custom metadata
func SuccessWithMeta(c *gin.Context, data interface{}, meta *Meta) {
	if meta == nil {
		meta = getMeta(c)
	} else {
		defaultMeta := getMeta(c)
		if meta.RequestID == "" {
			meta.RequestID = defaultMeta.RequestID
		}
		if meta.Timestamp == "" {
			meta.Timestamp = defaultMeta.Timestamp
		}
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// Created returns a 201 Created response
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{
		Success: true,
		Data:    data,
		Meta:    getMeta(c),
	})
}

// Accepted returns a 202 Accepted response
func Accepted(c *gin.Context, data interface{}) {
	c.JSON(http.StatusAccepted, APIResponse{
		Success: true,
		Data:    data,
		Meta:    getMeta(c),
	})
}

// NoContent returns a 204 No Content response
// RFC 7231 Section 6.3.5: 204 responses MUST NOT include a message body
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Error returns an error response based on AppError type
func Error(c *gin.Context, err error) {
	var statusCode int
	var apiError *APIError

	if appErr, ok := appErrors.IsAppError(err); ok {
		statusCode = appErr.StatusCode
		apiError = &APIError{
			Code:    string(appErr.Type),
			Message: appErr.Message,
			Details: appErr.Details,
			Type:    string(appErr.Type),
		}
	} else {
		statusCode = http.StatusInternalServerError
		apiError = &APIError{
			Code:    string(appErrors.InternalError),
			Message: "Internal server error",
			Details: "",
			Type:    string(appErrors.InternalError),
		}
	}

	c.JSON(statusCode, APIResponse{
		Success: false,
		Error:   apiError,
		Meta:    getMeta(c),
	})
}

// ErrorWithStatus returns an error response with custom status code
func ErrorWithStatus(c *gin.Context, statusCode int, code, message, details string) {
	c.JSON(statusCode, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: getMeta(c),
	})
}

// BadRequest returns a 400 Bad Request error
func BadRequest(c *gin.Context, message, details string) {
	ErrorWithStatus(c, http.StatusBadRequest, string(appErrors.BadRequestError), message, details)
}

// NotFound returns a 404 Not Found error
func NotFound(c *gin.Context, resource string) {
	ErrorWithStatus(c, http.StatusNotFound, string(appErrors.NotFoundError), resource+" not found", "")
}

// Unauthorized returns a 401 Unauthorized error
func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "Unauthorized access"
	}
	ErrorWithStatus(c, http.StatusUnauthorized, string(appErrors.UnauthorizedError), message, "")
}

// Forbidden returns a 403 Forbidden error
func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = "Access forbidden"
	}
	ErrorWithStatus(c, http.StatusForbidden, string(appErrors.ForbiddenError), message, "")
}

// Conflict returns a 409 Conflict error
func Conflict(c *gin.Context, message string) {
	ErrorWithStatus(c, http.StatusConflict, string(appErrors.ConflictError), message, "")
}

// ValidationError returns a 400 Bad Request error for validation failures
func ValidationError(c *gin.Context, message, details string) {
	ErrorWithStatus(c, http.StatusBadRequest, string(appErrors.ValidationError), message, details)
}

// InternalServerError returns a 500 Internal Server Error
func InternalServerError(c *gin.Context, message string) {
	if message == "" {
		message = "Internal server error"
	}
	ErrorWithStatus(c, http.StatusInternalServerError, string(appErrors.InternalError), message, "")
}

// RateLimit returns a 429 Too Many Requests error
func RateLimit(c *gin.Context, message string) {
	if message == "" {
		message = "Rate limit exceeded"
	}
	ErrorWithStatus(c, http.StatusTooManyRequests, string(appErrors.RateLimitError), message, "")
}

// TooManyRequests is an alias for RateLimit for better readability
func TooManyRequests(c *gin.Context, message string) {
	RateLimit(c, message)
}

// PaymentRequired returns a 402 Payment Required error
func PaymentRequired(c *gin.Context, message string) {
	if message == "" {
		message = "Payment required"
	}
	ErrorWithStatus(c, http.StatusPaymentRequired, string(appErrors.PaymentRequiredError), message, "")
}

// QuotaExceeded returns a 429 Too Many Requests error for quota limits
func QuotaExceeded(c *gin.Context, message string) {
	if message == "" {
		message = "Quota exceeded"
	}
	ErrorWithStatus(c, http.StatusTooManyRequests, string(appErrors.QuotaExceededError), message, "")
}

// AIProviderError returns a 502 Bad Gateway error for AI provider failures
func AIProviderError(c *gin.Context, message string) {
	if message == "" {
		message = "AI provider error"
	}
	ErrorWithStatus(c, http.StatusBadGateway, string(appErrors.AIProviderError), message, "")
}

// ServiceUnavailable returns a 503 Service Unavailable error
func ServiceUnavailable(c *gin.Context, message string) {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	ErrorWithStatus(c, http.StatusServiceUnavailable, string(appErrors.ServiceUnavailable), message, "")
}

// ErrorWithCode creates an error response using predefined error codes
func ErrorWithCode(c *gin.Context, statusCode int, code string, details string) {
	appErr := appErrors.NewErrorWithCode(code, details)
	c.JSON(statusCode, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: appErr.Message,
			Details: details,
			Type:    string(appErr.Type),
		},
		Meta: getMeta(c),
	})
}

// NewPagination creates offset pagination metadata
func NewPagination(page, limit int, total int64) *Pagination {
	// Validate limit (10, 25, 50, 100)
	if !pagination.IsValidPageSize(limit) {
		limit = pagination.DefaultPageSize // default 50
	}

	// Calculate total pages
	totalPages := pagination.CalculateTotalPages(total, limit)

	// Determine has_next and has_prev
	hasNext := page < totalPages
	hasPrev := page > 1

	return &Pagination{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    hasNext,
		HasPrev:    hasPrev,
	}
}

// ParsePaginationParams parses offset pagination parameters from query strings
func ParsePaginationParams(page, limit, sortBy, sortDir string) pagination.Params {
	params := pagination.Params{
		Page:    1,  // default page 1
		Limit:   50, // default limit 50
		SortBy:  "", // empty = repository will use domain-specific default
		SortDir: "desc",
	}

	// Parse page number
	if page != "" {
		if p, err := strconv.Atoi(page); err == nil && p >= 1 {
			params.Page = p
		}
	}

	// Parse limit (10, 25, 50, 100)
	if limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			if pagination.IsValidPageSize(l) {
				params.Limit = l
			}
		}
	}

	// Parse sort by
	if sortBy != "" {
		params.SortBy = sortBy
	}

	// Parse sort direction
	if sortDir == "asc" || sortDir == "desc" {
		params.SortDir = sortDir
	}

	// Validate and clamp to safe values
	if err := params.Validate(); err != nil {
		// If offset exceeds max, clamp to last safe page
		if params.GetOffset() > pagination.MaxOffset {
			params.Page = pagination.MaxOffset / params.Limit
		}
		// If page is invalid, reset to 1
		if params.Page < 1 {
			params.Page = 1
		}
	}

	return params
}

// getMeta creates standard metadata for responses
func getMeta(c *gin.Context) *Meta {
	meta := &Meta{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   "v1",
	}

	if requestID, exists := c.Get("request_id"); exists {
		if id, ok := requestID.(string); ok {
			meta.RequestID = id
		}
	}

	if timestamp, exists := c.Get("timestamp"); exists {
		if ts, ok := timestamp.(string); ok {
			meta.Timestamp = ts
		}
	}

	return meta
}
