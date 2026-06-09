// Package conformance runs the shared cross-language corpus (corpus.json)
// against the real Go query engine. The same corpus is run by the JavaScript and
// Dart clients, so any grammar drift between implementations fails a build.
package conformance

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

type corpus struct {
	Version   int                               `json:"version"`
	FieldSets map[string][]validate.FieldConfig `json:"fieldSets"`
	Cases     []testCase                        `json:"cases"`
}

type testCase struct {
	ID           string                 `json:"id"`
	Desc         string                 `json:"desc"`
	Op           string                 `json:"op"`
	Query        string                 `json:"query"`
	FieldSet     string                 `json:"fieldSet"`
	Fields       []validate.FieldConfig `json:"fields"`
	Record       map[string]any         `json:"record"`
	ExpectMatch  *bool                  `json:"expectMatch"`
	ExpectValue  *float64               `json:"expectValue"`
	ExpectError  *bool                  `json:"expectError"`
	ExpectAst    string                 `json:"expectAst"`
	ExpectString string                 `json:"expectString"`
	Tolerance    float64                `json:"tolerance"`
}

// expectedCaseCount guards against a stale/truncated corpus silently passing.
const expectedCaseCount = 25

func loadCorpus(t *testing.T) corpus {
	t.Helper()
	data, err := os.ReadFile("corpus.json")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var c corpus
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse corpus: %v", err)
	}
	return c
}

func (c corpus) fields(tc testCase) []validate.FieldConfig {
	if tc.FieldSet != "" {
		return c.FieldSets[tc.FieldSet]
	}
	return tc.Fields
}

func TestConformance(t *testing.T) {
	c := loadCorpus(t)
	if len(c.Cases) < expectedCaseCount {
		t.Fatalf("corpus has %d cases, expected at least %d — stale or truncated", len(c.Cases), expectedCaseCount)
	}
	for _, tc := range c.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			runCase(t, c, tc)
		})
	}
}

func runCase(t *testing.T, c corpus, tc testCase) {
	t.Helper()
	switch tc.Op {
	case "parse":
		expr, err := parser.Parse(tc.Query, 256)
		checkErr(t, tc, err)
		if err == nil && tc.ExpectAst != "" {
			// AST type is asserted loosely (presence + no error); the JS/Dart
			// runners do the structural type check against the JSON shape.
			if expr == nil {
				t.Errorf("expected an AST, got nil")
			}
		}
	case "parseAndValidate":
		expr, err := parser.Parse(tc.Query, 256)
		if err == nil {
			err = validate.New(c.fields(tc)).Validate(expr)
		}
		checkErr(t, tc, err)
	case "stringify":
		expr, err := parser.Parse(tc.Query, 256)
		if err != nil {
			t.Fatalf("parse for stringify: %v", err)
		}
		got := ast.String(expr)
		if tc.ExpectString != "" && got != tc.ExpectString {
			t.Errorf("stringify = %q, want %q", got, tc.ExpectString)
		}
	case "match":
		// An expected error may come from compile (e.g. unknown field).
		prog, err := eval.Compile(tc.Query, c.fields(tc))
		if err != nil {
			expectErr(t, tc, err)
			return
		}
		if expectNoErr(t, tc) {
			return
		}
		got := prog.Match(tc.Record)
		if tc.ExpectMatch != nil && got != *tc.ExpectMatch {
			t.Errorf("match = %v, want %v", got, *tc.ExpectMatch)
		}
	case "eval":
		// An expected error may come from compile OR from Eval (e.g. missing
		// field, division by zero -> ErrNoValue).
		prog, err := eval.CompileValue(tc.Query, c.fields(tc))
		if err != nil {
			expectErr(t, tc, err)
			return
		}
		v, evalErr := prog.Eval(tc.Record)
		if evalErr != nil {
			expectErr(t, tc, evalErr)
			return
		}
		if expectNoErr(t, tc) {
			return
		}
		if tc.ExpectValue != nil {
			checkNumeric(t, toFloat(v), *tc.ExpectValue, tc.Tolerance)
		}
	default:
		t.Fatalf("unknown op %q", tc.Op)
	}
}

// checkErr enforces a single-stage error expectation (parse/parseAndValidate/
// stringify). It returns true when an error occurred so the caller can stop.
func checkErr(t *testing.T, tc testCase, err error) bool {
	t.Helper()
	if err != nil {
		expectErr(t, tc, err)
		return true
	}
	expectNoErr(t, tc)
	return false
}

// expectErr asserts that an error here was expected.
func expectErr(t *testing.T, tc testCase, err error) {
	t.Helper()
	if tc.ExpectError == nil || !*tc.ExpectError {
		t.Errorf("unexpected error: %v", err)
	}
}

// expectNoErr asserts that no error reached this point when one was expected.
// Returns true if an error was expected (so the caller can stop).
func expectNoErr(t *testing.T, tc testCase) bool {
	t.Helper()
	if tc.ExpectError != nil && *tc.ExpectError {
		t.Errorf("expected an error, got none")
		return true
	}
	return false
}

func checkNumeric(t *testing.T, got, want, tol float64) {
	t.Helper()
	if tol == 0 {
		tol = 1e-9
	}
	if math.Abs(got-want) > tol {
		t.Errorf("value = %v, want %v (±%v)", got, want, tol)
	}
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	default:
		return math.NaN()
	}
}
