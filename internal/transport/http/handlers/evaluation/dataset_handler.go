package evaluation

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	evaluationDomain "brokle/internal/core/domain/evaluation"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"
)

type DatasetHandler struct {
	logger      *slog.Logger
	service     evaluationDomain.DatasetService
	itemService evaluationDomain.DatasetItemService
}

func NewDatasetHandler(
	logger *slog.Logger,
	service evaluationDomain.DatasetService,
	itemService evaluationDomain.DatasetItemService,
) *DatasetHandler {
	return &DatasetHandler{
		logger:      logger,
		service:     service,
		itemService: itemService,
	}
}

// @Summary Create dataset
// @Description Creates a new dataset for the project. Works for both SDK and Dashboard routes.
// @Tags Datasets, SDK - Datasets
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param request body evaluation.CreateDatasetRequest true "Dataset request"
// @Success 201 {object} evaluation.DatasetResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/datasets [post]
// @Router /v1/datasets [post]
func (h *DatasetHandler) Create(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req evaluationDomain.CreateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	dataset, err := h.service.Create(c.Request.Context(), projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("dataset created",
		"dataset_id", dataset.ID,
		"project_id", projectID,
		"name", dataset.Name,
	)

	response.Created(c, dataset.ToResponse())
}

// @Summary List datasets
// @Description Returns all datasets for the project with pagination, search, and sorting. Works for both SDK and Dashboard routes.
// @Tags Datasets, SDK - Datasets
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param search query string false "Search by name (case-insensitive partial match)"
// @Param sort_by query string false "Sort field (name, created_at, updated_at, item_count)" default(updated_at)
// @Param sort_dir query string false "Sort direction (asc, desc)" default(desc)
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page (10, 25, 50, 100)" default(50)
// @Success 200 {object} response.APIResponse{data=[]evaluation.DatasetWithItemCountResponse,meta=response.Meta}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets [get]
// @Router /v1/datasets [get]
func (h *DatasetHandler) List(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Parse filters
	filter := &evaluationDomain.DatasetFilter{}
	if search := c.Query("search"); search != "" {
		filter.Search = &search
	}

	// Parse pagination params
	params := response.ParsePaginationParams(
		c.Query("page"),
		c.Query("limit"),
		c.Query("sort_by"),
		c.Query("sort_dir"),
	)

	// Validate pagination parameters to prevent SQL injection
	if err := params.Validate(); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid pagination parameters", err.Error()))
		return
	}

	datasets, total, err := h.service.ListWithFilters(c.Request.Context(), projectID, filter, params)
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]*evaluationDomain.DatasetWithItemCountResponse, len(datasets))
	for i, dataset := range datasets {
		responses[i] = dataset.ToResponse()
	}

	paginationMeta := response.NewPagination(params.Page, params.Limit, total)
	response.SuccessWithPagination(c, responses, paginationMeta)
}

// @Summary Get dataset
// @Description Returns the dataset for a specific ID. Works for both SDK and Dashboard routes.
// @Tags Datasets, SDK - Datasets
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param datasetId path string true "Dataset ID"
// @Success 200 {object} evaluation.DatasetResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId} [get]
// @Router /v1/datasets/{datasetId} [get]
func (h *DatasetHandler) Get(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	dataset, err := h.service.GetByID(c.Request.Context(), datasetID, projectID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, dataset.ToResponse())
}

// @Summary Update dataset
// @Description Updates an existing dataset by ID. Works for both SDK and Dashboard routes.
// @Tags Datasets, SDK - Datasets
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param datasetId path string true "Dataset ID"
// @Param request body evaluation.UpdateDatasetRequest true "Update request"
// @Success 200 {object} evaluation.DatasetResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Name already exists"
// @Router /api/v1/projects/{projectId}/datasets/{datasetId} [put]
// @Router /v1/datasets/{datasetId} [patch]
func (h *DatasetHandler) Update(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	var req evaluationDomain.UpdateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	dataset, err := h.service.Update(c.Request.Context(), datasetID, projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, dataset.ToResponse())
}

