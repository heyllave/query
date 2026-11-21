// Package chars holds the character classes the lexer is built on. Every
// function is pure: it reads a byte, or a string at an index, and never
// keeps state, so each rule can be read and tested on its own.
package chars

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// IsDigit reports whether c is an ASCII digit.
func IsDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// At returns s[i], or 0 when i is past the end, so a look ahead never needs
// its own bounds check.
func At(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

// AllDigits reports whether s is one or more ASCII digits.
func AllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !IsDigit(s[i]) {
			return false
		}
	}
	return len(s) > 0
}

// IsIdentStart reports whether c may begin an identifier: an ASCII letter,
// '_', or any byte of a multi-byte UTF-8 sequence, so a non-ASCII word such
// as café or 日本 is one bare term rather than a run of unexpected bytes.
func IsIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c >= utf8.RuneSelf
}

// IsIdentChar reports whether c may continue an identifier. Hyphens and
// slashes are included so hyphenated names and label keys such as
// labels.example.com/team lex as one field segment.
func IsIdentChar(c byte) bool {
	return IsIdentStart(c) || IsDigit(c) || c == '-' || c == '/'
}

// IsDurationUnit reports whether c is one of the duration units m, h, d, w.
func IsDurationUnit(c byte) bool {
	return c == 'd' || c == 'h' || c == 'w' || c == 'm'
}

// IsArithOp reports whether c is one of the arithmetic operators + - * / %.
func IsArithOp(c byte) bool {
	return c == '+' || c == '-' || c == '*' || c == '/' || c == '%'
}

// IsSign reports whether c is a sign a number may start with.
func IsSign(c byte) bool {
	return c == '-' || c == '+'
}

// isASCIISpace reports whether c is one of the four ASCII spaces.
func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// SpaceWidth returns the byte width of the whitespace character at s[i], or
// 0 when there is none, past the end included. Unicode spaces count: a
// no-break space pasted between two words separates them like an ordinary
// one instead of gluing them into a single term.
func SpaceWidth(s string, i int) int {
	if i < 0 || i >= len(s) {
		return 0
	}
	c := s[i]
	if c < utf8.RuneSelf {
		if isASCIISpace(c) {
			return 1
		}
		return 0
	}
	if r, w := utf8.DecodeRuneInString(s[i:]); unicode.IsSpace(r) {
		return w
	}
	return 0
}

// SkipSpace returns the index of the first non-space character at or after i.
func SkipSpace(s string, i int) int {
	for i < len(s) {
		w := SpaceWidth(s, i)
		if w == 0 {
			break
		}
		i += w
	}
	return i
}

// IdentEnd returns the index just past the identifier characters starting at
// s[i]. A non-ASCII rune continues an identifier unless it is whitespace.
func IdentEnd(s string, i int) int {
	for i < len(s) {
		c := s[i]
		if c < utf8.RuneSelf {
			if !IsIdentChar(c) {
				break
			}
			i++
			continue
		}
		r, w := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) {
			break
		}
		i += w
	}
	return i
}

// PathEnd returns the index just past the dotted path of identifier
// segments starting at s[i]: labels.example.com/team. Dots and segments may
// come in any order, as a field name being typed can end in a dot.
func PathEnd(s string, i int) int {
	for i < len(s) {
		if s[i] == '.' {
			i++
			continue
		}
		end := IdentEnd(s, i)
		if end == i {
			break
		}
		i = end
	}
	return i
}

// IsCallAt reports whether an identifier starts at s[i] and is followed
// directly by '(', which makes it a function call.
func IsCallAt(s string, i int) bool {
	if i >= len(s) || !IsIdentStart(s[i]) {
		return false
	}
	end := IdentEnd(s, i)
	return end < len(s) && s[end] == '('
}

// QuotedEnd returns the index of the closing quote of the string whose
// opening quote is at s[open], skipping escaped characters, or len(s) when
// it never closes.
func QuotedEnd(s string, open int) int {
	j := open + 1
	for j < len(s) && s[j] != '"' {
		if s[j] == '\\' {
			j++
		}
		j++
	}
	return j
}

// SkipCall returns the index just past the ')' that closes the call body
// opening at s[open], skipping quoted strings inside it, or -1 when it never
// closes.
func SkipCall(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '"':
			i = QuotedEnd(s, i)
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// SkipFieldRef returns the index of the ']' that closes the field reference
// opening at s[open], skipping a quoted name inside it, or -1 when it never
// closes.
func SkipFieldRef(s string, open int) int {
	for i := open + 1; i < len(s); i++ {
		switch s[i] {
		case '"':
			i = QuotedEnd(s, i)
		case ']':
			return i
		}
	}
	return -1
}

// ReadQuoted decodes the double-quoted string whose opening quote is at
// s[open]. It returns the decoded text, the index just past the closing
// quote (or len(s)), and whether the string was closed. Escapes: \" \\ \n
// \t \r; any other \x is the literal x.
func ReadQuoted(s string, open int) (value string, end int, closed bool) {
	var buf strings.Builder
	i := open + 1
	for i < len(s) {
		c := s[i]
		switch {
		case c == '"':
			return buf.String(), i + 1, true
		case c == '\\' && i+1 < len(s):
			buf.WriteByte(unescape(s[i+1]))
			i += 2
		default:
			buf.WriteByte(c)
			i++
		}
	}
	return buf.String(), i, false
}

// unescape is the character a backslash before c stands for in a quoted
// string.
func unescape(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	}
	return c
}
