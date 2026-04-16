package observability

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	obsDomain "brokle/internal/core/domain/observability"
)

// Grammar:
//   expression  = or_expr
//   or_expr     = and_expr ("OR" and_expr)*
//   and_expr    = primary ("AND" primary)*
//   primary     = "(" expression ")" | condition
//   condition   = field operator value | field existence_op | field empty_op
//   field       = identifier ("." identifier)*
//   operator    = "=" | "!=" | ">" | "<" | ">=" | "<=" | "CONTAINS" | "NOT CONTAINS"
//                | "IN" | "NOT IN" | "STARTS WITH" | "ENDS WITH" | "REGEX" | "NOT REGEX" | "~"
//   existence_op = "EXISTS" | "NOT" "EXISTS"
//   empty_op   = "IS" "EMPTY" | "IS" "NOT" "EMPTY"
//   value       = string | number | "(" string_list ")"

// FilterParser parses filter expressions into an AST.
type FilterParser struct {
	tokens      []Token
	pos         int
	clauseCount int // Track complexity during parsing for early exit
}

// NewFilterParser creates a new filter parser.
func NewFilterParser() *FilterParser {
	return &FilterParser{}
}

// Parse parses a filter string into a FilterNode AST.
func (p *FilterParser) Parse(input string) (obsDomain.FilterNode, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, obsDomain.ErrEmptyFilter
	}

	if len(input) > obsDomain.SpanQueryMaxFilterLen {
		return nil, obsDomain.ErrFilterTooLong
	}

	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}

	p.tokens = tokens
	p.pos = 0
	p.clauseCount = 0

	node, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	if !p.isAtEnd() {
		return nil, fmt.Errorf("%w: unexpected token at position %d", obsDomain.ErrUnexpectedToken, p.currentToken().Pos)
	}

	return node, nil
}

// parseExpression parses the top-level expression (OR has lowest precedence).
func (p *FilterParser) parseExpression() (obsDomain.FilterNode, error) {
	return p.parseOrExpr()
}

// parseOrExpr handles OR expressions.
func (p *FilterParser) parseOrExpr() (obsDomain.FilterNode, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	for p.match(TokenOr) {
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &obsDomain.BinaryNode{
			Left:     left,
			Right:    right,
			Operator: obsDomain.LogicOr,
		}
	}

	return left, nil
}

// parseAndExpr handles AND expressions (higher precedence than OR).
func (p *FilterParser) parseAndExpr() (obsDomain.FilterNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for p.match(TokenAnd) {
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &obsDomain.BinaryNode{
			Left:     left,
			Right:    right,
			Operator: obsDomain.LogicAnd,
		}
	}

	return left, nil
}

// parsePrimary handles parenthesized expressions or conditions.
func (p *FilterParser) parsePrimary() (obsDomain.FilterNode, error) {
	if p.match(TokenLParen) {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if !p.match(TokenRParen) {
			return nil, obsDomain.ErrUnclosedParenthesis
		}
		return expr, nil
	}

	return p.parseCondition()
}

