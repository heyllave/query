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
	// Arity bounds the argument count, checked at compile time so a call with
	// the wrong number of arguments is a compile error rather than a predicate
	// that is silently false. The zero value leaves the count unchecked.
	Arity Arity
}

// Arity is the argument-count contract of a [Func]. Build one with
// [Exactly], [AtLeast] or [Between]; the zero value means unchecked.
type Arity struct {
	min, max int
	checked  bool
}

// Exactly is an arity of exactly n arguments.
func Exactly(n int) Arity { return Arity{min: n, max: n, checked: true} }

// AtLeast is an arity of n or more arguments.
func AtLeast(n int) Arity { return Arity{min: n, max: -1, checked: true} }

// Between is an arity of lo to hi arguments inclusive.
func Between(lo, hi int) Arity { return Arity{min: lo, max: hi, checked: true} }

// Check returns an error describing the mismatch when n arguments do not
// satisfy the arity, or nil when they do or the arity is unchecked.
func (a Arity) Check(name string, n int) error {
	if !a.checked || (n >= a.min && (a.max < 0 || n <= a.max)) {
		return nil
	}
	switch {
	case a.max < 0:
		return fmt.Errorf("%s() requires at least %d argument%s, got %d", name, a.min, plural(a.min), n)
	case a.min == a.max:
		return fmt.Errorf("%s() requires %d argument%s, got %d", name, a.min, plural(a.min), n)
	default:
		return fmt.Errorf("%s() requires %d to %d arguments, got %d", name, a.min, a.max, n)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
	r.Register(Func{Name: "lower", Call: fnLower, Arity: Exactly(1)})
	r.Register(Func{Name: "upper", Call: fnUpper, Arity: Exactly(1)})
	r.Register(Func{Name: "trim", Call: fnTrim, Arity: Exactly(1)})
	r.Register(Func{Name: "len", Call: fnLen, Arity: Exactly(1)})
	r.Register(Func{Name: "contains", Call: fnContains, Arity: Exactly(2)})
	r.Register(Func{Name: "startsWith", Call: fnStartsWith, Arity: Exactly(2)})
	r.Register(Func{Name: "endsWith", Call: fnEndsWith, Arity: Exactly(2)})

	// Numeric functions
	r.Register(Func{Name: "abs", Call: fnAbs, Arity: Exactly(1)})
	r.Register(Func{Name: "ceil", Call: fnCeil, Arity: Exactly(1)})
	r.Register(Func{Name: "floor", Call: fnFloor, Arity: Exactly(1)})
	r.Register(Func{Name: "round", Call: fnRound, Arity: Between(1, 2)})
	r.Register(Func{Name: "sqrt", Call: fnSqrt, Arity: Exactly(1)})
	r.Register(Func{Name: "pow", Call: fnPow, Arity: Exactly(2)})
	r.Register(Func{Name: "min", Call: fnMin, Arity: AtLeast(1)})
	r.Register(Func{Name: "max", Call: fnMax, Arity: AtLeast(1)})

	// Date/time functions
	r.Register(Func{Name: "now", Call: fnNow, Arity: Exactly(0)})
	r.Register(Func{Name: "today", Call: fnToday, Arity: Exactly(0)})
	r.Register(Func{Name: "year", Call: fnYear, Arity: Exactly(1)})
	r.Register(Func{Name: "month", Call: fnMonth, Arity: Exactly(1)})
	r.Register(Func{Name: "day", Call: fnDay, Arity: Exactly(1)})
	r.Register(Func{Name: "hour", Call: fnHour, Arity: Exactly(1)})
	r.Register(Func{Name: "minute", Call: fnMinute, Arity: Exactly(1)})
	r.Register(Func{Name: "second", Call: fnSecond, Arity: Exactly(1)})
	r.Register(Func{Name: "weekday", Call: fnWeekday, Arity: Exactly(1)})
	r.Register(Func{Name: "isBusinessDay", Call: fnIsBusinessDay, Arity: Between(1, 2)})
	r.Register(Func{Name: "daysAgo", Call: fnDaysAgo, Arity: Exactly(1)})
	r.Register(Func{Name: "addDays", Call: fnAddDays, Arity: Exactly(2)})
	r.Register(Func{Name: "addBusinessDays", Call: fnAddBusinessDays, Arity: Between(2, 3)})

	// List aggregations — value-producing reductions over a slice.
	r.Register(Func{Name: "count", Call: fnCount, Arity: Exactly(1)})
	r.Register(Func{Name: "sum", Call: fnSum, Arity: Exactly(1)})
	r.Register(Func{Name: "avg", Call: fnAvg, Arity: Exactly(1)})
	r.Register(Func{Name: "first", Call: fnFirst, Arity: Exactly(1)})
	r.Register(Func{Name: "last", Call: fnLast, Arity: Exactly(1)})

	// Type coercions — evaluation-time coercion hints, not static casts.
	r.Register(Func{Name: "int", Call: fnInt, Arity: Exactly(1)})
	r.Register(Func{Name: "float", Call: fnFloat, Arity: Exactly(1)})
	r.Register(Func{Name: "string", Call: fnString, Arity: Exactly(1)})

	// Logical helpers — cover the ternary/nullish gap without new syntax.
	r.Register(Func{Name: "coalesce", Call: fnCoalesce, Arity: AtLeast(1)})
	r.Register(Func{Name: "if", Call: fnIf, Arity: Between(2, 3)})

	return r
}

// isIntKind reports whether v is an integer-kinded Go value (any signed or
// unsigned width). It operates on the raw argument values a [Func] receives
// (not [ast.Value]), and is the basis for the int64-vs-float64 result rule in
// abs/min/max/sum: a result stays int64 only when every contributing operand
// is integer-kinded, mirroring how applyArith keeps integer arithmetic in
// int64 and promotes to float64 on any float operand.
func isIntKind(v any) bool {
	_, _, isInt, ok := numberOf(v)
	return ok && isInt
}

// text returns the string form of a function argument, reporting false for a
// nil argument (a missing field) so string functions resolve to no value
// instead of operating on the text "<nil>".
func text(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	return stringifyActual(v), true
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

// String functions resolve to no value (nil) when their subject is nil, which
// is what a missing field resolves to: lower(name) on a record without a name
// is not the text "<nil>", so len(name)>3 does not match such a record.

func fnLower(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("lower() requires 1 argument, got %d", len(args))
	}
	s, ok := text(args[0])
	if !ok {
		return nil, nil
	}
	return strings.ToLower(s), nil
}

func fnUpper(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("upper() requires 1 argument, got %d", len(args))
	}
	s, ok := text(args[0])
	if !ok {
		return nil, nil
	}
	return strings.ToUpper(s), nil
}

