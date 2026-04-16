package evaluation

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
)

type datasetItemService struct {
	itemRepo    evaluation.DatasetItemRepository
	datasetRepo evaluation.DatasetRepository
	traceRepo   observability.TraceRepository
	logger      *slog.Logger
}

func NewDatasetItemService(
	itemRepo evaluation.DatasetItemRepository,
	datasetRepo evaluation.DatasetRepository,
	traceRepo observability.TraceRepository,
	logger *slog.Logger,
) evaluation.DatasetItemService {
	return &datasetItemService{
		itemRepo:    itemRepo,
		datasetRepo: datasetRepo,
		traceRepo:   traceRepo,
		logger:      logger,
	}
}

func (s *datasetItemService) Create(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *evaluation.CreateDatasetItemRequest) (*evaluation.DatasetItem, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	item := evaluation.NewDatasetItem(datasetID, req.Input)
	item.Expected = req.Expected
	if req.Metadata != nil {
		item.Metadata = req.Metadata
	}

	hash := s.computeContentHash(req.Input, req.Expected)
	item.ContentHash = &hash

	if validationErrors := item.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	if err := s.itemRepo.Create(ctx, item); err != nil {
		return nil, appErrors.NewInternalError("failed to create dataset item", err)
	}

	s.logger.Info("dataset item created",
		"item_id", item.ID,
		"dataset_id", datasetID,
	)

	return item, nil
}

func (s *datasetItemService) CreateBatch(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *evaluation.CreateDatasetItemsBatchRequest) (int, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return 0, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return 0, appErrors.NewInternalError("failed to verify dataset", err)
	}

	if len(req.Items) == 0 {
		return 0, appErrors.NewValidationError("items", "items array cannot be empty")
	}

	// First pass: compute content hashes and validate all items
	contentHashes := make([]string, len(req.Items))
	for i, itemReq := range req.Items {
		contentHashes[i] = s.computeContentHash(itemReq.Input, itemReq.Expected)

		// Validate item structure
		item := evaluation.NewDatasetItem(datasetID, itemReq.Input)
		item.Expected = itemReq.Expected
		if itemReq.Metadata != nil {
			item.Metadata = itemReq.Metadata
		}
		if validationErrors := item.Validate(); len(validationErrors) > 0 {
			return 0, appErrors.NewValidationError(
				fmt.Sprintf("items[%d].%s", i, validationErrors[0].Field),
				validationErrors[0].Message,
			)
		}
	}

	// Check for existing items if deduplication is enabled
	var existingHashes map[string]bool
	if req.Deduplicate {
		var err error
		existingHashes, err = s.itemRepo.FindByContentHashes(ctx, datasetID, contentHashes)
		if err != nil {
			return 0, appErrors.NewInternalError("failed to check for duplicates", err)
		}
	}

	// Second pass: create items, skipping duplicates if deduplication is enabled
	items := make([]*evaluation.DatasetItem, 0, len(req.Items))
	skipped := 0
	seenHashes := make(map[string]bool) // Track hashes within this batch to avoid in-batch duplicates

	for i, itemReq := range req.Items {
		hash := contentHashes[i]

		// Skip if hash exists in database (deduplication enabled)
		if req.Deduplicate && existingHashes != nil && existingHashes[hash] {
			skipped++
			continue
		}

		// Skip if hash already seen in this batch (deduplication enabled)
		if req.Deduplicate && seenHashes[hash] {
			skipped++
			continue
		}

		item := evaluation.NewDatasetItem(datasetID, itemReq.Input)
		item.Expected = itemReq.Expected
		if itemReq.Metadata != nil {
			item.Metadata = itemReq.Metadata
		}
		item.ContentHash = &hash

		items = append(items, item)
		seenHashes[hash] = true
	}

	// Only create if there are items to create
	if len(items) > 0 {
		if err := s.itemRepo.CreateBatch(ctx, items); err != nil {
			return 0, appErrors.NewInternalError("failed to create dataset items", err)
		}
	}

	s.logger.Info("dataset items batch created",
		"dataset_id", datasetID,
		"created", len(items),
		"skipped", skipped,
	)

	return len(items), nil
}

