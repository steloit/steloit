package evaluation

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func ptr[T any](v T) *T { return &v }

func TestBuiltinScorer_ParseConfig(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name       string
		config     map[string]any
		wantName   string
		wantErr    bool
		errContain string
	}{
		{
			name:     "valid config with scorer_name",
			config:   map[string]any{"scorer_name": "contains", "config": map[string]any{"substring": "test"}},
			wantName: "contains",
			wantErr:  false,
		},
		{
			name:     "valid config without inner config",
			config:   map[string]any{"scorer_name": "json_valid"},
			wantName: "json_valid",
			wantErr:  false,
		},
		{
			name:       "missing scorer_name",
			config:     map[string]any{"config": map[string]any{}},
			wantErr:    true,
			errContain: "scorer_name is required",
		},
		{
			name:       "empty scorer_name",
			config:     map[string]any{"scorer_name": ""},
			wantErr:    true,
			errContain: "scorer_name is required",
		},
		{
			name:       "scorer_name wrong type",
			config:     map[string]any{"scorer_name": 123},
			wantErr:    true,
			errContain: "scorer_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := scorer.parseConfig(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantName, cfg.ScorerName)
			}
		})
	}
}

func TestBuiltinScorer_GetTargetText(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name     string
		job      *EvaluationJob
		expected string
	}{
		{
			name:     "from variables output",
			job:      &EvaluationJob{Variables: map[string]string{"output": "output text"}},
			expected: "output text",
		},
		{
			name:     "from variables input when no output",
			job:      &EvaluationJob{Variables: map[string]string{"input": "input text"}},
			expected: "input text",
		},
		{
			name:     "prefer output over input",
			job:      &EvaluationJob{Variables: map[string]string{"output": "out", "input": "in"}},
			expected: "out",
		},
		{
			name:     "from span data output",
			job:      &EvaluationJob{SpanData: map[string]any{"output": "span output"}},
			expected: "span output",
		},
		{
			name:     "from span data input",
			job:      &EvaluationJob{SpanData: map[string]any{"input": "span input"}},
			expected: "span input",
		},
		{
			name:     "prefer variables over span data",
			job:      &EvaluationJob{Variables: map[string]string{"output": "var out"}, SpanData: map[string]any{"output": "span out"}},
			expected: "var out",
		},
		{
			name:     "empty when nothing available",
			job:      &EvaluationJob{},
			expected: "",
		},
		{
			name:     "empty variables fallback to span data",
			job:      &EvaluationJob{Variables: map[string]string{"output": ""}, SpanData: map[string]any{"output": "span output"}},
			expected: "span output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.getTargetText(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuiltinScorer_Contains(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		config    map[string]any
		wantScore float64
		wantErr   bool
	}{
		{
			name:      "contains match",
			text:      "hello world",
			config:    map[string]any{"substring": "world"},
			wantScore: 1.0,
		},
		{
			name:      "contains no match",
			text:      "hello world",
			config:    map[string]any{"substring": "foo"},
			wantScore: 0.0,
		},
		{
			name:      "case sensitive match",
			text:      "Hello World",
			config:    map[string]any{"substring": "Hello", "case_sensitive": true},
			wantScore: 1.0,
		},
		{
			name:      "case sensitive no match",
			text:      "Hello World",
			config:    map[string]any{"substring": "hello", "case_sensitive": true},
			wantScore: 0.0,
		},
		{
			name:      "case insensitive match",
			text:      "HELLO WORLD",
			config:    map[string]any{"substring": "hello", "case_sensitive": false},
			wantScore: 1.0,
		},
		{
			name:      "empty text",
			text:      "",
			config:    map[string]any{"substring": "test"},
			wantScore: 0.0,
		},
		{
			name:      "empty substring matches",
			text:      "hello",
			config:    map[string]any{"substring": ""},
			wantScore: 1.0,
		},
		{
			name:    "missing substring config",
			text:    "hello",
			config:  map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeContains(tt.text, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "BOOLEAN", result.Scores[0].Type)
		})
	}
}

func TestBuiltinScorer_NotContains(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		config    map[string]any
		wantScore float64
	}{
		{
			name:      "not contains when absent",
			text:      "hello world",
			config:    map[string]any{"substring": "foo"},
			wantScore: 1.0,
		},
		{
			name:      "not contains when present",
			text:      "hello world",
			config:    map[string]any{"substring": "world"},
			wantScore: 0.0,
		},
		{
			name:      "case insensitive not contains",
			text:      "HELLO",
			config:    map[string]any{"substring": "hello", "case_sensitive": false},
			wantScore: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeNotContains(tt.text, tt.config)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "not_contains", result.Scores[0].Name)
		})
	}
}

