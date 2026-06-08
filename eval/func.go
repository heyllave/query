package eval

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Func is a registered function that can be called from query expressions.
// Functions receive resolved arguments and return a value.
type Func struct {
	Name string
	// Call receives the resolved argument values and returns the result.
	Call func(args ...any) (any, error)
}

// FuncRegistry maps function names to their implementations.
type FuncRegistry map[string]Func

// Register adds a function to the registry.
func (r FuncRegistry) Register(f Func) {
	r[f.Name] = f
}

// Get looks up a function by name. Returns the function and whether it exists.
func (r FuncRegistry) Get(name string) (Func, bool) {
	f, ok := r[name]
	return f, ok
}

// BuiltinFunctions returns the default set of built-in functions.
func BuiltinFunctions() FuncRegistry {
	r := make(FuncRegistry)

	// String functions
	r.Register(Func{Name: "lower", Call: fnLower})
	r.Register(Func{Name: "upper", Call: fnUpper})
	r.Register(Func{Name: "trim", Call: fnTrim})
	r.Register(Func{Name: "len", Call: fnLen})
	r.Register(Func{Name: "contains", Call: fnContains})
	r.Register(Func{Name: "startsWith", Call: fnStartsWith})
	r.Register(Func{Name: "endsWith", Call: fnEndsWith})

	// Numeric functions
	r.Register(Func{Name: "abs", Call: fnAbs})
	r.Register(Func{Name: "ceil", Call: fnCeil})
	r.Register(Func{Name: "floor", Call: fnFloor})
	r.Register(Func{Name: "round", Call: fnRound})
	r.Register(Func{Name: "sqrt", Call: fnSqrt})
	r.Register(Func{Name: "pow", Call: fnPow})
	r.Register(Func{Name: "min", Call: fnMin})
	r.Register(Func{Name: "max", Call: fnMax})

	// Date/time functions
	r.Register(Func{Name: "now", Call: fnNow})
	r.Register(Func{Name: "today", Call: fnToday})
	r.Register(Func{Name: "year", Call: fnYear})
	r.Register(Func{Name: "month", Call: fnMonth})
	r.Register(Func{Name: "day", Call: fnDay})
	r.Register(Func{Name: "hour", Call: fnHour})
	r.Register(Func{Name: "minute", Call: fnMinute})
	r.Register(Func{Name: "second", Call: fnSecond})
	r.Register(Func{Name: "weekday", Call: fnWeekday})
	r.Register(Func{Name: "daysAgo", Call: fnDaysAgo})
	r.Register(Func{Name: "addDays", Call: fnAddDays})

	// List aggregations — value-producing reductions over a slice.
	r.Register(Func{Name: "count", Call: fnCount})
	r.Register(Func{Name: "sum", Call: fnSum})
	r.Register(Func{Name: "avg", Call: fnAvg})
	r.Register(Func{Name: "first", Call: fnFirst})
	r.Register(Func{Name: "last", Call: fnLast})

	// Type coercions — evaluation-time coercion hints, not static casts.
	r.Register(Func{Name: "int", Call: fnInt})
	r.Register(Func{Name: "float", Call: fnFloat})
	r.Register(Func{Name: "string", Call: fnString})

	// Logical helpers — cover the ternary/nullish gap without new syntax.
	r.Register(Func{Name: "coalesce", Call: fnCoalesce})
	r.Register(Func{Name: "if", Call: fnIf})

	return r
}

// isIntKind reports whether v is an integer-kinded Go value. It operates on the
// raw argument values a [Func] receives (not [ast.Value]), and is the basis for
// the int64-vs-float64 result rule in abs/min/max/sum: a result stays int64
// only when every contributing operand is integer-kinded, mirroring how
// applyArith keeps integer arithmetic in int64 and promotes to float64 on any
// float operand.
func isIntKind(v any) bool {
	switch v.(type) {
	case int, int32, int64:
		return true
	default:
		return false
	}
}

// fnCoalesce returns the first argument that is non-nil and non-empty.
// Mirrors SQL COALESCE / expr-lang's `??`.
func fnCoalesce(args ...any) (any, error) {
	for _, a := range args {
		if a == nil {
			continue
		}
		if s, ok := a.(string); ok && s == "" {
			continue
		}
		return a, nil
	}
	return nil, nil
}

