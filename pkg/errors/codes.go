package errors

import "strings"

// HTTP status codes for different error types
const (
	StatusValidationError      = 400
	StatusNotFoundError        = 404
	StatusConflictError        = 409
	StatusUnauthorizedError    = 401
	StatusForbiddenError       = 403
	StatusInternalError        = 500
	StatusBadRequestError      = 400
	StatusServiceUnavailable   = 503
	StatusNotImplementedError  = 501
	StatusRateLimitError       = 429
	StatusPaymentRequiredError = 402
	StatusQuotaExceededError   = 429
	StatusAIProviderError      = 502
)

// Business error codes for the Brokle platform
const (
	// Authentication & Authorization
	CodeInvalidCredentials      = "AUTH_INVALID_CREDENTIALS"
	CodeTokenExpired            = "AUTH_TOKEN_EXPIRED"
	CodeTokenInvalid            = "AUTH_TOKEN_INVALID"
	CodeInsufficientPermissions = "AUTH_INSUFFICIENT_PERMISSIONS"
	CodeAPIKeyInvalid           = "AUTH_API_KEY_INVALID"
	CodeSessionExpired          = "AUTH_SESSION_EXPIRED"

	// User Management
	CodeUserNotFound         = "USER_NOT_FOUND"
	CodeUserAlreadyExists    = "USER_ALREADY_EXISTS"
	CodeUserInactive         = "USER_INACTIVE"
	CodeUserEmailNotVerified = "USER_EMAIL_NOT_VERIFIED"

	// Organization Management
	CodeOrganizationNotFound      = "ORG_NOT_FOUND"
	CodeOrganizationInactive      = "ORG_INACTIVE"
	CodeOrganizationLimitExceeded = "ORG_LIMIT_EXCEEDED"

	// Project Management
	CodeProjectNotFound      = "PROJECT_NOT_FOUND"
	CodeProjectInactive      = "PROJECT_INACTIVE"
	CodeProjectLimitExceeded = "PROJECT_LIMIT_EXCEEDED"

	// Billing & Quotas
	CodeInsufficientCredits   = "BILLING_INSUFFICIENT_CREDITS"
	CodeQuotaExceeded         = "BILLING_QUOTA_EXCEEDED"
	CodePaymentMethodRequired = "BILLING_PAYMENT_METHOD_REQUIRED"
	CodeSubscriptionInactive  = "BILLING_SUBSCRIPTION_INACTIVE"
	CodeInvoiceNotFound       = "BILLING_INVOICE_NOT_FOUND"

	// AI Routing & Providers
	CodeProviderNotFound     = "ROUTING_PROVIDER_NOT_FOUND"
	CodeProviderUnavailable  = "ROUTING_PROVIDER_UNAVAILABLE"
	CodeProviderRateLimit    = "ROUTING_PROVIDER_RATE_LIMIT"
	CodeProviderError        = "ROUTING_PROVIDER_ERROR"
	CodeModelNotSupported    = "ROUTING_MODEL_NOT_SUPPORTED"
	CodeRoutingConfigInvalid = "ROUTING_CONFIG_INVALID"

	// Analytics & Observability
	CodeMetricsUnavailable    = "METRICS_UNAVAILABLE"
	CodeAnalyticsQueryFailed  = "ANALYTICS_QUERY_FAILED"
	CodeDataRetentionExceeded = "ANALYTICS_DATA_RETENTION_EXCEEDED"

	// WebSocket & Real-time
	CodeWebSocketConnectionFailed = "WS_CONNECTION_FAILED"
	CodeWebSocketAuthFailed       = "WS_AUTH_FAILED"
	CodeRealtimeEventFailed       = "REALTIME_EVENT_FAILED"

	// Validation
	CodeInvalidInput         = "VALIDATION_INVALID_INPUT"
	CodeRequiredFieldMissing = "VALIDATION_REQUIRED_FIELD_MISSING"
	CodeInvalidFormat        = "VALIDATION_INVALID_FORMAT"
	CodeValueOutOfRange      = "VALIDATION_VALUE_OUT_OF_RANGE"

	// Configuration
	CodeConfigNotFound  = "CONFIG_NOT_FOUND"
	CodeConfigInvalid   = "CONFIG_INVALID"
	CodeFeatureDisabled = "CONFIG_FEATURE_DISABLED"

	// External Services
	CodeExternalServiceUnavailable = "EXTERNAL_SERVICE_UNAVAILABLE"
	CodeExternalServiceTimeout     = "EXTERNAL_SERVICE_TIMEOUT"
	CodeExternalServiceRateLimit   = "EXTERNAL_SERVICE_RATE_LIMIT"
)

