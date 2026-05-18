package parser

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

func TestParse_FuncCall(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, expr ast.Expression)
	}{
		{
			name:  "simple field transform",
			input: "lower(name)=john",
			check: func(t *testing.T, expr ast.Expression) {
				t.Helper()
				q, ok := expr.(*ast.QualifierExpr)
				if !ok {
					t.Fatalf("expected QualifierExpr, got %T", expr)
				}
				if !q.HasFieldFunc() || q.FieldFunc.Name != "lower" {
					t.Errorf("expected lower() field func")
				}
				if len(q.FieldFunc.Args) != 1 || q.FieldFunc.Args[0].Field == nil {
					t.Errorf("expected 1 field arg")
				}
			},
		},
		{
			name:  "standalone with two args",
			input: "contains(name, search)",
			check: func(t *testing.T, expr ast.Expression) {
				t.Helper()
				fc, ok := expr.(*ast.FuncCallExpr)
				if !ok {
					t.Fatalf("expected FuncCallExpr, got %T", expr)
				}
				if fc.Name != "contains" || len(fc.Args) != 2 {
					t.Errorf("got %s with %d args", fc.Name, len(fc.Args))
				}
			},
		},
		{
			name:  "nested calls",
			input: "int(year(created_at))=2026",
			check: func(t *testing.T, expr ast.Expression) {
				t.Helper()
				q, ok := expr.(*ast.QualifierExpr)
				if !ok {
					t.Fatalf("expected QualifierExpr, got %T", expr)
				}
				if q.FieldFunc == nil || q.FieldFunc.Name != "int" {
					t.Fatal("expected int() field func")
				}
				if q.FieldFunc.Args[0].Call == nil || q.FieldFunc.Args[0].Call.Name != "year" {
					t.Error("expected nested year() call")
				}
			},
		},
		{
			name:  "no args",
			input: "now()=today",
			check: func(t *testing.T, expr ast.Expression) {
				t.Helper()
				q, ok := expr.(*ast.QualifierExpr)
				if !ok {
					t.Fatalf("expected QualifierExpr, got %T", expr)
				}
				if q.FieldFunc == nil || len(q.FieldFunc.Args) != 0 {
					t.Error("expected no args")
				}
			},
		},
		{
			name:  "in logical expression",
			input: "lower(name)=john AND year>2020",
			check: func(t *testing.T, expr ast.Expression) {
				t.Helper()
				if _, ok := expr.(*ast.BinaryExpr); !ok {
					t.Fatalf("expected BinaryExpr, got %T", expr)
				}
			},
		},
		{
			name:  "followed by operator",
			input: "len(name)>=5",
			check: func(t *testing.T, expr ast.Expression) {
				t.Helper()
				q, ok := expr.(*ast.QualifierExpr)
				if !ok {
					t.Fatalf("expected QualifierExpr, got %T", expr)
				}
				if q.Operator != token.Gte {
					t.Errorf("got op %v, want Gte", q.Operator)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := Parse(tt.input, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			tt.check(t, expr)
		})
	}
}

func TestParse_FuncCall_Errors(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"missing comma", "contains(a b)"},
		{"unclosed paren", "lower(name=john"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input, 0); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"day", "1d", true},
		{"hour", "4h", true},
		{"minute", "30m", true},
		{"week", "2w", true},
		{"empty", "", false},
		{"too short", "1", false},
		{"bad chars", "abcd", false},
		{"bad suffix", "1x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDuration(tt.input)
			if tt.valid && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected error")
			}
		})
	}
}
