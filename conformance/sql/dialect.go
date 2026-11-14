// Package sql checks the query language against a real SQL engine.
//
// It is a separate Go module so the library itself keeps zero dependencies;
// the SQLite driver (modernc.org/sqlite, pure Go) lives only here. The tests
// take every case in testdata/cases.json, run it through the evaluator and
// through SQLite over the same records, and require both to produce the set of
// ids the case expects. A disagreement is a semantic drift between the
// evaluator and the documented SQL lowering, or a wrong expectation.
//
// The dialect in this file is the reference lowering for SQLite. It is written
// to agree with the evaluator, which means a few deliberate choices:
//
//   - Every comparison is wrapped in COALESCE(..., 0) so NULL behaves like a
//     missing field (false, and true under NOT), rather than SQL's three-valued
//     logic where NOT NULL is NULL.
//   - Text columns are created COLLATE NOCASE, matching case-insensitive
//     equality; LIKE is case-insensitive in SQLite for ASCII already.
//   - Timestamps are stored as RFC 3339 text in UTC with no fraction, so
//     lexicographic order is chronological; literals are bound in that form.
//   - A day literal is lowered to a half-open interval, as examples/sql does.
//   - Presence is `IS NOT NULL AND <> ”`, matching non-empty presence.
package sql

import (
	"fmt"
	"strings"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
	"github.com/heyllave/query/validate"
)

// Schema is the queryable surface of the invoices table used by the tests.
var Schema = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "customer", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "email", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "notes", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "zip", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "country", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "qty", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "paid", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
	{Name: "created_at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
}

// TextColumns are the schema's text fields, created COLLATE NOCASE.
var TextColumns = []string{"state", "customer", "email", "notes", "zip", "country"}

// functions maps query functions to SQLite expressions over one argument.
var functions = map[string]string{
	"lower": "lower(%s)",
	"upper": "upper(%s)",
	"trim":  "trim(%s)",
	"len":   "length(%s)",
}

// Visitor lowers a validated query to a SQLite WHERE clause with `?`
// placeholders. Build one per query; Params holds the bound values in order.
type Visitor struct {
	Params []any
	err    error
}

// Err returns the first lowering error, if any.
func (v *Visitor) Err() error { return v.err }

func (v *Visitor) fail(format string, args ...any) string {
	if v.err == nil {
		v.err = fmt.Errorf(format, args...)
	}
	return "0"
}

func (v *Visitor) bind(val any) string {
	v.Params = append(v.Params, val)
	return "?"
}

func (v *Visitor) column(fp ast.FieldPath) string {
	name := fp.String()
	for _, f := range Schema {
		if f.Name == name {
			return name
		}
	}
	return v.fail("field %q has no column", name)
}

// VisitBinary renders AND/OR; an OR under an AND is parenthesized.
func (v *Visitor) VisitBinary(e *ast.BinaryExpr) string {
	left := v.operand(e.Op, e.Left)
	right := v.operand(e.Op, e.Right)
	if e.Op == token.And {
		return left + " AND " + right
	}
	return left + " OR " + right
}

func (v *Visitor) operand(parent token.Type, child ast.Expression) string {
	s := ast.Visit[string](v, child)
	if b, ok := child.(*ast.BinaryExpr); ok && parent == token.And && b.Op == token.Or {
		return "(" + s + ")"
	}
	return s
}

// VisitUnary renders NOT over a two-valued operand.
func (v *Visitor) VisitUnary(e *ast.UnaryExpr) string {
	return "NOT (" + ast.Visit[string](v, e.Expr) + ")"
}

// VisitGroup keeps the user's parentheses.
func (v *Visitor) VisitGroup(e *ast.GroupExpr) string {
	return "(" + ast.Visit[string](v, e.Expr) + ")"
}

// VisitPresence renders non-empty presence.
func (v *Visitor) VisitPresence(e *ast.PresenceExpr) string {
	col := v.column(e.Field)
	return fmt.Sprintf("(%s IS NOT NULL AND %s <> '')", col, col)
}

// VisitSelector has no table to join; lists are out of scope for this table.
func (v *Visitor) VisitSelector(*ast.SelectorExpr) string {
	return v.fail("selectors are not supported by the invoices dialect")
}

// VisitFuncCall renders a standalone boolean call; none are supported here.
func (v *Visitor) VisitFuncCall(e *ast.FuncCallExpr) string {
	return v.fail("function %q cannot be a predicate in this dialect", e.Name)
}

