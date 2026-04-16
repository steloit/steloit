package evaluation

// TestEvaluatorRequest defines the request for testing an evaluator against sample spans.
type TestEvaluatorRequest struct {
	TraceID     *string          `json:"trace_id,omitempty"`     // Optional: test against specific trace
	SpanID      *string          `json:"span_id,omitempty"`      // Optional: test against specific span
	SpanIDs     []string         `json:"span_ids,omitempty"`     // Optional: test against specific spans
	Limit       int              `json:"limit,omitempty"`        // Max spans to evaluate (default: 5)
	TimeRange   string           `json:"time_range,omitempty"`   // Time range: "1h", "24h", "7d" (default: "24h")
	SampleInput *TestSampleInput `json:"sample_input,omitempty"` // Optional: manual sample input for dry-run
}

// TestSampleInput allows manual input for dry-run testing without actual spans.
type TestSampleInput struct {
	Input    string         `json:"input"`
	Output   string         `json:"output"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TestEvaluatorResponse is the response from testing an evaluator.
type TestEvaluatorResponse struct {
	Summary          TestSummary      `json:"summary"`
	Executions       []TestExecution  `json:"executions"`
	EvaluatorPreview EvaluatorPreview `json:"evaluator_preview"`
}

// TestSummary provides aggregated statistics from test execution.
type TestSummary struct {
	TotalSpans     int     `json:"total_spans"`
	MatchedSpans   int     `json:"matched_spans"`
	EvaluatedSpans int     `json:"evaluated_spans"`
	SuccessCount   int     `json:"success_count"`
	FailureCount   int     `json:"failure_count"`
	SkippedCount   int     `json:"skipped_count"`
	AverageScore   float64 `json:"average_score,omitempty"`
	AverageLatency int64   `json:"average_latency_ms"`
}

// TestExecution represents a single test execution result.
type TestExecution struct {
	SpanID            string             `json:"span_id"`
	TraceID           string             `json:"trace_id"`
	SpanName          string             `json:"span_name"`
	MatchedFilter     bool               `json:"matched_filter"`
	Status            string             `json:"status"` // success, failed, skipped, filtered
	ScoreResults      []TestScoreResult  `json:"score_results,omitempty"`
	PromptSent        []LLMMessage       `json:"prompt_sent,omitempty"`
	LLMResponse       string             `json:"llm_response,omitempty"`
	LLMResponseParsed map[string]any     `json:"llm_response_parsed,omitempty"`
	VariablesResolved []ResolvedVariable `json:"variables_resolved,omitempty"`
	ErrorMessage      string             `json:"error_message,omitempty"`
	LatencyMs         int64              `json:"latency_ms,omitempty"`
}

// TestScoreResult represents a single score output from the test.
type TestScoreResult struct {
	ScoreName  string  `json:"score_name"`
	Value      any     `json:"value"`
	Reasoning  string  `json:"reasoning,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// ResolvedVariable shows how a variable was resolved during execution.
type ResolvedVariable struct {
	VariableName  string `json:"variable_name"`
	Source        string `json:"source"`
	JSONPath      string `json:"json_path,omitempty"`
	ResolvedValue any    `json:"resolved_value"`
}

// EvaluatorPreview shows the evaluator configuration that was tested.
type EvaluatorPreview struct {
	Name              string   `json:"name"`
	ScorerType        string   `json:"scorer_type"`
	FilterDescription string   `json:"filter_description"`
	VariableNames     []string `json:"variable_names"`
	PromptPreview     string   `json:"prompt_preview,omitempty"`
	MatchingCount     int      `json:"matching_count,omitempty"`
}