// @Summary Delete dataset
// @Description Removes a dataset by its ID. Also deletes all items in the dataset. Works for both SDK and Dashboard routes.
// @Tags Datasets, SDK - Datasets
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param datasetId path string true "Dataset ID"
// @Success 204 "No Content"
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId} [delete]
// @Router /v1/datasets/{datasetId} [delete]
func (h *DatasetHandler) Delete(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	if err := h.service.Delete(c.Request.Context(), datasetID, projectID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// @Summary Batch create dataset items via SDK
// @Description Creates multiple items in a dataset using API key authentication.
// @Tags SDK - Datasets
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param datasetId path string true "Dataset ID"
// @Param request body evaluation.CreateDatasetItemsBatchRequest true "Batch items request"
// @Success 201 {object} SDKBatchCreateItemsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /v1/datasets/{datasetId}/items [post]
func (h *DatasetHandler) CreateItems(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	var req evaluationDomain.CreateDatasetItemsBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	count, err := h.itemService.CreateBatch(c.Request.Context(), datasetID, projectID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}

	h.logger.Info("dataset items created",
		"dataset_id", datasetID,
		"project_id", projectID,
		"count", count,
	)

	response.Created(c, &SDKBatchCreateItemsResponse{Created: count})
}

// @Summary List dataset items via SDK
// @Description Returns items for a dataset with pagination using API key authentication.
// @Tags SDK - Datasets
// @Produce json
// @Security ApiKeyAuth
// @Param datasetId path string true "Dataset ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (10, 25, 50, 100; default 50)"
// @Success 200 {object} response.ListResponse{data=[]DatasetItemResponse}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /v1/datasets/{datasetId}/items [get]
func (h *DatasetHandler) ListItems(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	params := response.ParsePaginationParams(c.Query("page"), c.Query("limit"), "", "")
	offset := (params.Page - 1) * params.Limit

	items, total, err := h.itemService.List(c.Request.Context(), datasetID, projectID, params.Limit, offset)
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

// @Summary Export dataset items
// @Description Exports all items from a dataset. Works for both SDK and Dashboard routes.
// @Tags Datasets, SDK - Datasets
// @Produce json
// @Security ApiKeyAuth
// @Param projectId path string false "Project ID (Dashboard routes)"
// @Param datasetId path string true "Dataset ID"
// @Success 200 {array} DatasetItemResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/projects/{projectId}/datasets/{datasetId}/items/export [get]
// @Router /v1/datasets/{datasetId}/items/export [get]
func (h *DatasetHandler) ExportItems(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	datasetID, err := ulid.Parse(c.Param("datasetId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid dataset ID", "datasetId must be a valid ULID"))
		return
	}

	items, err := h.itemService.ExportItems(c.Request.Context(), datasetID, projectID)
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

// ImportFromJSON imports dataset items from JSON data (SDK route)
// @Summary Import dataset items from JSON via SDK
// @Tags SDK - Datasets
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param datasetId path string true "Dataset ID"
// @Param request body ImportFromJSONRequest true "Import request"
// @Success 200 {object} BulkImportResponse
// @Router /v1/datasets/{datasetId}/items/import-json [post]
func (h *DatasetHandler) ImportFromJSON(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
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

	result, err := h.itemService.ImportFromJSON(c.Request.Context(), datasetID, projectID, domainReq)
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

// ImportFromCSV imports dataset items from CSV content (SDK route)
// @Summary Import dataset items from CSV via SDK
// @Tags SDK - Datasets
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param datasetId path string true "Dataset ID"
// @Param request body ImportFromCSVRequest true "Import CSV request"
// @Success 200 {object} BulkImportResponse
// @Router /v1/datasets/{datasetId}/items/import-csv [post]
func (h *DatasetHandler) ImportFromCSV(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
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

	result, err := h.itemService.ImportFromCSV(c.Request.Context(), datasetID, projectID, domainReq)
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

// CreateFromTraces creates dataset items from production traces (SDK route)
// @Summary Create dataset items from traces via SDK
// @Tags SDK - Datasets
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param datasetId path string true "Dataset ID"
// @Param request body CreateFromTracesRequest true "Create from traces request"
// @Success 200 {object} BulkImportResponse
// @Router /v1/datasets/{datasetId}/items/from-traces [post]
func (h *DatasetHandler) CreateFromTraces(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
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

	result, err := h.itemService.CreateFromTraces(c.Request.Context(), datasetID, projectID, domainReq)
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

// CreateFromSpans creates dataset items from production spans (SDK route)
// @Summary Create dataset items from spans via SDK
// @Tags SDK - Datasets
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param datasetId path string true "Dataset ID"
// @Param request body CreateFromSpansRequest true "Create from spans request"
// @Success 200 {object} BulkImportResponse
// @Router /v1/datasets/{datasetId}/items/from-spans [post]
func (h *DatasetHandler) CreateFromSpans(c *gin.Context) {
	projectID, err := extractProjectID(c)
	if err != nil {
		response.Error(c, err)
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

	result, err := h.itemService.CreateFromSpans(c.Request.Context(), datasetID, projectID, domainReq)
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