func fnTrim(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("trim() requires 1 argument, got %d", len(args))
	}
	s, ok := text(args[0])
	if !ok {
		return nil, nil
	}
	return strings.TrimSpace(s), nil
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
	s, ok := text(args[0])
	if !ok {
		return nil, nil
	}
	return int64(len(s)), nil
}

// fnContains reports membership two ways. When the first argument is a list, it
// is true when any element equals the second argument (case-insensitive on the
// string form). Otherwise it falls back to a case-insensitive substring test on
// the first argument's string form. The needle is a plain value, not a pattern.
func fnContains(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("contains() requires 2 arguments, got %d", len(args))
	}
	needle, ok := text(args[1])
	if !ok {
		return nil, nil
	}
	if elems, isList := toSlice(args[0]); isList {
		for _, e := range elems {
			if strings.EqualFold(stringifyActual(e), needle) {
				return true, nil
			}
		}
		return false, nil
	}
	hay, ok := text(args[0])
	if !ok {
		return nil, nil
	}
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle)), nil
}

func fnStartsWith(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("startsWith() requires 2 arguments, got %d", len(args))
	}
	hay, ok1 := text(args[0])
	prefix, ok2 := text(args[1])
	if !ok1 || !ok2 {
		return nil, nil
	}
	return strings.HasPrefix(strings.ToLower(hay), strings.ToLower(prefix)), nil
}

func fnEndsWith(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("endsWith() requires 2 arguments, got %d", len(args))
	}
	hay, ok1 := text(args[0])
	suffix, ok2 := text(args[1])
	if !ok1 || !ok2 {
		return nil, nil
	}
	return strings.HasSuffix(strings.ToLower(hay), strings.ToLower(suffix)), nil
}

// --- Numeric functions ---

