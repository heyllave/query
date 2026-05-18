// Example: JSON AST serialization from a query string.
//
// This demonstrates how to transform a query AST into a JSON tree structure,
// suitable for sending over APIs, storing in databases, or consuming from
// JavaScript/TypeScript frontends.
//
// Run:
//
//	go run ./examples/json "state=draft AND total>50000"
//	go run ./examples/json "(labels.dev=jane OR labels.env=bar) AND NOT cluster=demo"
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/heyllave/query"
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

type jsonNode struct {
	Type     string      `json:"type"`
	Op       string      `json:"op,omitempty"`
	Field    string      `json:"field,omitempty"`
	Value    any         `json:"value,omitempty"`
	EndValue any         `json:"endValue,omitempty"`
	Wildcard bool        `json:"wildcard,omitempty"`
	Left     *jsonNode   `json:"left,omitempty"`
	Right    *jsonNode   `json:"right,omitempty"`
	Expr     *jsonNode   `json:"expr,omitempty"`
	Children []*jsonNode `json:"children,omitempty"`
}

type jsonVisitor struct{}

func (v *jsonVisitor) VisitBinary(e *ast.BinaryExpr) *jsonNode {
	op := "AND"
	if e.Op == token.Or {
		op = "OR"
	}
	return &jsonNode{
		Type:  "binary",
		Op:    op,
		Left:  ast.Visit[*jsonNode](v, e.Left),
		Right: ast.Visit[*jsonNode](v, e.Right),
	}
}

func (v *jsonVisitor) VisitUnary(e *ast.UnaryExpr) *jsonNode {
	return &jsonNode{
		Type: "not",
		Expr: ast.Visit[*jsonNode](v, e.Expr),
	}
}

func (v *jsonVisitor) VisitQualifier(e *ast.QualifierExpr) *jsonNode {
	n := &jsonNode{
		Type:     "qualifier",
		Op:       token.OperatorSymbol(e.Operator),
		Field:    e.Field.String(),
		Value:    e.Value.Any(),
		Wildcard: e.IsWildcard(),
	}
	if e.IsRange() {
		n.Op = ".."
		n.EndValue = e.EndValue.Any()
	}
	return n
}

func (v *jsonVisitor) VisitPresence(e *ast.PresenceExpr) *jsonNode {
	return &jsonNode{Type: "presence", Field: e.Field.String()}
}

func (v *jsonVisitor) VisitGroup(e *ast.GroupExpr) *jsonNode {
	return &jsonNode{
		Type: "group",
		Expr: ast.Visit[*jsonNode](v, e.Expr),
	}
}

func (v *jsonVisitor) VisitSelector(e *ast.SelectorExpr) *jsonNode {
	return ast.Visit[*jsonNode](v, e.Base)
}

func (v *jsonVisitor) VisitFuncCall(e *ast.FuncCallExpr) *jsonNode {
	args := make([]*jsonNode, 0, len(e.Args))
	for _, arg := range e.Args {
		if arg.Call != nil {
			args = append(args, ast.Visit[*jsonNode](v, arg.Call))
		}
	}
	return &jsonNode{Type: "func", Op: e.Name, Children: args}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <query>\n", os.Args[0])
		os.Exit(1)
	}

	expr, err := query.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	jv := &jsonVisitor{}
	node := ast.Visit[*jsonNode](jv, expr)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(node); err != nil {
		fmt.Fprintf(os.Stderr, "json error: %v\n", err)
		os.Exit(1)
	}
}
