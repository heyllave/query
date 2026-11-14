// Package query provides a query language lexer, parser, AST, and validator
// for the Trazo platform.
//
// It implements a unified query syntax used across all clients (CLI, Web UI,
// API, WASM) with zero external dependencies. All semantic evaluation
// (SQL generation, execution) lives in the consumer.
//
// # Sub-packages
//
//   - [token] — lexical token types and position tracking
//   - [ast] — AST node types, Visitor pattern, Walk, String
//   - [parser] — lexer and recursive descent parser
//   - [validate] — field configuration, AST validation and sandboxing options
//   - [eval] — compile a query to a predicate ([eval.Compile]) or a value
//     ([eval.CompileValue]) and run it against maps or structs
//
// The command in cmd/query explains a query's tokens and tree, and the
// wasm and clients directories hold the JavaScript binding and a C shared
// library, which run the same engine.
//
// # Basic usage
//
//	expr, err := query.Parse("state=draft AND total>50000")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fields := []validate.FieldConfig{
//	    {Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
//	    {Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
//	}
//	if err := query.Validate(expr, fields); err != nil {
//	    log.Fatal(err)
//	}
//
//	ok, err := query.Match("state=draft AND total>50000", fields,
//	    map[string]any{"state": "draft", "total": 60000}) // true
//
// # Guarantees
//
// Parsing never panics on any input, and [ast.String] of a parsed tree
// parses back to the same tree; both are fuzzed. A query that compiles can
// be matched against any record shape. A record value that cannot be read
// as the literal's type makes the comparison false rather than comparing as
// zero, and a missing field makes every comparison false, including through
// functions. (A negated operator is a NOT, so total!>5 is true for a record
// without a total.) Nesting is bounded by [parser.MaxNesting] whatever the
// length limit.
//
// # Code generation via Visitor
//
// Implement [ast.Visitor] to transform the AST into SQL, JSON, filter functions,
// React components, or any other target. See the package examples for SQL and
// JSON visitors.
package query
