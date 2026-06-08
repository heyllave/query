package eval

import (
	"testing"
	"time"
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

func TestBuiltinList_SumAvgRequireList(t *testing.T) {
	for _, fn := range []string{"sum", "avg"} {
		if _, err := callFn(t, fn, "not a list"); err == nil {
			t.Errorf("%s(scalar) error = nil, want list-required error", fn)
		}
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
	t.Run("string substring fallback unchanged", func(t *testing.T) {
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
	for _, fn := range []string{"int", "float"} {
		t.Run(fn, func(t *testing.T) {
			got, err := callFn(t, fn, "notanumber")
			if err != nil {
				t.Fatalf("%s() error: %v", fn, err)
			}
			if got != nil {
				t.Errorf("%s(notanumber) = %v, want nil", fn, got)
			}
		})
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
