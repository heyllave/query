package query_test

import (
	"strings"
	"testing"
	"time"

	"github.com/heyllave/query"
	"github.com/heyllave/query/validate"
)

var testFields = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		opts    []query.Option
		wantErr bool
	}{
		{"simple parse", "state=draft", nil, false},
		{"max length exceeded", "state=draft", []query.Option{query.WithMaxLength(5)}, true},
		{"max length sufficient", "state=draft", []query.Option{query.WithMaxLength(100)}, false},
		{"max length disabled", strings.Repeat("a", 1000) + "=x", []query.Option{query.WithMaxLength(0)}, false},
		{"default exceeds 256", strings.Repeat("a", 300) + "=x", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := query.Parse(tt.input, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid query", "state=draft AND total>50000", false},
		{"unknown field", "nonexistent=value", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := query.Parse(tt.input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := query.Validate(expr, testFields); (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestParseAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "state=draft", false},
		{"parse error", "=invalid", true},
		{"validation error", "nonexistent=value", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := query.ParseAndValidate(tt.input, testFields)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestDefaultMaxLength(t *testing.T) {
	if query.DefaultMaxLength != 256 {
		t.Errorf("DefaultMaxLength = %d, want 256", query.DefaultMaxLength)
	}
}

func TestParse_StrictOptions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		opts    []query.Option
		wantErr bool
	}{
		{"implicit AND by default", "a=1 b=2", nil, false},
		{"explicit AND required", "a=1 b=2", []query.Option{query.WithExplicitAnd()}, true},
		{"explicit AND written", "a=1 AND b=2", []query.Option{query.WithExplicitAnd()}, false},
		{"operator in a value by default", "a=1+b=2", nil, false},
		{"strict values", "a=1+b=2", []query.Option{query.WithStrictValues()}, true},
		{"strict refuses both", "a=1 b=2", []query.Option{query.WithStrict()}, true},
		{"strict refuses both, values", "a=1+b=2", []query.Option{query.WithStrict()}, true},
		{"strict accepts a plain query", `a=1 AND name="x=y"`, []query.Option{query.WithStrict()}, false},
		{"strict keeps the length limit", "a=1", []query.Option{query.WithStrict(), query.WithMaxLength(2)}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			_, err := query.Parse(tc.input, tc.opts...)

			// Assert
			if (err != nil) != tc.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestMatch_StrictOptions(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "a", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
		{Name: "b", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	}
	record := map[string]any{"a": 1, "b": 2}

	t.Run("lenient by default", func(t *testing.T) {
		ok, err := query.Match("a=1 b=2", fields, record)
		if err != nil || !ok {
			t.Errorf("Match = %v, %v; want true", ok, err)
		}
	})

	t.Run("refuses what Parse refuses under the same option", func(t *testing.T) {
		_, parseErr := query.Parse("a=1 b=2", query.WithStrict())
		_, matchErr := query.MatchWith("a=1 b=2", fields, record, query.WithStrict())
		if parseErr == nil || matchErr == nil {
			t.Errorf("Parse error = %v, Match error = %v; want both refused", parseErr, matchErr)
		}
	})

	t.Run("accepts a plain query under strict", func(t *testing.T) {
		ok, err := query.MatchWith("a=1 AND b=2", fields, record, query.WithStrict())
		if err != nil || !ok {
			t.Errorf("Match = %v, %v; want true", ok, err)
		}
	})

	t.Run("honours the length limit", func(t *testing.T) {
		if _, err := query.MatchWith("a=1 AND b=2", fields, record, query.WithMaxLength(3)); err == nil {
			t.Error("expected a length error")
		}
	})
}

// Callers can store the original convenience functions without adapting their
// function types when opting in to the newer parsing API elsewhere.
func TestLegacyMatchAndEvalFunctionTypes(t *testing.T) {
	// Named types, so the assignments below check the signatures: they stop
	// compiling if Match or Eval gains a parameter.
	type (
		matchFunc func(string, []validate.FieldConfig, map[string]any) (bool, error)
		evalFunc  func(string, []validate.FieldConfig, map[string]any) (any, error)
	)
	var match matchFunc = query.Match
	var value evalFunc = query.Eval
	record := map[string]any{"state": "draft", "total": 5.0}
	got, err := match("state=draft", testFields, record)
	if err != nil || !got {
		t.Fatalf("legacy Match = %v, %v; want true", got, err)
	}
	gotValue, err := value("[total]+2", testFields, record)
	if err != nil || gotValue != float64(7) {
		t.Fatalf("legacy Eval = %v, %v; want 7", gotValue, err)
	}
}

func TestEvalWith_PreservesExplicitLengthLimit(t *testing.T) {
	record := map[string]any{"total": 5.0}
	if _, err := query.EvalWith("[total]+2", testFields, record, query.WithMaxLength(3)); err == nil {
		t.Fatal("EvalWith ignored the explicit length limit")
	}
	got, err := query.EvalWith("[total]+2", testFields, record, query.WithMaxLength(0))
	if err != nil || got != float64(7) {
		t.Fatalf("EvalWith = %v, %v; want 7", got, err)
	}
}

// A date written short is the day it names on a date field. Everywhere else
// it stays the subtraction the grammar reads: a text field refuses it like
// any arithmetic, and a number field refuses it too, since it would
// otherwise be a subtraction nobody meant.
func TestShortDate_FollowsTheDeclaredType(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "createdAt", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
		{Name: "day", Type: validate.TypeDate, AllowedOps: validate.DateOps},
		{Name: "period", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "n", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	}
	record := map[string]any{
		"createdAt": time.Date(2026, 11, 5, 14, 30, 0, 0, time.UTC),
		"day":       "2026-01-05",
		"period":    "2026-11",
		"n":         2015,
	}
	for q, want := range map[string]bool{
		"createdAt=2026-11-5":             true,
		"createdAt=2026-11-05":            true,
		"createdAt=2026-11-6":             false,
		"createdAt>2026-11-4":             true,
		"createdAt:2026-11-1..2026-11-30": true,
		"createdAt:2026-11-6..2026-11-30": false,
		`createdAt="2026-11-5"`:           true,
		"day=2026-1-5":                    true,
		"day IN (2026-1-4, 2026-1-5)":     true,
		`period="2026-11"`:                true,
		// With a space or a parenthesis it is the subtraction, on a number
		// field as on any other: 2026 - 11 is 2015.
		"n=2026 - 11":        true,
		"n=(2026)-11":        true,
		"n=(2026-11)":        true,
		"n=2026 - (11)":      true,
		"n=2026 - 12":        false,
		`period="2026-11-5"`: false,
	} {
		got, err := query.Match(q, fields, record)
		if err != nil {
			t.Errorf("%s: %v", q, err)
			continue
		}
		if got != want {
			t.Errorf("%s: matched %v, want %v", q, got, want)
		}
	}
	// Date arithmetic needs the date in full: only a value that is exactly a
	// date written short is converted, so 2026-11-5-1d stays a subtraction.
	// A text field refuses it like any other subtraction, 10-3 included.
	for _, q := range []string{
		"n=2026-11", "n=2026-11-5", "day=2026-11", "day=2026-2-30", "createdAt>2026-11-5-1d",
		"period=2026-11", "period=2026-11-5", "period=10-3",
		"period=2026 - 11", "createdAt=2026 - 11 - 5", "createdAt=(2026)-11-5", "createdAt=2026-11-5T10:00:00Z",
	} {
		if _, err := query.Match(q, fields, record); err == nil {
			t.Errorf("%s: matched without an error, want it refused", q)
		}
	}
}
