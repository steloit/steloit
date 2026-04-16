package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/common"
	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/prompt"
	appErrors "brokle/pkg/errors"
)

type experimentWizardService struct {
	transactor         common.Transactor
	experimentRepo     evaluation.ExperimentRepository
	configRepo         evaluation.ExperimentConfigRepository
	datasetRepo        evaluation.DatasetRepository
	datasetItemRepo    evaluation.DatasetItemRepository
	datasetVersionRepo evaluation.DatasetVersionRepository
	promptRepo         prompt.PromptRepository
	versionRepo        prompt.VersionRepository
	logger             *slog.Logger
}

func NewExperimentWizardService(
	transactor common.Transactor,
	experimentRepo evaluation.ExperimentRepository,
	configRepo evaluation.ExperimentConfigRepository,
	datasetRepo evaluation.DatasetRepository,
	datasetItemRepo evaluation.DatasetItemRepository,
	datasetVersionRepo evaluation.DatasetVersionRepository,
	promptRepo prompt.PromptRepository,
	versionRepo prompt.VersionRepository,
	logger *slog.Logger,
) evaluation.ExperimentWizardService {
	return &experimentWizardService{
		transactor:         transactor,
		experimentRepo:     experimentRepo,
		configRepo:         configRepo,
		datasetRepo:        datasetRepo,
		datasetItemRepo:    datasetItemRepo,
		datasetVersionRepo: datasetVersionRepo,
		promptRepo:         promptRepo,
		versionRepo:        versionRepo,
		logger:             logger,
	}
}

func (s *experimentWizardService) CreateFromWizard(
	ctx context.Context,
	projectID uuid.UUID,
	userID *uuid.UUID,
	req *evaluation.CreateExperimentFromWizardRequest,
) (*evaluation.Experiment, error) {
	// Parse and validate IDs
	promptID, err := uuid.Parse(req.PromptID)
	if err != nil {
		return nil, appErrors.NewValidationError("prompt_id", "must be a valid UUID")
	}

	promptVersionID, err := uuid.Parse(req.PromptVersionID)
	if err != nil {
		return nil, appErrors.NewValidationError("prompt_version_id", "must be a valid UUID")
	}

	datasetID, err := uuid.Parse(req.DatasetID)
	if err != nil {
		return nil, appErrors.NewValidationError("dataset_id", "must be a valid UUID")
	}

	// Verify prompt exists AND belongs to this project
	p, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if errors.Is(err, prompt.ErrPromptNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", req.PromptID))
		}
		return nil, appErrors.NewInternalError("failed to verify prompt", err)
	}
	if p.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", req.PromptID))
	}

	// Verify prompt version exists AND belongs to this prompt
	v, err := s.versionRepo.GetByID(ctx, promptVersionID)
	if err != nil {
		if errors.Is(err, prompt.ErrVersionNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt version %s", req.PromptVersionID))
		}
		return nil, appErrors.NewInternalError("failed to verify prompt version", err)
	}
	if v.PromptID != promptID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt version %s", req.PromptVersionID))
	}

	// Verify dataset exists
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", req.DatasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	// Parse optional dataset version ID
	var datasetVersionID *uuid.UUID
	if req.DatasetVersionID != nil {
		parsed, err := uuid.Parse(*req.DatasetVersionID)
		if err != nil {
			return nil, appErrors.NewValidationError("dataset_version_id", "must be a valid UUID")
		}
		datasetVersionID = &parsed

		// Verify dataset version exists AND belongs to this dataset
		if _, err := s.datasetVersionRepo.GetByID(ctx, parsed, datasetID); err != nil {
			if errors.Is(err, evaluation.ErrDatasetVersionNotFound) {
				return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset version %s", *req.DatasetVersionID))
			}
			return nil, appErrors.NewInternalError("failed to verify dataset version", err)
		}
	}

	// Create experiment with Source = dashboard
	experiment := evaluation.NewExperimentFromDashboard(projectID, req.Name)
	experiment.Description = req.Description
	experiment.DatasetID = &datasetID

	if validationErrors := experiment.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	// Create experiment config
	config := evaluation.NewExperimentConfig(experiment.ID, promptID, promptVersionID, datasetID)
	config.DatasetVersionID = datasetVersionID
	config.ModelConfig = req.ModelConfigOverride
	config.VariableMapping = req.VariableMapping
	config.Evaluators = req.Evaluators

	if validationErrors := config.Validate(); len(validationErrors) > 0 {
		return nil, appErrors.NewValidationError(validationErrors[0].Field, validationErrors[0].Message)
	}

	// Create both in a transaction with proper FK ordering:
	// 1. Create experiment with config_id = NULL (experiment has no FK dependency)
	// 2. Create config with experiment_id (config.experiment_id -> experiments.id FK is satisfied)
	// 3. Update experiment to set config_id (experiments.config_id -> experiment_configs.id FK is satisfied)
	err = s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		// Step 1: Create experiment first (config_id is nil)
		if err := s.experimentRepo.Create(ctx, experiment); err != nil {
			return fmt.Errorf("failed to create experiment: %w", err)
		}

		// Step 2: Create config (now experiment_id FK is valid)
		if err := s.configRepo.Create(ctx, config); err != nil {
			return fmt.Errorf("failed to create experiment config: %w", err)
		}

		// Step 3: Link experiment to config (now config_id FK is valid)
		experiment.ConfigID = &config.ID
		if err := s.experimentRepo.Update(ctx, experiment, projectID); err != nil {
			return fmt.Errorf("failed to link experiment to config: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, appErrors.NewInternalError("failed to create experiment", err)
	}

	// Load the config into the experiment for the response
	experiment.Config = config

	s.logger.Info("experiment created from wizard",
		"experiment_id", experiment.ID,
		"project_id", projectID,
		"name", experiment.Name,
		"prompt_id", promptID,
		"dataset_id", datasetID,
		"evaluators_count", len(req.Evaluators),
		"run_immediately", req.RunImmediately,
	)

	// If run_immediately is true, update status to running with started_at timestamp
	// Note: Actual execution would be handled by a worker or separate service
	if req.RunImmediately {
		now := time.Now()
		experiment.StartedAt = &now
		experiment.Status = evaluation.ExperimentStatusRunning
		if err := s.experimentRepo.Update(ctx, experiment, projectID); err != nil {
			// Log but don't fail - experiment is created
			s.logger.Warn("failed to update experiment status to running",
				"experiment_id", experiment.ID,
				"error", err,
			)
		}
	}

	return experiment, nil
}

