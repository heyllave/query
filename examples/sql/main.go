// Example: SQL WHERE clause generation from a query string.
//
// This demonstrates how to use ast.Visitor[T] to transform a parsed query
// into a parameterized SQL WHERE clause, suitable for use with database/sql
// or any query builder (e.g., squirrel).
//
// Everything that reaches the SQL text is allowlisted: the query is validated
// against a field schema, each field maps to a column through an explicit
// table, and each function maps to a SQL template through another. Literal
// values are bound as parameters. A field or function outside the allowlists
// is an error, never interpolated — so user input can name only what the
// adapter chose to expose.
//
// Covered:
//   - equality / comparison / wildcard (LIKE, NOT LIKE) / range (BETWEEN)
//   - day literals (createdAt=2026-09-28) as half-open intervals over the day
//   - logical AND/OR with precedence-aware parens around OR children
//   - NOT (UnaryExpr) and grouping (GroupExpr)
//   - field presence (IS NOT NULL)
//   - selectors @first / @last / @(...) / @any / @all / @none → EXISTS / NOT EXISTS
//   - function-valued operands (now(), daysAgo(7)) — translated through the
//     function allowlist
//   - arithmetic-valued operands (50000*1.1, now()-7d) — emitted as inline SQL
//     over bound parameters
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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/heyllave/query"
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
	"github.com/heyllave/query/validate"
)

// schema is the queryable surface: the fields a query may name, with their
// types and operators. Validation against it runs before any SQL is built.
var schema = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "created_at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
	{Name: "orders", Type: validate.TypeText, AllowedOps: []validate.Op{validate.OpPresence}},
	{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
}

// columns maps query fields to SQL column references. Only mapped fields can
// appear in the generated SQL; the schema above keeps the two lists aligned.
var columns = map[string]string{
	"state":      "invoices.state",
	"name":       "invoices.customer_name",
	"total":      "invoices.total",
	"created_at": "invoices.created_at",
	"orders":     "orders",
	"status":     "orders.status",
}

// sqlFunc renders one allowlisted function. args are already-rendered SQL
// fragments (bound parameters or nested expressions).
type sqlFunc func(args []string) (string, error)

// functions is the function allowlist with each function's SQL template and
// arity. An unlisted function is an error, so pg_sleep(10) cannot be smuggled
// through a query.
var functions = map[string]struct {
	arity  int
	render sqlFunc
}{
	"now":   {0, func([]string) (string, error) { return "NOW()", nil }},
	"today": {0, func([]string) (string, error) { return "CURRENT_DATE", nil }},
	"lower": {1, func(a []string) (string, error) { return "LOWER(" + a[0] + ")", nil }},
	"upper": {1, func(a []string) (string, error) { return "UPPER(" + a[0] + ")", nil }},
	"len":   {1, func(a []string) (string, error) { return "LENGTH(" + a[0] + ")", nil }},
	"daysAgo": {1, func(a []string) (string, error) {
		// Postgres-ish: NOW() - (<n> * INTERVAL '1 day')
		return fmt.Sprintf("NOW() - (%s * INTERVAL '1 day')", a[0]), nil
	}},
}

// sqlVisitor transforms a validated query AST into a SQL WHERE clause with
// numbered parameters ($1, $2, ...). The first error aborts rendering.
type sqlVisitor struct {
	params []any
	err    error
}

func (v *sqlVisitor) fail(format string, args ...any) string {
	if v.err == nil {
		v.err = fmt.Errorf(format, args...)
	}
	return "<error>"
}

func (v *sqlVisitor) column(fp ast.FieldPath) string {
	col, ok := columns[fp.String()]
	if !ok {
		return v.fail("field %q has no column mapping", fp)
	}
	return col
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
	var field string
	if e.HasFieldFunc() {
		field = v.renderFuncCall(e.FieldFunc)
	} else {
		field = v.column(e.Field)
	}

	// Range: field BETWEEN <left> AND <right>. A day literal at the end of a
	// range covers that whole day, so the bound becomes `< next day`.
	if e.IsRange() {
		if e.EndValue.Type == ast.ValueDate && e.EndValue.DayOnly {
			return fmt.Sprintf("(%s >= %s AND %s < %s)",
				field, v.renderValue(&e.Value), field, v.bindTime(e.EndValue.Date.AddDate(0, 0, 1)))
		}
		return fmt.Sprintf("%s BETWEEN %s AND %s",
			field, v.renderValue(&e.Value), v.renderValue(e.EndValue))
	}

	// A day literal (createdAt=2026-09-28) denotes the whole UTC day, the same
	// reading the evaluator uses, so `=` on a datetime column is a half-open
	// interval and the other operators move to the day's bounds.
	if e.Value.Type == ast.ValueDate && e.Value.DayOnly {
		if sql, ok := v.renderDay(field, e.Operator, e.Value.Date); ok {
			return sql
		}
	}

	// Wildcard: field LIKE $1 / field NOT LIKE $1
	if e.IsWildcard() {
		v.params = append(v.params, ast.WildcardToLike(e.Value.Str))
		return fmt.Sprintf("%s %s $%d", field, ast.SQLOperator(e.Operator, true), len(v.params))
	}

	// Standard comparison: field op <value>
	op := ast.SQLOperator(e.Operator, false)
	return fmt.Sprintf("%s %s %s", field, op, v.renderValue(&e.Value))
}