// fnIf returns args[1] when args[0] is truthy, args[2] otherwise. Two-arg
// form omits the else branch and returns nil. Covers the ternary `a ? b : c`
// gap without introducing dedicated syntax.
func fnIf(args ...any) (any, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("if() requires 2 or 3 arguments, got %d", len(args))
	}
	if toBool(args[0]) {
		return args[1], nil
	}
	if len(args) == 3 {
		return args[2], nil
	}
	return nil, nil
}

// --- String functions ---

func fnLower(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("lower() requires 1 argument, got %d", len(args))
	}
	return strings.ToLower(fmt.Sprint(args[0])), nil
}

func fnUpper(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("upper() requires 1 argument, got %d", len(args))
	}
	return strings.ToUpper(fmt.Sprint(args[0])), nil
}

func fnTrim(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("trim() requires 1 argument, got %d", len(args))
	}
	return strings.TrimSpace(fmt.Sprint(args[0])), nil
}

func fnLen(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("len() requires 1 argument, got %d", len(args))
	}
	// A list reports its element count; anything else reports the length of its
	// string form (so len() of a name is its character count).
	if elems, ok := toSlice(args[0]); ok {
		return int64(len(elems)), nil
	}
	return int64(len(fmt.Sprint(args[0]))), nil
}

// fnContains reports membership two ways. When the first argument is a list, it
// is true when any element equals the second argument (case-insensitive on the
// string form). Otherwise it falls back to a case-insensitive substring test on
// the first argument's string form. The needle is a plain value, not a pattern.
func fnContains(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("contains() requires 2 arguments, got %d", len(args))
	}
	if elems, ok := toSlice(args[0]); ok {
		needle := fmt.Sprint(args[1])
		for _, e := range elems {
			if strings.EqualFold(fmt.Sprint(e), needle) {
				return true, nil
			}
		}
		return false, nil
	}
	return strings.Contains(
		strings.ToLower(fmt.Sprint(args[0])),
		strings.ToLower(fmt.Sprint(args[1])),
	), nil
}

func fnStartsWith(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("startsWith() requires 2 arguments, got %d", len(args))
	}
	return strings.HasPrefix(
		strings.ToLower(fmt.Sprint(args[0])),
		strings.ToLower(fmt.Sprint(args[1])),
	), nil
}

func fnEndsWith(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("endsWith() requires 2 arguments, got %d", len(args))
	}
	return strings.HasSuffix(
		strings.ToLower(fmt.Sprint(args[0])),
		strings.ToLower(fmt.Sprint(args[1])),
	), nil
}

// --- Numeric functions ---

// fnAbs returns the absolute value of its argument, preserving kind: an
// integer-kinded input yields int64, anything else yields float64. A
// non-numeric input coerces to 0.
func fnAbs(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("abs() requires 1 argument, got %d", len(args))
	}
	switch args[0].(type) {
	case int, int32, int64:
		n := toInt64(args[0])
		if n < 0 {
			return -n, nil // abs(math.MinInt64) wraps, matching Go's two's complement
		}
		return n, nil
	case float32, float64:
		return math.Abs(toFloat64(args[0])), nil
	default:
		// Non-numeric input has no magnitude; report zero as an integer.
		return int64(0), nil
	}
}

// fnCeil returns the least integer value greater than or equal to its argument,
// as a float64. Wrap in int() to truncate to int64.
func fnCeil(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ceil() requires 1 argument, got %d", len(args))
	}
	return math.Ceil(toFloat64(args[0])), nil
}

// fnFloor returns the greatest integer value less than or equal to its
// argument, as a float64.
func fnFloor(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("floor() requires 1 argument, got %d", len(args))
	}
	return math.Floor(toFloat64(args[0])), nil
}

// fnRound rounds its argument to the nearest value, half away from zero, as a
// float64. The two-argument form rounds to the given number of decimal places
// (negative places round to tens, hundreds, …).
func fnRound(args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("round() requires 1 or 2 arguments, got %d", len(args))
	}
	x := toFloat64(args[0])
	if len(args) == 1 {
		return math.Round(x), nil
	}
	factor := math.Pow(10, float64(toInt64(args[1])))
	return math.Round(x*factor) / factor, nil
}

