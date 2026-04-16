package apikey

import (
	"log/slog"
	"time"

	"brokle/internal/config"
	"brokle/internal/core/domain/auth"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	config        *config.Config
	logger        *slog.Logger
	apiKeyService auth.APIKeyService
}

func NewHandler(
	config *config.Config,
	logger *slog.Logger,
	apiKeyService auth.APIKeyService,
) *Handler {
	return &Handler{
		config:        config,
		logger:        logger,
		apiKeyService: apiKeyService,
	}
}

// Request/Response Models

// APIKey represents an API key entity for response
type APIKey struct {
	ID         string     `json:"id" example:"key_01234567890123456789012345" description:"Unique API key identifier"`
	Name       string     `json:"name" example:"Production API Key" description:"Human-readable name for the API key"`
	Key        string     `json:"key,omitempty" example:"bk_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCd" description:"The actual API key (only shown on creation)"`
	KeyPreview string     `json:"key_preview" example:"bk_AbCd...AbCd" description:"Truncated version of the key for display"`
	ProjectID  string     `json:"project_id" example:"proj_01234567890123456789012345" description:"Project ID this key belongs to"`
	Status     string     `json:"status" example:"active" description:"API key status (active, expired)"`
	LastUsed   *time.Time `json:"last_used,omitempty" example:"2024-01-01T00:00:00Z" description:"Last time this key was used (null if never used)"`
	CreatedAt  time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z" description:"Creation timestamp"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" example:"2024-12-31T23:59:59Z" description:"Expiration timestamp (null if never expires)"`
	CreatedBy  string     `json:"created_by" example:"usr_01234567890123456789012345" description:"User ID who created this key"`
}

// CreateAPIKeyRequest represents the request to create an API key
type CreateAPIKeyRequest struct {
	Name         string `json:"name" binding:"required,min=2,max=100" example:"Production API Key" description:"Human-readable name for the API key (2-100 characters)"`
	ExpiryOption string `json:"expiry_option" binding:"required,oneof=30days 90days never" example:"90days" description:"Expiration option: '30days', '90days', or 'never'"`
}

// ListAPIKeysResponse represents the response when listing API keys
// NOTE: This struct is not used. When implementing, use response.SuccessWithPagination()
// with []APIKey directly and response.NewPagination() for consistent pagination format.
type ListAPIKeysResponse struct {
	APIKeys []APIKey `json:"api_keys" description:"List of API keys"`
	// Pagination fields removed - use response.SuccessWithPagination() instead
}

// List handles GET /api/v1/projects/:projectId/api-keys
// @Summary List API keys
// @Description Get a paginated list of API keys for a specific project. Keys are shown with preview format (bk_xxxx...yyyy) for security.
// @Tags API Keys
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_01234567890123456789012345")
// @Param status query string false "Filter by API key status" Enums(active,expired)
// @Param cursor query string false "Pagination cursor" example("eyJjcmVhdGVkX2F0IjoiMjAyNC0wMS0wMVQxMjowMDowMFoiLCJpZCI6IjAxSDJYM1k0WjUifQ==")
// @Param page_size query int false "Items per page" Enums(10,20,30,40,50) default(50)
// @Param sort_by query string false "Sort field" Enums(created_at,name,last_used_at) default("created_at")
// @Param sort_dir query string false "Sort direction" Enums(asc,desc) default("desc")
// @Success 200 {object} response.APIResponse{data=[]APIKey,meta=response.Meta{pagination=response.Pagination}} "List of project-scoped API keys with cursor pagination"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid project ID"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions to view API keys"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId}/api-keys [get]
func (h *Handler) List(c *gin.Context) {
	// Get project ID from URL path
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	// Get authenticated user from context
	userID := middleware.MustGetUserID(c)

	// Parse offset pagination parameters
	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)

	status := c.Query("status")

	// Create filters with project ID
	filters := &auth.APIKeyFilters{
		ProjectID: &projectID,
	}
	// Set embedded pagination fields
	filters.Params = params

	// Note: Environment filtering removed - environments are handled via SDK headers/tags

	// Filter by status if provided
	if status != "" {
		if status == "active" {
			isExpired := false
			filters.IsExpired = &isExpired
		} else if status == "expired" {
			isExpired := true
			filters.IsExpired = &isExpired
		}
	}

	// Get API keys
	apiKeys, err := h.apiKeyService.GetAPIKeys(c.Request.Context(), filters)
	if err != nil {
		h.logger.Error("Failed to list API keys", "error", err)
		response.Error(c, err)
		return
	}

	// Convert to response format
	responseKeys := make([]APIKey, len(apiKeys))
	for i, key := range apiKeys {
		responseKeys[i] = APIKey{
			ID:         key.ID.String(),
			Name:       key.Name,
			KeyPreview: key.KeyPreview, // Use stored preview
			ProjectID:  key.ProjectID.String(),
			Status:     getKeyStatus(*key),
			LastUsed:   key.LastUsedAt, // Pointer, will be null if nil
			CreatedAt:  key.CreatedAt,
			ExpiresAt:  key.ExpiresAt, // Pointer, will be null if nil
			CreatedBy:  key.UserID.String(),
		}
	}

	// Get total count for accurate pagination metadata
	total, err := h.apiKeyService.CountAPIKeys(c.Request.Context(), filters)
	if err != nil {
		h.logger.Error("Failed to count API keys", "error", err)
		response.Error(c, err)
		return
	}

	// Create offset pagination
	pag := response.NewPagination(params.Page, params.Limit, total)

	h.logger.Debug("Listed API keys",
		"user_id", userID,
		"project_id", projectID,
		"count", len(responseKeys),
	)

	response.SuccessWithPagination(c, responseKeys, pag)
}

// getKeyStatus determines the status of an API key
func getKeyStatus(key auth.APIKey) string {
	// Deleted keys are filtered by GORM soft delete
	// Only check if expired
	if key.IsExpired() {
		return "expired"
	}
	return "active"
}

// Create handles POST /api/v1/projects/:projectId/api-keys
// @Summary Create API key
// @Description Create a new industry-standard API key for the project. The full key will only be displayed once upon creation. Format: bk_{40_char_random}
// @Tags API Keys
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_01234567890123456789012345")
// @Param request body CreateAPIKeyRequest true "API key details"
// @Success 201 {object} response.SuccessResponse{data=APIKey} "Project-scoped API key created successfully (full key only shown once)"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid input or validation errors"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions to create API keys"
// @Failure 404 {object} response.ErrorResponse "Project not found"
// @Failure 409 {object} response.ErrorResponse "Conflict - API key name already exists in project"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId}/api-keys [post]
func (h *Handler) Create(c *gin.Context) {
	// Get project ID from URL path
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	// Parse request body
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Get authenticated user from context
	userID := middleware.MustGetUserID(c)

	// Convert expiry option to timestamp
	var expiresAt *time.Time
	switch req.ExpiryOption {
	case "30days":
		t := time.Now().Add(30 * 24 * time.Hour)
		expiresAt = &t
	case "90days":
		t := time.Now().Add(90 * 24 * time.Hour)
		expiresAt = &t
	case "never":
		expiresAt = nil
	}

	// Create service request
	serviceReq := &auth.CreateAPIKeyRequest{
		Name:      req.Name,
		ProjectID: projectID,
		ExpiresAt: expiresAt,
	}

	// Create the API key
	apiKeyResp, err := h.apiKeyService.CreateAPIKey(c.Request.Context(), userID, serviceReq)
	if err != nil {
		h.logger.Error("Failed to create API key", "error", err)
		response.Error(c, err)
		return
	}

	// Convert to response format
	responseKey := APIKey{
		ID:         apiKeyResp.ID,
		Name:       apiKeyResp.Name,
		Key:        apiKeyResp.Key, // Only shown once
		KeyPreview: apiKeyResp.KeyPreview,
		ProjectID:  apiKeyResp.ProjectID,
		Status:     "active",
		LastUsed:   nil, // New keys have never been used
		CreatedAt:  apiKeyResp.CreatedAt,
		ExpiresAt:  apiKeyResp.ExpiresAt, // Pointer, will be null if nil
		CreatedBy:  userID.String(),
	}

	h.logger.Info("API key created successfully",
		"user_id", userID,
		"api_key_id", apiKeyResp.ID,
		"project_id", projectID,
		"key_name", req.Name,
	)

	response.Created(c, responseKey)
}

// Delete handles DELETE /api/v1/projects/:projectId/api-keys/:keyId
// @Summary Delete project-scoped API key
// @Description Permanently revoke and delete a project-scoped API key. This action cannot be undone and will immediately invalidate the key across all environments.
// @Tags API Keys
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" example("proj_01234567890123456789012345")
// @Param keyId path string true "API Key ID" example("key_01234567890123456789012345")
// @Success 204 "Project-scoped API key deleted successfully"
// @Failure 400 {object} response.ErrorResponse "Bad request - invalid project ID or key ID"
// @Failure 401 {object} response.ErrorResponse "Unauthorized"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions to delete API keys"
// @Failure 404 {object} response.ErrorResponse "Project or API key not found"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/projects/{projectId}/api-keys/{keyId} [delete]
func (h *Handler) Delete(c *gin.Context) {
	// Get project ID from URL path
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	// Get API key ID from URL path
	keyID, err := uuid.Parse(c.Param("keyId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid API key ID", "keyId must be a valid UUID"))
		return
	}

	// Get authenticated user from context
	userID := middleware.MustGetUserID(c)

	// Delete the API key (service validates existence and project ownership)
	if err := h.apiKeyService.DeleteAPIKey(c.Request.Context(), keyID, projectID); err != nil {
		h.logger.Error("Failed to delete API key", "error", err)
		response.Error(c, err)
		return
	}

	h.logger.Info("API key deleted successfully",
		"user_id", userID,
		"api_key_id", keyID,
		"project_id", projectID,
	)

	response.NoContent(c)
}
