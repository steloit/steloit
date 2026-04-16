package prompt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	analyticsDomain "brokle/internal/core/domain/analytics"
	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/pkg/errors"
)

type AIModelProvider string

const (
	ProviderOpenAI     AIModelProvider = "openai"
	ProviderAnthropic  AIModelProvider = "anthropic"
	ProviderAzure      AIModelProvider = "azure"
	ProviderGemini     AIModelProvider = "gemini"
	ProviderOpenRouter AIModelProvider = "openrouter"
	ProviderCustom     AIModelProvider = "custom"
)

// API keys and base URLs are provided per-request via project credentials.
type AIClientConfig struct {
	DefaultTimeout time.Duration
}

type executionService struct {
	compiler       promptDomain.CompilerService
	pricingService analyticsDomain.ProviderPricingService // Optional: nil = use fallback pricing
	config         *AIClientConfig
	httpClient     *http.Client
}

// pricingService is optional - if nil, hardcoded fallback pricing is used.
func NewExecutionService(
	compiler promptDomain.CompilerService,
	pricingService analyticsDomain.ProviderPricingService,
	config *AIClientConfig,
) promptDomain.ExecutionService {
	timeout := config.DefaultTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &executionService{
		compiler:       compiler,
		pricingService: pricingService,
		config:         config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (s *executionService) Execute(ctx context.Context, prompt *promptDomain.PromptResponse, variables map[string]string, configOverrides *promptDomain.ModelConfig) (*promptDomain.ExecutePromptResponse, error) {
	startTime := time.Now()

	compiled, err := s.compiler.Compile(prompt.Template, prompt.Type, variables)
	if err != nil {
		return &promptDomain.ExecutePromptResponse{
			CompiledPrompt: nil,
			LatencyMs:      time.Since(startTime).Milliseconds(),
			Error:          fmt.Sprintf("failed to compile template: %v", err),
		}, nil
	}

	effectiveConfig := s.mergeConfig(nil, configOverrides)
	if effectiveConfig == nil || effectiveConfig.Model == "" {
		return &promptDomain.ExecutePromptResponse{
			CompiledPrompt: compiled,
			LatencyMs:      time.Since(startTime).Milliseconds(),
			Error:          "no model specified in config",
		}, nil
	}

	if effectiveConfig.Provider == "" {
		return &promptDomain.ExecutePromptResponse{
			CompiledPrompt: compiled,
			LatencyMs:      time.Since(startTime).Milliseconds(),
			Error:          "provider not specified in config",
		}, nil
	}
	provider := AIModelProvider(effectiveConfig.Provider)

	var llmResp *promptDomain.LLMResponse
	switch provider {
	case ProviderOpenAI, ProviderAzure, ProviderOpenRouter, ProviderCustom:
		llmResp, err = s.executeOpenAICompatible(ctx, prompt.Type, compiled, effectiveConfig, provider)
	case ProviderAnthropic:
		llmResp, err = s.executeAnthropic(ctx, prompt.Type, compiled, effectiveConfig)
	case ProviderGemini:
		llmResp, err = s.executeGemini(ctx, prompt.Type, compiled, effectiveConfig)
	default:
		return &promptDomain.ExecutePromptResponse{
			CompiledPrompt: compiled,
			LatencyMs:      time.Since(startTime).Milliseconds(),
			Error:          fmt.Sprintf("unsupported provider: %s", provider),
		}, nil
	}

	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return &promptDomain.ExecutePromptResponse{
			CompiledPrompt: compiled,
			LatencyMs:      latencyMs,
			Error:          err.Error(),
		}, nil
	}

	return &promptDomain.ExecutePromptResponse{
		CompiledPrompt: compiled,
		Response:       llmResp,
		LatencyMs:      latencyMs,
	}, nil
}

func (s *executionService) ExecuteStream(ctx context.Context, prompt *promptDomain.PromptResponse, variables map[string]string, configOverrides *promptDomain.ModelConfig) (<-chan promptDomain.StreamEvent, <-chan *promptDomain.StreamResult, error) {
	compiled, err := s.compiler.Compile(prompt.Template, prompt.Type, variables)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile template: %w", err)
	}

	effectiveConfig := s.mergeConfig(nil, configOverrides)
	if effectiveConfig == nil || effectiveConfig.Model == "" {
		return nil, nil, errors.NewValidationError("no model specified in config", "")
	}

	if effectiveConfig.Provider == "" {
		return nil, nil, errors.NewValidationError("provider not specified in config", "")
	}
	provider := AIModelProvider(effectiveConfig.Provider)

	eventChan := make(chan promptDomain.StreamEvent, 100)
	resultChan := make(chan *promptDomain.StreamResult, 1)

	go func() {
		switch provider {
		case ProviderOpenAI, ProviderAzure, ProviderOpenRouter, ProviderCustom:
			s.streamOpenAICompatible(ctx, prompt.Type, compiled, effectiveConfig, provider, eventChan, resultChan)
		case ProviderAnthropic:
			s.streamAnthropic(ctx, prompt.Type, compiled, effectiveConfig, eventChan, resultChan)
		case ProviderGemini:
			s.streamGemini(ctx, prompt.Type, compiled, effectiveConfig, eventChan, resultChan)
		default:
			eventChan <- promptDomain.StreamEvent{
				Type:  promptDomain.StreamEventError,
				Error: fmt.Sprintf("unsupported provider: %s", provider),
			}
			close(eventChan)
			close(resultChan)
		}
	}()

	return eventChan, resultChan, nil
}

func (s *executionService) Preview(ctx context.Context, prompt *promptDomain.PromptResponse, variables map[string]string) (interface{}, error) {
	return s.compiler.Compile(prompt.Template, prompt.Type, variables)
}

