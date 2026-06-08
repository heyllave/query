package eval

import (
	"errors"
	"testing"

	"github.com/heyllave/query/validate"
)

func fieldRefEvalFields() []validate.FieldConfig {
	return []validate.FieldConfig{
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		{Name: "base", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		{Name: "cpu", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
		{Name: "cpu_limit", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	}
}

// TestFieldRef_ArithmeticOperand: a field reference as an arithmetic operand on
// the comparison RHS.
func TestFieldRef_ArithmeticOperand(t *testing.T) {
	prog, err := Compile("total>=[base]*2", fieldRefEvalFields())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	tests := []struct {
		rec  map[string]any
		want bool
	}{
		{map[string]any{"total": 200.0, "base": 100.0}, true},  // 200 >= 200
		{map[string]any{"total": 150.0, "base": 100.0}, false}, // 150 >= 200
		{map[string]any{"total": 200.0}, false},                // missing base → false
	}
	for _, tt := range tests {
		if got := prog.Match(tt.rec); got != tt.want {
			t.Errorf("Match(%v) = %v, want %v", tt.rec, got, tt.want)
		}
	}
}

// TestFieldRef_CrossFieldComparison: a bare field reference as the RHS.
func TestFieldRef_CrossFieldComparison(t *testing.T) {
	prog, err := Compile("cpu>[cpu_limit]", fieldRefEvalFields())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	tests := []struct {
		rec  map[string]any
		want bool
	}{
		{map[string]any{"cpu": int64(90), "cpu_limit": int64(80)}, true},
		{map[string]any{"cpu": int64(50), "cpu_limit": int64(80)}, false},
		{map[string]any{"cpu": int64(90)}, false},                   // missing RHS field → false
		{map[string]any{"cpu": int64(90), "cpu_limit": nil}, false}, // present-but-nil → false
	}
	for _, tt := range tests {
		if got := prog.Match(tt.rec); got != tt.want {
			t.Errorf("Match(%v) = %v, want %v", tt.rec, got, tt.want)
		}
	}
}

// TestFieldRef_ValueDomain: a field reference (and field arithmetic) as a
// value-returning query.
func TestFieldRef_ValueDomain(t *testing.T) {
	t.Run("bare ref returns the field value", func(t *testing.T) {
		prog, err := CompileValue("[base]", fieldRefEvalFields())
		if err != nil {
			t.Fatalf("CompileValue: %v", err)
		}
		got, err := prog.Eval(map[string]any{"base": 5.0})
		if err != nil {
			t.Fatalf("Eval: %v", err)
		}
		if got != 5.0 {
			t.Errorf("[base] {base:5} = %v, want 5", got)
		}
	})

	t.Run("missing field is ErrNoValue", func(t *testing.T) {
		prog, _ := CompileValue("[base]", fieldRefEvalFields())
		if _, err := prog.Eval(map[string]any{}); !errors.Is(err, ErrNoValue) {
			t.Errorf("Eval missing field err = %v, want ErrNoValue", err)
		}
	})

	t.Run("arithmetic over a field", func(t *testing.T) {
		prog, _ := CompileValue("[base]*2", fieldRefEvalFields())
		got, err := prog.Eval(map[string]any{"base": 21.0})
		if err != nil {
			t.Fatalf("Eval: %v", err)
		}
		if got != 42.0 {
			t.Errorf("[base]*2 {base:21} = %v, want 42", got)
		}
	})
}

// TestFieldRef_QuotedName: a hyphenated field name via the quoted bracket form.
func TestFieldRef_QuotedName(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "base-1", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	}
	prog, err := CompileValue(`["base-1"]-1`, fields)
	if err != nil {
		t.Fatalf("CompileValue: %v", err)
	}
	got, err := prog.Eval(map[string]any{"base-1": int64(10)})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != int64(9) {
		t.Errorf(`["base-1"]-1 {base-1:10} = %v, want 9`, got)
	}
}
