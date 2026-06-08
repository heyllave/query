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

// LexValue tokenizes input as a value expression, starting in value mode so a
// leading literal, arithmetic expression, or function call lexes correctly at
// position 0 (where the boolean grammar would expect a field name). It backs
// [ParseValue].
func LexValue(input string, maxLength int) ([]token.Token, error) {
	if maxLength > 0 && len(input) > maxLength {
		return nil, ErrorList{newError(ErrQueryTooLong, token.Position{},
			"query length %d exceeds maximum of %d characters", len(input), maxLength)}
	}

	l := &lexer{input: input, afterOperator: true}
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
		case ch == '!' && l.peek(1) == '>' && l.peek(2) == '=':
			l.emit(token.Ngte, "!>=")
			l.pos += 3
			l.afterOperator = true
		case ch == '!' && l.peek(1) == '<' && l.peek(2) == '=':
			l.emit(token.Nlte, "!<=")
			l.pos += 3
			l.afterOperator = true
		case ch == '!' && l.peek(1) == '>':
			l.emit(token.Ngt, "!>")
			l.pos += 2
			l.afterOperator = true
		case ch == '!' && l.peek(1) == '<':
			l.emit(token.Nlt, "!<")
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
		case ch == '"':
			l.lexQuotedString()
		case isIdentStart(ch):
			l.lexIdentOrKeyword()
		case isDigit(ch):
			l.lexNumericLiteral()
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

	// Keywords are matched case-insensitively. The original casing is preserved
	// in Token.Value so consumers (and round-trip String()) can faithfully
	// reproduce the input.
	switch strings.ToUpper(word) {
	case "AND":
		l.tokens = append(l.tokens, token.Token{Type: token.And, Value: word, Pos: pos})
	case "OR":
		l.tokens = append(l.tokens, token.Token{Type: token.Or, Value: word, Pos: pos})
	case "NOT":
		l.tokens = append(l.tokens, token.Token{Type: token.Not, Value: word, Pos: pos})
	case "IN":
		l.tokens = append(l.tokens, token.Token{Type: token.In, Value: word, Pos: pos})
	default:
		// "true" and "false" are reserved boolean literals in any position —
		// matching what classifyValue does for the after-operator path. Their
		// casing is preserved on the token like AND/OR/NOT.
		if word == "true" || word == "false" {
			l.tokens = append(l.tokens, token.Token{Type: token.Boolean, Value: word, Pos: pos})
			return
		}
		l.tokens = append(l.tokens, token.Token{Type: token.Ident, Value: word, Pos: pos})
	}
}

// lexNumericLiteral lexes a number/date/duration literal at a non-value position.
// This lets function arguments and IN-list values include integer, float,
// date, and duration literals without needing the lexer to be in value mode.
//
//	addDays(date, 7)            // integer literal
//	IN(2020-01-01, 2020-12-31)  // date literals
//	IN(1d, 1w)                  // duration literals
//	year=2026                   // (an after-operator value; goes through lexValue)
//
// Wildcards, unquoted strings, and signed numbers are NOT recognized here —
// they remain reserved for the after-operator value path.
func (l *lexer) lexNumericLiteral() {
	start := l.pos
	// Consume digits, hyphens (for dates), dots (for floats), and duration
	// suffixes (d/h/m/w). The classifier below picks one interpretation.
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if isDigit(ch) {
			l.pos++
			continue
		}
		if ch == '.' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
			l.pos++
			continue
		}
		if ch == '-' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
			// Hyphen-followed-by-digit appears only inside dates (YYYY-MM-DD).
			// We only consume it if we're not yet past position 4 from start
			// (i.e., the year portion) OR position 7 (month portion). Easier:
			// consume any hyphen-digit run and let the classifier validate.
			l.pos++
			continue
		}
		// Trailing duration suffix.
		if ch == 'd' || ch == 'h' || ch == 'w' || (ch == 'm' && !isIdentChar(l.peek(1))) {
			l.pos++
			break
		}
		break
	}
	value := l.input[start:l.pos]
	pos := token.Position{Offset: start, Length: l.pos - start}
	tok := l.classifyValue(value, value, false, pos)
	l.tokens = append(l.tokens, tok)
}