// ErrorCodeToMessage maps error codes to human-readable messages
var ErrorCodeToMessage = map[string]string{
	// Authentication & Authorization
	CodeInvalidCredentials:      "Invalid username or password",
	CodeTokenExpired:            "Access token has expired",
	CodeTokenInvalid:            "Invalid or malformed token",
	CodeInsufficientPermissions: "Insufficient permissions to perform this action",
	CodeAPIKeyInvalid:           "Invalid API key",
	CodeSessionExpired:          "Session has expired",

	// User Management
	CodeUserNotFound:         "User not found",
	CodeUserAlreadyExists:    "User already exists",
	CodeUserInactive:         "User account is inactive",
	CodeUserEmailNotVerified: "Email address not verified",

	// Organization Management
	CodeOrganizationNotFound:      "Organization not found",
	CodeOrganizationInactive:      "Organization is inactive",
	CodeOrganizationLimitExceeded: "Organization limit exceeded",

	// Project Management
	CodeProjectNotFound:      "Project not found",
	CodeProjectInactive:      "Project is inactive",
	CodeProjectLimitExceeded: "Project limit exceeded",

	// Billing & Quotas
	CodeInsufficientCredits:   "Insufficient credits to complete request",
	CodeQuotaExceeded:         "Usage quota exceeded",
	CodePaymentMethodRequired: "Payment method required",
	CodeSubscriptionInactive:  "Subscription is inactive",
	CodeInvoiceNotFound:       "Invoice not found",

	// AI Routing & Providers
	CodeProviderNotFound:     "AI provider not found",
	CodeProviderUnavailable:  "AI provider is currently unavailable",
	CodeProviderRateLimit:    "AI provider rate limit exceeded",
	CodeProviderError:        "AI provider returned an error",
	CodeModelNotSupported:    "AI model not supported by provider",
	CodeRoutingConfigInvalid: "Invalid routing configuration",

	// Analytics & Observability
	CodeMetricsUnavailable:    "Metrics data unavailable",
	CodeAnalyticsQueryFailed:  "Analytics query failed",
	CodeDataRetentionExceeded: "Data retention period exceeded",

	// WebSocket & Real-time
	CodeWebSocketConnectionFailed: "WebSocket connection failed",
	CodeWebSocketAuthFailed:       "WebSocket authentication failed",
	CodeRealtimeEventFailed:       "Real-time event processing failed",

	// Validation
	CodeInvalidInput:         "Invalid input provided",
	CodeRequiredFieldMissing: "Required field is missing",
	CodeInvalidFormat:        "Invalid format",
	CodeValueOutOfRange:      "Value is out of acceptable range",

	// Configuration
	CodeConfigNotFound:  "Configuration not found",
	CodeConfigInvalid:   "Invalid configuration",
	CodeFeatureDisabled: "Feature is disabled",

	// External Services
	CodeExternalServiceUnavailable: "External service is unavailable",
	CodeExternalServiceTimeout:     "External service request timed out",
	CodeExternalServiceRateLimit:   "External service rate limit exceeded",
}

// GetErrorMessage returns a human-readable message for the given error code
func GetErrorMessage(code string) string {
	if message, exists := ErrorCodeToMessage[code]; exists {
		return message
	}
	return "An error occurred"
}

// NewErrorWithCode creates a new AppError whose AppErrorType is inferred from
// the code's prefix. Safe for codes of any length (including empty).
//
// Deprecated: prefer the typed constructors (NewAuthError, NewValidationError,
// NewNotFoundError, etc.) which carry their type explicitly and require no
// prefix parsing.
func NewErrorWithCode(code string, details string) *AppError {
	var errorType AppErrorType
	switch {
	case strings.HasPrefix(code, "AUTH"):
		if code == CodeInsufficientPermissions {
			errorType = ForbiddenError
		} else {
			errorType = UnauthorizedError
		}
	case strings.HasPrefix(code, "USER"),
		strings.HasPrefix(code, "ORG"),
		strings.HasPrefix(code, "PROJECT"),
		strings.HasPrefix(code, "CONFIG"):
		errorType = NotFoundError
	case strings.HasPrefix(code, "BILLING"):
		if code == CodeQuotaExceeded || code == CodeInsufficientCredits {
			errorType = PaymentRequiredError
		} else {
			errorType = BadRequestError
		}
	case strings.HasPrefix(code, "ROUTING"):
		if code == CodeProviderUnavailable {
			errorType = ServiceUnavailable
		} else {
			errorType = AIProviderError
		}
	case strings.HasPrefix(code, "VALIDATION"):
		errorType = ValidationError
	case strings.HasPrefix(code, "EXTERNAL"):
		errorType = ServiceUnavailable
	default:
		errorType = InternalError
	}

	return NewAppError(errorType, GetErrorMessage(code), details, nil)
}
