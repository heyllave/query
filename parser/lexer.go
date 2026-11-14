package parser

import (
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	// expectInList is set right after an IN keyword; the next '(' opens an
	// IN list, inside which every element is lexed in value mode.
	expectInList bool
	inList       bool
	// parens records, for each '(' open in field position, whether it opened
	// a function call's arguments; argValue is set while lexing an operand in
	// one, where a ',' ends the value as it does in an IN list.
	parens   []bool
	argValue bool
}

// Lex tokenizes the input query string and returns the token stream.
func Lex(input string, maxLength int) ([]token.Token, error) {
	if maxLength > 0 && len(input) > maxLength {
		return nil, ErrorList{newError(CodeQueryTooLong, ErrQueryTooLong, token.Position{},
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
		return nil, ErrorList{newError(CodeQueryTooLong, ErrQueryTooLong, token.Position{},
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
			l.argValue = false
			continue
		}

		ch := l.input[l.pos]
		if ch != '(' {
			// Only a '(' directly after IN opens a list; anything else means
			// the word was a field name (IN=1, IN x(...)).
			l.expectInList = false
		}
		switch {
		case ch == '(':
			l.parens = append(l.parens, !l.expectInList && l.afterName())
			l.emit(token.LParen, "(")
			l.pos++
			if l.expectInList {
				l.expectInList = false
				l.inList = true
				l.afterOperator = true
			}
		case ch == ')':
			l.emit(token.RParen, ")")
			l.pos++
			l.inList = false
			if n := len(l.parens); n > 0 {
				l.parens = l.parens[:n-1]
			}
		case ch == ',':
			l.emit(token.Comma, ",")
			l.pos++
			if l.inList {
				l.afterOperator = true
			}
		case (ch == '-' || ch == '+') && isDigit(l.peek(1)) && l.afterListSep():
			// Signed literal in an argument or IN-list slot: addDays(d, -7).
			l.lexNumericLiteral()
		case isArithOpChar(ch) && l.continuesArith():
			// An arithmetic operator after whitespace continues the preceding
			// value: `total>10 - 3`. The operand that follows is lexed in value
			// mode so the parser sees the same token stream as `10-3`.
			l.emit(arithTokenFor(ch), string(ch))
			l.pos++
			l.afterOperator = true
			l.argValue = len(l.parens) > 0 && l.parens[len(l.parens)-1]
		case ch == '[':
			// Bracketed field name in field position: ["labels.x/y"]=1.
			l.lexFieldRef()
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
			// Report the whole rune, not its first byte, so the message
			// shows what was typed and the span covers it.
			_, width := utf8.DecodeRuneInString(l.input[l.pos:])
			l.errors.add(newError(CodeUnexpectedChar, ErrSyntax, token.Position{Offset: l.pos, Length: width},
				"unexpected character %q", l.input[l.pos:l.pos+width]))
			l.pos += width
		}
	}

	l.tokens = append(l.tokens, token.Token{Type: token.EOF, Pos: token.Position{Offset: l.pos}})
}

func (l *lexer) lexIdentOrKeyword() {
	start := l.pos
	l.pos = l.identEnd(l.pos)
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
		l.expectInList = true
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
// Wildcards and unquoted strings are NOT recognized here — they remain
// reserved for the after-operator value path. A leading sign is consumed when
// present (the callers only route here with a sign when a digit follows).
func (l *lexer) lexNumericLiteral() {
	start := l.pos
	if l.input[l.pos] == '-' || l.input[l.pos] == '+' {
		l.pos++
	}
	digitsStart := l.pos
	// Consume digits, hyphens at the two date positions (YYYY-MM-DD), dots
	// (for floats), and duration suffixes (d/h/m/w). The classifier below
	// picks one interpretation.
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
		if ch == '-' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) && digitsStart == start && isDatePrefix(l.input[digitsStart:l.pos]) {
			// A hyphen is part of the literal only where a date has one, so
			// 2026-01-01 stays a date while 10-3 is the subtraction 10 - 3.
			// A signed number is never a date (+0000-0 is +0000 minus 0).
			l.pos++
			continue
		}
		// Trailing duration suffix: a single unit letter not followed by
		// another letter (so 30m-1h is two durations, while 5min is a word).
		if isDurationSuffix(ch) && !isIdentStart(l.peek(1)) {
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

// readQuotedString decodes a double-quoted run with the opening quote at l.pos,
// advancing past the closing quote. It returns the decoded text and whether the
// string was terminated. Escape sequences: \", \\, \n, \t, \r; any other \x is
// the literal x. Shared by lexQuotedString and lexFieldRef.
func (l *lexer) readQuotedString() (string, bool) {
	l.pos++ // skip opening quote
	var buf strings.Builder
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '"' {
			l.pos++
			return buf.String(), true
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
	return buf.String(), false
}

// lexQuotedString lexes a double-quoted string and emits a String token. The
// opening quote is at l.pos.
func (l *lexer) lexQuotedString() {
	start := l.pos
	value, terminated := l.readQuotedString()
	pos := token.Position{Offset: start, Length: l.pos - start}
	if !terminated {
		l.errors.add(newError(CodeUnclosedString, ErrSyntax, pos, "unterminated string literal"))
		return
	}
	// Quoted strings are always treated as String values, never reclassified
	// as numbers, dates, booleans, or wildcards. This is the user's signal
	// that they want a literal string.
	l.tokens = append(l.tokens, token.Token{
		Type:   token.String,
		Value:  value,
		Pos:    pos,
		Quoted: true,
	})
	// Quoted strings are full values; we are no longer expecting one.
	l.afterOperator = false
}

// lexFieldRef lexes a bracketed field reference in value position with the
// opening '[' at l.pos: [name], [labels.dev], or ["name with-special.chars"].
// The bare form is a dot-separated path over the identifier alphabet; the quoted
// form takes the whole inner text as a single segment. Emits a FieldRef token.
func (l *lexer) lexFieldRef() {
	start := l.pos
	l.pos++ // skip '['
	l.skipWhitespace()

	var name string
	var quoted bool
	if l.pos < len(l.input) && l.input[l.pos] == '"' {
		var ok bool
		name, ok = l.readQuotedString()
		quoted = true
		if !ok {
			l.errors.add(newError(CodeUnclosedFieldRef, ErrSyntax,
				token.Position{Offset: start, Length: l.pos - start},
				"unterminated field reference"))
			return
		}
	} else {
		nameStart := l.pos
		for l.pos < len(l.input) {
			c := l.input[l.pos]
			if c == '.' {
				l.pos++
				continue
			}
			end := l.identEnd(l.pos)
			if end == l.pos {
				break
			}
			l.pos = end
		}
		name = l.input[nameStart:l.pos]
	}

	l.skipWhitespace()
	if l.pos >= len(l.input) || l.input[l.pos] != ']' {
		l.errors.add(newError(CodeUnclosedFieldRef, ErrSyntax,
			token.Position{Offset: start, Length: l.pos - start},
			"unterminated field reference"))
		return
	}
	l.pos++ // skip ']'

	if name == "" {
		l.errors.add(newError(CodeEmptyFieldRef, ErrSyntax,
			token.Position{Offset: start, Length: l.pos - start},
			"empty field reference"))
		return
	}

	l.tokens = append(l.tokens, token.Token{
		Type:   token.FieldRef,
		Value:  name,
		Pos:    token.Position{Offset: start, Length: l.pos - start},
		Quoted: quoted,
	})
	// A field reference is a complete value.
	l.afterOperator = false
}

// isDatePrefix reports whether the digits lexed so far are the part of a date
// literal that precedes a hyphen: the year (YYYY) or year and month (YYYY-MM).
func isDatePrefix(s string) bool {
	switch len(s) {
	case 4:
		return allDigits(s)
	case 7:
		return s[4] == '-' && allDigits(s[:4]) && allDigits(s[5:])
	default:
		return false
	}
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return len(s) > 0
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
// the previous token ends a value primary and a primary begins after the
// operator and any whitespace.
func (l *lexer) continuesArith() bool {
	if len(l.tokens) == 0 {
		return false
	}
	if !l.afterValue() {
		return false
	}
	i := l.pos + 1
	for i < len(l.input) && l.spaceWidth(i) > 0 {
		i += l.spaceWidth(i)
	}
	if i >= len(l.input) {
		return false
	}
	return l.beginsPrimary(i)
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

func isArithOpChar(ch byte) bool {
	return ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '%'
}

func arithTokenFor(ch byte) token.Type {
	switch ch {
	case '+':
		return token.Plus
	case '-':
		return token.Minus
	case '*':
		return token.Mul
	case '/':
		return token.Div
	default:
		return token.Mod
	}
}

func isSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
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
	//   (50000+1000)*1.1, now()-7d, daysAgo(7)+1d, 50000*1.1, [base]*1.1
	//
	// To avoid breaking wildcard semantics like `year=202*` (where `*` is a
	// wildcard suffix, not a partial multiplication), the look-ahead in
	// isArithValueAhead requires every arithmetic operator to have operands
	// on BOTH sides. This runs before the lone-field-ref branch so a field ref
	// followed by an operator ([base]*1.1) lexes as one arithmetic value.
	if l.isArithValueAhead() {
		l.lexArithValueExpr()
		return
	}

	// Bracketed field reference standing alone: field=[other] or
	// field>=["other-field"].
	if ch == '[' {
		l.lexFieldRef()
		return
	}

	// A second operator where the value belongs: state==draft, total=>5,
	// a<>b. Read as a string these would compare against "=draft" or ">5"
	// without a word of complaint, so they are refused; a value that really
	// starts with one of these characters is written in quotes.
	if ch == '=' || ch == '<' || ch == '>' {
		l.errors.add(newError(CodeUnexpectedChar, ErrSyntax, token.Position{Offset: l.pos, Length: 1},
			"unexpected %q where a value belongs: operators are single (=, !=, >, >=, <, <=); quote the value if it starts with %q",
			string(ch), string(ch)))
	}

	// Otherwise: unquoted string with possible wildcards. A bareword field
	// reference is written in brackets ([field], handled above) so it does not
	// collide with hyphenated idents and unquoted string values.
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
//
// A value run that contains a bareword — letters that are neither a function
// call nor a duration suffix — is never arithmetic: it is an unquoted string
// such as a UUID (550e8400-e29b-...) or a version (1.2.3-beta), even when a
// two-sided operator appears somewhere inside it.
func (l *lexer) isArithValueAhead() bool {
	if l.pos >= len(l.input) {
		return false
	}
	ch := l.input[l.pos]
	if ch == '(' {
		return true
	}
	// A field reference at the start routes to arithmetic, e.g. [base]*1.1.
	if ch == '[' {
		return true
	}
	// Function-call primary: <ident>(
	if isIdentStart(ch) {
		end := l.identEnd(l.pos)
		if end < len(l.input) && l.input[end] == '(' {
			return true
		}
	}
	// Otherwise scan the run: reject on a bareword, accept on a two-sided
	// arithmetic operator.
	depth := 0
	found := false
	for i := l.pos; i < len(l.input); i++ {
		c := l.input[i]
		if depth == 0 {
			if l.spaceWidth(i) > 0 || c == ')' || c == ',' {
				break
			}
			if c == '.' && i+1 < len(l.input) && l.input[i+1] == '.' {
				break
			}
		}
		switch {
		case c == '(':
			depth++
			continue
		case c == ')':
			depth--
			continue
		case c == '"':
			// Skip a quoted string primary.
			j := i + 1
			for j < len(l.input) && l.input[j] != '"' {
				if l.input[j] == '\\' {
					j++
				}
				j++
			}
			i = j
			continue
		case isDigit(c):
			// A digit run with more than one dot (10.0.0.1, 1.2.3) is an
			// address or a version, not a number, so 10.0.0.1/24 is one
			// string rather than a division.
			end, dots := i, 0
			for end < len(l.input) && (isDigit(l.input[end]) ||
				(l.input[end] == '.' && end+1 < len(l.input) && isDigit(l.input[end+1]))) {
				if l.input[end] == '.' {
					dots++
				}
				end++
			}
			if dots > 1 {
				return false
			}
			i = end - 1
			continue
		case isIdentStart(c):
			if isDurationSuffix(c) && i > l.pos && isDigit(l.input[i-1]) && (i+1 >= len(l.input) || !isIdentStart(l.input[i+1])) {
				// Duration suffix on a number: 7d, 30m, also in 30m-1h.
				continue
			}
			end := l.identEnd(i)
			if end < len(l.input) && l.input[end] == '(' {
				// Function call: a primary. Its arguments are the call's
				// business (field names are fine there), so skip to the
				// matching ')'.
				i = l.skipCall(end) - 1
				continue
			}
			return false // bareword → unquoted string
		}
		if c == '+' || c == '/' || c == '%' || c == '*' || (c == '-' && i > l.pos) {
			// A leading `-` is a sign, not subtraction; every other operator
			// counts only with operands on both sides.
			if l.hasOperandsAround(i) {
				found = true
			}
		}
	}
	return found
}

// skipCall returns the index just past the ')' that closes the call body
// opening at l.input[open] (or len(input) when unclosed), skipping quoted
// strings inside it.
func (l *lexer) skipCall(open int) int {
	depth := 0
	for i := open; i < len(l.input); i++ {
		switch l.input[i] {
		case '"':
			for i++; i < len(l.input) && l.input[i] != '"'; i++ {
				if l.input[i] == '\\' {
					i++
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(l.input)
}

func isDurationSuffix(c byte) bool {
	return c == 'd' || c == 'h' || c == 'w' || c == 'm'
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
// a digit (number end), `)` (closing call/group), `]` (closing field ref), or a
// duration suffix immediately preceded by a digit (so "7d" / "30m" register as
// primaries).
func (l *lexer) endsPrimary(i int) bool {
	c := l.input[i]
	if isDigit(c) || c == ')' || c == ']' {
		return true
	}
	if (c == 'd' || c == 'h' || c == 'w' || c == 'm') && i >= 1 && isDigit(l.input[i-1]) {
		return true
	}
	return false
}

// beginsPrimary reports whether l.input[i] starts a value primary: a digit or
// signed digit, an opening paren, an opening field-ref bracket, or the start
// of a function-call identifier.
func (l *lexer) beginsPrimary(i int) bool {
	c := l.input[i]
	if isDigit(c) || c == '(' || c == '[' || c == '"' {
		return true
	}
	if (c == '-' || c == '+') && i+1 < len(l.input) && isDigit(l.input[i+1]) {
		return true // signed number: 10*-3
	}
	if isIdentStart(c) {
		// Treat as primary only if it's a function call.
		end := l.identEnd(i)
		if end < len(l.input) && l.input[end] == '(' {
			return true
		}
	}
	return false
}

// lexUnquotedStringValue lexes the unquoted-string value form used for
// bareword strings and wildcard patterns. Stops at whitespace, ')', or '..'.
//
// Escapes: \* \\ \( \) stand for the literal character. A value with an
// unescaped star is a wildcard pattern and its token value is in pattern
// form (see encodePattern), so a literal star or backslash stays
// distinguishable from a wildcard star.
func (l *lexer) lexUnquotedStringValue() {
	start := l.pos
	var buf strings.Builder
	hasWildcard := false
	var literalStars []int // byte offsets in buf of escaped stars

	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if l.spaceWidth(l.pos) > 0 || ch == ')' || ch == '[' || ch == ']' {
			break
		}
		if ch == ',' && (l.inList || l.argValue) {
			break
		}
		if ch == '.' && l.peek(1) == '.' {
			break
		}
		if ch == '\\' && l.pos+1 < len(l.input) {
			next := l.input[l.pos+1]
			switch next {
			case '*', '\\', '(', ')':
				if next == '*' {
					literalStars = append(literalStars, buf.Len())
				}
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
	if hasWildcard {
		value = encodePattern(value, literalStars)
	}
	pos := token.Position{Offset: start, Length: l.pos - start}

	if len(value) == 0 {
		return
	}

	tok := l.classifyValue(raw, value, hasWildcard, pos)
	if raw != value {
		tok.Raw = raw
	}
	l.tokens = append(l.tokens, tok)
}

// encodePattern writes a decoded wildcard value in pattern form: a
// backslash becomes `\\`, a literal star (one that was escaped in the source,
// at the given byte offsets of value) becomes `\*`, and a bare `*` stays a
// wildcard. ast.DecodePattern reverses it; the matcher and SQL lowering read
// the pattern form so a literal backslash can never be mistaken for the
// escape of a star.
func encodePattern(value string, literalStars []int) string {
	var b strings.Builder
	next := 0
	for i := 0; i < len(value); i++ {
		switch {
		case next < len(literalStars) && literalStars[next] == i:
			b.WriteString(`\*`)
			next++
		case value[i] == '\\':
			b.WriteString(`\\`)
		default:
			b.WriteByte(value[i])
		}
	}
	return b.String()
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
	l.start = l.pos
	l.emit(token.LParen, "(")
	l.pos++
	depth := 1
	for l.pos < len(l.input) && depth > 0 {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}
		l.start = l.pos
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
		case ch == '[':
			// Bracketed field name as an argument: abs([cpu-limit]).
			l.lexFieldRef()
		case ch == '.' && l.peek(1) != '.':
			// A dotted field path as an argument: lower(labels.team).
			l.emit(token.Dot, ".")
			l.pos++
		case isArithOpChar(ch) && l.afterValue():
			// An operator after a complete operand: addDays(d, 1-8).
			l.emit(arithTokenFor(ch), string(ch))
			l.pos++
		case isDigit(ch), (ch == '-' || ch == '+') && isDigit(l.peek(1)):
			l.lexNumericLiteral()
		case isIdentStart(ch):
			l.lexIdentOrKeyword()
		default:
			_, width := utf8.DecodeRuneInString(l.input[l.pos:])
			l.errors.add(newError(CodeUnexpectedChar, ErrSyntax,
				token.Position{Offset: l.pos, Length: width},
				"unexpected character %q in function arguments", l.input[l.pos:l.pos+width]))
			l.pos += width
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
		l.start = l.pos
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
	if ch == '[' {
		l.lexFieldRef()
		return true
	}
	if ch == ')' {
		l.errors.add(newError(CodeExpectedValue, ErrSyntax, token.Position{Offset: l.pos, Length: 1},
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
		l.pos = l.identEnd(l.pos)
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
	l.errors.add(newError(CodeUnexpectedChar, ErrSyntax, token.Position{Offset: l.pos, Length: 1},
		"unexpected character %q in value expression", string(ch)))
	l.pos++
	return false
}

func (l *lexer) classifyValue(raw, value string, hasWildcard bool, pos token.Position) token.Token {
	if hasWildcard {
		if !isValidWildcard(value) {
			l.errors.add(newError(CodeInvalidWildcard, ErrInvalidWildcard, pos,
				"invalid wildcard pattern %q: only prefix (foo*), suffix (*foo), and contains (*foo*) patterns are allowed", raw))
		}
		return token.Token{Type: token.Wildcard, Value: value, Pos: pos}
	}
	if value == "true" || value == "false" {
		return token.Token{Type: token.Boolean, Value: value, Pos: pos}
	}
	if isDateLiteral(value) || isDatetimeLiteral(value) {
		if _, err := ParseDateLiteral(value); err != nil {
			l.errors.add(newError(CodeInvalidDate, ErrInvalidDate, pos, "invalid date %q", value))
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
		w := l.spaceWidth(l.pos)
		if w == 0 {
			break
		}
		l.pos += w
	}
}

// spaceWidth returns the byte width of the whitespace character at i, or 0
// when there is none. Unicode spaces count: a no-break space pasted between
// two words separates them like an ordinary one instead of gluing them into
// a single term.
func (l *lexer) spaceWidth(i int) int {
	c := l.input[i]
	if c < utf8.RuneSelf {
		if isSpace(c) {
			return 1
		}
		return 0
	}
	if r, w := utf8.DecodeRuneInString(l.input[i:]); unicode.IsSpace(r) {
		return w
	}
	return 0
}

// identEnd returns the index just past the identifier characters starting at
// i. A non-ASCII rune continues an identifier unless it is whitespace.
func (l *lexer) identEnd(i int) int {
	for i < len(l.input) {
		c := l.input[i]
		if c < utf8.RuneSelf {
			if !isIdentChar(c) {
				break
			}
			i++
			continue
		}
		r, w := utf8.DecodeRuneInString(l.input[i:])
		if unicode.IsSpace(r) {
			break
		}
		i += w
	}
	return i
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

// isIdentStart reports whether ch may begin an identifier: an ASCII letter,
// '_', or any byte of a multi-byte UTF-8 sequence, so a non-ASCII word such
// as café or 日本 is one bare term rather than a run of unexpected bytes.
func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch >= utf8.RuneSelf
}

// isIdentChar reports whether ch may continue an identifier. Hyphens and
// slashes are included so hyphenated names and label keys such as
// labels.example.com/team lex as one field segment.
func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9') || ch == '-' || ch == '/'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isValidWildcard reports whether s is a prefix (foo*), suffix (*foo) or
// contains (*foo*) pattern. An escaped star (`\*`) is a literal and does not
// count.
func isValidWildcard(s string) bool {
	stars := 0
	first, last := false, false
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			continue
		}
		if s[i] == '*' {
			stars++
			first = first || i == 0
			last = i == len(s)-1
		}
	}
	switch stars {
	case 0:
		return true
	case 1:
		return first || last
	case 2:
		return first && last
	default:
		return false
	}
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
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

// isDatetimeLiteral reports whether s has the shape of an RFC 3339 timestamp
// (YYYY-MM-DDTHH:MM:SS with optional fraction and zone) or the same with a
// space separator. ParseDateLiteral decides whether it is a valid instant.
func isDatetimeLiteral(s string) bool {
	if len(s) < 19 || !isDateLiteral(s[:10]) {
		return false
	}
	if s[10] != 'T' && s[10] != 't' && s[10] != ' ' {
		return false
	}
	for i := 11; i < len(s); i++ {
		c := s[i]
		if !isDigit(c) && c != ':' && c != '.' && c != '+' && c != '-' && c != 'Z' && c != 'z' {
			return false
		}
	}
	return true
}

// dateLayouts are the literal forms ParseDateLiteral accepts, from the most
// to the least specific.
var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// IsDayLiteral reports whether s is a date literal without a time part
// (YYYY-MM-DD), which denotes a whole day rather than an instant.
func IsDayLiteral(s string) bool { return isDateLiteral(s) }

// ParseDateLiteral parses a date (2026-01-01) or datetime literal
// (2026-01-01T10:00:00Z, 2026-01-01T10:00:00, 2026-01-01 10:00:00). A date
// without a time is midnight UTC; a datetime without a zone is UTC.
func ParseDateLiteral(s string) (time.Time, error) {
	var firstErr error
	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return time.Time{}, firstErr
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
		if ch < '0' || ch > '9' {
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
		if ch < '0' || ch > '9' {
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
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return hasDot
}

// ParseDuration parses a duration literal like "1d", "4h", "30m", "2w".
// Go's time.ParseDuration does not support 'd' or 'w'.
func ParseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, newError(CodeInvalidDuration, ErrInvalidDuration, token.Position{}, "invalid duration %q", s)
	}
	var unit time.Duration
	switch s[len(s)-1] {
	case 'm':
		unit = time.Minute
	case 'h':
		unit = time.Hour
	case 'd':
		unit = 24 * time.Hour
	case 'w':
		unit = 7 * 24 * time.Hour
	default:
		return 0, newError(CodeInvalidDuration, ErrInvalidDuration, token.Position{}, "invalid duration suffix in %q", s)
	}
	// A count too large for time.Duration is an error, never a wrapped value.
	limit := int64(math.MaxInt64 / unit)
	var n int64
	for _, r := range s[:len(s)-1] {
		if r < '0' || r > '9' {
			return 0, newError(CodeInvalidDuration, ErrInvalidDuration, token.Position{}, "invalid duration %q", s)
		}
		n = n*10 + int64(r-'0')
		if n > limit {
			return 0, newError(CodeInvalidDuration, ErrInvalidDuration, token.Position{}, "duration %q is out of range", s)
		}
	}
	return time.Duration(n) * unit, nil
}
