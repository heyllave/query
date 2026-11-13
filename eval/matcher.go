package eval

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/token"
)

// matcher is a compiled function that evaluates a query against a data accessor.
type matcher func(get func(field string) (any, bool)) bool

// compileMatcher walks the AST and produces a closure tree for fast evaluation.
func compileMatcher(expr ast.Expression, funcs FuncRegistry) matcher {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		left := compileMatcher(e.Left, funcs)
		right := compileMatcher(e.Right, funcs)
		if e.Op == token.And {
			return func(get func(string) (any, bool)) bool {
				return left(get) && right(get)
			}
		}
		return func(get func(string) (any, bool)) bool {
			return left(get) || right(get)
		}

	case *ast.UnaryExpr:
		inner := compileMatcher(e.Expr, funcs)
		return func(get func(string) (any, bool)) bool {
			return !inner(get)
		}

	case *ast.GroupExpr:
		return compileMatcher(e.Expr, funcs)

	case *ast.QualifierExpr:
		return compileQualifier(e, funcs)

	case *ast.PresenceExpr:
		// Presence means the field exists and is non-empty: nil, an empty
		// string, and an empty list or map all count as absent.
		field := e.Field.String()
		return func(get func(string) (any, bool)) bool {
			v, ok := get(field)
			return ok && !isEmpty(v)
		}

	case *ast.FuncCallExpr:
		return compileFuncCallBool(e, funcs)

	case *ast.SelectorExpr:
		return compileSelector(e, funcs)

	default:
		return func(func(string) (any, bool)) bool { return false }
	}
}

func compileQualifier(e *ast.QualifierExpr, funcs FuncRegistry) matcher {
	field := e.Field.String()

	// If there's a field transform function (e.g., lower(name)=john*),
	// resolve the field value through the function first.
	var fieldResolver func(get func(string) (any, bool)) (any, bool)
	if e.HasFieldFunc() {
		call := compileCall(e.FieldFunc, funcs)
		fieldResolver = func(get func(string) (any, bool)) (any, bool) {
			result, err := call(get)
			if err != nil || result == nil {
				return nil, false
			}
			return result, true
		}
	} else {
		fieldResolver = func(get func(string) (any, bool)) (any, bool) {
			return get(field)
		}
	}

	// Resolve the RHS value at match time. Static literals close over their
	// own pointer; functions in value position (e.g. created_at>=now()) are
	// evaluated against the function registry each call.
	valueResolver := compileValueResolver(&e.Value, funcs)
	endResolver := compileValueResolver(e.EndValue, funcs)

	// Range: field BETWEEN start AND end
	if e.IsRange() {
		return compileRangeWithResolver(fieldResolver, valueResolver, endResolver)
	}

	// Wildcard: pattern matching; `field!=pat*` is its negation on a present
	// field (a missing field is false either way, as for every comparison).
	if e.IsWildcard() {
		return compileWildcardWithResolver(fieldResolver, e.Value.Str, e.Operator == token.Neq)
	}

	// Standard comparison
	return compileComparisonWithResolver(fieldResolver, e.Operator, valueResolver)
}

// valueResolver produces a *ast.Value at match time. For static literals it
// returns the same pointer every call; for ValueFunc it evaluates the function
// against the registry and synthesizes a transient Value carrying the result.
type valueResolver func(get func(string) (any, bool)) *ast.Value

func compileValueResolver(v *ast.Value, funcs FuncRegistry) valueResolver {
	if v == nil {
		return nil
	}
	switch v.Type {
	case ast.ValueFunc:
		if v.Func == nil {
			return func(func(string) (any, bool)) *ast.Value { return nil }
		}
		call := compileCall(v.Func, funcs)
		return func(get func(string) (any, bool)) *ast.Value {
			result, err := call(get)
			if err != nil {
				return nil
			}
			return valueFromAny(result)
		}
	case ast.ValueArith:
		if v.Arith == nil {
			return func(func(string) (any, bool)) *ast.Value { return nil }
		}
		left := compileValueResolver(v.Arith.Left, funcs)
		right := compileValueResolver(v.Arith.Right, funcs)
		op := v.Arith.Op
		return func(get func(string) (any, bool)) *ast.Value {
			l := left(get)
			r := right(get)
			if l == nil || r == nil {
				return nil
			}
			return applyArith(l, r, op)
		}
	case ast.ValueFieldRef:
		// Resolve the referenced field from the record at match time. A missing
		// or nil field yields a nil value, which folds the comparison to false
		// (predicate) or ErrNoValue (value domain), like any unresolved operand.
		field := v.Field.String()
		return func(get func(string) (any, bool)) *ast.Value {
			raw, ok := get(field)
			if !ok {
				return nil
			}
			return valueFromAny(raw)
		}
	default:
		// Capture the pointer — for static literals nothing depends on `get`.
		return func(func(string) (any, bool)) *ast.Value { return v }
	}
}