// fnSqrt returns the square root of its argument as a float64. A negative input
// has no real root and resolves to no value (nil), so the surrounding
// comparison is false, mirroring the division-by-zero contract in applyArith.
func fnSqrt(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sqrt() requires 1 argument, got %d", len(args))
	}
	r := math.Sqrt(toFloat64(args[0]))
	if math.IsNaN(r) {
		return nil, nil
	}
	return r, nil
}

// fnPow returns base raised to exp as a float64. A NaN or infinite result
// (e.g. a negative base with a fractional exponent, or overflow) resolves to no
// value so comparisons stay well defined.
func fnPow(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("pow() requires 2 arguments, got %d", len(args))
	}
	r := math.Pow(toFloat64(args[0]), toFloat64(args[1]))
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return nil, nil
	}
	return r, nil
}

// fnMin returns the smallest of its arguments. Called with a single list
// argument it reduces over the elements; otherwise it reduces over the variadic
// scalar arguments. The result stays int64 only when every operand is
// integer-kinded, else float64. An empty list resolves to no value.
func fnMin(args ...any) (any, error) {
	return reduceMinMax("min", args, true)
}

// fnMax returns the largest of its arguments, with the same variadic/list modes
// and int64-vs-float64 rule as fnMin.
func fnMax(args ...any) (any, error) {
	return reduceMinMax("max", args, false)
}

// reduceMinMax implements the shared min/max reduction. wantMin selects the
// comparison direction.
func reduceMinMax(name string, args []any, wantMin bool) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s() requires at least 1 argument, got 0", name)
	}
	operands := args
	if len(args) == 1 {
		if elems, ok := toSlice(args[0]); ok {
			if len(elems) == 0 {
				return nil, nil // min/max of an empty list is undefined
			}
			operands = elems
		}
	}
	allInt := true
	best := numericFloatAny(operands[0])
	bestIdx := 0
	for i, op := range operands {
		if !isIntKind(op) {
			allInt = false
		}
		f := numericFloatAny(op)
		if (wantMin && f < best) || (!wantMin && f > best) {
			best = f
			bestIdx = i
		}
	}
	if allInt {
		return toInt64(operands[bestIdx]), nil
	}
	return best, nil
}

// numericFloatAny coerces a raw argument value to float64 for numeric reduction.
func numericFloatAny(v any) float64 {
	return toFloat64(v)
}

// --- Date/time functions ---

func fnNow(args ...any) (any, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("now() takes no arguments, got %d", len(args))
	}
	return time.Now(), nil
}

func fnToday(args ...any) (any, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("today() takes no arguments, got %d", len(args))
	}
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
}

func fnYear(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("year() requires 1 argument, got %d", len(args))
	}
	t := toTime(args[0])
	return int64(t.Year()), nil
}

func fnMonth(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("month() requires 1 argument, got %d", len(args))
	}
	t := toTime(args[0])
	return int64(t.Month()), nil
}

func fnDay(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("day() requires 1 argument, got %d", len(args))
	}
	t := toTime(args[0])
	return int64(t.Day()), nil
}

func fnDaysAgo(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("daysAgo() requires 1 argument, got %d", len(args))
	}
	n := toInt64(args[0])
	return time.Now().AddDate(0, 0, -int(n)), nil
}

// fnHour extracts the hour of day (0–23) from a date/time argument. With now()
// it lets a query express a time-of-day predicate, e.g. hour(now())>=9.
func fnHour(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("hour() requires 1 argument, got %d", len(args))
	}
	return int64(toTime(args[0]).Hour()), nil
}

// fnMinute extracts the minute of the hour (0–59) from a date/time argument.
func fnMinute(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("minute() requires 1 argument, got %d", len(args))
	}
	return int64(toTime(args[0]).Minute()), nil
}

// fnSecond extracts the second of the minute (0–59) from a date/time argument.
func fnSecond(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("second() requires 1 argument, got %d", len(args))
	}
	return int64(toTime(args[0]).Second()), nil
}

// fnWeekday returns the day of the week as an integer, Sunday=0 through
// Saturday=6 (Go's time.Weekday numbering, which matches cron). Combined with
// now() it expresses day-of-week recurrence, e.g. weekday(now())>=1 AND
// weekday(now())<=5 for weekdays. Note the zero time (an unparseable argument)
// is a Monday and so returns 1, unlike hour/minute/second which return 0.
func fnWeekday(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("weekday() requires 1 argument, got %d", len(args))
	}
	return int64(toTime(args[0]).Weekday()), nil
}

