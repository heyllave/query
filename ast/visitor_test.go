package ast

import (
	"testing"

	"github.com/heyllave/query/token"
)

// countVisitor counts nodes of each type.
type countVisitor struct {
	binary, unary, qualifier, presence, group, selector, funcCall int
}

func (v *countVisitor) VisitBinary(*BinaryExpr) int       { v.binary++; return v.binary }
func (v *countVisitor) VisitUnary(*UnaryExpr) int         { v.unary++; return v.unary }
func (v *countVisitor) VisitQualifier(*QualifierExpr) int { v.qualifier++; return v.qualifier }
func (v *countVisitor) VisitPresence(*PresenceExpr) int   { v.presence++; return v.presence }
func (v *countVisitor) VisitGroup(*GroupExpr) int         { v.group++; return v.group }
func (v *countVisitor) VisitSelector(*SelectorExpr) int   { v.selector++; return v.selector }
func (v *countVisitor) VisitFuncCall(*FuncCallExpr) int   { v.funcCall++; return v.funcCall }

func TestVisit(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expression
		check    func(v *countVisitor) bool
		wantZero bool // expect Visit to return zero value
	}{
		{"binary", &BinaryExpr{}, func(v *countVisitor) bool { return v.binary == 1 }, false},
		{"unary", &UnaryExpr{}, func(v *countVisitor) bool { return v.unary == 1 }, false},
		{"qualifier", &QualifierExpr{}, func(v *countVisitor) bool { return v.qualifier == 1 }, false},
		{"presence", &PresenceExpr{}, func(v *countVisitor) bool { return v.presence == 1 }, false},
		{"group", &GroupExpr{}, func(v *countVisitor) bool { return v.group == 1 }, false},
		{"selector", &SelectorExpr{}, func(v *countVisitor) bool { return v.selector == 1 }, false},
		{"funcCall", &FuncCallExpr{}, func(v *countVisitor) bool { return v.funcCall == 1 }, false},
		{"nil returns zero", nil, func(*countVisitor) bool { return true }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &countVisitor{}
			result := Visit[int](v, tt.expr)
			if !tt.check(v) {
				t.Errorf("visitor state wrong: %+v", v)
			}
			if tt.wantZero && result != 0 {
				t.Errorf("expected zero value, got %d", result)
			}
		})
	}
}

func TestSQLOperator(t *testing.T) {
	tests := []struct {
		name     string
		op       token.Type
		wildcard bool
		want     string
	}{
		{"eq", token.Eq, false, "="},
		{"neq", token.Neq, false, "!="},
		{"gt", token.Gt, false, ">"},
		{"gte", token.Gte, false, ">="},
		{"lt", token.Lt, false, "<"},
		{"lte", token.Lte, false, "<="},
		{"wildcard", token.Eq, true, "LIKE"},
		{"default", token.Range, false, "="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SQLOperator(tt.op, tt.wildcard); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWildcardToLike(t *testing.T) {
	tests := []struct {
		name, pattern, want string
	}{
		{"prefix", "John*", "John%"},
		{"suffix", "*yota", "%yota"},
		{"contains", "*test*", "%test%"},
		{"escape percent", "100%", `100\%`},
		{"escape underscore", "under_score", `under\_score`},
		{"no wildcard", "plain", "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WildcardToLike(tt.pattern); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
