package validate

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/internal/hintwords"
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
	ErrFunctionNotAllowed                  // function outside [WithAllowedFunctions]
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
	case ErrFunctionNotAllowed:
		return "functionNotAllowed"
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
	// Hint says what the writer most likely meant and how to write it, when
	// the validator can tell: `did you mean "total"?` for totl>5. It is
	// English prose for a person, may be empty, and may be reworded between
	// versions; branch on Kind, not on Hint. It names only fields the
	// validator would accept, so it reveals nothing a restriction hides.
	Hint string
}

// Error implements the error interface. The hint, when there is one, follows
// the message in parentheses.
func (e *Error) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("position %d: %s (%s)", e.Position.Offset, e.Message, e.Hint)
	}
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

// WithAllowedFunctions restricts queries to the named functions; called with
// no names it refuses every function call. Without this option the validator
// accepts any function name, because which functions exist is up to whoever
// evaluates or translates the tree. A consumer that turns the tree into SQL or
// another language should list exactly the functions it can translate, so an
// unknown call is a validation error with a position rather than something its
// own code generator has to catch.
func WithAllowedFunctions(names ...string) Option {
	return func(v *Validator) {
		v.allowedFuncs = make(map[string]bool, len(names))
		for _, name := range names {
			v.allowedFuncs[name] = true
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
	allowedFuncs  map[string]bool
	// hasLists is set when the schema declares a list field. A schema that
	// declares one has declared them all, so a selector on any other field
	// is then an error.
	hasLists bool
	errors   ErrorList
	// missingTest is set for one Validate call when the query holds IS or
	// NULL as a stray word, so every bare word of `name IS NULL` gets the
	// same advice.
	missingTest bool
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
		if f.Type == TypeList {
			v.hasLists = true
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
	w.missingTest = w.hasMissingTest(expr)
	w.validate(expr)
	if w.allowedFuncs != nil {
		for _, fc := range ast.Calls(expr) {
			if !w.allowedFuncs[fc.Name] {
				w.addHinted(func() string { return w.functionHint(fc.Name) },
					ErrFunctionNotAllowed, fc.Position, "", "", "function %q is not allowed", fc.Name)
			}
		}
	}
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
		v.validatePredicateCall(e)
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
	op := tokenTypeToOp(q.Operator, q.Value.Wildcard)

	// Fields on the value side (name=[other], total>len(name)) are read by the
	// same comparison, so they are checked for existence and for the operator
	// as seen from their side.
	valueOp := mirrorOp(op)
	v.checkOperandFields(v.validateValueFuncs(&q.Value, q.Position), valueOp, q.Position)
	v.checkOperandFields(v.validateValueFuncs(q.EndValue, q.Position), valueOp, q.Position)

	// A function-wrapped qualifier (lower(name)=john) compares the function's
	// result, whose type is unknown here, so the value is not type-checked.
	// The operator is still policed: against the global allowed set, and
	// against each argument field's own operators.
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

// checkOperandFields polices the fields a comparison reads on its value side,
// directly ([other]) or through a function. They follow allowsOpThroughFunc,
// like a function on the left side.
func (v *Validator) checkOperandFields(fields []resolvedField, op Op, pos token.Position) {
	for _, f := range fields {
		if !allowsOpThroughFunc(f.cfg, op) {
			v.addError(ErrOperatorNotAllowed, pos, f.name, string(op),
				"operator %q is not allowed for field %q (type %s), which the compared value reads",
				string(op), f.name, f.cfg.Type)
		}
	}
}

// validatePredicateCall validates a function used as a predicate
// (contains(tags, "urgent")). No operator is written, but the function reads
// its fields as freely as a pattern match does (startsWith is a prefix
// wildcard), so each field must allow the operator predicateOp names, and
// WithAllowedOps must include it.
func (v *Validator) validatePredicateCall(fc *ast.FuncCallExpr) {
	for _, f := range v.validateFuncCallFields(fc) {
		op := predicateOp(f.cfg)
		if !v.opGloballyAllowed(op) || !f.cfg.AllowsOp(op) {
			v.addError(ErrOperatorNotAllowed, fc.Position, f.name, string(op),
				"%s() used as a predicate needs operator %q, which is not allowed for field %q (type %s)",
				fc.Name, string(op), f.name, f.cfg.Type)
		}
	}
}

// predicateOp is the operator a field must allow before a predicate function
// may read it: wildcard for text, equality for every other type.
func predicateOp(cfg FieldConfig) Op {
	if cfg.Type == TypeText {
		return OpWildcard
	}
	return OpEq
}

// mirrorOp is op as seen from the other operand: a>b reads b as b<a.
func mirrorOp(op Op) Op {
	switch op { //nolint:exhaustive // only ordering operators have a mirror
	case OpGt:
		return OpLt
	case OpGte:
		return OpLte
	case OpLt:
		return OpGt
	case OpLte:
		return OpGte
	default:
		return op
	}
}

// checkValue type-checks one literal operand against the field, coercing it
// to the field's type first when the lexer guessed a narrower one (see
// coerceLiteral). Function- and field-valued operands are resolved at match
// time and skipped; arithmetic is checked for fields it can never produce.
func (v *Validator) checkValue(cfg FieldConfig, val *ast.Value, pos token.Position, fieldName, what string) {
	if !v.coerceShortDate(cfg, val, pos, fieldName, what) {
		return
	}
	coerceLiteral(cfg.Type, val)
	if isDynamicValue(*val) {
		if val.Type != ast.ValueArith {
			return
		}
		if cfg.Type == TypeText || cfg.Type == TypeBoolean {
			v.addError(ErrTypeMismatch, pos, fieldName, "",
				"%s is arithmetic (%s), which cannot produce a %s for field %q; quote it to compare as text",
				what, val.Raw, cfg.Type, fieldName)
			return
		}
		v.checkArithmetic(cfg, val, pos, fieldName, what)
		return
	}
	if !typeCompatible(cfg.Type, *val) {
		v.addHinted(func() string { return v.typeHint(cfg, val) }, ErrTypeMismatch, pos, fieldName, "",
			"%s %q (%s) does not fit field %q (type %s)", what, val.Raw, val.Type, fieldName, cfg.Type)
	}
}

// kind is what a value is, as far as it can be told without a record: the
// result of arithmetic over literals has one, a function or a field does not.
type kind int

const (
	kindUnknown   kind = iota // depends on a function or a field
	kindNumber                // an integer or a float
	kindDate                  // a date or an instant
	kindDuration              // a length of time
	kindUndefined             // operands the operator has no meaning for
)

func (k kind) String() string {
	switch k {
	case kindNumber:
		return "number"
	case kindDate:
		return "date"
	case kindDuration:
		return "duration"
	default:
		return "value"
	}
}

// kindOf works out what a value gives when every operand is a literal,
// following the evaluator's rules: a date plus or minus a duration is a
// date, two dates subtract to a duration, a duration scales by a number.
// An operand that is a function, a field or text makes the result unknown,
// which is left to the evaluator.
func kindOf(val *ast.Value) kind {
	if val == nil {
		return kindUnknown
	}
	switch val.Type { //nolint:exhaustive // every other type is unknown until matched
	case ast.ValueInteger, ast.ValueFloat:
		return kindNumber
	case ast.ValueDate:
		return kindDate
	case ast.ValueDuration:
		return kindDuration
	case ast.ValueArith:
		if val.Arith == nil {
			return kindUnknown
		}
		return arithKind(kindOf(val.Arith.Left), kindOf(val.Arith.Right), val.Arith.Op)
	default:
		return kindUnknown
	}
}

func arithKind(left, right kind, op ast.ArithOp) kind {
	if left == kindUndefined || right == kindUndefined {
		return kindUndefined
	}
	if left == kindUnknown || right == kindUnknown {
		return kindUnknown
	}
	additive := op == ast.ArithAdd || op == ast.ArithSub
	switch {
	case left == kindNumber && right == kindNumber:
		return kindNumber
	case left == kindDate && right == kindDuration && additive:
		return kindDate
	case left == kindDate && right == kindDate && op == ast.ArithSub:
		return kindDuration
	case left == kindDuration && right == kindDuration && additive:
		return kindDuration
	case left == kindDuration && right == kindNumber && (op == ast.ArithMul || op == ast.ArithDiv):
		return kindDuration
	case left == kindNumber && right == kindDuration && op == ast.ArithMul:
		return kindDuration
	}
	return kindUndefined
}

// fits reports whether a value of kind k can be compared with a field of
// type t.
func (k kind) fits(t FieldValueType) bool {
	switch t { //nolint:exhaustive // text, boolean and list are refused before this
	case TypeInteger, TypeDecimal:
		return k == kindNumber
	case TypeDate, TypeDatetime:
		return k == kindDate
	case TypeDuration:
		return k == kindDuration
	}
	return true
}

// checkArithmetic refuses arithmetic over literals whose result is already
// known not to fit the field: createdAt=2026-11-5 subtracts three numbers,
// and ttl>10-3 compares a duration with 7. Left alone these never match,
// which reads as "there are none" rather than as a mistake.
func (v *Validator) checkArithmetic(cfg FieldConfig, val *ast.Value, pos token.Position, fieldName, what string) {
	switch k := kindOf(val); {
	case k == kindUnknown:
	case k == kindUndefined:
		v.addHinted(func() string { return arithmeticOperandsHint }, ErrTypeMismatch, pos, fieldName, "",
			"%s is arithmetic (%s) that has no result", what, val.Raw)
	case !k.fits(cfg.Type):
		v.addHinted(func() string { return arithmeticResultHint(cfg, val) }, ErrTypeMismatch, pos, fieldName, "",
			"%s is arithmetic (%s), which gives a %s and does not fit field %q (type %s)",
			what, val.Raw, k, fieldName, cfg.Type)
	case (cfg.Type == TypeInteger || cfg.Type == TypeDecimal) && isShortDate(val.Raw):
		// The result is a number and the field takes numbers, but the
		// writer almost surely meant a date: n=2026-11 would otherwise
		// compare with 2015 and silently match the wrong records.
		v.addHinted(func() string { return shortDateOnNumberHint(val.Raw) }, ErrTypeMismatch, pos, fieldName, "",
			"%s %s has the shape of a date that is not written in full, which field %q (type %s) does not take",
			what, val.Raw, fieldName, cfg.Type)
	}
}

// shortDateLayout reads a date whose month and day may have one digit.
const shortDateLayout = "2006-1-2"

// coerceShortDate gives a date written short (2026-11-5) the meaning the
// field's declared type asks for. The lexer reads it as a subtraction,
// because it cannot see the field; the validator can. A date field takes it
// as the day it names, and a text field as its characters. It reports false
// when it has already added an error and the value needs no more checks.
func (v *Validator) coerceShortDate(cfg FieldConfig, val *ast.Value, pos token.Position, fieldName, what string) bool {
	if val.Type != ast.ValueArith || !isShortDate(val.Raw) {
		return true
	}
	switch cfg.Type { //nolint:exhaustive // number fields refuse it in checkArithmetic; the rest refuse arithmetic
	case TypeText:
		*val = ast.Value{Type: ast.ValueString, Raw: val.Raw, Str: val.Raw}
	case TypeDate, TypeDatetime:
		if strings.Count(val.Raw, "-") != 2 {
			return true // no day: refused by checkArithmetic with a hint
		}
		t, err := time.Parse(shortDateLayout, val.Raw)
		if err != nil {
			v.addHinted(func() string { return "the month is 1 to 12 and the day exists in that month" },
				ErrTypeMismatch, pos, fieldName, "", "%s %s is not a date that exists", what, val.Raw)
			return false
		}
		*val = ast.Value{Type: ast.ValueDate, Raw: val.Raw, Date: t, DayOnly: true}
	}
	return true
}

// isShortDate reports whether raw is a date written without all its digits:
// four digits, then one or two hyphen-separated groups of one or two digits
// (2026-11, 2026-1-5, 2026-11-5). A complete YYYY-MM-DD never reaches here,
// because the lexer reads it as a date.
func isShortDate(raw string) bool {
	parts := strings.Split(raw, "-")
	if len(parts) < 2 || len(parts) > 3 || len(parts[0]) != 4 || !isDigits(parts[0]) {
		return false
	}
	for _, p := range parts[1:] {
		if len(p) < 1 || len(p) > 2 || !isDigits(p) {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// opGloballyAllowed reports whether op passes the WithAllowedOps restriction
// (always true when none is configured).
func (v *Validator) opGloballyAllowed(op Op) bool {
	return v.allowedOps == nil || v.allowedOps[op]
}

// allowsOpThroughFunc reports whether op may be applied to a value derived
// from field cfg: the result of a function over it, or the field read as the
// other operand of a comparison. Equality, inequality, wildcard and presence follow
// the field's own list: a field that forbids wildcards forbids
// trim(field)=a* too. Ordering and range operators follow the field's list
// when the field is ordered (numeric, date, duration). A text field has no
// ordering operators of its own, and a function is the only way to obtain
// something orderable from it (len(name)>5); ordering the result reveals as
// much as a pattern match (lower(name)>=m narrows the value like m*), so it
// is permitted exactly when the field allows wildcards. A boolean field
// follows its equality operator.
func allowsOpThroughFunc(cfg FieldConfig, op Op) bool {
	switch op {
	case OpGt, OpGte, OpLt, OpLte, OpRange:
		switch cfg.Type { //nolint:exhaustive // ordered types use their own list
		case TypeText:
			return cfg.AllowsOp(OpWildcard)
		case TypeBoolean:
			return cfg.AllowsOp(OpEq)
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
				*val = ast.Value{Type: ast.ValueDate, Raw: val.Raw, Date: t, Quoted: val.Quoted, DayOnly: parser.IsDayLiteral(val.Str)}
			} else if strings.Count(val.Str, "-") == 2 && isShortDate(val.Str) {
				if t, err := time.Parse(shortDateLayout, val.Str); err == nil {
					*val = ast.Value{Type: ast.ValueDate, Raw: val.Raw, Date: t, Quoted: val.Quoted, DayOnly: true}
				}
			}
		}
	case TypeInteger, TypeDecimal, TypeBoolean, TypeDuration, TypeList:
		// No coercion: a text literal on these fields is a genuine mismatch.
	}
}

// validateValueFuncs walks a value's function/arithmetic subtree and validates
// any embedded field references — both function-call field arguments and
// bracketed field-ref operands ([cpu_limit]) — against the field config. pos is
// the enclosing expression's position, used for any error (Value carries none).
// It returns the fields that resolved.
func (v *Validator) validateValueFuncs(val *ast.Value, pos token.Position) []resolvedField {
	if val == nil {
		return nil
	}
	var fields []resolvedField
	if val.Type == ast.ValueFieldRef {
		fieldName := val.Field.String()
		if cfg, ok := v.resolveField(fieldName); ok {
			fields = append(fields, resolvedField{name: fieldName, cfg: cfg})
		} else {
			v.addError(ErrFieldNotFound, pos, fieldName, "", "unknown field %q", fieldName)
		}
	}
	if val.Func != nil {
		fields = append(fields, v.validateFuncCallFields(val.Func)...)
	}
	if val.Arith != nil {
		fields = append(fields, v.validateValueFuncs(val.Arith.Left, pos)...)
		fields = append(fields, v.validateValueFuncs(val.Arith.Right, pos)...)
	}
	return fields
}

// resolvedField pairs a field name with its configuration.
type resolvedField struct {
	name string
	cfg  FieldConfig
}

// validateFuncCallFields validates field references inside function
// arguments, recursing into nested calls and value-position operands, and
// returns every field that resolved.
func (v *Validator) validateFuncCallFields(fc *ast.FuncCallExpr) []resolvedField {
	var fields []resolvedField
	for _, arg := range fc.Args {
		switch {
		case arg.Field != nil:
			fieldName := arg.Field.String()
			cfg, ok := v.resolveField(fieldName)
			if !ok {
				v.addHinted(func() string { return v.argumentHint(fieldName, fc.Name) },
					ErrFieldNotFound, fc.Position, fieldName, "", "unknown field %q in function %s()", fieldName, fc.Name)
				continue
			}
			fields = append(fields, resolvedField{name: fieldName, cfg: cfg})
		case arg.Call != nil:
			fields = append(fields, v.validateFuncCallFields(arg.Call)...)
		case arg.Value != nil:
			fields = append(fields, v.validateValueFuncs(arg.Value, fc.Position)...)
		}
	}
	return fields
}

// validateSelector checks a selector against the field it is applied to.
//
// A field declared as a list is held to its config: the selector must be one
// it allows, and the condition inside is checked against the fields of an
// element, not of the record. A schema with no list field at all predates
// list declarations, and there the base only has to exist and the condition
// is checked against the record's fields.
func (v *Validator) validateSelector(s *ast.SelectorExpr) {
	b, ok := s.Base.(*ast.PresenceExpr)
	if !ok {
		if q, isQualifier := s.Base.(*ast.QualifierExpr); isQualifier {
			v.validateQualifier(q)
		} else {
			v.validate(s.Base)
		}
		if s.Inner != nil {
			v.validate(s.Inner)
		}
		return
	}
	fieldName := b.Field.String()
	form := ast.SelectorForm(s.Selector)
	cfg, ok := v.resolveField(fieldName)
	switch {
	case !ok:
		v.addError(ErrFieldNotFound, b.Position, fieldName, "", "unknown field %q", fieldName)
	case cfg.Type == TypeList:
		if !cfg.AllowsSelector(s.Selector) {
			v.addHinted(func() string { return selectorHint(fieldName, s.Selector, cfg) },
				ErrOperatorNotAllowed, s.Position, fieldName, form,
				"selector %s is not allowed for field %q", form, fieldName)
		}
		if s.Inner != nil && len(cfg.Elements) > 0 {
			v.validateElements(cfg, s.Inner)
			return
		}
	case v.hasLists:
		v.addHinted(func() string { return v.listFieldsHint() },
			ErrOperatorNotAllowed, s.Position, fieldName, form, "%q is not a list field", fieldName)
		// The condition inside names fields of an element that does not
		// exist, so checking it would only add errors about the same mistake.
		return
	}
	if s.Inner != nil {
		v.validate(s.Inner)
	}
}

// validateElements checks the condition inside a selector against the fields
// of one element. Only those are in scope there: `forks@(name=x)` reads the
// name of a fork, not of the record that has the forks.
func (v *Validator) validateElements(list FieldConfig, inner ast.Expression) {
	scope := New(list.Elements)
	scope.allowedOps = v.allowedOps
	scope.allowedFuncs = v.allowedFuncs
	scope.missingTest = v.missingTest
	scope.validate(inner)
	for _, e := range scope.errors {
		// Say which list the name was looked up in, once: an error from a
		// list inside this one already names its own.
		if e.Kind == ErrFieldNotFound && !strings.Contains(e.Message, ` on "`) {
			e.Message += fmt.Sprintf(" on %q", list.Name)
		}
	}
	v.errors = append(v.errors, scope.errors...)
}

// selectorHint says what to write in place of a selector the list refuses.
func selectorHint(field, name string, cfg FieldConfig) string {
	switch {
	case name == ast.SelectorLast && cfg.AllowsSelector(ast.SelectorFirst):
		return fmt.Sprintf("to test that the list has an element write %s@first", field)
	case name == ast.SelectorAny && cfg.AllowsSelector(ast.SelectorExists):
		return fmt.Sprintf("write it without the word: %s@(...)", field)
	}
	return fmt.Sprintf("%s allows: %s", field, strings.Join(cfg.SelectorForms(), ", "))
}

// listFieldsHint names the fields a selector can be applied to.
func (v *Validator) listFieldsHint() string {
	var lists []string
	for name, cfg := range v.fields {
		if cfg.Type == TypeList {
			lists = append(lists, name)
		}
	}
	if len(lists) == 0 {
		return ""
	}
	sort.Strings(lists)
	return "a selector applies to a list field: " + strings.Join(lists, ", ")
}

func (v *Validator) validatePresence(p *ast.PresenceExpr) {
	fieldName := p.Field.String()
	cfg, ok := v.resolveField(fieldName)
	if !ok {
		v.addHinted(func() string { return v.bareWordHint(fieldName) },
			ErrFieldNotFound, p.Position, fieldName, "", "unknown field %q", fieldName)
		return
	}
	if !v.opGloballyAllowed(OpPresence) || !cfg.AllowsOp(OpPresence) {
		hint := func() string { return v.opHint(fieldName, OpPresence) }
		if v.missingTest {
			hint = func() string { return hintwords.Missing }
		}
		v.addHinted(hint, ErrOperatorNotAllowed, p.Position, fieldName, string(OpPresence),
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

// addError records a failure with the hint its kind implies: the nearest
// field for an unknown one, the operators a field allows for a refused one.
// A site that knows more (the function a name was passed to, the literal that
// did not fit) calls [Validator.addHinted] with its own.
func (v *Validator) addError(kind ErrorKind, pos token.Position, field, op, format string, args ...any) {
	var hint func() string
	switch kind { //nolint:exhaustive // the other kinds carry no generic hint
	case ErrFieldNotFound:
		hint = func() string { return v.fieldHint(field) }
	case ErrOperatorNotAllowed:
		if field != "" {
			hint = func() string { return v.opHint(field, Op(op)) }
		}
	}
	v.addHinted(hint, kind, pos, field, op, format, args...)
}

// maxHinted is how many errors of one validation get a hint. A hint compares
// a name against every field, so without a bound a long query of unknown
// names would cost its length times the size of the schema.
const maxHinted = 8

// addHinted records a failure whose hint is hint(), computed once and only
// for the first maxHinted errors. hint may be nil.
func (v *Validator) addHinted(hint func() string, kind ErrorKind, pos token.Position, field, op, format string, args ...any) {
	e := &Error{
		Message:  fmt.Sprintf(format, args...),
		Position: pos,
		Kind:     kind,
		Field:    field,
		Op:       op,
	}
	if hint != nil && len(v.errors) < maxHinted {
		e.Hint = hint()
	}
	v.errors = append(v.errors, e)
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
