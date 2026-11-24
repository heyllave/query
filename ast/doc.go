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
//	    return fmt.Sprintf("%s %s $1", e.Field, e.Operator)
//	}
//	// ... implement other methods ...
//
//	result := ast.Visit[string](v, expr)
//
// SQL-specific rendering helpers (operator and wildcard translation) live in
// the output package, not here — ast stays backend-agnostic.
//
// # Adding a node type
//
// The [Expression] set is sealed (the unexported node()/expr() markers), so the
// node types are a closed set only this module extends. Adding one touches
// several type switches, and most of them have a default arm that would ignore
// an unhandled node silently, so follow this checklist — the ast
// dispatch-guard test fails until the in-package steps are done:
//
//   - ast/node.go — define the type and its node()/expr() markers.
//   - ast/visitor.go — add a method to [Visitor] and a case to [Visit].
//   - ast/walk.go — a case in [Walk] (and [Fields] if it carries field refs).
//   - ast/string.go — a case so [String] round-trips it.
//   - ast/dispatch_guard_test.go — add it to allNodeTypes and the dispatch map.
//   - validate/validate.go, eval/matcher.go — handle it where queries are
//     validated and compiled.
//   - internal/bridgejson — both directions (AstToJSON and nodeToAST) and the
//     matching client types, if the node crosses the JSON contract.
//   - output/json.go, output/tree.go — compile errors flag these two.
package ast
