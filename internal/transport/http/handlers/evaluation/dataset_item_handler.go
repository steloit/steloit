package evaluation

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type DatasetItemHandler struct {
	logger  *slog.Logger
	service evaluationDomain.DatasetItemService
}

func NewDatasetItemHandler(
	logger *slog.Logger,
	service evaluationDomain.DatasetItemService,
) *DatasetItemHandler {
	return &DatasetItemHandler{
		logger:  logger,
		service: service,
	}
}

// @Summary List dataset items
// @Description Returns items for a dataset with pagination.
// @Tags Dataset Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page" Enums(10,25,50,100) default(50)
// @Success 200 {object} response.APIResponse{data=[]DatasetItemResponse,meta=response.Meta{pagination=response.Pagination}}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items [get]
func (h *DatasetItemHandler) List(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	// Parse pagination params using standard helper
	params := response.ParsePaginationParams(c.Query("page"), c.Query("limit"), "", "")

	// Convert page to offset for repository
	offset := (params.Page - 1) * params.Limit

	items, total, err := h.service.List(c.Request.Context(), datasetID, projectID, params.Limit, offset)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*DatasetItemResponse, len(items))
	for i, item := range items {
		domainResp := item.ToResponse()
		responses[i] = &DatasetItemResponse{
			ID:            domainResp.ID,
			DatasetID:     domainResp.DatasetID,
			Input:         domainResp.Input,
			Expected:      domainResp.Expected,
			Metadata:      domainResp.Metadata,
			Source:        string(domainResp.Source),
			SourceTraceID: domainResp.SourceTraceID,
			SourceSpanID:  domainResp.SourceSpanID,
			CreatedAt:     domainResp.CreatedAt,
		}
	}

	pag := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, pag)
}

