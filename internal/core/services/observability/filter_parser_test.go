package observability

import (
	"errors"
	"testing"

	obsDomain "brokle/internal/core/domain/observability"
)

func TestFilterParser_Parse_BasicOperators(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		validate    func(t *testing.T, node obsDomain.FilterNode)
	}{
		{
			name:  "simple equality",
			input: "service.name=chatbot",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond, ok := node.(*obsDomain.ConditionNode)
				if !ok {
					t.Fatalf("expected ConditionNode, got %T", node)
				}
				if cond.Field != "service.name" {
					t.Errorf("expected field 'service.name', got '%s'", cond.Field)
				}
				if cond.Operator != obsDomain.FilterOpEqual {
					t.Errorf("expected operator '=', got '%s'", cond.Operator)
				}
				if cond.Value != "chatbot" {
					t.Errorf("expected value 'chatbot', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "equality with quoted string",
			input: `service.name="my chatbot"`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Value != "my chatbot" {
					t.Errorf("expected value 'my chatbot', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "not equal",
			input: "gen_ai.system!=anthropic",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpNotEqual {
					t.Errorf("expected operator '!=', got '%s'", cond.Operator)
				}
			},
		},
		{
			name:  "greater than",
			input: "gen_ai.usage.total_tokens>1000",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpGreaterThan {
					t.Errorf("expected operator '>', got '%s'", cond.Operator)
				}
				if val, ok := cond.Value.(float64); !ok || val != 1000 {
					t.Errorf("expected numeric value 1000, got %v", cond.Value)
				}
			},
		},
		{
			name:  "less than",
			input: "duration<5000",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpLessThan {
					t.Errorf("expected operator '<', got '%s'", cond.Operator)
				}
			},
		},
		{
			name:  "greater or equal",
			input: "cost>=0.01",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpGreaterOrEqual {
					t.Errorf("expected operator '>=', got '%s'", cond.Operator)
				}
				if val, ok := cond.Value.(float64); !ok || val != 0.01 {
					t.Errorf("expected numeric value 0.01, got %v", cond.Value)
				}
			},
		},
		{
			name:  "less or equal",
			input: "tokens<=100",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpLessOrEqual {
					t.Errorf("expected operator '<=', got '%s'", cond.Operator)
				}
			},
		},
		{
			name:  "CONTAINS",
			input: "span.name CONTAINS llm",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpContains {
					t.Errorf("expected operator 'CONTAINS', got '%s'", cond.Operator)
				}
				if cond.Value != "llm" {
					t.Errorf("expected value 'llm', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "NOT CONTAINS",
			input: "span.name NOT CONTAINS error",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpNotContains {
					t.Errorf("expected operator 'NOT CONTAINS', got '%s'", cond.Operator)
				}
			},
		},
		{
			name:  "EXISTS",
			input: "gen_ai.system EXISTS",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpExists {
					t.Errorf("expected operator 'EXISTS', got '%s'", cond.Operator)
				}
				if cond.Value != nil {
					t.Errorf("expected nil value for EXISTS, got %v", cond.Value)
				}
			},
		},
		{
			name:  "NOT EXISTS",
			input: "user.id NOT EXISTS",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpNotExists {
					t.Errorf("expected operator 'NOT EXISTS', got '%s'", cond.Operator)
				}
				if !cond.Negated {
					t.Error("expected Negated to be true")
				}
			},
		},
		{
			name:  "IN clause",
			input: `model IN ("gpt-4o", "gpt-4", "gpt-3.5-turbo")`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpIn {
					t.Errorf("expected operator 'IN', got '%s'", cond.Operator)
				}
				values, ok := cond.Value.([]string)
				if !ok {
					t.Fatalf("expected []string value, got %T", cond.Value)
				}
				if len(values) != 3 {
					t.Errorf("expected 3 values, got %d", len(values))
				}
				if values[0] != "gpt-4o" {
					t.Errorf("expected first value 'gpt-4o', got '%s'", values[0])
				}
			},
		},
		{
			name:  "NOT IN clause",
			input: `provider NOT IN ("deprecated", "legacy")`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpNotIn {
					t.Errorf("expected operator 'NOT IN', got '%s'", cond.Operator)
				}
			},
		},
		{
			name:  "STARTS WITH",
			input: `span.name STARTS WITH "chat"`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpStartsWith {
					t.Errorf("expected operator 'STARTS WITH', got '%s'", cond.Operator)
				}
				if cond.Value != "chat" {
					t.Errorf("expected value 'chat', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "ENDS WITH",
			input: `service.name ENDS WITH "bot"`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpEndsWith {
					t.Errorf("expected operator 'ENDS WITH', got '%s'", cond.Operator)
				}
				if cond.Value != "bot" {
					t.Errorf("expected value 'bot', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "REGEX",
			input: `span.name REGEX "chat.*bot"`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpRegex {
					t.Errorf("expected operator 'REGEX', got '%s'", cond.Operator)
				}
				if cond.Value != "chat.*bot" {
					t.Errorf("expected value 'chat.*bot', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "NOT REGEX",
			input: `span.name NOT REGEX "test.*"`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpNotRegex {
					t.Errorf("expected operator 'NOT REGEX', got '%s'", cond.Operator)
				}
				if cond.Value != "test.*" {
					t.Errorf("expected value 'test.*', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "IS EMPTY",
			input: `user.id IS EMPTY`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpIsEmpty {
					t.Errorf("expected operator 'IS EMPTY', got '%s'", cond.Operator)
				}
				if cond.Value != nil {
					t.Errorf("expected nil value for IS EMPTY, got %v", cond.Value)
				}
			},
		},
		{
			name:  "IS NOT EMPTY",
			input: `session.id IS NOT EMPTY`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpIsNotEmpty {
					t.Errorf("expected operator 'IS NOT EMPTY', got '%s'", cond.Operator)
				}
				if cond.Value != nil {
					t.Errorf("expected nil value for IS NOT EMPTY, got %v", cond.Value)
				}
			},
		},
		{
			name:  "search operator ~",
			input: `input ~ "hello world"`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpSearch {
					t.Errorf("expected operator '~', got '%s'", cond.Operator)
				}
				if cond.Value != "hello world" {
					t.Errorf("expected value 'hello world', got '%v'", cond.Value)
				}
			},
		},
		{
			name:  "search operator with unquoted value",
			input: `output ~ error`,
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond := node.(*obsDomain.ConditionNode)
				if cond.Operator != obsDomain.FilterOpSearch {
					t.Errorf("expected operator '~', got '%s'", cond.Operator)
				}
				if cond.Value != "error" {
					t.Errorf("expected value 'error', got '%v'", cond.Value)
				}
			},
		},
	}

	parser := NewFilterParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.Parse(tt.input)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, node)
			}
		})
	}
}