// lexQuotedString lexes a double-quoted string and emits a String token. The
// opening quote is at l.pos. Supports the escape sequences: \", \\, \n, \t, \r.
// Any other \x is preserved as the literal character x.
func (l *lexer) lexQuotedString() {
	start := l.pos
	l.pos++ // skip opening quote
	var buf strings.Builder
	terminated := false
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '"' {
			terminated = true
			l.pos++
			break
		}
		if ch == '\\' && l.pos+1 < len(l.input) {
			next := l.input[l.pos+1]
			switch next {
			case '"', '\\':
				buf.WriteByte(next)
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case 'r':
				buf.WriteByte('\r')
			default:
				buf.WriteByte(next)
			}
			l.pos += 2
			continue
		}
		buf.WriteByte(ch)
		l.pos++
	}
	pos := token.Position{Offset: start, Length: l.pos - start}
	if !terminated {
		l.errors.add(newError(ErrSyntax, pos, "unterminated string literal"))
		return
	}
	// Quoted strings are always treated as String values, never reclassified
	// as numbers, dates, booleans, or wildcards. This is the user's signal
	// that they want a literal string.
	l.tokens = append(l.tokens, token.Token{
		Type:   token.String,
		Value:  buf.String(),
		Pos:    pos,
		Quoted: true,
	})
	// Quoted strings are full values; we are no longer expecting one.
	l.afterOperator = false
}

func (l *lexer) lexValue() {
	l.afterOperator = false
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return
	}

	ch := l.input[l.pos]

	// Quoted string after operator: field="hello world"
	if ch == '"' {
		l.lexQuotedString()
		return
	}

	// Arithmetic-style value. Examples:
	//   (50000+1000)*1.1, now()-7d, daysAgo(7)+1d, 50000*1.1
	//
	// To avoid breaking wildcard semantics like `year=202*` (where `*` is a
	// wildcard suffix, not a partial multiplication), the look-ahead in
	// isArithValueAhead requires every arithmetic operator to have operands
	// on BOTH sides.
	if l.isArithValueAhead() {
		l.lexArithValueExpr()
		return
	}

	// Otherwise: unquoted string with possible wildcards. Field references
	// inside arithmetic are intentionally not supported (would collide with
	// hyphenated idents and unquoted strings); use a custom function instead.
	l.lexUnquotedStringValue()
}

// isArithValueAhead scans the value run starting at l.pos and reports whether
// it should be lexed as an arithmetic expression. The scan stops at depth-0
// whitespace, `)`, `,`, `..`, or EOF. Arithmetic requires either:
//   - a paren group at the start (e.g. "(50000+1000)*1.1")
//   - a function-call primary at the start (e.g. "now()-7d")
//   - an unambiguous arithmetic operator with operand characters on both sides
//     ("50000*1.1", "now()-7d") — single-sided operators like `202*` remain
//     wildcards.
func (l *lexer) isArithValueAhead() bool {
	if l.pos >= len(l.input) {
		return false
	}
	ch := l.input[l.pos]
	if ch == '(' {
		return true
	}
	// Function-call primary: <ident>(
	if isIdentStart(ch) {
		end := l.pos
		for end < len(l.input) && isIdentChar(l.input[end]) {
			end++
		}
		if end < len(l.input) && l.input[end] == '(' {
			return true
		}
	}
	// Otherwise scan ahead looking for a two-sided arithmetic operator.
	depth := 0
	for i := l.pos; i < len(l.input); i++ {
		c := l.input[i]
		if depth == 0 {
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ')' || c == ',' {
				return false
			}
			if c == '.' && i+1 < len(l.input) && l.input[i+1] == '.' {
				return false
			}
		}
		if c == '(' {
			depth++
			continue
		}
		if c == ')' {
			depth--
			continue
		}
		if c == '+' || c == '/' || c == '%' {
			if l.hasOperandsAround(i) {
				return true
			}
		}
		if c == '-' && i > l.pos {
			// Mid-value `-` is subtraction only with operands on both sides.
			// Leading `-` (unary) doesn't count — startsArithValue handles
			// signed-numeric primaries via the digit path.
			if l.hasOperandsAround(i) {
				return true
			}
		}
		if c == '*' {
			if l.hasOperandsAround(i) {
				return true
			}
		}
	}
	return false
}

