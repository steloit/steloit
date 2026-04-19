package errors

// Domain error codes. Every value is a snake_case, namespace-prefixed
// identifier shipped in AppError.Code (and on the JSON wire as
// `error.code`). The namespace prefix conveys the originating subsystem
// without forcing a hierarchical schema; the format matches OpenAI's
// `error.code` (`invalid_api_key`, `model_not_found`) and Stripe's
// (`card_declined`, `resource_missing`) — the SDK ecosystem your
// consumers already use.
//
// Codes are an OPEN enumeration: adding a new value is not a breaking
// change, and SDK clients SHOULD NOT switch on Code without a default
// arm. Switch on AppError.Type (closed enum) for routing logic; use Code
// for observability, alerting, and per-error human messages.
const (
	// Authentication & authorization
	CodeInvalidCredentials      = "auth_invalid_credentials"
	CodeTokenExpired            = "auth_token_expired"
	CodeTokenInvalid            = "auth_token_invalid"
	CodeInsufficientPermissions = "auth_insufficient_permissions"
	CodeAPIKeyInvalid           = "auth_api_key_invalid"
	CodeSessionExpired          = "auth_session_expired"

	// User
	CodeUserNotFound         = "user_not_found"
	CodeUserAlreadyExists    = "user_already_exists"
	CodeUserInactive         = "user_inactive"
	CodeUserEmailNotVerified = "user_email_not_verified"

	// Organization
	CodeOrganizationNotFound      = "organization_not_found"
	CodeOrganizationInactive      = "organization_inactive"
	CodeOrganizationLimitExceeded = "organization_limit_exceeded"

	// Project
	CodeProjectNotFound      = "project_not_found"
	CodeProjectInactive      = "project_inactive"
	CodeProjectLimitExceeded = "project_limit_exceeded"

	// Billing
	CodeInsufficientCredits   = "billing_insufficient_credits"
	CodeQuotaExceeded         = "quota_exceeded"
	CodePaymentMethodRequired = "billing_payment_method_required"
	CodeSubscriptionInactive  = "billing_subscription_inactive"
	CodeInvoiceNotFound       = "billing_invoice_not_found"

	// AI routing & providers
	CodeProviderNotFound     = "provider_not_found"
	CodeProviderUnavailable  = "provider_unavailable"
	CodeProviderRateLimit    = "provider_rate_limit"
	CodeProviderError        = "provider_error"
	CodeModelNotSupported    = "model_not_supported"
	CodeRoutingConfigInvalid = "routing_config_invalid"

	// Analytics & observability
	CodeMetricsUnavailable    = "metrics_unavailable"
	CodeAnalyticsQueryFailed  = "analytics_query_failed"
	CodeDataRetentionExceeded = "data_retention_exceeded"

	// WebSocket & real-time
	CodeWebSocketConnectionFailed = "websocket_connection_failed"
	CodeWebSocketAuthFailed       = "websocket_auth_failed"
	CodeRealtimeEventFailed       = "realtime_event_failed"

	// Validation specifics
	CodeInvalidInput         = "invalid_input"
	CodeRequiredFieldMissing = "required_field_missing"
	CodeInvalidFormat        = "invalid_format"
	CodeValueOutOfRange      = "value_out_of_range"

	// Configuration
	CodeConfigNotFound  = "config_not_found"
	CodeConfigInvalid   = "config_invalid"
	CodeFeatureDisabled = "feature_disabled"

	// External services
	CodeExternalServiceUnavailable = "external_service_unavailable"
	CodeExternalServiceTimeout     = "external_service_timeout"
	CodeExternalServiceRateLimit   = "external_service_rate_limit"
)

// codeToMessage maps domain codes to default human-readable messages.
// Used by GetCodeMessage when a constructor needs to derive a message
// from a bare code (rare — typed constructors take an explicit message
// argument).
var codeToMessage = map[string]string{
	CodeInvalidCredentials:      "Invalid username or password",
	CodeTokenExpired:            "Access token has expired",
	CodeTokenInvalid:            "Invalid or malformed token",
	CodeInsufficientPermissions: "Insufficient permissions to perform this action",
	CodeAPIKeyInvalid:           "Invalid API key",
	CodeSessionExpired:          "Session has expired",

	CodeUserNotFound:         "User not found",
	CodeUserAlreadyExists:    "User already exists",
	CodeUserInactive:         "User account is inactive",
	CodeUserEmailNotVerified: "Email address not verified",

	CodeOrganizationNotFound:      "Organization not found",
	CodeOrganizationInactive:      "Organization is inactive",
	CodeOrganizationLimitExceeded: "Organization limit exceeded",

	CodeProjectNotFound:      "Project not found",
	CodeProjectInactive:      "Project is inactive",
	CodeProjectLimitExceeded: "Project limit exceeded",

	CodeInsufficientCredits:   "Insufficient credits to complete request",
	CodeQuotaExceeded:         "Usage quota exceeded",
	CodePaymentMethodRequired: "Payment method required",
	CodeSubscriptionInactive:  "Subscription is inactive",
	CodeInvoiceNotFound:       "Invoice not found",

	CodeProviderNotFound:     "AI provider not found",
	CodeProviderUnavailable:  "AI provider is currently unavailable",
	CodeProviderRateLimit:    "AI provider rate limit exceeded",
	CodeProviderError:        "AI provider returned an error",
	CodeModelNotSupported:    "AI model not supported by provider",
	CodeRoutingConfigInvalid: "Invalid routing configuration",

	CodeMetricsUnavailable:    "Metrics data unavailable",
	CodeAnalyticsQueryFailed:  "Analytics query failed",
	CodeDataRetentionExceeded: "Data retention period exceeded",

	CodeWebSocketConnectionFailed: "WebSocket connection failed",
	CodeWebSocketAuthFailed:       "WebSocket authentication failed",
	CodeRealtimeEventFailed:       "Real-time event processing failed",

	CodeInvalidInput:         "Invalid input provided",
	CodeRequiredFieldMissing: "Required field is missing",
	CodeInvalidFormat:        "Invalid format",
	CodeValueOutOfRange:      "Value is out of acceptable range",

	CodeConfigNotFound:  "Configuration not found",
	CodeConfigInvalid:   "Invalid configuration",
	CodeFeatureDisabled: "Feature is disabled",

	CodeExternalServiceUnavailable: "External service is unavailable",
	CodeExternalServiceTimeout:     "External service request timed out",
	CodeExternalServiceRateLimit:   "External service rate limit exceeded",
}

// GetCodeMessage returns the default human-readable message for a domain
// code, or "An error occurred" when the code is unknown.
func GetCodeMessage(code string) string {
	if msg, ok := codeToMessage[code]; ok {
		return msg
	}
	return "An error occurred"
}
