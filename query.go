package query

import (
	"github.com/heyllave/query/ast"
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