// applyArith evaluates `left op right` and returns a transient Value carrying
// the result. Type promotion mirrors Go's: int op int → int (except `/` which
// always produces float), int op float → float, time op duration → time,
// duration op int|float → duration. Anything else returns nil so the
// comparison falls back to the default-false missing-operand path.
func applyArith(left, right *ast.Value, op ast.ArithOp) *ast.Value {
	// time ± duration → time. Other ops on (date, duration) are undefined
	// and fall through to the final `return nil` below.
	if left.Type == ast.ValueDate && right.Type == ast.ValueDuration {
		switch op {
		case ast.ArithAdd:
			return valueFromAny(left.Date.Add(right.Duration))
		case ast.ArithSub:
			return valueFromAny(left.Date.Add(-right.Duration))
		default:
		}
	}
	// time - time → duration
	if left.Type == ast.ValueDate && right.Type == ast.ValueDate && op == ast.ArithSub {
		return valueFromAny(left.Date.Sub(right.Date))
	}
	// duration ± duration → duration. Multiplicative ops on two durations are
	// undefined and fall through.
	if left.Type == ast.ValueDuration && right.Type == ast.ValueDuration {
		switch op {
		case ast.ArithAdd:
			return valueFromAny(left.Duration + right.Duration)
		case ast.ArithSub:
			return valueFromAny(left.Duration - right.Duration)
		default:
		}
	}
	// duration * number → duration; duration / number → duration. Additive ops
	// between a duration and a unitless number are undefined and fall through.
	if left.Type == ast.ValueDuration && isNumericValue(right) {
		rf := numericFloat(right)
		switch op {
		case ast.ArithMul:
			return valueFromAny(time.Duration(float64(left.Duration) * rf))
		case ast.ArithDiv:
			if rf == 0 {
				return nil
			}
			return valueFromAny(time.Duration(float64(left.Duration) / rf))
		default:
		}
	}
	if isNumericValue(left) && right.Type == ast.ValueDuration && op == ast.ArithMul {
		lf := numericFloat(left)
		return valueFromAny(time.Duration(lf * float64(right.Duration)))
	}
	// pure numeric arithmetic
	if isNumericValue(left) && isNumericValue(right) {
		// Stay in int if both sides are integers and the op preserves it.
		// ArithDiv is excluded by the guard above (int/int always promotes to
		// float, matching Go's float division semantics for queries).
		if left.Type == ast.ValueInteger && right.Type == ast.ValueInteger && op != ast.ArithDiv {
			a, b := left.Int, right.Int
			switch op {
			case ast.ArithAdd:
				if r, ok := addInt64(a, b); ok {
					return valueFromAny(r)
				}
				return valueFromAny(float64(a) + float64(b)) // overflow promotes to float
			case ast.ArithSub:
				if r, ok := addInt64(a, -b); ok && b != math.MinInt64 {
					return valueFromAny(r)
				}
				return valueFromAny(float64(a) - float64(b))
			case ast.ArithMul:
				if r, ok := mulInt64(a, b); ok {
					return valueFromAny(r)
				}
				return valueFromAny(float64(a) * float64(b))
			case ast.ArithMod:
				if b == 0 {
					return nil
				}
				return valueFromAny(a % b)
			case ast.ArithDiv:
				// unreachable — guarded above
			}
		}
		a := numericFloat(left)
		b := numericFloat(right)
		switch op {
		case ast.ArithAdd:
			return valueFromAny(a + b)
		case ast.ArithSub:
			return valueFromAny(a - b)
		case ast.ArithMul:
			return valueFromAny(a * b)
		case ast.ArithDiv:
			if b == 0 {
				return nil
			}
			return valueFromAny(a / b)
		case ast.ArithMod:
			if b == 0 {
				return nil
			}
			// Modulo on floats — use math.Mod semantics via int conversion to
			// keep the closure-eval deps to zero. For real-world fractional
			// modulo, fall through to nil.
			if a == float64(int64(a)) && b == float64(int64(b)) {
				return valueFromAny(int64(a) % int64(b))
			}
			return nil
		}
	}
	return nil
}

