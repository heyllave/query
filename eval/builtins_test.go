package eval

import (
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// callFn looks up a built-in by name and invokes it with the given args.
func callFn(t *testing.T, name string, args ...any) (any, error) {
	t.Helper()
	fn, ok := BuiltinFunctions().Get(name)
	if !ok {
		t.Fatalf("built-in %q not registered", name)
	}
	return fn.Call(args...)
}

func TestBuiltinNumeric(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		args []any
		want any
	}{
		{"abs negative int preserves int64", "abs", []any{int64(-5)}, int64(5)},
		{"abs positive int", "abs", []any{int64(5)}, int64(5)},
		{"abs negative float", "abs", []any{-5.5}, 5.5},
		{"abs non-numeric is zero", "abs", []any{"x"}, int64(0)},
		{"ceil", "ceil", []any{4.2}, 5.0},
		{"ceil of int", "ceil", []any{int64(4)}, 4.0},
		{"floor", "floor", []any{4.8}, 4.0},
		{"round half away from zero", "round", []any{2.5}, 3.0},
		{"round negative half away", "round", []any{-2.5}, -3.0},
		{"round to 2 places", "round", []any{3.14159, int64(2)}, 3.14},
		{"round to tens", "round", []any{1234.0, int64(-1)}, 1230.0},
		{"sqrt", "sqrt", []any{9.0}, 3.0},
		{"pow", "pow", []any{2.0, 10.0}, 1024.0},
		{"min variadic ints", "min", []any{int64(5), int64(3), int64(8)}, int64(3)},
		{"max variadic ints", "max", []any{int64(5), int64(3), int64(8)}, int64(8)},
		{"min mixed promotes to float", "min", []any{int64(5), 3.5}, 3.5},
		{"max single arg", "max", []any{int64(7)}, int64(7)},
		{"min over list", "min", []any{[]any{int64(3), int64(1), int64(2)}}, int64(1)},
		{"max over float list", "max", []any{[]any{1.5, 2.5}}, 2.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := callFn(t, tt.fn, tt.args...)
			if err != nil {
				t.Fatalf("%s() error: %v", tt.fn, err)
			}
			if got != tt.want {
				t.Errorf("%s(%v) = %v (%T), want %v (%T)", tt.fn, tt.args, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestBuiltinNumeric_NoValue covers the operands that have no defined numeric
// result and must resolve to nil so the surrounding comparison stays false.
func TestBuiltinNumeric_NoValue(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		args []any
	}{
		{"sqrt of negative", "sqrt", []any{-1.0}},
		{"pow NaN", "pow", []any{-2.0, 0.5}},
		{"pow overflow to inf", "pow", []any{10.0, 400.0}},
		{"min of empty list", "min", []any{[]any{}}},
		{"max of empty list", "max", []any{[]any{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := callFn(t, tt.fn, tt.args...)
			if err != nil {
				t.Fatalf("%s() error: %v", tt.fn, err)
			}
			if got != nil {
				t.Errorf("%s(%v) = %v, want nil", tt.fn, tt.args, got)
			}
		})
	}
}

func TestBuiltinNumeric_Arity(t *testing.T) {
	bad := []struct {
		fn   string
		args []any
	}{
		{"abs", []any{}},
		{"abs", []any{1, 2}},
		{"ceil", []any{}},
		{"floor", []any{1, 2}},
		{"round", []any{}},
		{"round", []any{1, 2, 3}},
		{"sqrt", []any{}},
		{"pow", []any{1}},
		{"min", []any{}},
		{"max", []any{}},
	}
	for _, tt := range bad {
		t.Run(tt.fn, func(t *testing.T) {
			if _, err := callFn(t, tt.fn, tt.args...); err == nil {
				t.Errorf("%s(%v) error = nil, want arity error", tt.fn, tt.args)
			}
		})
	}
}

func TestBuiltinDatetime(t *testing.T) {
	// 2026-03-15 was a Sunday at 14:30:45.
	ts := time.Date(2026, 3, 15, 14, 30, 45, 0, time.UTC)
	tests := []struct {
		name string
		fn   string
		args []any
		want any
	}{
		{"hour", "hour", []any{ts}, int64(14)},
		{"minute", "minute", []any{ts}, int64(30)},
		{"second", "second", []any{ts}, int64(45)},
		{"weekday Sunday is 0", "weekday", []any{ts}, int64(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := callFn(t, tt.fn, tt.args...)
			if err != nil {
				t.Fatalf("%s() error: %v", tt.fn, err)
			}
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.fn, got, tt.want)
			}
		})
	}

	t.Run("weekday spans the week Sunday=0..Saturday=6", func(t *testing.T) {
		// 2026-03-15 is Sunday; the next six days walk the week.
		for i := 0; i < 7; i++ {
			day := ts.AddDate(0, 0, i)
			got, _ := callFn(t, "weekday", day)
			if got != int64(i) {
				t.Errorf("weekday(%s) = %v, want %d", day.Format("Mon"), got, i)
			}
		}
	})

	t.Run("addDays is calendar arithmetic", func(t *testing.T) {
		got, err := callFn(t, "addDays", ts, int64(20)) // crosses month boundary
		if err != nil {
			t.Fatalf("addDays error: %v", err)
		}
		gt, ok := got.(time.Time)
		if !ok {
			t.Fatalf("addDays returned %T, want time.Time", got)
		}
		want := time.Date(2026, 4, 4, 14, 30, 45, 0, time.UTC)
		if !gt.Equal(want) {
			t.Errorf("addDays(+20) = %v, want %v", gt, want)
		}
	})

	t.Run("addDays negative goes earlier", func(t *testing.T) {
		got, _ := callFn(t, "addDays", ts, int64(-15))
		want := time.Date(2026, 2, 28, 14, 30, 45, 0, time.UTC)
		if gt := got.(time.Time); !gt.Equal(want) {
			t.Errorf("addDays(-15) = %v, want %v", gt, want)
		}
	})
}

func TestBuiltinBusinessDay(t *testing.T) {
	// 2026-03-13 Fri, 03-14 Sat, 03-15 Sun, 03-16 Mon.
	fri := time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC)
	sat := time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)
	sun := time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)
	mon := time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC)

	t.Run("isBusinessDay", func(t *testing.T) {
		cases := []struct {
			day  time.Time
			want bool
		}{{fri, true}, {sat, false}, {sun, false}, {mon, true}}
		for _, c := range cases {
			got, err := callFn(t, "isBusinessDay", c.day)
			if err != nil {
				t.Fatalf("isBusinessDay error: %v", err)
			}
			if got != c.want {
				t.Errorf("isBusinessDay(%s) = %v, want %v", c.day.Format("Mon"), got, c.want)
			}
		}
	})

	t.Run("addBusinessDays skips the weekend", func(t *testing.T) {
		// Friday + 1 business day is the following Monday.
		got, err := callFn(t, "addBusinessDays", fri, int64(1))
		if err != nil {
			t.Fatalf("addBusinessDays error: %v", err)
		}
		want := time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC)
		if gt := got.(time.Time); !gt.Equal(want) {
			t.Errorf("addBusinessDays(Fri, 1) = %v, want Mon %v", gt, want)
		}
	})

	t.Run("addBusinessDays across more than a week", func(t *testing.T) {
		// Monday + 5 business days is the next Monday.
		got, _ := callFn(t, "addBusinessDays", mon, int64(5))
		want := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
		if gt := got.(time.Time); !gt.Equal(want) {
			t.Errorf("addBusinessDays(Mon, 5) = %v, want %v", gt, want)
		}
	})

	t.Run("addBusinessDays negative goes earlier", func(t *testing.T) {
		// Monday - 1 business day is the previous Friday.
		got, _ := callFn(t, "addBusinessDays", mon, int64(-1))
		want := time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC)
		if gt := got.(time.Time); !gt.Equal(want) {
			t.Errorf("addBusinessDays(Mon, -1) = %v, want Fri %v", gt, want)
		}
	})

	t.Run("addBusinessDays zero is unchanged", func(t *testing.T) {
		got, _ := callFn(t, "addBusinessDays", sat, int64(0))
		if gt := got.(time.Time); !gt.Equal(sat) {
			t.Errorf("addBusinessDays(Sat, 0) = %v, want %v", gt, sat)
		}
	})

	t.Run("isBusinessDay honors a holiday list", func(t *testing.T) {
		holidays := []any{mon} // make the Monday a holiday
		got, err := callFn(t, "isBusinessDay", mon, holidays)
		if err != nil {
			t.Fatalf("isBusinessDay error: %v", err)
		}
		if got != false {
			t.Errorf("isBusinessDay(Mon, [Mon]) = %v, want false (holiday)", got)
		}
		// A non-holiday weekday is still a business day.
		if got, _ := callFn(t, "isBusinessDay", fri, holidays); got != true {
			t.Errorf("isBusinessDay(Fri, [Mon]) = %v, want true", got)
		}
	})

	t.Run("addBusinessDays skips holidays", func(t *testing.T) {
		// Friday + 1 business day would be Monday, but Monday is a holiday, so
		// the result is the following Tuesday.
		holidays := []any{mon}
		got, err := callFn(t, "addBusinessDays", fri, int64(1), holidays)
		if err != nil {
			t.Fatalf("addBusinessDays error: %v", err)
		}
		want := time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC) // Tuesday
		if gt := got.(time.Time); !gt.Equal(want) {
			t.Errorf("addBusinessDays(Fri, 1, [Mon]) = %v, want Tue %v", gt, want)
		}
	})

	t.Run("arity errors", func(t *testing.T) {
		if _, err := callFn(t, "isBusinessDay"); err == nil {
			t.Error("isBusinessDay() with no args: want error")
		}
		if _, err := callFn(t, "isBusinessDay", fri, []any{}, "extra"); err == nil {
			t.Error("isBusinessDay() with 3 args: want error")
		}
		if _, err := callFn(t, "addBusinessDays", fri); err == nil {
			t.Error("addBusinessDays(1 arg): want error")
		}
	})
}

