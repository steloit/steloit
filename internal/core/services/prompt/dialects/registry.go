// Package dialects provides template dialect compilers and a registry for managing them.
package dialects

import (
	"regexp"
	"sync"

	promptDomain "brokle/internal/core/domain/prompt"
)

type registry struct {
	compilers map[promptDomain.TemplateDialect]promptDomain.DialectCompiler
	mu        sync.RWMutex
}

func NewRegistry() promptDomain.DialectRegistry {
	r := &registry{
		compilers: make(map[promptDomain.TemplateDialect]promptDomain.DialectCompiler),
	}
	r.register(NewSimpleCompiler())
	r.register(NewMustacheCompiler())
	r.register(NewJinja2Compiler())

	return r
}

func (r *registry) register(compiler promptDomain.DialectCompiler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compilers[compiler.Dialect()] = compiler
}

func (r *registry) Get(dialect promptDomain.TemplateDialect) (promptDomain.DialectCompiler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	compiler, ok := r.compilers[dialect]
	if !ok {
		return nil, promptDomain.NewUnsupportedDialectError(string(dialect))
	}
	return compiler, nil
}

func (r *registry) Detect(content string) promptDomain.TemplateDialect {
	return DetectDialect(content)
}

func (r *registry) SupportedDialects() []promptDomain.TemplateDialect {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dialects := make([]promptDomain.TemplateDialect, 0, len(r.compilers))
	for dialect := range r.compilers {
		dialects = append(dialects, dialect)
	}
	return dialects
}

var (
	detectJinja2BlockPattern     = regexp.MustCompile(`\{%\s*\w+`)            // {% ... %} blocks
	detectJinja2FilterPattern    = regexp.MustCompile(`\{\{[^}]+\|[^}]+\}\}`) // {{ ... | filter }}
	detectJinja2DotPattern       = regexp.MustCompile(`\{\{\s*\w+\.\w+`)      // {{ var.attr }}
	detectMustacheSectionPattern = regexp.MustCompile(`\{\{[#^>/]`)           // {{#...}}, {{^...}}, {{>...}}, {{/...}}
)

// DetectDialect auto-detects dialect: Jinja2 (most specific) → Mustache → Simple (default).
func DetectDialect(content string) promptDomain.TemplateDialect {
	if detectJinja2BlockPattern.MatchString(content) ||
		detectJinja2FilterPattern.MatchString(content) ||
		detectJinja2DotPattern.MatchString(content) {
		return promptDomain.DialectJinja2
	}

	if detectMustacheSectionPattern.MatchString(content) {
		return promptDomain.DialectMustache
	}

	return promptDomain.DialectSimple
}
