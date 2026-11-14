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
// # Functions
//
// Built-ins cover strings, numbers, dates, lists and coercions; register
// more with [WithFunctions]. A call to an unregistered function, or with an
// argument count outside the function's [Arity], is a compile error
// ([ErrUnknownFunction], [ErrArity]). A function that returns nil or an error
// resolves to no value, so the comparison around it is false; a missing
// field is passed to a function as nil, and the built-ins treat it the same
// way.
//
// # Records
//
// [Program.Match] reads a map, resolving a dotted field through nested maps
// when the flat key is absent (a key may itself contain dots, as label keys
// do: labels.example.com/team); [TypedProgram.MatchStruct] reads `query`
// tags, dereferencing pointer fields. Every Go numeric kind is read.
//
// # Restrict allowed operations
//
//	prog, err := eval.Compile(q, fields,
//	    eval.WithAllowedOps(validate.OpEq, validate.OpNeq),  // no >, <, etc.
//	    eval.WithAllowedFields("state", "name"),              // only these fields
//	    eval.WithMaxDepth(3),                                 // limit nesting
//	)
//
// The field and operator restrictions are enforced by the validator after
// field resolution, so they also bind a custom validator and reach function
// arguments, predicate functions and fields read on the value side of a
// comparison; see package validate for the exact rules. Depth counts
// nesting, not the length of a connective chain.
//
// # Custom data accessor
//
//	prog.MatchFunc(func(field string) (any, bool) {
//	    return myRecord.Get(field)
//	})
package eval
