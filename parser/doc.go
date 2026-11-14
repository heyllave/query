// Package parser implements the lexer and recursive descent parser for the
// query language. It converts a query string into an [ast.Expression] tree.
//
// [Parse] reads a boolean predicate (the default). [ParseValue] reads a
// value-producing expression — arithmetic, a function call, or a literal — as
// the root, producing an [ast.ValueExpr]; the [github.com/heyllave/query/eval]
// package builds on it via CompileValue.
//
// # Lexical notes
//
// Field names are identifiers (letters including any non-ASCII character,
// digits, '_', '-', '/') joined by
// dots, or a bracket-quoted name (["billing address"]) for anything else.
// The reserved words true, false and in may name a field anywhere; and, or
// and not only when an operator follows.
//
// An unquoted value is classified by shape: an integer, float, day
// (2026-01-01, denoting the whole UTC day), datetime (2026-01-01T10:00:00Z,
// an instant), duration (7d), boolean, or
// wildcard pattern (jo*), else a string. A value that contains letters is
// always a string (a UUID or a version is one value); a run of numbers,
// operators, durations, parens, [field refs] and calls is arithmetic, so
// 10-3, 10 - 3 and 10+3 all compute. The validator reconciles a classified
// literal with the field's declared type, so zip=90210 compares as text on a
// text field. Escapes \* \( \) \\ stand for the literal character.
//
// Most consumers should use the top-level [query.Parse] function, or
// eval.Compile / eval.CompileValue, instead of calling this package directly.
package parser
