// Package shape decides whether an unquoted value is arithmetic or text.
// It is the one place where the lexer looks ahead: a run such as 10-3 is a
// subtraction, while 10-x, 550e8400-e29b and 1.2.3-4 are text. The rules are
// the Grammar page's table, one function per rule, and every function is
// pure on the input and an index.
package shape

import (
	"github.com/heyllave/query/parser/internal/chars"
	"github.com/heyllave/query/parser/internal/literal"
)

// IsArithmetic reports whether the value starting at s[pos] is lexed as an
// arithmetic expression. A group, a field reference or a call at the start
// is always arithmetic. Otherwise the run up to the next space, ')' , ',' or
// '..' is arithmetic only when it holds no bare word and has an operator
// with an operand on both sides, so a star at one end stays a wildcard.
func IsArithmetic(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	switch c := s[pos]; {
	case c == '(':
		return true
	case c == '*':
		// No arithmetic begins with an operator: *0% is a pattern.
		return false
	case c == '[':
		return true
	case chars.IsCallAt(s, pos):
		return true
	}
	return scanRun(s, pos)
}

// RunEnd returns where the value run starting at s[pos] ends: the first
// space, ')', ',' or '..' outside a group, a quoted string or a field
// reference, or len(s). It is the span IsArithmetic judges, so a reader of
// an arithmetic value must not stop before it.
func RunEnd(s string, pos int) int {
	depth := 0
	for i := pos; i < len(s); i++ {
		if depth == 0 && endsRun(s, i) {
			return i
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '"':
			i = chars.QuotedEnd(s, i)
		case '[':
			if end := chars.SkipFieldRef(s, i); end >= 0 {
				i = end
			}
		}
	}
	return len(s)
}

// run is the state of one scan: where it began, how deep in parentheses it
// is, and whether a two-sided operator has been seen.
type run struct {
	s     string
	pos   int
	depth int
	found bool
}

// scanRun walks the value run starting at s[pos]. A group left open is not
// arithmetic, which keeps the answer the same whether the value is read
// inside a query or on its own.
func scanRun(s string, pos int) bool {
	r := run{s: s, pos: pos}
	for i := pos; i < len(s); i++ {
		if r.depth == 0 && endsRun(s, i) {
			break
		}
		last, ok := r.step(i)
		if !ok {
			return false
		}
		i = last
	}
	return r.found && r.depth == 0
}

// endsRun reports whether s[i] ends a value run at depth 0.
func endsRun(s string, i int) bool {
	c := s[i]
	return chars.SpaceWidth(s, i) > 0 || c == ')' || c == ',' ||
		(c == '.' && i+1 < len(s) && s[i+1] == '.')
}

// step reads the primary or operator at s[i]. It returns the index of the
// last character it took, and false when the run is text.
func (r *run) step(i int) (last int, ok bool) {
	c := r.s[i]
	switch {
	case c == '(':
		r.depth++
		return i, true
	case c == ')':
		r.depth--
		return i, true
	case c == '"':
		return chars.QuotedEnd(r.s, i), true
	case c == '[':
		// A bracketed field is a primary whatever its name holds, so
		// 2*[unit-price] is arithmetic and the name is not a bare word.
		end := chars.SkipFieldRef(r.s, i)
		return end, end >= 0
	case chars.IsDigit(c):
		return r.number(i)
	case chars.IsIdentStart(c):
		return r.word(i)
	}
	r.operator(i)
	return i, true
}

// number takes a date with a time, which is one operand whose T, colons and
// zone are not words or operators, or a number. A number with more than one
// dot (10.0.0.1, 1.2.3) is an address or a version, so the run is text.
func (r *run) number(i int) (last int, ok bool) {
	if end := literal.DatetimeEnd(r.s, i); end > 0 {
		if end+1 < len(r.s) && chars.IsSign(r.s[end]) && BeginsPrimary(r.s, end+1) {
			r.found = true
		}
		return end - 1, true
	}
	end, dots := numberEnd(r.s, i)
	return end - 1, dots <= 1
}

// numberEnd returns the index past the digits and dots starting at s[i],
// and how many dots it took. A dot counts only with a digit after it.
func numberEnd(s string, i int) (end, dots int) {
	for end = i; end < len(s); end++ {
		if s[end] == '.' && end+1 < len(s) && chars.IsDigit(s[end+1]) {
			dots++
			continue
		}
		if !chars.IsDigit(s[end]) {
			break
		}
	}
	return end, dots
}

// word takes a duration unit after a digit (7d, and 30m in 30m-1h) or a
// whole function call. Any other word makes the run text: a UUID, a
// version or a name.
func (r *run) word(i int) (last int, ok bool) {
	if r.isUnitAt(i) {
		return i, true
	}
	end := chars.IdentEnd(r.s, i)
	if end < len(r.s) && r.s[end] == '(' {
		// A call left open is not arithmetic, like a group left open.
		closeAt := chars.SkipCall(r.s, end)
		return closeAt - 1, closeAt >= 0
	}
	return i, false
}

// isUnitAt reports whether s[i] is a duration unit closing a number.
func (r *run) isUnitAt(i int) bool {
	s := r.s
	return chars.IsDurationUnit(s[i]) && i > r.pos && chars.IsDigit(s[i-1]) &&
		(i+1 >= len(s) || !chars.IsIdentStart(s[i+1]))
}

// operator records an arithmetic operator with an operand on both sides. A
// leading '-' is a sign, not a subtraction.
func (r *run) operator(i int) {
	c := r.s[i]
	if c == '+' || c == '/' || c == '%' || c == '*' || (c == '-' && i > r.pos) {
		if OperandsAround(r.s, i) {
			r.found = true
		}
	}
}

// OperandsAround reports whether the operator at s[i] has a value primary
// ending just before it and one beginning just after it. A bare word does
// not count: in a*b it is part of a pattern.
func OperandsAround(s string, i int) bool {
	if i == 0 || i+1 >= len(s) {
		return false
	}
	return EndsPrimary(s, i-1) && BeginsPrimary(s, i+1)
}

// EndsPrimary reports whether s[i] is the last character of a value
// primary: a digit, ')' closing a call or group, ']' closing a field
// reference, or a duration unit right after a digit (7d, 30m).
func EndsPrimary(s string, i int) bool {
	c := s[i]
	if chars.IsDigit(c) || c == ')' || c == ']' {
		return true
	}
	return chars.IsDurationUnit(c) && i >= 1 && chars.IsDigit(s[i-1])
}

// BeginsPrimary reports whether s[i] starts a value primary: a digit, a
// signed digit, '(', '[', a quote, or a function call.
func BeginsPrimary(s string, i int) bool {
	c := s[i]
	switch {
	case chars.IsDigit(c) || c == '(' || c == '[' || c == '"':
		return true
	case chars.IsSign(c) && i+1 < len(s) && chars.IsDigit(s[i+1]):
		return true // a signed number: 10*-3
	}
	return chars.IsCallAt(s, i)
}
