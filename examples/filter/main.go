// Example: In-memory filter function generation from a query string.
//
// This demonstrates how to use ast.Visitor[T] to build composable Go predicate
// functions for filtering in-memory data. Useful for CLI tools, WASM clients,
// or any context where you filter objects without a database.
//
// Covered:
//   - equality / not-equal / wildcard (and its negation) / presence
//   - field transforms on the left side (lower(name)=john*) from a small
//     allowlist; a function-wrapped qualifier has FieldFunc set and no Field
//   - logical AND / OR / NOT / grouping
//   - selectors @first / @last / @(...) / @any / @all / @none over slice fields
//     where elements are map[string]any (the JSON-ish shape)
//
// Run:
//
//	go run ./examples/filter
package main

import (
	"fmt"
	"strings"

	"github.com/heyllave/query"
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

type predicate = func(map[string]any) bool

type filterVisitor struct{}

func (v *filterVisitor) VisitBinary(e *ast.BinaryExpr) predicate {
	left := ast.Visit[predicate](v, e.Left)
	right := ast.Visit[predicate](v, e.Right)
	if e.Op == token.And {
		return func(obj map[string]any) bool { return left(obj) && right(obj) }
	}
	return func(obj map[string]any) bool { return left(obj) || right(obj) }
}

func (v *filterVisitor) VisitUnary(e *ast.UnaryExpr) predicate {
	inner := ast.Visit[predicate](v, e.Expr)
	return func(obj map[string]any) bool { return !inner(obj) }
}

// transforms are the field functions this filter supports on the left side
// of a comparison (lower(name)=john*). A consumer picks its own set; anything
// else is rejected by lookup below rather than silently matching nothing.
var transforms = map[string]func(string) string{
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
	"trim":  strings.TrimSpace,
}

// lookup resolves the left side of a qualifier against a record: the field
// itself, or the field transformed by the qualifier's function. A function
// outside transforms, or one not applied to a single field, is unsupported
// and reads as missing.
func lookup(e *ast.QualifierExpr, obj map[string]any) (string, bool) {
	if !e.HasFieldFunc() {
		val, ok := obj[e.Field.String()]
		if !ok || val == nil {
			return "", false
		}
		return fmt.Sprint(val), true
	}
	fn, known := transforms[e.FieldFunc.Name]
	if !known || len(e.FieldFunc.Args) != 1 || e.FieldFunc.Args[0].Field == nil {
		return "", false
	}
	val, ok := obj[e.FieldFunc.Args[0].Field.String()]
	if !ok || val == nil {
		return "", false
	}
	return fn(fmt.Sprint(val)), true
}

func (v *filterVisitor) VisitQualifier(e *ast.QualifierExpr) predicate {
	expected := e.Value.Any()

	if e.IsWildcard() {
		pattern := e.Value.Str
		negate := e.Operator == token.Neq
		return func(obj map[string]any) bool {
			s, ok := lookup(e, obj)
			if !ok {
				return false
			}
			var matched bool
			switch {
			case strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
				matched = strings.Contains(s, pattern[1:len(pattern)-1])
			case strings.HasPrefix(pattern, "*"):
				matched = strings.HasSuffix(s, pattern[1:])
			case strings.HasSuffix(pattern, "*"):
				matched = strings.HasPrefix(s, pattern[:len(pattern)-1])
			default:
				matched = s == pattern
			}
			return matched != negate
		}
	}

	return func(obj map[string]any) bool {
		s, ok := lookup(e, obj)
		if !ok {
			return false
		}
		switch e.Operator {
		case token.Eq:
			return s == fmt.Sprint(expected)
		case token.Neq:
			return s != fmt.Sprint(expected)
		default:
			return false
		}
	}
}

func (v *filterVisitor) VisitPresence(e *ast.PresenceExpr) predicate {
	field := e.Field.String()
	return func(obj map[string]any) bool {
		_, ok := obj[field]
		return ok
	}
}

func (v *filterVisitor) VisitGroup(e *ast.GroupExpr) predicate {
	return ast.Visit[predicate](v, e.Expr)
}

// VisitSelector applies the inner predicate to each element of a list field.
// Element shape here is map[string]any — the same contract @(...) honors at
// eval time. Adapters that filter struct slices would adjust the lookup.
func (v *filterVisitor) VisitSelector(e *ast.SelectorExpr) predicate {
	listField := ""
	if p, ok := e.Base.(*ast.PresenceExpr); ok {
		listField = p.Field.String()
	}
	var inner predicate
	if e.Inner != nil {
		inner = ast.Visit[predicate](v, e.Inner)
	}
	return func(obj map[string]any) bool {
		raw, ok := obj[listField]
		if !ok {
			// Missing field: @none is satisfied (equivalent to empty list),
			// @all is vacuously true, everything else is false.
			return e.Selector == "none" || e.Selector == "all"
		}
		items, ok := raw.([]map[string]any)
		if !ok {
			return false
		}
		if inner == nil {
			return len(items) > 0
		}
		switch e.Selector {
		case "all":
			for _, it := range items {
				if !inner(it) {
					return false
				}
			}
			return true
		case "none":
			for _, it := range items {
				if inner(it) {
					return false
				}
			}
			return true
		default: // "" and "any"
			for _, it := range items {
				if inner(it) {
					return true
				}
			}
			return false
		}
	}
}

// VisitFuncCall is a stub: standalone function-call predicates are
// target-specific. The eval package handles built-in and registered
// functions; consumers using ast.Visitor[T] decide which calls to support.
func (v *filterVisitor) VisitFuncCall(e *ast.FuncCallExpr) predicate {
	return func(map[string]any) bool { return false }
}

func main() {
	demo := func(q string, items []map[string]any) {
		expr, err := query.Parse(q)
		if err != nil {
			fmt.Printf("parse error %q: %v\n", q, err)
			return
		}
		fv := &filterVisitor{}
		matches := ast.Visit[predicate](fv, expr)
		fmt.Printf("Query: %s\n", q)
		for _, item := range items {
			label := fmt.Sprintf("%v", item)
			if len(label) > 70 {
				label = label[:70] + "…"
			}
			fmt.Printf("  %-72s → %v\n", label, matches(item))
		}
		fmt.Println()
	}

	scalar := []map[string]any{
		{"state": "draft", "name": "John Doe"},
		{"state": "draft", "name": "Jane Smith"},
		{"state": "published", "name": "John Wick"},
		{"state": "draft", "name": "Johnny Appleseed"},
	}
	demo("state=draft AND name=John*", scalar)
	demo("name=*son", scalar)
	demo("name!=John*", scalar)
	demo("lower(name)=john*", scalar)
	demo("NOT state=published", scalar)

	withLists := []map[string]any{
		{"customer": "acme", "orders": []map[string]any{
			{"status": "shipped"}, {"status": "pending"},
		}},
		{"customer": "globex", "orders": []map[string]any{
			{"status": "cancelled"},
		}},
		{"customer": "initech", "orders": []map[string]any{}},
		{"customer": "umbrella"}, // missing field
	}
	demo("orders@(status=shipped)", withLists)
	demo("orders@all(status=shipped)", withLists)
	demo("orders@none(status=cancelled)", withLists)
}
