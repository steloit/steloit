// Package playground provides HTTP handlers for the playground feature.
//
// Thin handler layer - all business logic lives in PlaygroundService.
package playground

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/organization"
	playgroundDomain "brokle/internal/core/domain/playground"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
)

// Handler provides HTTP handlers for playground operations.
// Thin layer - delegates all business logic to PlaygroundService.
type Handler struct {
	config            *config.Config
	logger            *slog.Logger
	playgroundService playgroundDomain.PlaygroundService
	projectService    organization.ProjectService // Required for project access validation
}

// NewHandler creates a new playground handler.
func NewHandler(
	config *config.Config,
	logger *slog.Logger,
	playgroundService playgroundDomain.PlaygroundService,
	projectService organization.ProjectService,
) *Handler {
	return &Handler{
		config:            config,
		logger:            logger,
		playgroundService: playgroundService,
		projectService:    projectService,
	}
}

// validateProjectAccess checks if the user has access to the specified project.
// Returns error if project_id is not provided or user lacks access.
// This prevents cross-project credential access attacks.
func (h *Handler) validateProjectAccess(c *gin.Context, projectIDStr *string) error {
	if projectIDStr == nil {
		return appErrors.NewValidationError("project_id is required", "playground execution requires project_id")
	}

	projectID, err := uuid.Parse(*projectIDStr)
	if err != nil {
		return appErrors.NewValidationError("Invalid project_id", "project_id must be a valid UUID")
	}

	userID := middleware.MustGetUserID(c)

	if err := h.projectService.ValidateProjectAccess(c.Request.Context(), userID, projectID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return appErrors.NewNotFoundError("Project not found")
		}
		h.logger.Warn("user attempted to access project credentials without permission",
			"user_id", userID.String(),
			"project_id", projectID.String(),
			"error", err,
		)
		return appErrors.NewForbiddenError("You don't have access to this project")
	}

	return nil
}

// validateSessionAccess checks if the user has access to the session's project.
// Returns error if session_id is provided but user lacks access.
// This prevents cross-project session data tampering attacks.
func (h *Handler) validateSessionAccess(c *gin.Context, sessionIDStr *string) error {
	// If no session ID provided, skip validation (no last_run update will happen)
	if sessionIDStr == nil || h.playgroundService == nil {
		return nil
	}

	sessionID, err := uuid.Parse(*sessionIDStr)
	if err != nil {
		return appErrors.NewValidationError("Invalid session_id", "session_id must be a valid UUID")
	}

	session, err := h.playgroundService.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		return appErrors.NewNotFoundError("Session not found")
	}

	userID := middleware.MustGetUserID(c)

	if err := h.projectService.ValidateProjectAccess(c.Request.Context(), userID, session.ProjectID); err != nil {
		h.logger.Warn("user attempted to access session without project permission",
			"user_id", userID.String(),
			"session_id", sessionID.String(),
			"project_id", session.ProjectID.String(),
		)
		return appErrors.NewForbiddenError("You don't have access to this session")
	}

	return nil
}