func (s *datasetItemService) List(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, limit, offset int) ([]*evaluation.DatasetItem, int64, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, 0, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, 0, appErrors.NewInternalError("failed to verify dataset", err)
	}

	items, total, err := s.itemRepo.List(ctx, datasetID, limit, offset)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list dataset items", err)
	}
	return items, total, nil
}

func (s *datasetItemService) Delete(ctx context.Context, id uuid.UUID, datasetID uuid.UUID, projectID uuid.UUID) error {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return appErrors.NewInternalError("failed to verify dataset", err)
	}

	if err := s.itemRepo.Delete(ctx, id, datasetID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetItemNotFound) {
			return appErrors.NewNotFoundError(fmt.Sprintf("dataset item %s", id))
		}
		return appErrors.NewInternalError("failed to delete dataset item", err)
	}

	s.logger.Info("dataset item deleted",
		"item_id", id,
		"dataset_id", datasetID,
	)

	return nil
}

// ImportFromJSON imports dataset items from a JSON array with optional field mapping and deduplication.
func (s *datasetItemService) ImportFromJSON(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *evaluation.ImportDatasetItemsFromJSONRequest) (*evaluation.BulkImportResult, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	if len(req.Items) == 0 {
		return nil, appErrors.NewValidationError("items", "items array cannot be empty")
	}

	// Validate source if provided
	if req.Source != "" && !req.Source.IsValid() {
		return nil, appErrors.NewValidationError("source", "must be one of: manual, trace, span, csv, json, sdk")
	}

	result := &evaluation.BulkImportResult{}
	items := make([]*evaluation.DatasetItem, 0, len(req.Items))
	contentHashes := make([]string, 0, len(req.Items))

	// First pass: compute content hashes for deduplication
	for _, rawItem := range req.Items {
		input, expected, _ := s.extractFieldsFromRaw(rawItem, req.KeysMapping)
		hash := s.computeContentHash(input, expected)
		contentHashes = append(contentHashes, hash)
	}

	// Check for existing items if deduplication is enabled
	var existingHashes map[string]bool
	if req.Deduplicate {
		var err error
		existingHashes, err = s.itemRepo.FindByContentHashes(ctx, datasetID, contentHashes)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check for duplicates", err)
		}
	}

	// Second pass: create items
	seenHashes := make(map[string]bool) // Track within-batch duplicates

	for i, rawItem := range req.Items {
		hash := contentHashes[i]

		// Skip if already in database
		if req.Deduplicate && existingHashes != nil && existingHashes[hash] {
			result.Skipped++
			continue
		}

		// Skip if already seen in this batch
		if req.Deduplicate && seenHashes[hash] {
			result.Skipped++
			continue
		}

		input, expected, metadata := s.extractFieldsFromRaw(rawItem, req.KeysMapping)

		source := req.Source
		if source == "" {
			source = evaluation.DatasetItemSourceJSON
		}
		item := evaluation.NewDatasetItemWithSource(datasetID, input, source)
		item.Expected = expected
		item.Metadata = metadata
		item.ContentHash = &hash

		if validationErrors := item.Validate(); len(validationErrors) > 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("items[%d]: %s", i, validationErrors[0].Message))
			continue
		}

		items = append(items, item)
		seenHashes[hash] = true // Mark as seen in this batch
	}

	if len(items) > 0 {
		if err := s.itemRepo.CreateBatch(ctx, items); err != nil {
			return nil, appErrors.NewInternalError("failed to create dataset items", err)
		}
	}

	result.Created = len(items)

	s.logger.Info("dataset items imported from JSON",
		"dataset_id", datasetID,
		"created", result.Created,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)

	return result, nil
}

