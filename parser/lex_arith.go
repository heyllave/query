package parser

import (
	"strings"
	"unicode/utf8"

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
		before := l.pos
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
			depth, expectPrimary, ok = l.lexArithOperator(depth, l.pos > before)
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
// closing a group, or a ','. Anything else ends the value. After a space
// at depth 0 that is the next condition (now()-7d AND x). A term glued to
// the operand, or after a space inside a group, is refused: now()x would
// otherwise read as now() AND x, and (1 2) would close the group early.
func (l *lexer) lexArithOperator(depth int, spaced bool) (newDepth int, expectPrimary, ok bool) {
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
	if beginsTerm(l.input[l.pos]) && (!spaced || depth > 0) && !l.atConnective() {
		l.refuseGluedValue()
	}
	return depth, false, false
}

// beginsTerm reports whether c can begin a term of its own: a word, a
// number, a quoted string, a bracketed field or a group.
func beginsTerm(c byte) bool {
	return chars.IsIdentStart(c) || chars.IsDigit(c) || c == '"' || c == '[' || c == '('
}

// atConnective reports whether the word at l.pos is AND or OR, which joins
// two conditions whether or not a space comes before it: total>[base]AND x.
func (l *lexer) atConnective() bool {
	word := strings.ToUpper(l.input[l.pos:chars.IdentEnd(l.input, l.pos)])
	return word == "AND" || word == "OR"
}

// refuseGluedValue reports a term that follows an arithmetic operand with no
// space and no operator between them: [a]x, now()"q", 2026-11-16[b], 7d7d.
// Read on, it would become a second condition joined by AND. After a
// number or a duration the likeliest mistake is a unit (7x, 1h30m).
func (l *lexer) refuseGluedValue() {
	width := gluedWidth(l.input, l.pos)
	e := newError(CodeUnexpected, ErrSyntax, token.Position{Offset: l.pos, Length: width},
		"unexpected %q right after a value", l.input[l.pos:l.pos+width])
	e.Hint = "put an operator between two values (now()-7d), or a space before the next condition (a=now() AND x); text that holds both goes in quotes"
	if c := l.input[l.pos]; l.afterCount() && (chars.IsIdentStart(c) || chars.IsDigit(c)) {
		e.Hint = hintDurationForm + "; write 90m rather than 1h30m"
	}
	l.errors.add(e)
}

// gluedWidth is the span an error names for the term glued at s[i]: the
// whole word or count when one starts there (NOT, not N; 30m, not 3),
// otherwise one character.
func gluedWidth(s string, i int) int {
	if chars.IsIdentStart(s[i]) || chars.IsDigit(s[i]) {
		return chars.IdentEnd(s, i) - i
	}
	_, width := utf8.DecodeRuneInString(s[i:])
	return width
}

// afterCount reports whether the last token is a number or a duration, so a
// word or count glued to it is most likely a unit: 7x, 1h30m. A date is not a count;
// 2026-11-16[b] gets the general hint.
func (l *lexer) afterCount() bool {
	if len(l.tokens) == 0 {
		return false
	}
	t := l.tokens[len(l.tokens)-1].Type
	return t == token.Integer || t == token.Float || t == token.Duration
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
		l.syntaxError(CodeExpectedValue, l.pos, 1, "expected value, got ')'")
		return false
	case chars.IsDigit(ch), chars.IsSign(ch) && chars.IsDigit(l.peek(1)):
		// A number, or a signed one: -50, +3.14.
		l.lexNumericLiteral()
	case chars.IsIdentStart(ch):
		l.lexArithWord()
	default:
		l.syntaxError(CodeUnexpectedChar, l.pos, 1, "unexpected character %q in value expression", string(ch))
		l.pos++
		return false
	}
	return true
}

// lexArithWord lexes a word inside an arithmetic value: a call when '('
// follows it, otherwise a string primary.
func (l *lexer) lexArithWord() {
	start := l.pos
	l.pos = chars.IdentEnd(l.input, l.pos)
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
