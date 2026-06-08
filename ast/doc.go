// Package ast defines the abstract syntax tree for the query language.
//
// It provides the node types, a generic [Visitor] for transforming the AST
// into any target representation (SQL, JSON, filter functions, React components),
// and traversal utilities like [Walk], [Fields], and [Qualifiers].
//
// # Node Types
//
//   - [BinaryExpr]   — logical AND / OR
//   - [UnaryExpr]    — logical NOT
//   - [QualifierExpr] — field op value (e.g., state=draft)
//   - [PresenceExpr] — field existence check (e.g., tire_size)
//   - [GroupExpr]    — parenthesized expression
//   - [SelectorExpr] — @first / @last / @(expr)
//   - [FuncCallExpr] — function call (field transform, value generator, or predicate)
//   - [ValueExpr]    — a bare value root (the query computes a value, not a boolean)
//
// # Visitor Pattern
//
// Implement [Visitor] to transform the AST into your target output:
//
//	type sqlVisitor struct{}
//	func (v *sqlVisitor) VisitQualifier(e *ast.QualifierExpr) string {
//	    return fmt.Sprintf("%s %s $1", e.Field, ast.SQLOperator(e.Operator, e.Value.Wildcard))
//	}
//	// ... implement other methods ...
//
//	result := ast.Visit[string](v, expr)
package ast
