package parser

import (
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
)

// TestValueShape_AsTheGrammarPageStatesIt pins the table under "The type of
// an unquoted value" in docs/grammar.md: every example there, at the step
// that decides it. A change here is a change to what a saved query means.
func TestValueShape_AsTheGrammarPageStatesIt(t *testing.T) {
	const pattern = ast.ValueType(-1) // a string value that is a wildcard
	cases := []struct {
		step  int
		value string
		want  ast.ValueType
	}{
		// 1. A complete date.
		{1, "2026-11-16", ast.ValueDate},
		{1, "2026-11-16T10:00:00Z", ast.ValueDate},
		// 2. A word makes the whole value text.
		{2, "10-x", ast.ValueString},
		{2, "pr-10-3", ast.ValueString},
		{2, "5+3x", ast.ValueString},
		{2, "1e5", ast.ValueString},
		{2, "7days", ast.ValueString},
		{2, "a/b", ast.ValueString},
		{2, "a+b", ast.ValueString},
		{2, "TRUE", ast.ValueString},
		{2, "0x10", ast.ValueString},
		{2, "1_000", ast.ValueString},
		{2, "2026-11-16T10", ast.ValueString},
		{2, "550e8400-e29b-41d4-a716-446655440000", ast.ValueString},
		{2, "check*", pattern},
		{2, "*check*", pattern},
		// 3. More than one dot in a number.
		{3, "1.2.3", ast.ValueString},
		{3, "1.2.3-4", ast.ValueString},
		{3, "10.0.0.1/24", ast.ValueString},
		// 4. An operator between two operands.
		{4, "10-3", ast.ValueArith},
		{4, "1-2", ast.ValueArith},
		{4, "10/2", ast.ValueArith},
		{4, "1+1", ast.ValueArith},
		{4, "2*3", ast.ValueArith},
		{4, "10%3", ast.ValueArith},
		{4, "7d-1h", ast.ValueArith},
		{4, "30m-1h", ast.ValueArith},
		{4, "3-2026", ast.ValueArith},
		{4, "999-5", ast.ValueArith},
		{4, "12345-6", ast.ValueArith},
		{4, "10*-3", ast.ValueArith},
		{4, "now()-7d", ast.ValueArith},
		{4, "2026-11-16-1", ast.ValueArith},
		// Four digits and a hyphen are a date only when the date is whole.
		{4, "2026-11-5", ast.ValueArith},
		{4, "2026-1", ast.ValueArith},
		{4, "2026-11", ast.ValueArith},
		{4, "2026-1-1", ast.ValueArith},
		{4, "1999-5", ast.ValueArith},
		{4, "2026-11-5-3", ast.ValueArith},
		// 5 to 7. One literal.
		{5, "true", ast.ValueBoolean},
		{5, "false", ast.ValueBoolean},
		{6, "7d", ast.ValueDuration},
		{6, "30m", ast.ValueDuration},
		{7, "42", ast.ValueInteger},
		{7, "-5", ast.ValueInteger},
		{7, "+5", ast.ValueInteger},
		{7, "007", ast.ValueInteger},
		{7, "1.5", ast.ValueFloat},
		// 8. A star at an end.
		{8, "10*", pattern},
		{8, "*10", pattern},
		// 9. Anything else.
		{9, "5-", ast.ValueString},
		{9, "10%", ast.ValueString},
		{9, "1h30m", ast.ValueString},
		{9, "1.5h", ast.ValueString},
		{9, "5m30", ast.ValueString},
		{9, "1,000", ast.ValueString},
	}
	for _, tc := range cases {
		for _, cfg := range []Config{{}, Config{}.Strict()} {
			expr, err := ParseWith("f="+tc.value, cfg)
			if err != nil {
				t.Errorf("step %d: f=%s does not parse: %v", tc.step, tc.value, err)
				continue
			}
			q, ok := expr.(*ast.QualifierExpr)
			if !ok {
				t.Errorf("step %d: f=%s parsed to %T", tc.step, tc.value, expr)
				continue
			}
			got := q.Value.Type
			if q.Value.Wildcard {
				got = pattern
			}
			if got != tc.want {
				t.Errorf("step %d: %s is %v, want %v", tc.step, tc.value, got, tc.want)
			}
		}
	}
}

func TestValueShape_AWholeDateThatCannotExistIsAnError(t *testing.T) {
	for _, v := range []string{"2026-13-45", "2026-02-30", "2026-00-10"} {
		_, err := Parse("f="+v, 0)
		errs := Errors(err)
		if len(errs) == 0 || errs[0].Code != CodeInvalidDate {
			t.Errorf("f=%s: got %v, want the code %s", v, err, CodeInvalidDate)
		}
	}
}

