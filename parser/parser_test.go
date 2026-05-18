package parser

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

func TestParse_SimpleEquality(t *testing.T) {
	expr := mustParse(t, "state=draft")
	q := assertQualifier(t, expr)
	if q.Field.String() != "state" {
		t.Errorf("field: got %q, want %q", q.Field.String(), "state")
	}
	if q.Operator != token.Eq {
		t.Errorf("operator: got %v, want Eq", q.Operator)
	}
}

func TestParse_Precedence(t *testing.T) {
	expr := mustParse(t, "a=1 OR b=2 AND c=3")
	b, ok := expr.(*ast.BinaryExpr)
	if !ok || b.Op != token.Or {
		t.Fatal("expected OR at top level")
	}
	right, ok := b.Right.(*ast.BinaryExpr)
	if !ok || right.Op != token.And {
		t.Fatal("expected AND on right")
	}
}

func TestParse_GroupNOT(t *testing.T) {
	expr := mustParse(t, "NOT (state=draft OR state=issued)")
	u, ok := expr.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", expr)
	}
	_, ok = u.Expr.(*ast.GroupExpr)
	if !ok {
		t.Fatalf("expected GroupExpr, got %T", u.Expr)
	}
}

func TestParse_Range(t *testing.T) {
	expr := mustParse(t, "created_at:2026-01-01..2026-03-31")
	q := assertQualifier(t, expr)
	if q.Operator != token.Range {
		t.Errorf("operator: got %v, want Range", q.Operator)
	}
	if q.EndValue == nil {
		t.Fatal("expected EndValue")
	}
}

func TestParse_Presence(t *testing.T) {
	expr := mustParse(t, "tire_size")
	_, ok := expr.(*ast.PresenceExpr)
	if !ok {
		t.Fatalf("expected PresenceExpr, got %T", expr)
	}
}

func TestParse_DottedField(t *testing.T) {
	expr := mustParse(t, "labels.dev=jane")
	q := assertQualifier(t, expr)
	if q.Field.String() != "labels.dev" {
		t.Errorf("field: got %q, want %q", q.Field.String(), "labels.dev")
	}
}

func TestParse_RoundTrip(t *testing.T) {
	examples := []string{
		"state=draft",
		"state!=cancelled",
		"year>2020",
		"tire_size",
		"name=John*",
		"NOT state=cancelled",
		"state=draft AND total>50000",
		"(state=draft OR state=issued) AND total>50000",
		"(labels.dev=jane OR labels.env=bar) AND NOT cluster=demo",
		"created_at:2026-01-01..2026-03-31",
		"items@first",
		"items@last",
		"items@(name=foo)",
		"orders@(status=shipped) AND total>500",
		"NOT items@(name=foo)",
	}
	for _, q := range examples {
		t.Run(q, func(t *testing.T) {
			expr := mustParse(t, q)
			got := ast.String(expr)
			if got != q {
				t.Errorf("round-trip:\n  got:  %q\n  want: %q", got, q)
			}
		})
	}
}

func TestParse_Selector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		baseName string
		selector string
		hasInner bool
	}{
		{"first", "items@first", "items", "first", false},
		{"last", "items@last", "items", "last", false},
		{"inner equality", "items@(name=foo)", "items", "", true},
		{"inner numeric", "line_items@(price>100)", "line_items", "", true},
		{"inner nested field", "orders@(labels.env=prod)", "orders", "", true},
		{"inner with AND", "orders@(status=shipped AND qty>0)", "orders", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := mustParse(t, tt.input)
			s, ok := expr.(*ast.SelectorExpr)
			if !ok {
				t.Fatalf("expected *SelectorExpr, got %T", expr)
			}
			base, ok := s.Base.(*ast.PresenceExpr)
			if !ok {
				t.Fatalf("expected PresenceExpr base, got %T", s.Base)
			}
			if base.Field.String() != tt.baseName {
				t.Errorf("base field: got %q, want %q", base.Field.String(), tt.baseName)
			}
			if s.Selector != tt.selector {
				t.Errorf("selector: got %q, want %q", s.Selector, tt.selector)
			}
			if (s.Inner != nil) != tt.hasInner {
				t.Errorf("inner presence: got %v, want %v", s.Inner != nil, tt.hasInner)
			}
		})
	}
}

func TestParse_SelectorComposition(t *testing.T) {
	// Selector inside a boolean composition.
	expr := mustParse(t, "orders@(status=shipped) AND total>500")
	b, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if _, ok := b.Left.(*ast.SelectorExpr); !ok {
		t.Errorf("expected SelectorExpr on left, got %T", b.Left)
	}
}

func TestParse_SelectorErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing selector body", "items@"},
		{"invalid selector name", "items@middle"},
		{"unclosed inner", "items@(name=foo"},
		{"empty inner", "items@()"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input, 0); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParse_Errors(t *testing.T) {
	bad := []string{
		"(state=draft",
		"",
		"=draft",
		"a=1 AND",
	}
	for _, input := range bad {
		t.Run(input, func(t *testing.T) {
			_, err := Parse(input, 0)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func mustParse(t *testing.T, input string) ast.Expression {
	t.Helper()
	expr, err := Parse(input, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return expr
}

func assertQualifier(t *testing.T, expr ast.Expression) *ast.QualifierExpr {
	t.Helper()
	q, ok := expr.(*ast.QualifierExpr)
	if !ok {
		t.Fatalf("expected *QualifierExpr, got %T", expr)
	}
	return q
}
