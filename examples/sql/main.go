// Example: SQL WHERE clause generation from a query string.
//
// This demonstrates how to use ast.Visitor[T] to transform a parsed query
// into a parameterized SQL WHERE clause, suitable for use with database/sql
// or any query builder (e.g., squirrel).
//
// Covered:
//   - equality / comparison / wildcard (LIKE) / range (BETWEEN)
//   - logical AND/OR with precedence-aware parens around OR children
//   - NOT (UnaryExpr) and grouping (GroupExpr)
//   - field presence (IS NOT NULL)
//   - selectors @first / @last / @(...) / @any / @all / @none → EXISTS / NOT EXISTS
//   - function-valued operands (now(), daysAgo(7)) — emitted as SQL function calls
//   - arithmetic-valued operands (50000*1.1, now()-7d) — emitted as inline SQL
//
// Run:
//
//	go run ./examples/sql "state=draft AND total>50000"
//	go run ./examples/sql "(state=draft OR state=issued) AND name=John*"
//	go run ./examples/sql "created_at:2026-01-01..2026-03-31"
//	go run ./examples/sql "created_at>=now()-7d"
//	go run ./examples/sql "orders@(status=shipped)"
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/heyllave/query"
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// sqlVisitor transforms a query AST into a SQL WHERE clause with numbered
// parameters ($1, $2, ...) for safe parameterized queries.
//
// Function-valued and arithmetic-valued operands are emitted inline (as SQL
// expressions, not bound parameters) because they reference SQL-side state
// (NOW(), date arithmetic) rather than user-supplied literals. A real adapter
// might whitelist which functions to translate; this example wires the
// common time builtins.
type sqlVisitor struct {
	params []any
}

func (v *sqlVisitor) VisitBinary(e *ast.BinaryExpr) string {
	left := v.binOperand(e.Op, e.Left)
	right := v.binOperand(e.Op, e.Right)
	if e.Op == token.And {
		return left + " AND " + right
	}
	return left + " OR " + right
}

// binOperand wraps a child expression in parens when its operator has lower
// precedence than the current parent. Without this, AND over an OR child
// (e.g. `state IN (draft, issued) AND total>10000`) would emit ambiguous SQL.
func (v *sqlVisitor) binOperand(parentOp token.Type, child ast.Expression) string {
	s := ast.Visit[string](v, child)
	if parentOp == token.And {
		if b, ok := child.(*ast.BinaryExpr); ok && b.Op == token.Or {
			return "(" + s + ")"
		}
	}
	return s
}

func (v *sqlVisitor) VisitUnary(e *ast.UnaryExpr) string {
	return "NOT (" + ast.Visit[string](v, e.Expr) + ")"
}

func (v *sqlVisitor) VisitQualifier(e *ast.QualifierExpr) string {
	field := e.Field.String()
	if e.HasFieldFunc() {
		field = v.renderFuncCall(e.FieldFunc)
	}

	// Range: field BETWEEN <left> AND <right>
	if e.IsRange() {
		return fmt.Sprintf("%s BETWEEN %s AND %s",
			field, v.renderValue(&e.Value), v.renderValue(e.EndValue))
	}

	// Wildcard: field LIKE $1
	if e.IsWildcard() {
		v.params = append(v.params, ast.WildcardToLike(e.Value.Str))
		return fmt.Sprintf("%s LIKE $%d", field, len(v.params))
	}

	// Standard comparison: field op <value>
	op := ast.SQLOperator(e.Operator, false)
	return fmt.Sprintf("%s %s %s", field, op, v.renderValue(&e.Value))
}

func (v *sqlVisitor) VisitPresence(e *ast.PresenceExpr) string {
	return e.Field.String() + " IS NOT NULL"
}

func (v *sqlVisitor) VisitGroup(e *ast.GroupExpr) string {
	return "(" + ast.Visit[string](v, e.Expr) + ")"
}

// VisitSelector translates the selector kinds into EXISTS / NOT EXISTS over a
// correlated subquery. The base of a selector is a presence reference to the
// list field, so we read its field path directly rather than recursing — the
// PresenceExpr visitor would emit "<field> IS NOT NULL", which is wrong here.
// Real adapters would resolve the join target (table, FK) from a schema; this
// example just shows the shape.
func (v *sqlVisitor) VisitSelector(e *ast.SelectorExpr) string {
	listField := "<list>"
	if p, ok := e.Base.(*ast.PresenceExpr); ok {
		listField = p.Field.String()
	}
	// @first / @last → list non-emptiness.
	if e.Inner == nil {
		return fmt.Sprintf("EXISTS (SELECT 1 FROM %s)", listField)
	}
	inner := ast.Visit[string](v, e.Inner)
	switch e.Selector {
	case "all":
		return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s WHERE NOT (%s))", listField, inner)
	case "none":
		return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s WHERE %s)", listField, inner)
	default: // "" (anonymous) and "any"
		return fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE %s)", listField, inner)
	}
}

// VisitFuncCall renders a standalone boolean function call (e.g.
// `contains(name, "urgent")` as a top-level filter). Field-transform and
// value-position calls are handled inline by renderFuncCall.
func (v *sqlVisitor) VisitFuncCall(e *ast.FuncCallExpr) string {
	return v.renderFuncCall(e)
}

// renderValue emits a value either as a bound parameter (for literals) or as
// inline SQL (for ValueFunc and ValueArith). Mixing the two is intentional:
// arithmetic and function calls live in the SQL world, while user input must
// stay parameterized to avoid injection.
func (v *sqlVisitor) renderValue(val *ast.Value) string {
	if val == nil {
		return "NULL"
	}
	switch val.Type {
	case ast.ValueFunc:
		return v.renderFuncCall(val.Func)
	case ast.ValueArith:
		return v.renderArith(val.Arith)
	default:
		v.params = append(v.params, val.Any())
		return fmt.Sprintf("$%d", len(v.params))
	}
}

func (v *sqlVisitor) renderArith(a *ast.ArithExpr) string {
	return "(" + v.renderValue(a.Left) + " " + a.Op + " " + v.renderValue(a.Right) + ")"
}

// renderFuncCall translates a few well-known functions to their SQL
// equivalents and falls back to passing the call through verbatim. Real
// adapters should whitelist this map.
func (v *sqlVisitor) renderFuncCall(fc *ast.FuncCallExpr) string {
	if fc == nil {
		return ""
	}
	args := make([]string, 0, len(fc.Args))
	for _, a := range fc.Args {
		switch {
		case a.Field != nil:
			args = append(args, a.Field.String())
		case a.Value != nil:
			args = append(args, v.renderValue(a.Value))
		case a.Call != nil:
			args = append(args, v.renderFuncCall(a.Call))
		}
	}
	name := fc.Name
	switch name {
	case "now":
		name = "NOW"
	case "today":
		return "CURRENT_DATE"
	case "daysAgo":
		// Postgres-ish: NOW() - INTERVAL '<n> days'
		if len(args) == 1 {
			return fmt.Sprintf("NOW() - (%s * INTERVAL '1 day')", args[0])
		}
	}
	return name + "(" + strings.Join(args, ", ") + ")"
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