// parseCondition handles individual filter conditions.
func (p *FilterParser) parseCondition() (obsDomain.FilterNode, error) {
	if !p.check(TokenField) {
		if p.isAtEnd() {
			return nil, obsDomain.ErrUnexpectedEndOfInput
		}
		return nil, fmt.Errorf("%w: expected field name at position %d", obsDomain.ErrInvalidFilterSyntax, p.currentToken().Pos)
	}

	field := p.advance().Value

	negated := false
	if p.check(TokenNot) {
		p.advance()
		negated = true
	}

	if p.check(TokenExists) {
		p.advance()
		op := obsDomain.FilterOpExists
		if negated {
			op = obsDomain.FilterOpNotExists
		}
		p.clauseCount++
		if err := p.checkComplexity(); err != nil {
			return nil, err
		}
		return &obsDomain.ConditionNode{
			Field:    field,
			Operator: op,
			Negated:  negated,
		}, nil
	}

	if p.check(TokenIs) {
		return p.parseIsCondition(field, negated)
	}

	if p.check(TokenStarts) {
		p.advance()
		if !p.match(TokenWith) {
			return nil, fmt.Errorf("%w: expected WITH after STARTS", obsDomain.ErrInvalidFilterSyntax)
		}
		value, err := p.parseValue(obsDomain.FilterOpStartsWith)
		if err != nil {
			return nil, err
		}
		p.clauseCount++
		if err := p.checkComplexity(); err != nil {
			return nil, err
		}
		return &obsDomain.ConditionNode{
			Field:    field,
			Operator: obsDomain.FilterOpStartsWith,
			Value:    value,
		}, nil
	}

	if p.check(TokenEnds) {
		p.advance()
		if !p.match(TokenWith) {
			return nil, fmt.Errorf("%w: expected WITH after ENDS", obsDomain.ErrInvalidFilterSyntax)
		}
		value, err := p.parseValue(obsDomain.FilterOpEndsWith)
		if err != nil {
			return nil, err
		}
		p.clauseCount++
		if err := p.checkComplexity(); err != nil {
			return nil, err
		}
		return &obsDomain.ConditionNode{
			Field:    field,
			Operator: obsDomain.FilterOpEndsWith,
			Value:    value,
		}, nil
	}

	if p.check(TokenRegex) {
		p.advance()
		op := obsDomain.FilterOpRegex
		if negated {
			op = obsDomain.FilterOpNotRegex
		}
		value, err := p.parseValue(op)
		if err != nil {
			return nil, err
		}
		p.clauseCount++
		if err := p.checkComplexity(); err != nil {
			return nil, err
		}
		return &obsDomain.ConditionNode{
			Field:    field,
			Operator: op,
			Value:    value,
			Negated:  negated,
		}, nil
	}

	if !p.check(TokenOperator) && !p.check(TokenContains) && !p.check(TokenIn) {
		if negated {
			return nil, fmt.Errorf("%w: NOT must be followed by EXISTS, CONTAINS, IN, or REGEX", obsDomain.ErrInvalidFilterSyntax)
		}
		return nil, fmt.Errorf("%w: expected operator at position %d", obsDomain.ErrMissingOperator, p.currentToken().Pos)
	}

	var op obsDomain.FilterOperator
	opToken := p.advance()

	switch opToken.Type {
	case TokenOperator:
		if opToken.Value == "~" {
			op = obsDomain.FilterOpSearch
		} else {
			op = obsDomain.FilterOperator(opToken.Value)
		}
	case TokenContains:
		if negated {
			op = obsDomain.FilterOpNotContains
		} else {
			op = obsDomain.FilterOpContains
		}
	case TokenIn:
		if negated {
			op = obsDomain.FilterOpNotIn
		} else {
			op = obsDomain.FilterOpIn
		}
	}

	value, err := p.parseValue(op)
	if err != nil {
		return nil, err
	}

	p.clauseCount++
	if err := p.checkComplexity(); err != nil {
		return nil, err
	}

	return &obsDomain.ConditionNode{
		Field:    field,
		Operator: op,
		Value:    value,
		Negated:  negated,
	}, nil
}

// parseIsCondition handles IS EMPTY / IS NOT EMPTY conditions.
func (p *FilterParser) parseIsCondition(field string, alreadyNegated bool) (obsDomain.FilterNode, error) {
	p.advance()

	negated := alreadyNegated
	if p.check(TokenNot) {
		p.advance()
		negated = !negated
	}

	if !p.check(TokenEmpty) {
		return nil, fmt.Errorf("%w: expected EMPTY after IS/IS NOT", obsDomain.ErrInvalidFilterSyntax)
	}
	p.advance()

	op := obsDomain.FilterOpIsEmpty
	if negated {
		op = obsDomain.FilterOpIsNotEmpty
	}

	p.clauseCount++
	if err := p.checkComplexity(); err != nil {
		return nil, err
	}

	return &obsDomain.ConditionNode{
		Field:    field,
		Operator: op,
		Negated:  negated,
	}, nil
}

// parseValue parses the value based on operator type.
func (p *FilterParser) parseValue(op obsDomain.FilterOperator) (interface{}, error) {
	if op == obsDomain.FilterOpIn || op == obsDomain.FilterOpNotIn {
		return p.parseInList()
	}

	if p.isAtEnd() {
		return nil, obsDomain.ErrMissingValue
	}

	token := p.currentToken()

	switch token.Type {
	case TokenString:
		p.advance()
		return token.Value, nil
	case TokenNumber:
		p.advance()
		// Try to parse as float64
		val, err := strconv.ParseFloat(token.Value, 64)
		if err != nil {
			return nil, obsDomain.ErrInvalidNumericValue
		}
		return val, nil
	default:
		// Unquoted value - treat as string
		if token.Type == TokenField {
			p.advance()
			return token.Value, nil
		}
		return nil, fmt.Errorf("%w at position %d", obsDomain.ErrInvalidValue, token.Pos)
	}
}

