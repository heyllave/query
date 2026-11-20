package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/heyllave/query/parser/internal/chars"
	"github.com/heyllave/query/parser/internal/literal"
	"github.com/heyllave/query/parser/internal/shape"
	"github.com/heyllave/query/token"
)

// lexValue lexes the value after an operator: a quoted string, arithmetic,
// a bracketed field, or a bare value. Arithmetic is tried before a lone
// field reference, so [base]*1.1 is one arithmetic value; shape decides it
// and keeps year=202* a pattern.
func (l *lexer) lexValue() {
	l.afterOperator = false
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return
	}
	ch := l.input[l.pos]
	switch {
	case ch == '"':
		l.lexQuotedString()
	case shape.IsArithmetic(l.input, l.pos):
		l.lexArithValueExpr()
	case ch == '[':
		l.lexFieldRef()
	default:
		l.refuseSecondOperator(ch)
		l.lexUnquotedStringValue()
	}
}

// refuseSecondOperator reports an operator where the value belongs:
// state==draft, total=>5, a<>b. Read as text these would compare with
// "=draft" or ">5" without a word of complaint; a value that really starts
// with one of these characters is written in quotes.
func (l *lexer) refuseSecondOperator(ch byte) {
	if ch == '=' || ch == '<' || ch == '>' {
		l.errors.add(newError(CodeUnexpectedChar, ErrSyntax, token.Position{Offset: l.pos, Length: 1},
			"unexpected %q where a value belongs", string(ch)))
	}
}

// bareValue collects an unquoted value as it is read.
type bareValue struct {
	start        int
	buf          strings.Builder
	hasWildcard  bool
	quoteSeen    bool
	operatorAt   int   // offset of the first comparison character, under strictValues
	literalStars []int // byte offsets in buf of escaped stars
}

// lexUnquotedStringValue lexes a bare value: text or a wildcard pattern. It
// stops at whitespace, ')', '[', ']', '..', and at ',' inside an IN list or
// a call. \* \\ \( \) stand for the character. A value with an unescaped
// star is a pattern, and its token value is in pattern form (see
// literal.EncodePattern), so a literal star stays distinct from a wildcard.
func (l *lexer) lexUnquotedStringValue() {
	v := &bareValue{start: l.pos, operatorAt: -1}
	for l.pos < len(l.input) && !l.endsBareValue() {
		l.takeBareChar(v)
	}
	raw := l.input[v.start:l.pos]
	value := v.buf.String()
	if v.operatorAt >= 0 {
		l.refuseInnerOperator(v.operatorAt, value)
	}
	if v.hasWildcard {
		value = literal.EncodePattern(value, v.literalStars)
	}
	if value == "" {
		return
	}
	tok := l.classifyValue(raw, value, v.hasWildcard, token.Position{Offset: v.start, Length: l.pos - v.start})
	if raw != value {
		tok.Raw = raw
	}
	l.tokens = append(l.tokens, tok)
}

// endsBareValue reports whether the character at l.pos ends a bare value.
func (l *lexer) endsBareValue() bool {
	ch := l.input[l.pos]
	switch {
	case chars.SpaceWidth(l.input, l.pos) > 0 || ch == ')' || ch == '[' || ch == ']':
		return true
	case ch == ',' && (l.inList || l.argValue):
		return true
	}
	return ch == '.' && l.peek(1) == '.'
}

// takeBareChar adds the character at l.pos, or the escape there, to v.
func (l *lexer) takeBareChar(v *bareValue) {
	ch := l.input[l.pos]
	if next := l.peek(1); ch == '\\' && l.pos+1 < len(l.input) && isBareEscape(next) {
		if next == '*' {
			v.literalStars = append(v.literalStars, v.buf.Len())
		}
		v.buf.WriteByte(next)
		l.pos += 2
		return
	}
	switch {
	case ch == '*':
		v.hasWildcard = true
	case ch == '"' && !v.quoteSeen:
		v.quoteSeen = true
		l.refuseInnerQuote()
	case l.strictValues && v.operatorAt < 0 && l.pos > v.start && (ch == '=' || ch == '<' || ch == '>'):
		// a=1+b=2 would otherwise compare a with the text "1+b=2": a second
		// condition swallowed by the first. A value that opens with one of
		// these is refused by lexValue, so only a later one is noted here.
		v.operatorAt = l.pos
	}
	v.buf.WriteByte(ch)
	l.pos++
}

// isBareEscape reports whether a backslash before c stands for c itself in
// a bare value.
func isBareEscape(c byte) bool {
	return c == '*' || c == '\\' || c == '(' || c == ')'
}

// refuseInnerQuote reports a quote inside a bare value. It could not be
// printed back: the value may not be quoted if it is a pattern, and left
// bare the quote starts a string for whatever reads the query next.
func (l *lexer) refuseInnerQuote() {
	l.errors.add(newError(CodeUnexpectedChar, ErrSyntax, token.Position{Offset: l.pos, Length: 1},
		`unexpected '"' inside an unquoted value; quote the whole value and write the quote as \"`))
}

// refuseInnerOperator reports a comparison character inside a bare value,
// under strictValues. The hint is set here, where the whole value is known,
// because the two readings need different fixes and only the writer knows
// which was meant.
func (l *lexer) refuseInnerOperator(at int, value string) {
	ch := string(l.input[at])
	e := newError(CodeUnexpectedChar, ErrSyntax, token.Position{Offset: at, Length: 1},
		"unexpected %q inside an unquoted value", ch)
	e.Hint = fmt.Sprintf("two conditions are joined with AND: a=1 AND b=2; if the %q is part of the value, put the value in quotes: %s",
		ch, strconv.Quote(value))
	l.errors.add(e)
}
