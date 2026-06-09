package query

import (
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// DefaultMaxLength is the default maximum query string length in bytes.
const DefaultMaxLength = 256

// options holds configuration for parsing.
type options struct {
	maxLength int
}

// Option configures parsing behavior.
type Option func(*options)

// WithMaxLength sets the maximum allowed query string length.
// A value of 0 disables length checking.
func WithMaxLength(n int) Option {
	return func(o *options) {
		o.maxLength = n
	}
}

func defaultOptions() options {
	return options{maxLength: DefaultMaxLength}
}

// Parse parses a query string into an AST expression.
func Parse(q string, opts ...Option) (ast.Expression, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return parser.Parse(q, o.maxLength)
}

// Validate validates an AST against field configurations.
func Validate(expr ast.Expression, fields []validate.FieldConfig) error {
	v := validate.New(fields)
	return v.Validate(expr)
}

// ParseAndValidate parses a query string and validates it against field configs.
func ParseAndValidate(q string, fields []validate.FieldConfig, opts ...Option) (ast.Expression, error) {
	expr, err := Parse(q, opts...)
	if err != nil {
		return nil, err
	}
	if err := Validate(expr, fields); err != nil {
		return nil, err
	}
	return expr, nil
}

// Match compiles a boolean predicate query and evaluates it against a single
// record in one call. It is a convenience for one-off checks; to evaluate the
// same query against many records, compile once with [github.com/heyllave/query/eval.Compile]
// and reuse the resulting program.
func Match(q string, fields []validate.FieldConfig, record map[string]any) (bool, error) {
	prog, err := eval.Compile(q, fields)
	if err != nil {
		return false, err
	}
	return prog.Match(record), nil
}

// Eval compiles a value expression and evaluates it against a single record in
// one call, returning the computed value (see
// [github.com/heyllave/query/eval.CompileValue]). For repeated evaluation,
// compile once with eval.CompileValue and reuse the program. It returns
// [github.com/heyllave/query/eval.ErrNoValue] when the expression cannot
// resolve (e.g. a missing field or division by zero).
func Eval(q string, fields []validate.FieldConfig, record map[string]any) (any, error) {
	prog, err := eval.CompileValue(q, fields)
	if err != nil {
		return nil, err
	}
	return prog.Eval(record)
}