// parseInList parses (value1, value2, ...) for IN clause.
func (p *FilterParser) parseInList() ([]string, error) {
	if !p.match(TokenLParen) {
		return nil, obsDomain.ErrInvalidInClause
	}

	var values []string

	for {
		if p.isAtEnd() {
			return nil, obsDomain.ErrUnclosedParenthesis
		}

		token := p.currentToken()

		switch token.Type {
		case TokenString:
			p.advance()
			values = append(values, token.Value)
		case TokenField, TokenNumber:
			p.advance()
			values = append(values, token.Value)
		default:
			return nil, fmt.Errorf("%w: expected value in IN list", obsDomain.ErrInvalidInClause)
		}

		// Check for comma or closing paren
		if p.match(TokenComma) {
			continue
		}
		if p.match(TokenRParen) {
			break
		}

		return nil, fmt.Errorf("%w: expected ',' or ')' in IN list", obsDomain.ErrInvalidInClause)
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("%w: empty IN list", obsDomain.ErrInvalidInClause)
	}

	return values, nil
}

func (p *FilterParser) currentToken() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF, Pos: -1}
	}
	return p.tokens[p.pos]
}

func (p *FilterParser) isAtEnd() bool {
	return p.pos >= len(p.tokens) || p.tokens[p.pos].Type == TokenEOF
}

func (p *FilterParser) check(tokenType TokenType) bool {
	if p.isAtEnd() {
		return false
	}
	return p.tokens[p.pos].Type == tokenType
}

func (p *FilterParser) advance() Token {
	if !p.isAtEnd() {
		p.pos++
	}
	return p.tokens[p.pos-1]
}

func (p *FilterParser) match(tokenType TokenType) bool {
	if p.check(tokenType) {
		p.advance()
		return true
	}
	return false
}

// checkComplexity returns an error if the clause limit has been exceeded.
// Called during parsing to enable early exit.
func (p *FilterParser) checkComplexity() error {
	if p.clauseCount > obsDomain.SpanQueryMaxClauses {
		return obsDomain.NewFilterTooComplexError(p.clauseCount)
	}
	return nil
}

// TokenType represents the type of a token.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenField
	TokenOperator
	TokenString
	TokenNumber
	TokenAnd
	TokenOr
	TokenNot
	TokenLParen
	TokenRParen
	TokenComma
	TokenExists
	TokenContains
	TokenIn
	// New token types for extended operators
	TokenStarts // "STARTS" keyword
	TokenEnds   // "ENDS" keyword
	TokenWith   // "WITH" keyword
	TokenRegex  // "REGEX" keyword
	TokenIs     // "IS" keyword
	TokenEmpty  // "EMPTY" keyword
	TokenSearch // "~" operator
)

// Token represents a lexical token.
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// Lexer tokenizes a filter expression string.
type Lexer struct {
	input  string
	pos    int
	tokens []Token
}

// NewLexer creates a new lexer.
func NewLexer(input string) *Lexer {
	return &Lexer{input: input}
}

// Tokenize converts the input string into tokens.
func (l *Lexer) Tokenize() ([]Token, error) {
	l.tokens = nil
	l.pos = 0

	for l.pos < len(l.input) {
		if unicode.IsSpace(rune(l.input[l.pos])) {
			l.pos++
			continue
		}

		startPos := l.pos

		switch l.input[l.pos] {
		case '(':
			l.tokens = append(l.tokens, Token{Type: TokenLParen, Value: "(", Pos: startPos})
			l.pos++
			continue
		case ')':
			l.tokens = append(l.tokens, Token{Type: TokenRParen, Value: ")", Pos: startPos})
			l.pos++
			continue
		case ',':
			l.tokens = append(l.tokens, Token{Type: TokenComma, Value: ",", Pos: startPos})
			l.pos++
			continue
		}

		if op := l.scanOperator(); op != "" {
			l.tokens = append(l.tokens, Token{Type: TokenOperator, Value: op, Pos: startPos})
			continue
		}

		if l.input[l.pos] == '"' || l.input[l.pos] == '\'' {
			str, err := l.scanString()
			if err != nil {
				return nil, err
			}
			l.tokens = append(l.tokens, Token{Type: TokenString, Value: str, Pos: startPos})
			continue
		}

		if unicode.IsDigit(rune(l.input[l.pos])) || (l.input[l.pos] == '-' && l.pos+1 < len(l.input) && unicode.IsDigit(rune(l.input[l.pos+1]))) {
			num := l.scanNumber()
			l.tokens = append(l.tokens, Token{Type: TokenNumber, Value: num, Pos: startPos})
			continue
		}

		if unicode.IsLetter(rune(l.input[l.pos])) || l.input[l.pos] == '_' {
			word := l.scanIdentifier()
			token := l.classifyWord(word, startPos)
			l.tokens = append(l.tokens, token)
			continue
		}

		return nil, fmt.Errorf("%w: unexpected character '%c' at position %d", obsDomain.ErrInvalidFilterSyntax, l.input[l.pos], l.pos)
	}

	l.tokens = append(l.tokens, Token{Type: TokenEOF, Pos: l.pos})
	return l.tokens, nil
}