func (s *executionService) mergeConfig(base, overrides *promptDomain.ModelConfig) *promptDomain.ModelConfig {
	if base == nil && overrides == nil {
		return nil
	}
	if base == nil {
		return overrides
	}
	if overrides == nil {
		return base
	}

	result := &promptDomain.ModelConfig{
		Model:            base.Model,
		Provider:         base.Provider,
		Temperature:      base.Temperature,
		MaxTokens:        base.MaxTokens,
		TopP:             base.TopP,
		FrequencyPenalty: base.FrequencyPenalty,
		PresencePenalty:  base.PresencePenalty,
		Stop:             base.Stop,
		Tools:            base.Tools,
		ToolChoice:       base.ToolChoice,
		ResponseFormat:   base.ResponseFormat,
		// Preserve credentials from overrides (set by handler after credential resolution)
		APIKey:          overrides.APIKey,
		ResolvedBaseURL: overrides.ResolvedBaseURL,
		ProviderConfig:  overrides.ProviderConfig, // Azure deployment_id, api_version
		CustomHeaders:   overrides.CustomHeaders,  // Custom provider headers
	}

	if overrides.Model != "" {
		result.Model = overrides.Model
	}
	if overrides.Provider != "" {
		result.Provider = overrides.Provider
	}
	if overrides.Temperature != nil {
		result.Temperature = overrides.Temperature
	}
	if overrides.MaxTokens != nil {
		result.MaxTokens = overrides.MaxTokens
	}
	if overrides.TopP != nil {
		result.TopP = overrides.TopP
	}
	if overrides.FrequencyPenalty != nil {
		result.FrequencyPenalty = overrides.FrequencyPenalty
	}
	if overrides.PresencePenalty != nil {
		result.PresencePenalty = overrides.PresencePenalty
	}
	if len(overrides.Stop) > 0 {
		result.Stop = overrides.Stop
	}
	if len(overrides.Tools) > 0 {
		result.Tools = overrides.Tools
	}
	if len(overrides.ToolChoice) > 0 {
		result.ToolChoice = overrides.ToolChoice
	}
	if len(overrides.ResponseFormat) > 0 {
		result.ResponseFormat = overrides.ResponseFormat
	}

	return result
}

