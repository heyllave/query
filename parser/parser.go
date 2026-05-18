package parser

import (
	"strconv"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// parser builds an AST from a token stream using recursive descent.
type parser struct {
	tokens []token.Token
	pos    int
	errors ErrorList
}

// Parse lexes and parses a query string into an AST.
func Parse(input string, maxLength int) (ast.Expression, error) {
	tokens, err := Lex(input, maxLength)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	expr := p.parseExpression()
	if err := p.errors.errOrNil(); err != nil {
		return nil, err
	}
	if p.peek().Type != token.EOF {
		tok := p.peek()
		p.errors.add(newError(ErrUnexpectedToken, tok.Pos,
			"unexpected token %s, expected end of query", tok))
	}
	if err := p.errors.errOrNil(); err != nil {
		return nil, err
	}
	return expr, nil
}

func (p *parser) parseExpression() ast.Expression {
	return p.parseLogicalOr()
}

func (p *parser) parseLogicalOr() ast.Expression {
	left := p.parseLogicalAnd()
	for p.peek().Type == token.Or {
		op := p.advance()
		right := p.parseLogicalAnd()
		if right == nil {
			break
		}
		left = &ast.BinaryExpr{
			Op:       token.Or,
			Left:     left,
			Right:    right,
			Position: op.Pos,
		}
	}
	return left
}

func (p *parser) parseLogicalAnd() ast.Expression {
	left := p.parseChainExpr()
	for p.peek().Type == token.And {
		op := p.advance()
		right := p.parseChainExpr()
		if right == nil {
			break
		}
		left = &ast.BinaryExpr{
			Op:       token.And,
			Left:     left,
			Right:    right,
			Position: op.Pos,
		}
	}
	return left
}

// parseChainExpr parses a term optionally followed by one or more selector
// expressions: `term ( '@' ( 'first' | 'last' | '(' expression ')' ) )*`.
func (p *parser) parseChainExpr() ast.Expression {
	expr := p.parseTerm()
	for expr != nil && p.peek().Type == token.At {
		at := p.advance()
		sel := p.parseSelector(expr, at.Pos)
		if sel == nil {
			return nil
		}
		expr = sel
	}
	return expr
}

// parseSelector parses the portion after an '@' token:
// `first`, `last`, or `(expression)`.
func (p *parser) parseSelector(base ast.Expression, pos token.Position) ast.Expression {
	next := p.peek()
	switch {
	case next.Type == token.Ident && (next.Value == "first" || next.Value == "last"):
		p.advance()
		return &ast.SelectorExpr{
			Base:     base,
			Selector: next.Value,
			Position: pos,
		}
	case next.Type == token.LParen:
		p.advance()
		inner := p.parseExpression()
		if p.peek().Type != token.RParen {
			p.errors.add(newError(ErrSyntax, p.peek().Pos,
				"expected ')' to close selector expression, got %s", p.peek()))
			return nil
		}
		p.advance()
		return &ast.SelectorExpr{
			Base:     base,
			Inner:    inner,
			Position: pos,
		}
	default:
		p.errors.add(newError(ErrUnexpectedToken, next.Pos,
			"expected 'first', 'last', or '(' after '@', got %s", next))
		return nil
	}
}

func (p *parser) parseTerm() ast.Expression {
	if p.peek().Type == token.Not {
		op := p.advance()
		// Use parseChainExpr so selectors bind tighter than NOT:
		//   NOT items@(x=y) → NOT (items@(x=y))
		expr := p.parseChainExpr()
		if expr == nil {
			return nil
		}
		return &ast.UnaryExpr{
			Op:       token.Not,
			Expr:     expr,
			Position: op.Pos,
		}
	}
	if p.peek().Type == token.LParen {
		open := p.advance()
		expr := p.parseExpression()
		if p.peek().Type != token.RParen {
			p.errors.add(newError(ErrSyntax, p.peek().Pos, "expected ')', got %s", p.peek()))
			return expr
		}
		p.advance()
		return &ast.GroupExpr{
			Expr:     expr,
			Position: open.Pos,
		}
	}
	return p.parseQualifier()
}

func (p *parser) parseQualifier() ast.Expression {
	if p.peek().Type != token.Ident {
		tok := p.peek()
		if tok.Type == token.EOF {
			p.errors.add(newError(ErrUnexpectedEOF, tok.Pos, "unexpected end of query, expected field name"))
		} else {
			p.errors.add(newError(ErrUnexpectedToken, tok.Pos, "expected field name, got %s", tok))
		}
		return nil
	}

	// Check if this is a function call: identifier followed by '('
	if p.peekAt(1).Type == token.LParen {
		return p.parseFuncCallOrQualifier()
	}

	startPos := p.peek().Pos
	field := p.parseFieldName()
	tok := p.peek()

	if tok.Type == token.Colon {
		p.advance()
		return p.parseRangeExpr(field, startPos)
	}
	if tok.Type.IsOperator() {
		p.advance()
		val := p.parseValue()
		if val == nil {
			return nil
		}
		return &ast.QualifierExpr{
			Field:    field,
			Operator: tok.Type,
			Value:    *val,
			Position: startPos,
		}
	}
	return &ast.PresenceExpr{
		Field:    field,
		Position: startPos,
	}
}

func (p *parser) parseRangeExpr(field ast.FieldPath, startPos token.Position) ast.Expression {
	startVal := p.parseValue()
	if startVal == nil {
		return nil
	}
	if p.peek().Type != token.Range {
		p.errors.add(newError(ErrSyntax, p.peek().Pos,
			"expected '..' in range expression, got %s", p.peek()))
		return nil
	}
	p.advance()
	endVal := p.parseValue()
	if endVal == nil {
		return nil
	}
	return &ast.QualifierExpr{
		Field:    field,
		Operator: token.Range,
		Value:    *startVal,
		EndValue: endVal,
		Position: startPos,
	}
}

func (p *parser) parseFieldName() ast.FieldPath {
	var parts []string
	parts = append(parts, p.advance().Value)
	for p.peek().Type == token.Dot {
		p.advance()
		if p.peek().Type != token.Ident {
			p.errors.add(newError(ErrSyntax, p.peek().Pos,
				"expected field name after '.', got %s", p.peek()))
			break
		}
		parts = append(parts, p.advance().Value)
	}
	return ast.FieldPath(parts)
}

func (p *parser) parseValue() *ast.Value {
	tok := p.peek()
	switch tok.Type {
	case token.String:
		p.advance()
		return &ast.Value{Type: ast.ValueString, Raw: tok.Value, Str: tok.Value}
	case token.Integer:
		p.advance()
		n, err := strconv.ParseInt(tok.Value, 10, 64)
		if err != nil {
			p.errors.add(newError(ErrInvalidValue, tok.Pos, "invalid integer %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueInteger, Raw: tok.Value, Int: n}
	case token.Float:
		p.advance()
		f, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			p.errors.add(newError(ErrInvalidValue, tok.Pos, "invalid float %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueFloat, Raw: tok.Value, Float: f}
	case token.Boolean:
		p.advance()
		return &ast.Value{Type: ast.ValueBoolean, Raw: tok.Value, Bool: tok.Value == "true"}
	case token.Date:
		p.advance()
		d, err := time.Parse("2006-01-02", tok.Value)
		if err != nil {
			p.errors.add(newError(ErrInvalidDate, tok.Pos, "invalid date %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueDate, Raw: tok.Value, Date: d}
	case token.Duration:
		p.advance()
		dur, err := ParseDuration(tok.Value)
		if err != nil {
			p.errors.add(newError(ErrInvalidDuration, tok.Pos, "invalid duration %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueDuration, Raw: tok.Value, Duration: dur}
	case token.Wildcard:
		p.advance()
		return &ast.Value{Type: ast.ValueString, Raw: tok.Value, Str: tok.Value, Wildcard: true}
	case token.EOF:
		p.errors.add(newError(ErrUnexpectedEOF, tok.Pos, "expected value, got end of query"))
		return nil
	default:
		p.errors.add(newError(ErrUnexpectedToken, tok.Pos, "expected value, got %s", tok))
		p.advance()
		return nil
	}
}

// parseFuncCallOrQualifier handles `func(args)` which can be:
//   - A standalone boolean function: contains(tags, "urgent")
//   - A field transform with comparison: lower(name)=john*
func (p *parser) parseFuncCallOrQualifier() ast.Expression {
	fc := p.parseFuncCall()
	if fc == nil {
		return nil
	}

	// If followed by an operator, this is a field-transform qualifier:
	// lower(name)=john* → qualifier where the "field" is the function result
	tok := p.peek()
	if tok.Type.IsOperator() {
		p.advance()
		val := p.parseValue()
		if val == nil {
			return nil
		}
		return &ast.QualifierExpr{
			Field:     ast.FieldPath{fc.Name}, // use func name as field for round-trip
			Operator:  tok.Type,
			Value:     *val,
			FieldFunc: fc,
			Position:  fc.Position,
		}
	}

	// Standalone function call (boolean predicate)
	return fc
}

// parseFuncCall parses: identifier "(" [arg {"," arg}] ")"
func (p *parser) parseFuncCall() *ast.FuncCallExpr {
	nameTok := p.advance() // consume identifier
	startPos := nameTok.Pos
	p.advance() // consume '('

	var args []ast.FuncArg
	for p.peek().Type != token.RParen && p.peek().Type != token.EOF {
		if len(args) > 0 {
			if p.peek().Type != token.Comma {
				p.errors.add(newError(ErrSyntax, p.peek().Pos,
					"expected ',' or ')' in function call, got %s", p.peek()))
				return nil
			}
			p.advance() // consume ','
		}
		arg := p.parseFuncArg()
		if arg == nil {
			return nil
		}
		args = append(args, *arg)
	}

	if p.peek().Type != token.RParen {
		p.errors.add(newError(ErrSyntax, p.peek().Pos, "expected ')' after function arguments"))
		return nil
	}
	p.advance() // consume ')'

	return &ast.FuncCallExpr{
		Name:     nameTok.Value,
		Args:     args,
		Position: startPos,
	}
}

// parseFuncArg parses a single function argument: field, literal, or nested call.
func (p *parser) parseFuncArg() *ast.FuncArg {
	tok := p.peek()

	// Nested function call: func(...)
	if tok.Type == token.Ident && p.peekAt(1).Type == token.LParen {
		call := p.parseFuncCall()
		if call == nil {
			return nil
		}
		return &ast.FuncArg{Call: call}
	}

	// Field reference: identifier or identifier.identifier
	if tok.Type == token.Ident {
		field := p.parseFieldName()
		return &ast.FuncArg{Field: &field}
	}

	// Literal value (use the value lexing for after-operator tokens)
	val := p.parseValue()
	if val != nil {
		return &ast.FuncArg{Value: val}
	}

	p.errors.add(newError(ErrUnexpectedToken, tok.Pos,
		"expected function argument, got %s", tok))
	return nil
}

func (p *parser) peek() token.Token {
	if p.pos >= len(p.tokens) {
		return token.Token{Type: token.EOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) peekAt(offset int) token.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		return token.Token{Type: token.EOF}
	}
	return p.tokens[idx]
}

func (p *parser) advance() token.Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}
