package parser

import (
	"strings"

	"github.com/heyllave/query/parser/internal/chars"
	"github.com/heyllave/query/parser/internal/shape"
	"github.com/heyllave/query/token"
)

// lexFieldPosition lexes one token where a field, a connective or a
// punctuation mark is expected, rather than a value.
func (l *lexer) lexFieldPosition() {
	ch := l.input[l.pos]
	if ch != '(' {
		// Only a '(' directly after IN opens a list; anything else means
		// the word was a field name (IN=1, IN x(...)).
		l.expectInList = false
	}
	if l.lexOperator() {
		return
	}
	switch {
	case ch == '(':
		l.lexOpenParen()
	case ch == ')':
		l.lexCloseParen()
	case ch == ',':
		l.lexComma()
	case chars.IsSign(ch) && chars.IsDigit(l.peek(1)) && l.afterListSep():
		// Signed literal in an argument or IN-list slot: addDays(d, -7).
		l.lexNumericLiteral()
	case chars.IsArithOp(ch) && l.continuesArith():
		l.lexContinuedArith(ch)
	case ch == '[':
		// Bracketed field name in field position: ["labels.x/y"]=1.
		l.lexFieldRef()
	case ch == '@':
		l.emit(token.At, "@")
		l.pos++
	default:
		l.lexFieldWord(ch)
	}
}

// lexFieldWord lexes a quoted name, a name or keyword, or a number where a
// field is expected, and reports anything else.
func (l *lexer) lexFieldWord(ch byte) {
	switch {
	case ch == '"':
		l.lexQuotedString()
	case chars.IsIdentStart(ch):
		l.lexIdentOrKeyword()
	case chars.IsDigit(ch):
		l.lexNumericLiteral()
	default:
		l.unexpectedRune("unexpected character %q")
	}
}

// fieldOperators are the operators and marks of field position, longest
// first so "!>=" is not read as "!>" followed by "=". value says whether a
// value follows.
var fieldOperators = []struct {
	text  string
	typ   token.Type
	value bool
}{
	{"!>=", token.Ngte, true},
	{"!<=", token.Nlte, true},
	{"!=", token.Neq, true},
	{"!>", token.Ngt, true},
	{"!<", token.Nlt, true},
	{">=", token.Gte, true},
	{"<=", token.Lte, true},
	{"..", token.Range, true},
	{"=", token.Eq, true},
	{">", token.Gt, true},
	{"<", token.Lt, true},
	{":", token.Colon, true},
	{".", token.Dot, false},
}

// lexOperator lexes a comparison operator, ':', '..' or '.' at l.pos and
// reports whether there was one.
func (l *lexer) lexOperator() bool {
	rest := l.input[l.pos:]
	for _, op := range fieldOperators {
		if strings.HasPrefix(rest, op.text) {
			l.emit(op.typ, op.text)
			l.pos += len(op.text)
			if op.value {
				l.afterOperator = true
			}
			return true
		}
	}
	return false
}

// lexOpenParen lexes '(' : a group, a call's arguments, or an IN list.
func (l *lexer) lexOpenParen() {
	l.parens = append(l.parens, !l.expectInList && l.afterName())
	l.emit(token.LParen, "(")
	l.pos++
	if l.expectInList {
		l.expectInList = false
		l.inList = true
		l.afterOperator = true
	}
}

func (l *lexer) lexCloseParen() {
	l.emit(token.RParen, ")")
	l.pos++
	l.inList = false
	if n := len(l.parens); n > 0 {
		l.parens = l.parens[:n-1]
	}
}

func (l *lexer) lexComma() {
	l.emit(token.Comma, ",")
	l.pos++
	if l.inList {
		l.afterOperator = true
	}
}

// lexContinuedArith lexes an arithmetic operator after whitespace that
// continues the preceding value: `total>10 - 3`. The operand that follows is
// lexed in value mode so the parser sees the same tokens as for `10-3`.
func (l *lexer) lexContinuedArith(ch byte) {
	l.emitArith(ch)
	l.afterOperator = true
	l.argValue = len(l.parens) > 0 && l.parens[len(l.parens)-1]
}

// keywords are the connectives, matched in any case.
var keywords = map[string]token.Type{
	"AND": token.And,
	"OR":  token.Or,
	"NOT": token.Not,
	"IN":  token.In,
}

// lexIdentOrKeyword lexes a word in field position. The original casing is
// kept in Token.Value so String() reproduces the input. true and false are
// booleans in any position, as classifyValue reads them after an operator.
func (l *lexer) lexIdentOrKeyword() {
	start := l.pos
	l.pos = l.identEnd(l.pos)
	word := l.input[start:l.pos]
	typ, keyword := keywords[strings.ToUpper(word)]
	switch {
	case keyword:
		if typ == token.In {
			l.expectInList = true
		}
	case word == "true" || word == "false":
		typ = token.Boolean
	default:
		typ = token.Ident
	}
	l.tokens = append(l.tokens, token.Token{Type: typ, Value: word, Pos: token.Position{Offset: start, Length: l.pos - start}})
}

// afterListSep reports whether the previous token opened an argument or
// IN-list slot ('(' or ','), where a signed numeric literal is permitted.
func (l *lexer) afterListSep() bool {
	if len(l.tokens) == 0 {
		return false
	}
	t := l.tokens[len(l.tokens)-1].Type
	return t == token.LParen || t == token.Comma
}

// afterName reports whether the previous token is a name ending exactly at
// l.pos, which makes a '(' here the start of a call's arguments.
func (l *lexer) afterName() bool {
	if len(l.tokens) == 0 {
		return false
	}
	prev := l.tokens[len(l.tokens)-1]
	return prev.Type == token.Ident && prev.Pos.Offset+prev.Pos.Length == l.pos
}

// continuesArith reports whether an arithmetic operator at l.pos (reached in
// field position, so after whitespace) continues the value that precedes it:
// the previous token ends a value and a primary begins after the operator
// and any whitespace.
func (l *lexer) continuesArith() bool {
	if !l.afterValue() {
		return false
	}
	i := chars.SkipSpace(l.input, l.pos+1)
	return i < len(l.input) && shape.BeginsPrimary(l.input, i)
}

// afterValue reports whether the previous token ends a value primary, so an
// arithmetic operator here is binary rather than a sign.
func (l *lexer) afterValue() bool {
	if len(l.tokens) == 0 {
		return false
	}
	prev := l.tokens[len(l.tokens)-1]
	return prev.Type.IsValueEnd() || (prev.Type == token.String && prev.Quoted)
}

// arithTokens maps an arithmetic operator to its token.
var arithTokens = map[byte]token.Type{
	'+': token.Plus,
	'-': token.Minus,
	'*': token.Mul,
	'/': token.Div,
	'%': token.Mod,
}

func arithTokenFor(ch byte) token.Type {
	if t, ok := arithTokens[ch]; ok {
		return t
	}
	return token.Mod
}