func (l *Lexer) scanOperator() string {
	if l.pos+1 < len(l.input) {
		twoChar := l.input[l.pos : l.pos+2]
		switch twoChar {
		case "!=", ">=", "<=":
			l.pos += 2
			return twoChar
		}
	}

	switch l.input[l.pos] {
	case '=', '>', '<':
		op := string(l.input[l.pos])
		l.pos++
		return op
	case '~':
		l.pos++
		return "~"
	}

	return ""
}

func (l *Lexer) scanString() (string, error) {
	quote := l.input[l.pos]
	l.pos++ // skip opening quote

	var sb strings.Builder
	for l.pos < len(l.input) {
		ch := l.input[l.pos]

		if ch == quote {
			l.pos++ // skip closing quote
			return sb.String(), nil
		}

		// Handle escape sequences
		if ch == '\\' && l.pos+1 < len(l.input) {
			l.pos++
			nextCh := l.input[l.pos]
			switch nextCh {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case '"', '\'', '\\':
				sb.WriteByte(nextCh)
			default:
				sb.WriteByte('\\')
				sb.WriteByte(nextCh)
			}
			l.pos++
			continue
		}

		sb.WriteByte(ch)
		l.pos++
	}

	return "", fmt.Errorf("%w: unterminated string starting at position %d", obsDomain.ErrInvalidStringValue, l.pos)
}

func (l *Lexer) scanNumber() string {
	start := l.pos

	if l.input[l.pos] == '-' {
		l.pos++
	}

	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.pos++
	}

	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		l.pos++
		for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
			l.pos++
		}
	}

	if l.pos < len(l.input) && (l.input[l.pos] == 'e' || l.input[l.pos] == 'E') {
		l.pos++
		if l.pos < len(l.input) && (l.input[l.pos] == '+' || l.input[l.pos] == '-') {
			l.pos++
		}
		for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
			l.pos++
		}
	}

	return l.input[start:l.pos]
}

func (l *Lexer) scanIdentifier() string {
	start := l.pos
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_' || ch == '.' {
			l.pos++
		} else {
			break
		}
	}
	return l.input[start:l.pos]
}

func (l *Lexer) classifyWord(word string, pos int) Token {
	upper := strings.ToUpper(word)
	switch upper {
	case "AND":
		return Token{Type: TokenAnd, Value: word, Pos: pos}
	case "OR":
		return Token{Type: TokenOr, Value: word, Pos: pos}
	case "NOT":
		return Token{Type: TokenNot, Value: word, Pos: pos}
	case "EXISTS":
		return Token{Type: TokenExists, Value: word, Pos: pos}
	case "CONTAINS":
		return Token{Type: TokenContains, Value: word, Pos: pos}
	case "IN":
		return Token{Type: TokenIn, Value: word, Pos: pos}
	// New keywords for extended operators
	case "STARTS":
		return Token{Type: TokenStarts, Value: word, Pos: pos}
	case "ENDS":
		return Token{Type: TokenEnds, Value: word, Pos: pos}
	case "WITH":
		return Token{Type: TokenWith, Value: word, Pos: pos}
	case "REGEX":
		return Token{Type: TokenRegex, Value: word, Pos: pos}
	case "IS":
		return Token{Type: TokenIs, Value: word, Pos: pos}
	case "EMPTY":
		return Token{Type: TokenEmpty, Value: word, Pos: pos}
	default:
		return Token{Type: TokenField, Value: word, Pos: pos}
	}
}
