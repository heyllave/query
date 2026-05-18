// Example: SQL WHERE clause generation from a query string.
//
// This demonstrates how to use ast.Visitor[T] to transform a parsed query
// into a parameterized SQL WHERE clause, suitable for use with database/sql
// or any query builder (e.g., squirrel).
//
// Run:
//
//	go run ./examples/sql "state=draft AND total>50000"
//	go run ./examples/sql "(state=draft OR state=issued) AND name=John*"
//	go run ./examples/sql "created_at:2026-01-01..2026-03-31"
package main

import (
	"fmt"
	"os"

	"github.com/heyllave/query"
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// sqlVisitor transforms a query AST into a SQL WHERE clause with numbered
// parameters ($1, $2, ...) for safe parameterized queries.
type sqlVisitor struct {
	params []any
}

func (v *sqlVisitor) VisitBinary(e *ast.BinaryExpr) string {
	left := ast.Visit[string](v, e.Left)
	right := ast.Visit[string](v, e.Right)
	if e.Op == token.And {
		return left + " AND " + right
	}
	return left + " OR " + right
}

func (v *sqlVisitor) VisitUnary(e *ast.UnaryExpr) string {
	return "NOT (" + ast.Visit[string](v, e.Expr) + ")"
}

func (v *sqlVisitor) VisitQualifier(e *ast.QualifierExpr) string {
	field := e.Field.String()

	// Range: field BETWEEN $1 AND $2
	if e.IsRange() {
		v.params = append(v.params, e.Value.Any(), e.EndValue.Any())
		return fmt.Sprintf("%s BETWEEN $%d AND $%d", field, len(v.params)-1, len(v.params))
	}

	// Wildcard: field LIKE $1
	if e.IsWildcard() {
		v.params = append(v.params, ast.WildcardToLike(e.Value.Str))
		return fmt.Sprintf("%s LIKE $%d", field, len(v.params))
	}

	// Standard comparison: field op $1
	op := ast.SQLOperator(e.Operator, false)
	v.params = append(v.params, e.Value.Any())
	return fmt.Sprintf("%s %s $%d", field, op, len(v.params))
}

func (v *sqlVisitor) VisitPresence(e *ast.PresenceExpr) string {
	return e.Field.String() + " IS NOT NULL"
}

func (v *sqlVisitor) VisitGroup(e *ast.GroupExpr) string {
	return "(" + ast.Visit[string](v, e.Expr) + ")"
}

func (v *sqlVisitor) VisitSelector(e *ast.SelectorExpr) string {
	return ast.Visit[string](v, e.Base)
}

func (v *sqlVisitor) VisitFuncCall(e *ast.FuncCallExpr) string {
	return ""
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <query>\n", os.Args[0])
		os.Exit(1)
	}
	input := os.Args[1]

	expr, err := query.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	v := &sqlVisitor{}
	where := ast.Visit[string](v, expr)

	fmt.Printf("Input:  %s\n", input)
	fmt.Printf("WHERE:  %s\n", where)
	fmt.Printf("Params: %v\n", v.params)
}
