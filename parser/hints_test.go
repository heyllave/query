package parser

import (
	"strings"
	"testing"
)

// hintCase is one mistaken query: the code it must fail with (codes never
// change) and a fragment the hint must contain (wording may change, so the
// fragment is the part that carries the advice).
type hintCase struct {
	name  string
	query string
	code  Code
	hint  string
}

func runHintCases(t *testing.T, cases []hintCase, parse func(string, int) error) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			errs := Errors(parse(tt.query, 0))

			// Assert
			if len(errs) == 0 {
				t.Fatalf("%q parsed, want error %s", tt.query, tt.code)
			}
			first := errs[0]
			if first.Code != tt.code {
				t.Errorf("%q code = %s, want %s", tt.query, first.Code, tt.code)
			}
			if !strings.Contains(first.Hint, tt.hint) {
				t.Errorf("%q hint = %q, want one containing %q", tt.query, first.Hint, tt.hint)
			}
		})
	}
}

func TestParse_Hints(t *testing.T) {
	parse := func(q string, n int) error {
		_, err := Parse(q, n)
		return err
	}
	runHintCases(t, []hintCase{
		// Arithmetic left of the operator, with the comparison rewritten.
		{"bracketed field times number", "[base]*1.1 > 3", CodeUnexpected, "float([base]*1.1)>3"},
		{"bare field times number", "total*2>3", CodeUnexpectedChar, "float([total]*2)>3"},
		{"grouped arithmetic", "([base]*1.1)>3", CodeUnclosedParen, "float([base]*1.1)>3"},
		{"call plus number", "len(name)+1>3", CodeUnexpected, "float(len(name)+1)>3"},
		{"arithmetic left, field right", "now()-7d<created_at", CodeUnexpected, "created_at>now()-7d"},
		{"arithmetic left in a longer query", "a=1 AND total*2>3", CodeUnexpectedChar, "arithmetic goes on the right"},

		// The literal written first.
		{"number first", "3<total", CodeExpectedField, "total>3"},
		{"number first, equality", "5=replicas", CodeExpectedField, "replicas=5"},
		{"date first", "2026-01-01<=created_at", CodeExpectedField, "created_at>=2026-01-01"},
		{"number first, arithmetic after", "3<[base]*1.1", CodeExpectedField, "starts with a field"},

		// Operators borrowed from other languages.
		{"double equals", "state==draft", CodeUnexpectedChar, "single ="},
		{"arrow", "total=>5", CodeUnexpectedChar, ">="},
		{"reverse arrow", "total=<5", CodeUnexpectedChar, "<="},
		{"sql not equal", "state<>draft", CodeUnexpectedChar, "!="},
		{"spaced operator", "total > = 5", CodeUnexpectedChar, "no space"},
		{"spaced not equal", "total ! = 5", CodeUnexpectedChar, "!= with no space"},
		{"double ampersand", "a=1 && b=2", CodeUnexpectedChar, "AND"},
		{"single ampersand", "a=1 & b=2", CodeUnexpectedChar, "AND"},
		{"double pipe", "a=1 || b=2", CodeUnexpectedChar, "OR"},
		{"leading bang", "!state=draft", CodeUnexpectedChar, "NOT"},
		{"tilde", "name~john", CodeUnexpectedChar, "wildcard"},
		{"comment", "state=draft -- note", CodeUnexpectedChar, "comments"},

		// SQL.
		{"like", "name LIKE john%", CodeUnexpectedChar, "no LIKE"},
		{"between", "total BETWEEN 1 AND 5", CodeUnexpected, "total:1..5"},
		{"select", "SELECT * FROM t", CodeUnexpectedChar, "only conditions"},
		{"limit", "state=draft LIMIT 5", CodeUnexpected, "only conditions"},

		// Missing pieces.
		{"empty", "", CodeExpectedField, "write a condition"},
		{"missing value", "state=", CodeExpectedValue, "add a value"},
		{"missing field", "=draft", CodeExpectedField, "field name is missing"},
		{"trailing and", "state=draft AND", CodeExpectedField, "missing after AND"},
		{"leading or", "OR state=draft", CodeExpectedField, "missing before OR"},
		{"second comparison without a field", "total>=1 AND <=5", CodeExpectedField, "its own field"},
		{"empty group", "()", CodeExpectedField, "inside the parentheses"},
		{"dangling arithmetic", "total>[base]*", CodeExpectedValue, "each side"},

		// Brackets, quotes and lists.
		{"unclosed group", "(state=draft", CodeUnclosedParen, `closing ")"`},
		{"stray close", "state=draft)", CodeUnexpected, "no matching"},
		{"unclosed quote", `name="john`, CodeUnclosedString, "closing quote"},
		{"unclosed field", "total>[base", CodeUnclosedFieldRef, `closing "]"`},
		{"in without parens", "state IN draft", CodeExpectedInList, "parentheses"},
		{"in with brackets", "state IN [a, b]", CodeUnclosedFieldRef, "parentheses"},
		{"in without commas", "state IN (a b)", CodeUnclosedIn, "commas"},
		{"empty in", "state IN ()", CodeEmptyInList, "at least one"},
		{"list after not", "state NOT (a,b)", CodeUnclosedParen, "NOT IN"},
		{"call left open", "lower(name=x", CodeUnclosedFuncArgs, "close the call"},

		// Ranges.
		{"colon as equals", "name:john", CodeExpectedRange, "name=john"},
		{"dash range", "total:1-5", CodeExpectedRange, "total:1..5"},
		{"open range", "total:1..", CodeExpectedValue, "both ends"},

		// Values.
		{"star in the middle", "name=jo*hn", CodeInvalidWildcard, "start or end"},
		{"bare field times number on the right", "total>=base*1.1", CodeInvalidWildcard, "[base]*1.1"},
		{"bad date", "created_at>2026-13-45", CodeInvalidDate, "YYYY-MM-DD"},

		// Selectors and names.
		{"selector without a condition", "tags@all", CodeExpectedSelector, "tags@all(name=x)"},
		{"selector synonym", "tags@some(x=1)", CodeExpectedSelector, "tags@any"},
		{"selector typo", "tags@frist", CodeExpectedSelector, "@first"},
		{"index", "tags.0=x", CodeExpectedField, "selector"},
		{"trailing dot", "labels.=x", CodeExpectedField, "after the dot"},
	}, parse)
}

