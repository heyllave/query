package parser_test

import (
	"strings"
	"testing"

	"github.com/heyllave/query/parser"
)

// TestParse_ErrorCode pins the machine identifier each distinguishable failure
// reports. A client renders its own copy from these, so a code changing is a
// breaking change for every client at once — the table is the contract.
func TestParse_ErrorCode(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  parser.Code
	}{
		{"unexpected character", "state=draft #", parser.CodeUnexpectedChar},
		{"unterminated string", `state="draft`, parser.CodeUnclosedString},
		{"unclosed paren", "(a=1 AND b=2", parser.CodeUnclosedParen},
		{"unclosed IN list", "state IN (draft, sent", parser.CodeUnclosedIn},
		{"empty IN list", "state IN ()", parser.CodeEmptyInList},
		{"IN without a list", "state IN draft", parser.CodeExpectedInList},
		{"missing field name", "=draft", parser.CodeExpectedField},
		{"value at end of query", "state=", parser.CodeExpectedValue},
		{"value missing inside a list", "state IN (draft,", parser.CodeExpectedValue},
		{"unknown selector", "items@nope(x=1)", parser.CodeExpectedSelector},
		{"invalid date", "created=2026-13-45", parser.CodeInvalidDate},
		{"invalid wildcard", "state=**b**", parser.CodeInvalidWildcard},
		{"unclosed function arguments", "upper(", parser.CodeUnclosedFuncArgs},
		{"stray closing paren", "a=1)", parser.CodeUnexpected},
		{"trailing token", "state=draft draft=", parser.CodeExpectedValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			_, err := parser.Parse(tt.query, 256)

			// Assert
			if err == nil {
				t.Fatalf("Parse(%q) returned no error, want code %q", tt.query, tt.want)
			}
			errs := parser.Errors(err)
			if len(errs) == 0 {
				t.Fatalf("Parse(%q) error carries no parser.Error", tt.query)
			}
			if got := errs[0].Code; got != tt.want {
				t.Errorf("Parse(%q) first code = %q, want %q (message: %s)",
					tt.query, got, tt.want, errs[0].Message)
			}
		})
	}
}

// TestCodes_EveryCodeIsProducible walks the whole vocabulary and proves each
// code has an input that produces it.
//
// A code nothing can report is a promise the engine does not keep: a client
// switching on it writes a branch that never runs, and a translator writes copy
// nobody reads. Adding a constant without an input here fails the build.
func TestCodes_EveryCodeIsProducible(t *testing.T) {
	// One input per code. Duration is reported by ParseDuration rather than
	// Parse, which is why the table names the entry point too.
	byCode := map[parser.Code]struct {
		input    string
		duration bool
	}{
		parser.CodeQueryTooLong:     {input: strings.Repeat("a", 300) + "=1"},
		parser.CodeUnexpectedChar:   {input: "state=draft #"},
		parser.CodeUnclosedString:   {input: `state="draft`},
		parser.CodeUnclosedParen:    {input: "(a=1 AND b=2"},
		parser.CodeUnclosedIn:       {input: "state IN (draft, sent"},
		parser.CodeEmptyInList:      {input: "state IN ()"},
		parser.CodeUnclosedFieldRef: {input: "total>[unclosed"},
		parser.CodeEmptyFieldRef:    {input: "total>[]"},
		parser.CodeExpectedField:    {input: "=draft"},
		parser.CodeExpectedValue:    {input: "state="},
		parser.CodeExpectedInList:   {input: "state IN draft"},
		parser.CodeExpectedSelector: {input: "items@nope(x=1)"},
		parser.CodeExpectedRange:    {input: "total:1"},
		parser.CodeUnexpected:       {input: "a=1)"},
		parser.CodeInvalidWildcard:  {input: "state=**b**"},
		parser.CodeInvalidDate:      {input: "created=2026-13-45"},
		parser.CodeInvalidInteger:   {input: "n>99999999999999999999999"},
		parser.CodeInvalidFloat:     {input: "total>."},
		parser.CodeUnclosedFuncArgs: {input: "upper("},
		parser.CodeInvalidDuration:  {input: "5zz", duration: true},
		parser.CodeNestingTooDeep:   {input: strings.Repeat("(", parser.MaxNesting+1) + "a"},
	}

	for _, code := range parser.Codes() {
		t.Run(string(code), func(t *testing.T) {
			// Arrange
			tc, known := byCode[code]
			if !known {
				t.Fatalf("code %q has no input in this table — add one, or drop the code", code)
			}

			// Act
			var err error
			if tc.duration {
				_, err = parser.ParseDuration(tc.input)
			} else {
				_, err = parser.Parse(tc.input, 256)
			}

			// Assert
			if err == nil {
				t.Fatalf("Parse(%q) succeeded; it must report %q", tc.input, code)
			}
			var got []parser.Code
			for _, e := range parser.Errors(err) {
				got = append(got, e.Code)
				if e.Code == code {
					return
				}
			}
			t.Errorf("input %q reported %v, want %q among them", tc.input, got, code)
		})
	}
}

// TestParse_EveryErrorCarriesACode guards the constructor: a failure reported
// without a code is invisible to every client that renders its own copy, and
// the only way to notice is to look for it.
func TestParse_EveryErrorCarriesACode(t *testing.T) {
	// A spread of malformed inputs, aimed at as many distinct failures as the
	// grammar has. Each must arrive coded.
	queries := []string{
		"", "#", "=1", "state=", `state="x`, "(a=1", "a IN (", "a IN ()", "a IN 1",
		"@x(a=1)", "@first", "created=2026-99-99", "total>1..", "a.=1", "upper(",
		"upper(a,", "total=abc..def", "state=draft AND", "NOT", "a=1)", "a==",
		"total>", "[unclosed", "[]=1", "a=*b*c*", "now(", "a=1 b", "-", "..",
	}

	for _, q := range queries {
		_, err := parser.Parse(q, 256)
		if err == nil {
			continue
		}
		for i, e := range parser.Errors(err) {
			if e.Code == "" {
				t.Errorf("Parse(%q) error %d has an empty Code (kind %v, message %q)",
					q, i, e.Kind, e.Message)
			}
		}
	}
}
