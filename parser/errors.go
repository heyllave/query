package parser

import (
	"errors"
	"fmt"
	"strings"

	"github.com/heyllave/query/token"
)

// ErrorKind classifies the type of parse error.
type ErrorKind int

// Error kind constants.
const (
	ErrSyntax          ErrorKind = iota // general syntax error
	ErrUnexpectedToken                  // unexpected token encountered
	ErrUnexpectedEOF                    // premature end of input
	ErrInvalidValue                     // malformed value literal
	ErrQueryTooLong                     // query exceeds max length
	ErrInvalidWildcard                  // unsupported wildcard pattern
	ErrInvalidDate                      // malformed date literal
	ErrInvalidDuration                  // malformed duration literal
)

var kindNames = [...]string{
	ErrSyntax:          "syntax error",
	ErrUnexpectedToken: "unexpected token",
	ErrUnexpectedEOF:   "unexpected end of input",
	ErrInvalidValue:    "invalid value",
	ErrQueryTooLong:    "query too long",
	ErrInvalidWildcard: "invalid wildcard",
	ErrInvalidDate:     "invalid date",
	ErrInvalidDuration: "invalid duration",
}

// String returns the human-readable name of the error kind.
func (k ErrorKind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("ErrorKind(%d)", k)
}

// Error is a structured parse error with position info.
//
//nolint:revive // Error is the canonical name; package qualifier makes it clear (parser.Error)
type Error struct {
	Message  string
	Position token.Position
	Kind     ErrorKind
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("position %d: %s", e.Position.Offset, e.Message)
}

// ErrorList is a collection of parse errors.
type ErrorList []*Error

// Error implements the error interface, joining all error messages.
func (el ErrorList) Error() string {
	switch len(el) {
	case 0:
		return "no errors"
	case 1:
		return el[0].Error()
	default:
		msgs := make([]string, len(el))
		for i, e := range el {
			msgs[i] = e.Error()
		}
		return strings.Join(msgs, "; ")
	}
}

// Unwrap returns the underlying errors for errors.Is/As compatibility.
func (el ErrorList) Unwrap() []error {
	errs := make([]error, len(el))
	for i, e := range el {
		errs[i] = e
	}
	return errs
}

func (el *ErrorList) add(err *Error) {
	*el = append(*el, err)
}

func (el ErrorList) errOrNil() error {
	if len(el) == 0 {
		return nil
	}
	return el
}

// IsParseError reports whether err (or any error in its chain) is a *Error.
func IsParseError(err error) bool {
	var pe *Error
	return errors.As(err, &pe)
}

// Errors extracts all *Error values from err.
func Errors(err error) []*Error {
	var el ErrorList
	if errors.As(err, &el) {
		return []*Error(el)
	}
	var pe *Error
	if errors.As(err, &pe) {
		return []*Error{pe}
	}
	return nil
}

func newError(kind ErrorKind, pos token.Position, format string, args ...any) *Error {
	return &Error{
		Message:  fmt.Sprintf(format, args...),
		Position: pos,
		Kind:     kind,
	}
}
