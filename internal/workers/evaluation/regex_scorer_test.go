package evaluation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegexScorer_ParseConfig(t *testing.T) {
	scorer := NewRegexScorer(newTestLogger())

	tests := []struct {
		name        string
		config      map[string]any
		wantPattern string
		wantName    string
		wantMatch   float64
		wantNoMatch float64
		wantCapture *int
		wantErr     bool
		errContain  string
	}{
		{
			name:        "minimal config",
			config:      map[string]any{"pattern": "test"},
			wantPattern: "test",
			wantName:    "regex_match",
			wantMatch:   1.0,
			wantNoMatch: 0.0,
		},
		{
			name: "full config",
			config: map[string]any{
				"pattern":        "test",
				"score_name":     "custom_score",
				"match_score":    10.0,
				"no_match_score": -5.0,
				"capture_group":  1.0,
			},
			wantPattern: "test",
			wantName:    "custom_score",
			wantMatch:   10.0,
			wantNoMatch: -5.0,
			wantCapture: ptr(1),
		},
		{
			name:       "missing pattern",
			config:     map[string]any{},
			wantErr:    true,
			errContain: "pattern is required",
		},
		{
			name:       "empty pattern",
			config:     map[string]any{"pattern": ""},
			wantErr:    true,
			errContain: "pattern is required",
		},
		{
			name:       "pattern wrong type",
			config:     map[string]any{"pattern": 123},
			wantErr:    true,
			errContain: "pattern is required",
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
			assert.Equal(t, tt.wantPattern, cfg.Pattern)
			assert.Equal(t, tt.wantName, cfg.ScoreName)
			assert.Equal(t, tt.wantMatch, cfg.MatchScore)
			assert.Equal(t, tt.wantNoMatch, cfg.NoMatchScore)
			if tt.wantCapture != nil {
				require.NotNil(t, cfg.CaptureGroup)
				assert.Equal(t, *tt.wantCapture, *cfg.CaptureGroup)
			} else {
				assert.Nil(t, cfg.CaptureGroup)
			}
		})
	}
}

