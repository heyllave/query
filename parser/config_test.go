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

// Under ExplicitAnd a word borrowed from SQL fails as a missing AND. The
// general advice for that code is to add the AND, which for these queries
// gives another wrong query, so the SQL word's own advice takes its place.
func TestParseWith_MissingAndFromSQLGetsTheSQLHint(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		not   string
	}{
		{"between", "x BETWEEN 1 AND 5", "x:1..5", "join two conditions"},
		{"between after a clause", "a=1 AND x BETWEEN 1 AND 5", "a=1 AND x:1..5", "join two conditions"},
		{"like", "name LIKE john", "no LIKE", "join two conditions"},
		{"is null", "name IS NULL", "NOT field", "join two conditions"},
		{"is not null", "name IS NOT NULL", "NOT field", "join two conditions"},
		{"contains", "name contains john", "name=*john*", "join two conditions"},
		{"select", "SELECT a FROM t", "only conditions", "join two conditions"},
		{"limit", "state=draft LIMIT 5", "only conditions", "join two conditions"},
		{"order by", "state=draft ORDER BY total", "only conditions", "join two conditions"},
		{"plain juxtaposition", "a=1 b=2", "join two conditions", "LIKE"},
		{"unquoted second word", "name=john smith", "join two conditions", "LIKE"},
		{"field named like SQL", "limit>5 b=2", "join two conditions", "only conditions"},
		{"SQL word inside quotes", `name="is null" b=2`, "join two conditions", "NOT field"},
		{"SQL word in an earlier clause", "a LIKE b AND c=1 d=2", "no LIKE", ""},
		{"bracket lookup", `labels["team"]=x`, "labels.team=x", "join two conditions"},
	}
	for _, cfg := range []parser.Config{{ExplicitAnd: true}, parser.Config{}.Strict()} {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Act
				_, err := parser.ParseWith(tt.input, cfg)

				// Assert
				errs := parser.Errors(err)
				if len(errs) == 0 || errs[0].Code != parser.CodeMissingAnd {
					t.Fatalf("ParseWith(%q) = %v, want missingAnd first", tt.input, err)
				}
				hint := errs[0].Hint
				if !strings.Contains(hint, tt.want) || (tt.not != "" && strings.Contains(hint, tt.not)) {
					t.Errorf("ParseWith(%q) hint = %q, want one containing %q and not %q", tt.input, hint, tt.want, tt.not)
				}
			})
		}
	}
}

// The later clause of a query is explained on its own terms: the SQL word
// before the AND does not reach it.
func TestParseWith_SQLHintStopsAtTheClause(t *testing.T) {
	// Act
	_, err := parser.ParseWith("a LIKE b AND c=1 d=2", parser.Config{ExplicitAnd: true})

	// Assert
	errs := parser.Errors(err)
	// The LIKE is one mistake, reported once; the missing AND after it is
	// another.
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), err)
	}
	if last := errs[1].Hint; !strings.Contains(last, "join two conditions") {
		t.Errorf("hint for the last clause = %q, want the general advice", last)
	}
}

