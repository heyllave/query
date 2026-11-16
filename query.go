package query

import (
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// DefaultMaxLength is the default maximum query string length in characters.
const DefaultMaxLength = 256

// options holds configuration for parsing.
type options struct {
	maxLength    int
	explicitAnd  bool
	strictValues bool
}

// Option configures parsing behavior.
type Option func(*options)

// WithMaxLength sets the maximum allowed query string length in characters.
// A value of 0 disables length checking; nesting stays bounded by
// [parser.MaxNesting] either way.
func WithMaxLength(n int) Option {
	return func(o *options) {
		o.maxLength = n
	}
}

// WithExplicitAnd requires AND or OR between conditions, refusing the
// implicit AND a space otherwise stands for. `state=draft total>5` and
// `labels[team]=x` become errors with the code missingAnd. See
// [parser.Config].
func WithExplicitAnd() Option {
	return func(o *options) {
		o.explicitAnd = true
	}
}

// WithStrictValues refuses `=`, `<` and `>` inside an unquoted value, so
// `a=1+b=2` is an error instead of a comparison with the text "1+b=2". A
// value that holds one of those characters is written in quotes. See
// [parser.Config].
func WithStrictValues() Option {
	return func(o *options) {
		o.strictValues = true
	}
}

// WithStrict turns on every strict option: [WithExplicitAnd] and
// [WithStrictValues]. Strict options only refuse queries, so a query accepted
// under WithStrict means the same under the default grammar. It suits a host
// whose queries come from scripts, saved links and other programs, such as
// an API's filter parameter.
func WithStrict() Option {
	return func(o *options) {
		o.explicitAnd = true
		o.strictValues = true
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
	return parser.ParseWith(q, parser.Config{
		MaxLength:    o.maxLength,
		ExplicitAnd:  o.explicitAnd,
		StrictValues: o.strictValues,
	})
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
