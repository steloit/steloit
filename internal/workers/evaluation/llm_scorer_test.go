package evaluation

import (
	"testing"

	"brokle/internal/core/domain/evaluation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMScorer_ParseConfig(t *testing.T) {
	scorer := &LLMScorer{logger: newTestLogger()}

	tests := []struct {
		name       string
		config     map[string]any
		wantCredID string
		wantModel  string
		wantMsgCnt int
		wantTemp   float64
		wantFormat string
		wantSchema int
		wantErr    bool
		errContain string
	}{
		{
			name: "valid minimal config",
			config: map[string]any{
				"credential_id": "01HNPXYZ123456789ABCDEFGH",
				"model":         "gpt-4o",
				"messages": []any{
					map[string]any{"role": "system", "content": "You are a judge."},
				},
			},
			wantCredID: "01HNPXYZ123456789ABCDEFGH",
			wantModel:  "gpt-4o",
			wantMsgCnt: 1,
			wantTemp:   0.0,
			wantFormat: "json",
		},
		{
			name: "valid full config",
			config: map[string]any{
				"credential_id":   "01HNPXYZ123456789ABCDEFGH",
				"model":           "gpt-4o",
				"temperature":     0.7,
				"response_format": "text",
				"messages": []any{
					map[string]any{"role": "system", "content": "System prompt"},
					map[string]any{"role": "user", "content": "User prompt {input}"},
				},
				"output_schema": []any{
					map[string]any{
						"name":      "score",
						"type":      "numeric",
						"min_value": 0.0,
						"max_value": 10.0,
					},
				},
			},
			wantCredID: "01HNPXYZ123456789ABCDEFGH",
			wantModel:  "gpt-4o",
			wantMsgCnt: 2,
			wantTemp:   0.7,
			wantFormat: "text",
			wantSchema: 1,
		},
		{
			name: "missing credential_id",
			config: map[string]any{
				"model": "gpt-4o",
				"messages": []any{
					map[string]any{"role": "user", "content": "test"},
				},
			},
			wantErr:    true,
			errContain: "credential_id is required",
		},
		{
			name: "missing model",
			config: map[string]any{
				"credential_id": "01HNPXYZ123456789ABCDEFGH",
				"messages": []any{
					map[string]any{"role": "user", "content": "test"},
				},
			},
			wantErr:    true,
			errContain: "model is required",
		},
		{
			name: "missing messages",
			config: map[string]any{
				"credential_id": "01HNPXYZ123456789ABCDEFGH",
				"model":         "gpt-4o",
			},
			wantErr:    true,
			errContain: "at least one message is required",
		},
		{
			name: "empty messages array",
			config: map[string]any{
				"credential_id": "01HNPXYZ123456789ABCDEFGH",
				"model":         "gpt-4o",
				"messages":      []any{},
			},
			wantErr:    true,
			errContain: "at least one message is required",
		},
		{
			name: "message with empty role ignored",
			config: map[string]any{
				"credential_id": "01HNPXYZ123456789ABCDEFGH",
				"model":         "gpt-4o",
				"messages": []any{
					map[string]any{"role": "", "content": "test"},
				},
			},
			wantErr:    true,
			errContain: "at least one message is required",
		},
		{
			name: "output schema with categories",
			config: map[string]any{
				"credential_id": "01HNPXYZ123456789ABCDEFGH",
				"model":         "gpt-4o",
				"messages": []any{
					map[string]any{"role": "user", "content": "test"},
				},
				"output_schema": []any{
					map[string]any{
						"name":       "quality",
						"type":       "categorical",
						"categories": []any{"good", "bad", "neutral"},
					},
				},
			},
			wantCredID: "01HNPXYZ123456789ABCDEFGH",
			wantModel:  "gpt-4o",
			wantMsgCnt: 1,
			wantFormat: "json",
			wantSchema: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := scorer.parseConfig(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCredID, cfg.CredentialID)
			assert.Equal(t, tt.wantModel, cfg.Model)
			assert.Len(t, cfg.Messages, tt.wantMsgCnt)
			assert.Equal(t, tt.wantTemp, cfg.Temperature)
			assert.Equal(t, tt.wantFormat, cfg.ResponseFormat)
			assert.Len(t, cfg.OutputSchema, tt.wantSchema)
		})
	}
}