func TestRegexScorer_GetTargetText(t *testing.T) {
	scorer := NewRegexScorer(newTestLogger())

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
			name:     "empty when nothing available",
			job:      &EvaluationJob{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.getTargetText(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegexScorer_PatternMatching(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	tests := []struct {
		name      string
		pattern   string
		text      string
		wantScore float64
		wantMatch bool
	}{
		{
			name:      "simple match",
			pattern:   "hello",
			text:      "hello world",
			wantScore: 1.0,
			wantMatch: true,
		},
		{
			name:      "simple no match",
			pattern:   "goodbye",
			text:      "hello world",
			wantScore: 0.0,
			wantMatch: false,
		},
		{
			name:      "case sensitive no match",
			pattern:   "Hello",
			text:      "hello world",
			wantScore: 0.0,
			wantMatch: false,
		},
		{
			name:      "case insensitive pattern",
			pattern:   "(?i)hello",
			text:      "HELLO world",
			wantScore: 1.0,
			wantMatch: true,
		},
		{
			name:      "regex anchor start",
			pattern:   "^hello",
			text:      "hello world",
			wantScore: 1.0,
			wantMatch: true,
		},
		{
			name:      "regex anchor end",
			pattern:   "world$",
			text:      "hello world",
			wantScore: 1.0,
			wantMatch: true,
		},
		{
			name:      "regex anchor no match",
			pattern:   "^world",
			text:      "hello world",
			wantScore: 0.0,
			wantMatch: false,
		},
		{
			name:      "word boundary",
			pattern:   `\bworld\b`,
			text:      "hello world today",
			wantScore: 1.0,
			wantMatch: true,
		},
		{
			name:      "digit pattern",
			pattern:   `\d+`,
			text:      "count: 42",
			wantScore: 1.0,
			wantMatch: true,
		},
		{
			name:      "email pattern",
			pattern:   `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
			text:      "contact us at test@example.com for more info",
			wantScore: 1.0,
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &EvaluationJob{
				ScorerConfig: map[string]any{"pattern": tt.pattern},
				Variables:    map[string]string{"output": tt.text},
			}

			result, err := scorer.Execute(ctx, job)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
		})
	}
}

func TestRegexScorer_CustomScores(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	tests := []struct {
		name         string
		matchScore   float64
		noMatchScore float64
		text         string
		pattern      string
		wantScore    float64
	}{
		{
			name:       "custom match score",
			matchScore: 10.0,
			text:       "hello world",
			pattern:    "hello",
			wantScore:  10.0,
		},
		{
			name:         "custom no match score",
			noMatchScore: -5.0,
			text:         "hello world",
			pattern:      "goodbye",
			wantScore:    -5.0,
		},
		{
			name:         "both custom scores - match",
			matchScore:   100.0,
			noMatchScore: -100.0,
			text:         "hello world",
			pattern:      "hello",
			wantScore:    100.0,
		},
		{
			name:         "both custom scores - no match",
			matchScore:   100.0,
			noMatchScore: -100.0,
			text:         "hello world",
			pattern:      "goodbye",
			wantScore:    -100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{"pattern": tt.pattern}
			if tt.matchScore != 0 {
				config["match_score"] = tt.matchScore
			}
			if tt.noMatchScore != 0 {
				config["no_match_score"] = tt.noMatchScore
			}

			job := &EvaluationJob{
				ScorerConfig: config,
				Variables:    map[string]string{"output": tt.text},
			}

			result, err := scorer.Execute(ctx, job)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)
		})
	}
}

func TestRegexScorer_CaptureGroups(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	tests := []struct {
		name           string
		pattern        string
		captureGroup   int
		text           string
		wantScore      float64
		wantString     string
		wantNumericVal bool
	}{
		{
			name:         "capture string value",
			pattern:      `name: (\w+)`,
			captureGroup: 1,
			text:         "name: Alice",
			wantScore:    1.0,
			wantString:   "Alice",
		},
		{
			name:           "capture numeric value",
			pattern:        `count: (\d+)`,
			captureGroup:   1,
			text:           "count: 42",
			wantScore:      42.0,
			wantString:     "42",
			wantNumericVal: true,
		},
		{
			name:           "capture float value",
			pattern:        `score: (\d+\.\d+)`,
			captureGroup:   1,
			text:           "score: 8.5",
			wantScore:      8.5,
			wantString:     "8.5",
			wantNumericVal: true,
		},
		{
			name:         "capture group 0 (full match)",
			pattern:      `\d+`,
			captureGroup: 0,
			text:         "value: 123",
			wantScore:    123.0,
			wantString:   "123",
		},
		{
			name:         "second capture group",
			pattern:      `(\w+): (\w+)`,
			captureGroup: 2,
			text:         "key: value",
			wantScore:    1.0,
			wantString:   "value",
		},
		{
			name:         "capture group out of range uses default score",
			pattern:      `(\w+)`,
			captureGroup: 5,
			text:         "test",
			wantScore:    1.0,
			wantString:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &EvaluationJob{
				ScorerConfig: map[string]any{
					"pattern":       tt.pattern,
					"capture_group": float64(tt.captureGroup),
				},
				Variables: map[string]string{"output": tt.text},
			}

			result, err := scorer.Execute(ctx, job)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			assert.Equal(t, tt.wantScore, *result.Scores[0].Value)

			if tt.wantString != "" {
				require.NotNil(t, result.Scores[0].StringValue)
				assert.Equal(t, tt.wantString, *result.Scores[0].StringValue)
			}
		})
	}
}

func TestRegexScorer_ReDoSProtection(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	tests := []struct {
		name       string
		pattern    string
		wantErr    bool
		errContain string
	}{
		{
			name:       "pattern too long",
			pattern:    strings.Repeat("a", 201),
			wantErr:    true,
			errContain: "too long",
		},
		{
			name:    "exactly max length is ok",
			pattern: strings.Repeat("a", 200),
			wantErr: false,
		},
		{
			name:       "too many wildcards",
			pattern:    strings.Repeat("a*", 11),
			wantErr:    true,
			errContain: "too complex",
		},
		{
			name:    "acceptable wildcards",
			pattern: "a*b*c*",
			wantErr: false,
		},
		{
			name:       "invalid regex syntax",
			pattern:    "[invalid(",
			wantErr:    true,
			errContain: "invalid regex",
		},
		{
			name:       "unclosed bracket",
			pattern:    "(unclosed",
			wantErr:    true,
			errContain: "invalid regex",
		},
		{
			name:    "valid complex pattern",
			pattern: `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &EvaluationJob{
				ScorerConfig: map[string]any{"pattern": tt.pattern},
				Variables:    map[string]string{"output": "test text"},
			}

			result, err := scorer.Execute(ctx, job)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestRegexScorer_EmptyText(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	job := &EvaluationJob{
		ScorerConfig: map[string]any{"pattern": "test"},
		Variables:    map[string]string{},
		SpanData:     map[string]any{},
	}

	result, err := scorer.Execute(ctx, job)
	require.NoError(t, err)
	assert.Empty(t, result.Scores, "expected no scores for empty text")
}

func TestRegexScorer_CustomScoreName(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	job := &EvaluationJob{
		ScorerConfig: map[string]any{
			"pattern":    "test",
			"score_name": "custom_regex_score",
		},
		Variables: map[string]string{"output": "this is a test"},
	}

	result, err := scorer.Execute(ctx, job)
	require.NoError(t, err)
	require.Len(t, result.Scores, 1)
	assert.Equal(t, "custom_regex_score", result.Scores[0].Name)
}

func TestRegexScorer_ReasonField(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	tests := []struct {
		name          string
		pattern       string
		text          string
		reasonContain string
	}{
		{
			name:          "match reason",
			pattern:       "hello",
			text:          "hello world",
			reasonContain: "Pattern matched",
		},
		{
			name:          "no match reason",
			pattern:       "goodbye",
			text:          "hello world",
			reasonContain: "did not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &EvaluationJob{
				ScorerConfig: map[string]any{"pattern": tt.pattern},
				Variables:    map[string]string{"output": tt.text},
			}

			result, err := scorer.Execute(ctx, job)
			require.NoError(t, err)
			require.Len(t, result.Scores, 1)
			require.NotNil(t, result.Scores[0].Reason)
			assert.Contains(t, *result.Scores[0].Reason, tt.reasonContain)
		})
	}
}

func TestRegexScorer_Type(t *testing.T) {
	ctx := context.Background()
	scorer := NewRegexScorer(newTestLogger())

	job := &EvaluationJob{
		ScorerConfig: map[string]any{"pattern": "test"},
		Variables:    map[string]string{"output": "test"},
	}

	result, err := scorer.Execute(ctx, job)
	require.NoError(t, err)
	require.Len(t, result.Scores, 1)
	assert.Equal(t, "NUMERIC", result.Scores[0].Type)
}