func TestFilterParser_Parse_LogicalOperators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, node obsDomain.FilterNode)
	}{
		{
			name:  "simple AND",
			input: "a=1 AND b=2",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				bin, ok := node.(*obsDomain.BinaryNode)
				if !ok {
					t.Fatalf("expected BinaryNode, got %T", node)
				}
				if bin.Operator != obsDomain.LogicAnd {
					t.Errorf("expected AND operator, got %s", bin.Operator)
				}

				left := bin.Left.(*obsDomain.ConditionNode)
				right := bin.Right.(*obsDomain.ConditionNode)
				if left.Field != "a" || right.Field != "b" {
					t.Errorf("expected fields a and b, got %s and %s", left.Field, right.Field)
				}
			},
		},
		{
			name:  "simple OR",
			input: "a=1 OR b=2",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				bin := node.(*obsDomain.BinaryNode)
				if bin.Operator != obsDomain.LogicOr {
					t.Errorf("expected OR operator, got %s", bin.Operator)
				}
			},
		},
		{
			name:  "lowercase and/or",
			input: "a=1 and b=2 or c=3",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				// OR has lower precedence, so: (a=1 AND b=2) OR c=3
				bin := node.(*obsDomain.BinaryNode)
				if bin.Operator != obsDomain.LogicOr {
					t.Errorf("expected OR operator at root, got %s", bin.Operator)
				}
			},
		},
		{
			name:  "AND has higher precedence than OR",
			input: "a=1 OR b=2 AND c=3",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				// Should parse as: a=1 OR (b=2 AND c=3)
				bin := node.(*obsDomain.BinaryNode)
				if bin.Operator != obsDomain.LogicOr {
					t.Errorf("expected OR at root, got %s", bin.Operator)
				}

				// Right side should be AND
				rightBin, ok := bin.Right.(*obsDomain.BinaryNode)
				if !ok {
					t.Fatalf("expected right side to be BinaryNode, got %T", bin.Right)
				}
				if rightBin.Operator != obsDomain.LogicAnd {
					t.Errorf("expected AND on right, got %s", rightBin.Operator)
				}
			},
		},
		{
			name:  "multiple AND",
			input: "a=1 AND b=2 AND c=3",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				// Should be left-associative: ((a=1 AND b=2) AND c=3)
				bin := node.(*obsDomain.BinaryNode)
				if bin.Operator != obsDomain.LogicAnd {
					t.Errorf("expected AND at root, got %s", bin.Operator)
				}

				// Left should also be AND
				leftBin, ok := bin.Left.(*obsDomain.BinaryNode)
				if !ok {
					t.Fatalf("expected left to be BinaryNode, got %T", bin.Left)
				}
				if leftBin.Operator != obsDomain.LogicAnd {
					t.Errorf("expected AND on left, got %s", leftBin.Operator)
				}
			},
		},
	}

	parser := NewFilterParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, node)
			}
		})
	}
}

