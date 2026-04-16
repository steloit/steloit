// Package credentials provides service implementations for credential management.
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	credentialsDomain "brokle/internal/core/domain/credentials"
	"brokle/pkg/encryption"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type providerCredentialService struct {
	repo       credentialsDomain.ProviderCredentialRepository
	encryptor  *encryption.Service
	logger     *slog.Logger
	httpClient *http.Client
}

func NewProviderCredentialService(
	repo credentialsDomain.ProviderCredentialRepository,
	encryptor *encryption.Service,
	logger *slog.Logger,
) credentialsDomain.ProviderCredentialService {
	return &providerCredentialService{
		repo:      repo,
		encryptor: encryptor,
		logger:    logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *providerCredentialService) Create(ctx context.Context, req *credentialsDomain.CreateCredentialRequest) (*credentialsDomain.ProviderCredentialResponse, error) {
	// Validate adapter
	if !req.Adapter.IsValid() {
		return nil, appErrors.NewValidationError("Invalid adapter", fmt.Sprintf("adapter must be one of: %v", credentialsDomain.ValidProviders()))
	}

	// Validate name
	if strings.TrimSpace(req.Name) == "" {
		return nil, appErrors.NewValidationError("Name required", "Configuration name is required")
	}

	existing, err := s.repo.GetByOrgAndName(ctx, req.OrganizationID, req.Name)
	if err != nil {
		s.logger.Error("failed to check name uniqueness",
			"error", err,
			"organization_id", req.OrganizationID,
			"name", req.Name,
		)
		return nil, appErrors.NewInternalError("Failed to check name uniqueness", err)
	}
	if existing != nil {
		return nil, appErrors.NewConflictError(fmt.Sprintf("A configuration named '%s' already exists in this organization", req.Name))
	}

	// Custom provider validation
	if req.Adapter == credentialsDomain.ProviderCustom {
		if req.BaseURL == nil || *req.BaseURL == "" {
			return nil, appErrors.NewValidationError("Base URL required", "Custom providers require a base_url")
		}
	}

	// Azure validation
	if req.Adapter == credentialsDomain.ProviderAzure {
		if req.BaseURL == nil || *req.BaseURL == "" {
			return nil, appErrors.NewValidationError("Base URL required", "Azure OpenAI requires a base_url (your Azure endpoint)")
		}
		deploymentID := ""
		if req.Config != nil {
			if d, ok := req.Config["deployment_id"].(string); ok {
				deploymentID = strings.TrimSpace(d)
			}
		}
		if deploymentID == "" {
			return nil, appErrors.NewValidationError("Deployment ID required", "Azure OpenAI requires deployment_id to be configured")
		}
	}

	// Validate API key
	if len(req.APIKey) < 10 {
		return nil, appErrors.NewValidationError("Invalid API key", "API key is too short")
	}

	if err := s.ValidateKey(ctx, req.Adapter, req.APIKey, req.BaseURL, req.Config); err != nil {
		return nil, err
	}

	// Encrypt API key
	encryptedKey, err := s.encryptor.Encrypt(req.APIKey)
	if err != nil {
		s.logger.Error("failed to encrypt API key",
			"error", err,
			"organization_id", req.OrganizationID,
			"adapter", req.Adapter,
		)
		return nil, appErrors.NewInternalError("Failed to secure API key", err)
	}
	keyPreview := credentialsDomain.MaskAPIKey(req.APIKey)

	// Encrypt headers if provided
	var encryptedHeaders string
	if len(req.Headers) > 0 {
		headersJSON, err := json.Marshal(req.Headers)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to serialize headers", err)
		}
		encryptedHeaders, err = s.encryptor.Encrypt(string(headersJSON))
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to secure headers", err)
		}
	}

	credential := &credentialsDomain.ProviderCredential{
		ID:             uid.New(),
		OrganizationID: req.OrganizationID,
		Name:           req.Name,
		Adapter:        req.Adapter,
		EncryptedKey:   encryptedKey,
		KeyPreview:     keyPreview,
		BaseURL:        req.BaseURL,
		Config:         req.Config,
		CustomModels:   req.CustomModels,
		Headers:        encryptedHeaders,
		CreatedBy:      req.CreatedBy,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.repo.Create(ctx, credential); err != nil {
		s.logger.Error("failed to create credential",
			"error", err,
			"organization_id", req.OrganizationID,
			"name", req.Name,
			"adapter", req.Adapter,
		)
		if errors.Is(err, credentialsDomain.ErrCredentialExists) {
			return nil, appErrors.NewConflictError(fmt.Sprintf("A configuration named '%s' already exists", req.Name))
		}
		return nil, appErrors.NewInternalError("Failed to create credential", err)
	}

	s.logger.Info("provider credential created",
		"organization_id", req.OrganizationID,
		"credential_id", credential.ID,
		"name", req.Name,
		"adapter", req.Adapter,
	)

	return s.toResponseWithHeaders(credential), nil
}

