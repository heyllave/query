package eval

import (
	"strings"
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// These tests cover eval-side behavior of the parity features: quoted strings,
// functions in value position, IN lists, and negated comparison operators.

var parityFields = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "description", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "year", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "created_at", Type: validate.TypeDate, AllowedOps: validate.DateOps},
}

func TestEval_QuotedStringValue(t *testing.T) {
	prog, err := Compile(`name="John Doe"`, parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"name": "John Doe"}) {
		t.Error("expected match for exact string")
	}
	if prog.Match(map[string]any{"name": "John"}) {
		t.Error("expected no match for partial")
	}
}

func TestEval_QuotedStringWithSpecialChars(t *testing.T) {
	// Values that previously could not be expressed: contain space, paren,
	// or operator characters.
	tests := []struct {
		query string
		value string
		want  bool
	}{
		{`description="hello world"`, "hello world", true},
		{`description="(parens)"`, "(parens)", true},
		{`description="a > b"`, "a > b", true},
		{`description="quote \"inside\""`, `quote "inside"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := Compile(tt.query, parityFields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			got := prog.Match(map[string]any{"description": tt.value})
			if got != tt.want {
				t.Errorf("match: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEval_FunctionInValuePosition_Now(t *testing.T) {
	// created_at>=now() — should match a future date and reject a past one.
	prog, err := Compile("created_at>=now()", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)
	if !prog.Match(map[string]any{"created_at": future}) {
		t.Error("future date should be >= now()")
	}
	if prog.Match(map[string]any{"created_at": past}) {
		t.Error("past date should not be >= now()")
	}
}

func TestEval_FunctionInValuePosition_DaysAgo(t *testing.T) {
	prog, err := Compile("created_at>=daysAgo(7)", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	recent := time.Now().Add(-3 * 24 * time.Hour)
	old := time.Now().Add(-30 * 24 * time.Hour)
	if !prog.Match(map[string]any{"created_at": recent}) {
		t.Error("3 days ago should be >= daysAgo(7)")
	}
	if prog.Match(map[string]any{"created_at": old}) {
		t.Error("30 days ago should not be >= daysAgo(7)")
	}
}

func TestEval_FunctionInValuePosition_CustomFunc(t *testing.T) {
	prog, err := Compile("total>=threshold()", parityFields,
		WithFunctions(Func{
			Name: "threshold",
			Call: func(...any) (any, error) { return int64(1000), nil },
		}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"total": 2000.0}) {
		t.Error("2000 should be >= threshold() = 1000")
	}
	if prog.Match(map[string]any{"total": 500.0}) {
		t.Error("500 should not be >= threshold() = 1000")
	}
}

func TestEval_FunctionInRangeValue(t *testing.T) {
	prog, err := Compile("created_at:daysAgo(30)..now()", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	recent := time.Now().Add(-10 * 24 * time.Hour)
	old := time.Now().Add(-365 * 24 * time.Hour)
	if !prog.Match(map[string]any{"created_at": recent}) {
		t.Error("10 days ago should be in range [daysAgo(30), now()]")
	}
	if prog.Match(map[string]any{"created_at": old}) {
		t.Error("365 days ago should not be in range [daysAgo(30), now()]")
	}
}

func TestEval_IN_StringList(t *testing.T) {
	prog, err := Compile("state IN (draft, issued, paid)", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, v := range []string{"draft", "issued", "paid"} {
		if !prog.Match(map[string]any{"state": v}) {
			t.Errorf("expected match for %q", v)
		}
	}
	if prog.Match(map[string]any{"state": "cancelled"}) {
		t.Error("cancelled should not match")
	}
}

func TestEval_IN_NumericList(t *testing.T) {
	prog, err := Compile("year IN (2020, 2021, 2022)", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, y := range []int{2020, 2021, 2022} {
		if !prog.Match(map[string]any{"year": y}) {
			t.Errorf("expected match for year=%d", y)
		}
	}
	if prog.Match(map[string]any{"year": 2019}) {
		t.Error("2019 should not match")
	}
}

func TestEval_IN_Composes(t *testing.T) {
	prog, err := Compile("state IN (draft, issued) AND total>1000", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"state": "issued", "total": 2000.0}) {
		t.Error("issued+2000 should match")
	}
	if prog.Match(map[string]any{"state": "paid", "total": 2000.0}) {
		t.Error("paid+2000 should not match")
	}
	if prog.Match(map[string]any{"state": "draft", "total": 500.0}) {
		t.Error("draft+500 should not match")
	}
}

func TestEval_NegatedComparison(t *testing.T) {
	tests := []struct {
		query string
		data  map[string]any
		want  bool
	}{
		// !> means NOT >
		{"total!>50000", map[string]any{"total": 30000.0}, true},
		{"total!>50000", map[string]any{"total": 60000.0}, false},
		{"total!>50000", map[string]any{"total": 50000.0}, true},
		// !>= means NOT >=
		{"year!>=2020", map[string]any{"year": 2019}, true},
		{"year!>=2020", map[string]any{"year": 2020}, false},
		// !< means NOT <
		{"year!<2020", map[string]any{"year": 2025}, true},
		{"year!<2020", map[string]any{"year": 2015}, false},
		// !<= means NOT <=
		{"year!<=2020", map[string]any{"year": 2025}, true},
		{"year!<=2020", map[string]any{"year": 2020}, false},
		// Missing field: NOT (false) = true. Critical correctness check —
		// a naive operator flip would return false here.
		{"total!>50000", map[string]any{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prog, err := Compile(tt.query, parityFields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			got := prog.Match(tt.data)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEval_CaseInsensitiveKeywords(t *testing.T) {
	queries := []string{
		"state=draft and total>1000",
		"state=draft AND total>1000",
		"state=draft And total>1000",
		"not state=cancelled",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, err := Compile(q, parityFields); err != nil {
				t.Errorf("compile: %v", err)
			}
		})
	}
}

func TestEval_QuotedStringInFunctionArg(t *testing.T) {
	prog, err := Compile(`contains(description, "urgent")`, parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"description": "This is an urgent task"}) {
		t.Error("expected substring match")
	}
	if prog.Match(map[string]any{"description": "no match here"}) {
		t.Error("unexpected substring match")
	}
}

func TestEval_FuncValuePosition_ValidatorErrorOnUnknownField(t *testing.T) {
	_, err := Compile("created_at>=fakeFn(unknown_field)", parityFields)
	if err == nil {
		t.Fatal("expected validation error for unknown field inside func value")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error message does not mention unknown field: %v", err)
	}
}

func TestEval_IN_QuotedValues(t *testing.T) {
	// Quoted values inside IN — exercises the lexer in non-value mode
	// recognizing "..." between commas.
	prog, err := Compile(`name IN ("John Doe", "Jane Smith")`, parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"name": "John Doe"}) {
		t.Error("John Doe should match")
	}
	if prog.Match(map[string]any{"name": "Bob"}) {
		t.Error("Bob should not match")
	}
}

func TestEval_NegatedComparisonWithCaseInsensitiveNot(t *testing.T) {
	// Make sure the case-insensitive NOT keyword works alongside !> on a
	// missing field — combinatorial check of two new features.
	prog, err := Compile("not total!>50000", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// !> desugars to NOT >, so the outer NOT cancels it: same as total>50000.
	if !prog.Match(map[string]any{"total": 60000.0}) {
		t.Error("60000 should match: NOT (NOT (total>50000)) ≡ total>50000")
	}
	if prog.Match(map[string]any{"total": 30000.0}) {
		t.Error("30000 should not match")
	}
}
