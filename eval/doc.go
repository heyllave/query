// Package eval compiles a query string into an executable program that can
// be evaluated against data. It combines parsing, validation, and evaluation
// into a single pipeline.
//
// # Compile and Match
//
//	fields := []validate.FieldConfig{
//	    {Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
//	    {Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
//	}
//
//	prog, err := eval.Compile("state=draft AND total>50000", fields)
//	if err != nil { ... }
//
//	prog.Match(map[string]any{"state": "draft", "total": 60000}) // true
//	prog.Match(map[string]any{"state": "draft", "total": 100})   // false
//
// # Compile and evaluate a value
//
// A query can also compute and return a value instead of a boolean. Use
// [CompileValue] for a value-producing expression — arithmetic, a function
// call, or a literal — and [ValueProgram.Eval] to get the typed Go value:
//
//	prog, err := eval.CompileValue("(50000+1000)*1.1", nil)
//	v, err := prog.Eval(nil) // 56100.00000000001 (float64)
//
//	prog, err = eval.CompileValue("now()-7d", nil)
//	v, err = prog.Eval(nil)  // time.Time, seven days ago
//
// A function returning a slice is preserved as a list ([]any); len() counts
// list elements. When an expression cannot resolve (division or modulo by
// zero, a missing field), Eval returns [ErrNoValue] rather than a
// silently-wrong zero.
//
// # Restrict allowed operations
//
//	prog, err := eval.Compile(q, fields,
//	    eval.WithAllowedOps(validate.OpEq, validate.OpNeq),  // no >, <, etc.
//	    eval.WithAllowedFields("state", "name"),              // only these fields
//	    eval.WithMaxDepth(3),                                 // limit nesting
//	)
//
// # Custom data accessor
//
//	prog.MatchFunc(func(field string) (any, bool) {
//	    return myRecord.Get(field)
//	})
package eval