func (s *experimentWizardService) ValidateStep(
	ctx context.Context,
	projectID uuid.UUID,
	req *evaluation.ValidateStepRequest,
) (*evaluation.ValidateStepResponse, error) {
	response := &evaluation.ValidateStepResponse{
		IsValid:  true,
		Errors:   []evaluation.ValidationError{},
		Warnings: []string{},
	}

	switch req.Step {
	case 1:
		// Validate Step 1: Name, Prompt, Version
		s.validateStep1(ctx, projectID, req.Data, response)
	case 2:
		// Validate Step 2: Dataset, Variable Mapping
		s.validateStep2(ctx, projectID, req.Data, response)
	case 3:
		// Validate Step 3: Evaluators
		s.validateStep3(ctx, req.Data, response)
	case 4:
		// Validate Step 4: Review (all previous steps)
		// This would typically validate the complete configuration
		response.IsValid = true
	default:
		return nil, appErrors.NewValidationError("step", "must be between 1 and 4")
	}

	response.IsValid = len(response.Errors) == 0
	return response, nil
}

func (s *experimentWizardService) validateStep1(
	ctx context.Context,
	projectID uuid.UUID,
	data map[string]any,
	response *evaluation.ValidateStepResponse,
) {
	// Validate name
	name, ok := data["name"].(string)
	if !ok || name == "" {
		response.Errors = append(response.Errors, evaluation.ValidationError{
			Field:   "name",
			Message: "name is required",
		})
	} else if len(name) > 255 {
		response.Errors = append(response.Errors, evaluation.ValidationError{
			Field:   "name",
			Message: "name must be at most 255 characters",
		})
	}

	// Track validated promptID for version ownership check
	var validatedPromptID *uuid.UUID

	// Validate prompt_id
	promptIDStr, ok := data["prompt_id"].(string)
	if !ok || promptIDStr == "" {
		response.Errors = append(response.Errors, evaluation.ValidationError{
			Field:   "prompt_id",
			Message: "prompt_id is required",
		})
	} else {
		promptID, err := uuid.Parse(promptIDStr)
		if err != nil {
			response.Errors = append(response.Errors, evaluation.ValidationError{
				Field:   "prompt_id",
				Message: "must be a valid UUID",
			})
		} else {
			// Verify prompt exists AND belongs to this project
			p, err := s.promptRepo.GetByID(ctx, promptID)
			if err != nil {
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "prompt_id",
					Message: "prompt not found",
				})
			} else if p.ProjectID != projectID {
				// Same error message to avoid leaking cross-project prompt existence
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "prompt_id",
					Message: "prompt not found",
				})
			} else {
				validatedPromptID = &promptID
			}
		}
	}

	// Validate prompt_version_id
	versionIDStr, ok := data["prompt_version_id"].(string)
	if !ok || versionIDStr == "" {
		response.Errors = append(response.Errors, evaluation.ValidationError{
			Field:   "prompt_version_id",
			Message: "prompt_version_id is required",
		})
	} else {
		versionID, err := uuid.Parse(versionIDStr)
		if err != nil {
			response.Errors = append(response.Errors, evaluation.ValidationError{
				Field:   "prompt_version_id",
				Message: "must be a valid UUID",
			})
		} else {
			// Verify version exists AND belongs to the validated prompt
			v, err := s.versionRepo.GetByID(ctx, versionID)
			if err != nil {
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "prompt_version_id",
					Message: "prompt version not found",
				})
			} else if validatedPromptID != nil && v.PromptID != *validatedPromptID {
				// Same error message to avoid leaking cross-prompt version existence
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "prompt_version_id",
					Message: "prompt version not found",
				})
			}
		}
	}
}

