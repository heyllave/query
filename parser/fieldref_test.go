package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
)

// firstValue parses q as a value expression and returns the root Value.
func parseFieldRefValue(t *testing.T, q string) *ast.Value {
	t.Helper()
	expr, err := ParseValue(q, 256)
	if err != nil {
		t.Fatalf("ParseValue(%q): %v", q, err)
	}
	ve, ok := expr.(*ast.ValueExpr)
	if !ok {
		t.Fatalf("ParseValue(%q) = %T, want *ast.ValueExpr", q, expr)
	}
	return &ve.Value
}

func TestParseValue_FieldRef(t *testing.T) {
	tests := []struct {
		name       string
		q          string
		wantSegs   []string
		wantQuoted bool
	}{
		{"bare", "[base]", []string{"base"}, false},
		{"dotted path", "[labels.dev]", []string{"labels", "dev"}, false},
		{"quoted hyphen", `["base-1"]`, []string{"base-1"}, true},
		{"quoted dot not split", `["weird.name"]`, []string{"weird.name"}, true},
		{"bare hyphen is one segment", "[base-1]", []string{"base-1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := parseFieldRefValue(t, tt.q)
			if v.Type != ast.ValueFieldRef {
				t.Fatalf("type = %v, want ValueFieldRef", v.Type)
			}
			if v.Quoted != tt.wantQuoted {
				t.Errorf("Quoted = %v, want %v", v.Quoted, tt.wantQuoted)
			}
			if len(v.Field) != len(tt.wantSegs) {
				t.Fatalf("segments = %v, want %v", []string(v.Field), tt.wantSegs)
			}
			for i, s := range tt.wantSegs {
				if v.Field[i] != s {
					t.Errorf("segment %d = %q, want %q", i, v.Field[i], s)
				}
			}
		})
	}
}

func TestParseValue_FieldRefInArithmetic(t *testing.T) {
	// [base]*1.1 must parse to an arithmetic value with a field-ref operand.
	v := parseFieldRefValue(t, "[base]*1.1")
	if v.Type != ast.ValueArith || v.Arith == nil {
		t.Fatalf("type = %v, want ValueArith", v.Type)
	}
	if v.Arith.Left.Type != ast.ValueFieldRef {
		t.Errorf("left operand type = %v, want ValueFieldRef", v.Arith.Left.Type)
	}
}

func TestParseValue_FieldRefErrors(t *testing.T) {
	for _, q := range []string{"[", "[base", `["base]`, "[]", "[ ]"} {
		t.Run(q, func(t *testing.T) {
			if _, err := ParseValue(q, 256); err == nil {
				t.Errorf("ParseValue(%q) error = nil, want error", q)
			}
		})
	}
}

// TestParse_FieldRefOnLeft confirms a bracketed name can start a predicate,
// naming a field the identifier alphabet cannot: ["foo bar"]=1, [labels.dev]=x.
func TestParse_FieldRefOnLeft(t *testing.T) {
	for q, want := range map[string]string{
		"[cpu]>5":                   "cpu",
		`["foo bar"]=1`:             "foo bar",
		`"foo bar"=1`:               "foo bar",
		"[labels.dev]=x":            "labels.dev",
		`["a.b/c"]`:                 "a.b/c",
		"labels.example.com/team=x": "labels.example.com/team",
		"café":                      "café",
		"naïve=1":                   "naïve",
		"日本語.テスト":                   "日本語.テスト",
	} {
		t.Run(q, func(t *testing.T) {
			expr, err := Parse(q, 256)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", q, err)
			}
			var got string
			switch e := expr.(type) {
			case *ast.QualifierExpr:
				got = e.Field.String()
			case *ast.PresenceExpr:
				got = e.Field.String()
			}
			if got != want {
				t.Errorf("Parse(%q) field = %q, want %q", q, got, want)
			}
		})
	}
}

// TestParse_BarewordValuesUnaffected guards that the new value-position bracket
// handling does not regress ordinary bareword/IN/wildcard values.
func TestParse_BarewordValuesUnaffected(t *testing.T) {
	for _, q := range []string{"status=draft", "state IN (draft, issued)", "year=202*", "name=John*"} {
		t.Run(q, func(t *testing.T) {
			if _, err := Parse(q, 256); err != nil {
				t.Errorf("Parse(%q) error: %v", q, err)
			}
		})
	}
}

// TestLex_UnexpectedChar confirms an unexpected character is reported once,
// as the character typed, with a span covering all its bytes. Any non-ASCII
// character is a word character, so the unexpected ones are ASCII symbols.
func TestLex_UnexpectedChar(t *testing.T) {
	_, err := Parse("a=1 $", 0)
	if err == nil {
		t.Fatal("expected an error")
	}
	var list ErrorList
	if !errors.As(err, &list) || len(list) != 1 {
		t.Fatalf("errors = %v, want exactly one", err)
	}
	if got := list[0].Error(); !strings.Contains(got, `"$"`) {
		t.Errorf("error = %q, want it to quote the character", got)
	}
	if list[0].Position.Length != 1 {
		t.Errorf("span length = %d, want 1", list[0].Position.Length)
	}
	// A non-ASCII symbol is a bare term, not an error (issue #23).
	expr, err := Parse("a=1 ¿", 0)
	if err != nil {
		t.Fatalf("Parse(%q): %v", "a=1 ¿", err)
	}
	if got := ast.String(expr); got != "a=1 AND ¿" {
		t.Errorf("String = %q, want %q", got, "a=1 AND ¿")
	}
}