type openAIRequest struct {
	Model            string            `json:"model"`
	Messages         []openAIMessage   `json:"messages,omitempty"`
	Prompt           string            `json:"prompt,omitempty"`
	Temperature      *float64          `json:"temperature,omitempty"`
	MaxTokens        *int              `json:"max_tokens,omitempty"`
	TopP             *float64          `json:"top_p,omitempty"`
	FrequencyPenalty *float64          `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64          `json:"presence_penalty,omitempty"`
	Stop             []string          `json:"stop,omitempty"`
	Tools            []json.RawMessage `json:"tools,omitempty"`
	ToolChoice       json.RawMessage   `json:"tool_choice,omitempty"`
	ResponseFormat   json.RawMessage   `json:"response_format,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string            `json:"role"`
			Content   *string           `json:"content"` // Pointer to accept null when tool_calls are returned
			ToolCalls []json.RawMessage `json:"tool_calls,omitempty"`
		} `json:"message"`
		Text         string `json:"text,omitempty"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (s *executionService) getOpenAICompatibleConfig(provider AIModelProvider, config *promptDomain.ModelConfig) (baseURL, authHeader, authValue string, err error) {
	switch provider {
	case ProviderOpenAI:
		baseURL = "https://api.openai.com/v1"
		if config.ResolvedBaseURL != nil && *config.ResolvedBaseURL != "" {
			baseURL = *config.ResolvedBaseURL
		}
		authHeader = "Authorization"
		authValue = "Bearer " + config.APIKey

	case ProviderAzure:
		if config.ResolvedBaseURL == nil || *config.ResolvedBaseURL == "" {
			return "", "", "", errors.NewValidationError("Azure OpenAI requires base URL", "configure base_url in provider credentials")
		}

		// Extract deployment_id from ProviderConfig
		deploymentID := ""
		if config.ProviderConfig != nil {
			if d, ok := config.ProviderConfig["deployment_id"].(string); ok && d != "" {
				deploymentID = d
			}
		}
		if deploymentID == "" {
			return "", "", "", errors.NewValidationError("Azure OpenAI requires deployment_id", "configure deployment_id in provider credentials config")
		}

		// Build correct Azure endpoint: {baseURL}/openai/deployments/{deployment_id}
		baseURL = strings.TrimSuffix(*config.ResolvedBaseURL, "/") + "/openai/deployments/" + deploymentID
		authHeader = "api-key" // Azure uses api-key header, NOT Bearer
		authValue = config.APIKey

	case ProviderOpenRouter:
		baseURL = "https://openrouter.ai/api/v1"
		if config.ResolvedBaseURL != nil && *config.ResolvedBaseURL != "" {
			baseURL = *config.ResolvedBaseURL
		}
		authHeader = "Authorization"
		authValue = "Bearer " + config.APIKey

	case ProviderCustom:
		if config.ResolvedBaseURL == nil || *config.ResolvedBaseURL == "" {
			return "", "", "", errors.NewValidationError("Custom provider requires base URL", "configure base_url in provider credentials")
		}
		baseURL = *config.ResolvedBaseURL
		authHeader = "Authorization"
		authValue = "Bearer " + config.APIKey

	default:
		return "", "", "", errors.NewValidationError("provider not OpenAI-compatible", string(provider))
	}

	return baseURL, authHeader, authValue, nil
}

func (s *executionService) executeOpenAICompatible(ctx context.Context, promptType promptDomain.PromptType, compiled interface{}, config *promptDomain.ModelConfig, provider AIModelProvider) (*promptDomain.LLMResponse, error) {
	if config.APIKey == "" {
		return nil, errors.NewValidationError("API key not provided", fmt.Sprintf("%s API key must be provided via project credentials", provider))
	}

	baseURL, authHeader, authValue, err := s.getOpenAICompatibleConfig(provider, config)
	if err != nil {
		return nil, err
	}

	req := openAIRequest{
		Model:            config.Model,
		Temperature:      config.Temperature,
		MaxTokens:        config.MaxTokens,
		TopP:             config.TopP,
		FrequencyPenalty: config.FrequencyPenalty,
		PresencePenalty:  config.PresencePenalty,
		Stop:             config.Stop,
		Tools:            config.Tools,
		ToolChoice:       config.ToolChoice,
		ResponseFormat:   config.ResponseFormat,
	}

	var endpoint string
	switch promptType {
	case promptDomain.PromptTypeChat:
		messages, ok := compiled.([]promptDomain.ChatMessage)
		if !ok {
			return nil, errors.NewValidationError("invalid compiled chat messages", "")
		}
		req.Messages = make([]openAIMessage, len(messages))
		for i, msg := range messages {
			req.Messages[i] = openAIMessage{
				Role:    msg.Role,
				Content: msg.Content,
			}
		}
		endpoint = baseURL + "/chat/completions"

	case promptDomain.PromptTypeText:
		text, ok := compiled.(string)
		if !ok {
			return nil, errors.NewValidationError("invalid compiled text prompt", "")
		}
		// Text prompts use chat API with user role
		req.Messages = []openAIMessage{
			{Role: "user", Content: text},
		}
		endpoint = baseURL + "/chat/completions"

	default:
		return nil, errors.NewValidationError("unsupported prompt type: "+string(promptType), "")
	}

	// Add api-version query param for Azure
	if provider == ProviderAzure {
		apiVersion := "2024-10-21" // default (latest GA)
		if config.ProviderConfig != nil {
			if v, ok := config.ProviderConfig["api_version"].(string); ok && v != "" {
				apiVersion = v
			}
		}
		endpoint += "?api-version=" + apiVersion
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(authHeader, authValue)

	// Set custom headers from credentials (for proxies/custom providers)
	for key, value := range config.CustomHeaders {
		httpReq.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("%s API error: %s (%s)", provider, openAIResp.Error.Message, openAIResp.Error.Type)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in %s response", provider)
	}

	var content string
	if openAIResp.Choices[0].Message.Content != nil && *openAIResp.Choices[0].Message.Content != "" {
		content = *openAIResp.Choices[0].Message.Content
	} else if openAIResp.Choices[0].Text != "" {
		content = openAIResp.Choices[0].Text
	}

	cost := s.calculateCost(ctx, provider, config.Model, openAIResp.Usage.PromptTokens, openAIResp.Usage.CompletionTokens)

	return &promptDomain.LLMResponse{
		Content: content,
		Model:   openAIResp.Model,
		Usage: &promptDomain.LLMUsage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
		Cost:         &cost,
		FinishReason: openAIResp.Choices[0].FinishReason,
		ToolCalls:    openAIResp.Choices[0].Message.ToolCalls,
	}, nil
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	StopSeq     []string           `json:"stop_sequences,omitempty"`
	// Tools for function calling (Anthropic format)
	Tools      []json.RawMessage `json:"tools,omitempty"`
	ToolChoice json.RawMessage   `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (s *executionService) executeAnthropic(ctx context.Context, promptType promptDomain.PromptType, compiled interface{}, config *promptDomain.ModelConfig) (*promptDomain.LLMResponse, error) {
	if config.APIKey == "" {
		return nil, errors.NewValidationError("API key not provided", "Anthropic API key must be provided via project credentials")
	}

	baseURL := "https://api.anthropic.com"
	if config.ResolvedBaseURL != nil && *config.ResolvedBaseURL != "" {
		baseURL = *config.ResolvedBaseURL
	}

	maxTokens := 4096
	if config.MaxTokens != nil {
		maxTokens = *config.MaxTokens
	}

	req := anthropicRequest{
		Model:       config.Model,
		MaxTokens:   maxTokens,
		Temperature: config.Temperature,
		TopP:        config.TopP,
		StopSeq:     config.Stop,
	}

	switch promptType {
	case promptDomain.PromptTypeChat:
		messages, ok := compiled.([]promptDomain.ChatMessage)
		if !ok {
			return nil, errors.NewValidationError("invalid compiled chat messages", "")
		}

		// Anthropic uses separate system field instead of system role message
		var anthropicMsgs []anthropicMessage
		for _, msg := range messages {
			if msg.Role == "system" {
				req.System = msg.Content
				continue
			}
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
		req.Messages = anthropicMsgs

	case promptDomain.PromptTypeText:
		text, ok := compiled.(string)
		if !ok {
			return nil, errors.NewValidationError("invalid compiled text prompt", "")
		}
		req.Messages = []anthropicMessage{
			{Role: "user", Content: text},
		}

	default:
		return nil, errors.NewValidationError("unsupported prompt type: "+string(promptType), "")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if anthropicResp.Error != nil {
		return nil, fmt.Errorf("Anthropic API error: %s (%s)", anthropicResp.Error.Message, anthropicResp.Error.Type)
	}

	var content string
	for _, c := range anthropicResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	cost := s.calculateCost(ctx, ProviderAnthropic, config.Model, anthropicResp.Usage.InputTokens, anthropicResp.Usage.OutputTokens)

	return &promptDomain.LLMResponse{
		Content: content,
		Model:   anthropicResp.Model,
		Usage: &promptDomain.LLMUsage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
		Cost: &cost,
	}, nil
}

// calculateCost calculates execution cost using ProviderPricingService if available,
// with fallback to hardcoded provider-specific pricing.
// This unified method handles all providers and integrates with the analytics domain.
func (s *executionService) calculateCost(ctx context.Context, provider AIModelProvider, model string, promptTokens, completionTokens int) float64 {
	// Try pricing service if available
	if s.pricingService != nil {
		cost, err := s.calculateCostFromService(ctx, model, promptTokens, completionTokens)
		if err == nil {
			return cost
		}
		// Fall through to hardcoded pricing on error
	}

	// Fallback to hardcoded pricing by provider
	switch provider {
	case ProviderOpenAI, ProviderAzure, ProviderOpenRouter, ProviderCustom:
		return s.calculateOpenAICostFallback(model, promptTokens, completionTokens)
	case ProviderAnthropic:
		return s.calculateAnthropicCostFallback(model, promptTokens, completionTokens)
	case ProviderGemini:
		return s.calculateGeminiCostFallback(model, promptTokens, completionTokens)
	default:
		// Generic fallback: $1/$2 per million tokens
		return (float64(promptTokens)*1.0 + float64(completionTokens)*2.0) / 1_000_000
	}
}

func (s *executionService) calculateCostFromService(ctx context.Context, model string, promptTokens, completionTokens int) (float64, error) {
	// Get pricing snapshot for this model (global pricing, no project-specific override)
	snapshot, err := s.pricingService.GetProviderPricingSnapshot(ctx, nil, model, time.Now())
	if err != nil {
		return 0, err
	}

	// Build usage map for the pricing service
	usage := map[string]uint64{
		"input_tokens":  uint64(promptTokens),
		"output_tokens": uint64(completionTokens),
	}

	// Calculate cost using pricing service
	costs := s.pricingService.CalculateProviderCost(usage, snapshot)
	total, ok := costs["total"]
	if !ok {
		return 0, fmt.Errorf("no total cost in pricing result")
	}

	// Convert decimal.Decimal to float64
	cost, _ := total.Float64()
	return cost, nil
}

// calculateOpenAICostFallback uses hardcoded pricing for OpenAI-compatible providers.
// Used when ProviderPricingService is unavailable or returns an error.
// Prices are in USD per 1M tokens.
func (s *executionService) calculateOpenAICostFallback(model string, promptTokens, completionTokens int) float64 {
	var inputPrice, outputPrice float64

	switch {
	case strings.HasPrefix(model, "gpt-4o-mini"):
		inputPrice, outputPrice = 0.15, 0.60
	case strings.HasPrefix(model, "gpt-4o"):
		inputPrice, outputPrice = 2.50, 10.00
	case strings.HasPrefix(model, "gpt-4-turbo"), strings.HasPrefix(model, "gpt-4-1106"):
		inputPrice, outputPrice = 10.00, 30.00
	case strings.HasPrefix(model, "gpt-4"):
		inputPrice, outputPrice = 30.00, 60.00
	case strings.HasPrefix(model, "gpt-3.5-turbo"):
		inputPrice, outputPrice = 0.50, 1.50
	case strings.HasPrefix(model, "o1-mini"):
		inputPrice, outputPrice = 3.00, 12.00
	case strings.HasPrefix(model, "o1"):
		inputPrice, outputPrice = 15.00, 60.00
	default:
		inputPrice, outputPrice = 1.00, 2.00 // Default fallback
	}

	return (float64(promptTokens)*inputPrice + float64(completionTokens)*outputPrice) / 1_000_000
}

// calculateAnthropicCostFallback uses hardcoded pricing for Anthropic.
// Prices are in USD per 1M tokens.
func (s *executionService) calculateAnthropicCostFallback(model string, inputTokens, outputTokens int) float64 {
	var inputPrice, outputPrice float64

	switch {
	case strings.Contains(model, "claude-3-5-sonnet"), strings.Contains(model, "claude-sonnet-4"):
		inputPrice, outputPrice = 3.00, 15.00
	case strings.Contains(model, "claude-3-5-haiku"), strings.Contains(model, "claude-haiku-3-5"):
		inputPrice, outputPrice = 1.00, 5.00
	case strings.Contains(model, "claude-3-opus"), strings.Contains(model, "claude-opus"):
		inputPrice, outputPrice = 15.00, 75.00
	case strings.Contains(model, "claude-3-haiku"):
		inputPrice, outputPrice = 0.25, 1.25
	case strings.Contains(model, "claude-3"):
		inputPrice, outputPrice = 3.00, 15.00
	default:
		inputPrice, outputPrice = 3.00, 15.00 // Default fallback
	}

	return (float64(inputTokens)*inputPrice + float64(outputTokens)*outputPrice) / 1_000_000
}

// calculateGeminiCostFallback uses hardcoded pricing for Google Gemini.
// Prices are in USD per 1M tokens.
func (s *executionService) calculateGeminiCostFallback(model string, promptTokens, completionTokens int) float64 {
	var inputPrice, outputPrice float64

	switch {
	case strings.Contains(model, "gemini-2.0-flash"):
		inputPrice, outputPrice = 0.10, 0.40
	case strings.Contains(model, "gemini-1.5-pro"):
		inputPrice, outputPrice = 1.25, 5.00
	case strings.Contains(model, "gemini-1.5-flash"):
		inputPrice, outputPrice = 0.075, 0.30
	case strings.Contains(model, "gemini-pro"):
		inputPrice, outputPrice = 0.50, 1.50
	default:
		inputPrice, outputPrice = 0.50, 1.50 // Default fallback
	}

	return (float64(promptTokens)*inputPrice + float64(completionTokens)*outputPrice) / 1_000_000
}

// Gemini uses a unique API format different from OpenAI-compatible providers.
type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
	Error         *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content       *geminiContent `json:"content,omitempty"`
	FinishReason  string         `json:"finishReason,omitempty"`
	SafetyRatings []struct {
		Category    string `json:"category"`
		Probability string `json:"probability"`
	} `json:"safetyRatings,omitempty"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// executeGemini executes prompts using Google Gemini API.
// Uses x-goog-api-key header for security (API keys in URLs get logged).
func (s *executionService) executeGemini(ctx context.Context, promptType promptDomain.PromptType, compiled interface{}, config *promptDomain.ModelConfig) (*promptDomain.LLMResponse, error) {
	if config.APIKey == "" {
		return nil, errors.NewValidationError("API key not provided", "Gemini API key must be provided via project credentials")
	}

	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if config.ResolvedBaseURL != nil && *config.ResolvedBaseURL != "" {
		baseURL = *config.ResolvedBaseURL
	}

	req := geminiRequest{
		GenerationConfig: &geminiGenConfig{
			Temperature:     config.Temperature,
			MaxOutputTokens: config.MaxTokens,
			TopP:            config.TopP,
			StopSequences:   config.Stop,
		},
	}

	switch promptType {
	case promptDomain.PromptTypeChat:
		messages, ok := compiled.([]promptDomain.ChatMessage)
		if !ok {
			return nil, errors.NewValidationError("invalid compiled chat messages", "")
		}

		var contents []geminiContent
		for _, msg := range messages {
			// Convert roles: system goes to systemInstruction, assistant -> model
			switch msg.Role {
			case "system":
				req.SystemInstruction = &geminiContent{
					Parts: []geminiPart{{Text: msg.Content}},
				}
			case "assistant":
				contents = append(contents, geminiContent{
					Role:  "model", // Gemini uses "model" instead of "assistant"
					Parts: []geminiPart{{Text: msg.Content}},
				})
			default: // user and any other roles
				contents = append(contents, geminiContent{
					Role:  msg.Role,
					Parts: []geminiPart{{Text: msg.Content}},
				})
			}
		}
		req.Contents = contents

	case promptDomain.PromptTypeText:
		text, ok := compiled.(string)
		if !ok {
			return nil, errors.NewValidationError("invalid compiled text prompt", "")
		}
		req.Contents = []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: text}},
			},
		}

	default:
		return nil, errors.NewValidationError("unsupported prompt type: "+string(promptType), "")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build endpoint without API key in URL (security: keys in URLs get logged)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", baseURL, config.Model)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", config.APIKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("Gemini API error: %s (code: %d, status: %s)", geminiResp.Error.Message, geminiResp.Error.Code, geminiResp.Error.Status)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in Gemini response")
	}

	var content string
	if geminiResp.Candidates[0].Content != nil {
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			content += part.Text
		}
	}

	var usage *promptDomain.LLMUsage
	var promptTokens, completionTokens int
	if geminiResp.UsageMetadata != nil {
		promptTokens = geminiResp.UsageMetadata.PromptTokenCount
		completionTokens = geminiResp.UsageMetadata.CandidatesTokenCount
		usage = &promptDomain.LLMUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}

	cost := s.calculateCost(ctx, ProviderGemini, config.Model, promptTokens, completionTokens)

	return &promptDomain.LLMResponse{
		Content: content,
		Model:   config.Model,
		Usage:   usage,
		Cost:    &cost,
	}, nil
}

// toolCallAccumulator accumulates streaming tool call deltas by index
type toolCallAccumulator struct {
	ID   string
	Type string
	Name string
	Args strings.Builder // Accumulate arguments across deltas
}

type streamAccumulator struct {
	content      strings.Builder
	model        string
	finishReason string
	usage        *promptDomain.LLMUsage
	toolCalls    map[int]*toolCallAccumulator // Map by index for merging deltas
}

// getToolCalls converts accumulated tool calls to json.RawMessage format for StreamResult
func (acc *streamAccumulator) getToolCalls() []json.RawMessage {
	if len(acc.toolCalls) == 0 {
		return nil
	}

	// Sort by index to maintain order
	indices := make([]int, 0, len(acc.toolCalls))
	for idx := range acc.toolCalls {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	result := make([]json.RawMessage, 0, len(indices))
	for _, idx := range indices {
		tc := acc.toolCalls[idx]
		toolCall := map[string]interface{}{
			"id":   tc.ID,
			"type": tc.Type,
			"function": map[string]string{
				"name":      tc.Name,
				"arguments": tc.Args.String(),
			},
		}
		data, _ := json.Marshal(toolCall)
		result = append(result, data)
	}
	return result
}

type openAIStreamRequest struct {
	Model            string            `json:"model"`
	Messages         []openAIMessage   `json:"messages,omitempty"`
	Prompt           string            `json:"prompt,omitempty"`
	Temperature      *float64          `json:"temperature,omitempty"`
	MaxTokens        *int              `json:"max_tokens,omitempty"`
	TopP             *float64          `json:"top_p,omitempty"`
	FrequencyPenalty *float64          `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64          `json:"presence_penalty,omitempty"`
	Stop             []string          `json:"stop,omitempty"`
	Tools            []json.RawMessage `json:"tools,omitempty"`
	ToolChoice       json.RawMessage   `json:"tool_choice,omitempty"`
	ResponseFormat   json.RawMessage   `json:"response_format,omitempty"`
	Stream           bool              `json:"stream"`
	StreamOptions    *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

// openAIStreamToolCallDelta represents a tool call delta in streaming
type openAIStreamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type openAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string                      `json:"role,omitempty"`
			Content   string                      `json:"content,omitempty"`
			ToolCalls []openAIStreamToolCallDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type anthropicStreamRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	StopSeq     []string           `json:"stop_sequences,omitempty"`
	Tools       []json.RawMessage  `json:"tools,omitempty"`
	ToolChoice  json.RawMessage    `json:"tool_choice,omitempty"`
	Stream      bool               `json:"stream"`
}

type anthropicMessageStart struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type anthropicContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"` // For tool_use input_json_delta
	} `json:"delta"`
}

