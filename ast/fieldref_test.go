package ast

import "testing"

func TestFieldRefString(t *testing.T) {
	tests := []struct {
		name   string
		fp     FieldPath
		quoted bool
		want   string
	}{
		{"bare", FieldPath{"base"}, false, "[base]"},
		{"dotted", FieldPath{"labels", "dev"}, false, "[labels.dev]"},
		{"forced quoted", FieldPath{"base-1"}, true, `["base-1"]`},
		{"auto-quoted space", FieldPath{"weird name"}, false, `["weird name"]`},
		{"hyphen stays bare", FieldPath{"base-1"}, false, "[base-1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FieldRefString(tt.fp, tt.quoted); got != tt.want {
				t.Errorf("FieldRefString(%v, %v) = %q, want %q", []string(tt.fp), tt.quoted, got, tt.want)
			}
		})
	}
}

func TestFields_IncludesValuePositionRefs(t *testing.T) {
	// cpu > [cpu_limit]  →  Fields reports both cpu and cpu_limit.
	crossField := &QualifierExpr{
		Field: FieldPath{"cpu"},
		Value: Value{Type: ValueFieldRef, Field: FieldPath{"cpu_limit"}},
	}
	got := fieldKeys(Fields(crossField))
	assertHasAll(t, got, "cpu", "cpu_limit")

	// total >= [base]*1.1  →  total and base.
	arith := &QualifierExpr{
		Field: FieldPath{"total"},
		Value: Value{Type: ValueArith, Arith: &ArithExpr{
			Op:    ArithMul,
			Left:  &Value{Type: ValueFieldRef, Field: FieldPath{"base"}},
			Right: &Value{Type: ValueFloat, Float: 1.1},
		}},
	}
	assertHasAll(t, fieldKeys(Fields(arith)), "total", "base")

	// A field ref inside a function argument is also surfaced.
	funcArg := &QualifierExpr{
		Field: FieldPath{"total"},
		Value: Value{Type: ValueFunc, Func: &FuncCallExpr{
			Name: "max",
			Args: []FuncArg{{Field: &FieldPath{"floor_price"}}},
		}},
	}
	assertHasAll(t, fieldKeys(Fields(funcArg)), "total", "floor_price")
}

func fieldKeys(fps []FieldPath) map[string]bool {
	m := make(map[string]bool, len(fps))
	for _, fp := range fps {
		m[fp.String()] = true
	}
	return m
}

func assertHasAll(t *testing.T, got map[string]bool, want ...string) {
	t.Helper()
	for _, w := range want {
		if !got[w] {
			t.Errorf("Fields() missing %q; got %v", w, got)
		}
	}
}

func TestString_FieldRefRoundTrip(t *testing.T) {
	// A ValueFieldRef value renders back to its bracketed source form.
	tests := []struct {
		v    Value
		want string
	}{
		{Value{Type: ValueFieldRef, Field: FieldPath{"base"}}, "[base]"},
		{Value{Type: ValueFieldRef, Field: FieldPath{"labels", "dev"}}, "[labels.dev]"},
		{Value{Type: ValueFieldRef, Field: FieldPath{"base-1"}, Quoted: true}, `["base-1"]`},
	}
	for _, tt := range tests {
		expr := &ValueExpr{Value: tt.v}
		if got := String(expr); got != tt.want {
			t.Errorf("String(field ref %v) = %q, want %q", []string(tt.v.Field), got, tt.want)
		}
	}
}
