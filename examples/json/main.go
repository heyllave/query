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
	Selector string      `json:"selector,omitempty"`
	Base     *jsonNode   `json:"base,omitempty"`
	Inner    *jsonNode   `json:"inner,omitempty"`
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
		Value:    valueJSON(&e.Value),
		Wildcard: e.IsWildcard(),
	}
	if e.IsRange() {
		n.Op = ".."
		n.EndValue = valueJSON(e.EndValue)
	}
	return n
}

// valueJSON returns a JSON-friendly representation of a Value. Function- and
// arithmetic-valued expressions (e.g. now() or 50000*1.1 in value position)
// emit a small object so the JSON tree stays self-describing.
func valueJSON(v *ast.Value) any {
	if v == nil {
		return nil
	}
	if v.Type == ast.ValueFunc && v.Func != nil {
		args := make([]any, 0, len(v.Func.Args))
		for _, a := range v.Func.Args {
			switch {
			case a.Field != nil:
				args = append(args, map[string]any{"field": a.Field.String()})
			case a.Value != nil:
				args = append(args, valueJSON(a.Value))
			case a.Call != nil:
				args = append(args, map[string]any{"func": a.Call.Name})
			}
		}
		return map[string]any{"func": v.Func.Name, "args": args}
	}
	if v.Type == ast.ValueArith && v.Arith != nil {
		return map[string]any{
			"arith": v.Arith.Op,
			"left":  valueJSON(v.Arith.Left),
			"right": valueJSON(v.Arith.Right),
		}
	}
	return v.Any()
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
	// Selector codegen is target-specific (SQL EXISTS, JSON path, etc.). The
	// JSON tree surfaces the kind and inner predicate so a consumer can choose
	// how to translate. @first / @last carry no inner expression.
	kind := e.Selector
	if kind == "" {
		kind = "any"
	}
	n := &jsonNode{
		Type:     "selector",
		Selector: kind,
		Base:     ast.Visit[*jsonNode](v, e.Base),
	}
	if e.Inner != nil {
		n.Inner = ast.Visit[*jsonNode](v, e.Inner)
	}
	return n
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
