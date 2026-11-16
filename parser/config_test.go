package parser_test

import (
	"strings"
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

// A hint that rewrites the query is only offered if the caller would accept
// it typed back, which under a strict option means under that option.
func TestParseWith_HintsFollowTheStrictOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		cfg      parser.Config
		code     parser.Code
		wantHint string // substring; "" means only that the listed text is absent
		notHint  string
	}{
		{
			name: "a rewrite the strict grammar refuses is not suggested", input: "a=1 b==2",
			cfg: parser.Config{ExplicitAnd: true}, code: parser.CodeUnexpectedChar,
			wantHint: "equality is a single =", notHint: "a=1 b=2",
		},
		{
			name: "the same rewrite is suggested by default", input: "a=1 b==2",
			cfg: parser.Config{}, code: parser.CodeUnexpectedChar, wantHint: "a=1 b=2",
		},
		{
			name: "a comparison inside a value says both ways out", input: "a=1+b=2",
			cfg: parser.Config{StrictValues: true}, code: parser.CodeUnexpectedChar,
			wantHint: `joined with AND`, notHint: "starts with",
		},
		{
			name: "and shows the value quoted", input: "a=1+b=2",
			cfg: parser.Config{StrictValues: true}, code: parser.CodeUnexpectedChar, wantHint: `"1+b=2"`,
		},
		{
			name: "a keyed lookup is told to use a dot", input: `labels["team"]=x`,
			cfg: parser.Config{ExplicitAnd: true}, code: parser.CodeMissingAnd,
			wantHint: "labels.team=x", notHint: "AND",
		},
		{
			name: "an unquoted key too", input: "a=1 AND labels[team]=x",
			cfg: parser.Config{ExplicitAnd: true}, code: parser.CodeMissingAnd,
			wantHint: "a=1 AND labels.team=x", notHint: "join two",
		},
		{
			name: "a field reference after a space is still two conditions", input: "a=1 [b]=2",
			cfg: parser.Config{ExplicitAnd: true}, code: parser.CodeMissingAnd, wantHint: "join two conditions",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			_, err := parser.ParseWith(tc.input, tc.cfg)

			// Assert
			var hint string
			found := false
			for _, e := range parser.Errors(err) {
				if e.Code == tc.code {
					hint, found = e.Hint, true
					break
				}
			}
			if !found {
				t.Fatalf("ParseWith(%q) reported %v, want %q among them", tc.input, codesOf(err), tc.code)
			}
			if tc.wantHint != "" && !strings.Contains(hint, tc.wantHint) {
				t.Errorf("hint = %q, want it to contain %q", hint, tc.wantHint)
			}
			if tc.notHint != "" && strings.Contains(hint, tc.notHint) {
				t.Errorf("hint = %q, must not contain %q", hint, tc.notHint)
			}
		})
	}
}

func TestParseWith_StrictErrorsAreReportedOnceAndTogether(t *testing.T) {
	both := parser.Config{ExplicitAnd: true, StrictValues: true}

	t.Run("a doubled operator is one error, as by default", func(t *testing.T) {
		_, strictErr := parser.ParseWith("a==b", parser.Config{StrictValues: true})
		_, defaultErr := parser.Parse("a==b", 0)
		if got, want := len(parser.Errors(strictErr)), len(parser.Errors(defaultErr)); got != want || got != 1 {
			t.Errorf("strict reports %d errors %v, default %d; want 1 each", got, codesOf(strictErr), want)
		}
	})

	t.Run("a missing connective is reported beside a lexical error after it", func(t *testing.T) {
		_, err := parser.ParseWith("a=1 b==2", both)
		errs := parser.Errors(err)
		if len(errs) != 2 || errs[0].Code != parser.CodeMissingAnd || errs[1].Code != parser.CodeUnexpectedChar {
			t.Fatalf("reported %v, want [missingAnd unexpectedChar]", codesOf(err))
		}
		if errs[0].Position.Offset != 4 {
			t.Errorf("missingAnd at %d, want 4", errs[0].Position.Offset)
		}
	})

	t.Run("nothing is inferred after the first lexical error", func(t *testing.T) {
		for _, input := range []string{`a="x y z`, "a==b c=1", "a=1 # b c"} {
			_, err := parser.ParseWith(input, both)
			for _, e := range parser.Errors(err) {
				if e.Code == parser.CodeMissingAnd && e.Position.Offset > parser.Errors(err)[0].Position.Offset {
					t.Errorf("%q: missingAnd at %d, after the first error", input, e.Position.Offset)
				}
			}
		}
	})
}

func TestConfig_Strict(t *testing.T) {
	got := parser.Config{MaxLength: 7}.Strict()
	if !got.ExplicitAnd || !got.StrictValues || got.MaxLength != 7 {
		t.Errorf("Strict() = %+v, want both switches on and MaxLength kept", got)
	}
}