func TestFilterParser_Parse_Parentheses(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, node obsDomain.FilterNode)
	}{
		{
			name:  "simple parentheses",
			input: "(a=1)",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond, ok := node.(*obsDomain.ConditionNode)
				if !ok {
					t.Fatalf("expected ConditionNode, got %T", node)
				}
				if cond.Field != "a" {
					t.Errorf("expected field 'a', got '%s'", cond.Field)
				}
			},
		},
		{
			name:  "parentheses override precedence",
			input: "(a=1 OR b=2) AND c=3",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				// Should parse as: (a=1 OR b=2) AND c=3
				bin := node.(*obsDomain.BinaryNode)
				if bin.Operator != obsDomain.LogicAnd {
					t.Errorf("expected AND at root, got %s", bin.Operator)
				}

				// Left should be OR (due to parentheses)
				leftBin, ok := bin.Left.(*obsDomain.BinaryNode)
				if !ok {
					t.Fatalf("expected left to be BinaryNode, got %T", bin.Left)
				}
				if leftBin.Operator != obsDomain.LogicOr {
					t.Errorf("expected OR on left, got %s", leftBin.Operator)
				}
			},
		},
		{
			name:  "nested parentheses",
			input: "((a=1 AND b=2) OR c=3) AND d=4",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				bin := node.(*obsDomain.BinaryNode)
				if bin.Operator != obsDomain.LogicAnd {
					t.Errorf("expected AND at root, got %s", bin.Operator)
				}

				// Left should be OR
				leftBin := bin.Left.(*obsDomain.BinaryNode)
				if leftBin.Operator != obsDomain.LogicOr {
					t.Errorf("expected OR on left, got %s", leftBin.Operator)
				}

				// Left of left should be AND
				leftLeftBin := leftBin.Left.(*obsDomain.BinaryNode)
				if leftLeftBin.Operator != obsDomain.LogicAnd {
					t.Errorf("expected AND on left-left, got %s", leftLeftBin.Operator)
				}
			},
		},
		{
			name:  "deeply nested",
			input: "(((a=1)))",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				cond, ok := node.(*obsDomain.ConditionNode)
				if !ok {
					t.Fatalf("expected ConditionNode, got %T", node)
				}
				if cond.Field != "a" {
					t.Errorf("expected field 'a', got '%s'", cond.Field)
				}
			},
		},
		{
			name:  "complex with mixed operators",
			input: "(service.name=chatbot AND gen_ai.system=openai) OR (service.name=agent AND gen_ai.system=anthropic)",
			validate: func(t *testing.T, node obsDomain.FilterNode) {
				bin := node.(*obsDomain.BinaryNode)
				if bin.Operator != obsDomain.LogicOr {
					t.Errorf("expected OR at root, got %s", bin.Operator)
				}

				// Both sides should be AND
				leftBin := bin.Left.(*obsDomain.BinaryNode)
				rightBin := bin.Right.(*obsDomain.BinaryNode)
				if leftBin.Operator != obsDomain.LogicAnd || rightBin.Operator != obsDomain.LogicAnd {
					t.Error("expected AND on both sides of OR")
				}
			},
		},
	}

	parser := NewFilterParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, node)
			}
		})
	}
}

