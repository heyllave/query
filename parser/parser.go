package parser

import (
	"fmt"
	"strconv"
	"strings"
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

// ParseValue lexes and parses a bare value expression as the root, producing an
// *[ast.ValueExpr]. Unlike [Parse], which requires a boolean predicate, this
// accepts a value-producing expression — a literal, arithmetic, or function
// call — so the query computes and returns a value rather than true/false.
//
//	now()-7d
//	(50000+1000)*1.1
//	upper(name)
//
// Field references are only reachable through function arguments, matching the
// arithmetic-operand rules of value position.
func ParseValue(input string, maxLength int) (ast.Expression, error) {
	tokens, err := LexValue(input, maxLength)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	startPos := p.peek().Pos
	val := p.parseValue()
	if err := p.errors.errOrNil(); err != nil {
		return nil, err
	}
	if val == nil {
		return nil, ErrorList{newError(ErrUnexpectedEOF, startPos, "expected a value expression")}
	}
	if p.peek().Type != token.EOF {
		tok := p.peek()
		p.errors.add(newError(ErrUnexpectedToken, tok.Pos,
			"unexpected token %s, expected end of value expression", tok))
	}
	if err := p.errors.errOrNil(); err != nil {
		return nil, err
	}
	return &ast.ValueExpr{Value: *val, Position: startPos}, nil
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
	for {
		t := p.peek().Type
		// Explicit AND or an implicit one (juxtaposition). Implicit AND fires
		// when the next token can legitimately start a new term — Ident,
		// LParen, NOT, or function-call ident. Keywords like AND/OR don't
		// trigger it (they're handled by the explicit branches), and EOF/`)`
		// end the chain.
		implicit := t != token.And && p.canStartTerm(t)
		if t != token.And && !implicit {
			return left
		}
		var pos token.Position
		if t == token.And {
			pos = p.advance().Pos
		} else {
			pos = p.peek().Pos
		}
		right := p.parseChainExpr()
		if right == nil {
			break
		}
		left = &ast.BinaryExpr{
			Op:       token.And,
			Left:     left,
			Right:    right,
			Position: pos,
		}
	}
	return left
}

// canStartTerm reports whether a token type can start a fresh term (qualifier,
// presence, group, or NOT). Used by parseLogicalAnd to detect implicit-AND
// between adjacent terms separated only by whitespace.
func (p *parser) canStartTerm(t token.Type) bool {
	switch t { //nolint:exhaustive // only term-starters matter
	case token.Ident, token.LParen, token.Not:
		return true
	default:
		return false
	}
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
//
//	@first | @last                — list non-emptiness
//	@(expression)                  — EXISTS (any element matches)
//	@any(expression)               — alias for @(...)
//	@all(expression)               — universal (every element matches)
//	@none(expression)              — no element matches
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
	case next.Type == token.Ident && (next.Value == "any" || next.Value == "all" || next.Value == "none") && p.peekAt(1).Type == token.LParen:
		name := p.advance().Value
		p.advance() // consume '('
		inner := p.parseExpression()
		if p.peek().Type != token.RParen {
			p.errors.add(newError(ErrSyntax, p.peek().Pos,
				"expected ')' to close @%s selector, got %s", name, p.peek()))
			return nil
		}
		p.advance()
		return &ast.SelectorExpr{
			Base:     base,
			Selector: name,
			Inner:    inner,
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
			"expected 'first', 'last', 'any(...)', 'all(...)', 'none(...)', or '(' after '@', got %s", next))
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
	if tok.Type == token.In {
		p.advance()
		return p.parseInExpr(field, startPos)
	}
	if tok.Type.IsOperator() {
		p.advance()
		val := p.parseValue()
		if val == nil {
			return nil
		}
		// Desugar negated comparisons (field !> val) into NOT (field > val).
		// Missing-field semantics in eval — see compileComparisonWithResolver —
		// make a plain operator flip incorrect: a missing field makes both
		// `field>val` and `field<=val` return false, so `NOT (field>val)` is
		// the only logically consistent rewrite.
		if tok.Type.IsNegatedOperator() {
			inner := &ast.QualifierExpr{
				Field:    field,
				Operator: token.NegateOperator(tok.Type),
				Value:    *val,
				Position: startPos,
			}
			return &ast.UnaryExpr{
				Op:       token.Not,
				Expr:     inner,
				Position: startPos,
			}
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

// parseInExpr parses the right-hand side of `field IN (...)` and lowers it to
// an OR chain of equality qualifiers. The IN form is pure surface syntax —
// once parsed it is indistinguishable from `field=v1 OR field=v2`, and
// round-tripping normalizes to the expanded form.
func (p *parser) parseInExpr(field ast.FieldPath, startPos token.Position) ast.Expression {
	if p.peek().Type != token.LParen {
		p.errors.add(newError(ErrSyntax, p.peek().Pos,
			"expected '(' after IN, got %s", p.peek()))
		return nil
	}
	p.advance() // consume '('

	var values []ast.Value
	for p.peek().Type != token.RParen && p.peek().Type != token.EOF {
		if len(values) > 0 {
			if p.peek().Type != token.Comma {
				p.errors.add(newError(ErrSyntax, p.peek().Pos,
					"expected ',' or ')' in IN list, got %s", p.peek()))
				return nil
			}
			p.advance()
		}
		val := p.parseValue()
		if val == nil {
			return nil
		}
		values = append(values, *val)
	}
	if p.peek().Type != token.RParen {
		p.errors.add(newError(ErrSyntax, p.peek().Pos, "expected ')' to close IN list"))
		return nil
	}
	p.advance() // consume ')'

	if len(values) == 0 {
		p.errors.add(newError(ErrSyntax, startPos, "IN list cannot be empty"))
		return nil
	}

	var expr ast.Expression
	for i, v := range values {
		q := &ast.QualifierExpr{
			Field:    field,
			Operator: token.Eq,
			Value:    v,
			Position: startPos,
		}
		if i == 0 {
			expr = q
			continue
		}
		expr = &ast.BinaryExpr{
			Op:       token.Or,
			Left:     expr,
			Right:    q,
			Position: startPos,
		}
	}
	return expr
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

// parseValue parses a value expression. Top-level entry runs additive
// precedence so arithmetic like 50000*1.1 + 1000 is recognized.
func (p *parser) parseValue() *ast.Value {
	return p.parseValueAdditive()
}

// parseValueAdditive: parseValueMultiplicative (('+' | '-') parseValueMultiplicative)*
func (p *parser) parseValueAdditive() *ast.Value {
	left := p.parseValueMultiplicative()
	if left == nil {
		return nil
	}
	for {
		t := p.peek().Type
		if t != token.Plus && t != token.Minus {
			return left
		}
		op := p.advance()
		right := p.parseValueMultiplicative()
		if right == nil {
			return nil
		}
		left = mkArithValue(op.Type, left, right, left.Raw+token.OperatorSymbol(op.Type)+right.Raw)
	}
}

// parseValueMultiplicative: parseValuePrimary (('*' | '/' | '%') parseValuePrimary)*
func (p *parser) parseValueMultiplicative() *ast.Value {
	left := p.parseValuePrimary()
	if left == nil {
		return nil
	}
	for {
		t := p.peek().Type
		if t != token.Mul && t != token.Div && t != token.Mod {
			return left
		}
		op := p.advance()
		right := p.parseValuePrimary()
		if right == nil {
			return nil
		}
		left = mkArithValue(op.Type, left, right, left.Raw+token.OperatorSymbol(op.Type)+right.Raw)
	}
}

// parseValuePrimary parses a single arithmetic-value primary: a literal, a
// quoted string, a function call, a wildcard pattern, or a parenthesized
// sub-expression.
func (p *parser) parseValuePrimary() *ast.Value {
	tok := p.peek()
	switch tok.Type {
	case token.LParen:
		p.advance()
		inner := p.parseValueAdditive()
		if inner == nil {
			return nil
		}
		if p.peek().Type != token.RParen {
			p.errors.add(newError(ErrSyntax, p.peek().Pos,
				"expected ')' to close value expression, got %s", p.peek()))
			return nil
		}
		p.advance()
		// Preserve grouping in Raw for faithful round-trip — the AST itself
		// doesn't have a value-group node, but the Raw text retains parens.
		inner.Raw = "(" + inner.Raw + ")"
		return inner
	case token.Ident:
		// An Ident in value position is either a function call (now(),
		// daysAgo(7)) or a barword string value (e.g. inside IN(draft, issued)).
		if p.peekAt(1).Type == token.LParen {
			fc := p.parseFuncCall()
			if fc == nil {
				return nil
			}
			return &ast.Value{Type: ast.ValueFunc, Raw: funcCallString(fc), Func: fc}
		}
		p.advance()
		return &ast.Value{Type: ast.ValueString, Raw: tok.Value, Str: tok.Value}
	case token.String:
		p.advance()
		return &ast.Value{Type: ast.ValueString, Raw: tok.Value, Str: tok.Value, Quoted: tok.Quoted}
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

// mkArithValue wraps two operands and an arithmetic operator into a Value of
// type ValueArith. Raw is preserved for round-tripping.
func mkArithValue(op token.Type, left, right *ast.Value, raw string) *ast.Value {
	return &ast.Value{
		Type: ast.ValueArith,
		Raw:  raw,
		Arith: &ast.ArithExpr{
			Op:    arithOpFromToken(op),
			Left:  left,
			Right: right,
		},
	}
}

// arithOpFromToken maps the lexer's arithmetic token types onto the AST's
// typed [ast.ArithOp] constants. Any non-arithmetic token type is a parser
// invariant violation and panics.
func arithOpFromToken(op token.Type) ast.ArithOp {
	switch op {
	case token.Plus:
		return ast.ArithAdd
	case token.Minus:
		return ast.ArithSub
	case token.Mul:
		return ast.ArithMul
	case token.Div:
		return ast.ArithDiv
	case token.Mod:
		return ast.ArithMod
	default:
		panic(fmt.Sprintf("arithOpFromToken: not an arithmetic token: %v", op))
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

// funcCallString renders a function call back to source form. Used for Value.Raw
// when a function appears in value position (e.g. created_at>=now()) so the
// QualifierExpr.String() can faithfully round-trip the expression.
func funcCallString(fc *ast.FuncCallExpr) string {
	var b strings.Builder
	writeFuncCall(&b, fc)
	return b.String()
}

func writeFuncCall(b *strings.Builder, fc *ast.FuncCallExpr) {
	b.WriteString(fc.Name)
	b.WriteByte('(')
	for i, arg := range fc.Args {
		if i > 0 {
			b.WriteString(", ")
		}
		switch {
		case arg.Field != nil:
			b.WriteString(arg.Field.String())
		case arg.Value != nil:
			b.WriteString(arg.Value.Raw)
		case arg.Call != nil:
			writeFuncCall(b, arg.Call)
		}
	}
	b.WriteByte(')')
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
