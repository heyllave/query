package parser

import (
	"errors"
	"time"

	"github.com/heyllave/query/parser/internal/literal"
	"github.com/heyllave/query/token"
)

// IsDayLiteral reports whether s is a date literal without a time part
// (YYYY-MM-DD), which denotes a whole day rather than an instant.
func IsDayLiteral(s string) bool { return literal.IsDate(s) }

// ParseDateLiteral parses a date (2026-01-01) or datetime literal
// (2026-01-01T10:00:00Z, 2026-01-01T10:00:00, 2026-01-01 10:00:00). A date
// without a time is midnight UTC; a datetime without a zone is UTC.
func ParseDateLiteral(s string) (time.Time, error) { return literal.ParseDate(s) }

// ParseDuration parses a duration literal like "1d", "4h", "30m", "2w".
// Go's time.ParseDuration does not support 'd' or 'w'.
func ParseDuration(s string) (time.Duration, error) {
	d, err := literal.ParseDuration(s)
	switch {
	case err == nil:
		return d, nil
	case errors.Is(err, literal.ErrDurationUnit):
		return 0, newError(CodeInvalidDuration, ErrInvalidDuration, token.Position{}, "invalid duration suffix in %q", s)
	case errors.Is(err, literal.ErrDurationRange):
		return 0, newError(CodeInvalidDuration, ErrInvalidDuration, token.Position{}, "duration %q is out of range", s)
	}
	return 0, newError(CodeInvalidDuration, ErrInvalidDuration, token.Position{}, "invalid duration %q", s)
}
