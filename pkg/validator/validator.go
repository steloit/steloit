package validator

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	if len(errs) == 0 {
		return ""
	}

	var messages []string
	for _, err := range errs {
		messages = append(messages, err.Error())
	}

	return strings.Join(messages, "; ")
}

// HasErrors returns true if there are validation errors
func (errs ValidationErrors) HasErrors() bool {
	return len(errs) > 0
}

// Add adds a validation error
func (errs *ValidationErrors) Add(field, message string, value ...string) {
	err := ValidationError{
		Field:   field,
		Message: message,
	}
	if len(value) > 0 {
		err.Value = value[0]
	}
	*errs = append(*errs, err)
}

// Validator provides validation functions
type Validator struct {
	errors ValidationErrors
}

// New creates a new validator instance
func New() *Validator {
	return &Validator{}
}

// HasErrors returns true if there are validation errors
func (v *Validator) HasErrors() bool {
	return v.errors.HasErrors()
}

// Errors returns all validation errors
func (v *Validator) Errors() ValidationErrors {
	return v.errors
}

// Clear clears all validation errors
func (v *Validator) Clear() {
	v.errors = ValidationErrors{}
}

// Required validates that a field is not empty
func (v *Validator) Required(field string, value any, message ...string) *Validator {
	msg := "is required"
	if len(message) > 0 {
		msg = message[0]
	}

	if isEmpty(value) {
		v.errors.Add(field, msg, fmt.Sprintf("%v", value))
	}

	return v
}

// MinLength validates minimum string length
func (v *Validator) MinLength(field string, value string, minLen int, message ...string) *Validator {
	msg := fmt.Sprintf("must be at least %d characters long", minLen)
	if len(message) > 0 {
		msg = message[0]
	}

	if len(value) < minLen {
		v.errors.Add(field, msg, value)
	}

	return v
}

// MaxLength validates maximum string length
func (v *Validator) MaxLength(field string, value string, maxLen int, message ...string) *Validator {
	msg := fmt.Sprintf("must not exceed %d characters", maxLen)
	if len(message) > 0 {
		msg = message[0]
	}

	if len(value) > maxLen {
		v.errors.Add(field, msg, value)
	}

	return v
}

// Length validates exact string length
func (v *Validator) Length(field string, value string, length int, message ...string) *Validator {
	msg := fmt.Sprintf("must be exactly %d characters long", length)
	if len(message) > 0 {
		msg = message[0]
	}

	if len(value) != length {
		v.errors.Add(field, msg, value)
	}

	return v
}

// Min validates minimum numeric value
func (v *Validator) Min(field string, value any, min float64, message ...string) *Validator {
	msg := fmt.Sprintf("must be at least %v", min)
	if len(message) > 0 {
		msg = message[0]
	}

	val, ok := toFloat64(value)
	if !ok {
		v.errors.Add(field, "must be a valid number", fmt.Sprintf("%v", value))
		return v
	}

	if val < min {
		v.errors.Add(field, msg, fmt.Sprintf("%v", value))
	}

	return v
}

// Max validates maximum numeric value
func (v *Validator) Max(field string, value any, max float64, message ...string) *Validator {
	msg := fmt.Sprintf("must not exceed %v", max)
	if len(message) > 0 {
		msg = message[0]
	}

	val, ok := toFloat64(value)
	if !ok {
		v.errors.Add(field, "must be a valid number", fmt.Sprintf("%v", value))
		return v
	}

	if val > max {
		v.errors.Add(field, msg, fmt.Sprintf("%v", value))
	}

	return v
}

// Range validates numeric value within range
func (v *Validator) Range(field string, value any, min, max float64, message ...string) *Validator {
	msg := fmt.Sprintf("must be between %v and %v", min, max)
	if len(message) > 0 {
		msg = message[0]
	}

	val, ok := toFloat64(value)
	if !ok {
		v.errors.Add(field, "must be a valid number", fmt.Sprintf("%v", value))
		return v
	}

	if val < min || val > max {
		v.errors.Add(field, msg, fmt.Sprintf("%v", value))
	}

	return v
}

