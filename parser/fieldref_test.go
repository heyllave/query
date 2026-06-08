package parser

import (
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

// TestParse_FieldRefOnLeftRejected confirms a bracket cannot start a boolean
// predicate: brackets are a value-position sigil only.
func TestParse_FieldRefOnLeftRejected(t *testing.T) {
	if _, err := Parse("[cpu]>5", 256); err == nil {
		t.Error("Parse(\"[cpu]>5\") error = nil, want error (LHS bracket unsupported)")
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
