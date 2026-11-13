// Package ast defines the abstract syntax tree for the query language.
//
// It provides the node types, a generic [Visitor] for transforming the AST
// into any target representation (SQL, JSON, filter functions, React components),
// and traversal utilities like [Walk], [Fields], [Calls], and [Qualifiers].
// [Walk] visits function calls wherever they occur, and [String] prints a
// tree as a query that parses back to the same tree.
//
// # Node Types
//
//   - [BinaryExpr]   — logical AND / OR
//   - [UnaryExpr]    — logical NOT
//   - [QualifierExpr] — field op value (e.g., state=draft); with FieldFunc set the
//     left side is a function (lower(name)=john) and Field is empty
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
