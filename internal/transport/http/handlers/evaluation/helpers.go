package evaluation

import (
	"github.com/gin-gonic/gin"

	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/ulid"
)

// extractProjectID gets the project ID from SDK auth context (for /v1/* routes)
// or from the URL path parameter (for /api/v1/* routes).
func extractProjectID(c *gin.Context) (ulid.ULID, error) {
	// Try SDK auth context first
	if projectIDPtr, exists := middleware.GetProjectID(c); exists && projectIDPtr != nil {
		return *projectIDPtr, nil
	}

	// Fall back to URL path param for dashboard routes
	projectIDStr := c.Param("projectId")
	if projectIDStr == "" {
		return ulid.ULID{}, appErrors.NewValidationError("Missing project ID", "projectId is required")
	}

	id, err := ulid.Parse(projectIDStr)
	if err != nil {
		return ulid.ULID{}, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID")
	}
	return id, nil
}
