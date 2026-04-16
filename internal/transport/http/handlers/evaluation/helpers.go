package evaluation

import (
	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
)

// extractProjectID gets the project ID from SDK auth context (for /v1/* routes)
// or from the URL path parameter (for /api/v1/* routes).
func extractProjectID(c *gin.Context) (uuid.UUID, error) {
	// Try SDK auth context first
	if projectID, exists := middleware.GetProjectID(c); exists {
		return projectID, nil
	}

	// Fall back to URL path param for dashboard routes
	projectIDStr := c.Param("projectId")
	if projectIDStr == "" {
		return uuid.UUID{}, appErrors.NewValidationError("Missing project ID", "projectId is required")
	}

	id, err := uuid.Parse(projectIDStr)
	if err != nil {
		return uuid.UUID{}, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID")
	}
	return id, nil
}