func (s *experimentWizardService) validateStep2(
	ctx context.Context,
	projectID uuid.UUID,
	data map[string]any,
	response *evaluation.ValidateStepResponse,
) {
	// Validate dataset_id
	datasetIDStr, ok := data["dataset_id"].(string)
	if !ok || datasetIDStr == "" {
		response.Errors = append(response.Errors, evaluation.ValidationError{
			Field:   "dataset_id",
			Message: "dataset_id is required",
		})
	} else {
		datasetID, err := uuid.Parse(datasetIDStr)
		if err != nil {
			response.Errors = append(response.Errors, evaluation.ValidationError{
				Field:   "dataset_id",
				Message: "must be a valid UUID",
			})
		} else {
			if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "dataset_id",
					Message: "dataset not found",
				})
			}
		}
	}

	// Validate variable_mapping if provided
	if mappings, ok := data["variable_mapping"].([]any); ok {
		for i, m := range mappings {
			mapping, ok := m.(map[string]any)
			if !ok {
				continue
			}

			varName, _ := mapping["variable_name"].(string)
			if varName == "" {
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "variable_mapping",
					Message: fmt.Sprintf("variable_name is required at index %d", i),
				})
			}

			source, _ := mapping["source"].(string)
			validSources := map[string]bool{
				"dataset_input":    true,
				"dataset_expected": true,
				"dataset_metadata": true,
			}
			if !validSources[source] {
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "variable_mapping",
					Message: fmt.Sprintf("invalid source '%s' at index %d", source, i),
				})
			}

			fieldPath, _ := mapping["field_path"].(string)
			if fieldPath == "" {
				response.Errors = append(response.Errors, evaluation.ValidationError{
					Field:   "variable_mapping",
					Message: fmt.Sprintf("field_path is required at index %d", i),
				})
			}
		}
	}
}