func TestBuiltinScorer_JSONValid(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		wantScore float64
	}{
		{
			name:      "valid object",
			text:      `{"key": "value"}`,
			wantScore: 1.0,
		},
		{
			name:      "valid array",
			text:      `[1, 2, 3]`,
			wantScore: 1.0,
		},
		{
			name:      "valid string",
			text:      `"hello"`,
			wantScore: 1.0,
		},
		{
			name:      "valid number",
			text:      `42`,
			wantScore: 1.0,
		},
		{
			name:      "valid BOOLEAN",
			text:      `true`,
			wantScore: 1.0,
		},
		{
			name:      "valid null",
			text:      `null`,
			wantScore: 1.0,
		},
		{
			name:      "invalid json - missing quote",
			text:      `{"key: "value"}`,
			wantScore: 0.0,
		},
		{
			name:      "invalid json - truncated",
			text:      `{"key":`,
			wantScore: 0.0,
		},
		{
			name:      "invalid json - plain text",
			text:      `hello world`,
			wantScore: 0.0,
		},
		{
			name:      "empty string",
			text:      ``,
			wantScore: 0.0,
		},
		{
			name:      "nested valid json",
			text:      `{"outer": {"inner": [1, 2, 3]}}`,
			wantScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeJSONValid(tt.text)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "json_valid", result.Scores[0].Name)
			assert.Equal(t, "BOOLEAN", result.Scores[0].Type)
		})
	}
}

func TestBuiltinScorer_LengthCheck(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		config    map[string]any
		wantScore float64
	}{
		{
			name:      "within bounds",
			text:      "hello",
			config:    map[string]any{"min_length": 1.0, "max_length": 10.0},
			wantScore: 1.0,
		},
		{
			name:      "too short",
			text:      "hi",
			config:    map[string]any{"min_length": 5.0},
			wantScore: 0.0,
		},
		{
			name:      "too long",
			text:      "hello world",
			config:    map[string]any{"max_length": 5.0},
			wantScore: 0.0,
		},
		{
			name:      "exact min length",
			text:      "hello",
			config:    map[string]any{"min_length": 5.0},
			wantScore: 1.0,
		},
		{
			name:      "exact max length",
			text:      "hello",
			config:    map[string]any{"max_length": 5.0},
			wantScore: 1.0,
		},
		{
			name:      "no max limit",
			text:      "a very long string that goes on and on",
			config:    map[string]any{"min_length": 1.0},
			wantScore: 1.0,
		},
		{
			name:      "no min limit",
			text:      "",
			config:    map[string]any{"max_length": 10.0},
			wantScore: 1.0,
		},
		{
			name:      "empty config defaults",
			text:      "hello",
			config:    map[string]any{},
			wantScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeLengthCheck(tt.text, tt.config)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "BOOLEAN", result.Scores[0].Type)
		})
	}
}

func TestBuiltinScorer_StartsWith(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		config    map[string]any
		wantScore float64
		wantErr   bool
	}{
		{
			name:      "starts with match",
			text:      "Hello World",
			config:    map[string]any{"prefix": "Hello"},
			wantScore: 1.0,
		},
		{
			name:      "starts with no match",
			text:      "Hello World",
			config:    map[string]any{"prefix": "World"},
			wantScore: 0.0,
		},
		{
			name:      "empty prefix always matches",
			text:      "hello",
			config:    map[string]any{"prefix": ""},
			wantScore: 1.0,
		},
		{
			name:      "case sensitive",
			text:      "Hello",
			config:    map[string]any{"prefix": "hello"},
			wantScore: 0.0,
		},
		{
			name:    "missing prefix config",
			text:    "hello",
			config:  map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeStartsWith(tt.text, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "starts_with", result.Scores[0].Name)
		})
	}
}

func TestBuiltinScorer_EndsWith(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		config    map[string]any
		wantScore float64
		wantErr   bool
	}{
		{
			name:      "ends with match",
			text:      "Hello World",
			config:    map[string]any{"suffix": "World"},
			wantScore: 1.0,
		},
		{
			name:      "ends with no match",
			text:      "Hello World",
			config:    map[string]any{"suffix": "Hello"},
			wantScore: 0.0,
		},
		{
			name:      "empty suffix always matches",
			text:      "hello",
			config:    map[string]any{"suffix": ""},
			wantScore: 1.0,
		},
		{
			name:      "case sensitive",
			text:      "Hello World",
			config:    map[string]any{"suffix": "world"},
			wantScore: 0.0,
		},
		{
			name:    "missing suffix config",
			text:    "hello",
			config:  map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeEndsWith(tt.text, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "ends_with", result.Scores[0].Name)
		})
	}
}

