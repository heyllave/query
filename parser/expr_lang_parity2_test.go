package parser

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// These tests cover the second batch of parity additions: arithmetic in value
// position, @all / @none selectors, and implicit AND (juxtaposition).

func TestParse_Arith_BinaryNumeric(t *testing.T) {
	expr, err := Parse("total>=50000*1.1", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q, ok := expr.(*ast.QualifierExpr)
	if !ok {
		t.Fatalf("expected QualifierExpr, got %T", expr)
	}
	if q.Value.Type != ast.ValueArith {
		t.Fatalf("value type: got %v, want ValueArith", q.Value.Type)
	}
	if q.Value.Arith == nil || q.Value.Arith.Op != "*" {
		t.Fatalf("arith op: got %+v, want *", q.Value.Arith)
	}
	if q.Value.Arith.Left.Type != ast.ValueInteger || q.Value.Arith.Left.Int != 50000 {
		t.Errorf("left: got %+v, want Integer(50000)", q.Value.Arith.Left)
	}
	if q.Value.Arith.Right.Type != ast.ValueFloat || q.Value.Arith.Right.Float != 1.1 {
		t.Errorf("right: got %+v, want Float(1.1)", q.Value.Arith.Right)
	}
}

func TestParse_Arith_Precedence(t *testing.T) {
	// 1 + 2 * 3 should bind * tighter: AST is (1 + (2 * 3)).
	expr, err := Parse("total>=1+2*3", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := expr.(*ast.QualifierExpr)
	if q.Value.Arith.Op != "+" {
		t.Errorf("outer op: got %q, want +", q.Value.Arith.Op)
	}
	if q.Value.Arith.Right.Arith == nil || q.Value.Arith.Right.Arith.Op != "*" {
		t.Errorf("right subtree should be * — got %+v", q.Value.Arith.Right)
	}
}

func TestParse_Arith_ParensOverridePrecedence(t *testing.T) {
	// (1 + 2) * 3 should bind + first.
	expr, err := Parse("total>=(1+2)*3", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := expr.(*ast.QualifierExpr)
	if q.Value.Arith.Op != "*" {
		t.Errorf("outer op: got %q, want *", q.Value.Arith.Op)
	}
	if q.Value.Arith.Left.Arith == nil || q.Value.Arith.Left.Arith.Op != "+" {
		t.Errorf("left subtree should be + — got %+v", q.Value.Arith.Left)
	}
}

func TestParse_Arith_FunctionAndDuration(t *testing.T) {
	expr, err := Parse("created_at>=now()-7d", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := expr.(*ast.QualifierExpr)
	if q.Value.Type != ast.ValueArith {
		t.Fatalf("expected ValueArith, got %v", q.Value.Type)
	}
	if q.Value.Arith.Op != "-" {
		t.Errorf("op: got %q, want -", q.Value.Arith.Op)
	}
	if q.Value.Arith.Left.Type != ast.ValueFunc || q.Value.Arith.Left.Func.Name != "now" {
		t.Errorf("left: got %+v, want now()", q.Value.Arith.Left)
	}
	if q.Value.Arith.Right.Type != ast.ValueDuration {
		t.Errorf("right: got %v, want duration", q.Value.Arith.Right.Type)
	}
}

func TestParse_Arith_WildcardStillWorks(t *testing.T) {
	// `name=John*` and `year=202*` should still lex as wildcards, not as
	// arithmetic with a missing right operand. The disambiguation rule:
	// arithmetic ops need primary-ending operands on BOTH sides.
	for _, q := range []string{"name=John*", "year=202*", "name=*urgent"} {
		expr, err := Parse(q, 0)
		if err != nil {
			t.Fatalf("%s: parse: %v", q, err)
		}
		qe, ok := expr.(*ast.QualifierExpr)
		if !ok {
			t.Fatalf("%s: expected QualifierExpr, got %T", q, expr)
		}
		if !qe.Value.Wildcard {
			t.Errorf("%s: expected wildcard value, got %+v", q, qe.Value)
		}
	}
	// a*b*c remains an invalid wildcard error (more than two stars, not all
	// at the edges).
	if _, err := Parse("name=a*b*c", 0); err == nil {
		t.Error("name=a*b*c should be an invalid-wildcard error")
	}
}

func TestParse_Arith_RoundTrip(t *testing.T) {
	tests := []string{
		"total>=50000*1.1",
		"created_at>=now()-7d",
		"total>=(50000+1000)*1.1",
		"created_at:daysAgo(30)..now()-1d",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			expr, err := Parse(q, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := ast.String(expr); got != q {
				t.Errorf("round-trip:\n  got:  %q\n  want: %q", got, q)
			}
		})
	}
}

func TestParse_SelectorAllAnyNone(t *testing.T) {
	tests := []struct {
		input    string
		selector string
	}{
		{"orders@all(status=shipped)", "all"},
		{"orders@any(status=shipped)", "any"},
		{"orders@none(status=cancelled)", "none"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := Parse(tt.input, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			s, ok := expr.(*ast.SelectorExpr)
			if !ok {
				t.Fatalf("expected SelectorExpr, got %T", expr)
			}
			if s.Selector != tt.selector {
				t.Errorf("selector name: got %q, want %q", s.Selector, tt.selector)
			}
			if s.Inner == nil {
				t.Error("expected inner expression")
			}
		})
	}
}

func TestParse_SelectorAllRoundTrip(t *testing.T) {
	tests := []string{
		"orders@all(status=shipped)",
		"orders@any(status=shipped)",
		"orders@none(status=cancelled)",
		"orders@(status=shipped)", // anonymous EXISTS — unchanged
		"orders@first",
		"orders@last",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			expr, _ := Parse(q, 0)
			if got := ast.String(expr); got != q {
				t.Errorf("round-trip:\n  got:  %q\n  want: %q", got, q)
			}
		})
	}
}

func TestParse_ImplicitAnd(t *testing.T) {
	// state=draft tire_size should parse the same as
	// state=draft AND tire_size.
	expr, err := Parse("state=draft tire_size", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, ok := expr.(*ast.BinaryExpr)
	if !ok || b.Op != token.And {
		t.Fatalf("expected AND BinaryExpr, got %T", expr)
	}
	if _, ok := b.Left.(*ast.QualifierExpr); !ok {
		t.Errorf("left: got %T", b.Left)
	}
	if _, ok := b.Right.(*ast.PresenceExpr); !ok {
		t.Errorf("right: got %T", b.Right)
	}
}

func TestParse_ImplicitAnd_ChainsAndGroups(t *testing.T) {
	tests := []string{
		"state=draft year>2020 total>1000",         // three implicit ANDs
		"(state=draft OR state=issued) total>1000", // group then implicit AND
		"NOT cancelled total>1000",                 // NOT then implicit AND
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if _, err := Parse(q, 0); err != nil {
				t.Errorf("parse: %v", err)
			}
		})
	}
}
