package eval

import (
	"fmt"
	"strings"
	"time"

	"github.com/heyllave/query/ast"
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
		field := e.Field.String()
		return func(get func(string) (any, bool)) bool {
			_, ok := get(field)
			return ok
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
		argResolver := compileArgResolvers(e.FieldFunc.Args, funcs)
		fn, hasFn := funcs.Get(e.FieldFunc.Name)
		fieldResolver = func(get func(string) (any, bool)) (any, bool) {
			if !hasFn {
				return nil, false
			}
			args := resolveArgs(argResolver, get)
			result, err := fn.Call(args...)
			if err != nil {
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

	// Wildcard: pattern matching
	if e.IsWildcard() {
		return compileWildcardWithResolver(fieldResolver, e.Value.Str)
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
		argResolvers := compileArgResolvers(v.Func.Args, funcs)
		fn, hasFn := funcs.Get(v.Func.Name)
		return func(get func(string) (any, bool)) *ast.Value {
			if !hasFn {
				return nil
			}
			args := resolveArgs(argResolvers, get)
			result, err := fn.Call(args...)
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
				return valueFromAny(a + b)
			case ast.ArithSub:
				return valueFromAny(a - b)
			case ast.ArithMul:
				return valueFromAny(a * b)
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
	case int:
		return &ast.Value{Type: ast.ValueInteger, Raw: fmt.Sprint(x), Int: int64(x)}
	case int32:
		return &ast.Value{Type: ast.ValueInteger, Raw: fmt.Sprint(x), Int: int64(x)}
	case int64:
		return &ast.Value{Type: ast.ValueInteger, Raw: fmt.Sprint(x), Int: x}
	case float32:
		return &ast.Value{Type: ast.ValueFloat, Raw: fmt.Sprint(x), Float: float64(x)}
	case float64:
		return &ast.Value{Type: ast.ValueFloat, Raw: fmt.Sprint(x), Float: x}
	case time.Time:
		return &ast.Value{Type: ast.ValueDate, Raw: x.Format(time.RFC3339), Date: x}
	case time.Duration:
		return &ast.Value{Type: ast.ValueDuration, Raw: x.String(), Duration: x}
	case []any:
		return &ast.Value{Type: ast.ValueList, Raw: fmt.Sprint(x), List: x}
	case nil:
		return nil
	default:
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
	fn, hasFn := funcs.Get(e.Name)
	if !hasFn {
		return func(func(string) (any, bool)) bool { return false }
	}
	argResolvers := compileArgResolvers(e.Args, funcs)

	return func(get func(string) (any, bool)) bool {
		args := resolveArgs(argResolvers, get)
		result, err := fn.Call(args...)
		if err != nil {
			return false
		}
		return toBool(result)
	}
}

type argResolver func(get func(string) (any, bool)) any

func compileArgResolvers(args []ast.FuncArg, funcs FuncRegistry) []argResolver {
	resolvers := make([]argResolver, len(args))
	for i, arg := range args {
		switch {
		case arg.Field != nil:
			field := arg.Field.String()
			resolvers[i] = func(get func(string) (any, bool)) any {
				v, _ := get(field)
				return v
			}
		case arg.Value != nil:
			val := arg.Value.Any()
			resolvers[i] = func(func(string) (any, bool)) any {
				return val
			}
		case arg.Call != nil:
			fn, hasFn := funcs.Get(arg.Call.Name)
			innerResolvers := compileArgResolvers(arg.Call.Args, funcs)
			resolvers[i] = func(get func(string) (any, bool)) any {
				if !hasFn {
					return nil
				}
				innerArgs := resolveArgs(innerResolvers, get)
				result, _ := fn.Call(innerArgs...)
				return result
			}
		default:
			resolvers[i] = func(func(string) (any, bool)) any { return nil }
		}
	}
	return resolvers
}

func resolveArgs(resolvers []argResolver, get func(string) (any, bool)) []any {
	args := make([]any, len(resolvers))
	for i, r := range resolvers {
		args[i] = r(get)
	}
	return args
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
		return compareValues(raw, s, token.Gte) && compareValues(raw, e, token.Lte)
	}
}

func compileWildcardWithResolver(resolve func(func(string) (any, bool)) (any, bool), pattern string) matcher {
	prefix := strings.HasPrefix(pattern, "*")
	suffix := strings.HasSuffix(pattern, "*")
	inner := strings.Trim(pattern, "*")

	return func(get func(string) (any, bool)) bool {
		raw, ok := resolve(get)
		if !ok {
			return false
		}
		s := strings.ToLower(fmt.Sprint(raw))
		lowerInner := strings.ToLower(inner)
		switch {
		case prefix && suffix:
			return strings.Contains(s, lowerInner)
		case prefix:
			return strings.HasSuffix(s, lowerInner)
		case suffix:
			return strings.HasPrefix(s, lowerInner)
		default:
			return s == lowerInner
		}
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
			return !equalValues(raw, ev)
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

func equalValues(actual any, expected *ast.Value) bool {
	switch expected.Type {
	case ast.ValueString:
		return strings.EqualFold(stringifyActual(actual), expected.Str)
	case ast.ValueInteger:
		return toInt64(actual) == expected.Int
	case ast.ValueFloat:
		return toFloat64(actual) == expected.Float
	case ast.ValueBoolean:
		return toBool(actual) == expected.Bool
	case ast.ValueDate:
		return toTime(actual).Equal(expected.Date)
	case ast.ValueDuration:
		return toDuration(actual) == expected.Duration
	case ast.ValueList:
		// A list-valued operand that reaches a scalar comparison is matched on
		// its string form, case-insensitively — the same semantics every other
		// comparable type uses.
		return strings.EqualFold(stringifyActual(actual), expected.Raw)
	default:
		return stringifyActual(actual) == expected.Raw
	}
}

func compareValues(actual any, expected *ast.Value, op token.Type) bool {
	switch expected.Type {
	case ast.ValueInteger:
		a := toInt64(actual)
		b := expected.Int
		return compareOrdered(a, b, op)
	case ast.ValueFloat:
		a := toFloat64(actual)
		b := expected.Float
		return compareOrdered(a, b, op)
	case ast.ValueDate:
		a := toTime(actual)
		b := expected.Date
		switch op { //nolint:exhaustive // only relational
		case token.Gt:
			return a.After(b)
		case token.Gte:
			return !a.Before(b)
		case token.Lt:
			return a.Before(b)
		case token.Lte:
			return !a.After(b)
		default:
			return a.Equal(b)
		}
	case ast.ValueDuration:
		a := int64(toDuration(actual))
		b := int64(expected.Duration)
		return compareOrdered(a, b, op)
	default:
		a := stringifyActual(actual)
		b := expected.Raw
		return compareOrdered(a, b, op)
	}
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

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(b, "true")
	default:
		return false
	}
}

func toTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func toDuration(v any) time.Duration {
	switch d := v.(type) {
	case time.Duration:
		return d
	case string:
		if parsed, err := time.ParseDuration(d); err == nil {
			return parsed
		}
	}
	return 0
}