func (s *providerCredentialService) Update(ctx context.Context, id uuid.UUID, orgID uuid.UUID, req *credentialsDomain.UpdateCredentialRequest) (*credentialsDomain.ProviderCredentialResponse, error) {
	credential, err := s.repo.GetByID(ctx, id, orgID)
	if err != nil {
		if errors.Is(err, credentialsDomain.ErrCredentialNotFound) {
			return nil, appErrors.NewNotFoundError("Credential not found")
		}
		return nil, appErrors.NewInternalError("Failed to get credential", err)
	}

	if req.Name != nil && *req.Name != credential.Name {
		trimmedName := strings.TrimSpace(*req.Name)
		if trimmedName == "" {
			return nil, appErrors.NewValidationError("Name required", "Configuration name cannot be empty")
		}
		existing, err := s.repo.GetByOrgAndName(ctx, credential.OrganizationID, trimmedName)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to check name uniqueness", err)
		}
		if existing != nil && existing.ID != id {
			return nil, appErrors.NewConflictError(fmt.Sprintf("A configuration named '%s' already exists", trimmedName))
		}
		credential.Name = trimmedName
	}

	// Update API key if provided
	if req.APIKey != nil && *req.APIKey != "" {
		if len(*req.APIKey) < 10 {
			return nil, appErrors.NewValidationError("Invalid API key", "API key is too short")
		}

		// Use existing config if not updated
		configForValidation := credential.Config
		if req.Config != nil {
			configForValidation = req.Config
		}
		baseURLForValidation := credential.BaseURL
		if req.BaseURL != nil {
			baseURLForValidation = req.BaseURL
		}

		if err := s.ValidateKey(ctx, credential.Adapter, *req.APIKey, baseURLForValidation, configForValidation); err != nil {
			return nil, err
		}

		encryptedKey, err := s.encryptor.Encrypt(*req.APIKey)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to secure API key", err)
		}
		credential.EncryptedKey = encryptedKey
		credential.KeyPreview = credentialsDomain.MaskAPIKey(*req.APIKey)
	}

	// Update other fields
	if req.BaseURL != nil {
		credential.BaseURL = req.BaseURL
	}
	if req.Config != nil {
		credential.Config = req.Config
	}
	if req.CustomModels != nil {
		credential.CustomModels = req.CustomModels
	}
	// Update headers if explicitly provided in request
	// nil = not provided (don't change), empty map = clear, non-empty = set new
	if req.Headers != nil {
		if len(*req.Headers) > 0 {
			headersJSON, err := json.Marshal(*req.Headers)
			if err != nil {
				return nil, appErrors.NewInternalError("Failed to serialize headers", err)
			}
			encryptedHeaders, err := s.encryptor.Encrypt(string(headersJSON))
			if err != nil {
				return nil, appErrors.NewInternalError("Failed to secure headers", err)
			}
			credential.Headers = encryptedHeaders
		} else {
			// Empty map explicitly clears headers
			credential.Headers = ""
		}
	}

	credential.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, credential, orgID); err != nil {
		s.logger.Error("failed to update credential",
			"error", err,
			"credential_id", id,
		)
		if errors.Is(err, credentialsDomain.ErrCredentialExists) {
			return nil, appErrors.NewConflictError("A configuration with that name already exists")
		}
		return nil, appErrors.NewInternalError("Failed to update credential", err)
	}

	s.logger.Info("provider credential updated",
		"credential_id", id,
		"name", credential.Name,
	)

	return s.toResponseWithHeaders(credential), nil
}

