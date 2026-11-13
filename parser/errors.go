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

// Code is a stable machine identifier for a parse failure.
//
// Kind classifies a failure broadly and Message describes it in English. A
// client that renders its own copy — a UI translating the failure, a linter
// grouping by cause — needs neither: it needs an identifier that survives a
// reworded message. Code is that identifier, and it never changes once shipped.
type Code string

// The parse failures the engine can report. One per distinguishable cause, so a
// client can map each to its own wording and its own remedy.
const (
	CodeQueryTooLong     Code = "queryTooLong"     // the query exceeds the caller's maximum length
	CodeUnexpectedChar   Code = "unexpectedChar"   // a character the grammar has no meaning for
	CodeUnclosedString   Code = "unclosedString"   // a quoted literal never closes
	CodeUnclosedParen    Code = "unclosedParen"    // a '(' never closes
	CodeUnclosedIn       Code = "unclosedIn"       // an IN list never closes
	CodeEmptyInList      Code = "emptyInList"      // IN () with nothing between the parens
	CodeUnclosedFieldRef Code = "unclosedFieldRef" // a bracketed field reference never closes
	CodeEmptyFieldRef    Code = "emptyFieldRef"    // a field reference with no name inside
	CodeExpectedField    Code = "expectedField"    // a field name was required here
	CodeExpectedValue    Code = "expectedValue"    // a value was required here
	CodeExpectedInList   Code = "expectedInList"   // IN was not followed by a parenthesised list
	CodeExpectedSelector Code = "expectedSelector" // '@' was not followed by a known selector
	CodeExpectedRange    Code = "expectedRange"    // a range was required here
	CodeUnexpected       Code = "unexpected"       // a token that cannot appear in this position
	CodeInvalidWildcard  Code = "invalidWildcard"  // a '*' pattern the grammar cannot express
	CodeInvalidDate      Code = "invalidDate"      // a malformed date literal
	CodeInvalidDuration  Code = "invalidDuration"  // a malformed duration literal
	CodeInvalidInteger   Code = "invalidInteger"   // a malformed integer literal
	CodeInvalidFloat     Code = "invalidFloat"     // a malformed float literal
	CodeUnclosedFuncArgs Code = "unclosedFuncArgs" // a function's argument list never closes
	CodeNestingTooDeep   Code = "nestingTooDeep"   // groups, negations or calls nest deeper than MaxNesting
)

// codes is every failure the parser can report, and the reason a client can
// switch on the set exhaustively: a code that nothing produces is a promise
// the engine does not keep, and one that exists without appearing here is
// invisible to the test that walks them.
var codes = [...]Code{
	CodeQueryTooLong, CodeUnexpectedChar, CodeUnclosedString, CodeUnclosedParen,
	CodeUnclosedIn, CodeEmptyInList, CodeUnclosedFieldRef, CodeEmptyFieldRef,
	CodeExpectedField, CodeExpectedValue, CodeExpectedInList, CodeExpectedSelector,
	CodeExpectedRange, CodeUnexpected, CodeInvalidWildcard, CodeInvalidDate,
	CodeInvalidDuration, CodeInvalidInteger, CodeInvalidFloat, CodeUnclosedFuncArgs,
	CodeNestingTooDeep,
}

// Codes returns every failure code the parser can report.
func Codes() []Code {
	out := make([]Code, len(codes))
	copy(out, codes[:])
	return out
}

// Error is a structured parse error with position info.
//
//nolint:revive // Error is the canonical name; package qualifier makes it clear (parser.Error)
type Error struct {
	Message  string
	Code     Code
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

func newError(code Code, kind ErrorKind, pos token.Position, format string, args ...any) *Error {
	return &Error{
		Message:  fmt.Sprintf(format, args...),
		Code:     code,
		Position: pos,
		Kind:     kind,
	}
}
