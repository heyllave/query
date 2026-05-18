//go:build wasm

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// jsonAST is the JSON-serializable representation of an AST node.
type jsonAST struct {
	Type     string   `json:"type"`
	Op       string   `json:"op,omitempty"`
	Field    []string `json:"field,omitempty"`
	Value    *jsonVal `json:"value,omitempty"`
	EndValue *jsonVal `json:"endValue,omitempty"`
	Selector string   `json:"selector,omitempty"`
	Left     *jsonAST `json:"left,omitempty"`
	Right    *jsonAST `json:"right,omitempty"`
	Expr     *jsonAST `json:"expr,omitempty"`
	Inner    *jsonAST `json:"inner,omitempty"`
	Base     *jsonAST `json:"base,omitempty"`
}

type jsonVal struct {
	Type     string `json:"type"`
	Raw      string `json:"raw"`
	Value    any    `json:"value"`
	Wildcard bool   `json:"wildcard,omitempty"`
}

// astToJSON converts an ast.Expression into a JSON-serializable structure.
func astToJSON(expr ast.Expression) *jsonAST {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		op := "AND"
		if e.Op == token.Or {
			op = "OR"
		}
		return &jsonAST{
			Type:  "binary",
			Op:    op,
			Left:  astToJSON(e.Left),
			Right: astToJSON(e.Right),
		}
	case *ast.UnaryExpr:
		return &jsonAST{
			Type: "not",
			Expr: astToJSON(e.Expr),
		}
	case *ast.QualifierExpr:
		n := &jsonAST{
			Type:  "qualifier",
			Op:    token.OperatorSymbol(e.Operator),
			Field: []string(e.Field),
			Value: valueToJSON(&e.Value),
		}
		if e.EndValue != nil {
			n.EndValue = valueToJSON(e.EndValue)
		}
		return n
	case *ast.PresenceExpr:
		return &jsonAST{
			Type:  "presence",
			Field: []string(e.Field),
		}
	case *ast.GroupExpr:
		return &jsonAST{
			Type: "group",
			Expr: astToJSON(e.Expr),
		}
	case *ast.SelectorExpr:
		return &jsonAST{
			Type:     "selector",
			Selector: e.Selector,
			Base:     astToJSON(e.Base),
			Inner:    astToJSON(e.Inner),
		}
	default:
		return nil
	}
}

func valueToJSON(v *ast.Value) *jsonVal {
	return &jsonVal{
		Type:     v.Type.String(),
		Raw:      v.Raw,
		Value:    v.Any(),
		Wildcard: v.Wildcard,
	}
}

// jsonToAST converts a JSON string back into an ast.Expression.
func jsonToAST(data string) (ast.Expression, error) {
	var node jsonAST
	if err := json.Unmarshal([]byte(data), &node); err != nil {
		return nil, err
	}
	return nodeToAST(&node)
}

func nodeToAST(n *jsonAST) (ast.Expression, error) {
	if n == nil {
		return nil, fmt.Errorf("nil node")
	}
	switch n.Type {
	case "binary":
		op := token.And
		if n.Op == "OR" {
			op = token.Or
		}
		left, err := nodeToAST(n.Left)
		if err != nil {
			return nil, err
		}
		right, err := nodeToAST(n.Right)
		if err != nil {
			return nil, err
		}
		return &ast.BinaryExpr{Op: op, Left: left, Right: right}, nil
	case "not":
		inner, err := nodeToAST(n.Expr)
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: token.Not, Expr: inner}, nil
	case "qualifier":
		val, err := jsonToValue(n.Value)
		if err != nil {
			return nil, err
		}
		q := &ast.QualifierExpr{
			Field:    ast.FieldPath(n.Field),
			Operator: symbolToToken(n.Op),
			Value:    *val,
		}
		if n.EndValue != nil {
			ev, err := jsonToValue(n.EndValue)
			if err != nil {
				return nil, err
			}
			q.EndValue = ev
			q.Operator = token.Range
		}
		return q, nil
	case "presence":
		return &ast.PresenceExpr{Field: ast.FieldPath(n.Field)}, nil
	case "group":
		inner, err := nodeToAST(n.Expr)
		if err != nil {
			return nil, err
		}
		return &ast.GroupExpr{Expr: inner}, nil
	default:
		return nil, fmt.Errorf("unknown node type %q", n.Type)
	}
}

func jsonToValue(v *jsonVal) (*ast.Value, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	val := &ast.Value{Raw: v.Raw, Wildcard: v.Wildcard}
	switch v.Type {
	case "string":
		val.Type = ast.ValueString
		val.Str = v.Raw
	case "integer":
		val.Type = ast.ValueInteger
		if f, ok := v.Value.(float64); ok {
			val.Int = int64(f)
		}
	case "float":
		val.Type = ast.ValueFloat
		if f, ok := v.Value.(float64); ok {
			val.Float = f
		}
	case "boolean":
		val.Type = ast.ValueBoolean
		if b, ok := v.Value.(bool); ok {
			val.Bool = b
		}
	case "date":
		val.Type = ast.ValueDate
		if d, err := time.Parse("2006-01-02", v.Raw); err == nil {
			val.Date = d
		}
	case "duration":
		val.Type = ast.ValueDuration
	}
	return val, nil
}

func symbolToToken(op string) token.Type {
	switch op {
	case "=":
		return token.Eq
	case "!=":
		return token.Neq
	case ">":
		return token.Gt
	case ">=":
		return token.Gte
	case "<":
		return token.Lt
	case "<=":
		return token.Lte
	case "..":
		return token.Range
	default:
		return token.Eq
	}
}