// fnAbs returns the absolute value of its argument, preserving kind: an
// integer-kinded input yields int64, anything else yields float64. A
// non-numeric input coerces to 0.
func fnAbs(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("abs() requires 1 argument, got %d", len(args))
	}
	i, f, isInt, ok := numberOf(args[0])
	switch {
	case !ok:
		// Non-numeric input has no magnitude; report zero as an integer.
		return int64(0), nil
	case isInt:
		if i < 0 {
			return -i, nil // abs(math.MinInt64) wraps, matching Go's two's complement
		}
		return i, nil
	default:
		return math.Abs(f), nil
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
	r := math.Round(x*factor) / factor
	if math.IsNaN(r) || math.IsInf(r, 0) {
		// An absurd number of places overflows the scale factor; there is no
		// meaningful result, so resolve to no value rather than NaN.
		return nil, nil
	}
	return r, nil
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
// comparison direction. All operands must be numeric; a non-numeric operand is
// an error rather than a silent zero. When every operand is integer-kinded the
// comparison and result are int64 (exact beyond 2^53); otherwise it is float64.
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
	for _, op := range operands {
		if !isNumericKind(op) {
			return nil, fmt.Errorf("%s() requires numeric operands, got %T", name, op)
		}
		if !isIntKind(op) {
			allInt = false
		}
	}
	if allInt {
		best := toInt64(operands[0])
		for _, op := range operands[1:] {
			v := toInt64(op)
			if (wantMin && v < best) || (!wantMin && v > best) {
				best = v
			}
		}
		return best, nil
	}
	best := toFloat64(operands[0])
	for _, op := range operands[1:] {
		v := toFloat64(op)
		if (wantMin && v < best) || (!wantMin && v > best) {
			best = v
		}
	}
	return best, nil
}

