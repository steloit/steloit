package response

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appErrors "brokle/pkg/errors"
	"brokle/pkg/pagination"
)

// All API endpoints should use the helpers in this package (Success,
// Error, NoContent, etc.) to return responses wrapped in the standard
// APIResponse envelope.
//
// Intentional exceptions that use c.JSON() directly:
//   - Health endpoints (/health, /health/ready, /health/live):
//     infrastructure endpoints consumed by K8s probes and monitoring
//     tools that expect flat responses.
//   - Billing export JSON (usage_handler.go exportJSON): file download
//     endpoint where Content-Disposition is set and the raw JSON is the
//     downloadable file content.

// APIResponse represents the standard API response format
// @Description Standard API response wrapper
type APIResponse struct {
	Data    any       `json:"data,omitempty" description:"Response data payload"`
	Error   *APIError `json:"error,omitempty" description:"Error information if request failed"`
	Meta    *Meta     `json:"meta,omitempty" description:"Response metadata"`
	Success bool      `json:"success" example:"true" description:"Indicates if the request was successful"`
}

// APIError represents error information in API responses. The shape
// mirrors Stripe / OpenAI / Anthropic — Type is the closed coarse
// classification (clients switch on it for retry / alert / SDK-class
// behaviour), Code is the open fine-grained identifier (per-error SDK
// subclasses, observability), Param optionally points at the input
// field that failed.
//
// @Description Error details for failed API requests
type APIError struct {
	Type    string `json:"type" example:"validation_error" description:"Closed coarse error classification (retry/alert key)"`
	Code    string `json:"code,omitempty" example:"project_not_found" description:"Open fine-grained domain code (snake_case)"`
	Message string `json:"message" example:"Invalid request data" description:"Human-readable error message"`
	Details string `json:"details,omitempty" example:"projectId must be a valid UUID" description:"Additional error context"`
	Param   string `json:"param,omitempty" example:"projectId" description:"Input field that triggered the error, when applicable"`
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
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
		Meta:    getMeta(c),
	})
}

// SuccessWithPagination returns a successful response with offset pagination in meta
func SuccessWithPagination(c *gin.Context, data any, pag *Pagination) {
	meta := getMeta(c)
	meta.Pagination = pag

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// SuccessWithStatus returns a successful response with custom status code
func SuccessWithStatus(c *gin.Context, statusCode int, data any) {
	c.JSON(statusCode, APIResponse{
		Success: true,
		Data:    data,
		Meta:    getMeta(c),
	})
}

// SuccessWithMeta returns a successful response with custom metadata
func SuccessWithMeta(c *gin.Context, data any, meta *Meta) {
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
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, APIResponse{
		Success: true,
		Data:    data,
		Meta:    getMeta(c),
	})
}

// Accepted returns a 202 Accepted response
func Accepted(c *gin.Context, data any) {
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

// Error renders err as the canonical APIResponse error envelope. err
// must be (or wrap) an *AppError; non-AppError errors surface as the
// catch-all TypeAPIError (HTTP 500) so the response stays well-formed
// but the path is treated as a bug — services and handlers should
// always construct a typed AppError before forwarding.
//
// HTTP status is derived from AppError.Type via the canonical
// HTTPStatus() mapping, never from a stored field. This is the
// invariant that eliminates the status / type misclassification
// regression class.
func Error(c *gin.Context, err error) {
	apiError, statusCode := buildAPIError(err)
	c.JSON(statusCode, APIResponse{
		Success: false,
		Error:   apiError,
		Meta:    getMeta(c),
	})
}

// WriteError writes the canonical APIResponse error envelope directly
// to a stdlib http.ResponseWriter. Used by chi-router middleware that
// rejects a request (auth failure, rate limit, panic) before it reaches
// a Huma operation, where there is no *gin.Context in scope.
//
// HTTP status is derived from AppError.Type via the canonical mapping;
// non-AppError errors surface as TypeAPIError (HTTP 500).
//
// Skips encoding the body when status is 204 (RFC 9110 §15.3.5
// requires no body) or when the request is HEAD. Errors from the JSON
// encoder are intentionally swallowed — the response is already
// committed and there is nothing useful to do at that point.
func WriteError(w http.ResponseWriter, err error) {
	apiError, statusCode := buildAPIError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if statusCode == http.StatusNoContent {
		return
	}
	_ = json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   apiError,
	})
}