// ImportFromCSV imports dataset items from CSV content with column mapping.
func (s *datasetItemService) ImportFromCSV(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *evaluation.ImportDatasetItemsFromCSVRequest) (*evaluation.BulkImportResult, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	if req.Content == "" {
		return nil, appErrors.NewValidationError("content", "content cannot be empty")
	}

	if req.ColumnMapping.InputColumn == "" {
		return nil, appErrors.NewValidationError("column_mapping.input_column", "input column is required")
	}

	reader := csv.NewReader(strings.NewReader(req.Content))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, appErrors.NewValidationError("content", fmt.Sprintf("invalid CSV format: %v", err))
	}

	if len(records) == 0 {
		return nil, appErrors.NewValidationError("content", "CSV content is empty")
	}

	var headers []string
	startRow := 0
	if req.HasHeader {
		if len(records) < 2 {
			return nil, appErrors.NewValidationError("content", "CSV must have at least one data row after header")
		}
		headers = records[0]
		startRow = 1
	} else {
		// Generate column names like "col_0", "col_1", etc.
		if len(records[0]) > 0 {
			headers = make([]string, len(records[0]))
			for i := range records[0] {
				headers[i] = fmt.Sprintf("col_%d", i)
			}
		}
	}

	columnIndex := make(map[string]int)
	for i, header := range headers {
		columnIndex[header] = i
	}

	if _, ok := columnIndex[req.ColumnMapping.InputColumn]; !ok {
		return nil, appErrors.NewValidationError("column_mapping.input_column", fmt.Sprintf("column '%s' not found in CSV", req.ColumnMapping.InputColumn))
	}
	if req.ColumnMapping.ExpectedColumn != "" {
		if _, ok := columnIndex[req.ColumnMapping.ExpectedColumn]; !ok {
			return nil, appErrors.NewValidationError("column_mapping.expected_column", fmt.Sprintf("column '%s' not found in CSV", req.ColumnMapping.ExpectedColumn))
		}
	}
	for _, col := range req.ColumnMapping.MetadataColumns {
		if _, ok := columnIndex[col]; !ok {
			return nil, appErrors.NewValidationError("column_mapping.metadata_columns", fmt.Sprintf("column '%s' not found in CSV", col))
		}
	}

	result := &evaluation.BulkImportResult{}
	items := make([]*evaluation.DatasetItem, 0, len(records)-startRow)
	contentHashes := make([]string, 0, len(records)-startRow)

	// First pass: extract data and compute content hashes
	type csvRowData struct {
		input    map[string]interface{}
		expected map[string]interface{}
		metadata map[string]interface{}
		hash     string
	}
	rowDataList := make([]csvRowData, 0, len(records)-startRow)

	inputIdx := columnIndex[req.ColumnMapping.InputColumn]
	var expectedIdx *int
	if req.ColumnMapping.ExpectedColumn != "" {
		idx := columnIndex[req.ColumnMapping.ExpectedColumn]
		expectedIdx = &idx
	}

	for i := startRow; i < len(records); i++ {
		row := records[i]
		if len(row) == 0 {
			continue
		}

		input := make(map[string]interface{})
		if inputIdx < len(row) {
			input["value"] = s.parseCSVValue(row[inputIdx])
		}

		expected := make(map[string]interface{})
		if expectedIdx != nil && *expectedIdx < len(row) {
			expected["value"] = s.parseCSVValue(row[*expectedIdx])
		}

		metadata := make(map[string]interface{})
		for _, col := range req.ColumnMapping.MetadataColumns {
			idx := columnIndex[col]
			if idx < len(row) {
				metadata[col] = s.parseCSVValue(row[idx])
			}
		}

		hash := s.computeContentHash(input, expected)
		contentHashes = append(contentHashes, hash)

		rowDataList = append(rowDataList, csvRowData{
			input:    input,
			expected: expected,
			metadata: metadata,
			hash:     hash,
		})
	}

	// Check for existing items if deduplication is enabled
	var existingHashes map[string]bool
	if req.Deduplicate && len(contentHashes) > 0 {
		var err error
		existingHashes, err = s.itemRepo.FindByContentHashes(ctx, datasetID, contentHashes)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check for duplicates", err)
		}
	}

	// Second pass: create items
	seenHashes := make(map[string]bool) // Track within-batch duplicates

	for i, rd := range rowDataList {
		// Skip if already in database
		if req.Deduplicate && existingHashes != nil && existingHashes[rd.hash] {
			result.Skipped++
			continue
		}

		// Skip if already seen in this batch
		if req.Deduplicate && seenHashes[rd.hash] {
			result.Skipped++
			continue
		}

		item := evaluation.NewDatasetItemWithSource(datasetID, rd.input, evaluation.DatasetItemSourceCSV)
		if len(rd.expected) > 0 {
			item.Expected = rd.expected
		}
		if len(rd.metadata) > 0 {
			item.Metadata = rd.metadata
		}
		item.ContentHash = &rd.hash

		if validationErrors := item.Validate(); len(validationErrors) > 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: %s", i+startRow+1, validationErrors[0].Message))
			continue
		}

		items = append(items, item)
		seenHashes[rd.hash] = true // Mark as seen in this batch
	}

	if len(items) > 0 {
		if err := s.itemRepo.CreateBatch(ctx, items); err != nil {
			return nil, appErrors.NewInternalError("failed to create dataset items", err)
		}
	}

	result.Created = len(items)

	s.logger.Info("dataset items imported from CSV",
		"dataset_id", datasetID,
		"created", result.Created,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)

	return result, nil
}