func (s *providerCredentialService) GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*credentialsDomain.ProviderCredentialResponse, error) {
	credential, err := s.repo.GetByID(ctx, id, orgID)
	if err != nil {
		if errors.Is(err, credentialsDomain.ErrCredentialNotFound) {
			return nil, appErrors.NewNotFoundError("Credential not found")
		}
		return nil, appErrors.NewInternalError("Failed to retrieve credential", err)
	}
	return s.toResponseWithHeaders(credential), nil
}

func (s *providerCredentialService) GetByName(ctx context.Context, orgID uuid.UUID, name string) (*credentialsDomain.ProviderCredentialResponse, error) {
	credential, err := s.repo.GetByOrgAndName(ctx, orgID, name)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to retrieve credential", err)
	}
	if credential == nil {
		return nil, appErrors.NewNotFoundError("Credential not found")
	}
	return s.toResponseWithHeaders(credential), nil
}

func (s *providerCredentialService) List(ctx context.Context, orgID uuid.UUID) ([]*credentialsDomain.ProviderCredentialResponse, error) {
	credentials, err := s.repo.ListByOrganization(ctx, orgID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to list credentials", err)
	}

	responses := make([]*credentialsDomain.ProviderCredentialResponse, len(credentials))
	for i, cred := range credentials {
		responses[i] = cred.ToResponse() // Use safe response without decrypted headers
	}
	return responses, nil
}

func (s *providerCredentialService) Delete(ctx context.Context, id uuid.UUID, orgID uuid.UUID) error {
	// Get credential first for logging
	credential, err := s.repo.GetByID(ctx, id, orgID)
	if err != nil {
		if errors.Is(err, credentialsDomain.ErrCredentialNotFound) {
			return appErrors.NewNotFoundError("Credential not found")
		}
		return appErrors.NewInternalError("Failed to get credential", err)
	}

	if err := s.repo.Delete(ctx, id, orgID); err != nil {
		if errors.Is(err, credentialsDomain.ErrCredentialNotFound) {
			return appErrors.NewNotFoundError("Credential not found")
		}
		return appErrors.NewInternalError("Failed to delete credential", err)
	}

	s.logger.Info("provider credential deleted",
		"credential_id", id,
		"name", credential.Name,
		"adapter", credential.Adapter,
	)
	return nil
}

func (s *providerCredentialService) GetDecryptedByID(ctx context.Context, credentialID uuid.UUID, orgID uuid.UUID) (*credentialsDomain.DecryptedKeyConfig, error) {
	credential, err := s.repo.GetByID(ctx, credentialID, orgID)
	if err != nil {
		if errors.Is(err, credentialsDomain.ErrCredentialNotFound) {
			return nil, credentialsDomain.ErrCredentialNotFound
		}
		return nil, err
	}

	return s.decryptCredential(credential)
}

// toResponseWithHeaders converts a credential to a response DTO with decrypted headers.
// This is the preferred method for returning credentials to the frontend.
func (s *providerCredentialService) toResponseWithHeaders(credential *credentialsDomain.ProviderCredential) *credentialsDomain.ProviderCredentialResponse {
	resp := credential.ToResponse()

	// Decrypt headers if present
	if credential.Headers != "" {
		decryptedHeaders, err := s.encryptor.Decrypt(credential.Headers)
		if err != nil {
			s.logger.Warn("failed to decrypt headers for response",
				"error", err,
				"credential_id", credential.ID,
			)
		} else {
			var headers map[string]string
			if err := json.Unmarshal([]byte(decryptedHeaders), &headers); err != nil {
				s.logger.Warn("failed to parse decrypted headers for response",
					"error", err,
					"credential_id", credential.ID,
				)
			} else {
				resp.Headers = headers
			}
		}
	}

	return resp
}

