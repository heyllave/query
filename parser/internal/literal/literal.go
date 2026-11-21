// Package literal decides the shape of a whole literal: a date, a date with
// a time, a duration, a number, a wildcard pattern. It knows nothing of the
// query around the literal, so each rule here is the whole rule.
package literal

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/heyllave/query/parser/internal/chars"
)

// IsDate reports whether s is exactly YYYY-MM-DD in digits. Whether that day
// exists is ParseDate's question.
func IsDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i == 4 || i == 7 {
			if s[i] != '-' {
				return false
			}
		} else if !chars.IsDigit(s[i]) {
			return false
		}
	}
	return true
}

// IsDatePrefix reports whether s is the part of a date that comes before a
// hyphen: the year (YYYY) or the year and month (YYYY-MM).
func IsDatePrefix(s string) bool {
	switch len(s) {
	case 4:
		return chars.AllDigits(s)
	case 7:
		return s[4] == '-' && chars.AllDigits(s[:4]) && chars.AllDigits(s[5:])
	}
	return false
}

// IsDatetime reports whether s has the shape of an RFC 3339 timestamp
// (YYYY-MM-DDTHH:MM:SS with an optional fraction and zone) or the same with
// a space separator. ParseDate decides whether it is a valid instant.
func IsDatetime(s string) bool {
	if len(s) < 19 || !IsDate(s[:10]) {
		return false
	}
	if s[10] != 'T' && s[10] != 't' && s[10] != ' ' {
		return false
	}
	for i := 11; i < len(s); i++ {
		if !isTimeChar(s[i]) {
			return false
		}
	}
	return true
}

func isTimeChar(c byte) bool {
	return chars.IsDigit(c) || c == ':' || c == '.' || c == '+' || c == '-' || c == 'Z' || c == 'z'
}

// DatetimeEnd returns the index just past a date with a time that starts at
// s[i], written YYYY-MM-DDTHH:MM:SS with an optional fraction and an
// optional zone (Z or an offset such as +02:00), or -1 when none starts
// there. It lets a datetime be one operand of arithmetic. Only the T form
// counts: a space would end the value.
func DatetimeEnd(s string, i int) int {
	t := s[i:]
	if len(t) < 19 || !IsDate(t[:10]) || (t[10] != 'T' && t[10] != 't') {
		return -1
	}
	if !isClock(t, 11) {
		return -1
	}
	n := fractionEnd(t, 19)
	return i + zoneEnd(t, n)
}

// isClock reports whether HH:MM:SS starts at s[i].
func isClock(s string, i int) bool {
	return twoDigits(s, i) && s[i+2] == ':' && twoDigits(s, i+3) && s[i+5] == ':' && twoDigits(s, i+6)
}

// fractionEnd returns the index past an optional .digits at s[n].
func fractionEnd(s string, n int) int {
	if n+1 < len(s) && s[n] == '.' && chars.IsDigit(s[n+1]) {
		for n++; n < len(s) && chars.IsDigit(s[n]); n++ {
		}
	}
	return n
}

// zoneEnd returns the index past an optional Z or ±HH:MM at s[n].
func zoneEnd(s string, n int) int {
	switch {
	case n < len(s) && (s[n] == 'Z' || s[n] == 'z'):
		return n + 1
	case n+5 < len(s) && chars.IsSign(s[n]) && twoDigits(s, n+1) && s[n+3] == ':' && twoDigits(s, n+4):
		return n + 6
	}
	return n
}

func twoDigits(s string, i int) bool {
	return i+1 < len(s) && chars.IsDigit(s[i]) && chars.IsDigit(s[i+1])
}

// dateLayouts are the forms ParseDate accepts, from the most to the least
// specific.
var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// ParseDate parses a date (2026-01-01) or a date with a time
// (2026-01-01T10:00:00Z, 2026-01-01T10:00:00, 2026-01-01 10:00:00). A date
// without a time is midnight UTC; a time without a zone is UTC.
func ParseDate(s string) (time.Time, error) {
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

// IsBoolean reports whether s is true or false, which are booleans only in
// lower case.
func IsBoolean(s string) bool {
	return s == "true" || s == "false"
}

// IsDuration reports whether s is digits followed by one unit.
func IsDuration(s string) bool {
	return len(s) >= 2 && chars.IsDurationUnit(s[len(s)-1]) && chars.AllDigits(s[:len(s)-1])
}

// The reasons ParseDuration refuses a duration.
var (
	ErrDurationSyntax = errors.New("invalid duration")
	ErrDurationUnit   = errors.New("invalid duration suffix")
	ErrDurationRange  = errors.New("duration out of range")
)

// durationUnits maps a unit letter to its length.
var durationUnits = map[byte]time.Duration{
	'm': time.Minute,
	'h': time.Hour,
	'd': 24 * time.Hour,
	'w': 7 * 24 * time.Hour,
}

// ParseDuration parses a duration such as 1d, 4h, 30m or 2w. Go's
// time.ParseDuration has no d or w. A count too large for time.Duration is
// ErrDurationRange, never a wrapped value.
func ParseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, ErrDurationSyntax
	}
	unit, ok := durationUnits[s[len(s)-1]]
	if !ok {
		return 0, ErrDurationUnit
	}
	limit := int64(math.MaxInt64 / unit)
	var n int64
	for _, r := range s[:len(s)-1] {
		if r < '0' || r > '9' {
			return 0, ErrDurationSyntax
		}
		n = n*10 + int64(r-'0')
		if n > limit {
			return 0, ErrDurationRange
		}
	}
	return time.Duration(n) * unit, nil
}

// IsInteger reports whether s is digits with an optional sign.
func IsInteger(s string) bool {
	return chars.AllDigits(unsigned(s))
}

// IsFloat reports whether s is digits with exactly one dot and an optional
// sign. Either side of the dot may be empty (.5 and 5.), as the lexer has
// always read them.
func IsFloat(s string) bool {
	if s == "" || unsigned(s) == "" {
		return false
	}
	whole, frac, ok := strings.Cut(unsigned(s), ".")
	if !ok || strings.Contains(frac, ".") {
		return false
	}
	return allDigitsOrEmpty(whole) && allDigitsOrEmpty(frac)
}

// unsigned is s without one leading sign.
func unsigned(s string) string {
	if s != "" && chars.IsSign(s[0]) {
		return s[1:]
	}
	return s
}

func allDigitsOrEmpty(s string) bool {
	return s == "" || chars.AllDigits(s)
}

// IsValidWildcard reports whether s is a prefix (foo*), suffix (*foo) or
// contains (*foo*) pattern. An escaped star (\*) is a literal and does not
// count.
func IsValidWildcard(s string) bool {
	stars, first, last := countStars(s)
	switch stars {
	case 0:
		return true
	case 1:
		return first || last
	case 2:
		return first && last
	}
	return false
}

// countStars counts the unescaped stars of s and reports whether one is the
// first character and whether one is the last.
func countStars(s string) (stars int, first, last bool) {
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
	return stars, first, last
}

// EncodePattern writes a decoded wildcard value in pattern form: a
// backslash becomes \\, a literal star (one escaped in the source, at the
// given byte offsets of value) becomes \*, and a bare * stays a wildcard.
// ast.DecodePattern reverses it, so a literal backslash is never mistaken
// for the escape of a star.
func EncodePattern(value string, literalStars []int) string {
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
