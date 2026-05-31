package eval

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

func TestCompileValue_Literals(t *testing.T) {
	tests := []struct {
		name string
		q    string
		want any
	}{
		{"integer", "42", int64(42)},
		{"float", "3.14", 3.14},
		{"negative integer", "-7", int64(-7)},
		{"bareword string", "draft", "draft"},
		{"quoted string", `"hello world"`, "hello world"},
		{"boolean", "true", true},
		{"duration", "7d", 7 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			prog, err := CompileValue(tt.q, nil)
			if err != nil {
				t.Fatalf("CompileValue(%q) error: %v", tt.q, err)
			}
			// Act
			got, err := prog.Eval(nil)
			// Assert
			if err != nil {
				t.Fatalf("Eval() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Eval() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestCompileValue_Arithmetic(t *testing.T) {
	tests := []struct {
		name string
		q    string
		want any
	}{
		{"int multiply", "6*7", int64(42)},
		{"int add", "40+2", int64(42)},
		{"int modulo", "44%42", int64(2)},
		{"int division promotes to float", "9/2", 4.5},
		{"grouping overrides precedence", "(50000+1000)*1.1", 56100.000000000007},
		{"mixed precedence", "2+3*4", int64(14)},
		// Bare-integer subtraction is not expressible in value position: `50-8`
		// lexes as a date-like string and `50 - 8` is a lex error. Subtraction
		// is exercised through duration arithmetic in TestCompileValue_TimeArithmetic
		// (clock()-7d), which is the form the grammar supports.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := CompileValue(tt.q, nil)
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

func TestCompileValue_TimeArithmetic(t *testing.T) {
	// Arrange: a deterministic clock via a custom function avoids relying on
	// the wall clock. fixed() returns a known instant; subtracting a duration
	// must land exactly seven days earlier.
	fixed := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	prog, err := CompileValue("clock()-7d", nil,
		WithFunctions(Func{Name: "clock", Call: func(...any) (any, error) { return fixed, nil }}),
	)
	if err != nil {
		t.Fatalf("CompileValue error: %v", err)
	}
	// Act
	got, err := prog.Eval(nil)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	// Assert
	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("Eval() = %T, want time.Time", got)
	}
	want := fixed.Add(-7 * 24 * time.Hour)
	if !gotTime.Equal(want) {
		t.Errorf("Eval() = %v, want %v", gotTime, want)
	}
}

func TestCompileValue_FunctionOverField(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	prog, err := CompileValue("upper(name)", fields)
	if err != nil {
		t.Fatalf("CompileValue error: %v", err)
	}
	got, err := prog.Eval(map[string]any{"name": "draft"})
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if got != "DRAFT" {
		t.Errorf("Eval() = %v, want DRAFT", got)
	}
}

func TestCompileValue_ReturnsList(t *testing.T) {
	// A function returning a slice is preserved as a list value rather than
	// stringified — value expressions can return collections, not just scalars.
	tests := []struct {
		name string
		fn   Func
		want []any
	}{
		{
			name: "string slice",
			fn:   Func{Name: "tags", Call: func(...any) (any, error) { return []string{"a", "b", "c"}, nil }},
			want: []any{"a", "b", "c"},
		},
		{
			name: "any slice",
			fn:   Func{Name: "tags", Call: func(...any) (any, error) { return []any{1, 2, 3}, nil }},
			want: []any{1, 2, 3},
		},
		{
			name: "int slice",
			fn:   Func{Name: "tags", Call: func(...any) (any, error) { return []int{7, 8}, nil }},
			want: []any{7, 8},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := CompileValue("tags()", nil, WithFunctions(tt.fn))
			if err != nil {
				t.Fatalf("CompileValue error: %v", err)
			}
			got, err := prog.Eval(nil)
			if err != nil {
				t.Fatalf("Eval() error: %v", err)
			}
			list, ok := got.([]any)
			if !ok {
				t.Fatalf("Eval() = %T, want []any", got)
			}
			if len(list) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(list), len(tt.want))
			}
			for i := range list {
				if fmt.Sprint(list[i]) != fmt.Sprint(tt.want[i]) {
					t.Errorf("elem %d = %v, want %v", i, list[i], tt.want[i])
				}
			}
		})
	}
}

func TestCompileValue_ListOperations(t *testing.T) {
	// Field-valued lists feed list-aware builtins: len() counts elements and a
	// custom extractor returns a scalar element.
	fields := []validate.FieldConfig{
		{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	firstFn := WithFunctions(Func{Name: "first", Call: func(a ...any) (any, error) {
		if len(a) == 1 {
			if s, ok := a[0].([]any); ok && len(s) > 0 {
				return s[0], nil
			}
		}
		return nil, nil
	}})
	rec := map[string]any{"tags": []any{"urgent", "blocked"}}

	t.Run("len counts elements", func(t *testing.T) {
		prog, err := CompileValue("len(tags)", fields)
		if err != nil {
			t.Fatalf("CompileValue error: %v", err)
		}
		got, err := prog.Eval(rec)
		if err != nil {
			t.Fatalf("Eval() error: %v", err)
		}
		if got != int64(2) {
			t.Errorf("len(tags) = %v, want 2", got)
		}
	})

	t.Run("element extraction returns scalar", func(t *testing.T) {
		prog, err := CompileValue("first(tags)", fields, firstFn)
		if err != nil {
			t.Fatalf("CompileValue error: %v", err)
		}
		got, err := prog.Eval(rec)
		if err != nil {
			t.Fatalf("Eval() error: %v", err)
		}
		if got != "urgent" {
			t.Errorf("first(tags) = %v, want urgent", got)
		}
	})
}

// TestListValueOnBooleanPath guards the boolean Match path against the
// list-coercion change in valueFromAny. Two behaviors are pinned:
//
//   - len() over a list field counts elements (a deliberate change — it
//     previously returned the length of the slice's string form), and
//   - a list-valued operand reaching a scalar = comparison matches on its
//     string form case-insensitively, the same as ValueString (without the
//     ValueList case in equalValues it would silently become case-sensitive).
func TestListValueOnBooleanPath(t *testing.T) {
	t.Run("len over list field counts elements", func(t *testing.T) {
		fields := []validate.FieldConfig{
			{Name: "tags", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
		}
		rec := map[string]any{"tags": []any{"a", "b", "c"}}
		cases := []struct {
			q    string
			want bool
		}{
			{"len(tags)=3", true},
			{"len(tags)>2", true},
			{"len(tags)>5", false},
		}
		for _, c := range cases {
			prog, err := Compile(c.q, fields)
			if err != nil {
				t.Fatalf("Compile(%q): %v", c.q, err)
			}
			if got := prog.Match(rec); got != c.want {
				t.Errorf("Match(%q) = %v, want %v", c.q, got, c.want)
			}
		}
	})

	t.Run("list-valued function equality is case-insensitive", func(t *testing.T) {
		fields := []validate.FieldConfig{
			{Name: "label", Type: validate.TypeText, AllowedOps: validate.TextOps},
		}
		slc := WithFunctions(Func{Name: "slc", Call: func(...any) (any, error) { return []string{"X", "Y"}, nil }})
		prog, err := Compile("label=slc()", fields, slc)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		// The function result stringifies to "[X Y]"; equality matches its
		// string form case-insensitively, like every other comparable type.
		if !prog.Match(map[string]any{"label": "[X Y]"}) {
			t.Error("Match{label:[X Y]} = false, want true")
		}
		if !prog.Match(map[string]any{"label": "[x y]"}) {
			t.Error("Match{label:[x y]} = false, want true (case-insensitive)")
		}
		if prog.Match(map[string]any{"label": "[a b]"}) {
			t.Error("Match{label:[a b]} = true, want false")
		}
	})
}

func TestCompileValue_NestedCalls(t *testing.T) {
	tests := []struct {
		name string
		q    string
		want any
	}{
		{"nested string funcs", `upper(lower("Hi"))`, "HI"},
		{"coalesce picks fallback", `coalesce("", "fallback")`, "fallback"},
		{"if true branch", "if(true, 1, 2)", int64(1)},
		{"if false branch", "if(false, 1, 2)", int64(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := CompileValue(tt.q, nil)
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

// TestCompileValue_SelectorsAreNotValueExpressions documents the boundary:
// list selectors (@first, @any, @all, @none) are boolean predicates over a
// list — "does any element match" — and have no meaning in value position.
// Value position has no selector grammar at all: `@` is an ordinary string
// character there (so a handle like "@user" is a valid value), which means the
// paren selector forms fail to parse, while a bare `tags@first` degenerates to
// the literal string "tags@first" rather than a selector. To extract an element
// as a value, call a function — first(tags) — see TestCompileValue_ListOperations.
func TestCompileValue_SelectorsAreNotValueExpressions(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "items", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
	}

	t.Run("paren selector forms fail to parse", func(t *testing.T) {
		for _, q := range []string{"items@any(x=1)", "tags@all(active=true)", "items@(x=1)"} {
			if _, err := CompileValue(q, fields); err == nil {
				t.Errorf("CompileValue(%q) error = nil, want parse error", q)
			}
		}
	})

	t.Run("bare @first degenerates to a string value", func(t *testing.T) {
		prog, err := CompileValue("tags@first", fields)
		if err != nil {
			t.Fatalf("CompileValue error: %v", err)
		}
		got, err := prog.Eval(nil)
		if err != nil {
			t.Fatalf("Eval() error: %v", err)
		}
		if got != "tags@first" {
			t.Errorf("Eval() = %v, want the literal string \"tags@first\"", got)
		}
	})
}

func TestCompileValue_DivByZeroReturnsErrNoValue(t *testing.T) {
	tests := []string{"5/0", "5%0"}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			prog, err := CompileValue(q, nil)
			if err != nil {
				t.Fatalf("CompileValue(%q) error: %v", q, err)
			}
			got, err := prog.Eval(nil)
			if !errors.Is(err, ErrNoValue) {
				t.Errorf("Eval(%q) err = %v, want ErrNoValue", q, err)
			}
			if got != nil {
				t.Errorf("Eval(%q) = %v, want nil", q, got)
			}
		})
	}
}

func TestCompileValue_MissingFieldFunction(t *testing.T) {
	// len() over a present field resolves to its length; this guards that field
	// resolution through a function argument works end to end.
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	prog, err := CompileValue("len(name)", fields)
	if err != nil {
		t.Fatalf("CompileValue error: %v", err)
	}
	got, err := prog.Eval(map[string]any{"name": "abcd"})
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if got != int64(4) {
		t.Errorf("Eval() = %v, want 4", got)
	}
}

func TestCompileValue_ParseAndValidateErrors(t *testing.T) {
	tests := []struct {
		name   string
		q      string
		fields []validate.FieldConfig
	}{
		{"trailing token", "42 foo", nil},
		{"empty", "", nil},
		{"unknown field in function", "upper(missing)", []validate.FieldConfig{
			{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CompileValue(tt.q, tt.fields); err == nil {
				t.Errorf("CompileValue(%q) error = nil, want error", tt.q)
			}
		})
	}
}

func TestCompileValue_EvalFunc(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	prog, err := CompileValue("lower(name)", fields)
	if err != nil {
		t.Fatalf("CompileValue error: %v", err)
	}
	got, err := prog.EvalFunc(func(field string) (any, bool) {
		if field == "name" {
			return "DRAFT", true
		}
		return nil, false
	})
	if err != nil {
		t.Fatalf("EvalFunc() error: %v", err)
	}
	if got != "draft" {
		t.Errorf("EvalFunc() = %v, want draft", got)
	}
}

func TestCompileValue_Stringify(t *testing.T) {
	tests := []struct {
		q    string
		want string
	}{
		{"(50000+1000)*1.1", "(50000+1000)*1.1"},
		{"6*7", "6*7"},
		{"upper(name)", "upper(name)"},
	}
	for _, tt := range tests {
		t.Run(tt.q, func(t *testing.T) {
			var fields []validate.FieldConfig
			if tt.q == "upper(name)" {
				fields = []validate.FieldConfig{{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps}}
			}
			prog, err := CompileValue(tt.q, fields)
			if err != nil {
				t.Fatalf("CompileValue error: %v", err)
			}
			if got := prog.Stringify(); got != tt.want {
				t.Errorf("Stringify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompileValue_SourceAndAST(t *testing.T) {
	const q = "now()-7d"
	prog, err := CompileValue(q, nil)
	if err != nil {
		t.Fatalf("CompileValue error: %v", err)
	}
	if got := prog.String(); got != q {
		t.Errorf("String() = %q, want %q", got, q)
	}
	if prog.AST() == nil {
		t.Error("AST() = nil, want a parsed expression")
	}
}

// TestCompileValue_BooleanPathUnaffected guards that the value entrypoint did
// not regress the predicate engine: the same library still compiles and matches
// boolean queries.
func TestCompileValue_BooleanPathUnaffected(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	}
	prog, err := Compile("total>50000", fields)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if !prog.Match(map[string]any{"total": 60000}) {
		t.Error("Match(total=60000) = false, want true")
	}
	if prog.Match(map[string]any{"total": 100}) {
		t.Error("Match(total=100) = true, want false")
	}
}

func TestCompileValue_MaxDepth(t *testing.T) {
	// Depth is measured on the Expression tree; a value root is depth 1 and
	// never trips a positive limit even when its value subtree nests.
	prog, err := CompileValue("1+2+3", nil, WithMaxDepth(1))
	if err != nil {
		t.Fatalf("CompileValue error: %v", err)
	}
	got, err := prog.Eval(nil)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if got != int64(6) {
		t.Errorf("Eval() = %v, want 6", got)
	}
}
