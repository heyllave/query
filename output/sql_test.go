package output

import (
	"testing"

	"github.com/heyllave/query/token"
)

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
		{"wildcard neq", token.Neq, true, "NOT LIKE"},
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
	tests := map[string]string{
		"John*":    "John%",
		"*yota":    "%yota",
		"*test*":   "%test%",
		"100%":     `100\%`,
		"50%*":     `50\%%`,
		"a_b*":     `a\_b%`,
		`a\*b*`:    "a*b%",
		`back\\s*`: `back\\s%`,
		`c\\*`:     `c\\%`,
		"plain":    "plain",
	}
	for in, want := range tests {
		if got := WildcardToLike(in); got != want {
			t.Errorf("WildcardToLike(%q) = %q, want %q", in, got, want)
		}
	}
}
