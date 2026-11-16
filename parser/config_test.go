package parser_test

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

func codesOf(err error) []parser.Code {
	var out []parser.Code
	for _, e := range parser.Errors(err) {
		out = append(out, e.Code)
	}
	return out
}

func TestParseWith_ExplicitAnd(t *testing.T) {
	strict := parser.Config{ExplicitAnd: true}

	t.Run("refuses juxtaposition", func(t *testing.T) {
		tests := []struct {
			name, input string
			wantOffset  int
		}{
			{"two conditions", "a=1 b=2", 4},
			{"two bare terms", "frontend checkout", 9},
			{"term then condition", "frontend cluster=prod", 9},
			{"condition then group", "a=1 (b=2 OR c=3)", 4},
			{"condition then NOT", "a=1 NOT b=2", 4},
			{"inside a group", "(a=1 b=2)", 5},
			{"inside a selector", "forks@(name=a kind=b)", 14},
			{"bracket after a name", "labels[team]=x", 6},
			{"quoted key after a name", `labels["team"]=x`, 6},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				// Act
				_, err := parser.ParseWith(tc.input, strict)

				// Assert
				errs := parser.Errors(err)
				if len(errs) != 1 {
					t.Fatalf("ParseWith(%q) reported %v, want exactly one error", tc.input, codesOf(err))
				}
				if errs[0].Code != parser.CodeMissingAnd {
					t.Errorf("code = %q, want %q", errs[0].Code, parser.CodeMissingAnd)
				}
				if errs[0].Position.Offset != tc.wantOffset {
					t.Errorf("offset = %d, want %d", errs[0].Position.Offset, tc.wantOffset)
				}
				if errs[0].Hint == "" {
					t.Error("missingAnd carries no hint")
				}
			})
		}
	})

	t.Run("one error per missing connective", func(t *testing.T) {
		_, err := parser.ParseWith("a b c", strict)
		if got := len(parser.Errors(err)); got != 2 {
			t.Errorf("got %d errors (%v), want 2", got, codesOf(err))
		}
	})

	t.Run("accepts everything written with connectives", func(t *testing.T) {
		tests := []string{
			"a=1 AND b=2",
			"a=1 and b=2",
			"a=1 OR b=2 AND c=3",
			"NOT (a=1 OR b=2) AND NOT c",
			"frontend AND cluster=prod",
			`name="John Doe"`,
			"state IN (draft, issued)",
			"state NOT IN (draft, issued)",
			"total>10 - 3",
			"created_at:2026-01-01..2026-03-31",
			"forks@(name=a AND kind=b)",
			"forks@all(kind=Deployment) AND NOT labels.ephemeral",
			"lower(name)=jo* AND createdAt>=now()-7d",
			"[labels.x/y]=1",
		}
		for _, input := range tests {
			t.Run(input, func(t *testing.T) {
				// Act
				strictExpr, err := parser.ParseWith(input, strict)

				// Assert: accepted, and with the meaning the default grammar gives it.
				if err != nil {
					t.Fatalf("ParseWith(%q) failed: %v", input, err)
				}
				lenient, err := parser.Parse(input, 0)
				if err != nil {
					t.Fatalf("Parse(%q) failed: %v", input, err)
				}
				if got, want := ast.String(strictExpr), ast.String(lenient); got != want {
					t.Errorf("strict reads %q, default reads %q", got, want)
				}
			})
		}
	})

	t.Run("off by default", func(t *testing.T) {
		if _, err := parser.ParseWith("a=1 b=2", parser.Config{}); err != nil {
			t.Errorf("default grammar refused an implicit AND: %v", err)
		}
		if _, err := parser.Parse("a=1 b=2", 0); err != nil {
			t.Errorf("Parse refused an implicit AND: %v", err)
		}
	})
}

func TestParseWith_StrictValues(t *testing.T) {
	strict := parser.Config{StrictValues: true}

	t.Run("refuses a comparison character in an unquoted value", func(t *testing.T) {
		tests := []struct {
			name, input string
			wantOffset  int
		}{
			{"old plus-joined conditions", "a=1+b=2", 5},
			{"chained equals", "a=b=c", 3},
			{"greater-than inside", "a=x>y", 3},
			{"less-than inside", "a=x<y", 3},
			{"url with a query string", "url=https://x/?a=b", 16},
			{"in an IN list", "a IN (x=1, y)", 7},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				// Act
				_, err := parser.ParseWith(tc.input, strict)

				// Assert
				errs := parser.Errors(err)
				if len(errs) == 0 {
					t.Fatalf("ParseWith(%q) succeeded", tc.input)
				}
				if errs[0].Code != parser.CodeUnexpectedChar {
					t.Errorf("code = %q, want %q", errs[0].Code, parser.CodeUnexpectedChar)
				}
				if errs[0].Position.Offset != tc.wantOffset {
					t.Errorf("offset = %d, want %d", errs[0].Position.Offset, tc.wantOffset)
				}
			})
		}
	})

	t.Run("accepts quoted and ordinary values", func(t *testing.T) {
		tests := []string{
			`url="https://x/?a=b"`,
			`note="a<b"`,
			"a=1 AND b=2",
			"total>=10",
			"total!>=10",
			"id=550e8400-e29b-41d4-a716-446655440000",
			"image=example.com/my-app:dev-abcdef",
			"name=checkout*",
			"createdAt>=now()-7d",
			"total>=(50000+1000)*1.1",
		}
		for _, input := range tests {
			t.Run(input, func(t *testing.T) {
				if _, err := parser.ParseWith(input, strict); err != nil {
					t.Errorf("ParseWith(%q) failed: %v", input, err)
				}
			})
		}
	})

	t.Run("off by default", func(t *testing.T) {
		if _, err := parser.Parse("a=1+b=2", 0); err != nil {
			t.Errorf("default grammar refused it: %v", err)
		}
	})
}
