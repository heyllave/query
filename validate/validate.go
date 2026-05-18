package validate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// ErrorKind classifies validation errors.
type ErrorKind int

// Validation error kind constants.
const (
	ErrFieldNotFound      ErrorKind = iota // field not in config
	ErrOperatorNotAllowed                  // operator not permitted for field
	ErrTypeMismatch                        // value type incompatible with field type
	ErrCustomRule                          // error raised by a custom [AstValidator]
)

// Error is a structured validation error.
//
//nolint:revive // Error is the canonical name for this package
type Error struct {
	Message  string
	Position token.Position
	Kind     ErrorKind
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("position %d: %s", e.Position.Offset, e.Message)
}

// ErrorList is a collection of validation errors.
type ErrorList []*Error

// Error implements the error interface.
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

// Unwrap returns the underlying errors.
func (el ErrorList) Unwrap() []error {
	errs := make([]error, len(el))
	for i, e := range el {
		errs[i] = e
	}
	return errs
}

// Option configures a [Validator].
type Option func(*Validator)

// WithCustomValidator installs an [AstValidator] hook that extends validation
// with consumer-defined rules.
//
// When set, [AstValidator.GetFieldConfig] is consulted first for every field
// reference and overrides the static config: a result of (_, false) means the
// field is unknown, even if it is declared statically. This enables tenant-
// scoped field access control. Consumers that want to fall back to the static
// config should do so explicitly inside their implementation.
//
// [AstValidator.ValidateCustomRules] is invoked once on the root expression
// after all built-in checks have run. Errors returned are appended to the
// [ErrorList] alongside built-in errors; an [ErrorList] or *[Error] is merged
// as-is, other errors are wrapped with [ErrCustomRule].
func WithCustomValidator(cv AstValidator) Option {
	return func(v *Validator) { v.custom = cv }
}

// Validator validates a parsed AST against field configurations.
type Validator struct {
	fields map[string]FieldConfig
	nested map[string]FieldConfig
	custom AstValidator
	errors ErrorList
}

// New creates a validator with the given field configs and options.
func New(fields []FieldConfig, opts ...Option) *Validator {
	v := &Validator{
		fields: make(map[string]FieldConfig, len(fields)),
		nested: make(map[string]FieldConfig),
	}
	for _, f := range fields {
		v.fields[f.Name] = f
		if f.Nested {
			v.nested[f.Name] = f
		}
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Validate checks the AST against the field configurations.
// It collects all errors rather than stopping at the first one.
//
// When a custom [AstValidator] is installed via [WithCustomValidator], its
// [AstValidator.ValidateCustomRules] method is called once on the root
// expression after built-in checks complete. Returned errors are merged into
// the resulting [ErrorList].
func (v *Validator) Validate(expr ast.Expression) error {
	v.errors = nil
	v.validate(expr)
	if v.custom != nil && expr != nil {
		if err := v.custom.ValidateCustomRules(expr); err != nil {
			v.appendCustomErr(expr, err)
		}
	}
	if len(v.errors) == 0 {
		return nil
	}
	return v.errors
}

// appendCustomErr merges an error returned by [AstValidator.ValidateCustomRules]
// into the running error list. Structured errors ([Error], [ErrorList]) are
// preserved; any other error is wrapped as a single [ErrCustomRule] entry
// positioned at the root expression. Wrapping via [errors.As] is honored so
// consumers can wrap structured errors with context.
func (v *Validator) appendCustomErr(root ast.Expression, err error) {
	var list ErrorList
	if errors.As(err, &list) {
		v.errors = append(v.errors, list...)
		return
	}
	var single *Error
	if errors.As(err, &single) {
		v.errors = append(v.errors, single)
		return
	}
	// Fall back to wrapping unknown error shapes as a single entry at the root.
	// Note: errors.Join-style errors are treated as opaque — the combined
	// message is preserved but positions of individual entries are lost.
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		for _, sub := range joined.Unwrap() {
			v.appendCustomErr(root, sub)
		}
		return
	}
	v.errors = append(v.errors, &Error{
		Message:  err.Error(),
		Position: root.Pos(),
		Kind:     ErrCustomRule,
	})
}

func (v *Validator) validate(expr ast.Expression) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		v.validate(e.Left)
		v.validate(e.Right)
	case *ast.UnaryExpr:
		v.validate(e.Expr)
	case *ast.GroupExpr:
		v.validate(e.Expr)
	case *ast.SelectorExpr:
		v.validateSelector(e)
	case *ast.FuncCallExpr:
		v.validateFuncCallFields(e)
	case *ast.QualifierExpr:
		v.validateQualifier(e)
	case *ast.PresenceExpr:
		v.validatePresence(e)
	}
}

func (v *Validator) validateQualifier(q *ast.QualifierExpr) {
	// If the qualifier has a field transform function (e.g., lower(name)=john),
	// validate the field references inside the function args instead.
	if q.HasFieldFunc() {
		v.validateFuncCallFields(q.FieldFunc)
	}

	// Validate functions appearing in value position (e.g., created_at>=now()).
	// Their return type is opaque to the validator, so we only ensure the
	// field references inside the function are declared.
	v.validateValueFuncs(&q.Value)
	v.validateValueFuncs(q.EndValue)

	if q.HasFieldFunc() {
		return
	}

	fieldName := q.Field.String()
	cfg, ok := v.resolveField(fieldName)
	if !ok {
		v.addError(ErrFieldNotFound, q.Position, "unknown field %q", fieldName)
		return
	}
	op := tokenTypeToOp(q.Operator, q.Value.Wildcard)
	if !cfg.AllowsOp(op) {
		v.addError(ErrOperatorNotAllowed, q.Position,
			"operator %q is not allowed for field %q (type %s)", string(op), fieldName, cfg.Type)
		return
	}
	// Skip type-compatibility checks for function- and arithmetic-valued
	// operands — we can't know the return type at parse time. The eval
	// engine coerces at match time; mismatches there fall back to the
	// comparison's default-false path.
	if !isDynamicValue(q.Value) && !typeCompatible(cfg.Type, q.Value) {
		v.addError(ErrTypeMismatch, q.Position,
			"value type %s is not compatible with field %q (type %s)", q.Value.Type, fieldName, cfg.Type)
	}
	if q.EndValue != nil && !isDynamicValue(*q.EndValue) {
		if !typeCompatible(cfg.Type, *q.EndValue) {
			v.addError(ErrTypeMismatch, q.Position,
				"range end value type %s is not compatible with field %q (type %s)", q.EndValue.Type, fieldName, cfg.Type)
		}
	}
}

