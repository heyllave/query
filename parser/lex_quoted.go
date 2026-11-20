package parser

import (
	"github.com/heyllave/query/parser/internal/chars"
	"github.com/heyllave/query/token"
)

// readQuotedString decodes the double-quoted string whose opening quote is
// at l.pos and moves past it. It returns the text and whether the string was
// closed. Shared by lexQuotedString and lexFieldRef.
func (l *lexer) readQuotedString() (string, bool) {
	value, end, closed := chars.ReadQuoted(l.input, l.pos)
	l.pos = end
	return value, closed
}

// lexQuotedString lexes a double-quoted string, opening quote at l.pos, as a
// String token. A quoted value is never reclassified as a number, a date, a
// boolean or a pattern: the quotes are the writer's signal that it is text.
func (l *lexer) lexQuotedString() {
	start := l.pos
	value, closed := l.readQuotedString()
	pos := token.Position{Offset: start, Length: l.pos - start}
	if !closed {
		l.errors.add(newError(CodeUnclosedString, ErrSyntax, pos, "unterminated string literal"))
		return
	}
	l.tokens = append(l.tokens, token.Token{Type: token.String, Value: value, Pos: pos, Quoted: true})
}

// lexFieldRef lexes a bracketed field reference with the opening '[' at
// l.pos: [name], [labels.dev], or ["name with-special.chars"]. The bare form
// is a dotted path over the identifier alphabet; the quoted form takes the
// whole inner text as one segment. Emits a FieldRef token.
func (l *lexer) lexFieldRef() {
	start := l.pos
	l.pos++ // skip '['
	l.skipWhitespace()
	name, quoted, closed := l.readFieldRefName()
	if closed {
		l.skipWhitespace()
		closed = l.pos < len(l.input) && l.input[l.pos] == ']'
	}
	if !closed {
		l.fieldRefError(start, CodeUnclosedFieldRef, "unterminated field reference")
		return
	}
	l.pos++ // skip ']'
	if name == "" {
		l.fieldRefError(start, CodeEmptyFieldRef, "empty field reference")
		return
	}
	pos := token.Position{Offset: start, Length: l.pos - start}
	l.tokens = append(l.tokens, token.Token{Type: token.FieldRef, Value: name, Pos: pos, Quoted: quoted})
}

// readFieldRefName reads the name inside a field reference: a quoted string,
// or dot-separated identifier segments. closed is false only for a quoted
// name that never closes.
func (l *lexer) readFieldRefName() (name string, quoted, closed bool) {
	if l.pos < len(l.input) && l.input[l.pos] == '"' {
		name, closed = l.readQuotedString()
		return name, true, closed
	}
	start := l.pos
	l.pos = chars.PathEnd(l.input, l.pos)
	return l.input[start:l.pos], false, true
}

// fieldRefError reports a field reference from start to l.pos.
func (l *lexer) fieldRefError(start int, code Code, message string) {
	l.errors.add(newError(code, ErrSyntax, token.Position{Offset: start, Length: l.pos - start}, "%s", message))
}
