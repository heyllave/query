// Command value demonstrates value-returning queries: expressions that compute
// and return a value (number, string, time, duration, or list) rather than
// evaluating to a boolean. eval.CompileValue + ValueProgram.Eval is the
// value-domain counterpart to the boolean predicate engine (eval.Compile +
// Program.Match).
//
// Run with:
//
//	go run .
package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

func main() {
	arithmetic()
	functionsOverFields()
	timeValues()
	lists()
	errorHandling()
	booleanPredicate()
}

// arithmetic: a query can compute a number. Precedence and grouping are honored;
// integer division promotes to float.
func arithmetic() {
	section("arithmetic")
	for _, q := range []string{"6*7", "(50000+1000)*1.1", "2+3*4", "9/2"} {
		prog, err := eval.CompileValue(q, nil)
		if err != nil {
			log.Fatalf("compile %q: %v", q, err)
		}
		v, err := prog.Eval(nil)
		if err != nil {
			log.Fatalf("eval %q: %v", q, err)
		}
		fmt.Printf("  %-18s => %v (%T)\n", q, v, v)
	}
}

// functionsOverFields: a value query can transform a record field. Field
// references in value position are reached through function arguments.
func functionsOverFields() {
	section("functions over fields")
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	rec := map[string]any{"name": "  Draft Invoice  "}
	for _, q := range []string{"upper(name)", "trim(name)", "len(trim(name))"} {
		prog, err := eval.CompileValue(q, fields)
		if err != nil {
			log.Fatalf("compile %q: %v", q, err)
		}
		v, err := prog.Eval(rec)
		if err != nil {
			log.Fatalf("eval %q: %v", q, err)
		}
		fmt.Printf("  %-18s => %#v\n", q, v)
	}
}

// timeValues: durations and the now() builtin combine into computed timestamps.
func timeValues() {
	section("time values")
	prog, err := eval.CompileValue("now()-7d", nil)
	if err != nil {
		log.Fatalf("compile: %v", err)
	}
	v, err := prog.Eval(nil)
	if err != nil {
		log.Fatalf("eval: %v", err)
	}
	when, _ := v.(time.Time)
	fmt.Printf("  %-18s => %s (~%v ago)\n", "now()-7d", when.Format(time.RFC3339), time.Since(when).Round(time.Hour))
}

// lists: a function returning a slice is preserved as a collection, and the
// list-aware built-ins (len, count, first, last, sum, avg, contains) operate on
// it. labels() and amounts() stand in for list-valued record fields.
func lists() {
	section("lists")
	withFns := eval.WithFunctions(
		eval.Func{Name: "labels", Call: func(...any) (any, error) {
			return []string{"urgent", "backend", "p0"}, nil
		}},
		eval.Func{Name: "amounts", Call: func(...any) (any, error) {
			return []float64{19.99, 5.50, 100.00}, nil
		}},
	)
	for _, q := range []string{
		"labels()",
		"count(labels())",
		"first(labels())",
		"last(labels())",
		"sum(amounts())",
		"avg(amounts())",
	} {
		prog, err := eval.CompileValue(q, nil, withFns)
		if err != nil {
			log.Fatalf("compile %q: %v", q, err)
		}
		v, err := prog.Eval(nil)
		if err != nil {
			log.Fatalf("eval %q: %v", q, err)
		}
		fmt.Printf("  %-18s => %#v\n", q, v)
	}
}

// errorHandling: an expression that cannot resolve returns eval.ErrNoValue
// rather than a silently-wrong zero value.
func errorHandling() {
	section("error handling")
	prog, err := eval.CompileValue("5/0", nil)
	if err != nil {
		log.Fatalf("compile: %v", err)
	}
	_, err = prog.Eval(nil)
	fmt.Printf("  %-18s => err: %v (is ErrNoValue: %v)\n", "5/0", err, errors.Is(err, eval.ErrNoValue))
}

// booleanPredicate: the predicate domain — eval.Compile + Match answer a
// boolean query against a record, alongside the value domain shown above.
func booleanPredicate() {
	section("boolean predicate")
	fields := []validate.FieldConfig{
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	}
	prog, err := eval.Compile("total>50000", fields)
	if err != nil {
		log.Fatalf("compile: %v", err)
	}
	fmt.Printf("  %-18s {total:60000} => %v\n", "total>50000", prog.Match(map[string]any{"total": 60000}))
	fmt.Printf("  %-18s {total:100}   => %v\n", "total>50000", prog.Match(map[string]any{"total": 100}))
}

func section(name string) {
	fmt.Printf("\n# %s\n", name)
}