// An SQL word is one mistake however many connectives it leaves missing, but
// only the errors that stand next to it are folded into it. A second SQL
// word, in the same clause or another, is its own error; and a connective
// that is really missing further on is kept, so fixing the SQL word does not
// reveal an error the first answer hid.
func TestParseWith_SQLWordIsReportedOncePerUse(t *testing.T) {
	const general = "join two conditions"
	tests := []struct {
		name    string
		input   string
		offsets []int
		hints   []string
	}{
		{"like", "name LIKE john", []int{5}, []string{"no LIKE"}},
		{"is null", "name IS NULL", []int{5}, []string{"NOT field"}},
		{"is not null", "name IS NOT NULL", []int{5}, []string{"NOT field"}},
		{"order by", "state=draft ORDER BY total", []int{12}, []string{"only conditions"}},
		{"statement", "SELECT a FROM t", []int{7}, []string{"only conditions"}},
		{"quoted value", `name LIKE "john"`, []int{5}, []string{"no LIKE"}},
		{"two clauses", "a LIKE b AND c LIKE d", []int{2, 15}, []string{"no LIKE", "no LIKE"}},
		{"three clauses", "a LIKE b AND c LIKE d AND e LIKE f", []int{2, 15, 28}, []string{"no LIKE", "no LIKE", "no LIKE"}},
		{"two missing tests", "a IS NULL AND b IS NULL", []int{2, 16}, []string{"NOT field", "NOT field"}},
		{"statement around a connective", "SELECT a AND b LIMIT 5", []int{7, 15}, []string{"only conditions", "only conditions"}},
		{"missing connectives after it", "a LIKE b c=1 d=2", []int{2, 9, 13}, []string{"no LIKE", general, general}},
		{"after a group", "(a LIKE b) c=1 d=2", []int{3, 11, 15}, []string{"no LIKE", general, general}},
		{"twice in one clause", "a LIKE b c LIKE d", []int{2, 9, 11}, []string{"no LIKE", general, "no LIKE"}},
		{"no SQL word", "a b c", []int{2, 4}, []string{general, general}},
		{"a number after it", "a=1 LIMIT 5", []int{4}, []string{"only conditions"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := parser.ParseWith(tt.input, parser.Config{ExplicitAnd: true})

			// Assert
			errs := parser.Errors(err)
			if len(errs) != len(tt.offsets) {
				t.Fatalf("ParseWith(%q) gave %d errors, want %d: %v", tt.input, len(errs), len(tt.offsets), err)
			}
			for i, e := range errs {
				if e.Code != parser.CodeMissingAnd || e.Position.Offset != tt.offsets[i] || !strings.Contains(e.Hint, tt.hints[i]) {
					t.Errorf("error %d = %s at %d, hint %q; want missingAnd at %d with a hint containing %q",
						i, e.Code, e.Position.Offset, e.Hint, tt.offsets[i], tt.hints[i])
				}
			}
		})
	}
}

// A missing connective does not hide the token the parser stops at after it:
// the writer who adds the AND is not then handed an error the first answer
// left out.
func TestParseWith_MissingAndKeepsTheErrorAfterIt(t *testing.T) {
	// Act
	_, err := parser.ParseWith("a=1 x 5 b=2", parser.Config{ExplicitAnd: true})

	// Assert
	errs := parser.Errors(err)
	if len(errs) != 2 || errs[0].Code != parser.CodeMissingAnd || errs[0].Position.Offset != 4 ||
		errs[1].Code != parser.CodeUnexpected || errs[1].Position.Offset != 6 {
		t.Fatalf("got %v, want missingAnd at 4 then unexpected at 6", err)
	}

	// The default grammar reports the same stray token.
	_, err = parser.ParseWith("a=1 x 5 b=2", parser.Config{})
	if errs := parser.Errors(err); len(errs) != 1 || errs[0].Code != parser.CodeUnexpected || errs[0].Position.Offset != 6 {
		t.Fatalf("lenient: got %v, want unexpected at 6", err)
	}
}

// How many errors come back does not depend on how many of them get a hint.
func TestParseWith_SQLWordFoldingIgnoresTheHintLimit(t *testing.T) {
	// Act
	_, err := parser.ParseWith("a LIKE b c d e f g h i j k l", parser.Config{ExplicitAnd: true})

	// Assert: the LIKE, then one for each of the ten words after b.
	if errs := parser.Errors(err); len(errs) != 11 {
		t.Fatalf("got %d errors, want 11: %v", len(errs), err)
	}
}

// A rewrite offered for a missing AND is itself accepted under the same
// options.
func TestParseWith_MissingAndRewritesParseStrictly(t *testing.T) {
	strict := parser.Config{}.Strict()
	for _, rewrite := range []string{"x:1..5", "a=1 AND x:1..5", "labels.team=x"} {
		t.Run(rewrite, func(t *testing.T) {
			if _, err := parser.ParseWith(rewrite, strict); err != nil {
				t.Errorf("ParseWith(%q) under strict options: %v", rewrite, err)
			}
		})
	}
}

