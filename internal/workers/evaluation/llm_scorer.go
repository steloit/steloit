package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"brokle/internal/core/domain/credentials"
	"brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/prompt"
)

// LLMScorer implements LLM-as-a-judge scoring using project credentials
type LLMScorer struct {
	credentialsService credentials.ProviderCredentialService
	executionService   prompt.ExecutionService
	logger             *slog.Logger
}

// NewLLMScorer creates a new LLM scorer
func NewLLMScorer(
	credentialsService credentials.ProviderCredentialService,
	executionService prompt.ExecutionService,
	logger *slog.Logger,
) *LLMScorer {
	return &LLMScorer{
		credentialsService: credentialsService,
		executionService:   executionService,
		logger:             logger,
	}
}

func (s *LLMScorer) Type() evaluation.ScorerType {
	return evaluation.ScorerTypeLLM
}

func (s *LLMScorer) Execute(ctx context.Context, job *EvaluationJob) (*ScorerResult, error) {
	// Parse config
	config, err := s.parseConfig(job.ScorerConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid LLM scorer config: %w", err)
	}

	credentialID := config.CredentialID
	keyConfig, err := s.credentialsService.GetDecryptedByID(ctx, credentialID, job.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	// Build the compiled prompt from messages
	compiledPrompt := s.buildPrompt(config.Messages, job.Variables)

	// Prepare prompt for execution
	promptResp := &prompt.PromptResponse{
		Type:      "chat",
		Template:  compiledPrompt,
		Variables: []string{}, // Variables already substituted
	}

	// Build model config
	modelConfig := &prompt.ModelConfig{
		Provider:     string(keyConfig.Provider),
		Model:        config.Model,
		Temperature:  &config.Temperature,
		APIKey:       keyConfig.APIKey,
		CredentialID: &credentialID,
	}

	if keyConfig.BaseURL != "" {
		modelConfig.ResolvedBaseURL = &keyConfig.BaseURL
	}
	if keyConfig.Config != nil {
		modelConfig.ProviderConfig = keyConfig.Config
	}
	if keyConfig.Headers != nil {
		modelConfig.CustomHeaders = keyConfig.Headers
	}

	// Execute the prompt
	execResp, err := s.executionService.Execute(ctx, promptResp, map[string]string{}, modelConfig)
	if err != nil {
		return nil, fmt.Errorf("LLM execution failed: %w", err)
	}

	if execResp.Error != "" {
		errStr := execResp.Error
		return &ScorerResult{
			Scores: []ScoreOutput{},
			Error:  &errStr,
		}, nil
	}

	if execResp.Response == nil || execResp.Response.Content == "" {
		return &ScorerResult{Scores: []ScoreOutput{}}, nil
	}

	// Parse the response
	scores, err := s.parseResponse(execResp.Response.Content, config.OutputSchema)
	if err != nil {
		s.logger.Warn("Failed to parse LLM response",
			"error", err,
			"job_id", job.JobID,
			"response", execResp.Response.Content,
		)
		// Return the raw response as a single score
		reason := "Failed to parse structured response: " + err.Error()
		return &ScorerResult{
			Scores: []ScoreOutput{
				{
					Name:        "llm_response",
					StringValue: &execResp.Response.Content,
					Type:        "CATEGORICAL",
					Reason:      &reason,
				},
			},
		}, nil
	}

	s.logger.Debug("LLM scorer executed",
		"job_id", job.JobID,
		"model", config.Model,
		"score_count", len(scores),
	)

	return &ScorerResult{Scores: scores}, nil
}

func (s *LLMScorer) parseConfig(config map[string]any) (*evaluation.LLMScorerConfig, error) {
	credentialIDStr, ok := config["credential_id"].(string)
	if !ok || credentialIDStr == "" {
		return nil, fmt.Errorf("credential_id is required")
	}
	credentialID, err := uuid.Parse(credentialIDStr)
	if err != nil {
		return nil, fmt.Errorf("credential_id must be a valid UUID: %w", err)
	}

	model, ok := config["model"].(string)
	if !ok || model == "" {
		return nil, fmt.Errorf("model is required")
	}

	temperature := 0.0
	if v, ok := config["temperature"].(float64); ok {
		temperature = v
	}

	responseFormat := "json"
	if v, ok := config["response_format"].(string); ok {
		responseFormat = v
	}

	// Parse messages
	var messages []evaluation.LLMMessage
	if rawMessages, ok := config["messages"].([]interface{}); ok {
		for _, rawMsg := range rawMessages {
			if msgMap, ok := rawMsg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)
				if role != "" && content != "" {
					messages = append(messages, evaluation.LLMMessage{
						Role:    role,
						Content: content,
					})
				}
			}
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	// Parse output schema
	var outputSchema []evaluation.OutputField
	if rawSchema, ok := config["output_schema"].([]interface{}); ok {
		for _, rawField := range rawSchema {
			if fieldMap, ok := rawField.(map[string]interface{}); ok {
				field := evaluation.OutputField{
					Name:        getString(fieldMap, "name"),
					Type:        getString(fieldMap, "type"),
					Description: getString(fieldMap, "description"),
				}
				if v, ok := fieldMap["min_value"].(float64); ok {
					field.MinValue = &v
				}
				if v, ok := fieldMap["max_value"].(float64); ok {
					field.MaxValue = &v
				}
				if cats, ok := fieldMap["categories"].([]interface{}); ok {
					for _, cat := range cats {
						if s, ok := cat.(string); ok {
							field.Categories = append(field.Categories, s)
						}
					}
				}
				if field.Name != "" {
					outputSchema = append(outputSchema, field)
				}
			}
		}
	}

	return &evaluation.LLMScorerConfig{
		CredentialID:   credentialID,
		Model:          model,
		Messages:       messages,
		Temperature:    temperature,
		ResponseFormat: responseFormat,
		OutputSchema:   outputSchema,
	}, nil
}

func (s *LLMScorer) buildPrompt(messages []evaluation.LLMMessage, variables map[string]string) string {
	var parts []string

	for _, msg := range messages {
		content := s.substituteVariables(msg.Content, variables)
		parts = append(parts, fmt.Sprintf("<%s>\n%s\n</%s>", msg.Role, content, msg.Role))
	}

	return strings.Join(parts, "\n\n")
}

func (s *LLMScorer) substituteVariables(template string, variables map[string]string) string {
	result := template
	for key, value := range variables {
		placeholder := "{" + key + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

func (s *LLMScorer) parseResponse(content string, schema []evaluation.OutputField) ([]ScoreOutput, error) {
	// Try to parse as JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		// Try to extract JSON from the response
		content = extractJSON(content)
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			return nil, fmt.Errorf("response is not valid JSON: %w", err)
		}
	}

	var scores []ScoreOutput

	// If we have a schema, use it to extract scores
	if len(schema) > 0 {
		for _, field := range schema {
			value, exists := parsed[field.Name]
			if !exists {
				continue
			}

			output := ScoreOutput{
				Name: field.Name,
				Type: s.mapFieldTypeToScoreType(field.Type),
			}

			switch field.Type {
			case "numeric":
				if v, ok := toFloat(value); ok {
					output.Value = &v
				}
			case "categorical":
				if v, ok := value.(string); ok {
					output.StringValue = &v
				}
			case "boolean":
				if v, ok := value.(bool); ok {
					boolVal := 0.0
					if v {
						boolVal = 1.0
					}
					output.Value = &boolVal
				}
			default:
				if v, ok := value.(string); ok {
					output.StringValue = &v
				} else if v, ok := toFloat(value); ok {
					output.Value = &v
				}
			}

			// Extract reason if present
			reasonKey := field.Name + "_reason"
			if reason, ok := parsed[reasonKey].(string); ok {
				output.Reason = &reason
			} else if reason, ok := parsed["reason"].(string); ok && len(schema) == 1 {
				output.Reason = &reason
			}

			scores = append(scores, output)
		}
	} else {
		// No schema - try to extract any score-like fields
		for key, value := range parsed {
			if strings.HasSuffix(key, "_reason") {
				continue
			}

			output := ScoreOutput{
				Name: key,
				Type: "NUMERIC",
			}

			if v, ok := toFloat(value); ok {
				output.Value = &v
			} else if v, ok := value.(string); ok {
				output.StringValue = &v
				output.Type = "CATEGORICAL"
			} else if v, ok := value.(bool); ok {
				boolVal := 0.0
				if v {
					boolVal = 1.0
				}
				output.Value = &boolVal
				output.Type = "BOOLEAN"
			} else {
				continue
			}

			// Check for associated reason
			reasonKey := key + "_reason"
			if reason, ok := parsed[reasonKey].(string); ok {
				output.Reason = &reason
			}

			scores = append(scores, output)
		}
	}

	return scores, nil
}

// mapFieldTypeToScoreType maps LLM output schema field types to standardized score types
func (s *LLMScorer) mapFieldTypeToScoreType(fieldType string) string {
	switch strings.ToLower(fieldType) {
	case "numeric":
		return "NUMERIC"
	case "categorical", "text":
		return "CATEGORICAL"
	case "boolean":
		return "BOOLEAN"
	default:
		return "NUMERIC" // Default to numeric
	}
}

// Helper functions

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}

// extractJSON attempts to extract a JSON object from a string that might contain
// additional text (like markdown code blocks or explanatory text)
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Try to find JSON in markdown code blocks
	if strings.Contains(s, "```json") {
		start := strings.Index(s, "```json") + 7
		end := strings.Index(s[start:], "```")
		if end > 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}

	if strings.Contains(s, "```") {
		start := strings.Index(s, "```") + 3
		// Skip optional language identifier
		if idx := strings.Index(s[start:], "\n"); idx >= 0 {
			start += idx + 1
		}
		end := strings.Index(s[start:], "```")
		if end > 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}

	// Try to find raw JSON object
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}

	return s
}
