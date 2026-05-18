package eval

import "github.com/heyllave/query/validate"

type options struct {
	maxLength     int
	maxDepth      int
	allowedOps    []validate.Op
	allowedFields []string
	funcs         FuncRegistry
	noBuiltins    bool
	customVal     validate.AstValidator
}

func defaultOpts() options {
	return options{
		maxLength: 256,
	}
}

// Option configures query compilation.
type Option func(*options)

// WithMaxLength sets the maximum query string length. 0 disables the limit.
func WithMaxLength(n int) Option {
	return func(o *options) { o.maxLength = n }
}

// WithMaxDepth limits the nesting depth of the expression tree.
// 0 means no limit.
func WithMaxDepth(n int) Option {
	return func(o *options) { o.maxDepth = n }
}

// WithAllowedOps restricts which operators are permitted in the query.
// If empty, all operators from the field config are allowed.
func WithAllowedOps(ops ...validate.Op) Option {
	return func(o *options) { o.allowedOps = ops }
}

// WithAllowedFields restricts which fields can appear in the query.
// If empty, all fields from the config are allowed.
func WithAllowedFields(fields ...string) Option {
	return func(o *options) { o.allowedFields = fields }
}

// WithFunctions registers custom functions available in query expressions.
// These are merged with the built-in functions (lower, upper, now, etc.).
func WithFunctions(funcs ...Func) Option {
	return func(o *options) {
		if o.funcs == nil {
			o.funcs = make(FuncRegistry)
		}
		for _, f := range funcs {
			o.funcs.Register(f)
		}
	}
}

// WithNoBuiltins disables the built-in functions (lower, upper, now, etc.).
// Only explicitly registered functions via [WithFunctions] will be available.
func WithNoBuiltins() Option {
	return func(o *options) { o.noBuiltins = true }
}

// WithCustomValidator installs a [validate.AstValidator] hook that extends
// validation with consumer-defined rules. See [validate.WithCustomValidator]
// for full semantics, including how [validate.AstValidator.GetFieldConfig]
// overrides the static field config.
func WithCustomValidator(cv validate.AstValidator) Option {
	return func(o *options) { o.customVal = cv }
}