// parseCSVValue attempts to parse a CSV cell value into its appropriate type.
// Tries JSON parsing first (for objects, arrays, booleans, numbers), falls back to string.
func (s *datasetItemService) parseCSVValue(value string) interface{} {
	value = strings.TrimSpace(value)

	var parsed interface{}
	if err := json.Unmarshal([]byte(value), &parsed); err == nil {
		return parsed
	}

	return value
}

// CreateFromTraces creates dataset items from existing trace data (OTEL-native import).
func (s *datasetItemService) CreateFromTraces(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *evaluation.CreateDatasetItemsFromTracesRequest) (*evaluation.BulkImportResult, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	if len(req.TraceIDs) == 0 {
		return nil, appErrors.NewValidationError("trace_ids", "trace_ids array cannot be empty")
	}

	result := &evaluation.BulkImportResult{}
	items := make([]*evaluation.DatasetItem, 0, len(req.TraceIDs))
	contentHashes := make([]string, 0, len(req.TraceIDs))

	// First pass: fetch traces and compute content hashes
	traceDataMap := make(map[string]*traceData)
	for _, traceID := range req.TraceIDs {
		// Use project-scoped query to prevent cross-project data access
		rootSpan, err := s.traceRepo.GetRootSpanByProject(ctx, traceID, projectID.String())
		if err != nil {
			// Generic error message to prevent enumeration attacks
			result.Errors = append(result.Errors, fmt.Sprintf("trace %s: not found or unauthorized", traceID))
			continue
		}

		input, expected, metadata := s.extractFieldsFromSpan(rootSpan, req.KeysMapping)
		hash := s.computeContentHash(input, expected)
		contentHashes = append(contentHashes, hash)

		traceDataMap[traceID] = &traceData{
			input:    input,
			expected: expected,
			metadata: metadata,
			hash:     hash,
		}
	}

	// Check for existing items if deduplication is enabled
	var existingHashes map[string]bool
	if req.Deduplicate && len(contentHashes) > 0 {
		var err error
		existingHashes, err = s.itemRepo.FindByContentHashes(ctx, datasetID, contentHashes)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check for duplicates", err)
		}
	}

	// Second pass: create items
	seenHashes := make(map[string]bool) // Track within-batch duplicates

	for _, traceID := range req.TraceIDs {
		td, ok := traceDataMap[traceID]
		if !ok {
			continue
		}

		// Skip if already in database
		if req.Deduplicate && existingHashes != nil && existingHashes[td.hash] {
			result.Skipped++
			continue
		}

		// Skip if already seen in this batch
		if req.Deduplicate && seenHashes[td.hash] {
			result.Skipped++
			continue
		}

		item := evaluation.NewDatasetItemWithSource(datasetID, td.input, evaluation.DatasetItemSourceTrace)
		item.Expected = td.expected
		item.Metadata = td.metadata
		item.SourceTraceID = &traceID
		item.ContentHash = &td.hash

		if validationErrors := item.Validate(); len(validationErrors) > 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("trace %s: %s", traceID, validationErrors[0].Message))
			continue
		}

		items = append(items, item)
		seenHashes[td.hash] = true // Mark as seen in this batch
	}

	if len(items) > 0 {
		if err := s.itemRepo.CreateBatch(ctx, items); err != nil {
			return nil, appErrors.NewInternalError("failed to create dataset items", err)
		}
	}

	result.Created = len(items)

	s.logger.Info("dataset items created from traces",
		"dataset_id", datasetID,
		"created", result.Created,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)

	return result, nil
}