func TestLLMScorer_BuildPrompt(t *testing.T) {
	scorer := &LLMScorer{logger: newTestLogger()}

	tests := []struct {
		name      string
		messages  []evaluation.LLMMessage
		variables map[string]string
		expected  string
	}{
		{
			name: "single message",
			messages: []evaluation.LLMMessage{
				{Role: "user", Content: "Hello world"},
			},
			variables: map[string]string{},
			expected:  "<user>\nHello world\n</user>",
		},
		{
			name: "multiple messages",
			messages: []evaluation.LLMMessage{
				{Role: "system", Content: "Be helpful"},
				{Role: "user", Content: "Question?"},
			},
			variables: map[string]string{},
			expected:  "<system>\nBe helpful\n</system>\n\n<user>\nQuestion?\n</user>",
		},
		{
			name: "with variable substitution",
			messages: []evaluation.LLMMessage{
				{Role: "user", Content: "Evaluate: {input}"},
			},
			variables: map[string]string{"input": "test content"},
			expected:  "<user>\nEvaluate: test content\n</user>",
		},
		{
			name: "multiple variables",
			messages: []evaluation.LLMMessage{
				{Role: "user", Content: "Input: {input}\nOutput: {output}"},
			},
			variables: map[string]string{"input": "query", "output": "response"},
			expected:  "<user>\nInput: query\nOutput: response\n</user>",
		},
		{
			name: "three roles",
			messages: []evaluation.LLMMessage{
				{Role: "system", Content: "You are a judge"},
				{Role: "user", Content: "Rate this"},
				{Role: "assistant", Content: "I will rate it"},
			},
			variables: map[string]string{},
			expected:  "<system>\nYou are a judge\n</system>\n\n<user>\nRate this\n</user>\n\n<assistant>\nI will rate it\n</assistant>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.buildPrompt(tt.messages, tt.variables)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLLMScorer_SubstituteVariables(t *testing.T) {
	scorer := &LLMScorer{logger: newTestLogger()}

	tests := []struct {
		name      string
		template  string
		variables map[string]string
		expected  string
	}{
		{
			name:      "no variables in template",
			template:  "Hello world",
			variables: map[string]string{"name": "Alice"},
			expected:  "Hello world",
		},
		{
			name:      "single variable",
			template:  "Hello {name}",
			variables: map[string]string{"name": "Alice"},
			expected:  "Hello Alice",
		},
		{
			name:      "multiple different variables",
			template:  "{greeting} {name}!",
			variables: map[string]string{"greeting": "Hi", "name": "Bob"},
			expected:  "Hi Bob!",
		},
		{
			name:      "repeated variable",
			template:  "{x} + {x} = 2 * {x}",
			variables: map[string]string{"x": "5"},
			expected:  "5 + 5 = 2 * 5",
		},
		{
			name:      "missing variable unchanged",
			template:  "Hello {missing}",
			variables: map[string]string{},
			expected:  "Hello {missing}",
		},
		{
			name:      "variable with special characters",
			template:  "Value: {data}",
			variables: map[string]string{"data": "a\nb\tc"},
			expected:  "Value: a\nb\tc",
		},
		{
			name:      "variable with braces in value",
			template:  "JSON: {json}",
			variables: map[string]string{"json": `{"key": "value"}`},
			expected:  `JSON: {"key": "value"}`,
		},
		{
			name:      "empty variable value",
			template:  "Value: {empty}",
			variables: map[string]string{"empty": ""},
			expected:  "Value: ",
		},
		{
			name:      "nil map",
			template:  "Hello {name}",
			variables: nil,
			expected:  "Hello {name}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.substituteVariables(tt.template, tt.variables)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLLMScorer_ParseResponse(t *testing.T) {
	scorer := &LLMScorer{logger: newTestLogger()}

	tests := []struct {
		name       string
		content    string
		schema     []evaluation.OutputField
		wantScores int
		wantErr    bool
		validate   func(t *testing.T, scores []ScoreOutput)
	}{
		{
			name:       "simple NUMERIC json without schema",
			content:    `{"score": 8.5}`,
			schema:     nil,
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, "score", scores[0].Name)
				assert.Equal(t, 8.5, *scores[0].Value)
				assert.Equal(t, "NUMERIC", scores[0].Type)
			},
		},
		{
			name:    "json with schema",
			content: `{"relevance": 7, "coherence": 9}`,
			schema: []evaluation.OutputField{
				{Name: "relevance", Type: "numeric"},
				{Name: "coherence", Type: "numeric"},
			},
			wantScores: 2,
			validate: func(t *testing.T, scores []ScoreOutput) {
				scoreMap := make(map[string]float64)
				for _, s := range scores {
					scoreMap[s.Name] = *s.Value
				}
				assert.Equal(t, 7.0, scoreMap["relevance"])
				assert.Equal(t, 9.0, scoreMap["coherence"])
			},
		},
		{
			name:       "json in markdown code block",
			content:    "Here is my evaluation:\n```json\n{\"score\": 5}\n```",
			schema:     nil,
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 5.0, *scores[0].Value)
			},
		},
		{
			name:       "json in generic code block",
			content:    "```\n{\"score\": 5}\n```",
			schema:     nil,
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 5.0, *scores[0].Value)
			},
		},
		{
			name:       "json with surrounding text",
			content:    "Here is my analysis:\n{\"score\": 5}\nThat's my answer.",
			schema:     nil,
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 5.0, *scores[0].Value)
			},
		},
		{
			name:    "BOOLEAN value with schema",
			content: `{"is_valid": true}`,
			schema: []evaluation.OutputField{
				{Name: "is_valid", Type: "boolean"},
			},
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 1.0, *scores[0].Value)
				assert.Equal(t, "BOOLEAN", scores[0].Type)
			},
		},
		{
			name:    "BOOLEAN false value",
			content: `{"is_valid": false}`,
			schema: []evaluation.OutputField{
				{Name: "is_valid", Type: "boolean"},
			},
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 0.0, *scores[0].Value)
			},
		},
		{
			name:    "CATEGORICAL value",
			content: `{"quality": "good"}`,
			schema: []evaluation.OutputField{
				{Name: "quality", Type: "categorical"},
			},
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, "good", *scores[0].StringValue)
				assert.Equal(t, "CATEGORICAL", scores[0].Type)
			},
		},
		{
			name:    "with reason field",
			content: `{"score": 8, "score_reason": "Well written"}`,
			schema: []evaluation.OutputField{
				{Name: "score", Type: "numeric"},
			},
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 8.0, *scores[0].Value)
				assert.NotNil(t, scores[0].Reason)
				assert.Equal(t, "Well written", *scores[0].Reason)
			},
		},
		{
			name:    "generic reason for single schema field",
			content: `{"score": 8, "reason": "Good quality"}`,
			schema: []evaluation.OutputField{
				{Name: "score", Type: "numeric"},
			},
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, "Good quality", *scores[0].Reason)
			},
		},
		{
			name:    "integer value parses to float",
			content: `{"count": 42}`,
			schema:  nil,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 42.0, *scores[0].Value)
			},
		},
		{
			name:       "string value without schema",
			content:    `{"category": "excellent"}`,
			schema:     nil,
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, "excellent", *scores[0].StringValue)
				assert.Equal(t, "CATEGORICAL", scores[0].Type)
			},
		},
		{
			name:       "BOOLEAN without schema",
			content:    `{"passed": true}`,
			schema:     nil,
			wantScores: 1,
			validate: func(t *testing.T, scores []ScoreOutput) {
				assert.Equal(t, 1.0, *scores[0].Value)
				assert.Equal(t, "BOOLEAN", scores[0].Type)
			},
		},
		{
			name:    "reason field skipped in output",
			content: `{"score": 5, "score_reason": "test reason"}`,
			schema:  nil,
			validate: func(t *testing.T, scores []ScoreOutput) {
				// Only score field should be output, reason attached to it
				for _, s := range scores {
					assert.NotEqual(t, "score_reason", s.Name)
				}
			},
		},
		{
			name:    "invalid json",
			content: `{invalid json`,
			schema:  nil,
			wantErr: true,
		},
		{
			name:    "empty content",
			content: ``,
			schema:  nil,
			wantErr: true,
		},
		{
			name:    "non-json text",
			content: `This is just plain text with no JSON`,
			schema:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scores, err := scorer.parseResponse(tt.content, tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantScores > 0 {
				assert.Len(t, scores, tt.wantScores)
			}
			if tt.validate != nil {
				tt.validate(t, scores)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "pure json",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "json in json code block",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "json in generic code block",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "json with prefix text",
			input:    "Here is the result: {\"key\": \"value\"}",
			expected: `{"key": "value"}`,
		},
		{
			name:     "json with suffix text",
			input:    "{\"key\": \"value\"} - that's my answer",
			expected: `{"key": "value"}`,
		},
		{
			name:     "json with both prefix and suffix",
			input:    "Analysis: {\"score\": 5} Done.",
			expected: `{"score": 5}`,
		},
		{
			name:     "whitespace around json",
			input:    "  {\"key\": \"value\"}  ",
			expected: `{"key": "value"}`,
		},
		{
			name:     "no json present",
			input:    "no json here",
			expected: "no json here",
		},
		{
			name:     "nested json",
			input:    `{"outer": {"inner": "value"}}`,
			expected: `{"outer": {"inner": "value"}}`,
		},
		{
			name:     "json code block with language on same line",
			input:    "```json{\"key\": \"value\"}```",
			expected: `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		want   float64
		wantOK bool
	}{
		{
			name:   "float64",
			input:  float64(3.14),
			want:   3.14,
			wantOK: true,
		},
		{
			name:   "float32",
			input:  float32(2.5),
			want:   2.5,
			wantOK: true,
		},
		{
			name:   "int",
			input:  int(42),
			want:   42.0,
			wantOK: true,
		},
		{
			name:   "int64",
			input:  int64(100),
			want:   100.0,
			wantOK: true,
		},
		{
			name:   "int32",
			input:  int32(50),
			want:   50.0,
			wantOK: true,
		},
		{
			name:   "string not supported",
			input:  "42",
			want:   0,
			wantOK: false,
		},
		{
			name:   "nil",
			input:  nil,
			want:   0,
			wantOK: false,
		},
		{
			name:   "bool not supported",
			input:  true,
			want:   0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected string
	}{
		{
			name:     "string value",
			m:        map[string]any{"name": "test"},
			key:      "name",
			expected: "test",
		},
		{
			name:     "number value returns empty",
			m:        map[string]any{"count": 42},
			key:      "count",
			expected: "",
		},
		{
			name:     "missing key",
			m:        map[string]any{"other": "value"},
			key:      "missing",
			expected: "",
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "any",
			expected: "",
		},
		{
			name:     "empty string value",
			m:        map[string]any{"empty": ""},
			key:      "empty",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getString(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLLMScorer_Type(t *testing.T) {
	scorer := &LLMScorer{logger: newTestLogger()}
	assert.Equal(t, evaluation.ScorerTypeLLM, scorer.Type())
}