// @Summary Create dataset item
// @Description Creates a new item in the dataset.
// @Tags Dataset Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Param request body evaluation.CreateDatasetItemRequest true "Item request"
// @Success 201 {object} DatasetItemResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items [post]
func (h *DatasetItemHandler) Create(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	var req evaluationDomain.CreateDatasetItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	item, err := h.service.Create(c.Request.Context(), datasetID, projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	domainResp := item.ToResponse()
	response.Created(c, &DatasetItemResponse{
		ID:            domainResp.ID,
		DatasetID:     domainResp.DatasetID,
		Input:         domainResp.Input,
		Expected:      domainResp.Expected,
		Metadata:      domainResp.Metadata,
		Source:        string(domainResp.Source),
		SourceTraceID: domainResp.SourceTraceID,
		SourceSpanID:  domainResp.SourceSpanID,
		CreatedAt:     domainResp.CreatedAt,
	})
}

// @Summary Delete dataset item
// @Description Removes an item from the dataset.
// @Tags Dataset Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Param itemId path string true "Item ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items/{itemId} [delete]
func (h *DatasetItemHandler) Delete(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	itemID, err := ulid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid item ID", "itemId must be a valid ULID"))
		return
	}

	if err := h.service.Delete(c.Request.Context(), itemID, datasetID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Import dataset items from JSON
// @Description Imports dataset items from a JSON array with optional field mapping and deduplication.
// @Tags Dataset Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Param request body ImportFromJSONRequest true "Import request"
// @Success 200 {object} BulkImportResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items/import-json [post]
func (h *DatasetItemHandler) ImportFromJSON(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	var req ImportFromJSONRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validate source if provided
	if req.Source != "" {
		source := evaluationDomain.DatasetItemSource(req.Source)
		if !source.IsValid() {
			response.Error(c, appErrors.NewValidationError("Invalid source", "source must be one of: manual, trace, span, csv, json, sdk"))
			return
		}
	}

	domainReq := &evaluationDomain.ImportDatasetItemsFromJSONRequest{
		Items:       req.Items,
		Deduplicate: req.Deduplicate,
		Source:      evaluationDomain.DatasetItemSource(req.Source),
	}
	if req.KeysMapping != nil {
		domainReq.KeysMapping = &evaluationDomain.KeysMapping{
			InputKeys:    req.KeysMapping.InputKeys,
			ExpectedKeys: req.KeysMapping.ExpectedKeys,
			MetadataKeys: req.KeysMapping.MetadataKeys,
		}
	}

	result, err := h.service.ImportFromJSON(c.Request.Context(), datasetID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, &BulkImportResponse{
		Created: result.Created,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	})
}

// @Summary Import dataset items from CSV
// @Description Imports dataset items from CSV content with column mapping and optional deduplication.
// @Tags Dataset Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Param request body ImportFromCSVRequest true "Import CSV request"
// @Success 200 {object} BulkImportResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items/import-csv [post]
func (h *DatasetItemHandler) ImportFromCSV(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	var req ImportFromCSVRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &evaluationDomain.ImportDatasetItemsFromCSVRequest{
		Content:     req.Content,
		HasHeader:   req.HasHeader,
		Deduplicate: req.Deduplicate,
		ColumnMapping: evaluationDomain.CSVColumnMapping{
			InputColumn:     req.ColumnMapping.InputColumn,
			ExpectedColumn:  req.ColumnMapping.ExpectedColumn,
			MetadataColumns: req.ColumnMapping.MetadataColumns,
		},
	}

	result, err := h.service.ImportFromCSV(c.Request.Context(), datasetID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, &BulkImportResponse{
		Created: result.Created,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	})
}

// @Summary Create dataset items from traces
// @Description Creates dataset items from existing trace data (OTEL-native import).
// @Tags Dataset Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Param request body CreateFromTracesRequest true "Create from traces request"
// @Success 200 {object} BulkImportResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items/from-traces [post]
func (h *DatasetItemHandler) CreateFromTraces(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	var req CreateFromTracesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &evaluationDomain.CreateDatasetItemsFromTracesRequest{
		TraceIDs:    req.TraceIDs,
		Deduplicate: req.Deduplicate,
	}
	if req.KeysMapping != nil {
		domainReq.KeysMapping = &evaluationDomain.KeysMapping{
			InputKeys:    req.KeysMapping.InputKeys,
			ExpectedKeys: req.KeysMapping.ExpectedKeys,
			MetadataKeys: req.KeysMapping.MetadataKeys,
		}
	}

	result, err := h.service.CreateFromTraces(c.Request.Context(), datasetID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, &BulkImportResponse{
		Created: result.Created,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	})
}

// @Summary Create dataset items from spans
// @Description Creates dataset items from existing span data.
// @Tags Dataset Items
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Param request body CreateFromSpansRequest true "Create from spans request"
// @Success 200 {object} BulkImportResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items/from-spans [post]
func (h *DatasetItemHandler) CreateFromSpans(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	var req CreateFromSpansRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	domainReq := &evaluationDomain.CreateDatasetItemsFromSpansRequest{
		SpanIDs:     req.SpanIDs,
		Deduplicate: req.Deduplicate,
	}
	if req.KeysMapping != nil {
		domainReq.KeysMapping = &evaluationDomain.KeysMapping{
			InputKeys:    req.KeysMapping.InputKeys,
			ExpectedKeys: req.KeysMapping.ExpectedKeys,
			MetadataKeys: req.KeysMapping.MetadataKeys,
		}
	}

	result, err := h.service.CreateFromSpans(c.Request.Context(), datasetID, projectID, domainReq)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, &BulkImportResponse{
		Created: result.Created,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	})
}

// @Summary Export dataset items
// @Description Exports all dataset items as JSON.
// @Tags Dataset Items
// @Produce json
// @Param projectId path string true "Project ID"
// @Param datasetId path string true "Dataset ID"
// @Success 200 {array} DatasetItemResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items/export [get]
func (h *DatasetItemHandler) Export(c *gin.Context) {
	projectID, err := ulid.Parse(c.Param("projectId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid ULID"))
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	items, err := h.service.ExportItems(c.Request.Context(), datasetID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*DatasetItemResponse, len(items))
	for i, item := range items {
		domainResp := item.ToResponse()
		responses[i] = &DatasetItemResponse{
			ID:            domainResp.ID,
			DatasetID:     domainResp.DatasetID,
			Input:         domainResp.Input,
			Expected:      domainResp.Expected,
			Metadata:      domainResp.Metadata,
			Source:        string(domainResp.Source),
			SourceTraceID: domainResp.SourceTraceID,
			SourceSpanID:  domainResp.SourceSpanID,
			CreatedAt:     domainResp.CreatedAt,
		}
	}

	response.Success(c, responses)
}
