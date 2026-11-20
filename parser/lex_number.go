package parser

import (
	"github.com/heyllave/query/parser/internal/chars"
	"github.com/heyllave/query/parser/internal/literal"
	"github.com/heyllave/query/token"
)

// lexNumericLiteral lexes a number, date or duration literal, with an
// optional sign, as one primary. It serves function arguments, IN lists and
// the operands of arithmetic:
//
//	addDays(date, 7)            // an integer
//	IN(2020-01-01, 2020-12-31)  // dates
//	IN(1d, 1w)                  // durations
//	now()-7d                    // an operand
//
// Wildcards and bare words are not recognised here; they belong to the
// value path after an operator.
func (l *lexer) lexNumericLiteral() {
	start := l.pos
	if chars.IsSign(l.input[l.pos]) {
		l.pos++
	}
	digitsStart := l.pos
	// A date with a time is one primary, so 2026-11-16T10:00:00Z-1h is that
	// instant minus an hour. Its zone offset (+02:00) is part of it, not a
	// sum. A signed number is never a date.
	if end := literal.DatetimeEnd(l.input, start); digitsStart == start && end > 0 {
		l.pos = end
		l.emitLiteral(start)
		return
	}
	l.pos = numberRunEnd(l.input, start, digitsStart)
	l.giveBackShortDate(start, digitsStart)
	l.emitLiteral(start)
}

// numberRunEnd returns the end of the digits, dots, date hyphens and
// duration unit that start at s[digitsStart]. A hyphen is taken only where a
// date has one (after YYYY or YYYY-MM), so 2026-01-01 stays together while
// 10-3 is 10, a minus and 3. A unit letter counts only when no letter
// follows it, so 30m-1h is two durations while 5min is a word.
func numberRunEnd(s string, start, digitsStart int) int {
	i := digitsStart
	for i < len(s) {
		ch := s[i]
		next := chars.At(s, i+1)
		switch {
		case chars.IsDigit(ch), ch == '.' && chars.IsDigit(next):
			i++
		case ch == '-' && chars.IsDigit(next) && digitsStart == start && literal.IsDatePrefix(s[digitsStart:i]):
			i++
		case chars.IsDurationUnit(ch) && !chars.IsIdentStart(next):
			return i + 1
		default:
			return i
		}
	}
	return i
}

// giveBackShortDate undoes the hyphens taken on the promise of a date when
// the run did not turn out to be one (2026-1, 2026-11-5): it is the number
// 2026 and a subtraction, the reading 10-3 gets, rather than a piece of text
// only four leading digits could produce.
func (l *lexer) giveBackShortDate(start, digitsStart int) {
	year := digitsStart + 4
	if digitsStart == start && l.pos > year && l.input[year] == '-' &&
		literal.IsDatePrefix(l.input[digitsStart:year]) && !literal.IsDate(l.input[start:l.pos]) {
		l.pos = year
	}
}

// emitLiteral appends the literal from start to l.pos, classified.
func (l *lexer) emitLiteral(start int) {
	value := l.input[start:l.pos]
	pos := token.Position{Offset: start, Length: l.pos - start}
	l.tokens = append(l.tokens, l.classifyValue(value, value, false, pos))
}

// classifyValue gives an unquoted literal its token type: a pattern, a date,
// or by literalType. A pattern that is not prefix, suffix or contains, and a
// date that cannot exist, are reported here.
func (l *lexer) classifyValue(raw, value string, hasWildcard bool, pos token.Position) token.Token {
	if hasWildcard {
		if !literal.IsValidWildcard(value) {
			l.errors.add(newError(CodeInvalidWildcard, ErrInvalidWildcard, pos,
				"invalid wildcard pattern %q: only prefix (foo*), suffix (*foo), and contains (*foo*) patterns are allowed", raw))
		}
		return token.Token{Type: token.Wildcard, Value: value, Pos: pos}
	}
	if literal.IsDate(value) || literal.IsDatetime(value) {
		if _, err := literal.ParseDate(value); err != nil {
			l.errors.add(newError(CodeInvalidDate, ErrInvalidDate, pos, "invalid date %q", value))
		}
		return token.Token{Type: token.Date, Value: value, Pos: pos}
	}
	return token.Token{Type: literalType(value), Value: value, Pos: pos}
}

// literalType is the type of a literal that is not a pattern or a date.
func literalType(value string) token.Type {
	switch {
	case value == "true" || value == "false":
		return token.Boolean
	case literal.IsDuration(value):
		return token.Duration
	case literal.IsInteger(value):
		return token.Integer
	case literal.IsFloat(value):
		return token.Float
	}
	return token.String
}