func (s *providerCredentialService) decryptCredential(credential *credentialsDomain.ProviderCredential) (*credentialsDomain.DecryptedKeyConfig, error) {
	decryptedKey, err := s.encryptor.Decrypt(credential.EncryptedKey)
	if err != nil {
		s.logger.Error("failed to decrypt API key",
			"error", err,
			"credential_id", credential.ID,
		)
		return nil, credentialsDomain.ErrDecryptionFailed
	}

	config := &credentialsDomain.DecryptedKeyConfig{
		Provider: credential.Adapter,
		APIKey:   decryptedKey,
		Config:   credential.Config,
	}

	if credential.BaseURL != nil {
		config.BaseURL = *credential.BaseURL
	}

	// Decrypt custom headers if present
	if credential.Headers != "" {
		decryptedHeaders, err := s.encryptor.Decrypt(credential.Headers)
		if err != nil {
			s.logger.Warn("failed to decrypt custom headers",
				"error", err,
				"credential_id", credential.ID,
			)
		} else {
			var headers map[string]string
			if err := json.Unmarshal([]byte(decryptedHeaders), &headers); err != nil {
				s.logger.Warn("failed to parse decrypted headers",
					"error", err,
					"credential_id", credential.ID,
				)
			} else {
				config.Headers = headers
			}
		}
	}

	return config, nil
}

// GetExecutionConfig returns the key configuration for AI execution.
// Requires credential_id and validates that the credential's adapter matches.
// Returns ErrAdapterMismatch if the credential's adapter doesn't match the expected adapter.
// Returns ErrCredentialNotFound if the credential doesn't exist.
func (s *providerCredentialService) GetExecutionConfig(ctx context.Context, orgID uuid.UUID, credentialID uuid.UUID, adapter credentialsDomain.Provider) (*credentialsDomain.DecryptedKeyConfig, error) {
	config, err := s.GetDecryptedByID(ctx, credentialID, orgID)
	if err != nil {
		return nil, err // Don't convert - let caller handle
	}

	// Validate adapter match
	if config.Provider != adapter {
		return nil, credentialsDomain.NewAdapterMismatchError(string(adapter), string(config.Provider))
	}

	return config, nil
}

func (s *providerCredentialService) ValidateKey(ctx context.Context, adapter credentialsDomain.Provider, apiKey string, baseURL *string, config map[string]any) error {
	switch adapter {
	case credentialsDomain.ProviderOpenAI:
		return s.validateOpenAIKey(ctx, apiKey, baseURL)
	case credentialsDomain.ProviderAnthropic:
		return s.validateAnthropicKey(ctx, apiKey, baseURL)
	case credentialsDomain.ProviderAzure:
		return s.validateAzureKey(ctx, apiKey, baseURL, config)
	case credentialsDomain.ProviderGemini:
		return s.validateGeminiKey(ctx, apiKey)
	case credentialsDomain.ProviderOpenRouter:
		return s.validateOpenRouterKey(ctx, apiKey)
	case credentialsDomain.ProviderCustom:
		return s.validateCustomProvider(ctx, apiKey, baseURL)
	default:
		return credentialsDomain.NewInvalidAdapterError(string(adapter))
	}
}

func (s *providerCredentialService) TestConnection(ctx context.Context, req *credentialsDomain.TestConnectionRequest) *credentialsDomain.TestConnectionResponse {
	if !req.Adapter.IsValid() {
		return &credentialsDomain.TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid adapter: must be one of %v", credentialsDomain.ValidProviders()),
		}
	}

	if len(req.APIKey) < 10 {
		return &credentialsDomain.TestConnectionResponse{
			Success: false,
			Error:   "API key is too short",
		}
	}

	err := s.ValidateKey(ctx, req.Adapter, req.APIKey, req.BaseURL, req.Config)
	if err != nil {
		return &credentialsDomain.TestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		}
	}

	return &credentialsDomain.TestConnectionResponse{
		Success: true,
	}
}

func (s *providerCredentialService) validateOpenAIKey(ctx context.Context, apiKey string, baseURL *string) error {
	endpoint := "https://api.openai.com/v1/models"
	if baseURL != nil && *baseURL != "" {
		endpoint = strings.TrimSuffix(*baseURL, "/") + "/models"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return appErrors.NewInternalError("Failed to create validation request", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return appErrors.NewValidationError("API key validation failed", "Could not connect to OpenAI: "+err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return appErrors.NewValidationError("Invalid API key", "OpenAI rejected the API key")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return appErrors.NewValidationError("API key validation failed", fmt.Sprintf("OpenAI returned status %d: %s", resp.StatusCode, string(body)))
	}

	return nil
}

func (s *providerCredentialService) validateAnthropicKey(ctx context.Context, apiKey string, baseURL *string) error {
	endpoint := "https://api.anthropic.com/v1/messages"
	if baseURL != nil && *baseURL != "" {
		endpoint = strings.TrimSuffix(*baseURL, "/") + "/v1/messages"
	}

	reqBody := `{"model":"claude-3-haiku-20240307","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(reqBody))
	if err != nil {
		return appErrors.NewInternalError("Failed to create validation request", err)
	}

	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return appErrors.NewValidationError("API key validation failed", "Could not connect to Anthropic: "+err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return appErrors.NewValidationError("Invalid API key", "Anthropic rejected the API key")
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
		if resp.StatusCode == http.StatusBadRequest {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			var errResp struct {
				Error struct {
					Type string `json:"type"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &errResp) == nil {
				if errResp.Error.Type == "authentication_error" {
					return appErrors.NewValidationError("Invalid API key", "Anthropic authentication failed")
				}
			}
		}
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return appErrors.NewValidationError("API key validation failed", fmt.Sprintf("Anthropic returned status %d: %s", resp.StatusCode, string(body)))
}