// isNumericKind reports whether v is an integer- or float-kinded Go value.
func isNumericKind(v any) bool {
	_, _, _, ok := numberOf(v)
	return ok
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

// Date component functions resolve to no value when the argument is nil or
// not a date, rather than reporting the components of the zero time.

func fnYear(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("year() requires 1 argument, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return int64(t.Year()), nil
}

func fnMonth(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("month() requires 1 argument, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return int64(t.Month()), nil
}

func fnDay(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("day() requires 1 argument, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return int64(t.Day()), nil
}

func fnDaysAgo(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("daysAgo() requires 1 argument, got %d", len(args))
	}
	n, ok := dayCount(args[0])
	if !ok {
		return nil, nil
	}
	return time.Now().AddDate(0, 0, -n), nil
}

// fnHour extracts the hour of day (0–23) from a date/time argument. With now()
// it lets a query express a time-of-day predicate, e.g. hour(now())>=9.
func fnHour(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("hour() requires 1 argument, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return int64(t.Hour()), nil
}

// fnMinute extracts the minute of the hour (0–59) from a date/time argument.
func fnMinute(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("minute() requires 1 argument, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return int64(t.Minute()), nil
}

// fnSecond extracts the second of the minute (0–59) from a date/time argument.
func fnSecond(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("second() requires 1 argument, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return int64(t.Second()), nil
}

// fnWeekday returns the day of the week as an integer, Sunday=0 through
// Saturday=6 (Go's time.Weekday numbering, which matches cron). Combined with
// now() it expresses day-of-week recurrence, e.g. weekday(now())>=1 AND
// weekday(now())<=5 for weekdays.
func fnWeekday(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("weekday() requires 1 argument, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return int64(t.Weekday()), nil
}

// MaxCalendarDays bounds the day count accepted by daysAgo, addDays and
// addBusinessDays: about 10,000 years, far beyond any real schedule, and small
// enough that the business-day walk cannot be turned into a CPU sink by a
// large literal. A larger count resolves to no value.
const MaxCalendarDays = 4_000_000

// dayCount coerces a day-count argument, reporting false when it is not a
// number or lies outside ±MaxCalendarDays.
func dayCount(v any) (int, bool) {
	i, f, isInt, ok := numberOf(v)
	if !ok {
		return 0, false
	}
	if !isInt {
		if math.IsNaN(f) || f > MaxCalendarDays || f < -MaxCalendarDays {
			return 0, false
		}
		i = int64(f)
	}
	if i > MaxCalendarDays || i < -MaxCalendarDays {
		return 0, false
	}
	return int(i), true
}

// fnAddDays returns the date n calendar days after the given date (negative n
// for earlier). Calendar arithmetic is month-length and DST aware, which
// duration addition cannot express. addDays(now(), 7) is one week from now.
func fnAddDays(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("addDays() requires 2 arguments, got %d", len(args))
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	n, ok := dayCount(args[1])
	if !ok {
		return nil, nil
	}
	return t.AddDate(0, 0, n), nil
}

// fnIsBusinessDay reports whether a date is a working day: a weekday
// (Monday–Friday) that is not a public holiday. An optional second argument is
// the holiday calendar — a list of dates; any date matching one (by calendar
// day) is not a business day. Holidays are calendar- and locale-specific, so
// the caller supplies them rather than the library guessing:
//
//	isBusinessDay(due)                 // weekends only
//	isBusinessDay(due, holidays())     // weekends + a supplied holiday list
func fnIsBusinessDay(args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("isBusinessDay() requires 1 or 2 arguments, got %d", len(args))
	}
	var holidays map[string]bool
	if len(args) == 2 {
		holidays = holidaySet(args[1])
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	return isBusinessDay(t, holidays), nil
}

// fnAddBusinessDays returns the date n business days after the given date,
// skipping weekends and any supplied holidays (negative n moves earlier). With
// n=0 the date is returned unchanged even if it is not a business day. An
// optional third argument is the holiday calendar, as for isBusinessDay:
//
//	addBusinessDays(start, 3)              // skip weekends
//	addBusinessDays(start, 3, holidays())  // skip weekends + holidays
//
// The walk is linear in n, so n is bounded by MaxCalendarDays.
func fnAddBusinessDays(args ...any) (any, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("addBusinessDays() requires 2 or 3 arguments, got %d", len(args))
	}
	var holidays map[string]bool
	if len(args) == 3 {
		holidays = holidaySet(args[2])
	}
	t, ok := toTime(args[0])
	if !ok {
		return nil, nil
	}
	n, ok := dayCount(args[1])
	if !ok {
		return nil, nil
	}
	step := 1
	if n < 0 {
		step = -1
		n = -n
	}
	for n > 0 {
		t = t.AddDate(0, 0, step)
		if isBusinessDay(t, holidays) {
			n--
		}
	}
	return t, nil
}

// isBusinessDay reports whether t is a weekday (Monday–Friday) that is not in
// the holidays set. A nil holidays set means weekends only.
func isBusinessDay(t time.Time, holidays map[string]bool) bool {
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	return !holidays[dayKey(t)]
}

// holidaySet builds a lookup of holiday calendar days from a list argument. Each
// element is coerced to a date and keyed by its calendar day, so the time of day
// and location do not affect matching. A non-list argument yields an empty set.
func holidaySet(v any) map[string]bool {
	elems, ok := toSlice(v)
	if !ok {
		return nil
	}
	set := make(map[string]bool, len(elems))
	for _, e := range elems {
		if t, ok := toTime(e); ok {
			set[dayKey(t)] = true
		}
	}
	return set
}

// dayKey is the calendar-day identity of t (YYYY-MM-DD), used to compare dates
// while ignoring time of day.
func dayKey(t time.Time) string {
	return t.Format("2006-01-02")
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

// fnSum returns the numeric total of a list's elements. A non-list argument is
// treated as a single-element collection (sum(7)=7). The result stays int64
// only when every element is integer-kinded — accumulated in int64 so it is
// exact beyond 2^53 — else float64. An empty or nil list sums to 0 (the
// additive identity).
func fnSum(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sum() requires 1 argument, got %d", len(args))
	}
	elems := listElems(args[0])
	allInt := true
	var intTotal int64
	var floatTotal float64
	for _, e := range elems {
		if isIntKind(e) {
			intTotal += toInt64(e)
		} else {
			allInt = false
		}
		floatTotal += toFloat64(e)
	}
	if allInt {
		return intTotal, nil
	}
	return floatTotal, nil
}

// fnAvg returns the arithmetic mean of a list's elements as a float64. A
// non-list argument is treated as a single-element collection. The mean of an
// empty or nil list is undefined and resolves to no value.
func fnAvg(args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("avg() requires 1 argument, got %d", len(args))
	}
	elems := listElems(args[0])
	if len(elems) == 0 {
		return nil, nil
	}
	var total float64
	for _, e := range elems {
		total += toFloat64(e)
	}
	return total / float64(len(elems)), nil
}

// listElems normalizes an aggregation argument to a slice of elements: a list
// is its elements, a nil argument is empty, and any other scalar is a
// one-element collection. This keeps sum/avg consistent with count's graceful
// handling of non-list arguments.
func listElems(v any) []any {
	if elems, ok := toSlice(v); ok {
		return elems
	}
	if v == nil {
		return nil
	}
	return []any{v}
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
		s := strings.TrimSpace(v)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, nil
		}
		// Fall back to a float parse, but only a finite value in int64 range
		// truncates to an integer; NaN/Inf/out-of-range resolve to no value.
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) || f >= math.MaxInt64 || f <= math.MinInt64 {
				return nil, nil
			}
			return int64(f), nil
		}
		return nil, nil
	case nil:
		return nil, nil
	default:
		i, f, isInt, ok := numberOf(v)
		if !ok {
			return nil, nil
		}
		if !isInt {
			if math.IsNaN(f) || math.IsInf(f, 0) || f >= math.MaxInt64 || f <= math.MinInt64 {
				return nil, nil
			}
			return int64(f), nil
		}
		return i, nil
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
		// A finite float parses; NaN and Inf strings resolve to no value so the
		// result is always a comparable number.
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return nil, nil
			}
			return f, nil
		}
		return nil, nil
	case nil:
		return nil, nil
	default:
		_, f, _, ok := numberOf(v)
		if !ok {
			return nil, nil
		}
		return f, nil
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
		return stringifyActual(v), nil
	}
}
