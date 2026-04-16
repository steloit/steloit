package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"brokle/internal/core/domain/evaluation"
)

// BuiltinScorer implements pre-defined scoring functions
type BuiltinScorer struct {
	logger *slog.Logger
}

// NewBuiltinScorer creates a new builtin scorer
func NewBuiltinScorer(logger *slog.Logger) *BuiltinScorer {
	return &BuiltinScorer{
		logger: logger,
	}
}

func (s *BuiltinScorer) Type() evaluation.ScorerType {
	return evaluation.ScorerTypeBuiltin
}

func (s *BuiltinScorer) Execute(ctx context.Context, job *EvaluationJob) (*ScorerResult, error) {
	config, err := s.parseConfig(job.ScorerConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid builtin scorer config: %w", err)
	}

	text := s.getTargetText(job)

	switch config.ScorerName {
	case "contains":
		return s.executeContains(text, config.Config)
	case "not_contains":
		return s.executeNotContains(text, config.Config)
	case "json_valid":
		return s.executeJSONValid(text)
	case "length_check":
		return s.executeLengthCheck(text, config.Config)
	case "starts_with":
		return s.executeStartsWith(text, config.Config)
	case "ends_with":
		return s.executeEndsWith(text, config.Config)
	case "equals":
		return s.executeEquals(text, config.Config)
	case "not_empty":
		return s.executeNotEmpty(text)
	default:
		return nil, fmt.Errorf("unknown builtin scorer: %s", config.ScorerName)
	}
}

func (s *BuiltinScorer) parseConfig(config map[string]any) (*evaluation.BuiltinScorerConfig, error) {
	scorerName, ok := config["scorer_name"].(string)
	if !ok || scorerName == "" {
		return nil, fmt.Errorf("scorer_name is required")
	}

	innerConfig := make(map[string]any)
	if cfg, ok := config["config"].(map[string]any); ok {
		innerConfig = cfg
	}

	return &evaluation.BuiltinScorerConfig{
		ScorerName: scorerName,
		Config:     innerConfig,
	}, nil
}

func (s *BuiltinScorer) getTargetText(job *EvaluationJob) string {
	// Check variables first
	if output, ok := job.Variables["output"]; ok && output != "" {
		return output
	}
	if input, ok := job.Variables["input"]; ok && input != "" {
		return input
	}

	// Fall back to span data
	if output, ok := job.SpanData["output"].(string); ok {
		return output
	}
	if input, ok := job.SpanData["input"].(string); ok {
		return input
	}

	return ""
}

// executeContains checks if text contains a substring
func (s *BuiltinScorer) executeContains(text string, config map[string]any) (*ScorerResult, error) {
	substring, ok := config["substring"].(string)
	if !ok {
		return nil, fmt.Errorf("substring is required for contains scorer")
	}

	caseSensitive := true
	if cs, ok := config["case_sensitive"].(bool); ok {
		caseSensitive = cs
	}

	scoreName := "contains"
	if name, ok := config["score_name"].(string); ok {
		scoreName = name
	}

	var contains bool
	if caseSensitive {
		contains = strings.Contains(text, substring)
	} else {
		contains = strings.Contains(strings.ToLower(text), strings.ToLower(substring))
	}

	value := 0.0
	if contains {
		value = 1.0
	}

	reason := fmt.Sprintf("Text %s '%s'",
		map[bool]string{true: "contains", false: "does not contain"}[contains],
		substring)

	return &ScorerResult{
		Scores: []ScoreOutput{
			{
				Name:   scoreName,
				Value:  &value,
				Type:   "BOOLEAN",
				Reason: &reason,
			},
		},
	}, nil
}

// executeNotContains checks if text does not contain a substring
func (s *BuiltinScorer) executeNotContains(text string, config map[string]any) (*ScorerResult, error) {
	result, err := s.executeContains(text, config)
	if err != nil {
		return nil, err
	}

	// Invert the result
	if result.Scores[0].Value != nil {
		inverted := 1.0 - *result.Scores[0].Value
		result.Scores[0].Value = &inverted
	}

	if name, ok := config["score_name"].(string); ok {
		result.Scores[0].Name = name
	} else {
		result.Scores[0].Name = "not_contains"
	}

	return result, nil
}

// executeJSONValid checks if text is valid JSON
func (s *BuiltinScorer) executeJSONValid(text string) (*ScorerResult, error) {
	var js json.RawMessage
	err := json.Unmarshal([]byte(text), &js)

	value := 0.0
	var reason string
	if err == nil {
		value = 1.0
		reason = "Valid JSON"
	} else {
		reason = fmt.Sprintf("Invalid JSON: %s", err.Error())
	}

	return &ScorerResult{
		Scores: []ScoreOutput{
			{
				Name:   "json_valid",
				Value:  &value,
				Type:   "BOOLEAN",
				Reason: &reason,
			},
		},
	}, nil
}

