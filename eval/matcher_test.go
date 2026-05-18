package eval

import (
	"testing"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

func TestEqualValues(t *testing.T) {
	fixedDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	otherDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		actual   any
		expected *ast.Value
		want     bool
	}{
		{"string ci match", "DRAFT", &ast.Value{Type: ast.ValueString, Str: "draft"}, true},
		{"string mismatch", "published", &ast.Value{Type: ast.ValueString, Str: "draft"}, false},
		{"int64 match", int64(42), &ast.Value{Type: ast.ValueInteger, Int: 42}, true},
		{"int match", int(42), &ast.Value{Type: ast.ValueInteger, Int: 42}, true},
		{"int mismatch", int64(41), &ast.Value{Type: ast.ValueInteger, Int: 42}, false},
		{"float match", 3.14, &ast.Value{Type: ast.ValueFloat, Float: 3.14}, true},
		{"float mismatch", 3.14, &ast.Value{Type: ast.ValueFloat, Float: 2.71}, false},
		{"bool match", true, &ast.Value{Type: ast.ValueBoolean, Bool: true}, true},
		{"bool mismatch", false, &ast.Value{Type: ast.ValueBoolean, Bool: true}, false},
		{"date match", fixedDate, &ast.Value{Type: ast.ValueDate, Date: fixedDate}, true},
		{"date mismatch", otherDate, &ast.Value{Type: ast.ValueDate, Date: fixedDate}, false},
		{"duration match", time.Hour, &ast.Value{Type: ast.ValueDuration, Duration: time.Hour}, true},
		{"duration mismatch", 2 * time.Hour, &ast.Value{Type: ast.ValueDuration, Duration: time.Hour}, false},
		{"default branch", "anything", &ast.Value{Type: 9999, Raw: "anything"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := equalValues(tt.actual, tt.expected); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareValues_DefaultString(t *testing.T) {
	actual := "bravo"
	expected := &ast.Value{Type: 9999, Raw: "alpha"}
	if !compareValues(actual, expected, token.Gt) {
		t.Error("bravo > alpha")
	}
}

func TestCompareOrdered(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		op   token.Type
		want bool
	}{
		{"gt true", 5, 3, token.Gt, true},
		{"gt false", 3, 5, token.Gt, false},
		{"gte true", 5, 3, token.Gte, true},
		{"gte equal", 5, 5, token.Gte, true},
		{"lt false", 5, 3, token.Lt, false},
		{"lt true", 3, 5, token.Lt, true},
		{"lte true", 3, 5, token.Lte, true},
		{"eq false", 5, 3, token.Eq, false},
		{"eq true", 5, 5, token.Eq, true},
		{"neq true", 5, 3, token.Neq, true},
		{"default", 5, 3, token.And, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareOrdered(tt.a, tt.b, tt.op); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompileMatcher_NilExpr(t *testing.T) {
	m := compileMatcher(nil, nil)
	if m(func(string) (any, bool) { return nil, false }) {
		t.Error("expected false for nil expr")
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestFuncCall_ErrorPaths(t *testing.T) {
	fp := ast.FieldPath{"x"}
	buggyFn := Func{
		Name: "buggy",
		Call: func(...any) (any, error) { return nil, &testError{msg: "boom"} },
	}

	tests := []struct {
		name     string
		expr     ast.Expression
		registry FuncRegistry
	}{
		{
			name:     "standalone unknown function",
			expr:     &ast.FuncCallExpr{Name: "nonexistent"},
			registry: BuiltinFunctions(),
		},
		{
			name: "standalone function errors",
			expr: &ast.FuncCallExpr{
				Name: "buggy",
				Args: []ast.FuncArg{{Field: &fp}},
			},
			registry: FuncRegistry{"buggy": buggyFn},
		},
		{
			name: "field func errors",
			expr: &ast.QualifierExpr{
				Field:    ast.FieldPath{"buggy"},
				Operator: token.Eq,
				Value:    ast.Value{Type: ast.ValueString, Str: "y"},
				FieldFunc: &ast.FuncCallExpr{
					Name: "buggy",
					Args: []ast.FuncArg{{Field: &fp}},
				},
			},
			registry: FuncRegistry{"buggy": buggyFn},
		},
		{
			name: "field func missing",
			expr: &ast.QualifierExpr{
				Field:    ast.FieldPath{"missing"},
				Operator: token.Eq,
				Value:    ast.Value{Type: ast.ValueString, Str: "y"},
				FieldFunc: &ast.FuncCallExpr{
					Name: "missing",
					Args: []ast.FuncArg{{Field: &fp}},
				},
			},
			registry: FuncRegistry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := compileMatcher(tt.expr, tt.registry)
			got := m(func(string) (any, bool) { return "v", true })
			if got {
				t.Error("expected false")
			}
		})
	}
}