// TestStringOfTimeMatchesTimeField guards that string(timeExpr) on the RHS
// compares equal to a time-typed field, i.e. equalValues stringifies a
// time.Time actual via the same RFC3339 form string() produces.
func TestStringOfTimeMatchesTimeField(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "created", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	ts := time.Date(2026, 6, 8, 14, 30, 0, 0, time.UTC)
	clock := WithFunctions(Func{Name: "clock", Call: func(...any) (any, error) { return ts, nil }})
	prog, err := Compile("created=string(clock())", fields, clock)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	// A time.Time field value must match string(clock())'s RFC3339 rendering.
	if !prog.Match(map[string]any{"created": ts}) {
		t.Error("created=string(clock()) over a time.Time field = false, want true")
	}
}

func TestBuiltinList(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		args []any
		want any
	}{
		{"count list", "count", []any{[]any{1, 2, 3}}, int64(3)},
		{"count nil is zero", "count", []any{nil}, int64(0)},
		{"count scalar is one", "count", []any{"x"}, int64(1)},
		{"count empty list", "count", []any{[]any{}}, int64(0)},
		{"sum ints stays int64", "sum", []any{[]any{int64(1), int64(2), int64(3)}}, int64(6)},
		{"sum with float promotes", "sum", []any{[]any{int64(1), 2.5}}, 3.5},
		{"sum empty is zero", "sum", []any{[]any{}}, int64(0)},
		{"avg is float", "avg", []any{[]any{int64(2), int64(4)}}, 3.0},
		{"first", "first", []any{[]any{"a", "b"}}, "a"},
		{"last", "last", []any{[]any{"a", "b"}}, "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := callFn(t, tt.fn, tt.args...)
			if err != nil {
				t.Fatalf("%s() error: %v", tt.fn, err)
			}
			if got != tt.want {
				t.Errorf("%s(%v) = %v (%T), want %v (%T)", tt.fn, tt.args, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestBuiltinList_NoValue covers list reductions over an empty list that have
// no defined result.
func TestBuiltinList_NoValue(t *testing.T) {
	for _, fn := range []string{"avg", "first", "last"} {
		t.Run(fn, func(t *testing.T) {
			got, err := callFn(t, fn, []any{})
			if err != nil {
				t.Fatalf("%s() error: %v", fn, err)
			}
			if got != nil {
				t.Errorf("%s([]) = %v, want nil", fn, got)
			}
		})
	}
}

func TestBuiltinContains_ListMembership(t *testing.T) {
	t.Run("list membership case-insensitive", func(t *testing.T) {
		got, _ := callFn(t, "contains", []any{"x", "URGENT"}, "urgent")
		if got != true {
			t.Errorf("contains(list, urgent) = %v, want true", got)
		}
	})
	t.Run("list non-membership", func(t *testing.T) {
		got, _ := callFn(t, "contains", []any{"x", "y"}, "urgent")
		if got != false {
			t.Errorf("contains(list, urgent) = %v, want false", got)
		}
	})
	t.Run("string substring fallback", func(t *testing.T) {
		got, _ := callFn(t, "contains", "hello world", "WORLD")
		if got != true {
			t.Errorf("contains(string, sub) = %v, want true", got)
		}
	})
}

func TestBuiltinTypeCoercion(t *testing.T) {
	d := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name string
		fn   string
		arg  any
		want any
	}{
		{"int from string", "int", "42", int64(42)},
		{"int from float string truncates", "int", "3.9", int64(3)},
		{"int from float", "int", 3.9, int64(3)},
		{"int from bool", "int", true, int64(1)},
		{"float from string", "float", "3.5", 3.5},
		{"float from int", "float", int64(7), 7.0},
		{"float from bool", "float", false, 0.0},
		{"string from int", "string", int64(42), "42"},
		{"string from date is rfc3339", "string", d, "2026-01-02T03:04:05Z"},
		{"string from nil is empty", "string", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := callFn(t, tt.fn, tt.arg)
			if err != nil {
				t.Fatalf("%s() error: %v", tt.fn, err)
			}
			if got != tt.want {
				t.Errorf("%s(%v) = %v (%T), want %v (%T)", tt.fn, tt.arg, got, got, tt.want, tt.want)
			}
		})
	}

	t.Run("string from duration", func(t *testing.T) {
		got, _ := callFn(t, "string", 90*time.Minute)
		if got != "1h30m0s" {
			t.Errorf("string(duration) = %v, want 1h30m0s", got)
		}
	})
}

func TestBuiltinTypeCoercion_NoValue(t *testing.T) {
	// Unparseable, non-finite, and out-of-range strings all resolve to no value
	// so a coerced result is always a comparable, finite number.
	cases := []struct {
		fn  string
		arg string
	}{
		{"int", "notanumber"},
		{"float", "notanumber"},
		{"float", "NaN"},
		{"float", "inf"},
		{"float", "infinity"},
		{"int", "NaN"},
		{"int", "inf"},
		{"int", "1e400"}, // overflows float64 to +Inf
		{"int", "1e30"},  // finite but out of int64 range
	}
	for _, c := range cases {
		t.Run(c.fn+"("+c.arg+")", func(t *testing.T) {
			got, err := callFn(t, c.fn, c.arg)
			if err != nil {
				t.Fatalf("%s() error: %v", c.fn, err)
			}
			if got != nil {
				t.Errorf("%s(%q) = %v, want nil", c.fn, c.arg, got)
			}
		})
	}
}

// TestBuiltinReduce_Int64Precision guards that min/max/sum stay exact for
// int64 values beyond 2^53, where adjacent integers share one float64.
func TestBuiltinReduce_Int64Precision(t *testing.T) {
	const a = int64(9007199254740992) // 2^53
	const b = int64(9007199254740993) // 2^53 + 1 (same float64 as a)
	checks := []struct {
		fn   string
		args []any
		want int64
	}{
		{"min", []any{b, a}, a},
		{"max", []any{a, b}, b},
		{"sum", []any{[]any{a, b}}, a + b},
	}
	for _, c := range checks {
		t.Run(c.fn, func(t *testing.T) {
			got, err := callFn(t, c.fn, c.args...)
			if err != nil {
				t.Fatalf("%s error: %v", c.fn, err)
			}
			if got != c.want {
				t.Errorf("%s = %v, want %v", c.fn, got, c.want)
			}
		})
	}
}

// TestBuiltinReduce_RejectsNonNumeric guards that min/max error on a
// non-numeric operand rather than coercing it to zero.
func TestBuiltinReduce_RejectsNonNumeric(t *testing.T) {
	for _, fn := range []string{"min", "max"} {
		if _, err := callFn(t, fn, int64(5), "oops", int64(3)); err == nil {
			t.Errorf("%s(5, \"oops\", 3) error = nil, want numeric-operand error", fn)
		}
	}
}

// TestBuiltinSumAvg_ScalarArg guards the graceful single-scalar handling that
// matches count: a scalar is a one-element collection.
func TestBuiltinSumAvg_ScalarArg(t *testing.T) {
	if got, _ := callFn(t, "sum", int64(7)); got != int64(7) {
		t.Errorf("sum(7) = %v, want 7", got)
	}
	if got, _ := callFn(t, "avg", 7.0); got != 7.0 {
		t.Errorf("avg(7.0) = %v, want 7", got)
	}
	if got, _ := callFn(t, "sum", nil); got != int64(0) {
		t.Errorf("sum(nil) = %v, want 0", got)
	}
}

// TestBuiltinEndToEnd_ValuePath exercises the new built-ins through CompileValue,
// proving they compose with the value-expression entry point.
func TestBuiltinEndToEnd_ValuePath(t *testing.T) {
	withLists := WithFunctions(
		Func{Name: "amounts", Call: func(...any) (any, error) { return []float64{10, 20, 30}, nil }},
	)
	tests := []struct {
		q    string
		want any
	}{
		{"ceil(4.2)", 5.0},
		{"round(3.14159, 2)", 3.14},
		{"sqrt(16)", 4.0},
		{"min(8, 3, 5)", int64(3)},
		{"sum(amounts())", 60.0},
		{"avg(amounts())", 20.0},
		{`int("100")`, int64(100)},
	}
	for _, tt := range tests {
		t.Run(tt.q, func(t *testing.T) {
			prog, err := CompileValue(tt.q, nil, withLists)
			if err != nil {
				t.Fatalf("CompileValue(%q) error: %v", tt.q, err)
			}
			got, err := prog.Eval(nil)
			if err != nil {
				t.Fatalf("Eval() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Eval(%q) = %v (%T), want %v (%T)", tt.q, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestBuiltinEndToEnd_BooleanPath exercises the new built-ins in predicate
// position (the cron-recurrence and aggregate-threshold use cases).
func TestBuiltinEndToEnd_BooleanPath(t *testing.T) {
	withList := WithFunctions(
		Func{Name: "prices", Call: func(...any) (any, error) { return []float64{5, 15, 25}, nil }},
	)
	t.Run("aggregate threshold", func(t *testing.T) {
		prog, err := Compile("sum(prices())>40", nil, withList)
		if err != nil {
			t.Fatalf("Compile error: %v", err)
		}
		if !prog.Match(nil) {
			t.Error("sum(prices())>40 = false, want true (sum is 45)")
		}
	})
	t.Run("weekday recurrence predicate", func(t *testing.T) {
		// weekday(created_at) over a known Sunday must equal 0.
		prog, err := Compile("weekday(created_at)=0", testFields)
		if err != nil {
			t.Fatalf("Compile error: %v", err)
		}
		sunday := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		if !prog.Match(map[string]any{"created_at": sunday}) {
			t.Error("weekday(created_at)=0 over a Sunday = false, want true")
		}
	})
}