// Email validates email format
func (v *Validator) Email(field string, value string, message ...string) *Validator {
	msg := "must be a valid email address"
	if len(message) > 0 {
		msg = message[0]
	}

	if !IsValidEmail(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// URL validates URL format
func (v *Validator) URL(field string, value string, message ...string) *Validator {
	msg := "must be a valid URL"
	if len(message) > 0 {
		msg = message[0]
	}

	if !IsValidURL(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// Pattern validates against regular expression
func (v *Validator) Pattern(field string, value string, pattern string, message ...string) *Validator {
	msg := "must match pattern " + pattern
	if len(message) > 0 {
		msg = message[0]
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		v.errors.Add(field, "invalid pattern: "+err.Error(), value)
		return v
	}

	if !re.MatchString(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// AlphaNumeric validates alphanumeric characters only
func (v *Validator) AlphaNumeric(field string, value string, message ...string) *Validator {
	msg := "must contain only letters and numbers"
	if len(message) > 0 {
		msg = message[0]
	}

	if !IsAlphaNumeric(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// Alpha validates alphabetic characters only
func (v *Validator) Alpha(field string, value string, message ...string) *Validator {
	msg := "must contain only letters"
	if len(message) > 0 {
		msg = message[0]
	}

	if !IsAlpha(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// Numeric validates numeric characters only
func (v *Validator) Numeric(field string, value string, message ...string) *Validator {
	msg := "must be a valid number"
	if len(message) > 0 {
		msg = message[0]
	}

	if !IsNumeric(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// OneOf validates value is one of allowed values
func (v *Validator) OneOf(field string, value string, allowed []string, message ...string) *Validator {
	msg := "must be one of: " + strings.Join(allowed, ", ")
	if len(message) > 0 {
		msg = message[0]
	}

	for _, allowedValue := range allowed {
		if value == allowedValue {
			return v
		}
	}

	v.errors.Add(field, msg, value)
	return v
}

// Date validates date format
func (v *Validator) Date(field string, value string, format string, message ...string) *Validator {
	msg := "must be a valid date in format " + format
	if len(message) > 0 {
		msg = message[0]
	}

	if format == "" {
		format = "2006-01-02"
	}

	if _, err := time.Parse(format, value); err != nil {
		v.errors.Add(field, msg, value)
	}

	return v
}

// UUID validates UUID format
func (v *Validator) UUID(field string, value string, message ...string) *Validator {
	msg := "must be a valid UUID"
	if len(message) > 0 {
		msg = message[0]
	}

	if !IsValidUUID(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// JSON validates JSON format
func (v *Validator) JSON(field string, value string, message ...string) *Validator {
	msg := "must be valid JSON"
	if len(message) > 0 {
		msg = message[0]
	}

	if !IsValidJSON(value) {
		v.errors.Add(field, msg, value)
	}

	return v
}

// Custom validates using custom function
func (v *Validator) Custom(field string, value any, fn func(any) bool, message string) *Validator {
	if !fn(value) {
		v.errors.Add(field, message, fmt.Sprintf("%v", value))
	}

	return v
}

// Conditional validates only if condition is true
func (v *Validator) Conditional(condition bool, fn func(*Validator) *Validator) *Validator {
	if condition {
		return fn(v)
	}
	return v
}

// Helper functions

func isEmpty(value any) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		return strings.TrimSpace(v.String()) == ""
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr:
		return v.IsNil()
	default:
		return false
	}
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// Validation functions

// IsValidEmail validates email format
func IsValidEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

// IsValidURL validates URL format
func IsValidURL(urlStr string) bool {
	_, err := url.ParseRequestURI(urlStr)
	return err == nil
}

// IsAlphaNumeric validates alphanumeric characters
func IsAlphaNumeric(s string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	return re.MatchString(s)
}

// IsAlpha validates alphabetic characters only
func IsAlpha(s string) bool {
	re := regexp.MustCompile(`^[a-zA-Z]+$`)
	return re.MatchString(s)
}

// IsNumeric validates numeric characters
func IsNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// IsValidUUID validates UUID format (all versions including v7)
func IsValidUUID(s string) bool {
	re := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	return re.MatchString(s)
}

// IsValidJSON validates JSON format
func IsValidJSON(s string) bool {
	// Simple JSON validation - could be enhanced with actual parsing
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}

	// Must start and end with { } or [ ]
	return (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"))
}

// Quick validation functions for common cases

// ValidateRequired validates required field
func ValidateRequired(field string, value any) error {
	v := New()
	v.Required(field, value)
	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateEmail validates email field
func ValidateEmail(field, email string) error {
	v := New()
	v.Required(field, email).Email(field, email)
	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}

// ValidateURL validates URL field
func ValidateURL(field, urlStr string) error {
	v := New()
	v.Required(field, urlStr).URL(field, urlStr)
	if v.HasErrors() {
		return v.Errors()
	}
	return nil
}
