package parser

import (
	"strings"
	"time"
	"unicode"

	"github.com/heyllave/query/token"
)

// lexer tokenizes a query string into a sequence of tokens.
type lexer struct {
	input         string
	pos           int
	start         int
	tokens        []token.Token
	errors        ErrorList
	afterOperator bool
}

// Lex tokenizes the input query string and returns the token stream.
func Lex(input string, maxLength int) ([]token.Token, error) {
	if maxLength > 0 && len(input) > maxLength {
		return nil, ErrorList{newError(ErrQueryTooLong, token.Position{},
			"query length %d exceeds maximum of %d characters", len(input), maxLength)}
	}

	l := &lexer{input: input}
	l.run()

	if err := l.errors.errOrNil(); err != nil {
		return nil, err
	}
	return l.tokens, nil
}

func (l *lexer) run() {
	for l.pos < len(l.input) {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}

		l.start = l.pos

		if l.afterOperator {
			l.lexValue()
			continue
		}

		ch := l.input[l.pos]
		switch {
		case ch == '(':
			l.emit(token.LParen, "(")
			l.pos++
		case ch == ')':
			l.emit(token.RParen, ")")
			l.pos++
		case ch == ',':
			l.emit(token.Comma, ",")
			l.pos++
		case ch == '@':
			l.emit(token.At, "@")
			l.pos++
		case ch == '!' && l.peek(1) == '=':
			l.emit(token.Neq, "!=")
			l.pos += 2
			l.afterOperator = true
		case ch == '>' && l.peek(1) == '=':
			l.emit(token.Gte, ">=")
			l.pos += 2
			l.afterOperator = true
		case ch == '<' && l.peek(1) == '=':
			l.emit(token.Lte, "<=")
			l.pos += 2
			l.afterOperator = true
		case ch == '=':
			l.emit(token.Eq, "=")
			l.pos++
			l.afterOperator = true
		case ch == '>':
			l.emit(token.Gt, ">")
			l.pos++
			l.afterOperator = true
		case ch == '<':
			l.emit(token.Lt, "<")
			l.pos++
			l.afterOperator = true
		case ch == ':':
			l.emit(token.Colon, ":")
			l.pos++
			l.afterOperator = true
		case ch == '.' && l.peek(1) == '.':
			l.emit(token.Range, "..")
			l.pos += 2
			l.afterOperator = true
		case ch == '.':
			l.emit(token.Dot, ".")
			l.pos++
		case isIdentStart(ch):
			l.lexIdentOrKeyword()
		default:
			l.errors.add(newError(ErrSyntax, token.Position{Offset: l.pos, Length: 1},
				"unexpected character %q", string(ch)))
			l.pos++
		}
	}

	l.tokens = append(l.tokens, token.Token{Type: token.EOF, Pos: token.Position{Offset: l.pos}})
}

func (l *lexer) lexIdentOrKeyword() {
	start := l.pos
	for l.pos < len(l.input) && isIdentChar(l.input[l.pos]) {
		l.pos++
	}
	word := l.input[start:l.pos]
	pos := token.Position{Offset: start, Length: l.pos - start}

	switch word {
	case "AND":
		l.tokens = append(l.tokens, token.Token{Type: token.And, Value: word, Pos: pos})
	case "OR":
		l.tokens = append(l.tokens, token.Token{Type: token.Or, Value: word, Pos: pos})
	case "NOT":
		l.tokens = append(l.tokens, token.Token{Type: token.Not, Value: word, Pos: pos})
	default:
		l.tokens = append(l.tokens, token.Token{Type: token.Ident, Value: word, Pos: pos})
	}
}

func (l *lexer) lexValue() {
	l.afterOperator = false
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return
	}

	start := l.pos
	var buf strings.Builder
	hasWildcard := false

	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == ')' {
			break
		}
		if ch == '.' && l.peek(1) == '.' {
			break
		}
		if ch == '\\' && l.pos+1 < len(l.input) {
			next := l.input[l.pos+1]
			switch next {
			case '*', '\\', '(', ')':
				buf.WriteByte(next)
				l.pos += 2
				continue
			}
		}
		if ch == '*' {
			hasWildcard = true
		}
		buf.WriteByte(ch)
		l.pos++
	}

	raw := l.input[start:l.pos]
	value := buf.String()
	pos := token.Position{Offset: start, Length: l.pos - start}

	if len(value) == 0 {
		return
	}

	tok := l.classifyValue(raw, value, hasWildcard, pos)
	l.tokens = append(l.tokens, tok)
}