func isNumericValue(v *ast.Value) bool {
	return v != nil && (v.Type == ast.ValueInteger || v.Type == ast.ValueFloat)
}

// addInt64 adds with overflow detection; ok is false when the sum wraps.
func addInt64(a, b int64) (int64, bool) {
	r := a + b
	// Overflow iff both operands share a sign that the result does not.
	if (a >= 0) == (b >= 0) && (r >= 0) != (a >= 0) {
		return 0, false
	}
	return r, true
}

// mulInt64 multiplies with overflow detection; ok is false when the product
// wraps.
func mulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	r := a * b
	if r/b != a || (a == -1 && b == math.MinInt64) || (b == -1 && a == math.MinInt64) {
		return 0, false
	}
	return r, true
}

func numericFloat(v *ast.Value) float64 {
	if v.Type == ast.ValueInteger {
		return float64(v.Int)
	}
	return v.Float
}

// valueFromAny coerces a Go value (typically the return of a registered
// function) into an *ast.Value suitable for comparison. The mapping mirrors
// the literal types the lexer can produce. Unknown shapes fall back to
// stringification.
func valueFromAny(v any) *ast.Value {
	switch x := v.(type) {
	case string:
		return &ast.Value{Type: ast.ValueString, Raw: x, Str: x}
	case bool:
		return &ast.Value{Type: ast.ValueBoolean, Raw: fmt.Sprint(x), Bool: x}
	case time.Time:
		return &ast.Value{Type: ast.ValueDate, Raw: x.Format(time.RFC3339), Date: x}
	case time.Duration:
		return &ast.Value{Type: ast.ValueDuration, Raw: x.String(), Duration: x}
	case []any:
		return &ast.Value{Type: ast.ValueList, Raw: fmt.Sprint(x), List: x}
	case nil:
		return nil
	default:
		if i, f, isInt, ok := numberOf(x); ok {
			if isInt {
				return &ast.Value{Type: ast.ValueInteger, Raw: fmt.Sprint(i), Int: i}
			}
			return &ast.Value{Type: ast.ValueFloat, Raw: fmt.Sprint(f), Float: f}
		}
		// Typed slices ([]string, []int, …) are preserved as lists so value
		// expressions can return collections; everything else stringifies.
		if elems, ok := toSlice(x); ok {
			return &ast.Value{Type: ast.ValueList, Raw: fmt.Sprint(x), List: elems}
		}
		s := fmt.Sprint(x)
		return &ast.Value{Type: ast.ValueString, Raw: s, Str: s}
	}
}

// compileFuncCallBool compiles a standalone function call as a boolean predicate.
// e.g., contains(tags, "urgent")
func compileFuncCallBool(e *ast.FuncCallExpr, funcs FuncRegistry) matcher {
	call := compileCall(e, funcs)
	return func(get func(string) (any, bool)) bool {
		result, err := call(get)
		if err != nil {
			return false
		}
		return toBool(result)
	}
}

// errUnknownFunc is the runtime error for a call to a function the registry
// does not hold. Compile reports it earlier (see checkCalls); this guards
// registries assembled without going through Compile.
var errUnknownFunc = errors.New("unknown function")

// call evaluates a compiled function call against a record. An error means
// the call did not resolve: the function is unknown, an argument's nested
// call failed, or the function itself returned an error.
type call func(get func(string) (any, bool)) (any, error)

// compileCall compiles a function call, including its arguments, into a
// closure. Errors from nested calls propagate instead of being replaced by a
// nil argument, so len(lower()) is unresolved rather than len("<nil>").
func compileCall(fc *ast.FuncCallExpr, funcs FuncRegistry) call {
	fn, hasFn := funcs.Get(fc.Name)
	if !hasFn {
		return func(func(string) (any, bool)) (any, error) {
			return nil, fmt.Errorf("%w %q", errUnknownFunc, fc.Name)
		}
	}
	argResolvers := compileArgResolvers(fc.Args, funcs)
	return func(get func(string) (any, bool)) (any, error) {
		args, err := resolveArgs(argResolvers, get)
		if err != nil {
			return nil, err
		}
		return fn.Call(args...)
	}
}

type argResolver func(get func(string) (any, bool)) (any, error)

