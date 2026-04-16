package dialects

import (
	"regexp"
	"sort"
	"strings"

	"github.com/cbroglie/mustache"

	promptDomain "brokle/internal/core/domain/prompt"
)

var (
	mustacheVarPattern          = regexp.MustCompile(`\{\{[{]?([a-zA-Z][a-zA-Z0-9_.]*)[\}]?\}\}`) // {{variable}} or {{{unescaped}}}
	mustacheSectionStartPattern = regexp.MustCompile(`\{\{[#^]([a-zA-Z][a-zA-Z0-9_.]*)\}\}`)      // {{#section}} or {{^inverted}}
	mustacheSectionEndPattern   = regexp.MustCompile(`\{\{/([a-zA-Z][a-zA-Z0-9_.]*)\}\}`)
	mustachePartialPattern      = regexp.MustCompile(`\{\{>([a-zA-Z][a-zA-Z0-9_.]*)\}\}`)
	mustacheCommentPattern      = regexp.MustCompile(`\{\{!.*?\}\}`)
)

type mustacheCompiler struct{}

func NewMustacheCompiler() promptDomain.DialectCompiler {
	return &mustacheCompiler{}
}

func (c *mustacheCompiler) Dialect() promptDomain.TemplateDialect {
	return promptDomain.DialectMustache
}

func (c *mustacheCompiler) ExtractVariables(content string) ([]string, error) {
	seen := make(map[string]bool)
	var vars []string

	for _, match := range mustacheVarPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			varName := match[1]
			rootVar := strings.Split(varName, ".")[0] // For nested paths like user.name, extract root
			if !seen[rootVar] {
				seen[rootVar] = true
				vars = append(vars, rootVar)
			}
		}
	}

	for _, match := range mustacheSectionStartPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			varName := match[1]
			rootVar := strings.Split(varName, ".")[0]
			if !seen[rootVar] {
				seen[rootVar] = true
				vars = append(vars, rootVar)
			}
		}
	}

	sort.Strings(vars)
	return vars, nil
}

func (c *mustacheCompiler) Compile(content string, variables map[string]any) (string, error) {
	if len(content) > promptDomain.MaxTemplateSize {
		return "", promptDomain.NewTemplateTooLargeError(len(content), promptDomain.MaxTemplateSize)
	}

	tmpl, err := mustache.ParseString(content)
	if err != nil {
		return "", promptDomain.NewDialectCompilationError("mustache", err.Error())
	}

	result, err := tmpl.Render(variables)
	if err != nil {
		return "", promptDomain.NewDialectCompilationError("mustache", err.Error())
	}

	return result, nil
}

func (c *mustacheCompiler) Validate(content string) (*promptDomain.ValidationResult, error) {
	result := promptDomain.NewValidationResult(true, promptDomain.DialectMustache)

	if len(content) > promptDomain.MaxTemplateSize {
		result.AddError(0, 0, "template exceeds maximum size limit", promptDomain.ErrCodeTemplateTooLarge)
		return result, nil
	}

	_, err := mustache.ParseString(content)
	if err != nil {
		result.AddError(0, 0, "mustache syntax error: "+err.Error(), promptDomain.ErrCodeInvalidSyntax)
		return result, nil
	}

	c.validateSections(result, content)

	vars, _ := c.ExtractVariables(content)
	if len(vars) > promptDomain.MaxVariables {
		result.AddWarning(0, 0, "template has many variables, consider simplifying", promptDomain.WarnCodeUnusedVariable)
	}

	if depth := c.calculateNestingDepth(content); depth > promptDomain.MaxNestingDepth {
		result.AddError(0, 0, "template nesting exceeds maximum depth", promptDomain.ErrCodeNestedTooDeep)
	}

	return result, nil
}

func (c *mustacheCompiler) validateSections(result *promptDomain.ValidationResult, content string) {
	lines := strings.Split(content, "\n")
	stack := make([]struct {
		name string
		line int
	}, 0)

	for lineNum, line := range lines {
		startMatches := mustacheSectionStartPattern.FindAllStringSubmatchIndex(line, -1)
		for _, match := range startMatches {
			if len(match) >= 4 {
				name := line[match[2]:match[3]]
				stack = append(stack, struct {
					name string
					line int
				}{name, lineNum + 1})
			}
		}

		endMatches := mustacheSectionEndPattern.FindAllStringSubmatchIndex(line, -1)
		for _, match := range endMatches {
			if len(match) >= 4 {
				name := line[match[2]:match[3]]
				if len(stack) == 0 {
					result.AddError(lineNum+1, match[0]+1,
						"closing tag without matching opening: {{/"+name+"}}",
						promptDomain.ErrCodeUnmatchedClosing)
				} else {
					top := stack[len(stack)-1]
					if top.name != name {
						result.AddError(lineNum+1, match[0]+1,
							"mismatched section tags: expected {{/"+top.name+"}} but found {{/"+name+"}}",
							promptDomain.ErrCodeInvalidSyntax)
					}
					stack = stack[:len(stack)-1]
				}
			}
		}
	}

	for _, unclosed := range stack {
		result.AddError(unclosed.line, 0,
			"unclosed section: {{#"+unclosed.name+"}} or {{^"+unclosed.name+"}}",
			promptDomain.ErrCodeUnmatchedOpening)
	}
}

func (c *mustacheCompiler) calculateNestingDepth(content string) int {
	maxDepth := 0
	currentDepth := 0

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		startMatches := mustacheSectionStartPattern.FindAllString(line, -1)
		endMatches := mustacheSectionEndPattern.FindAllString(line, -1)

		currentDepth += len(startMatches)
		if currentDepth > maxDepth {
			maxDepth = currentDepth
		}
		currentDepth -= len(endMatches)
	}

	return maxDepth
}
