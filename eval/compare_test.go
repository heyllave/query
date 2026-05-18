package eval

import (
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// Cover all date comparison operators.
func TestMatch_DateComparisons(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "d", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	}
	older := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	target := map[string]any{"d": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

	cases := []struct {
		q    string
		want bool
	}{
		{"d>2025-01-01", true},
		{"d>2027-01-01", false},
		{"d>=2026-01-01", true},
		{"d<2025-01-01", false},
		{"d<2027-01-01", true},
		{"d<=2026-01-01", true},
		{"d=2026-01-01", true},
		{"d!=2025-01-01", true},
	}
	for _, c := range cases {
		t.Run(c.q, func(t *testing.T) {
			prog, err := Compile(c.q, fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if prog.Match(target) != c.want {
				t.Errorf("got %v, want %v", !c.want, c.want)
			}
		})
	}
	_ = older
	_ = newer
}

// Cover the default branch of date comparison (Eq).
func TestCompareDate_DefaultOp(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "d", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	}
	prog, err := Compile("d=2026-01-01", fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !prog.Match(map[string]any{"d": d}) {
		t.Error("expected equality match")
	}
}

// Cover the default branch in compareOrdered via an unrecognized op.
// This happens when compileComparisonWithResolver is called with an
// operator that isn't Eq/Neq and falls through to compareValues with
// an unsupported op for ordered types.
