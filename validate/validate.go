package validate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
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

// Code is the stable machine identifier for a kind, the counterpart of the
// parser's failure codes: clients render their own wording from it.
func (k ErrorKind) Code() string {
	switch k {
	case ErrFieldNotFound:
		return "fieldNotFound"
	case ErrOperatorNotAllowed:
		return "operatorNotAllowed"
	case ErrTypeMismatch:
		return "typeMismatch"
	default:
		return "customRule"
	}
}

// Error is a structured validation error.
//
//nolint:revive // Error is the canonical name for this package
type Error struct {
	Message  string
	Position token.Position
	Kind     ErrorKind
	// Field is the dotted path the failure is about, when it is about one.
	Field string
	// Op is the refused operator for [ErrOperatorNotAllowed] ("?" for a
	// presence check).
	Op string
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

// WithAllowedFields restricts queries to the named fields. The restriction is
// enforced after field resolution, so it also applies to fields a custom
// [AstValidator] resolves, and to fields referenced inside function arguments
// and value-position field references.
func WithAllowedFields(fields ...string) Option {
	return func(v *Validator) {
		v.allowedFields = make(map[string]bool, len(fields))
		for _, f := range fields {
			v.allowedFields[f] = true
		}
	}
}

// WithAllowedOps restricts every comparison in the query to the given
// operators, on top of each field's own AllowedOps. Unlike the per-field list
// it also governs comparisons whose left side is a function (lower(name)=x),
// so it is the option to use for sandboxing.
func WithAllowedOps(ops ...Op) Option {
	return func(v *Validator) {
		v.allowedOps = make(map[Op]bool, len(ops))
		for _, op := range ops {
			v.allowedOps[op] = true
		}
	}
}

// Validator validates a parsed AST against field configurations. A Validator
// is safe for concurrent use; each call to [Validator.Validate] collects its
// errors privately.
type Validator struct {
	fields        map[string]FieldConfig
	nested        map[string]FieldConfig
	custom        AstValidator
	allowedFields map[string]bool
	allowedOps    map[Op]bool
	errors        ErrorList
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
//
// Validation also normalizes literals to the declared field type where the
// lexer's guess was narrower than the field: `zip=90210` on a text field keeps
// the integer-looking literal as the string "90210", and a quoted RFC 3339
// string on a date field becomes a date value. The expression is updated in
// place so evaluation compares with the field's type.
func (v *Validator) Validate(expr ast.Expression) error {
	// Work on a copy so concurrent Validate calls do not share the error list.
	w := *v
	w.errors = nil
	w.validate(expr)
	if w.custom != nil && expr != nil {
		if err := w.custom.ValidateCustomRules(expr); err != nil {
			w.appendCustomErr(expr, err)
		}
	}
	if len(w.errors) == 0 {
		return nil
	}
	return w.errors
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
	case *ast.ValueExpr:
		// A value-domain root has no declared result type to check against —
		// its type is resolved at match time, like any function/arithmetic
		// value (see isDynamicValue). We only ensure embedded function field
		// references are declared.
		v.validateValueFuncs(&e.Value, e.Position)
	}
}

func (v *Validator) validateQualifier(q *ast.QualifierExpr) {
	// Validate functions appearing in value position (e.g., created_at>=now()).
	// Their return type is opaque to the validator, so we only ensure the
	// field references inside the function are declared.
	v.validateValueFuncs(&q.Value, q.Position)
	v.validateValueFuncs(q.EndValue, q.Position)

	op := tokenTypeToOp(q.Operator, q.Value.Wildcard)

	// A function-wrapped qualifier (lower(name)=john) compares the function's
	// result, whose type is unknown here, so the value is not type-checked.
	// The operator is still policed: against the global allowed set, and
	// against each argument field's own operators where that is meaningful.
	if q.HasFieldFunc() {
		fields := v.validateFuncCallFields(q.FieldFunc)
		if !v.opGloballyAllowed(op) {
			v.addError(ErrOperatorNotAllowed, q.Position, "", string(op),
				"operator %q is not allowed", string(op))
			return
		}
		for _, f := range fields {
			if !allowsOpThroughFunc(f.cfg, op) {
				v.addError(ErrOperatorNotAllowed, q.Position, f.name, string(op),
					"operator %q is not allowed for field %q (type %s), including through %s()",
					string(op), f.name, f.cfg.Type, q.FieldFunc.Name)
			}
		}
		return
	}

	fieldName := q.Field.String()
	cfg, ok := v.resolveField(fieldName)
	if !ok {
		v.addError(ErrFieldNotFound, q.Position, fieldName, "", "unknown field %q", fieldName)
		return
	}
	if !v.opGloballyAllowed(op) || !cfg.AllowsOp(op) {
		v.addError(ErrOperatorNotAllowed, q.Position, fieldName, string(op),
			"operator %q is not allowed for field %q (type %s)", string(op), fieldName, cfg.Type)
		return
	}
	v.checkValue(cfg, &q.Value, q.Position, fieldName, "value")
	if q.EndValue != nil {
		v.checkValue(cfg, q.EndValue, q.Position, fieldName, "range end value")
	}
}

// checkValue type-checks one literal operand against the field, coercing it
// to the field's type first when the lexer guessed a narrower one (see
// coerceLiteral). Function- and field-valued operands are resolved at match
// time and skipped; arithmetic is checked for fields it can never produce.
func (v *Validator) checkValue(cfg FieldConfig, val *ast.Value, pos token.Position, fieldName, what string) {
	coerceLiteral(cfg.Type, val)
	if isDynamicValue(*val) {
		if val.Type == ast.ValueArith && (cfg.Type == TypeText || cfg.Type == TypeBoolean) {
			v.addError(ErrTypeMismatch, pos, fieldName, "",
				"%s is arithmetic (%s), which cannot produce a %s for field %q; quote it to compare as text",
				what, val.Raw, cfg.Type, fieldName)
		}
		return
	}
	if !typeCompatible(cfg.Type, *val) {
		v.addError(ErrTypeMismatch, pos, fieldName, "",
			"%s type %s is not compatible with field %q (type %s)", what, val.Type, fieldName, cfg.Type)
	}
}

// opGloballyAllowed reports whether op passes the WithAllowedOps restriction
// (always true when none is configured).
func (v *Validator) opGloballyAllowed(op Op) bool {
	return v.allowedOps == nil || v.allowedOps[op]
}

// allowsOpThroughFunc reports whether op may be applied to the result of a
// function over field cfg. Equality, inequality, wildcard and presence follow
// the field's own list: a field that forbids wildcards forbids
// trim(field)=a* too. Ordering and range operators follow the field's list
// when the field is ordered (numeric, date, duration); on a text or boolean
// field a function is the only way to obtain something orderable
// (len(name)>5), so those operators are permitted there.
func allowsOpThroughFunc(cfg FieldConfig, op Op) bool {
	switch op {
	case OpEq, OpNeq, OpWildcard, OpPresence:
		return cfg.AllowsOp(op)
	case OpGt, OpGte, OpLt, OpLte, OpRange:
		if cfg.Type == TypeText || cfg.Type == TypeBoolean {
			return true
		}
		return cfg.AllowsOp(op)
	default:
		return cfg.AllowsOp(op)
	}
}

// isDynamicValue reports whether a Value is resolved at match time (function
// call, arithmetic, or a field reference) and therefore should not be
// type-checked against the field's declared type.
func isDynamicValue(v ast.Value) bool {
	return v.Type == ast.ValueFunc || v.Type == ast.ValueArith || v.Type == ast.ValueFieldRef
}

// coerceLiteral rewrites a literal to the field's type when the lexer's guess
// was narrower: any scalar literal on a text field is the text it was written
// as (zip=90210 compares the string "90210"), and a quoted string on a date
// or datetime field that parses as a date literal becomes that date.
func coerceLiteral(ft FieldValueType, val *ast.Value) {
	switch ft {
	case TypeText:
		switch val.Type { //nolint:exhaustive // only scalar literals coerce
		case ast.ValueInteger, ast.ValueFloat, ast.ValueBoolean, ast.ValueDate, ast.ValueDuration:
			*val = ast.Value{Type: ast.ValueString, Raw: val.Raw, Str: val.Raw}
		}
	case TypeDate, TypeDatetime:
		if val.Type == ast.ValueString && !val.Wildcard {
			if t, err := parser.ParseDateLiteral(val.Str); err == nil {
				*val = ast.Value{Type: ast.ValueDate, Raw: val.Raw, Date: t, Quoted: val.Quoted}
			}
		}
	case TypeInteger, TypeDecimal, TypeBoolean, TypeDuration:
		// No coercion: a text literal on these fields is a genuine mismatch.
	}
}

// validateValueFuncs walks a value's function/arithmetic subtree and validates
// any embedded field references — both function-call field arguments and
// bracketed field-ref operands ([cpu_limit]) — against the field config. pos is
// the enclosing expression's position, used for any error (Value carries none).
func (v *Validator) validateValueFuncs(val *ast.Value, pos token.Position) {
	if val == nil {
		return
	}
	if val.Type == ast.ValueFieldRef {
		fieldName := val.Field.String()
		if _, ok := v.resolveField(fieldName); !ok {
			v.addError(ErrFieldNotFound, pos, fieldName, "", "unknown field %q", fieldName)
		}
	}
	if val.Func != nil {
		v.validateFuncCallFields(val.Func)
	}
	if val.Arith != nil {
		v.validateValueFuncs(val.Arith.Left, pos)
		v.validateValueFuncs(val.Arith.Right, pos)
	}
}

// resolvedField pairs a field name with its configuration.
type resolvedField struct {
	name string
	cfg  FieldConfig
}

// validateFuncCallFields validates field references inside function
// arguments, recursing into nested calls and value-position operands, and
// returns the fields that resolved.
func (v *Validator) validateFuncCallFields(fc *ast.FuncCallExpr) []resolvedField {
	var fields []resolvedField
	for _, arg := range fc.Args {
		switch {
		case arg.Field != nil:
			fieldName := arg.Field.String()
			cfg, ok := v.resolveField(fieldName)
			if !ok {
				v.addError(ErrFieldNotFound, fc.Position, fieldName, "", "unknown field %q in function %s()", fieldName, fc.Name)
				continue
			}
			fields = append(fields, resolvedField{fieldName, cfg})
		case arg.Call != nil:
			fields = append(fields, v.validateFuncCallFields(arg.Call)...)
		case arg.Value != nil:
			v.validateValueFuncs(arg.Value, fc.Position)
		}
	}
	return fields
}

// validateSelector checks that the selector's base field is declared and
// recurses into the inner expression. The base acts as a list reference, so
// we only require the field to exist — OpPresence is not required.
func (v *Validator) validateSelector(s *ast.SelectorExpr) {
	switch b := s.Base.(type) {
	case *ast.PresenceExpr:
		fieldName := b.Field.String()
		if _, ok := v.resolveField(fieldName); !ok {
			v.addError(ErrFieldNotFound, b.Position, fieldName, "", "unknown field %q", fieldName)
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
		v.addError(ErrFieldNotFound, p.Position, fieldName, "", "unknown field %q", fieldName)
		return
	}
	if !v.opGloballyAllowed(OpPresence) || !cfg.AllowsOp(OpPresence) {
		v.addError(ErrOperatorNotAllowed, p.Position, fieldName, string(OpPresence),
			"presence check is not allowed for field %q", fieldName)
	}
}

func (v *Validator) resolveField(name string) (FieldConfig, bool) {
	cfg, ok := v.lookupField(name)
	if !ok {
		return FieldConfig{}, false
	}
	// WithAllowedFields is a restriction over whatever source resolved the
	// field, so it is applied here rather than by filtering the static list.
	// A nested path is allowed when its top-level segment is.
	if v.allowedFields != nil && !v.allowedFields[name] {
		idx := strings.IndexByte(name, '.')
		if idx <= 0 || !cfg.Nested || !v.allowedFields[name[:idx]] {
			return FieldConfig{}, false
		}
	}
	return cfg, true
}

func (v *Validator) lookupField(name string) (FieldConfig, bool) {
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

func (v *Validator) addError(kind ErrorKind, pos token.Position, field, op, format string, args ...any) {
	v.errors = append(v.errors, &Error{
		Message:  fmt.Sprintf(format, args...),
		Position: pos,
		Kind:     kind,
		Field:    field,
		Op:       op,
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