func TestParseValue_Hints(t *testing.T) {
	parse := func(q string, n int) error {
		_, err := ParseValue(q, n)
		return err
	}
	runHintCases(t, []hintCase{
		{"bare field times number", "base*1.1", CodeInvalidWildcard, "[base]*1.1"},
		{"power", "2**3", CodeInvalidWildcard, "pow(2, 3)"},
		{"comparison", "[base]*1.1 > 3", CodeUnexpected, "as a condition"},
		{"connective", "state=draft AND total>5", CodeUnexpected, "as a condition"},
		{"spelled-out unit", "now() - 7 days", CodeUnexpected, "7d"},
		{"unknown unit", "now()-7x", CodeUnexpected, "one unit"},
		{"mod", "10 mod 3", CodeUnexpected, "10%3"},
		{"dangling operator", "[base]*", CodeExpectedValue, "each side"},
		{"doubled operator", "1 ++ 2", CodeUnexpectedChar, "each side"},
		{"two values", "1 2", CodeUnexpected, "one value"},
		{"empty", "", CodeExpectedValue, "write a value"},
		{"unclosed group", "(1+2", CodeUnclosedParen, `closing ")"`},
	}, parse)
}

// A field may be named after an SQL word; using it as a field must not be
// mistaken for SQL.
func TestParse_HintsLeaveSQLNamedFieldsAlone(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"field named limit", "limit>5 AND state==draft"},
		{"field named from", "from=a AND x==1"},
		{"nested key named order", "labels.order=1 AND x==1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := Parse(tt.query, 0)

			// Assert
			errs := Errors(err)
			if len(errs) == 0 {
				t.Fatalf("%q parsed, want an error", tt.query)
			}
			if !strings.Contains(errs[0].Hint, "single =") {
				t.Errorf("%q hint = %q, want the == advice", tt.query, errs[0].Hint)
			}
		})
	}
}

func TestParse_RepeatedCharacterIsOneError(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		offset int
		length int
	}{
		{"double ampersand", "a=1 && b=2", 4, 2},
		{"double pipe", "a=1 || b=2", 4, 2},
		{"triple ampersand", "a=1 &&& b=2", 4, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := Parse(tt.query, 0)

			// Assert
			errs := Errors(err)
			if len(errs) != 1 {
				t.Fatalf("%q gave %d errors, want 1: %v", tt.query, len(errs), err)
			}
			if got := errs[0].Position; got.Offset != tt.offset || got.Length != tt.length {
				t.Errorf("%q position = %d+%d, want %d+%d", tt.query, got.Offset, got.Length, tt.offset, tt.length)
			}
		})
	}
}