// anthropicContentBlockStart represents a content_block_start event
type anthropicContentBlockStart struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type  string `json:"type"`            // "text" or "tool_use"
		ID    string `json:"id,omitempty"`    // Tool use ID
		Name  string `json:"name,omitempty"`  // Function name for tool_use
		Input any    `json:"input,omitempty"` // Empty object for tool_use start
	} `json:"content_block"`
}

type anthropicMessageDelta struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (s *executionService) streamOpenAICompatible(
	ctx context.Context,
	promptType promptDomain.PromptType,
	compiled interface{},
	config *promptDomain.ModelConfig,
	provider AIModelProvider,
	eventChan chan<- promptDomain.StreamEvent,
	resultChan chan<- *promptDomain.StreamResult,
) {
	defer close(eventChan)
	defer close(resultChan)

	startTime := time.Now()

	if config.APIKey == "" {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("%s API key not provided via project credentials", provider)}
		return
	}

	baseURL, authHeader, authValue, err := s.getOpenAICompatibleConfig(provider, config)
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: err.Error()}
		return
	}

	req := openAIStreamRequest{
		Model:            config.Model,
		Temperature:      config.Temperature,
		MaxTokens:        config.MaxTokens,
		TopP:             config.TopP,
		FrequencyPenalty: config.FrequencyPenalty,
		PresencePenalty:  config.PresencePenalty,
		Stop:             config.Stop,
		Tools:            config.Tools,
		ToolChoice:       config.ToolChoice,
		ResponseFormat:   config.ResponseFormat,
		Stream:           true,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
	}

	var endpoint string
	switch promptType {
	case promptDomain.PromptTypeChat:
		messages, ok := compiled.([]promptDomain.ChatMessage)
		if !ok {
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "invalid compiled chat messages"}
			return
		}
		req.Messages = make([]openAIMessage, len(messages))
		for i, msg := range messages {
			req.Messages[i] = openAIMessage{Role: msg.Role, Content: msg.Content}
		}
		endpoint = baseURL + "/chat/completions"

	case promptDomain.PromptTypeText:
		text, ok := compiled.(string)
		if !ok {
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "invalid compiled text prompt"}
			return
		}
		req.Messages = []openAIMessage{{Role: "user", Content: text}}
		endpoint = baseURL + "/chat/completions"

	default:
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "unsupported prompt type"}
		return
	}

	// Add api-version query param for Azure
	if provider == ProviderAzure {
		apiVersion := "2024-10-21" // default (latest GA)
		if config.ProviderConfig != nil {
			if v, ok := config.ProviderConfig["api_version"].(string); ok && v != "" {
				apiVersion = v
			}
		}
		endpoint += "?api-version=" + apiVersion
	}

	body, err := json.Marshal(req)
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to marshal request: %v", err)}
		return
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to create request: %v", err)}
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(authHeader, authValue)

	// Set custom headers from credentials (for proxies/custom providers)
	for key, value := range config.CustomHeaders {
		httpReq.Header.Set(key, value)
	}

	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to execute request: %v", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("%s error (status %d): %s", provider, resp.StatusCode, string(body))}
		return
	}

	eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventStart}

	acc := streamAccumulator{
		toolCalls: make(map[int]*toolCallAccumulator),
	}
	reader := bufio.NewReader(resp.Body)
	var firstTokenTime *time.Time

	for {
		select {
		case <-ctx.Done():
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "stream cancelled"}
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("stream read error: %v", err)}
			return
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" || lineStr == "data: [DONE]" {
			continue
		}

		if !strings.HasPrefix(lineStr, "data: ") {
			continue
		}

		data := strings.TrimPrefix(lineStr, "data: ")
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if firstTokenTime == nil && len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			now := time.Now()
			firstTokenTime = &now
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Content != "" {
				acc.content.WriteString(delta.Content)
				eventChan <- promptDomain.StreamEvent{
					Type:    promptDomain.StreamEventContent,
					Content: delta.Content,
				}
			}

			// Process tool call deltas
			for _, tc := range delta.ToolCalls {
				if _, exists := acc.toolCalls[tc.Index]; !exists {
					acc.toolCalls[tc.Index] = &toolCallAccumulator{}
				}
				// Update fields when non-empty (providers may send id/name in later chunks)
				if tc.ID != "" {
					acc.toolCalls[tc.Index].ID = tc.ID
				}
				if tc.Type != "" {
					acc.toolCalls[tc.Index].Type = tc.Type
				}
				if tc.Function.Name != "" {
					acc.toolCalls[tc.Index].Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.toolCalls[tc.Index].Args.WriteString(tc.Function.Arguments)
				}
			}

			if chunk.Choices[0].FinishReason != nil {
				acc.finishReason = *chunk.Choices[0].FinishReason
			}
		}

		if chunk.Model != "" {
			acc.model = chunk.Model
		}

		if chunk.Usage != nil {
			acc.usage = &promptDomain.LLMUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}
	}

	totalDuration := time.Since(startTime).Milliseconds()
	var ttftMs *float64
	if firstTokenTime != nil {
		ttft := float64(firstTokenTime.Sub(startTime).Milliseconds())
		ttftMs = &ttft
	}

	var cost *float64
	if acc.usage != nil {
		c := s.calculateCost(ctx, provider, config.Model, acc.usage.PromptTokens, acc.usage.CompletionTokens)
		cost = &c
	}

	eventChan <- promptDomain.StreamEvent{
		Type:         promptDomain.StreamEventEnd,
		FinishReason: acc.finishReason,
	}

	resultChan <- &promptDomain.StreamResult{
		Content:       acc.content.String(),
		Model:         acc.model,
		Usage:         acc.usage,
		Cost:          cost,
		FinishReason:  acc.finishReason,
		TTFTMs:        ttftMs,
		TotalDuration: totalDuration,
		ToolCalls:     acc.getToolCalls(),
	}
}