func (s *providerCredentialService) validateAzureKey(ctx context.Context, apiKey string, baseURL *string, config map[string]any) error {
	if baseURL == nil || *baseURL == "" {
		return appErrors.NewValidationError("Base URL required", "Azure OpenAI requires a base URL (your Azure endpoint)")
	}

	deploymentID := ""
	if config != nil {
		if d, ok := config["deployment_id"].(string); ok {
			deploymentID = strings.TrimSpace(d)
		}
	}
	if deploymentID == "" {
		return appErrors.NewValidationError("Deployment ID required", "Azure OpenAI requires deployment_id to be configured")
	}

	apiVersion := "2024-10-21"
	if config != nil {
		if v, ok := config["api_version"].(string); ok && v != "" {
			apiVersion = v
		}
	}

	endpoint := strings.TrimSuffix(*baseURL, "/") + "/openai/deployments?api-version=" + apiVersion

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return appErrors.NewInternalError("Failed to create validation request", err)
	}

	req.Header.Set("api-key", apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return appErrors.NewValidationError("API key validation failed", "Could not connect to Azure OpenAI: "+err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return appErrors.NewValidationError("Invalid API key", "Azure OpenAI rejected the API key")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return appErrors.NewValidationError("API key validation failed", fmt.Sprintf("Azure OpenAI returned status %d: %s", resp.StatusCode, string(body)))
	}

	return nil
}

func (s *providerCredentialService) validateGeminiKey(ctx context.Context, apiKey string) error {
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models"

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return appErrors.NewInternalError("Failed to create validation request", err)
	}

	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return appErrors.NewValidationError("API key validation failed", "Could not connect to Google Gemini: "+err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return appErrors.NewValidationError("Invalid API key", "Google Gemini rejected the API key")
	}

	if resp.StatusCode == http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if strings.Contains(string(body), "API_KEY_INVALID") || strings.Contains(string(body), "INVALID_ARGUMENT") {
			return appErrors.NewValidationError("Invalid API key", "Google Gemini API key is invalid")
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return appErrors.NewValidationError("API key validation failed", fmt.Sprintf("Google Gemini returned status %d: %s", resp.StatusCode, string(body)))
	}

	return nil
}

func (s *providerCredentialService) validateOpenRouterKey(ctx context.Context, apiKey string) error {
	endpoint := "https://openrouter.ai/api/v1/models"

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return appErrors.NewInternalError("Failed to create validation request", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return appErrors.NewValidationError("API key validation failed", "Could not connect to OpenRouter: "+err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return appErrors.NewValidationError("Invalid API key", "OpenRouter rejected the API key")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return appErrors.NewValidationError("API key validation failed", fmt.Sprintf("OpenRouter returned status %d: %s", resp.StatusCode, string(body)))
	}

	return nil
}

func (s *providerCredentialService) validateCustomProvider(ctx context.Context, apiKey string, baseURL *string) error {
	if baseURL == nil || *baseURL == "" {
		return appErrors.NewValidationError("Base URL required", "Custom providers require a base URL")
	}

	endpoint := strings.TrimSuffix(*baseURL, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return appErrors.NewInternalError("Failed to create validation request", err)
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Warn("custom provider validation failed",
			"error", err,
			"base_url", *baseURL,
		)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return appErrors.NewValidationError("Invalid API key", "Custom provider rejected the API key")
	}

	return nil
}
