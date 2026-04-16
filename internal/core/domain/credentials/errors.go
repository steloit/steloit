// Package credentials provides the credentials management domain model.
//
// The credentials domain handles secure storage of user-provided LLM API keys
// with AES-256-GCM encryption at rest. Keys are scoped per-project and support
// multiple providers (OpenAI, Anthropic, etc.).
package credentials

import (
	"errors"
	"fmt"
)

// Domain errors for credential management
var (
	// Credential errors
	ErrCredentialNotFound = errors.New("credential not found")
	ErrCredentialExists   = errors.New("credential with this name already exists")
	ErrInvalidProvider    = errors.New("invalid adapter type")
	ErrInvalidAPIKey      = errors.New("invalid API key format")
	ErrNoKeyConfigured    = errors.New("no API key configured")
	ErrAdapterMismatch    = errors.New("credential adapter mismatch")

	// Encryption errors
	ErrEncryptionFailed     = errors.New("failed to encrypt API key")
	ErrDecryptionFailed     = errors.New("failed to decrypt API key")
	ErrEncryptionKeyMissing = errors.New("encryption key not configured")

	// Validation errors
	ErrAPIKeyValidationFailed = errors.New("API key validation failed")
	ErrInvalidBaseURL         = errors.New("invalid base URL")
)

// Error codes for structured API responses
const (
	ErrCodeCredentialNotFound = "CREDENTIAL_NOT_FOUND"
	ErrCodeCredentialExists   = "CREDENTIAL_EXISTS"
	ErrCodeInvalidProvider    = "INVALID_PROVIDER"
	ErrCodeInvalidAPIKey      = "INVALID_API_KEY"
	ErrCodeNoKeyConfigured    = "NO_KEY_CONFIGURED"
	ErrCodeEncryptionFailed   = "ENCRYPTION_FAILED"
	ErrCodeValidationFailed   = "VALIDATION_FAILED"
)

func NewCredentialNotFoundError(identifier string, projectID string) error {
	return fmt.Errorf("%w: %s in project=%s", ErrCredentialNotFound, identifier, projectID)
}

func NewCredentialExistsError(name string, projectID string) error {
	return fmt.Errorf("%w: name=%s in project=%s", ErrCredentialExists, name, projectID)
}

func NewInvalidAdapterError(adapter string) error {
	return fmt.Errorf("%w: '%s' (must be one of: openai, anthropic, azure, gemini, openrouter, custom)", ErrInvalidProvider, adapter)
}

func NewAPIKeyValidationError(adapter string, details string) error {
	return fmt.Errorf("%w: %s - %s", ErrAPIKeyValidationFailed, adapter, details)
}

func NewNoKeyConfiguredError(adapter string) error {
	return fmt.Errorf("%w: %s (set via project settings or environment variable)", ErrNoKeyConfigured, adapter)
}

func NewAdapterMismatchError(expected, actual string) error {
	return fmt.Errorf("%w: expected '%s', credential uses '%s'", ErrAdapterMismatch, expected, actual)
}

func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrCredentialNotFound)
}

func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidProvider) ||
		errors.Is(err, ErrInvalidAPIKey) ||
		errors.Is(err, ErrAPIKeyValidationFailed) ||
		errors.Is(err, ErrInvalidBaseURL) ||
		errors.Is(err, ErrAdapterMismatch)
}

func IsEncryptionError(err error) bool {
	return errors.Is(err, ErrEncryptionFailed) ||
		errors.Is(err, ErrDecryptionFailed) ||
		errors.Is(err, ErrEncryptionKeyMissing)
}

func IsConflictError(err error) bool {
	return errors.Is(err, ErrCredentialExists)
}
