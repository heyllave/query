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

	// Build function registry
	funcs := make(FuncRegistry)
	if !o.noBuiltins {
		for k, v := range BuiltinFunctions() {
			funcs[k] = v
		}
	}
	for k, v := range o.funcs {
		funcs[k] = v
	}

	// Restrict fields if specified
	activeFields := fields
	if len(o.allowedFields) > 0 {
		activeFields = filterFields(fields, o.allowedFields)
	}

	// Restrict operators if specified
	if len(o.allowedOps) > 0 {
		activeFields = restrictOps(activeFields, o.allowedOps)
	}

	// Parse
	expr, err := parser.Parse(q, o.maxLength)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Check depth
	if o.maxDepth > 0 && ast.Depth(expr) > o.maxDepth {
		return nil, fmt.Errorf("query depth %d exceeds maximum of %d", ast.Depth(expr), o.maxDepth)
	}

	// Validate (skip validation for function-call-only expressions)
	var vopts []validate.Option
	if o.customVal != nil {
		vopts = append(vopts, validate.WithCustomValidator(o.customVal))
	}
	v := validate.New(activeFields, vopts...)
	if err := v.Validate(expr); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
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

func filterFields(all []validate.FieldConfig, allowed []string) []validate.FieldConfig {
	set := make(map[string]bool, len(allowed))
	for _, f := range allowed {
		set[f] = true
	}
	var result []validate.FieldConfig
	for _, f := range all {
		if set[f.Name] {
			result = append(result, f)
		}
	}
	return result
}

func restrictOps(fields []validate.FieldConfig, allowed []validate.Op) []validate.FieldConfig {
	set := make(map[validate.Op]bool, len(allowed))
	for _, op := range allowed {
		set[op] = true
	}
	result := make([]validate.FieldConfig, len(fields))
	for i, f := range fields {
		result[i] = f
		var ops []validate.Op
		for _, op := range f.AllowedOps {
			if set[op] {
				ops = append(ops, op)
			}
		}
		result[i].AllowedOps = ops
	}
	return result
}
