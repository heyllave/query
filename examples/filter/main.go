// Example: In-memory filter function generation from a query string.
//
// This demonstrates how to use ast.Visitor[T] to build composable Go predicate
// functions for filtering in-memory data. Useful for CLI tools, WASM clients,
// or any context where you filter objects without a database.
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

type filterVisitor struct{}

func (v *filterVisitor) VisitBinary(e *ast.BinaryExpr) func(map[string]any) bool {
	left := ast.Visit[func(map[string]any) bool](v, e.Left)
	right := ast.Visit[func(map[string]any) bool](v, e.Right)
	if e.Op == token.And {
		return func(obj map[string]any) bool { return left(obj) && right(obj) }
	}
	return func(obj map[string]any) bool { return left(obj) || right(obj) }
}

func (v *filterVisitor) VisitUnary(e *ast.UnaryExpr) func(map[string]any) bool {
	inner := ast.Visit[func(map[string]any) bool](v, e.Expr)
	return func(obj map[string]any) bool { return !inner(obj) }
}

func (v *filterVisitor) VisitQualifier(e *ast.QualifierExpr) func(map[string]any) bool {
	field := e.Field.String()
	expected := e.Value.Any()

	if e.IsWildcard() {
		pattern := e.Value.Str
		return func(obj map[string]any) bool {
			val, ok := obj[field]
			if !ok {
				return false
			}
			s := fmt.Sprint(val)
			if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
				return strings.Contains(s, pattern[1:len(pattern)-1])
			}
			if strings.HasPrefix(pattern, "*") {
				return strings.HasSuffix(s, pattern[1:])
			}
			if strings.HasSuffix(pattern, "*") {
				return strings.HasPrefix(s, pattern[:len(pattern)-1])
			}
			return s == pattern
		}
	}

	return func(obj map[string]any) bool {
		val, ok := obj[field]
		if !ok {
			return false
		}
		switch e.Operator {
		case token.Eq:
			return fmt.Sprint(val) == fmt.Sprint(expected)
		case token.Neq:
			return fmt.Sprint(val) != fmt.Sprint(expected)
		default:
			return false
		}
	}
}

func (v *filterVisitor) VisitPresence(e *ast.PresenceExpr) func(map[string]any) bool {
	field := e.Field.String()
	return func(obj map[string]any) bool {
		_, ok := obj[field]
		return ok
	}
}

func (v *filterVisitor) VisitGroup(e *ast.GroupExpr) func(map[string]any) bool {
	return ast.Visit[func(map[string]any) bool](v, e.Expr)
}

func (v *filterVisitor) VisitSelector(e *ast.SelectorExpr) func(map[string]any) bool {
	return ast.Visit[func(map[string]any) bool](v, e.Base)
}

func (v *filterVisitor) VisitFuncCall(e *ast.FuncCallExpr) func(map[string]any) bool {
	return func(map[string]any) bool { return false }
}

func main() {
	q := "state=draft AND name=John*"
	expr, err := query.Parse(q)
	if err != nil {
		panic(err)
	}

	fv := &filterVisitor{}
	matches := ast.Visit[func(map[string]any) bool](fv, expr)

	items := []map[string]any{
		{"state": "draft", "name": "John Doe"},
		{"state": "draft", "name": "Jane Smith"},
		{"state": "published", "name": "John Wick"},
		{"state": "draft", "name": "Johnny Appleseed"},
	}

	fmt.Printf("Query: %s\n\n", q)
	for _, item := range items {
		fmt.Printf("  %-12s %-20s → %v\n", item["state"], item["name"], matches(item))
	}
}