// bindTime binds a timestamp as a parameter and returns its placeholder.
func (v *sqlVisitor) bindTime(t time.Time) string {
	v.params = append(v.params, t)
	return fmt.Sprintf("$%d", len(v.params))
}

// renderDay lowers a comparison against a whole day. ok is false for
// operators whose lowering is the plain one (>= start, < start).
func (v *sqlVisitor) renderDay(field string, op token.Type, day time.Time) (string, bool) {
	next := day.AddDate(0, 0, 1)
	switch op { //nolint:exhaustive // the rest use the plain lowering
	case token.Eq:
		return fmt.Sprintf("(%s >= %s AND %s < %s)", field, v.bindTime(day), field, v.bindTime(next)), true
	case token.Neq:
		return fmt.Sprintf("(%s < %s OR %s >= %s)", field, v.bindTime(day), field, v.bindTime(next)), true
	case token.Lte:
		return fmt.Sprintf("%s < %s", field, v.bindTime(next)), true
	case token.Gt:
		return fmt.Sprintf("%s >= %s", field, v.bindTime(next)), true
	default:
		return "", false
	}
}

func (v *sqlVisitor) VisitPresence(e *ast.PresenceExpr) string {
	return v.column(e.Field) + " IS NOT NULL"
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
	p, ok := e.Base.(*ast.PresenceExpr)
	if !ok {
		return v.fail("selector base must be a field")
	}
	listField := v.column(p.Field)
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
// arithmetic and function calls live in the SQL world, while every literal,
// including those inside arithmetic, stays parameterized.
func (v *sqlVisitor) renderValue(val *ast.Value) string {
	if val == nil {
		return "NULL"
	}
	switch val.Type {
	case ast.ValueFunc:
		return v.renderFuncCall(val.Func)
	case ast.ValueArith:
		return v.renderArith(val.Arith)
	case ast.ValueFieldRef:
		return v.column(val.Field)
	case ast.ValueDuration:
		// A duration becomes an interval of whole seconds.
		v.params = append(v.params, int64(val.Duration/time.Second))
		return fmt.Sprintf("($%d * INTERVAL '1 second')", len(v.params))
	default:
		v.params = append(v.params, val.Any())
		return fmt.Sprintf("$%d", len(v.params))
	}
}

func (v *sqlVisitor) renderArith(a *ast.ArithExpr) string {
	return "(" + v.renderValue(a.Left) + " " + a.Op.String() + " " + v.renderValue(a.Right) + ")"
}

// renderFuncCall translates an allowlisted function through its template.
// Field arguments go through the column map, literals are bound, and nested
// calls recurse. Anything not in the allowlist is an error.
func (v *sqlVisitor) renderFuncCall(fc *ast.FuncCallExpr) string {
	if fc == nil {
		return v.fail("nil function call")
	}
	spec, ok := functions[fc.Name]
	if !ok {
		return v.fail("function %q is not available in SQL queries", fc.Name)
	}
	if len(fc.Args) != spec.arity {
		return v.fail("%s() takes %d argument(s), got %d", fc.Name, spec.arity, len(fc.Args))
	}
	args := make([]string, 0, len(fc.Args))
	for _, a := range fc.Args {
		switch {
		case a.Field != nil:
			args = append(args, v.column(*a.Field))
		case a.Value != nil:
			args = append(args, v.renderValue(a.Value))
		case a.Call != nil:
			args = append(args, v.renderFuncCall(a.Call))
		}
	}
	out, err := spec.render(args)
	if err != nil {
		return v.fail("%s(): %v", fc.Name, err)
	}
	return out
}

// toSQL validates the query against the schema and renders it. It is the
// only entry point: nothing reaches the visitor unvalidated.
func toSQL(input string) (string, []any, error) {
	expr, err := query.ParseAndValidate(input, schema)
	if err != nil {
		return "", nil, err
	}
	v := &sqlVisitor{}
	where := ast.Visit[string](v, expr)
	if v.err != nil {
		return "", nil, v.err
	}
	return where, v.params, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <query>\n", os.Args[0])
		os.Exit(1)
	}
	input := os.Args[1]

	where, params, err := toSQL(input)
	if err != nil {
		var verrs validate.ErrorList
		if errors.As(err, &verrs) {
			fmt.Fprintf(os.Stderr, "query rejected: %v\n", strings.Join(messages(verrs), "; "))
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("Input:  %s\n", input)
	fmt.Printf("WHERE:  %s\n", where)
	fmt.Printf("Params: %v\n", params)
}

func messages(errs validate.ErrorList) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Message
	}
	return out
}
