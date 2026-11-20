package validate

import (
	"errors"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

func arithFields() []FieldConfig {
	return []FieldConfig{
		{Name: "n", Type: TypeInteger, AllowedOps: NumericOps},
		{Name: "price", Type: TypeDecimal, AllowedOps: NumericOps},
		{Name: "createdAt", Type: TypeDatetime, AllowedOps: DateOps},
		{Name: "day", Type: TypeDate, AllowedOps: DateOps},
		{Name: "ttl", Type: TypeDuration, AllowedOps: DurationOps},
		{Name: "name", Type: TypeText, AllowedOps: TextOps},
	}
}

func validateArith(t *testing.T, q string) error {
	t.Helper()
	expr, err := parser.ParseWith(q, parser.Config{}.Strict())
	if err != nil {
		t.Fatalf("%s does not parse: %v", q, err)
	}
	return New(arithFields()).Validate(expr)
}

func TestValidate_ArithmeticOverLiteralsThatFitsTheField(t *testing.T) {
	for _, q := range []string{
		// numbers
		"n=10-3", "n>2*3+1", "price<10/4", "n=10%3", "n>(1+2)*3", "n:1+1..10-1", "n IN (1+1, 2*2)",
		// a date and a duration give a date
		"createdAt>2026-01-01+7d", "createdAt<2026-01-31-1w", "day>=2026-01-01+1d", "createdAt>2026-01-01+7d-1h",
		// two dates give a duration
		"ttl>2026-01-31 - (2026-01-01)",
		// durations
		"ttl>7d-1h", "ttl<1h+30m", "ttl=2h*3", "ttl=3*2h", "ttl>1d/2", "ttl:1h+1m..2h-1m",
		// What depends on a function or a field is left to the evaluator.
		"createdAt>now()-7d", "n>len(name)+1", "ttl>now()-[createdAt]", "n=[price]*2", "createdAt>daysAgo(7)+1d",
		"ttl>[n]*1h", "n>abs(-3)+now()",
	} {
		if err := validateArith(t, q); err != nil {
			t.Errorf("%s: unexpected error: %v", q, err)
		}
	}
}

func TestValidate_ArithmeticOverLiteralsThatDoesNotFit(t *testing.T) {
	cases := []struct {
		query, message, hint string
	}{
		// A date written short is three numbers subtracted.
		{"createdAt=2026-11-5", `value is arithmetic (2026-11-5), which gives a number and does not fit field "createdAt" (type datetime)`,
			"a date is written in full, YYYY-MM-DD: 2026-01-05 rather than 2026-1-5"},
		{"day>2026-1", `gives a number and does not fit field "day" (type date)`, "written in full"},
		{"createdAt:2026-01-01..2026-2-1", `range end value is arithmetic (2026-2-1), which gives a number`, "written in full"},
		{"createdAt>10*3", `gives a number and does not fit field "createdAt"`, "for a relative date use now()-7d"},
		{"createdAt>7d-1h", `gives a duration and does not fit field "createdAt"`, "for a relative date"},
		// Numbers where a duration belongs, and the other way round.
		{"ttl>10-3", `gives a number and does not fit field "ttl" (type duration)`, "a duration is a number and one unit"},
		{"ttl=2026-01-01+7d", `gives a date and does not fit field "ttl"`, "a duration is a number and one unit"},
		{"n>7d-1h", `gives a duration and does not fit field "n" (type integer)`, "a number looks like"},
		{"price=2026-01-01+1d", `gives a date and does not fit field "price" (type decimal)`, "a number looks like"},
		{"n IN (1, 7d*2)", `gives a duration and does not fit field "n"`, "a number looks like"},
		// Operands the operator has no meaning for.
		{"n>10-7d", "value is arithmetic (10-7d) that has no result", "numbers combine with numbers"},
		{"ttl>7d+3", "value is arithmetic (7d+3) that has no result", "a duration takes + or - a duration"},
		{"ttl>7d*1h", "that has no result", "numbers combine with numbers"},
		{"createdAt>2026-01-01*2", "that has no result", "a date takes + or - a duration"},
		{"createdAt>2026-01-01+(2026-01-02)", "that has no result", "a date takes"},
		{"ttl>3/1h", "that has no result", "numbers combine"},
		{"n>(10-7d)*2", "that has no result", "numbers combine"},
		// Text and boolean fields refuse any arithmetic, as before.
		{"name=10-3", `cannot produce a text for field "name"; quote it to compare as text`, ""},
	}
	for _, tc := range cases {
		err := validateArith(t, tc.query)
		var list ErrorList
		if !errors.As(err, &list) || len(list) != 1 {
			t.Errorf("%s: got %v, want one validation error", tc.query, err)
			continue
		}
		e := list[0]
		if e.Kind != ErrTypeMismatch {
			t.Errorf("%s: kind = %v, want a type mismatch", tc.query, e.Kind)
		}
		if !strings.Contains(e.Message, tc.message) {
			t.Errorf("%s: message = %q, want it to contain %q", tc.query, e.Message, tc.message)
		}
		if !strings.Contains(e.Hint, tc.hint) || (tc.hint == "" && e.Hint != "") {
			t.Errorf("%s: hint = %q, want it to contain %q", tc.query, e.Hint, tc.hint)
		}
	}
}

func TestKindOf_FollowsTheEvaluator(t *testing.T) {
	// One row per rule of eval's applyArith, so the two cannot drift
	// without this table being looked at.
	for expr, want := range map[string]kind{
		"1+2": kindNumber, "1.5*2": kindNumber, "7/2": kindNumber, "7%2": kindNumber,
		"2026-01-01+7d": kindDate, "2026-01-01-7d": kindDate,
		"2026-01-02 - (2026-01-01)": kindDuration,
		"7d+1h":                     kindDuration, "7d-1h": kindDuration,
		"7d*2": kindDuration, "7d/2": kindDuration, "2*7d": kindDuration,
		"7d*1h": kindUndefined, "7d/1h": kindUndefined, "2/7d": kindUndefined, "7d+2": kindUndefined,
		"2-7d": kindUndefined, "2026-01-01*2": kindUndefined, "7d+(2026-01-01)": kindUndefined,
		"2026-01-01%7d": kindUndefined, "7d%2": kindUndefined,
		"now()-7d": kindUnknown, "[a]+1": kindUnknown, "len(x)*2": kindUnknown,
		"(7d*1h)+now()": kindUndefined,
	} {
		parsed, err := parser.ParseValue(expr, 0)
		if err != nil {
			t.Errorf("%s does not parse: %v", expr, err)
			continue
		}
		val := valueOf(parsed)
		if got := kindOf(val); got != want {
			t.Errorf("kindOf(%s) = %v (%d), want %d", expr, got, got, want)
		}
	}
	if kindOf(nil) != kindUnknown {
		t.Error("kindOf(nil) is not unknown")
	}
}

func valueOf(e ast.Expression) *ast.Value {
	return &e.(*ast.ValueExpr).Value
}