func (s *executionService) streamAnthropic(
	ctx context.Context,
	promptType promptDomain.PromptType,
	compiled interface{},
	config *promptDomain.ModelConfig,
	eventChan chan<- promptDomain.StreamEvent,
	resultChan chan<- *promptDomain.StreamResult,
) {
	defer close(eventChan)
	defer close(resultChan)

	startTime := time.Now()

	if config.APIKey == "" {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "Anthropic API key not provided via project credentials"}
		return
	}

	baseURL := "https://api.anthropic.com"
	if config.ResolvedBaseURL != nil && *config.ResolvedBaseURL != "" {
		baseURL = *config.ResolvedBaseURL
	}

	maxTokens := 4096
	if config.MaxTokens != nil {
		maxTokens = *config.MaxTokens
	}

	req := anthropicStreamRequest{
		Model:       config.Model,
		MaxTokens:   maxTokens,
		Temperature: config.Temperature,
		TopP:        config.TopP,
		StopSeq:     config.Stop,
		Tools:       config.Tools,
		ToolChoice:  config.ToolChoice,
		Stream:      true,
	}

	switch promptType {
	case promptDomain.PromptTypeChat:
		messages, ok := compiled.([]promptDomain.ChatMessage)
		if !ok {
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "invalid compiled chat messages"}
			return
		}

		// Separate system messages from user/assistant
		var systemPrompts []string
		var chatMessages []anthropicMessage
		for _, msg := range messages {
			if msg.Role == "system" {
				systemPrompts = append(systemPrompts, msg.Content)
			} else {
				chatMessages = append(chatMessages, anthropicMessage{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
		}

		if len(systemPrompts) > 0 {
			req.System = strings.Join(systemPrompts, "\n\n")
		}
		req.Messages = chatMessages

	case promptDomain.PromptTypeText:
		text, ok := compiled.(string)
		if !ok {
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "invalid compiled text prompt"}
			return
		}
		req.Messages = []anthropicMessage{{Role: "user", Content: text}}

	default:
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "unsupported prompt type"}
		return
	}

	body, err := json.Marshal(req)
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to marshal request: %v", err)}
		return
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to create request: %v", err)}
		return
	}

	httpReq.Header.Set("x-api-key", config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to execute request: %v", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("Anthropic error (status %d): %s", resp.StatusCode, string(body))}
		return
	}

	eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventStart}

	acc := streamAccumulator{
		toolCalls: make(map[int]*toolCallAccumulator),
	}
	var inputTokens int
	reader := bufio.NewReader(resp.Body)
	var firstTokenTime *time.Time
	var currentEvent string

	for {
		select {
		case <-ctx.Done():
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "stream cancelled"}
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("stream read error: %v", err)}
			return
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		if strings.HasPrefix(lineStr, "event: ") {
			currentEvent = strings.TrimPrefix(lineStr, "event: ")
			continue
		}

		if strings.HasPrefix(lineStr, "data: ") {
			data := strings.TrimPrefix(lineStr, "data: ")

			switch currentEvent {
			case "message_start":
				var msg anthropicMessageStart
				if err := json.Unmarshal([]byte(data), &msg); err == nil {
					acc.model = msg.Message.Model
					inputTokens = msg.Message.Usage.InputTokens
				}

			case "content_block_start":
				// Handle tool_use block start
				var block anthropicContentBlockStart
				if err := json.Unmarshal([]byte(data), &block); err == nil {
					if block.ContentBlock.Type == "tool_use" {
						acc.toolCalls[block.Index] = &toolCallAccumulator{
							ID:   block.ContentBlock.ID,
							Type: "function",
							Name: block.ContentBlock.Name,
						}
					}
				}

			case "content_block_delta":
				var delta anthropicContentBlockDelta
				if err := json.Unmarshal([]byte(data), &delta); err == nil {
					if delta.Delta.Type == "text_delta" && delta.Delta.Text != "" {
						if firstTokenTime == nil {
							now := time.Now()
							firstTokenTime = &now
						}
						acc.content.WriteString(delta.Delta.Text)
						eventChan <- promptDomain.StreamEvent{
							Type:    promptDomain.StreamEventContent,
							Content: delta.Delta.Text,
						}
					} else if delta.Delta.Type == "input_json_delta" && delta.Delta.PartialJSON != "" {
						// Tool use argument streaming
						if tc, exists := acc.toolCalls[delta.Index]; exists {
							tc.Args.WriteString(delta.Delta.PartialJSON)
						}
					}
				}

			case "message_delta":
				var msgDelta anthropicMessageDelta
				if err := json.Unmarshal([]byte(data), &msgDelta); err == nil {
					acc.finishReason = msgDelta.Delta.StopReason
					acc.usage = &promptDomain.LLMUsage{
						PromptTokens:     inputTokens,
						CompletionTokens: msgDelta.Usage.OutputTokens,
						TotalTokens:      inputTokens + msgDelta.Usage.OutputTokens,
					}
				}

			case "message_stop":
			}
		}
	}

	totalDuration := time.Since(startTime).Milliseconds()
	var ttftMs *float64
	if firstTokenTime != nil {
		ttft := float64(firstTokenTime.Sub(startTime).Milliseconds())
		ttftMs = &ttft
	}

	var cost *float64
	if acc.usage != nil {
		c := s.calculateCost(ctx, ProviderAnthropic, config.Model, acc.usage.PromptTokens, acc.usage.CompletionTokens)
		cost = &c
	}

	eventChan <- promptDomain.StreamEvent{
		Type:         promptDomain.StreamEventEnd,
		FinishReason: acc.finishReason,
	}

	resultChan <- &promptDomain.StreamResult{
		Content:       acc.content.String(),
		Model:         acc.model,
		Usage:         acc.usage,
		Cost:          cost,
		FinishReason:  acc.finishReason,
		TTFTMs:        ttftMs,
		TotalDuration: totalDuration,
		ToolCalls:     acc.getToolCalls(),
	}
}

