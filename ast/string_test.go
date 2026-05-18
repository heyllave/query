package ast

import (
	"testing"

	"github.com/heyllave/query/token"
)

func TestString(t *testing.T) {
	fp := FieldPath{"name"}
	innerFC := &FuncCallExpr{Name: "year", Args: []FuncArg{{Field: &fp}}}

	tests := []struct {
		name string
		expr Expression
		want string
	}{
		{
			name: "binary AND",
			expr: &BinaryExpr{
				Op:    token.And,
				Left:  &QualifierExpr{Field: FieldPath{"a"}, Operator: token.Eq, Value: Value{Raw: "1"}},
				Right: &QualifierExpr{Field: FieldPath{"b"}, Operator: token.Eq, Value: Value{Raw: "2"}},
			},
			want: "a=1 AND b=2",
		},
		{
			name: "binary OR",
			expr: &BinaryExpr{
				Op:    token.Or,
				Left:  &QualifierExpr{Field: FieldPath{"a"}, Operator: token.Eq, Value: Value{Raw: "1"}},
				Right: &QualifierExpr{Field: FieldPath{"b"}, Operator: token.Eq, Value: Value{Raw: "2"}},
			},
			want: "a=1 OR b=2",
		},
		{
			name: "unary NOT",
			expr: &UnaryExpr{
				Op:   token.Not,
				Expr: &QualifierExpr{Field: FieldPath{"state"}, Operator: token.Eq, Value: Value{Raw: "draft"}},
			},
			want: "NOT state=draft",
		},
		{
			name: "range",
			expr: &QualifierExpr{
				Field:    FieldPath{"created_at"},
				Operator: token.Range,
				Value:    Value{Raw: "2026-01-01"},
				EndValue: &Value{Raw: "2026-03-31"},
			},
			want: "created_at:2026-01-01..2026-03-31",
		},
		{
			name: "presence",
			expr: &PresenceExpr{Field: FieldPath{"tire_size"}},
			want: "tire_size",
		},
		{
			name: "group",
			expr: &GroupExpr{
				Expr: &QualifierExpr{Field: FieldPath{"state"}, Operator: token.Eq, Value: Value{Raw: "draft"}},
			},
			want: "(state=draft)",
		},
		{
			name: "selector first",
			expr: &SelectorExpr{
				Base:     &QualifierExpr{Field: FieldPath{"items"}, Operator: token.Eq, Value: Value{Raw: "a"}},
				Selector: "first",
			},
			want: "items=a@first",
		},
		{
			name: "selector inner",
			expr: &SelectorExpr{
				Base:  &QualifierExpr{Field: FieldPath{"items"}, Operator: token.Eq, Value: Value{Raw: "a"}},
				Inner: &QualifierExpr{Field: FieldPath{"x"}, Operator: token.Eq, Value: Value{Raw: "1"}},
			},
			want: "items=a@(x=1)",
		},
		{
			name: "func call with field",
			expr: &FuncCallExpr{Name: "lower", Args: []FuncArg{{Field: &fp}}},
			want: "lower(name)",
		},
		{
			name: "func call with value",
			expr: &FuncCallExpr{Name: "test", Args: []FuncArg{{Value: &Value{Raw: "42"}}}},
			want: "test(42)",
		},
		{
			name: "nested func call",
			expr: &FuncCallExpr{Name: "int", Args: []FuncArg{{Call: innerFC}}},
			want: "int(year(name))",
		},
		{
			name: "nil expression",
			expr: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.expr); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
