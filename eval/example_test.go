package eval_test

import (
	"fmt"

	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

// CompileValue compiles a value expression — one that computes and returns a
// value rather than a boolean. Arithmetic precedence and grouping are honored;
// integer division promotes to float, mirroring the predicate engine.
func ExampleCompileValue() {
	prog, err := eval.CompileValue("(100+50)*2", nil)
	if err != nil {
		panic(err)
	}
	v, err := prog.Eval(nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%v (%T)\n", v, v)
	// Output: 300 (int64)
}

// A value expression can call functions over record fields, returning the
// transformed value.
func ExampleCompileValue_function() {
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	}
	prog, err := eval.CompileValue("upper(name)", fields)
	if err != nil {
		panic(err)
	}
	v, err := prog.Eval(map[string]any{"name": "draft"})
	if err != nil {
		panic(err)
	}
	fmt.Println(v)
	// Output: DRAFT
}

// A value expression can return a list. A function that yields a slice is
// preserved as a collection, and len() reports its element count.
func ExampleCompileValue_list() {
	labels := eval.WithFunctions(eval.Func{
		Name: "labels",
		Call: func(...any) (any, error) { return []string{"urgent", "backend"}, nil },
	})

	list, err := eval.CompileValue("labels()", nil, labels)
	if err != nil {
		panic(err)
	}
	v, _ := list.Eval(nil)
	fmt.Printf("list:  %v\n", v)

	count, err := eval.CompileValue("len(labels())", nil, labels)
	if err != nil {
		panic(err)
	}
	n, _ := count.Eval(nil)
	fmt.Printf("count: %v\n", n)
	// Output:
	// list:  [urgent backend]
	// count: 2
}

// When a value expression cannot resolve — division by zero, a missing field —
// Eval returns eval.ErrNoValue rather than a silently-wrong result. (In a
// boolean predicate the same condition folds to a false comparison.)
func ExampleValueProgram_errNoValue() {
	prog, err := eval.CompileValue("5/0", nil)
	if err != nil {
		panic(err)
	}
	_, err = prog.Eval(nil)
	fmt.Println(err)
	// Output: expression did not resolve to a value
}