func (s *experimentWizardService) validateStep3(
	ctx context.Context,
	data map[string]any,
	response *evaluation.ValidateStepResponse,
) {
	// Validate evaluators
	evaluators, ok := data["evaluators"].([]any)
	if !ok || len(evaluators) == 0 {
		response.Errors = append(response.Errors, evaluation.ValidationError{
			Field:   "evaluators",
			Message: "at least one evaluator is required",
		})
		return
	}

	for i, e := range evaluators {
		eval, ok := e.(map[string]any)
		if !ok {
			continue
		}

		name, _ := eval["name"].(string)
		if name == "" {
			response.Errors = append(response.Errors, evaluation.ValidationError{
				Field:   "evaluators",
				Message: fmt.Sprintf("name is required at index %d", i),
			})
		}

		scorerType, _ := eval["scorer_type"].(string)
		validTypes := map[string]bool{
			"llm":     true,
			"builtin": true,
			"regex":   true,
		}
		if !validTypes[scorerType] {
			response.Errors = append(response.Errors, evaluation.ValidationError{
				Field:   "evaluators",
				Message: fmt.Sprintf("invalid scorer_type '%s' at index %d", scorerType, i),
			})
		}

		if _, ok := eval["scorer_config"].(map[string]any); !ok {
			response.Errors = append(response.Errors, evaluation.ValidationError{
				Field:   "evaluators",
				Message: fmt.Sprintf("scorer_config is required at index %d", i),
			})
		}
	}
}

func (s *experimentWizardService) EstimateCost(
	ctx context.Context,
	projectID uuid.UUID,
	req *evaluation.EstimateCostRequest,
) (*evaluation.EstimateCostResponse, error) {
	// Parse dataset ID
	datasetID, err := uuid.Parse(req.DatasetID)
	if err != nil {
		return nil, appErrors.NewValidationError("dataset_id", "must be a valid UUID")
	}

	// Validate dataset belongs to this project (security: prevent cross-project information disclosure)
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", req.DatasetID))
		}
		return nil, appErrors.NewInternalError("failed to verify dataset", err)
	}

	// Get dataset item count
	itemCount, err := s.datasetItemRepo.CountByDataset(ctx, datasetID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to count dataset items", err)
	}

	// Estimate tokens per item (rough estimate)
	// In a real implementation, you would sample items and calculate actual token counts
	estimatedTokensPerItem := int64(500) // Average tokens per prompt + response

	// Count LLM evaluators (they cost tokens)
	llmEvaluatorCount := 0
	for _, eval := range req.Evaluators {
		if eval.ScorerType == evaluation.ScorerTypeLLM {
			llmEvaluatorCount++
		}
	}

	// Calculate costs
	promptExecutionTokens := itemCount * estimatedTokensPerItem
	evaluatorTokens := int64(llmEvaluatorCount) * itemCount * int64(200) // ~200 tokens per LLM evaluation
	totalTokens := promptExecutionTokens + evaluatorTokens

	// Rough cost estimate ($0.01 per 1K tokens, typical for GPT-3.5)
	costPer1KTokens := 0.01
	estimatedCost := float64(totalTokens) / 1000.0 * costPer1KTokens

	breakdown := []evaluation.CostItem{
		{
			Description:    "Prompt Execution",
			EstimatedCost:  float64(promptExecutionTokens) / 1000.0 * costPer1KTokens,
			EstimatedUnits: promptExecutionTokens,
			UnitType:       "tokens",
		},
	}

	if llmEvaluatorCount > 0 {
		breakdown = append(breakdown, evaluation.CostItem{
			Description:    fmt.Sprintf("LLM Evaluators (%d)", llmEvaluatorCount),
			EstimatedCost:  float64(evaluatorTokens) / 1000.0 * costPer1KTokens,
			EstimatedUnits: evaluatorTokens,
			UnitType:       "tokens",
		})
	}

	return &evaluation.EstimateCostResponse{
		ItemCount:       int(itemCount),
		EstimatedTokens: totalTokens,
		EstimatedCost:   estimatedCost,
		Currency:        "USD",
		CostBreakdown:   breakdown,
	}, nil
}