// Every rewrite a hint proposes for these queries must itself parse: a hint
// that suggests a broken query is worse than none.
func TestParse_SuggestedRewritesParse(t *testing.T) {
	tests := []struct {
		query   string
		rewrite string
	}{
		{"status==active", "status=active"},
		{"total=>5 AND a=1", "total>=5 AND a=1"},
		{"total=<5", "total<=5"},
		{"a<>b", "a!=b"},
		{"total > = 5", "total >= 5"},
		{"status=active && priority>3", "status=active AND priority>3"},
		{"a=1 & b=2", "a=1 AND b=2"},
		{"a=1 || b=2", "a=1 OR b=2"},
		{"!state=draft", "NOT state=draft"},
		{"x=1 AND !state=draft", "x=1 AND NOT state=draft"},
		{"priority BETWEEN 1 AND 5", "priority:1..5"},
		{"a=1 AND priority between 1 and 5 OR b=2", "a=1 AND priority:1..5 OR b=2"},
		{"[base]*1.1 > 3", "float([base]*1.1)>3"},
		{"total*2>3", "float([total]*2)>3"},
		{"len(name)+1>3", "float(len(name)+1)>3"},
		{"now()-7d<created_at", "created_at>now()-7d"},
		{"3<total", "total>3"},
		{"2026-01-01<=created_at", "created_at>=2026-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			// Act
			_, err := Parse(tt.query, 0)
			_, rewriteErr := Parse(tt.rewrite, 0)

			// Assert
			errs := Errors(err)
			if len(errs) == 0 || !strings.Contains(errs[0].Hint, tt.rewrite) {
				t.Fatalf("%q hint = %v, want one containing %q", tt.query, err, tt.rewrite)
			}
			if rewriteErr != nil {
				t.Errorf("suggested rewrite %q does not parse: %v", tt.rewrite, rewriteErr)
			}
		})
	}
}

// A rewrite is only offered when it is a valid query; otherwise the hint
// falls back to the rule.
func TestParse_NoRewriteWhenItWouldNotParse(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
		not   string
	}{
		{"bang inside a word", "state!draft", "NOT state=draft, state!=draft", "stateNOT"},
		{"between without both ends", "x BETWEEN 1", "total:1..5", "x:1"},
		{"double equals before a second error", "a==b AND (", "single =", "a=b AND ("},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := Parse(tt.query, 0)

			// Assert
			errs := Errors(err)
			if len(errs) == 0 {
				t.Fatalf("%q parsed, want an error", tt.query)
			}
			if hint := errs[0].Hint; !strings.Contains(hint, tt.want) || strings.Contains(hint, tt.not) {
				t.Errorf("%q hint = %q, want one containing %q and not %q", tt.query, hint, tt.want, tt.not)
			}
		})
	}
}

func TestError_ErrorIncludesHint(t *testing.T) {
	// Arrange
	_, err := Parse("state==draft", 0)

	// Act
	text := err.Error()

	// Assert
	if !strings.Contains(text, "unexpected") || !strings.HasSuffix(text, "(equality is a single =: state=draft)") {
		t.Errorf("Error() = %q, want the message followed by the hint in parentheses", text)
	}
}

// Every code a query can fail with either has a hint or is on the short list
// of codes where there is nothing useful to add.
func TestParse_EveryErrorHasAHintOrAReason(t *testing.T) {
	silent := map[Code]bool{
		CodeQueryTooLong: true, CodeInvalidInteger: true, CodeInvalidFloat: true, CodeNestingTooDeep: true,
	}
	queries := []string{
		"[base]*1.1 > 3", "3<total", "state==draft", "a=1 && b=2", "state=", "=draft", "(state=draft",
		"state=draft)", `name="john`, "total>[base", "total>[]", "state IN draft", "state IN (a b)", "state IN ()",
		"name:john", "name=jo*hn", "created_at>2026-13-45", "tags@", "tags@nope", "lower(name", "lower(name=x",
		"labels.=x", ".team=x", "state=draft draft 5", "a=1 AND", "AND a=1", "()", "name~x", "a=1 ^ b",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			// Act
			_, err := Parse(q, 0)

			// Assert
			errs := Errors(err)
			if len(errs) == 0 {
				t.Fatalf("%q parsed, want an error", q)
			}
			for _, e := range errs {
				if e.Hint == "" && !silent[e.Code] {
					t.Errorf("%q: %s (%s) has no hint", q, e.Code, e.Message)
				}
			}
		})
	}
}

// With the length limit lifted, a query made of nothing but mistakes must
// still fail in time proportional to its length: only the first few errors
// get a hint.
func TestParse_HintsAreBoundedOnLongInput(t *testing.T) {
	// Arrange: twenty thousand separate errors.
	query := strings.Repeat("a==b ", 20000)

	// Act
	_, err := Parse(query, 0)

	// Assert
	errs := Errors(err)
	if len(errs) < 20000 {
		t.Fatalf("got %d errors, want one per repetition", len(errs))
	}
	hinted := 0
	for _, e := range errs {
		if e.Hint != "" {
			hinted++
		}
	}
	if hinted != maxHinted {
		t.Errorf("%d errors carry a hint, want %d", hinted, maxHinted)
	}
}