// CreateFromSpans creates dataset items from existing span data.
func (s *datasetItemService) CreateFromSpans(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID, req *evaluation.CreateDatasetItemsFromSpansRequest) (*evaluation.BulkImportResult, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	if len(req.SpanIDs) == 0 {
		return nil, appErrors.NewValidationError("span_ids", "span_ids array cannot be empty")
	}

	result := &evaluation.BulkImportResult{}
	items := make([]*evaluation.DatasetItem, 0, len(req.SpanIDs))
	contentHashes := make([]string, 0, len(req.SpanIDs))

	// First pass: fetch spans and compute content hashes
	spanDataMap := make(map[string]*spanData)
	for _, spanID := range req.SpanIDs {
		// Use project-scoped query to prevent cross-project data access
		span, err := s.traceRepo.GetSpanByProject(ctx, spanID, projectID.String())
		if err != nil {
			// Generic error message to prevent enumeration attacks
			result.Errors = append(result.Errors, fmt.Sprintf("span %s: not found or unauthorized", spanID))
			continue
		}

		input, expected, metadata := s.extractFieldsFromSpan(span, req.KeysMapping)
		hash := s.computeContentHash(input, expected)
		contentHashes = append(contentHashes, hash)

		spanDataMap[spanID] = &spanData{
			traceID:  span.TraceID,
			input:    input,
			expected: expected,
			metadata: metadata,
			hash:     hash,
		}
	}

	// Check for existing items if deduplication is enabled
	var existingHashes map[string]bool
	if req.Deduplicate && len(contentHashes) > 0 {
		var err error
		existingHashes, err = s.itemRepo.FindByContentHashes(ctx, datasetID, contentHashes)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check for duplicates", err)
		}
	}

	// Second pass: create items
	seenHashes := make(map[string]bool) // Track within-batch duplicates

	for _, spanID := range req.SpanIDs {
		sd, ok := spanDataMap[spanID]
		if !ok {
			continue
		}

		// Skip if already in database
		if req.Deduplicate && existingHashes != nil && existingHashes[sd.hash] {
			result.Skipped++
			continue
		}

		// Skip if already seen in this batch
		if req.Deduplicate && seenHashes[sd.hash] {
			result.Skipped++
			continue
		}

		item := evaluation.NewDatasetItemWithSource(datasetID, sd.input, evaluation.DatasetItemSourceSpan)
		item.Expected = sd.expected
		item.Metadata = sd.metadata
		item.SourceTraceID = &sd.traceID
		item.SourceSpanID = &spanID
		item.ContentHash = &sd.hash

		if validationErrors := item.Validate(); len(validationErrors) > 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("span %s: %s", spanID, validationErrors[0].Message))
			continue
		}

		items = append(items, item)
		seenHashes[sd.hash] = true // Mark as seen in this batch
	}

	if len(items) > 0 {
		if err := s.itemRepo.CreateBatch(ctx, items); err != nil {
			return nil, appErrors.NewInternalError("failed to create dataset items", err)
		}
	}

	result.Created = len(items)

	s.logger.Info("dataset items created from spans",
		"dataset_id", datasetID,
		"created", result.Created,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)

	return result, nil
}

// ExportItems exports all dataset items for a dataset.
func (s *datasetItemService) ExportItems(ctx context.Context, datasetID uuid.UUID, projectID uuid.UUID) ([]*evaluation.DatasetItem, error) {
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	items, err := s.itemRepo.ListAll(ctx, datasetID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to export dataset items", err)
	}

	s.logger.Info("dataset items exported",
		"dataset_id", datasetID,
		"count", len(items),
	)

	return items, nil
}

