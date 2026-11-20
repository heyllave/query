package eval

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// TestValidatorAgreesWithTheEvaluatorOnLiteralArithmetic builds every
// arithmetic expression of one and two operators over a number, a float, a
// duration and a date, evaluates each, and checks the validator's verdict
// against what came out: a comparison is accepted exactly when the
// expression has a result and the result is of the field's type. The
// validator works the result out from the operands alone, so this is what
// keeps its rules the evaluator's rules.
func TestValidatorAgreesWithTheEvaluatorOnLiteralArithmetic(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "n", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		{Name: "at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
		{Name: "ttl", Type: validate.TypeDuration, AllowedOps: validate.DurationOps},
	}
	operands := []string{"2", "1.5", "7d", "(2026-01-01)"}
	ops := []string{"+", "-", "*", "/", "%"}

	var exprs []string
	for _, a := range operands {
		for _, op := range ops {
			for _, b := range operands {
				one := a + op + b
				exprs = append(exprs, one)
				for _, op2 := range ops {
					for _, c := range operands {
						exprs = append(exprs, "("+one+")"+op2+c, c+op2+"("+one+")")
					}
				}
			}
		}
	}

	checked := map[string]int{}
	for _, expr := range exprs {
		prog, err := CompileValue(expr, nil, WithMaxLength(0))
		if err != nil {
			t.Fatalf("%s does not compile as a value: %v", expr, err)
		}
		got, err := prog.Eval(nil)
		kind := "none"
		switch {
		case errors.Is(err, ErrNoValue):
		case err != nil:
			t.Fatalf("%s: %v", expr, err)
		default:
			switch got.(type) {
			case int64, float64:
				kind = "n"
			case time.Time:
				kind = "at"
			case time.Duration:
				kind = "ttl"
			default:
				t.Fatalf("%s evaluated to %T", expr, got)
			}
		}
		// A division or a modulo can have no result for these particular
		// numbers (a zero divisor, a fractional modulo) while being
		// well-typed. The validator judges types, not numbers, so those
		// are left out.
		if kind == "none" && strings.ContainsAny(expr, "/%") {
			continue
		}
		checked[kind]++
		for _, f := range fields {
			_, err := Compile(fmt.Sprintf("%s>%s", f.Name, expr), fields, WithMaxLength(0))
			if want := kind == f.Name; (err == nil) != want {
				t.Errorf("%s>%s: evaluates to %q, but validation says %v", f.Name, expr, kind, err)
			}
		}
	}
	for _, kind := range []string{"n", "at", "ttl", "none"} {
		if checked[kind] < 10 {
			t.Errorf("only %d expressions of kind %q were checked", checked[kind], kind)
		}
	}
}