func (s *experimentWizardService) GetDatasetFields(
	ctx context.Context,
	projectID uuid.UUID,
	datasetID uuid.UUID,
) (*evaluation.DatasetFieldsResponse, error) {
	// Verify dataset exists
	if _, err := s.datasetRepo.GetByID(ctx, datasetID, projectID); err != nil {
		if errors.Is(err, evaluation.ErrDatasetNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("dataset %s", datasetID))
		}
		return nil, appErrors.NewInternalError("failed to get dataset", err)
	}

	// Get a single sample item to infer schema (limit=1 for performance)
	items, _, err := s.datasetItemRepo.List(ctx, datasetID, 1, 0)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list dataset items", err)
	}

	response := &evaluation.DatasetFieldsResponse{
		InputFields:    []evaluation.FieldInfo{},
		ExpectedFields: []evaluation.FieldInfo{},
		MetadataFields: []evaluation.FieldInfo{},
	}

	if len(items) == 0 {
		return response, nil
	}

	// Sample first item to infer schema
	sampleItem := items[0]

	// Extract input fields
	response.InputFields = extractFieldsFromMap(sampleItem.Input, "")

	// Extract expected fields
	if len(sampleItem.Expected) > 0 {
		response.ExpectedFields = extractFieldsFromMap(sampleItem.Expected, "")
	}

	// Extract metadata fields
	if sampleItem.Metadata != nil {
		response.MetadataFields = extractFieldsFromMap(sampleItem.Metadata, "")
	}

	return response, nil
}

func (s *experimentWizardService) GetExperimentConfig(
	ctx context.Context,
	experimentID uuid.UUID,
	projectID uuid.UUID,
) (*evaluation.ExperimentConfig, error) {
	// First verify experiment exists and belongs to project
	experiment, err := s.experimentRepo.GetByID(ctx, experimentID, projectID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentNotFound) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("experiment %s", experimentID))
		}
		return nil, appErrors.NewInternalError("failed to get experiment", err)
	}

	// Check if experiment has a config
	if experiment.ConfigID == nil {
		return nil, appErrors.NewNotFoundError("experiment config")
	}

	// Get the config
	config, err := s.configRepo.GetByExperimentID(ctx, experimentID)
	if err != nil {
		if errors.Is(err, evaluation.ErrExperimentConfigNotFound) {
			return nil, appErrors.NewNotFoundError("experiment config")
		}
		return nil, appErrors.NewInternalError("failed to get experiment config", err)
	}

	return config, nil
}

// Helper functions

func extractFieldsFromMap(data map[string]any, prefix string) []evaluation.FieldInfo {
	fields := []evaluation.FieldInfo{}

	for key, value := range data {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		fieldType := inferType(value)

		// Handle nested objects
		if nestedMap, ok := value.(map[string]any); ok && len(nestedMap) > 0 {
			// Add the parent field
			fields = append(fields, evaluation.FieldInfo{
				Path: path,
				Type: "object",
			})
			// Add nested fields
			fields = append(fields, extractFieldsFromMap(nestedMap, path)...)
		} else if arr, ok := value.([]any); ok && len(arr) > 0 {
			// For arrays, check the first element
			elemType := "unknown"
			if len(arr) > 0 {
				elemType = inferType(arr[0])
			}
			fields = append(fields, evaluation.FieldInfo{
				Path:        path,
				Type:        "array",
				SampleValue: fmt.Sprintf("array<%s>", elemType),
			})
		} else {
			fields = append(fields, evaluation.FieldInfo{
				Path:        path,
				Type:        fieldType,
				SampleValue: value,
			})
		}
	}

	return fields
}

func inferType(value any) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case string:
		return "string"
	case float64, float32, int, int32, int64:
		return "number"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		// Use reflection for other types
		rt := reflect.TypeOf(v)
		if rt != nil {
			return rt.Kind().String()
		}
		return "unknown"
	}
}

// Helper to safely marshal and unmarshal for type conversion
func convertToVariableMapping(data []any) ([]evaluation.ExperimentVariableMapping, error) {
	mappings := make([]evaluation.ExperimentVariableMapping, 0, len(data))

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonBytes, &mappings); err != nil {
		return nil, err
	}

	return mappings, nil
}
