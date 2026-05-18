package parser

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// These tests cover the parser-side improvements that close the gap with
// expr-lang: quoted strings, case-insensitive keywords, IN lists, negated
// comparison operators, and functions / numeric literals outside the
// after-operator value position.

func TestLex_QuotedString_BasicAndEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", `name="hello world"`, "hello world"},
		{"escape quote", `name="say \"hi\""`, `say "hi"`},
		{"escape backslash", `name="back\\slash"`, `back\slash`},
		{"escape newline", `name="line1\nline2"`, "line1\nline2"},
		{"empty", `name=""`, ""},
		{"contains parens", `name="(value)"`, "(value)"},
		{"contains operators", `name="a > b"`, "a > b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := Lex(tt.input, 0)
			if err != nil {
				t.Fatalf("lex: %v", err)
			}
			// tokens: Ident "name", Eq, String "...", EOF
			if tokens[2].Type != token.String {
				t.Fatalf("got %v, want String", tokens[2].Type)
			}
			if tokens[2].Value != tt.want {
				t.Errorf("value: got %q, want %q", tokens[2].Value, tt.want)
			}
			if !tokens[2].Quoted {
				t.Error("expected Quoted=true on the String token")
			}
		})
	}
}

func TestLex_QuotedString_Unterminated(t *testing.T) {
	if _, err := Lex(`name="hello`, 0); err == nil {
		t.Fatal("expected unterminated string error")
	}
}

func TestLex_LowercaseKeywords(t *testing.T) {
	tests := []struct {
		input string
		want  []token.Type
	}{
		{"a=1 and b=2", []token.Type{token.Ident, token.Eq, token.Integer, token.And, token.Ident, token.Eq, token.Integer, token.EOF}},
		{"a=1 or b=2", []token.Type{token.Ident, token.Eq, token.Integer, token.Or, token.Ident, token.Eq, token.Integer, token.EOF}},
		{"not a=1", []token.Type{token.Not, token.Ident, token.Eq, token.Integer, token.EOF}},
		{"a And b Or c", []token.Type{token.Ident, token.And, token.Ident, token.Or, token.Ident, token.EOF}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens, err := Lex(tt.input, 0)
			if err != nil {
				t.Fatalf("lex: %v", err)
			}
			assertTokenTypes(t, tokens, tt.want)
		})
	}
}

func TestLex_NegatedComparisonOperators(t *testing.T) {
	tests := []struct {
		input string
		typ   token.Type
		raw   string
	}{
		{"total!>50000", token.Ngt, "!>"},
		{"total!>=50000", token.Ngte, "!>="},
		{"total!<50000", token.Nlt, "!<"},
		{"total!<=50000", token.Nlte, "!<="},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens, err := Lex(tt.input, 0)
			if err != nil {
				t.Fatalf("lex: %v", err)
			}
			if tokens[1].Type != tt.typ {
				t.Errorf("operator: got %v, want %v", tokens[1].Type, tt.typ)
			}
			if tokens[1].Value != tt.raw {
				t.Errorf("operator value: got %q, want %q", tokens[1].Value, tt.raw)
			}
		})
	}
}

func TestParse_NegatedComparisonsDesugar(t *testing.T) {
	tests := []struct {
		input string
		op    token.Type // operator on the inner qualifier
	}{
		{"total!>50000", token.Gt},
		{"total!>=50000", token.Gte},
		{"total!<50000", token.Lt},
		{"total!<=50000", token.Lte},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := Parse(tt.input, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			u, ok := expr.(*ast.UnaryExpr)
			if !ok {
				t.Fatalf("expected UnaryExpr, got %T", expr)
			}
			if u.Op != token.Not {
				t.Errorf("op: got %v, want NOT", u.Op)
			}
			q, ok := u.Expr.(*ast.QualifierExpr)
			if !ok {
				t.Fatalf("inner: expected QualifierExpr, got %T", u.Expr)
			}
			if q.Operator != tt.op {
				t.Errorf("inner op: got %v, want %v", q.Operator, tt.op)
			}
		})
	}
}

