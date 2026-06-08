package parser

import (
	"testing"

	"github.com/heyllave/query/ast"
)

func TestParseValue_ProducesValueExpr(t *testing.T) {
	tests := []struct {
		name     string
		q        string
		wantType ast.ValueType
	}{
		{"integer", "42", ast.ValueInteger},
		{"float", "3.14", ast.ValueFloat},
		{"duration", "7d", ast.ValueDuration},
		{"quoted string", `"hi"`, ast.ValueString},
		{"arithmetic", "6*7", ast.ValueArith},
		{"grouped arithmetic", "(1+2)*3", ast.ValueArith},
		{"function call", "now()", ast.ValueFunc},
		{"function minus duration", "now()-7d", ast.ValueArith},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			expr, err := ParseValue(tt.q, 256)
			// Assert
			if err != nil {
				t.Fatalf("ParseValue(%q) error: %v", tt.q, err)
			}
			ve, ok := expr.(*ast.ValueExpr)
			if !ok {
				t.Fatalf("ParseValue(%q) = %T, want *ast.ValueExpr", tt.q, expr)
			}
			if ve.Value.Type != tt.wantType {
				t.Errorf("value type = %v, want %v", ve.Value.Type, tt.wantType)
			}
		})
	}
}

func TestParseValue_Errors(t *testing.T) {
	tests := []struct {
		name string
		q    string
	}{
		{"empty", ""},
		{"trailing token", "42 foo"},
		{"trailing integer", "1 2"},
		{"unclosed paren", "(1+2"},
		{"close paren only", ")"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseValue(tt.q, 256); err == nil {
				t.Errorf("ParseValue(%q) error = nil, want error", tt.q)
			}
		})
	}
}

func TestParseValue_RoundTrip(t *testing.T) {
	tests := []string{"42", "6*7", "(1+2)*3", "now()-7d"}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			expr, err := ParseValue(q, 256)
			if err != nil {
				t.Fatalf("ParseValue(%q) error: %v", q, err)
			}
			if got := ast.String(expr); got != q {
				t.Errorf("round-trip = %q, want %q", got, q)
			}
		})
	}
}

// TestParseValue_TooLong guards the length check on the value entrypoint.
func TestParseValue_TooLong(t *testing.T) {
	if _, err := ParseValue("123456", 3); err == nil {
		t.Error("ParseValue with over-limit input error = nil, want error")
	}
}

// TestParseValue_SelectorsHaveNoValueGrammar confirms list selectors belong to
// the boolean grammar (Parse), not ParseValue. Value position has no `@`
// selector syntax: the paren forms fail to parse, while a bare `@first`/`@last`
// folds into an ordinary string value (`@` is a legal string character, so a
// value like "@user" is allowed).
func TestParseValue_SelectorsHaveNoValueGrammar(t *testing.T) {
	t.Run("paren forms error", func(t *testing.T) {
		for _, q := range []string{"items@any(x=1)", "tags@all(y=2)", "items@(x=1)"} {
			if _, err := ParseValue(q, 256); err == nil {
				t.Errorf("ParseValue(%q) error = nil, want error", q)
			}
		}
	})
	t.Run("bare @first is a string value", func(t *testing.T) {
		expr, err := ParseValue("tags@first", 256)
		if err != nil {
			t.Fatalf("ParseValue error: %v", err)
		}
		ve, ok := expr.(*ast.ValueExpr)
		if !ok {
			t.Fatalf("got %T, want *ast.ValueExpr", expr)
		}
		if ve.Value.Type != ast.ValueString || ve.Value.Str != "tags@first" {
			t.Errorf("got type=%v str=%q, want string \"tags@first\"", ve.Value.Type, ve.Value.Str)
		}
	})
}