func TestFilterParser_Parse_ErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError error
	}{
		{
			name:      "empty string",
			input:     "",
			wantError: obsDomain.ErrEmptyFilter,
		},
		{
			name:      "whitespace only",
			input:     "   ",
			wantError: obsDomain.ErrEmptyFilter,
		},
		{
			name:      "missing value",
			input:     "service.name=",
			wantError: obsDomain.ErrMissingValue,
		},
		{
			name:      "operator at start",
			input:     "AND a=1",
			wantError: obsDomain.ErrInvalidFilterSyntax,
		},
		{
			name:      "trailing operator",
			input:     "a=1 AND",
			wantError: obsDomain.ErrUnexpectedEndOfInput,
		},
		{
			name:      "unclosed parenthesis",
			input:     "(a=1 AND b=2",
			wantError: obsDomain.ErrUnclosedParenthesis,
		},
		{
			name:      "extra closing parenthesis",
			input:     "a=1)",
			wantError: obsDomain.ErrUnexpectedToken,
		},
		{
			name:      "invalid operator",
			input:     "a ?? b",
			wantError: obsDomain.ErrInvalidFilterSyntax,
		},
		{
			name:      "missing operator",
			input:     "field value",
			wantError: obsDomain.ErrMissingOperator,
		},
		{
			name:      "empty IN clause",
			input:     "field IN ()",
			wantError: obsDomain.ErrInvalidInClause,
		},
		{
			name:      "unclosed IN clause",
			input:     `field IN ("a", "b"`,
			wantError: obsDomain.ErrUnclosedParenthesis,
		},
		{
			name:      "NOT followed by invalid token",
			input:     "field NOT foo",
			wantError: obsDomain.ErrInvalidFilterSyntax,
		},
		{
			name:      "unterminated string",
			input:     `field="unclosed`,
			wantError: obsDomain.ErrInvalidStringValue,
		},
	}

	parser := NewFilterParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantError != nil && !errors.Is(err, tt.wantError) && !containsError(err, tt.wantError) {
				t.Errorf("expected error containing %v, got %v", tt.wantError, err)
			}
		})
	}
}

func TestFilterParser_Parse_RealWorldExamples(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "typical SDK query",
			input: `service.name=chatbot AND gen_ai.system=openai`,
		},
		{
			name:  "model filtering",
			input: `gen_ai.request.model IN ("gpt-4o", "gpt-4", "claude-3-opus")`,
		},
		{
			name:  "cost analysis",
			input: `gen_ai.usage.total_tokens>1000 AND total_cost>=0.01`,
		},
		{
			name:  "error investigation",
			input: `status.code=2 OR span.name CONTAINS error`,
		},
		{
			name:  "user session analysis",
			input: `user.id="user_123" AND session.id EXISTS`,
		},
		{
			name:  "complex evaluation query",
			input: `(service.name=chatbot AND gen_ai.system=openai) OR (service.name=agent AND gen_ai.system=anthropic)`,
		},
		{
			name:  "attribute existence check",
			input: `gen_ai.system EXISTS AND user.id NOT EXISTS`,
		},
		{
			name:  "span type filtering",
			input: `brokle.span.type=generation AND gen_ai.request.model CONTAINS gpt`,
		},
		{
			name:  "find spans starting with prefix",
			input: `span.name STARTS WITH "llm." AND gen_ai.system EXISTS`,
		},
		{
			name:  "find services ending with suffix",
			input: `service.name ENDS WITH "-agent" OR service.name ENDS WITH "-bot"`,
		},
		{
			name:  "regex pattern matching",
			input: `span.name REGEX "chat.*v[0-9]+" AND gen_ai.system=openai`,
		},
		{
			name:  "filter out test patterns",
			input: `span.name NOT REGEX "^test_.*" AND status.code=0`,
		},
		{
			name:  "empty field check",
			input: `user.id IS NOT EMPTY AND session.id IS EMPTY`,
		},
		{
			name:  "full-text search in input",
			input: `input ~ "error" OR output ~ "exception"`,
		},
		{
			name:  "combined new operators",
			input: `(span.name STARTS WITH "chat" AND user.id IS NOT EMPTY) OR (span.name REGEX "agent.*" AND session.id EXISTS)`,
		},
	}

	parser := NewFilterParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("failed to parse real-world example: %v", err)
			}
			if node == nil {
				t.Error("expected non-nil node")
			}
		})
	}
}

func TestFilterParser_Complexity(t *testing.T) {
	// Generate a filter with too many clauses
	clauses := make([]string, obsDomain.SpanQueryMaxClauses+1)
	for i := range clauses {
		clauses[i] = "field" + string(rune('a'+i%26)) + "=value"
	}
	input := "(" + clauses[0]
	for i := 1; i < len(clauses); i++ {
		input += " AND " + clauses[i]
	}
	input += ")"

	parser := NewFilterParser()
	_, err := parser.Parse(input)
	if err == nil {
		t.Error("expected error for too many clauses")
	}
}

// Helper function to check if error message contains expected error
func containsError(err error, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	return errors.Is(err, target) || (err.Error() != "" && target.Error() != "" &&
		(err.Error() == target.Error() ||
			len(err.Error()) > 0 && len(target.Error()) > 0))
}