func TestParse_INListExpandsToOrChain(t *testing.T) {
	expr, err := Parse("state IN (draft, issued, paid)", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Should be (state=draft OR state=issued) OR state=paid — left-leaning OR.
	outer, ok := expr.(*ast.BinaryExpr)
	if !ok || outer.Op != token.Or {
		t.Fatalf("outer: expected OR BinaryExpr, got %T (op %v)", expr, outer)
	}
	right, ok := outer.Right.(*ast.QualifierExpr)
	if !ok {
		t.Fatalf("outer.right: expected QualifierExpr, got %T", outer.Right)
	}
	if right.Value.Str != "paid" {
		t.Errorf("right value: got %q, want %q", right.Value.Str, "paid")
	}
	inner, ok := outer.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("outer.left: expected BinaryExpr, got %T", outer.Left)
	}
	leftQ, ok := inner.Left.(*ast.QualifierExpr)
	if !ok {
		t.Fatalf("inner.left: expected QualifierExpr, got %T", inner.Left)
	}
	if leftQ.Value.Str != "draft" || leftQ.Field.String() != "state" {
		t.Errorf("first value: got %q on field %q", leftQ.Value.Str, leftQ.Field.String())
	}
}

func TestParse_INListCaseInsensitive(t *testing.T) {
	if _, err := Parse("state in (draft, issued)", 0); err != nil {
		t.Errorf("lowercase 'in' should parse: %v", err)
	}
}

func TestParse_INListNumeric(t *testing.T) {
	expr, err := Parse("year IN (2020, 2021, 2022)", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Ensure leaf values are typed as integers (not stringified).
	collect := func(e ast.Expression) []*ast.QualifierExpr {
		var out []*ast.QualifierExpr
		ast.Walk(e, func(ex ast.Expression) bool {
			if q, ok := ex.(*ast.QualifierExpr); ok {
				out = append(out, q)
			}
			return true
		})
		return out
	}
	qs := collect(expr)
	if len(qs) != 3 {
		t.Fatalf("got %d qualifiers, want 3", len(qs))
	}
	for _, q := range qs {
		if q.Value.Type != ast.ValueInteger {
			t.Errorf("year IN entry %q: type %v, want ValueInteger", q.Value.Raw, q.Value.Type)
		}
	}
}

func TestParse_INListErrors(t *testing.T) {
	tests := []string{
		"state IN ()",
		"state IN (draft",
		"state IN draft",
		"state IN (draft issued)",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if _, err := Parse(q, 0); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParse_FunctionInValuePosition(t *testing.T) {
	tests := []struct {
		input    string
		funcName string
	}{
		{"created_at>=now()", "now"},
		{"created_at<today()", "today"},
		{"created_at>=daysAgo(7)", "daysAgo"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := Parse(tt.input, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			q, ok := expr.(*ast.QualifierExpr)
			if !ok {
				t.Fatalf("expected QualifierExpr, got %T", expr)
			}
			if q.Value.Type != ast.ValueFunc {
				t.Fatalf("value type: got %v, want ValueFunc", q.Value.Type)
			}
			if q.Value.Func == nil || q.Value.Func.Name != tt.funcName {
				t.Errorf("function name: got %v, want %q", q.Value.Func, tt.funcName)
			}
		})
	}
}

func TestParse_FunctionArgsWithLiterals(t *testing.T) {
	// addDays(date, 7), contains(name, "urgent"), year(now()) — these all used
	// to require field-only args. They now accept literals (numbers, dates,
	// durations, quoted strings) and nested function calls.
	tests := []string{
		`contains(name, "urgent")`,
		`addDays(start, 7)`,
		`between(start, 2026-01-01, 2026-12-31)`,
		`shifted(start, 7d)`,
		`year(now())=2026`,
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if _, err := Parse(q, 0); err != nil {
				t.Errorf("parse: %v", err)
			}
		})
	}
}

func TestParse_RoundTripNewFeatures(t *testing.T) {
	tests := []struct {
		input string
		want  string // optional override; empty = expect equal to input
	}{
		// IN normalizes to OR chain — documented behavior, not a regression.
		{"state IN (draft, issued)", "state=draft OR state=issued"},
		// !> normalizes to NOT (field > value).
		{"total!>50000", "NOT total>50000"},
		// case-insensitive keywords normalize to uppercase in round-trip — the
		// AST does not carry the original casing.
		{"a=1 and b=2", "a=1 AND b=2"},
		// Quoted strings round-trip (Raw preserves quotes).
		{`name="hello world"`, ""},
		// Functions in value position round-trip.
		{`created_at>=now()`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := Parse(tt.input, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := ast.String(expr)
			want := tt.want
			if want == "" {
				want = tt.input
			}
			if got != want {
				t.Errorf("round-trip:\n  got:  %q\n  want: %q", got, want)
			}
		})
	}
}
