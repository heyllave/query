package parser

import (
	"github.com/heyllave/query/parser/internal/chars"
	"github.com/heyllave/query/token"
)

// lexArithValueExpr lexes an arithmetic value: primaries (numbers, dates,
// durations, quoted strings, field references, calls) joined by + - * / %,
// with parentheses for grouping. At depth 0 it stops at '..', ',', ')' or
// anything that is not an operator.
//
//	50000*1.1
//	(50000+1000)*1.1
//	now()-7d
//	addDays(start, 30)+1d
func (l *lexer) lexArithValueExpr() {
	depth := 0
	expectPrimary := true
	for l.pos < len(l.input) {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}
		l.start = l.pos
		if depth == 0 && endsArithValue(l.input[l.pos], l.peek(1)) {
			return
		}
		var ok bool
		if expectPrimary {
			depth, expectPrimary, ok = l.lexArithOperand(depth)
		} else {
			depth, expectPrimary, ok = l.lexArithOperator(depth)
		}
		if !ok {
			return
		}
	}
}

// endsArithValue reports whether ch, followed by next, ends an arithmetic
// value at depth 0.
func endsArithValue(ch, next byte) bool {
	return (ch == '.' && next == '.') || ch == ',' || ch == ')'
}

// lexArithOperand lexes what may stand where an operand is expected: '('
// opening a group, which still expects an operand, or a primary. It returns
// the new depth, whether an operand is still expected, and false when the
// value ends.
func (l *lexer) lexArithOperand(depth int) (newDepth int, expectPrimary, ok bool) {
	if l.input[l.pos] == '(' {
		l.emit(token.LParen, "(")
		l.pos++
		return depth + 1, true, true
	}
	if !l.lexArithPrimary() {
		return depth, true, false
	}
	return depth, false, true
}

// lexArithOperator lexes what may follow an operand: an operator, a ')'
// closing a group, or a ','. Anything else, such as a letter, ends the
// value.
func (l *lexer) lexArithOperator(depth int) (newDepth int, expectPrimary, ok bool) {
	switch ch := l.input[l.pos]; {
	case chars.IsArithOp(ch):
		l.emitArith(ch)
		return depth, true, true
	case ch == ')':
		l.emit(token.RParen, ")")
		l.pos++
		return depth - 1, false, true
	case ch == ',':
		l.emit(token.Comma, ",")
		l.pos++
		return depth, true, true
	}
	return depth, false, false
}

// lexArithPrimary lexes one primary of an arithmetic value, other than a
// group. It returns false when none could be lexed.
func (l *lexer) lexArithPrimary() bool {
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return false
	}
	switch ch := l.input[l.pos]; {
	case ch == '"':
		l.lexQuotedString()
	case ch == '[':
		l.lexFieldRef()
	case ch == ')':
		l.errors.add(newError(CodeExpectedValue, ErrSyntax, token.Position{Offset: l.pos, Length: 1},
			"expected value, got ')'"))
		return false
	case chars.IsDigit(ch), chars.IsSign(ch) && chars.IsDigit(l.peek(1)):
		// A number, or a signed one: -50, +3.14.
		l.lexNumericLiteral()
	case chars.IsIdentStart(ch):
		l.lexArithWord()
	default:
		l.errors.add(newError(CodeUnexpectedChar, ErrSyntax, token.Position{Offset: l.pos, Length: 1},
			"unexpected character %q in value expression", string(ch)))
		l.pos++
		return false
	}
	return true
}

// lexArithWord lexes a word inside an arithmetic value: a call when '('
// follows it, otherwise a string primary.
func (l *lexer) lexArithWord() {
	start := l.pos
	l.pos = l.identEnd(l.pos)
	tok := token.Token{Type: token.String, Value: l.input[start:l.pos], Pos: token.Position{Offset: start, Length: l.pos - start}}
	if l.pos < len(l.input) && l.input[l.pos] == '(' {
		tok.Type = token.Ident
		l.tokens = append(l.tokens, tok)
		l.lexFuncCallBody()
		return
	}
	l.tokens = append(l.tokens, tok)
}

// lexFuncCallBody lexes a call's arguments from the '(' at l.pos through the
// matching ')', so a call such as now() inside now()-7d is one primary and
// the arithmetic around it can look for operators after it.
func (l *lexer) lexFuncCallBody() {
	if l.pos >= len(l.input) || l.input[l.pos] != '(' {
		return
	}
	l.start = l.pos
	l.emit(token.LParen, "(")
	l.pos++
	for depth := 1; l.pos < len(l.input) && depth > 0; {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}
		l.start = l.pos
		depth += l.lexCallArgument()
	}
}

// lexCallArgument lexes one token inside a call's arguments and returns how
// it changes the parenthesis depth.
func (l *lexer) lexCallArgument() int {
	ch := l.input[l.pos]
	switch {
	case ch == '(':
		l.emit(token.LParen, "(")
		l.pos++
		return 1
	case ch == ')':
		l.emit(token.RParen, ")")
		l.pos++
		return -1
	case ch == ',':
		l.emit(token.Comma, ",")
		l.pos++
	case ch == '"':
		l.lexQuotedString()
	case ch == '[':
		// A bracketed field name as an argument: abs([cpu-limit]).
		l.lexFieldRef()
	case ch == '.' && l.peek(1) != '.':
		// A dotted field path as an argument: lower(labels.team).
		l.emit(token.Dot, ".")
		l.pos++
	default:
		l.lexCallOperand(ch)
	}
	return 0
}

// lexCallOperand lexes an operator, a number or a name inside a call's
// arguments, and reports anything else.
func (l *lexer) lexCallOperand(ch byte) {
	switch {
	case chars.IsArithOp(ch) && l.afterValue():
		// An operator after a complete operand: addDays(d, 1-8).
		l.emitArith(ch)
	case chars.IsDigit(ch), chars.IsSign(ch) && chars.IsDigit(l.peek(1)):
		l.lexNumericLiteral()
	case chars.IsIdentStart(ch):
		l.lexIdentOrKeyword()
	default:
		l.unexpectedRune("unexpected character %q in function arguments")
	}
}