func TestBuiltinScorer_Equals(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		config    map[string]any
		wantScore float64
		wantErr   bool
	}{
		{
			name:      "equals exact match",
			text:      "hello",
			config:    map[string]any{"value": "hello"},
			wantScore: 1.0,
		},
		{
			name:      "equals no match",
			text:      "hello",
			config:    map[string]any{"value": "world"},
			wantScore: 0.0,
		},
		{
			name:      "case sensitive no match",
			text:      "Hello",
			config:    map[string]any{"value": "hello", "case_sensitive": true},
			wantScore: 0.0,
		},
		{
			name:      "case insensitive match",
			text:      "HELLO",
			config:    map[string]any{"value": "hello", "case_sensitive": false},
			wantScore: 1.0,
		},
		{
			name:      "empty equals empty",
			text:      "",
			config:    map[string]any{"value": ""},
			wantScore: 1.0,
		},
		{
			name:    "missing value config",
			text:    "hello",
			config:  map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeEquals(tt.text, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "equals", result.Scores[0].Name)
		})
	}
}

func TestBuiltinScorer_NotEmpty(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name      string
		text      string
		wantScore float64
	}{
		{
			name:      "not empty with content",
			text:      "hello",
			wantScore: 1.0,
		},
		{
			name:      "empty string",
			text:      "",
			wantScore: 0.0,
		},
		{
			name:      "whitespace only",
			text:      "   ",
			wantScore: 0.0,
		},
		{
			name:      "tabs and newlines only",
			text:      "\t\n\r",
			wantScore: 0.0,
		},
		{
			name:      "content with whitespace",
			text:      "  hello  ",
			wantScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.executeNotEmpty(tt.text)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
			assert.Equal(t, "not_empty", result.Scores[0].Name)
		})
	}
}

func TestBuiltinScorer_Execute(t *testing.T) {
	ctx := context.Background()
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name       string
		job        *EvaluationJob
		wantScore  float64
		wantErr    bool
		errContain string
	}{
		{
			name: "contains via execute",
			job: &EvaluationJob{
				ScorerConfig: map[string]any{
					"scorer_name": "contains",
					"config":      map[string]any{"substring": "hello"},
				},
				Variables: map[string]string{"output": "hello world"},
			},
			wantScore: 1.0,
		},
		{
			name: "json_valid via execute",
			job: &EvaluationJob{
				ScorerConfig: map[string]any{
					"scorer_name": "json_valid",
				},
				Variables: map[string]string{"output": `{"valid": true}`},
			},
			wantScore: 1.0,
		},
		{
			name: "not_empty via execute",
			job: &EvaluationJob{
				ScorerConfig: map[string]any{
					"scorer_name": "not_empty",
				},
				Variables: map[string]string{"output": "content"},
			},
			wantScore: 1.0,
		},
		{
			name: "unknown scorer",
			job: &EvaluationJob{
				ScorerConfig: map[string]any{
					"scorer_name": "unknown_scorer",
				},
			},
			wantErr:    true,
			errContain: "unknown builtin scorer",
		},
		{
			name: "missing scorer_name",
			job: &EvaluationJob{
				ScorerConfig: map[string]any{},
			},
			wantErr:    true,
			errContain: "scorer_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.Execute(ctx, tt.job)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
		})
	}
}

func TestBuiltinScorer_CustomScoreName(t *testing.T) {
	scorer := NewBuiltinScorer(newTestLogger())

	tests := []struct {
		name          string
		scorerType    string
		config        map[string]any
		wantScoreName string
	}{
		{
			name:          "contains custom name",
			scorerType:    "contains",
			config:        map[string]any{"substring": "test", "score_name": "has_test"},
			wantScoreName: "has_test",
		},
		{
			name:          "length_check custom name",
			scorerType:    "length_check",
			config:        map[string]any{"min_length": 1.0, "score_name": "valid_length"},
			wantScoreName: "valid_length",
		},
		{
			name:          "starts_with custom name",
			scorerType:    "starts_with",
			config:        map[string]any{"prefix": "test", "score_name": "starts_test"},
			wantScoreName: "starts_test",
		},
		{
			name:          "ends_with custom name",
			scorerType:    "ends_with",
			config:        map[string]any{"suffix": "test", "score_name": "ends_test"},
			wantScoreName: "ends_test",
		},
		{
			name:          "equals custom name",
			scorerType:    "equals",
			config:        map[string]any{"value": "test", "score_name": "is_test"},
			wantScoreName: "is_test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			job := &EvaluationJob{
				ScorerConfig: map[string]any{
					"scorer_name": tt.scorerType,
					"config":      tt.config,
				},
				Variables: map[string]string{"output": "test content"},
			}

			result, err := scorer.Execute(ctx, job)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScoreName, result.Scores[0].Name)
		})
	}
}