// hasOperandsAround reports whether the byte at position i is bracketed by
// valid arithmetic operands. Bareword identifiers don't count — they're
// either part of a wildcard pattern (`a*b`) or unsupported field refs. Only
// digits, durations (`7d` / `4h` / `2w` / `30m`), closing parens (end of a
// function call or sub-group), and function-call starts qualify.
func (l *lexer) hasOperandsAround(i int) bool {
	if i == 0 || i+1 >= len(l.input) {
		return false
	}
	return l.endsPrimary(i-1) && l.beginsPrimary(i+1)
}

// endsPrimary reports whether l.input[i] is the last byte of a value primary:
// a digit (number end), `)` (closing call/group), or a duration suffix
// immediately preceded by a digit (so "7d" / "30m" register as primaries).
func (l *lexer) endsPrimary(i int) bool {
	c := l.input[i]
	if isDigit(c) || c == ')' {
		return true
	}
	if (c == 'd' || c == 'h' || c == 'w' || c == 'm') && i >= 1 && isDigit(l.input[i-1]) {
		return true
	}
	return false
}

// beginsPrimary reports whether l.input[i] starts a value primary: a digit,
// an opening paren, or the start of a function-call identifier.
func (l *lexer) beginsPrimary(i int) bool {
	c := l.input[i]
	if isDigit(c) || c == '(' {
		return true
	}
	if isIdentStart(c) {
		// Treat as primary only if it's a function call.
		end := i
		for end < len(l.input) && isIdentChar(l.input[end]) {
			end++
		}
		if end < len(l.input) && l.input[end] == '(' {
			return true
		}
	}
	return false
}

// lexUnquotedStringValue lexes the unquoted-string value form used for
// bareword strings and wildcard patterns. Stops at whitespace, ')', or '..'.
func (l *lexer) lexUnquotedStringValue() {
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

// lexFuncCallBody lexes everything from the opening `(` of a function call
// (which is currently at l.pos) through to the matching `)`. Used by the
// arithmetic-value lexer so a call like `now()` inside `now()-7d` can be
// consumed in one chunk, leaving the surrounding arith mode free to look for
// trailing operators after the closing paren.
func (l *lexer) lexFuncCallBody() {
	if l.pos >= len(l.input) || l.input[l.pos] != '(' {
		return
	}
	l.emit(token.LParen, "(")
	l.pos++
	depth := 1
	for l.pos < len(l.input) && depth > 0 {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}
		ch := l.input[l.pos]
		switch {
		case ch == '(':
			l.emit(token.LParen, "(")
			l.pos++
			depth++
		case ch == ')':
			l.emit(token.RParen, ")")
			l.pos++
			depth--
		case ch == ',':
			l.emit(token.Comma, ",")
			l.pos++
		case ch == '"':
			l.lexQuotedString()
		case isDigit(ch):
			l.lexNumericLiteral()
		case isIdentStart(ch):
			l.lexIdentOrKeyword()
		default:
			l.errors.add(newError(ErrSyntax,
				token.Position{Offset: l.pos, Length: 1},
				"unexpected character %q in function arguments", string(ch)))
			l.pos++
		}
	}
}

