package eval

import (
	"fmt"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// Compile parses, validates, and compiles a query into an executable [Program].
//
// The fields parameter defines the allowed field names, types, and operators.
// Options can further restrict which fields and operators are permitted,
// register custom functions, and set depth limits.
func Compile(q string, fields []validate.FieldConfig, opts ...Option) (*Program, error) {
	o := defaultOpts()
	for _, opt := range opts {
		opt(&o)
	}

	funcs := o.registry()

	// Parse
	expr, err := parser.Parse(q, o.maxLength)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	if err := o.check(expr, fields, funcs); err != nil {
		return nil, err
	}

	// Compile the matcher
	m := compileMatcher(expr, funcs)

	return &Program{
		source:  q,
		expr:    expr,
		fields:  ast.Fields(expr),
		funcs:   funcs,
		matcher: m,
	}, nil
}

// registry assembles the function registry: the builtins unless disabled,
// overlaid with the functions registered through WithFunctions.
func (o *options) registry() FuncRegistry {
	funcs := make(FuncRegistry)
	if !o.noBuiltins {
		for k, v := range BuiltinFunctions() {
			funcs[k] = v
		}
	}
	for k, v := range o.funcs {
		funcs[k] = v
	}
	return funcs
}

// check runs the compile-time checks shared by Compile and CompileValue: the
// depth limit, validation against the field configs (with the allowed-field
// and allowed-operator restrictions enforced inside the validator so they
// also bind a custom validator), and function existence and arity.
func (o *options) check(expr ast.Expression, fields []validate.FieldConfig, funcs FuncRegistry) error {
	if o.maxDepth > 0 && ast.Depth(expr) > o.maxDepth {
		return fmt.Errorf("query depth %d exceeds maximum of %d", ast.Depth(expr), o.maxDepth)
	}

	var vopts []validate.Option
	if o.customVal != nil {
		vopts = append(vopts, validate.WithCustomValidator(o.customVal))
	}
	if len(o.allowedFields) > 0 {
		vopts = append(vopts, validate.WithAllowedFields(o.allowedFields...))
	}
	if len(o.allowedOps) > 0 {
		vopts = append(vopts, validate.WithAllowedOps(o.allowedOps...))
	}
	v := validate.New(fields, vopts...)
	if err := v.Validate(expr); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	if err := checkCalls(expr, funcs); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	return nil
}