type geminiStreamChunk struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
	Error         *geminiError      `json:"error,omitempty"`
}

// streamGemini handles Google Gemini streaming execution.
// Endpoint: POST /v1beta/models/{model}:streamGenerateContent?alt=sse
// Uses x-goog-api-key header for security (API keys in URLs get logged).
func (s *executionService) streamGemini(
	ctx context.Context,
	promptType promptDomain.PromptType,
	compiled interface{},
	config *promptDomain.ModelConfig,
	eventChan chan<- promptDomain.StreamEvent,
	resultChan chan<- *promptDomain.StreamResult,
) {
	defer close(eventChan)
	defer close(resultChan)

	startTime := time.Now()

	if config.APIKey == "" {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "Gemini API key not provided via project credentials"}
		return
	}

	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if config.ResolvedBaseURL != nil && *config.ResolvedBaseURL != "" {
		baseURL = *config.ResolvedBaseURL
	}

	req := geminiRequest{
		GenerationConfig: &geminiGenConfig{
			Temperature:     config.Temperature,
			MaxOutputTokens: config.MaxTokens,
			TopP:            config.TopP,
			StopSequences:   config.Stop,
		},
	}

	switch promptType {
	case promptDomain.PromptTypeChat:
		messages, ok := compiled.([]promptDomain.ChatMessage)
		if !ok {
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "invalid compiled chat messages"}
			return
		}

		var contents []geminiContent
		for _, msg := range messages {
			switch msg.Role {
			case "system":
				req.SystemInstruction = &geminiContent{
					Parts: []geminiPart{{Text: msg.Content}},
				}
			case "assistant":
				contents = append(contents, geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: msg.Content}},
				})
			default:
				contents = append(contents, geminiContent{
					Role:  msg.Role,
					Parts: []geminiPart{{Text: msg.Content}},
				})
			}
		}
		req.Contents = contents

	case promptDomain.PromptTypeText:
		text, ok := compiled.(string)
		if !ok {
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "invalid compiled text prompt"}
			return
		}
		req.Contents = []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: text}},
			},
		}

	default:
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "unsupported prompt type"}
		return
	}

	body, err := json.Marshal(req)
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to marshal request: %v", err)}
		return
	}

	// Gemini streaming endpoint (no key in URL for security)
	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", baseURL, config.Model)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to create request: %v", err)}
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", config.APIKey)

	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("failed to execute request: %v", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("Gemini error (status %d): %s", resp.StatusCode, string(body))}
		return
	}

	eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventStart}

	var acc streamAccumulator
	var lastUsage *geminiUsage
	reader := bufio.NewReader(resp.Body)
	var firstTokenTime *time.Time

	for {
		select {
		case <-ctx.Done():
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: "stream cancelled"}
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("stream read error: %v", err)}
			return
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		// Gemini SSE format: "data: {json}"
		if !strings.HasPrefix(lineStr, "data: ") {
			continue
		}

		data := strings.TrimPrefix(lineStr, "data: ")
		var chunk geminiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			eventChan <- promptDomain.StreamEvent{Type: promptDomain.StreamEventError, Error: fmt.Sprintf("Gemini API error: %s", chunk.Error.Message)}
			return
		}

		// Extract content from candidates
		for _, candidate := range chunk.Candidates {
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						if firstTokenTime == nil {
							now := time.Now()
							firstTokenTime = &now
						}
						acc.content.WriteString(part.Text)
						eventChan <- promptDomain.StreamEvent{
							Type:    promptDomain.StreamEventContent,
							Content: part.Text,
						}
					}
				}
			}

			if candidate.FinishReason != "" {
				acc.finishReason = candidate.FinishReason
			}
		}

		// Track usage metadata
		if chunk.UsageMetadata != nil {
			lastUsage = chunk.UsageMetadata
		}
	}

	acc.model = config.Model

	totalDuration := time.Since(startTime).Milliseconds()
	var ttftMs *float64
	if firstTokenTime != nil {
		ttft := float64(firstTokenTime.Sub(startTime).Milliseconds())
		ttftMs = &ttft
	}

	var usage *promptDomain.LLMUsage
	var promptTokens, completionTokens int
	if lastUsage != nil {
		promptTokens = lastUsage.PromptTokenCount
		completionTokens = lastUsage.CandidatesTokenCount
		usage = &promptDomain.LLMUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      lastUsage.TotalTokenCount,
		}
		acc.usage = usage
	}

	var cost *float64
	if lastUsage != nil {
		c := s.calculateCost(ctx, ProviderGemini, config.Model, promptTokens, completionTokens)
		cost = &c
	}

	eventChan <- promptDomain.StreamEvent{
		Type:         promptDomain.StreamEventEnd,
		FinishReason: acc.finishReason,
	}

	resultChan <- &promptDomain.StreamResult{
		Content:       acc.content.String(),
		Model:         acc.model,
		Usage:         usage,
		Cost:          cost,
		FinishReason:  acc.finishReason,
		TTFTMs:        ttftMs,
		TotalDuration: totalDuration,
	}
}
