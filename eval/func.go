package eval

import (
	"fmt"
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

	// Date/time functions
	r.Register(Func{Name: "now", Call: fnNow})
	r.Register(Func{Name: "today", Call: fnToday})
	r.Register(Func{Name: "year", Call: fnYear})
	r.Register(Func{Name: "month", Call: fnMonth})
	r.Register(Func{Name: "day", Call: fnDay})
	r.Register(Func{Name: "daysAgo", Call: fnDaysAgo})

	// Logical helpers — cover the ternary/nullish gap without new syntax.
	r.Register(Func{Name: "coalesce", Call: fnCoalesce})
	r.Register(Func{Name: "if", Call: fnIf})

	return r
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

func fnContains(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("contains() requires 2 arguments, got %d", len(args))
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