// Helper types for trace/span data extraction
type traceData struct {
	input    map[string]interface{}
	expected map[string]interface{}
	metadata map[string]interface{}
	hash     string
}

type spanData struct {
	traceID  string
	input    map[string]interface{}
	expected map[string]interface{}
	metadata map[string]interface{}
	hash     string
}

// computeContentHash computes a SHA256 hash of the input and expected fields for deduplication.
func (s *datasetItemService) computeContentHash(input, expected map[string]interface{}) string {
	return ComputeContentHash(input, expected)
}

// extractFieldsFromRaw extracts input, expected, and metadata fields from a raw JSON item using keys mapping.
func (s *datasetItemService) extractFieldsFromRaw(raw map[string]interface{}, mapping *evaluation.KeysMapping) (input, expected, metadata map[string]interface{}) {
	input = make(map[string]interface{})
	expected = make(map[string]interface{})
	metadata = make(map[string]interface{})

	if mapping == nil || (len(mapping.InputKeys) == 0 && len(mapping.ExpectedKeys) == 0 && len(mapping.MetadataKeys) == 0) {
		// No mapping: use "input", "expected", "metadata" keys or the whole object as input
		if v, ok := raw["input"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				input = m
			} else {
				input["value"] = v
			}
		} else {
			// If no "input" key, use entire object as input (excluding expected/metadata)
			for k, v := range raw {
				if k != "expected" && k != "metadata" {
					input[k] = v
				}
			}
		}

		if v, ok := raw["expected"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				expected = m
			} else {
				expected["value"] = v
			}
		}

		if v, ok := raw["metadata"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				metadata = m
			} else {
				metadata["value"] = v
			}
		}
		return
	}

	// Apply keys mapping
	for _, key := range mapping.InputKeys {
		if v, ok := raw[key]; ok {
			input[key] = v
		}
	}

	for _, key := range mapping.ExpectedKeys {
		if v, ok := raw[key]; ok {
			expected[key] = v
		}
	}

	for _, key := range mapping.MetadataKeys {
		if v, ok := raw[key]; ok {
			metadata[key] = v
		}
	}

	return
}

// extractFieldsFromSpan extracts input, expected, and metadata fields from a span using keys mapping.
func (s *datasetItemService) extractFieldsFromSpan(span *observability.Span, mapping *evaluation.KeysMapping) (input, expected, metadata map[string]interface{}) {
	input = make(map[string]interface{})
	expected = make(map[string]interface{})
	metadata = make(map[string]interface{})

	// Parse span input
	if span.Input != nil && *span.Input != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(*span.Input), &parsed); err == nil {
			if m, ok := parsed.(map[string]interface{}); ok {
				input = m
			} else {
				input["value"] = parsed
			}
		} else {
			input["value"] = *span.Input
		}
	}

	// Parse span output as expected
	if span.Output != nil && *span.Output != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(*span.Output), &parsed); err == nil {
			if m, ok := parsed.(map[string]interface{}); ok {
				expected = m
			} else {
				expected["value"] = parsed
			}
		} else {
			expected["value"] = *span.Output
		}
	}

	// Apply keys mapping to filter specific fields
	if mapping != nil && len(mapping.InputKeys) > 0 {
		filtered := make(map[string]interface{})
		for _, key := range mapping.InputKeys {
			if v, ok := input[key]; ok {
				filtered[key] = v
			}
		}
		input = filtered
	}

	if mapping != nil && len(mapping.ExpectedKeys) > 0 {
		filtered := make(map[string]interface{})
		for _, key := range mapping.ExpectedKeys {
			if v, ok := expected[key]; ok {
				filtered[key] = v
			}
		}
		expected = filtered
	}

	// Extract metadata from span attributes using MetadataKeys mapping
	if mapping != nil && len(mapping.MetadataKeys) > 0 && span.SpanAttributes != nil {
		for _, key := range mapping.MetadataKeys {
			if v, ok := span.SpanAttributes[key]; ok {
				metadata[key] = v
			}
		}
	}

	return
}
