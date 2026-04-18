package prompt

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	promptDomain "brokle/internal/core/domain/prompt"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
)

// ValidateTemplateRequest represents the request body for template validation.
type ValidateTemplateRequest struct {
	Template any                  `json:"template" binding:"required"`
	Type     promptDomain.PromptType      `json:"type" binding:"required" swaggertype:"string"`
	Dialect  promptDomain.TemplateDialect `json:"dialect,omitempty" swaggertype:"string"` // Optional: auto-detect if not specified
}

// ValidateTemplateResponse represents the response for template validation.
type ValidateTemplateResponse struct {
	Valid     bool                         `json:"valid"`
	Dialect   promptDomain.TemplateDialect `json:"dialect" swaggertype:"string"`
	Variables []string                     `json:"variables"`
	Errors    []promptDomain.SyntaxError   `json:"errors,omitempty" swaggertype:"array,object"`
	Warnings  []promptDomain.SyntaxWarning `json:"warnings,omitempty" swaggertype:"array,object"`
}

// PreviewTemplateRequest represents the request body for template preview/compilation.
type PreviewTemplateRequest struct {
	Template  any                  `json:"template" binding:"required"`
	Type      promptDomain.PromptType      `json:"type" binding:"required" swaggertype:"string"`
	Variables map[string]any               `json:"variables" binding:"required" swaggertype:"object"`
	Dialect   promptDomain.TemplateDialect `json:"dialect,omitempty" swaggertype:"string"` // Optional: auto-detect if not specified
}

// PreviewTemplateResponse represents the response for template preview.
type PreviewTemplateResponse struct {
	Compiled any                  `json:"compiled"`
	Dialect  promptDomain.TemplateDialect `json:"dialect" swaggertype:"string"`
}

// ValidateTemplate handles POST /api/v1/projects/:projectId/prompts/validate-template
// @Summary Validate a template
// @Description Validate template syntax and extract variables without saving
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body ValidateTemplateRequest true "Validate template request"
// @Success 200 {object} response.APIResponse{data=ValidateTemplateResponse} "Validation result"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/validate-template [post]
func (h *Handler) ValidateTemplate(c *gin.Context) {
	// Validate project ID exists (for authorization)
	if _, err := uuid.Parse(c.Param("projectId")); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req ValidateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Type != promptDomain.PromptTypeText && req.Type != promptDomain.PromptTypeChat {
		response.Error(c, appErrors.NewValidationError("Invalid type", "type must be 'text' or 'chat'"))
		return
	}

	// Auto-detect dialect if not specified
	dialect := req.Dialect
	if dialect == "" || dialect == promptDomain.DialectAuto {
		detectedDialect, err := h.compilerService.DetectDialect(req.Template, req.Type)
		if err != nil {
			response.Error(c, err)
			return
		}
		dialect = detectedDialect
	}

	result, err := h.compilerService.ValidateSyntax(req.Template, req.Type, dialect)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Extract variables (gracefully degrade to empty list on failure)
	variables, err := h.compilerService.ExtractVariablesWithDialect(req.Template, req.Type, dialect)
	if err != nil {
		variables = []string{}
	}

	resp := ValidateTemplateResponse{
		Valid:     result.Valid,
		Dialect:   result.Dialect,
		Variables: variables,
		Errors:    result.Errors,
		Warnings:  result.Warnings,
	}

	// Ensure slices serialize to [] instead of null
	if resp.Variables == nil {
		resp.Variables = []string{}
	}
	if resp.Errors == nil {
		resp.Errors = []promptDomain.SyntaxError{}
	}
	if resp.Warnings == nil {
		resp.Warnings = []promptDomain.SyntaxWarning{}
	}

	response.Success(c, resp)
}

// PreviewTemplate handles POST /api/v1/projects/:projectId/prompts/preview-template
// @Summary Preview a compiled template
// @Description Compile a template with provided variables without saving
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body PreviewTemplateRequest true "Preview template request"
// @Success 200 {object} response.APIResponse{data=PreviewTemplateResponse} "Compiled template"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 422 {object} response.APIResponse{error=response.APIError} "Compilation failed"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/preview-template [post]
func (h *Handler) PreviewTemplate(c *gin.Context) {
	// Validate project ID exists (for authorization)
	if _, err := uuid.Parse(c.Param("projectId")); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req PreviewTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Type != promptDomain.PromptTypeText && req.Type != promptDomain.PromptTypeChat {
		response.Error(c, appErrors.NewValidationError("Invalid type", "type must be 'text' or 'chat'"))
		return
	}

	// Auto-detect dialect if not specified
	dialect := req.Dialect
	if dialect == "" || dialect == promptDomain.DialectAuto {
		detectedDialect, err := h.compilerService.DetectDialect(req.Template, req.Type)
		if err != nil {
			response.Error(c, err)
			return
		}
		dialect = detectedDialect
	}

	compiled, err := h.compilerService.CompileWithDialect(req.Template, req.Type, req.Variables, dialect)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Wrap compiled output in appropriate template structure to match frontend expectations
	var wrappedCompiled any
	switch req.Type {
	case promptDomain.PromptTypeText:
		content, ok := compiled.(string)
		if !ok {
			response.InternalServerError(c, "unexpected compilation result type for text template")
			return
		}
		wrappedCompiled = promptDomain.TextTemplate{
			Content: content,
		}
	case promptDomain.PromptTypeChat:
		messages, ok := compiled.([]promptDomain.ChatMessage)
		if !ok {
			response.InternalServerError(c, "unexpected compilation result type for chat template")
			return
		}
		wrappedCompiled = promptDomain.ChatTemplate{
			Messages: messages,
		}
	default:
		wrappedCompiled = compiled
	}

	resp := PreviewTemplateResponse{
		Compiled: wrappedCompiled,
		Dialect:  dialect,
	}

	response.Success(c, resp)
}

// DetectDialect handles POST /api/v1/projects/:projectId/prompts/detect-dialect
// @Summary Detect template dialect
// @Description Auto-detect the template dialect from content
// @Tags Prompts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body DetectDialectRequest true "Detect dialect request"
// @Success 200 {object} response.APIResponse{data=DetectDialectResponse} "Detected dialect"
// @Failure 400 {object} response.APIResponse{error=response.APIError} "Invalid request"
// @Failure 401 {object} response.APIResponse{error=response.APIError} "Unauthorized"
// @Failure 500 {object} response.APIResponse{error=response.APIError} "Internal server error"
// @Router /api/v1/projects/{projectId}/prompts/detect-dialect [post]
func (h *Handler) DetectDialect(c *gin.Context) {
	// Validate project ID exists (for authorization)
	if _, err := uuid.Parse(c.Param("projectId")); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))
		return
	}

	var req DetectDialectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.Type != promptDomain.PromptTypeText && req.Type != promptDomain.PromptTypeChat {
		response.Error(c, appErrors.NewValidationError("Invalid type", "type must be 'text' or 'chat'"))
		return
	}

	dialect, err := h.compilerService.DetectDialect(req.Template, req.Type)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := DetectDialectResponse{
		Dialect: dialect,
	}

	response.Success(c, resp)
}

// DetectDialectRequest represents the request body for dialect detection.
type DetectDialectRequest struct {
	Template any             `json:"template" binding:"required"`
	Type     promptDomain.PromptType `json:"type" binding:"required" swaggertype:"string"`
}

// DetectDialectResponse represents the response for dialect detection.
type DetectDialectResponse struct {
	Dialect promptDomain.TemplateDialect `json:"dialect" swaggertype:"string"`
}
