package ast

import (
	"testing"

	"github.com/heyllave/query/token"
)

func TestWalk_CountQualifiers(t *testing.T) {
	// Build AST: a=1 AND (b=2 OR c=3)
	expr := &BinaryExpr{
		Op: token.And,
		Left: &QualifierExpr{
			Field:    FieldPath{"a"},
			Operator: token.Eq,
			Value:    Value{Type: ValueString, Raw: "1", Str: "1"},
		},
		Right: &GroupExpr{
			Expr: &BinaryExpr{
				Op: token.Or,
				Left: &QualifierExpr{
					Field: FieldPath{"b"}, Operator: token.Eq,
					Value: Value{Type: ValueString, Raw: "2", Str: "2"},
				},
				Right: &QualifierExpr{
					Field: FieldPath{"c"}, Operator: token.Eq,
					Value: Value{Type: ValueString, Raw: "3", Str: "3"},
				},
			},
		},
	}

	var fields []string
	Walk(expr, func(e Expression) bool {
		if q, ok := e.(*QualifierExpr); ok {
			fields = append(fields, q.Field.String())
		}
		return true
	})
	if len(fields) != 3 {
		t.Fatalf("got %d fields, want 3", len(fields))
	}
}

func TestFields(t *testing.T) {
	tests := []struct {
		name string
		expr Expression
		want int
	}{
		{
			name: "two fields",
			expr: &BinaryExpr{
				Op:    token.And,
				Left:  &QualifierExpr{Field: FieldPath{"state"}},
				Right: &PresenceExpr{Field: FieldPath{"labels", "dev"}},
			},
			want: 2,
		},
		{
			name: "single qualifier",
			expr: &QualifierExpr{Field: FieldPath{"state"}},
			want: 1,
		},
		{
			name: "single presence",
			expr: &PresenceExpr{Field: FieldPath{"tire_size"}},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fields(tt.expr)
			if len(got) != tt.want {
				t.Errorf("got %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestIsSimple(t *testing.T) {
	tests := []struct {
		name string
		expr Expression
		want bool
	}{
		{"qualifier", &QualifierExpr{}, true},
		{"presence", &PresenceExpr{}, true},
		{"binary", &BinaryExpr{}, false},
		{"unary", &UnaryExpr{}, false},
		{"group", &GroupExpr{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSimple(tt.expr); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDepth(t *testing.T) {
	tests := []struct {
		name string
		expr Expression
		want int
	}{
		{"nil", nil, 0},
		{"single qualifier", &QualifierExpr{}, 1},
		{
			name: "binary with two qualifiers",
			expr: &BinaryExpr{
				Left:  &QualifierExpr{},
				Right: &QualifierExpr{},
			},
			want: 2,
		},
		{
			name: "unary + group + qualifier",
			expr: &UnaryExpr{
				Expr: &GroupExpr{
					Expr: &QualifierExpr{Field: FieldPath{"a"}},
				},
			},
			want: 3,
		},
		{
			name: "selector with inner",
			expr: &SelectorExpr{
				Base:  &QualifierExpr{Field: FieldPath{"items"}},
				Inner: &QualifierExpr{Field: FieldPath{"x"}},
			},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Depth(tt.expr); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestString_Nil(t *testing.T) {
	if got := String(nil); got != "" {
		t.Errorf("String(nil): got %q, want empty", got)
	}
}

func TestWalk_Nil(t *testing.T) {
	Walk(nil, func(Expression) bool { return true })
}

func TestFieldPath(t *testing.T) {
	tests := []struct {
		name     string
		fp       FieldPath
		wantStr  string
		wantRoot string
		wantNest bool
	}{
		{"empty", FieldPath{}, "", "", false},
		{"single", FieldPath{"state"}, "state", "state", false},
		{"nested", FieldPath{"labels", "dev"}, "labels.dev", "labels", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fp.String(); got != tt.wantStr {
				t.Errorf("String: got %q, want %q", got, tt.wantStr)
			}
			if got := tt.fp.Root(); got != tt.wantRoot {
				t.Errorf("Root: got %q, want %q", got, tt.wantRoot)
			}
			if got := tt.fp.IsNested(); got != tt.wantNest {
				t.Errorf("IsNested: got %v, want %v", got, tt.wantNest)
			}
		})
	}
}

func TestValueAny(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want any
	}{
		{"integer", Value{Type: ValueInteger, Int: 42}, int64(42)},
		{"boolean", Value{Type: ValueBoolean, Bool: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.Any()
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