// executeLengthCheck checks if text length is within bounds
func (s *BuiltinScorer) executeLengthCheck(text string, config map[string]any) (*ScorerResult, error) {
	length := len(text)

	minLen := 0
	if v, ok := config["min_length"].(float64); ok {
		minLen = int(v)
	}

	maxLen := -1 // -1 means no max
	if v, ok := config["max_length"].(float64); ok {
		maxLen = int(v)
	}

	scoreName := "length_check"
	if name, ok := config["score_name"].(string); ok {
		scoreName = name
	}

	valid := length >= minLen && (maxLen < 0 || length <= maxLen)

	value := 0.0
	if valid {
		value = 1.0
	}

	reason := fmt.Sprintf("Length %d (min: %d, max: %d)", length, minLen, maxLen)

	return &ScorerResult{
		Scores: []ScoreOutput{
			{
				Name:   scoreName,
				Value:  &value,
				Type:   "BOOLEAN",
				Reason: &reason,
			},
		},
	}, nil
}

// executeStartsWith checks if text starts with a prefix
func (s *BuiltinScorer) executeStartsWith(text string, config map[string]any) (*ScorerResult, error) {
	prefix, ok := config["prefix"].(string)
	if !ok {
		return nil, fmt.Errorf("prefix is required for starts_with scorer")
	}

	scoreName := "starts_with"
	if name, ok := config["score_name"].(string); ok {
		scoreName = name
	}

	startsWith := strings.HasPrefix(text, prefix)

	value := 0.0
	if startsWith {
		value = 1.0
	}

	reason := fmt.Sprintf("Text %s with '%s'",
		map[bool]string{true: "starts", false: "does not start"}[startsWith],
		prefix)

	return &ScorerResult{
		Scores: []ScoreOutput{
			{
				Name:   scoreName,
				Value:  &value,
				Type:   "BOOLEAN",
				Reason: &reason,
			},
		},
	}, nil
}

// executeEndsWith checks if text ends with a suffix
func (s *BuiltinScorer) executeEndsWith(text string, config map[string]any) (*ScorerResult, error) {
	suffix, ok := config["suffix"].(string)
	if !ok {
		return nil, fmt.Errorf("suffix is required for ends_with scorer")
	}

	scoreName := "ends_with"
	if name, ok := config["score_name"].(string); ok {
		scoreName = name
	}

	endsWith := strings.HasSuffix(text, suffix)

	value := 0.0
	if endsWith {
		value = 1.0
	}

	reason := fmt.Sprintf("Text %s with '%s'",
		map[bool]string{true: "ends", false: "does not end"}[endsWith],
		suffix)

	return &ScorerResult{
		Scores: []ScoreOutput{
			{
				Name:   scoreName,
				Value:  &value,
				Type:   "BOOLEAN",
				Reason: &reason,
			},
		},
	}, nil
}

// executeEquals checks if text equals a value
func (s *BuiltinScorer) executeEquals(text string, config map[string]any) (*ScorerResult, error) {
	expected, ok := config["value"].(string)
	if !ok {
		return nil, fmt.Errorf("value is required for equals scorer")
	}

	caseSensitive := true
	if cs, ok := config["case_sensitive"].(bool); ok {
		caseSensitive = cs
	}

	scoreName := "equals"
	if name, ok := config["score_name"].(string); ok {
		scoreName = name
	}

	var equals bool
	if caseSensitive {
		equals = text == expected
	} else {
		equals = strings.EqualFold(text, expected)
	}

	value := 0.0
	if equals {
		value = 1.0
	}

	reason := fmt.Sprintf("Text %s expected value",
		map[bool]string{true: "equals", false: "does not equal"}[equals])

	return &ScorerResult{
		Scores: []ScoreOutput{
			{
				Name:   scoreName,
				Value:  &value,
				Type:   "BOOLEAN",
				Reason: &reason,
			},
		},
	}, nil
}

// executeNotEmpty checks if text is not empty
func (s *BuiltinScorer) executeNotEmpty(text string) (*ScorerResult, error) {
	notEmpty := strings.TrimSpace(text) != ""

	value := 0.0
	if notEmpty {
		value = 1.0
	}

	reason := fmt.Sprintf("Text is %s",
		map[bool]string{true: "not empty", false: "empty"}[notEmpty])

	return &ScorerResult{
		Scores: []ScoreOutput{
			{
				Name:   "not_empty",
				Value:  &value,
				Type:   "BOOLEAN",
				Reason: &reason,
			},
		},
	}, nil
}
