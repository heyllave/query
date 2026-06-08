package eval

import (
	"errors"
	"fmt"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// ErrNoValue is returned by [ValueProgram.Eval] and [ValueProgram.EvalFunc]
// when the expression cannot resolve to a value: a referenced field is missing,
// a registered function returns an error, or arithmetic divides (or takes a
// modulo) by zero. In a predicate ([Program.Match]) these conditions silently
// fold to a false comparison; when the value *is* the result they surface here.
var ErrNoValue = errors.New("expression did not resolve to a value")

// ValueProgram is a compiled value expression ready for evaluation against
// data. Unlike [Program], which answers a boolean predicate, a ValueProgram
// computes and returns a typed value — boolean is one result domain of the
// query language, a value is another. It is safe for concurrent use.
type ValueProgram struct {
	source  string
	expr    ast.Expression
	funcs   FuncRegistry
	resolve valueResolver
}

// CompileValue parses, validates, and compiles a value expression into a
// [ValueProgram]. The query must be a value-producing expression — arithmetic,
// a function call, or a literal — not a boolean predicate; see
// [parser.ParseValue].
//
// The fields parameter declares the field names, types, and operators reachable
// through function arguments (value position has no bare field references). It
// may be nil for expressions that reference no fields. Options behave as for
// [Compile].
//
//	prog, _ := eval.CompileValue("now()-7d", nil)
//	v, _ := prog.Eval(nil) // time.Time, seven days ago
//
//	prog, _ := eval.CompileValue("upper(name)", []validate.FieldConfig{
//	    {Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
//	})
//	v, _ := prog.Eval(map[string]any{"name": "draft"}) // "DRAFT"
func CompileValue(q string, fields []validate.FieldConfig, opts ...Option) (*ValueProgram, error) {
	o := defaultOpts()
	for _, opt := range opts {
		opt(&o)
	}

	funcs := make(FuncRegistry)
	if !o.noBuiltins {
		for k, v := range BuiltinFunctions() {
			funcs[k] = v
		}
	}
	for k, v := range o.funcs {
		funcs[k] = v
	}

	activeFields := fields
	if len(o.allowedFields) > 0 {
		activeFields = filterFields(fields, o.allowedFields)
	}
	if len(o.allowedOps) > 0 {
		activeFields = restrictOps(activeFields, o.allowedOps)
	}

	expr, err := parser.ParseValue(q, o.maxLength)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	if o.maxDepth > 0 && ast.Depth(expr) > o.maxDepth {
		return nil, fmt.Errorf("query depth %d exceeds maximum of %d", ast.Depth(expr), o.maxDepth)
	}

	var vopts []validate.Option
	if o.customVal != nil {
		vopts = append(vopts, validate.WithCustomValidator(o.customVal))
	}
	v := validate.New(activeFields, vopts...)
	if err := v.Validate(expr); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	ve, ok := expr.(*ast.ValueExpr)
	if !ok {
		// ParseValue always returns an *ast.ValueExpr on success; this guards
		// the type assertion rather than asserting a parser invariant blindly.
		return nil, fmt.Errorf("expected value expression, got %T", expr)
	}

	return &ValueProgram{
		source:  q,
		expr:    expr,
		funcs:   funcs,
		resolve: compileValueResolver(&ve.Value, funcs),
	}, nil
}

// Eval computes the value against a map of field values. A nil or empty map is
// valid for expressions that reference no fields (now()-7d, 50000*1.1). It
// returns [ErrNoValue] if the expression cannot resolve.
//
// The result is the typed Go value: int64, float64, string, bool, time.Time, or
// time.Duration.
func (p *ValueProgram) Eval(data map[string]any) (any, error) {
	return p.EvalFunc(func(field string) (any, bool) {
		v, ok := data[field]
		return v, ok
	})
}

// EvalFunc computes the value using a custom field accessor. The accessor
// returns the value for a field and whether it exists. It returns [ErrNoValue]
// if the expression cannot resolve.
func (p *ValueProgram) EvalFunc(get func(field string) (any, bool)) (any, error) {
	if p.resolve == nil {
		return nil, ErrNoValue
	}
	v := p.resolve(get)
	if v == nil {
		return nil, ErrNoValue
	}
	return v.Any(), nil
}

// String returns the original query string.
func (p *ValueProgram) String() string { return p.source }

// AST returns the parsed expression tree.
func (p *ValueProgram) AST() ast.Expression { return p.expr }

// Stringify returns the AST serialized back to a query string.
func (p *ValueProgram) Stringify() string { return ast.String(p.expr) }