// VisitQualifier renders a comparison, two-valued.
func (v *Visitor) VisitQualifier(e *ast.QualifierExpr) string {
	var left string
	if e.HasFieldFunc() {
		left = v.renderCall(e.FieldFunc)
	} else {
		left = v.column(e.Field)
	}
	return "COALESCE((" + v.compare(left, e) + "), 0)"
}

func (v *Visitor) compare(left string, e *ast.QualifierExpr) string {
	if e.IsRange() {
		if e.EndValue.Type == ast.ValueDate && e.EndValue.DayOnly {
			return fmt.Sprintf("%s >= %s AND %s < %s", left, v.renderValue(&e.Value), left, v.bindTime(e.EndValue.Date.AddDate(0, 0, 1)))
		}
		return fmt.Sprintf("%s BETWEEN %s AND %s", left, v.renderValue(&e.Value), v.renderValue(e.EndValue))
	}
	if e.IsWildcard() {
		op := "LIKE"
		if e.Operator == token.Neq {
			op = "NOT LIKE"
		}
		return fmt.Sprintf(`%s %s %s ESCAPE '\'`, left, op, v.bind(ast.WildcardToLike(e.Value.Str)))
	}
	if e.Value.Type == ast.ValueDate && e.Value.DayOnly {
		day := e.Value.Date
		next := day.AddDate(0, 0, 1)
		switch e.Operator { //nolint:exhaustive // the rest use the plain form
		case token.Eq:
			return fmt.Sprintf("%s >= %s AND %s < %s", left, v.bindTime(day), left, v.bindTime(next))
		case token.Neq:
			return fmt.Sprintf("%s < %s OR %s >= %s", left, v.bindTime(day), left, v.bindTime(next))
		case token.Lte:
			return fmt.Sprintf("%s < %s", left, v.bindTime(next))
		case token.Gt:
			return fmt.Sprintf("%s >= %s", left, v.bindTime(next))
		}
	}
	return fmt.Sprintf("%s %s %s", left, ast.SQLOperator(e.Operator, false), v.renderValue(&e.Value))
}

func (v *Visitor) bindTime(t time.Time) string {
	return v.bind(t.UTC().Format(time.RFC3339))
}

// renderValue binds a literal or renders a field reference, call or
// arithmetic inline over bound literals.
func (v *Visitor) renderValue(val *ast.Value) string {
	if val == nil {
		return "NULL"
	}
	switch val.Type {
	case ast.ValueFieldRef:
		return v.column(val.Field)
	case ast.ValueFunc:
		return v.renderCall(val.Func)
	case ast.ValueArith:
		return "(" + v.renderValue(val.Arith.Left) + " " + val.Arith.Op.String() + " " + v.renderValue(val.Arith.Right) + ")"
	case ast.ValueDate:
		return v.bindTime(val.Date)
	case ast.ValueBoolean:
		if val.Bool {
			return v.bind(1)
		}
		return v.bind(0)
	case ast.ValueDuration:
		return v.fail("durations have no column form in this dialect")
	default:
		return v.bind(val.Any())
	}
}

func (v *Visitor) renderCall(fc *ast.FuncCallExpr) string {
	tpl, ok := functions[fc.Name]
	if !ok {
		return v.fail("function %q is not available in SQL", fc.Name)
	}
	if len(fc.Args) != 1 {
		return v.fail("%s() takes 1 argument", fc.Name)
	}
	var arg string
	switch a := fc.Args[0]; {
	case a.Field != nil:
		arg = v.column(*a.Field)
	case a.Value != nil:
		arg = v.renderValue(a.Value)
	case a.Call != nil:
		arg = v.renderCall(a.Call)
	}
	return fmt.Sprintf(tpl, arg)
}

// Where lowers a validated expression to a WHERE clause and its parameters.
func Where(expr ast.Expression) (string, []any, error) {
	v := &Visitor{}
	where := ast.Visit[string](v, expr)
	if v.err != nil {
		return "", nil, v.err
	}
	return where, v.Params, nil
}

// CreateTable is the DDL for the invoices table the tests load.
func CreateTable() string {
	cols := []string{"id INTEGER PRIMARY KEY"}
	for _, c := range TextColumns {
		cols = append(cols, c+" TEXT COLLATE NOCASE")
	}
	cols = append(cols, "total REAL", "qty INTEGER", "paid INTEGER", "created_at TEXT")
	return "CREATE TABLE invoices (" + strings.Join(cols, ", ") + ")"
}