// One SQL word is one mistake, however many connectives it leaves missing.
func TestParseWith_SQLWordIsReportedOnce(t *testing.T) {
	strict := parser.Config{ExplicitAnd: true}
	tests := []struct {
		name, input string
		want        int
		wantHint    string
	}{
		{"LIKE", "name LIKE john", 1, "there is no LIKE"},
		{"IS NULL", "name IS NULL", 1, "missing value"},
		{"the general advice still counts each one", "a b c", 2, "join two conditions"},
		{"two keyed lookups are two mistakes", "a[x]=1 AND b[y]=2", 2, "after a dot"},
		{"an SQL word, then an unrelated missing connective", "name LIKE john AND a b", 2, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			_, err := parser.ParseWith(tc.input, strict)

			// Assert
			errs := parser.Errors(err)
			if len(errs) != tc.want {
				var hints []string
				for _, e := range errs {
					hints = append(hints, e.Hint)
				}
				t.Fatalf("ParseWith(%q) reported %d errors %q, want %d", tc.input, len(errs), hints, tc.want)
			}
			if tc.wantHint != "" && !strings.Contains(errs[0].Hint, tc.wantHint) {
				t.Errorf("hint = %q, want it to contain %q", errs[0].Hint, tc.wantHint)
			}
		})
	}
}

// A NOT with nothing to negate is a dangling connective under ExplicitAnd,
// and the term "not" under the default grammar.
func TestParseWith_DanglingNotIsRefusedUnderExplicitAnd(t *testing.T) {
	strict := parser.Config{ExplicitAnd: true}
	for _, q := range []string{"a=1 AND NOT", "NOT", "(NOT)", "NOT OR b=2", "a=1 AND NOT AND b=2", "a=1 OR NOT"} {
		// Act
		_, err := parser.ParseWith(q, strict)

		// Assert
		errs := parser.Errors(err)
		if len(errs) != 1 || errs[0].Code != parser.CodeExpectedField {
			t.Errorf("%q: got %v, want one expectedField error", q, err)
		}
		if _, err := parser.ParseWith(q, parser.Config{}); err != nil {
			t.Errorf("%q: the default grammar refused it: %v", q, err)
		}
	}
}

// NOT stays a name wherever something names it as one.
func TestParseWith_NotAsAFieldNameSurvivesExplicitAnd(t *testing.T) {
	strict := parser.Config{ExplicitAnd: true}
	for _, q := range []string{"not=1", "NOT=1", "not.x=1", "not@first", "not IN (a, b)", "labels.not=x", `"not"`, "a=1 AND NOT b=2", "NOT NOT a=1"} {
		// Act
		got, err := parser.ParseWith(q, strict)

		// Assert
		if err != nil {
			t.Errorf("%q: %v", q, err)
			continue
		}
		want, err := parser.ParseWith(q, parser.Config{})
		if err != nil || ast.String(got) != ast.String(want) {
			t.Errorf("%q: strict parsed to %s, default to %s (%v)", q, ast.String(got), ast.String(want), err)
		}
	}
}

// A word that cannot stand alone as a term is shown in quotes.
func TestParseWith_BareWordHintOffersQuotes(t *testing.T) {
	tests := []struct{ q, hint string }{
		{"812", `a number or a date alone is not a condition; to search for the text, put it in quotes: "812"`},
		{"a=1 AND 2026-11-16", `a number or a date alone is not a condition; to search for the text, put it in quotes: "2026-11-16"`},
		{"v1.2", `a part of a name cannot start with a digit; to search for the text, put it in quotes: "v1.2"`},
		{"foo@bar.com", `@ asks something of a list: tags@first; to search for the text, put it in quotes: "foo@bar.com"`},
		{"checkout*", `a * pattern goes after a field and an operator: name=checkout*; to search for the text, put it in quotes: "checkout*"`},
		{"(*checkout) AND b=1", `a * pattern goes after a field and an operator: name=*checkout; to search for the text, put it in quotes: "*checkout"`},
		// Not a bare word: the existing advice stands.
		{"tags.0=x", "list elements are reached with a selector, not an index: tags@first or tags@(name=x)"},
		{"tags@frist", "did you mean @first?"},
		{"3<total", "the field goes on the left: total>3"},
		{"total*2>3", "arithmetic goes on the right of the operator or inside a function: float([total]*2)>3"},
	}
	for _, tt := range tests {
		// Act
		_, err := parser.ParseWith(tt.q, parser.Config{}.Strict())

		// Assert
		errs := parser.Errors(err)
		if len(errs) == 0 || errs[0].Hint != tt.hint {
			t.Errorf("%q: got %v, want hint %q", tt.q, err, tt.hint)
		}
	}
}
