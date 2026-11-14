package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// parser builds an AST from a token stream using recursive descent.
type parser struct {
	tokens []token.Token
	pos    int
	errors ErrorList
	// nesting counts the open groups, negations, selectors, value parentheses
	// and function calls around the current position; see MaxNesting.
	nesting int
	tooDeep bool
}

// MaxNesting is the deepest a query may nest groups, negations, selectors,
// parenthesised values and function calls. The parser recurses once per
// level, so without a bound a long enough run of '(' would exhaust the stack,
// which a caller that disabled the length limit cannot recover from. No
// hand-written query comes near it.
const MaxNesting = 128

// enter opens one nesting level, reporting false (and recording the failure
// once) when the query nests deeper than MaxNesting. Each successful enter is
// paired with a leave.
func (p *parser) enter(pos token.Position) bool {
	if p.nesting >= MaxNesting {
		if !p.tooDeep {
			p.tooDeep = true
			p.errors.add(newError(CodeNestingTooDeep, ErrSyntax, pos,
				"query nests deeper than %d levels", MaxNesting))
		}
		return false
	}
	p.nesting++
	return true
}

func (p *parser) leave() { p.nesting-- }

// callFollows reports whether the current token is immediately followed by
// '(' with no space between, the form of a function call. `urgent (a OR b)`
// is a term and a group, not a call to urgent.
func (p *parser) callFollows() bool {
	tok, next := p.peek(), p.peekAt(1)
	return next.Type == token.LParen && next.Pos.Offset == tok.Pos.Offset+tok.Pos.Length
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
		p.errors.add(newError(CodeUnexpected, ErrUnexpectedToken, tok.Pos,
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
		return nil, ErrorList{newError(CodeExpectedValue, ErrUnexpectedEOF, startPos, "expected a value expression")}
	}
	if p.peek().Type != token.EOF {
		tok := p.peek()
		p.errors.add(newError(CodeUnexpected, ErrUnexpectedToken, tok.Pos,
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
	for left != nil {
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
	case token.Ident, token.LParen, token.Not, token.FieldRef, token.String, token.Boolean, token.In:
		return true
	default:
		return false
	}
}

// isFieldToken reports whether the current token can name a field in field
// position: a bare identifier, a bracketed name ([labels.x/y] or ["with
// space"]), a quoted name ("with space"), or a reserved word used as a name.
// true, false and in are accepted as bare terms; and/or (and not, see
// keywordIsField) only when an operator follows, so `AND foo` stays an error.
func (p *parser) isFieldToken() bool {
	tok := p.peek()
	switch tok.Type { //nolint:exhaustive // only field-name tokens
	case token.Ident, token.FieldRef, token.Boolean, token.In:
		return true
	case token.String:
		return tok.Quoted
	case token.And, token.Or:
		next := p.peekAt(1).Type
		return next.IsOperator() || next == token.Dot || next == token.In
	case token.Not:
		return p.keywordIsField()
	default:
		return false
	}
}

// keywordIsField reports whether a NOT keyword at the current position is a
// field name rather than a negation: `not=1`, `not`, `(not)`, `not@first`.
func (p *parser) keywordIsField() bool {
	next := p.peekAt(1).Type
	return next.IsOperator() || next == token.EOF || next == token.RParen ||
		next == token.Dot || next == token.At || next == token.In || next == token.And || next == token.Or
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
		if !p.enter(pos) {
			return nil
		}
		inner := p.parseExpression()
		p.leave()
		if p.tooDeep {
			return nil
		}
		if p.peek().Type != token.RParen {
			p.errors.add(newError(CodeUnclosedParen, ErrSyntax, p.peek().Pos,
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
		if !p.enter(pos) {
			return nil
		}
		inner := p.parseExpression()
		p.leave()
		if p.tooDeep {
			return nil
		}
		if p.peek().Type != token.RParen {
			p.errors.add(newError(CodeUnclosedParen, ErrSyntax, p.peek().Pos,
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
		p.errors.add(newError(CodeExpectedSelector, ErrUnexpectedToken, next.Pos,
			"expected 'first', 'last', 'any(...)', 'all(...)', 'none(...)', or '(' after '@', got %s", next))
		return nil
	}
}

func (p *parser) parseTerm() ast.Expression {
	if p.peek().Type == token.Not && !p.keywordIsField() {
		op := p.advance()
		if !p.enter(op.Pos) {
			return nil
		}
		// Use parseChainExpr so selectors bind tighter than NOT:
		//   NOT items@(x=y) → NOT (items@(x=y))
		expr := p.parseChainExpr()
		p.leave()
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
		if !p.enter(open.Pos) {
			return nil
		}
		expr := p.parseExpression()
		p.leave()
		if p.tooDeep {
			return nil
		}
		if p.peek().Type != token.RParen {
			p.errors.add(newError(CodeUnclosedParen, ErrSyntax, p.peek().Pos, "expected ')', got %s", p.peek()))
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
	if !p.isFieldToken() {
		tok := p.peek()
		if tok.Type == token.EOF {
			p.errors.add(newError(CodeExpectedField, ErrUnexpectedEOF, tok.Pos, "unexpected end of query, expected field name"))
		} else {
			p.errors.add(newError(CodeExpectedField, ErrUnexpectedToken, tok.Pos, "expected field name, got %s", tok))
		}
		return nil
	}

	// Check if this is a function call: identifier followed by '('. A
	// reserved word cannot name a function: `in(` opens an IN list and
	// `not(`/`and(`/`or(` read as connectives, so such a call could never
	// print back unambiguously.
	if tok := p.peek(); tok.Type.IsKeyword() && (p.callFollows() || (tok.Type == token.In && p.peekAt(1).Type == token.LParen)) {
		p.errors.add(newError(CodeExpectedField, ErrUnexpectedToken, tok.Pos,
			"reserved word %q cannot name a function", tok.Value))
		return nil
	}
	if p.peek().Type != token.FieldRef && p.peek().Type != token.String && p.callFollows() {
		return p.parseFuncCallOrQualifier()
	}

	startPos := p.peek().Pos
	if first := p.peek(); first.Type == token.String && first.Value == "" {
		p.errors.add(newError(CodeEmptyFieldRef, ErrSyntax, first.Pos, "empty field name"))
		p.advance()
		return nil
	}
	field := p.parseFieldName()
	tok := p.peek()

	if tok.Type == token.Colon {
		p.advance()
		return p.parseRangeExpr(field, nil, startPos)
	}
	if in, negated := p.acceptIn(); in {
		return p.parseInExpr(field, nil, negated, startPos)
	}
	if tok.Type.IsOperator() {
		p.advance()
		val := p.parseValue()
		if val == nil {
			return nil
		}
		return comparison(field, nil, tok.Type, *val, startPos)
	}
	return &ast.PresenceExpr{
		Field:    field,
		Position: startPos,
	}
}

// comparison builds the qualifier for `field op value` (or `fn(...) op value`
// when fieldFunc is set), desugaring a negated operator (field !> val) into
// NOT (field > val). Missing-field semantics in eval — see
// compileComparisonWithResolver — make a plain operator flip incorrect: a
// missing field makes both `field>val` and `field<=val` return false, so
// `NOT (field>val)` is the only logically consistent rewrite.
func comparison(field ast.FieldPath, fieldFunc *ast.FuncCallExpr, op token.Type, val ast.Value, pos token.Position) ast.Expression {
	q := &ast.QualifierExpr{
		Field:     field,
		FieldFunc: fieldFunc,
		Operator:  token.NegateOperator(op),
		Value:     val,
		Position:  pos,
	}
	if op.IsNegatedOperator() {
		return &ast.UnaryExpr{
			Op:       token.Not,
			Expr:     q,
			Position: pos,
		}
	}
	return q
}

// acceptIn consumes `IN` or `NOT IN` after a field or function when a list
// follows, reporting whether it did and whether the form was negated. NOT
// counts only directly before `IN (`, so `a NOT b` is still a AND NOT b.
func (p *parser) acceptIn() (in, negated bool) {
	switch p.peek().Type { //nolint:exhaustive // only IN and NOT IN
	case token.In:
		p.advance()
		return true, false
	case token.Not:
		if p.peekAt(1).Type == token.In && p.peekAt(2).Type == token.LParen {
			p.advance()
			p.advance()
			return true, true
		}
	}
	return false, false
}

// parseInExpr parses the right-hand side of `field IN (...)` and lowers it to
// an OR chain of equality qualifiers; `field NOT IN (...)` lowers to an AND
// chain of inequalities, so like SQL it is false for a record without the
// field. The left side may be a function (lower(name) IN (a, b)). Both forms
// are pure surface syntax — once parsed they are indistinguishable from
// `field=v1 OR field=v2` and `field!=v1 AND field!=v2`, and round-tripping
// normalizes to the expanded form.
func (p *parser) parseInExpr(field ast.FieldPath, fieldFunc *ast.FuncCallExpr, negated bool, startPos token.Position) ast.Expression {
	if p.peek().Type != token.LParen {
		p.errors.add(newError(CodeExpectedInList, ErrSyntax, p.peek().Pos,
			"expected '(' after IN, got %s", p.peek()))
		return nil
	}
	p.advance() // consume '('

	var values []ast.Value
	for p.peek().Type != token.RParen && p.peek().Type != token.EOF {
		if len(values) > 0 {
			if p.peek().Type != token.Comma {
				p.errors.add(newError(CodeUnclosedIn, ErrSyntax, p.peek().Pos,
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
		p.errors.add(newError(CodeUnclosedIn, ErrSyntax, p.peek().Pos, "expected ')' to close IN list"))
		return nil
	}
	p.advance() // consume ')'

	if len(values) == 0 {
		p.errors.add(newError(CodeEmptyInList, ErrSyntax, startPos, "IN list cannot be empty"))
		return nil
	}

	op, join := token.Eq, token.Or
	if negated {
		op, join = token.Neq, token.And
	}
	var expr ast.Expression
	for i, v := range values {
		q := &ast.QualifierExpr{
			Field:     field,
			FieldFunc: fieldFunc,
			Operator:  op,
			Value:     v,
			Position:  startPos,
		}
		if i == 0 {
			expr = q
			continue
		}
		expr = &ast.BinaryExpr{
			Op:       join,
			Left:     expr,
			Right:    q,
			Position: startPos,
		}
	}
	return expr
}

func (p *parser) parseRangeExpr(field ast.FieldPath, fieldFunc *ast.FuncCallExpr, startPos token.Position) ast.Expression {
	startVal := p.parseValue()
	if startVal == nil {
		return nil
	}
	if p.peek().Type != token.Range {
		p.errors.add(newError(CodeExpectedRange, ErrSyntax, p.peek().Pos,
			"expected '..' in range expression, got %s", p.peek()))
		return nil
	}
	p.advance()
	endVal := p.parseValue()
	if endVal == nil {
		return nil
	}
	return &ast.QualifierExpr{
		Field:     field,
		FieldFunc: fieldFunc,
		Operator:  token.Range,
		Value:     *startVal,
		EndValue:  endVal,
		Position:  startPos,
	}
}

func (p *parser) parseFieldName() ast.FieldPath {
	first := p.advance()
	switch {
	case first.Type == token.FieldRef && !first.Quoted:
		// [labels.dev] — a dotted path over the identifier alphabet.
		return ast.FieldPath(strings.Split(first.Value, "."))
	case first.Type == token.FieldRef || first.Type == token.String:
		// ["weird.name"] or "weird name" — one segment, dots included.
		return ast.FieldPath{first.Value}
	}
	var parts []string
	parts = append(parts, first.Value)
	for p.peek().Type == token.Dot {
		p.advance()
		// A segment after a dot may be a reserved word (labels.in).
		if p.peek().Type != token.Ident && !p.peek().Type.IsKeyword() {
			p.errors.add(newError(CodeExpectedField, ErrSyntax, p.peek().Pos,
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
		if !p.enter(tok.Pos) {
			return nil
		}
		inner := p.parseValueAdditive()
		p.leave()
		if inner == nil {
			return nil
		}
		if p.peek().Type != token.RParen {
			p.errors.add(newError(CodeUnclosedParen, ErrSyntax, p.peek().Pos,
				"expected ')' to close value expression, got %s", p.peek()))
			return nil
		}
		p.advance()
		// Preserve grouping in Raw for faithful round-trip — the AST itself
		// doesn't have a value-group node, but the Raw text retains parens.
		// A grouped literal is just the literal, so only arithmetic keeps them.
		if inner.Type == ast.ValueArith {
			inner.Raw = "(" + inner.Raw + ")"
		}
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
	case token.FieldRef:
		p.advance()
		var fp ast.FieldPath
		if tok.Quoted {
			// The quoted form takes the whole inner text as one segment so a
			// name like "weird.name" is not split into a nested path.
			fp = ast.FieldPath{tok.Value}
		} else {
			fp = ast.FieldPath(strings.Split(tok.Value, "."))
		}
		// Raw carries the bracket form so arithmetic round-trips ([base]*1.1
		// must not collapse to base*1.1); shared with ast.String via
		// ast.FieldRefString.
		return &ast.Value{
			Type:   ast.ValueFieldRef,
			Raw:    ast.FieldRefString(fp, tok.Quoted),
			Field:  fp,
			Quoted: tok.Quoted,
		}
	case token.String:
		p.advance()
		return &ast.Value{Type: ast.ValueString, Raw: rawOf(tok), Str: tok.Value, Quoted: tok.Quoted}
	case token.Integer:
		p.advance()
		n, err := strconv.ParseInt(tok.Value, 10, 64)
		if err != nil {
			p.errors.add(newError(CodeInvalidInteger, ErrInvalidValue, tok.Pos, "invalid integer %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueInteger, Raw: tok.Value, Int: n}
	case token.Float:
		p.advance()
		f, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			p.errors.add(newError(CodeInvalidFloat, ErrInvalidValue, tok.Pos, "invalid float %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueFloat, Raw: tok.Value, Float: f}
	case token.Boolean:
		p.advance()
		return &ast.Value{Type: ast.ValueBoolean, Raw: tok.Value, Bool: tok.Value == "true"}
	case token.Date:
		p.advance()
		d, err := ParseDateLiteral(tok.Value)
		if err != nil {
			p.errors.add(newError(CodeInvalidDate, ErrInvalidDate, tok.Pos, "invalid date %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueDate, Raw: tok.Value, Date: d, DayOnly: IsDayLiteral(tok.Value)}
	case token.Duration:
		p.advance()
		dur, err := ParseDuration(tok.Value)
		if err != nil {
			p.errors.add(newError(CodeInvalidDuration, ErrInvalidDuration, tok.Pos, "invalid duration %q", tok.Value))
			return nil
		}
		return &ast.Value{Type: ast.ValueDuration, Raw: tok.Value, Duration: dur}
	case token.Wildcard:
		p.advance()
		return &ast.Value{Type: ast.ValueString, Raw: rawOf(tok), Str: tok.Value, Wildcard: true}
	case token.EOF:
		p.errors.add(newError(CodeExpectedValue, ErrUnexpectedEOF, tok.Pos, "expected value, got end of query"))
		return nil
	default:
		p.errors.add(newError(CodeExpectedValue, ErrUnexpectedToken, tok.Pos, "expected value, got %s", tok))
		p.advance()
		return nil
	}
}

// rawOf returns the source text of an unquoted value token: the escaped form
// when the lexer decoded escapes, else the value itself. Keeping the escaped
// form in Value.Raw lets ast.String re-emit `a\(b` instead of `a(b`.
func rawOf(tok token.Token) string {
	if tok.Raw != "" {
		return tok.Raw
	}
	return tok.Value
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
	// lower(name)=john* → qualifier where the "field" is the function result.
	// Field is left empty: the function is the field, and consumers that read
	// Field would otherwise mistake the function name for a field name.
	tok := p.peek()
	if tok.Type == token.Colon {
		p.advance()
		return p.parseRangeExpr(nil, fc, fc.Position)
	}
	if in, negated := p.acceptIn(); in {
		return p.parseInExpr(nil, fc, negated, fc.Position)
	}
	if tok.Type.IsOperator() {
		p.advance()
		val := p.parseValue()
		if val == nil {
			return nil
		}
		return comparison(nil, fc, tok.Type, *val, fc.Position)
	}

	// Standalone function call (boolean predicate)
	return fc
}

// parseFuncCall parses: identifier "(" [arg {"," arg}] ")"
func (p *parser) parseFuncCall() *ast.FuncCallExpr {
	nameTok := p.advance() // consume identifier
	startPos := nameTok.Pos
	p.advance() // consume '('
	if !p.enter(startPos) {
		return nil
	}
	defer p.leave()

	var args []ast.FuncArg
	for p.peek().Type != token.RParen && p.peek().Type != token.EOF {
		if len(args) > 0 {
			if p.peek().Type != token.Comma {
				p.errors.add(newError(CodeUnclosedFuncArgs, ErrSyntax, p.peek().Pos,
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
		p.errors.add(newError(CodeUnclosedFuncArgs, ErrSyntax, p.peek().Pos, "expected ')' after function arguments"))
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

	// Arithmetic that opens with a bracketed field or a call, abs([a]-[b]) or
	// min(len(name)+1, 9): the whole argument is a value expression.
	if p.argIsArithmetic() {
		if val := p.parseValue(); val != nil {
			return &ast.FuncArg{Value: val}
		}
		return nil
	}

	// Nested function call: func(...)
	if tok.Type == token.Ident && p.peekAt(1).Type == token.LParen {
		call := p.parseFuncCall()
		if call == nil {
			return nil
		}
		return &ast.FuncArg{Call: call}
	}

	// Field reference: identifier, identifier.identifier, [bracketed], or a
	// reserved word used as a name. A quoted string stays a literal here.
	if tok.Type == token.Ident || tok.Type == token.FieldRef || tok.Type == token.In || tok.Type == token.And ||
		tok.Type == token.Or || tok.Type == token.Not {
		field := p.parseFieldName()
		return &ast.FuncArg{Field: &field}
	}

	// Literal value (use the value lexing for after-operator tokens)
	val := p.parseValue()
	if val != nil {
		return plainArg(val)
	}

	// parseValue has already reported the missing value on every path that
	// reaches here, so this is the defensive tail of the same failure and
	// carries the same code rather than inventing a second name for it.
	p.errors.add(newError(CodeExpectedValue, ErrUnexpectedToken, tok.Pos,
		"expected function argument, got %s", tok))
	return nil
}

// plainArg turns a parsed value into an argument. A paren group around a lone
// field or call, f(([a])) or f((g(x))), is that field or call, so it takes
// the same shape as f([a]) and f(g(x)) and prints the same way.
func plainArg(val *ast.Value) *ast.FuncArg {
	switch {
	case val.Type == ast.ValueFieldRef:
		field := val.Field
		return &ast.FuncArg{Field: &field}
	case val.Type == ast.ValueFunc && val.Func != nil:
		return &ast.FuncArg{Call: val.Func}
	}
	return &ast.FuncArg{Value: val}
}

// argIsArithmetic reports whether the argument at the cursor is a bracketed
// field or a call that an arithmetic operator follows.
func (p *parser) argIsArithmetic() bool {
	end := 1
	switch tok := p.peek(); {
	case tok.Type == token.FieldRef:
	case tok.Type == token.Ident && p.peekAt(1).Type == token.LParen:
		depth := 0
		for ; ; end++ {
			switch p.peekAt(end).Type { //nolint:exhaustive // only brackets matter
			case token.LParen:
				depth++
			case token.RParen:
				depth--
			case token.EOF:
				return false
			}
			if depth == 0 {
				break
			}
		}
		end++
	default:
		return false
	}
	return p.peekAt(end).Type.IsArithmetic()
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
