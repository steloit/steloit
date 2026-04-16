package evaluation

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type ExperimentItemHandler struct {
	logger  *slog.Logger
	service evaluationDomain.ExperimentItemService
}

func NewExperimentItemHandler(
	logger *slog.Logger,
	service evaluationDomain.ExperimentItemService,
) *ExperimentItemHandler {
	return &ExperimentItemHandler{
		logger:  logger,
		service: service,
	}
}

// @Summary List experiment items
// @Description Returns items for an experiment with pagination.
// @Tags Experiment Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param experimentId path string true "Experiment ID"
// @Param limit query int false "Limit (default 50, max 100)"
// @Param offset query int false "Offset (default 0)"
// @Success 200 {object} ExperimentItemListResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/experiments/{experimentId}/items [get]
func (h *ExperimentItemHandler) List(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	experimentID, err := ulid.Parse(c.Param("experimentId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid experiment ID", "experimentId must be a valid ULID"))
		return
	}

	limit := 50
	offset := 0

	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	items, total, err := h.service.List(c.Request.Context(), experimentID, projectID, limit, offset)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*ExperimentItemResponse, len(items))
	for i, item := range items {
		domainResp := item.ToResponse()
		responses[i] = &ExperimentItemResponse{
			ID:            domainResp.ID,
			ExperimentID:  domainResp.ExperimentID,
			DatasetItemID: domainResp.DatasetItemID,
			TraceID:       domainResp.TraceID,
			Input:         domainResp.Input,
			Output:        domainResp.Output,
			Expected:      domainResp.Expected,
			TrialNumber:   domainResp.TrialNumber,
			Metadata:      domainResp.Metadata,
			CreatedAt:     domainResp.CreatedAt,
		}
	}

	response.Success(c, &ExperimentItemListResponse{
		Items: responses,
		Total: total,
	})
}
