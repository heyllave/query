// Package bridgejson is the shared JSON contract for the query language's
// foreign-language bridges (WASM/JS and cgo/FFI). It converts between the AST
// and a stable JSON representation and decodes field configs, so every bridge
// produces and accepts identical shapes — the single source of truth that keeps
// the JavaScript and Dart clients from drifting apart.
//
// It carries no build tag and depends only on the pure-Go library packages, so
// it compiles in every target (host, WASM, cgo).
package bridgejson

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
	"github.com/heyllave/query/validate"
)

// AST is the JSON-serializable representation of an AST node.
type AST struct {
	Type     string   `json:"type"`
	Op       string   `json:"op,omitempty"`
	Field    []string `json:"field,omitempty"`
	Value    *Val     `json:"value,omitempty"`
	EndValue *Val     `json:"endValue,omitempty"`
	Selector string   `json:"selector,omitempty"`
	Left     *AST     `json:"left,omitempty"`
	Right    *AST     `json:"right,omitempty"`
	Expr     *AST     `json:"expr,omitempty"`
	Inner    *AST     `json:"inner,omitempty"`
	Base     *AST     `json:"base,omitempty"`
}

// Val is the JSON-serializable representation of a value.
type Val struct {
	Type     string `json:"type"`
	Raw      string `json:"raw"`
	Value    any    `json:"value"`
	Wildcard bool   `json:"wildcard,omitempty"`
}

// AstToJSON converts an [ast.Expression] into a JSON-serializable structure.
func AstToJSON(expr ast.Expression) *AST {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		op := "AND"
		if e.Op == token.Or {
			op = "OR"
		}
		return &AST{
			Type:  "binary",
			Op:    op,
			Left:  AstToJSON(e.Left),
			Right: AstToJSON(e.Right),
		}
	case *ast.UnaryExpr:
		return &AST{
			Type: "not",
			Expr: AstToJSON(e.Expr),
		}
	case *ast.QualifierExpr:
		n := &AST{
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
		return &AST{
			Type:  "presence",
			Field: []string(e.Field),
		}
	case *ast.GroupExpr:
		return &AST{
			Type: "group",
			Expr: AstToJSON(e.Expr),
		}
	case *ast.SelectorExpr:
		return &AST{
			Type:     "selector",
			Selector: e.Selector,
			Base:     AstToJSON(e.Base),
			Inner:    AstToJSON(e.Inner),
		}
	default:
		return nil
	}
}

func valueToJSON(v *ast.Value) *Val {
	return &Val{
		Type:     v.Type.String(),
		Raw:      v.Raw,
		Value:    v.Any(),
		Wildcard: v.Wildcard,
	}
}

// JSONToAST converts a JSON string back into an [ast.Expression].
func JSONToAST(data string) (ast.Expression, error) {
	var node AST
	if err := json.Unmarshal([]byte(data), &node); err != nil {
		return nil, err
	}
	return nodeToAST(&node)
}

func nodeToAST(n *AST) (ast.Expression, error) {
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

func jsonToValue(v *Val) (*ast.Value, error) {
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

// ParseFields decodes a field-config JSON array, shared by the bridges so the
// "invalid fields config" error wording stays identical across clients.
func ParseFields(fieldsJSON string) ([]validate.FieldConfig, error) {
	var fields []validate.FieldConfig
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return nil, fmt.Errorf("invalid fields config: %w", err)
	}
	return fields, nil
}
