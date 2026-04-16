// Package prompt provides the prompt management domain model.
package prompt

// TemplateDialect represents the template engine dialect to use for compilation.
type TemplateDialect string

const (
	// DialectSimple uses basic {{variable}} substitution only (backward compatible).
	DialectSimple TemplateDialect = "simple"

	// DialectMustache provides full Mustache support with conditionals, loops, and partials.
	DialectMustache TemplateDialect = "mustache"

	// DialectJinja2 provides Jinja2/Nunjucks support with filters, inheritance, and macros.
	DialectJinja2 TemplateDialect = "jinja2"
)

func ValidDialects() []TemplateDialect {
	return []TemplateDialect{DialectSimple, DialectMustache, DialectJinja2}
}

func (d TemplateDialect) IsValid() bool {
	switch d {
	case DialectSimple, DialectMustache, DialectJinja2:
		return true
	default:
		return false
	}
}

func (d TemplateDialect) String() string {
	return string(d)
}

// DialectAuto is used when the dialect should be auto-detected from the template content.
const DialectAuto TemplateDialect = "auto"

// ValidationResult contains the result of template syntax validation.
type ValidationResult struct {
	Valid    bool            `json:"valid"`
	Dialect  TemplateDialect `json:"dialect"`
	Errors   []SyntaxError   `json:"errors,omitempty"`
	Warnings []SyntaxWarning `json:"warnings,omitempty"`
}

// SyntaxError represents a template syntax error with location information.
type SyntaxError struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Code    string `json:"code"` // Error code for programmatic handling
}

// SyntaxWarning represents a non-fatal template warning.
type SyntaxWarning struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// TemplateMetadata contains metadata about a template stored in the _meta field.
type TemplateMetadata struct {
	Dialect        TemplateDialect `json:"dialect,omitempty"`
	DialectVersion string          `json:"dialect_version,omitempty"`
}

// CompilationOptions provides options for template compilation.
type CompilationOptions struct {
	Dialect         TemplateDialect `json:"dialect,omitempty"`          // Explicit dialect (if not auto-detect)
	StrictMode      bool            `json:"strict_mode,omitempty"`      // Fail if any variable is missing
	PreserveMissing bool            `json:"preserve_missing,omitempty"` // Keep {{var}} if var not provided
}

// DialectCompiler defines the interface for a specific template dialect compiler.
// Each dialect (simple, mustache, jinja2) implements this interface.
type DialectCompiler interface {
	// Dialect returns the dialect this compiler handles.
	Dialect() TemplateDialect

	// ExtractVariables extracts all variable names from the template content.
	ExtractVariables(content string) ([]string, error)

	// Compile renders the template with the provided variables.
	// Variables can be any type; the compiler handles serialization.
	Compile(content string, variables map[string]any) (string, error)

	// Validate checks the template syntax and returns detailed error information.
	Validate(content string) (*ValidationResult, error)
}

// DialectRegistry provides access to dialect compilers by dialect type.
type DialectRegistry interface {
	// Get returns the compiler for the specified dialect.
	Get(dialect TemplateDialect) (DialectCompiler, error)

	// Detect auto-detects the dialect from template content.
	Detect(content string) TemplateDialect

	// SupportedDialects returns all supported dialect types.
	SupportedDialects() []TemplateDialect
}

// Error codes for syntax validation
const (
	ErrCodeUnmatchedOpening    = "UNMATCHED_OPENING"
	ErrCodeUnmatchedClosing    = "UNMATCHED_CLOSING"
	ErrCodeInvalidVariableName = "INVALID_VARIABLE_NAME"
	ErrCodeInvalidSyntax       = "INVALID_SYNTAX"
	ErrCodeNestedTooDeep       = "NESTED_TOO_DEEP"
	ErrCodeUnknownFilter       = "UNKNOWN_FILTER"
	ErrCodeUnknownBlock        = "UNKNOWN_BLOCK"
	// Note: ErrCodeTemplateTooLarge is defined in errors.go
)

// Warning codes for syntax validation
const (
	WarnCodeUnusedVariable     = "UNUSED_VARIABLE"
	WarnCodeDeprecatedSyntax   = "DEPRECATED_SYNTAX"
	WarnCodeEmptyBlock         = "EMPTY_BLOCK"
	WarnCodePotentialInjection = "POTENTIAL_INJECTION"
)

// Template limits for security and performance
const (
	MaxTemplateSize = 100 * 1024 // 100KB max template size
	MaxNestingDepth = 10         // Max nesting depth for loops/conditionals
	MaxVariables    = 100        // Max variables per template
)

func NewValidationResult(valid bool, dialect TemplateDialect) *ValidationResult {
	return &ValidationResult{
		Valid:   valid,
		Dialect: dialect,
	}
}

func (r *ValidationResult) AddError(line, column int, message, code string) {
	r.Valid = false
	r.Errors = append(r.Errors, SyntaxError{
		Line:    line,
		Column:  column,
		Message: message,
		Code:    code,
	})
}

func (r *ValidationResult) AddWarning(line, column int, message, code string) {
	r.Warnings = append(r.Warnings, SyntaxWarning{
		Line:    line,
		Column:  column,
		Message: message,
		Code:    code,
	})
}

func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}