func compileArgResolvers(args []ast.FuncArg, funcs FuncRegistry) []argResolver {
	resolvers := make([]argResolver, len(args))
	for i, arg := range args {
		switch {
		case arg.Field != nil:
			field := arg.Field.String()
			resolvers[i] = func(get func(string) (any, bool)) (any, error) {
				v, _ := get(field)
				return v, nil
			}
		case arg.Value != nil:
			resolver := compileValueResolver(arg.Value, funcs)
			resolvers[i] = func(get func(string) (any, bool)) (any, error) {
				v := resolver(get)
				if v == nil {
					return nil, nil
				}
				return v.Any(), nil
			}
		case arg.Call != nil:
			resolvers[i] = argResolver(compileCall(arg.Call, funcs))
		default:
			resolvers[i] = func(func(string) (any, bool)) (any, error) { return nil, nil }
		}
	}
	return resolvers
}

func resolveArgs(resolvers []argResolver, get func(string) (any, bool)) ([]any, error) {
	args := make([]any, len(resolvers))
	for i, r := range resolvers {
		v, err := r(get)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	return args, nil
}

func compileRangeWithResolver(resolve func(func(string) (any, bool)) (any, bool), start, end valueResolver) matcher {
	return func(get func(string) (any, bool)) bool {
		raw, ok := resolve(get)
		if !ok {
			return false
		}
		s := start(get)
		if s == nil {
			return false
		}
		e := end(get)
		if e == nil {
			return false
		}
		// Lte on a day literal already reads as "before the end of that
		// day", so created:2026-01-01..2026-01-31 covers all of the 31st.
		return compareValues(raw, s, token.Gte) && compareValues(raw, e, token.Lte)
	}
}

func compileWildcardWithResolver(resolve func(func(string) (any, bool)) (any, bool), pattern string, negate bool) matcher {
	// An escaped star in the pattern is a literal star (see the lexer); the
	// wildcard stars are the unescaped ones at either end.
	prefix := strings.HasPrefix(pattern, "*")
	suffix := len(pattern) > 1 && strings.HasSuffix(pattern, "*") && !strings.HasSuffix(pattern, `\*`)
	inner := pattern
	if prefix {
		inner = inner[1:]
	}
	if suffix {
		inner = inner[:len(inner)-1]
	}
	if pattern == "*" {
		suffix = true // a lone star matches any present value
	}
	lowerInner := strings.ToLower(strings.ReplaceAll(inner, `\*`, "*"))

	return func(get func(string) (any, bool)) bool {
		raw, ok := resolve(get)
		if !ok {
			return false
		}
		s := strings.ToLower(stringifyActual(raw))
		var matched bool
		switch {
		case prefix && suffix:
			matched = strings.Contains(s, lowerInner)
		case prefix:
			matched = strings.HasSuffix(s, lowerInner)
		case suffix:
			matched = strings.HasPrefix(s, lowerInner)
		default:
			matched = s == lowerInner
		}
		return matched != negate
	}
}

func compileComparisonWithResolver(resolve func(func(string) (any, bool)) (any, bool), op token.Type, expected valueResolver) matcher {
	return func(get func(string) (any, bool)) bool {
		raw, ok := resolve(get)
		if !ok {
			return false
		}
		ev := expected(get)
		if ev == nil {
			return false
		}
		switch op { //nolint:exhaustive // only comparison operators
		case token.Eq:
			return equalValues(raw, ev)
		case token.Neq:
			// A record value that cannot be read as the literal's type is
			// unequal in the sense of "not comparable", and the comparison is
			// false like every other operator, so `n!=0` does not match a
			// record whose n is "abc" any more than `n=0` does.
			return readableAs(raw, ev) && !equalValues(raw, ev)
		default:
			return compareValues(raw, ev, op)
		}
	}
}

// stringifyActual renders a record value to its canonical string form for
// comparison against a string-typed expected value. time.Time and
// time.Duration use the same RFC3339 / String() forms as valueFromAny, so a
// time-typed field compares equal to string(timeExpr); everything else uses its
// default representation.
func stringifyActual(v any) string {
	switch x := v.(type) {
	case time.Time:
		return x.Format(time.RFC3339)
	case time.Duration:
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}

// equalValues reports whether a record value equals an expected literal. A
// record value that cannot be read as the literal's type (a string where a
// number is expected, an unparseable date) is unequal: it never compares as
// zero, so `n=0` does not match a record whose n is "abc", and `n!=0` does.
func equalValues(actual any, expected *ast.Value) bool {
	switch expected.Type {
	case ast.ValueString:
		if actual == nil {
			return false
		}
		return strings.EqualFold(stringifyActual(actual), expected.Str)
	case ast.ValueInteger, ast.ValueFloat:
		return compareNumbers(actual, expected, token.Eq)
	case ast.ValueBoolean:
		b, ok := actual.(bool)
		if !ok {
			s, isStr := actual.(string)
			if !isStr {
				return false
			}
			b, ok = parseBool(s)
			if !ok {
				return false
			}
		}
		return b == expected.Bool
	case ast.ValueDate:
		t, ok := toTime(actual)
		return ok && compareDate(t, expected, token.Eq)
	case ast.ValueDuration:
		d, ok := toDuration(actual)
		return ok && d == expected.Duration
	case ast.ValueList:
		// A list-valued operand that reaches a scalar comparison is matched on
		// its string form, case-insensitively — the same semantics every other
		// comparable type uses.
		return strings.EqualFold(stringifyActual(actual), expected.Raw)
	default:
		return stringifyActual(actual) == expected.Raw
	}
}

// compareValues applies a relational operator between a record value and an
// expected literal. A record value that cannot be read as the literal's type
// makes the comparison false, whatever the operator.
func compareValues(actual any, expected *ast.Value, op token.Type) bool {
	switch expected.Type {
	case ast.ValueInteger, ast.ValueFloat:
		return compareNumbers(actual, expected, op)
	case ast.ValueDate:
		a, ok := toTime(actual)
		if !ok {
			return false
		}
		return compareDate(a, expected, op)
	case ast.ValueDuration:
		a, ok := toDuration(actual)
		if !ok {
			return false
		}
		return compareOrdered(int64(a), int64(expected.Duration), op)
	default:
		if actual == nil {
			return false
		}
		a := stringifyActual(actual)
		b := expected.Raw
		return compareOrdered(a, b, op)
	}
}

// readableAs reports whether a record value can be read as the literal's
// type at all: a number for a numeric literal, a date for a date literal, and
// so on. Strings and lists accept any non-nil value.
func readableAs(actual any, expected *ast.Value) bool {
	if actual == nil {
		return false
	}
	switch expected.Type { //nolint:exhaustive // other literal types read as text
	case ast.ValueInteger, ast.ValueFloat:
		_, _, _, ok := numberOf(actual)
		return ok
	case ast.ValueBoolean:
		if _, ok := actual.(bool); ok {
			return true
		}
		s, ok := actual.(string)
		if !ok {
			return false
		}
		_, ok = parseBool(s)
		return ok
	case ast.ValueDate:
		_, ok := toTime(actual)
		return ok
	case ast.ValueDuration:
		_, ok := toDuration(actual)
		return ok
	default:
		return true
	}
}

// compareDate compares an instant with a date literal. A literal with a time
// is an instant and compares exactly. A day literal (2026-09-28) is the UTC
// day [start, end): `=` is any instant in it (so createdAt=2026-09-28 finds
// everything created that day, issue #24), `!=` its complement, `<` before
// the day, `<=` before its end, `>` at or after its end, `>=` at or after its
// start. A value at midnight behaves the same under both readings, so date
// fields are unaffected.
func compareDate(a time.Time, expected *ast.Value, op token.Type) bool {
	start := expected.Date
	end := start
	if expected.DayOnly {
		end = start.AddDate(0, 0, 1)
	}
	inDay := !a.Before(start) && (a.Before(end) || (!expected.DayOnly && a.Equal(start)))
	switch op { //nolint:exhaustive // only comparison operators
	case token.Eq:
		return inDay
	case token.Neq:
		return !inDay
	case token.Lt:
		return a.Before(start)
	case token.Lte:
		if expected.DayOnly {
			return a.Before(end)
		}
		return !a.After(start)
	case token.Gt:
		if expected.DayOnly {
			return !a.Before(end)
		}
		return a.After(start)
	case token.Gte:
		return !a.Before(start)
	default:
		return false
	}
}

// compareNumbers compares a record value with a numeric literal. The
// comparison is exact in int64 when both sides are integers and in float64
// otherwise, so `price>3` against 3.5 is true (nothing is truncated) and
// `price=3` against 3.9 is false.
func compareNumbers(actual any, expected *ast.Value, op token.Type) bool {
	ai, af, aIsInt, ok := numberOf(actual)
	if !ok {
		return false
	}
	if aIsInt && expected.Type == ast.ValueInteger {
		return compareOrdered(ai, expected.Int, op)
	}
	ef := expected.Float
	if expected.Type == ast.ValueInteger {
		ef = float64(expected.Int)
	}
	return compareOrdered(af, ef, op)
}

type ordered interface {
	~int64 | ~float64 | ~string
}

func compareOrdered[T ordered](a, b T, op token.Type) bool {
	switch op { //nolint:exhaustive // only relational operators
	case token.Gt:
		return a > b
	case token.Gte:
		return a >= b
	case token.Lt:
		return a < b
	case token.Lte:
		return a <= b
	case token.Eq:
		return a == b
	case token.Neq:
		return a != b
	default:
		return false
	}
}

// Type coercion helpers

// numberOf reads v as a number. Every Go integer kind (signed and unsigned,
// any width) reports isInt with its int64 value (and the same value as a
// float); float32 and float64 report their float64 value. Named numeric
// types (type Cents int64) are read through reflection. Anything else is not
// a number. A uint64 above math.MaxInt64 is read as a float, since it has no
// exact int64 form.
func numberOf(v any) (i int64, f float64, isInt, ok bool) {
	switch n := v.(type) {
	case int:
		return int64(n), float64(n), true, true
	case int64:
		return n, float64(n), true, true
	case int32:
		return int64(n), float64(n), true, true
	case int16:
		return int64(n), float64(n), true, true
	case int8:
		return int64(n), float64(n), true, true
	case uint:
		return uintNumber(uint64(n))
	case uint64:
		return uintNumber(n)
	case uint32:
		return int64(n), float64(n), true, true
	case uint16:
		return int64(n), float64(n), true, true
	case uint8:
		return int64(n), float64(n), true, true
	case float64:
		return int64(n), n, false, true
	case float32:
		return int64(n), float64(n), false, true
	case time.Duration:
		// A Duration is an int64 of nanoseconds; comparing it with a number
		// literal is not meaningful, so it is not a number here. Duration
		// literals compare through toDuration.
		return 0, 0, false, false
	case nil:
		return 0, 0, false, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() { //nolint:exhaustive // only numeric kinds
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := rv.Int()
		return n, float64(n), true, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintNumber(rv.Uint())
	case reflect.Float32, reflect.Float64:
		n := rv.Float()
		return int64(n), n, false, true
	default:
		return 0, 0, false, false
	}
}

func uintNumber(n uint64) (int64, float64, bool, bool) {
	if n > math.MaxInt64 {
		return 0, float64(n), false, true
	}
	return int64(n), float64(n), true, true
}

// toInt64 reads v as an int64, truncating floats toward zero; a non-number is
// 0. Comparisons do not use it (they use numberOf, which distinguishes a
// non-number from zero); it serves the builtins that take a count argument.
func toInt64(v any) int64 {
	i, _, _, _ := numberOf(v)
	return i
}

// toFloat64 reads v as a float64; a non-number is 0. See toInt64.
func toFloat64(v any) float64 {
	_, f, _, _ := numberOf(v)
	return f
}

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		r, _ := parseBool(b)
		return r
	default:
		return false
	}
}

// parseBool reads the strings true/false (any case) as booleans.
func parseBool(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// toTime reads v as a time: a time.Time, or a string in one of the date
// literal layouts (2026-01-01, RFC 3339, 2026-01-01T10:00:00,
// 2026-01-01 10:00:00). ok is false for anything else, including an
// unparseable string, so a comparison never silently uses the zero time.
func toTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	case string:
		parsed, err := parser.ParseDateLiteral(t)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	}
	return time.Time{}, false
}

// toDuration reads v as a duration: a time.Duration, or a string in Go's
// duration syntax (1h30m) or the query literal syntax (2d, 1w).
func toDuration(v any) (time.Duration, bool) {
	switch d := v.(type) {
	case time.Duration:
		return d, true
	case string:
		if parsed, err := time.ParseDuration(d); err == nil {
			return parsed, true
		}
		if parsed, err := parser.ParseDuration(d); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// isEmpty reports whether a present value should count as absent for a
// presence check: nil, an empty string, or an empty slice, array or map.
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() { //nolint:exhaustive // only container and pointer kinds
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}