// lexArithValueExpr lexes an arithmetic-style value expression: a sequence of
// numeric / date / duration / quoted-string / function-call primaries separated
// by arithmetic operators (+ - * / %), with parentheses for grouping. Stops at
// depth-0 whitespace before a logical keyword, an unmatched ')', '..', or EOF.
//
//	50000*1.1
//	(50000+1000)*1.1
//	now()-7d
//	addDays(start, 30)+1d
//
// Field references are NOT permitted as arithmetic operands — bareword
// identifiers in value position are reserved for IN-list values and unquoted
// string compatibility. Use a custom function for field-based arithmetic.
func (l *lexer) lexArithValueExpr() {
	depth := 0
	expectPrimary := true
	for l.pos < len(l.input) {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}
		ch := l.input[l.pos]

		// At depth 0, certain markers end the value expression.
		if depth == 0 {
			if ch == '.' && l.peek(1) == '.' {
				return
			}
			if ch == ',' {
				return
			}
			if ch == ')' {
				return
			}
		}

		if expectPrimary {
			// `(` opens a sub-expression. We stay in the primary-expected state
			// for the contents.
			if ch == '(' {
				l.emit(token.LParen, "(")
				l.pos++
				depth++
				continue
			}
			if !l.lexArithPrimary() {
				return
			}
			expectPrimary = false
			continue
		}

		// Operator slot.
		switch ch {
		case '+':
			l.emit(token.Plus, "+")
			l.pos++
			expectPrimary = true
		case '-':
			l.emit(token.Minus, "-")
			l.pos++
			expectPrimary = true
		case '*':
			l.emit(token.Mul, "*")
			l.pos++
			expectPrimary = true
		case '/':
			l.emit(token.Div, "/")
			l.pos++
			expectPrimary = true
		case '%':
			l.emit(token.Mod, "%")
			l.pos++
			expectPrimary = true
		case ')':
			// Closes the inner sub-expression. We just finished a primary, so
			// expectPrimary stays false — the next token is an operator or
			// the outer terminator.
			l.emit(token.RParen, ")")
			l.pos++
			depth--
		case ',':
			l.emit(token.Comma, ",")
			l.pos++
			expectPrimary = true
		default:
			// Anything else (letter, etc.) ends the value expression.
			return
		}
	}
}

// lexArithPrimary lexes a single non-paren primary inside an arithmetic-value
// expression. Returns false when no primary could be lexed.
func (l *lexer) lexArithPrimary() bool {
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return false
	}
	ch := l.input[l.pos]

	if ch == '"' {
		l.lexQuotedString()
		return true
	}
	if ch == ')' {
		l.errors.add(newError(ErrSyntax, token.Position{Offset: l.pos, Length: 1},
			"expected value, got ')'"))
		return false
	}
	// Signed numeric primary: -50, +3.14.
	if (ch == '-' || ch == '+') && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
		l.lexNumericLiteral()
		return true
	}
	if isDigit(ch) {
		l.lexNumericLiteral()
		return true
	}
	if isIdentStart(ch) {
		start := l.pos
		for l.pos < len(l.input) && isIdentChar(l.input[l.pos]) {
			l.pos++
		}
		word := l.input[start:l.pos]
		pos := token.Position{Offset: start, Length: l.pos - start}
		if l.pos < len(l.input) && l.input[l.pos] == '(' {
			l.tokens = append(l.tokens, token.Token{Type: token.Ident, Value: word, Pos: pos})
			l.lexFuncCallBody()
			return true
		}
		// Bareword without '(' — emit as a string primary (rare; e.g.
		// inside a paren-grouped IN-style construct).
		l.tokens = append(l.tokens, token.Token{Type: token.String, Value: word, Pos: pos})
		return true
	}
	l.errors.add(newError(ErrSyntax, token.Position{Offset: l.pos, Length: 1},
		"unexpected character %q in value expression", string(ch)))
	l.pos++
	return false
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

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
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