func (l *lexer) classifyValue(raw, value string, hasWildcard bool, pos token.Position) token.Token {
	if hasWildcard {
		if !isValidWildcard(value) {
			l.errors.add(newError(ErrInvalidWildcard, pos,
				"invalid wildcard pattern %q: only prefix (foo*), suffix (*foo), and contains (*foo*) patterns are allowed", raw))
		}
		return token.Token{Type: token.Wildcard, Value: value, Pos: pos}
	}
	if value == "true" || value == "false" {
		return token.Token{Type: token.Boolean, Value: value, Pos: pos}
	}
	if isDateLiteral(value) {
		if _, err := time.Parse("2006-01-02", value); err != nil {
			l.errors.add(newError(ErrInvalidDate, pos, "invalid date %q", value))
		}
		return token.Token{Type: token.Date, Value: value, Pos: pos}
	}
	if isDurationLiteral(value) {
		return token.Token{Type: token.Duration, Value: value, Pos: pos}
	}
	if isIntegerLiteral(value) {
		return token.Token{Type: token.Integer, Value: value, Pos: pos}
	}
	if isFloatLiteral(value) {
		return token.Token{Type: token.Float, Value: value, Pos: pos}
	}
	return token.Token{Type: token.String, Value: value, Pos: pos}
}

func (l *lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
			break
		}
		l.pos++
	}
}

func (l *lexer) emit(typ token.Type, value string) {
	l.tokens = append(l.tokens, token.Token{
		Type:  typ,
		Value: value,
		Pos:   token.Position{Offset: l.start, Length: len(value)},
	})
}

func (l *lexer) peek(offset int) byte {
	idx := l.pos + offset
	if idx >= len(l.input) {
		return 0
	}
	return l.input[idx]
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9') || ch == '-'
}

func isValidWildcard(s string) bool {
	idx := strings.Index(s, "*")
	if idx == -1 {
		return true
	}
	stripped := strings.ReplaceAll(s, "*", "")
	stars := len(s) - len(stripped)
	if stars == 1 {
		return s[0] == '*' || s[len(s)-1] == '*'
	}
	if stars == 2 {
		return s[0] == '*' && s[len(s)-1] == '*'
	}
	return false
}

func isDateLiteral(s string) bool {
	if len(s) != 10 {
		return false
	}
	for i, ch := range s {
		switch i {
		case 4, 7:
			if ch != '-' {
				return false
			}
		default:
			if !unicode.IsDigit(ch) {
				return false
			}
		}
	}
	return true
}

func isDurationLiteral(s string) bool {
	if len(s) < 2 {
		return false
	}
	suffix := s[len(s)-1]
	if suffix != 'd' && suffix != 'h' && suffix != 'm' && suffix != 'w' {
		return false
	}
	for _, ch := range s[:len(s)-1] {
		if !unicode.IsDigit(ch) {
			return false
		}
	}
	return true
}

func isIntegerLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for _, ch := range s[start:] {
		if !unicode.IsDigit(ch) {
			return false
		}
	}
	return true
}

func isFloatLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	hasDot := false
	for _, ch := range s[start:] {
		if ch == '.' {
			if hasDot {
				return false
			}
			hasDot = true
			continue
		}
		if !unicode.IsDigit(ch) {
			return false
		}
	}
	return hasDot
}

// ParseDuration parses a duration literal like "1d", "4h", "30m", "2w".
// Go's time.ParseDuration does not support 'd' or 'w'.
func ParseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, newError(ErrInvalidDuration, token.Position{}, "invalid duration %q", s)
	}
	numStr := s[:len(s)-1]
	n := 0
	for _, r := range numStr {
		if r < '0' || r > '9' {
			return 0, newError(ErrInvalidDuration, token.Position{}, "invalid duration %q", s)
		}
		n = n*10 + int(r-'0')
	}
	switch s[len(s)-1] {
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	default:
		return 0, newError(ErrInvalidDuration, token.Position{}, "invalid duration suffix in %q", s)
	}
}
