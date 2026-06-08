// Package parser implements the lexer and recursive descent parser for the
// query language. It converts a query string into an [ast.Expression] tree.
//
// [Parse] reads a boolean predicate (the default). [ParseValue] reads a
// value-producing expression — arithmetic, a function call, or a literal — as
// the root, producing an [ast.ValueExpr]; the [github.com/heyllave/query/eval]
// package builds on it via CompileValue.
//
// Most consumers should use the top-level [query.Parse] function, or
// eval.Compile / eval.CompileValue, instead of calling this package directly.
package parser