// buildAPIError renders an arbitrary error into the wire APIError plus
// the HTTP status to write. Centralised so the post-Chi-migration
// stdlib-http handler layer can share it without duplicating the
// Type → Status derivation.
func buildAPIError(err error) (*APIError, int) {
	if appErr := appErrors.AsAppError(err); appErr != nil {
		return &APIError{
			Type:    string(appErr.Type),
			Code:    appErr.CodeOrType(),
			Message: appErr.Message,
			Details: appErr.Details,
			Param:   appErr.Param,
		}, appErr.HTTPStatus()
	}
	return &APIError{
		Type:    string(appErrors.TypeAPIError),
		Code:    string(appErrors.TypeAPIError),
		Message: "Internal server error",
	}, http.StatusInternalServerError
}

// ErrorWithStatus writes a custom-status error response. The Type is
// derived from statusCode via appErrors.FromHTTPStatus so the on-wire
// envelope still carries a semantically correct closed-enum Type, even
// for ad-hoc statuses callers reach for outside the typed AppError
// constructors. code is the open-enum domain code (snake_case).
//
// Used by infrastructure middleware (CSRF, OTLP HTTP receiver, scope
// resolver) where callers know the exact HTTP status they want to
// emit. Handlers should prefer Error(c, appErrors.New*Error(...)).
func ErrorWithStatus(c *gin.Context, statusCode int, code, message, details string) {
	c.JSON(statusCode, APIResponse{
		Success: false,
		Error: &APIError{
			Type:    string(appErrors.FromHTTPStatus(statusCode, message).Type),
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: getMeta(c),
	})
}

// The named-status helpers below are convenience shims for middleware
// and infrastructure code that needs to emit a specific HTTP status
// without first constructing an AppError. Handler code MUST use
// Error(c, appErrors.New*Error(...)) instead — see CLAUDE.md gotcha #4.

// BadRequest returns a 400 Bad Request error
func BadRequest(c *gin.Context, message, details string) {
	ErrorWithStatus(c, http.StatusBadRequest, string(appErrors.TypeInvalidRequest), message, details)
}

// NotFound returns a 404 Not Found error
func NotFound(c *gin.Context, resource string) {
	ErrorWithStatus(c, http.StatusNotFound, string(appErrors.TypeNotFound), resource+" not found", "")
}

// Unauthorized returns a 401 Unauthorized error
func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "Unauthorized access"
	}
	ErrorWithStatus(c, http.StatusUnauthorized, string(appErrors.TypeAuthentication), message, "")
}

// Forbidden returns a 403 Forbidden error
func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = "Access forbidden"
	}
	ErrorWithStatus(c, http.StatusForbidden, string(appErrors.TypePermission), message, "")
}

// Conflict returns a 409 Conflict error
func Conflict(c *gin.Context, message string) {
	ErrorWithStatus(c, http.StatusConflict, string(appErrors.TypeConflict), message, "")
}

// ValidationError returns a 422 Unprocessable Entity error for
// declarative-validation failures.
func ValidationError(c *gin.Context, message, details string) {
	ErrorWithStatus(c, http.StatusUnprocessableEntity, string(appErrors.TypeValidation), message, details)
}

// InternalServerError returns a 500 Internal Server Error
func InternalServerError(c *gin.Context, message string) {
	if message == "" {
		message = "Internal server error"
	}
	ErrorWithStatus(c, http.StatusInternalServerError, string(appErrors.TypeAPIError), message, "")
}

// RateLimit returns a 429 Too Many Requests error
func RateLimit(c *gin.Context, message string) {
	if message == "" {
		message = "Rate limit exceeded"
	}
	ErrorWithStatus(c, http.StatusTooManyRequests, string(appErrors.TypeRateLimit), message, "")
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
	ErrorWithStatus(c, http.StatusPaymentRequired, string(appErrors.TypePaymentRequired), message, "")
}

// QuotaExceeded returns a 429 Too Many Requests error for quota limits.
// Surfaces with the distinct "quota_exceeded" code so SDK consumers can
// render a "you've used your quota" message instead of "slow down".
func QuotaExceeded(c *gin.Context, message string) {
	if message == "" {
		message = "Quota exceeded"
	}
	ErrorWithStatus(c, http.StatusTooManyRequests, appErrors.CodeQuotaExceeded, message, "")
}

// AIProviderError returns a 502 Bad Gateway error for upstream LLM
// provider failures.
func AIProviderError(c *gin.Context, message string) {
	if message == "" {
		message = "AI provider error"
	}
	ErrorWithStatus(c, http.StatusBadGateway, string(appErrors.TypeUpstreamProvider), message, "")
}

// ServiceUnavailable returns a 503 Service Unavailable error
func ServiceUnavailable(c *gin.Context, message string) {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	ErrorWithStatus(c, http.StatusServiceUnavailable, string(appErrors.TypeServiceUnavailable), message, "")
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