// fnAddDays returns the date n calendar days after the given date (negative n
// for earlier). Calendar arithmetic is month-length and DST aware, which
// duration addition cannot express. addDays(now(), 7) is one week from now.
func fnAddDays(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("addDays() requires 2 arguments, got %d", len(args))
	}
	return toTime(args[0]).AddDate(0, 0, int(toInt64(args[1]))), nil
}

// --- List aggregations ---

// fnCount returns the number of elements in a list. A nil argument is 0; a
// non-list scalar counts as a single-element collection (1). For string length
// use len().
func fnCount(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("count() requires 1 argument, got %d", len(args))
	}
	if elems, ok := toSlice(args[0]); ok {
		return int64(len(elems)), nil
	}
	if args[0] == nil {
		return int64(0), nil
	}
	return int64(1), nil
}

// fnSum returns the numeric total of a list's elements. The result stays int64
// only when every element is integer-kinded, else float64. An empty list sums
// to 0 (the additive identity).
func fnSum(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sum() requires 1 argument, got %d", len(args))
	}
	elems, ok := toSlice(args[0])
	if !ok {
		return nil, fmt.Errorf("sum() requires a list argument")
	}
	allInt := true
	var total float64
	for _, e := range elems {
		if !isIntKind(e) {
			allInt = false
		}
		total += toFloat64(e)
	}
	if allInt {
		return int64(total), nil
	}
	return total, nil
}

// fnAvg returns the arithmetic mean of a list's elements as a float64. The mean
// of an empty list is undefined and resolves to no value.
func fnAvg(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("avg() requires 1 argument, got %d", len(args))
	}
	elems, ok := toSlice(args[0])
	if !ok {
		return nil, fmt.Errorf("avg() requires a list argument")
	}
	if len(elems) == 0 {
		return nil, nil
	}
	var total float64
	for _, e := range elems {
		total += toFloat64(e)
	}
	return total / float64(len(elems)), nil
}

// fnFirst returns the first element of a list, or no value if the list is empty.
// This is distinct from the @first selector, which tests list non-emptiness.
func fnFirst(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("first() requires 1 argument, got %d", len(args))
	}
	elems, ok := toSlice(args[0])
	if !ok || len(elems) == 0 {
		return nil, nil
	}
	return elems[0], nil
}

// fnLast returns the last element of a list, or no value if the list is empty.
func fnLast(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("last() requires 1 argument, got %d", len(args))
	}
	elems, ok := toSlice(args[0])
	if !ok || len(elems) == 0 {
		return nil, nil
	}
	return elems[len(elems)-1], nil
}

// --- Type coercions ---
//
// int, float, and string are evaluation-time coercion hints, not static casts:
// the validator treats every function result as dynamically typed, so these
// coerce at match time and resolve to no value when a string cannot be parsed.

// fnInt coerces its argument to an int64. Numbers truncate toward zero; strings
// parse as base-10 integers, falling back to a float parse then truncation;
// booleans map to 1/0. An unparseable value resolves to no value.
func fnInt(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("int() requires 1 argument, got %d", len(args))
	}
	switch v := args[0].(type) {
	case bool:
		if v {
			return int64(1), nil
		}
		return int64(0), nil
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n, nil
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return int64(f), nil
		}
		return nil, nil
	case nil:
		return nil, nil
	default:
		return toInt64(v), nil
	}
}

// fnFloat coerces its argument to a float64. Strings parse as floats; booleans
// map to 1.0/0.0. An unparseable value resolves to no value.
func fnFloat(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("float() requires 1 argument, got %d", len(args))
	}
	switch v := args[0].(type) {
	case bool:
		if v {
			return 1.0, nil
		}
		return 0.0, nil
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f, nil
		}
		return nil, nil
	case nil:
		return nil, nil
	default:
		return toFloat64(v), nil
	}
}

// fnString coerces its argument to its string form. Dates render as RFC3339 and
// durations via their String() form, matching how the engine stringifies value
// literals so results round-trip. A nil argument is the empty string.
func fnString(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("string() requires 1 argument, got %d", len(args))
	}
	switch v := args[0].(type) {
	case time.Time:
		return v.Format(time.RFC3339), nil
	case time.Duration:
		return v.String(), nil
	case nil:
		return "", nil
	default:
		return fmt.Sprint(v), nil
	}
}