// A date attempt that fails must give the same tokens wherever a number may
// stand: after an operator, in an IN list, and as a function argument.
func TestValueShape_AFailedDateIsASubtractionEverywhere(t *testing.T) {
	for _, q := range []string{"f=2026-11-5", "f IN (2026-11-5, 1)", "f>abs(2026-11-5)", "f:2026-1..2026-2", "f>2026 - 1"} {
		expr, err := Parse(q, 0)
		if err != nil {
			t.Errorf("%s does not parse: %v", q, err)
			continue
		}
		out := ast.String(expr)
		again, err := Parse(out, 0)
		if err != nil || ast.String(again) != out {
			t.Errorf("%s prints as %s, which does not read back the same (%v)", q, out, err)
		}
	}
	expr, err := ParseValue("2026-11-5", 0)
	if err != nil {
		t.Fatal(err)
	}
	if v := expr.(*ast.ValueExpr).Value; v.Type != ast.ValueArith {
		t.Fatalf("2026-11-5 as a value is %v, want arithmetic", v.Type)
	}
}

// A star makes a pattern out of text only. After arithmetic it is an error,
// as the Grammar page says under the table.
func TestValueShape_AStarAfterArithmeticIsAnError(t *testing.T) {
	for _, v := range []string{"2026-11*", "10-3*", "2026-11-5*"} {
		for _, cfg := range []Config{{}, Config{}.Strict()} {
			if _, err := ParseWith("f="+v, cfg); err == nil {
				t.Errorf("f=%s parses", v)
			}
		}
	}
}

// A date-shaped run is marked only when its parts could be a month and a
// day, and only as one unbroken run that is the whole value.
func TestValueShape_ShortDateMark(t *testing.T) {
	for v, want := range map[string]bool{
		"2026-11": true, "2026-1": true, "2026-12": true, "2026-11-5": true, "2026-1-31": true, "2026-2-30": true,
		"2026-13": false, "2026-0": false, "2026-00": false, "2026-1-32": false, "2026-1-0": false, "1500-50": false,
		"999-5": false, "12345-6": false, "2026-111": false, "2026-11-5-3": false, "2026-11-5-1d": false,
		"2026 - 11": false, "(2026)-11": false, "(2026-11)": false, "2026+11": false, "2026-11*2": false,
		"-2026-11": false, "2026--11": false, "10-3": false,
	} {
		expr, err := ParseWith("f="+v, Config{})
		if err != nil {
			t.Errorf("f=%s does not parse: %v", v, err)
			continue
		}
		q, ok := expr.(*ast.QualifierExpr)
		if !ok {
			t.Errorf("f=%s parsed to %T", v, expr)
			continue
		}
		if q.Value.ShortDate != want {
			t.Errorf("%s: marked %v, want %v", v, q.Value.ShortDate, want)
		}
	}
	// The mark holds in a range end, an IN list and a function argument, and
	// survives printing.
	for _, q := range []string{"f:1..2026-11", "f IN (1, 2026-11)", "f>abs(2026-11-5)", "contains(f, 2026-11)"} {
		expr, err := Parse(q, 0)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		out := ast.String(expr)
		again, err := Parse(out, 0)
		if err != nil || ast.String(again) != out {
			t.Errorf("%s prints as %s, which does not read back the same (%v)", q, out, err)
		}
		if !strings.Contains(out, "2026-11") {
			t.Errorf("%s prints as %s, which no longer holds the run as written", q, out)
		}
	}
}

// A bare run of digits and hyphens where a field belongs is reported whole.
func TestBareNumberRunIsReportedWhole(t *testing.T) {
	for q, want := range map[string]string{
		"2026-11":           `expected a field name, got "2026-11"`,
		"a=1 AND 2026-11-5": `expected a field name, got "2026-11-5"`,
		"812":               `expected a field name, got "812"`,
		"(10-3)":            `expected a field name, got "10-3"`,
	} {
		_, err := ParseWith(q, Config{}.Strict())
		errs := Errors(err)
		if len(errs) == 0 || errs[0].Message != want {
			t.Errorf("%s: got %v, want the message %s", q, err, want)
			continue
		}
		if got := q[errs[0].Position.Offset : errs[0].Position.Offset+errs[0].Position.Length]; !strings.Contains(want, got) {
			t.Errorf("%s: the error spans %q", q, got)
		}
	}
}