// isDynamicValue reports whether a Value is resolved at match time
// (function call or arithmetic) and therefore should not be type-checked
// against the field's declared type.
func isDynamicValue(v ast.Value) bool {
	return v.Type == ast.ValueFunc || v.Type == ast.ValueArith
}

// validateValueFuncs walks a value's function/arithmetic subtree and validates
// any embedded function-call field references.
func (v *Validator) validateValueFuncs(val *ast.Value) {
	if val == nil {
		return
	}
	if val.Func != nil {
		v.validateFuncCallFields(val.Func)
	}
	if val.Arith != nil {
		v.validateValueFuncs(val.Arith.Left)
		v.validateValueFuncs(val.Arith.Right)
	}
}

// validateFuncCallFields validates field references inside function arguments.
func (v *Validator) validateFuncCallFields(fc *ast.FuncCallExpr) {
	for _, arg := range fc.Args {
		if arg.Field != nil {
			fieldName := arg.Field.String()
			if _, ok := v.resolveField(fieldName); !ok {
				v.addError(ErrFieldNotFound, fc.Position, "unknown field %q in function %s()", fieldName, fc.Name)
			}
		}
		if arg.Call != nil {
			v.validateFuncCallFields(arg.Call)
		}
	}
}

// validateSelector checks that the selector's base field is declared and
// recurses into the inner expression. The base acts as a list reference, so
// we only require the field to exist — OpPresence is not required.
func (v *Validator) validateSelector(s *ast.SelectorExpr) {
	switch b := s.Base.(type) {
	case *ast.PresenceExpr:
		fieldName := b.Field.String()
		if _, ok := v.resolveField(fieldName); !ok {
			v.addError(ErrFieldNotFound, b.Position, "unknown field %q", fieldName)
		}
	case *ast.QualifierExpr:
		v.validateQualifier(b)
	default:
		v.validate(s.Base)
	}
	if s.Inner != nil {
		v.validate(s.Inner)
	}
}

func (v *Validator) validatePresence(p *ast.PresenceExpr) {
	fieldName := p.Field.String()
	cfg, ok := v.resolveField(fieldName)
	if !ok {
		v.addError(ErrFieldNotFound, p.Position, "unknown field %q", fieldName)
		return
	}
	if !cfg.AllowsOp(OpPresence) {
		v.addError(ErrOperatorNotAllowed, p.Position,
			"presence check is not allowed for field %q", fieldName)
	}
}

func (v *Validator) resolveField(name string) (FieldConfig, bool) {
	if v.custom != nil {
		if cfg, ok := v.custom.GetFieldConfig(name); ok {
			return cfg, true
		}
		// Nested fallthrough still honors the custom validator: look up the
		// top-level segment (e.g., "labels" for "labels.dev") through the hook
		// so tenants can allow/deny entire nested namespaces.
		if idx := strings.IndexByte(name, '.'); idx > 0 {
			if cfg, ok := v.custom.GetFieldConfig(name[:idx]); ok && cfg.Nested {
				return cfg, true
			}
		}
		return FieldConfig{}, false
	}
	if cfg, ok := v.fields[name]; ok {
		return cfg, true
	}
	if idx := strings.IndexByte(name, '.'); idx > 0 {
		if cfg, ok := v.nested[name[:idx]]; ok {
			return cfg, true
		}
	}
	return FieldConfig{}, false
}

func (v *Validator) addError(kind ErrorKind, pos token.Position, format string, args ...any) {
	v.errors = append(v.errors, &Error{
		Message:  fmt.Sprintf(format, args...),
		Position: pos,
		Kind:     kind,
	})
}

func tokenTypeToOp(tt token.Type, wildcard bool) Op {
	if wildcard {
		return OpWildcard
	}
	switch tt { //nolint:exhaustive // only comparison operators
	case token.Eq:
		return OpEq
	case token.Neq:
		return OpNeq
	case token.Gt:
		return OpGt
	case token.Gte:
		return OpGte
	case token.Lt:
		return OpLt
	case token.Lte:
		return OpLte
	case token.Range:
		return OpRange
	default:
		return OpEq
	}
}

func typeCompatible(fieldType FieldValueType, val ast.Value) bool {
	switch fieldType {
	case TypeText:
		return val.Type == ast.ValueString || val.Wildcard
	case TypeInteger:
		return val.Type == ast.ValueInteger
	case TypeDecimal:
		return val.Type == ast.ValueInteger || val.Type == ast.ValueFloat
	case TypeBoolean:
		return val.Type == ast.ValueBoolean
	case TypeDate, TypeDatetime:
		return val.Type == ast.ValueDate
	case TypeDuration:
		return val.Type == ast.ValueDuration
	default:
		return false
	}
}
