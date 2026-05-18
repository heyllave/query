//go:build wasm

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// jsonAST is the JSON-serializable representation of an AST node. All fields
// that the parser can produce must round-trip — losing any of them means a
// downstream queryValidate / queryStringify call will return the wrong
// result for queries that use functions, arithmetic, or selectors.
type jsonAST struct {
	Type      string    `json:"type"`
	Op        string    `json:"op,omitempty"`
	Field     []string  `json:"field,omitempty"`
	FieldFunc *jsonFunc `json:"fieldFunc,omitempty"`
	Value     *jsonVal  `json:"value,omitempty"`
	EndValue  *jsonVal  `json:"endValue,omitempty"`
	Selector  string    `json:"selector,omitempty"`
	Name      string    `json:"name,omitempty"`
	Args      []jsonArg `json:"args,omitempty"`
	Left      *jsonAST  `json:"left,omitempty"`
	Right     *jsonAST  `json:"right,omitempty"`
	Expr      *jsonAST  `json:"expr,omitempty"`
	Inner     *jsonAST  `json:"inner,omitempty"`
	Base      *jsonAST  `json:"base,omitempty"`
}

type jsonVal struct {
	Type     string     `json:"type"`
	Raw      string     `json:"raw"`
	Value    any        `json:"value"`
	Wildcard bool       `json:"wildcard,omitempty"`
	Quoted   bool       `json:"quoted,omitempty"`
	Func     *jsonFunc  `json:"func,omitempty"`
	Arith    *jsonArith `json:"arith,omitempty"`
}

// jsonFunc is a serializable [ast.FuncCallExpr]. The Field qualifier on a
// qualifier expression carries one of these when the LHS is a function
// transform (`lower(name)=john`), and a value of type ValueFunc carries one
// when the RHS is a function call (`created_at>=now()`).
type jsonFunc struct {
	Name string    `json:"name"`
	Args []jsonArg `json:"args,omitempty"`
}

// jsonArg mirrors [ast.FuncArg] — exactly one of Field / Value / Call is set.
type jsonArg struct {
	Field []string  `json:"field,omitempty"`
	Value *jsonVal  `json:"value,omitempty"`
	Call  *jsonFunc `json:"call,omitempty"`
}

// jsonArith mirrors [ast.ArithExpr].
type jsonArith struct {
	Op    string   `json:"op"`
	Left  *jsonVal `json:"left"`
	Right *jsonVal `json:"right"`
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
		if e.FieldFunc != nil {
			n.FieldFunc = funcToJSON(e.FieldFunc)
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
	case *ast.FuncCallExpr:
		// Standalone boolean function call (contains(tags, "x") at the top
		// level). Wrap into the AST node shape so consumers can render it.
		return &jsonAST{
			Type: "funccall",
			Name: e.Name,
			Args: argsToJSON(e.Args),
		}
	default:
		return nil
	}
}

func valueToJSON(v *ast.Value) *jsonVal {
	out := &jsonVal{
		Type:     v.Type.String(),
		Raw:      v.Raw,
		Value:    v.Any(),
		Wildcard: v.Wildcard,
		Quoted:   v.Quoted,
	}
	if v.Func != nil {
		out.Func = funcToJSON(v.Func)
	}
	if v.Arith != nil {
		out.Arith = &jsonArith{
			Op:    string(v.Arith.Op),
			Left:  valueToJSON(v.Arith.Left),
			Right: valueToJSON(v.Arith.Right),
		}
	}
	return out
}

func funcToJSON(fc *ast.FuncCallExpr) *jsonFunc {
	if fc == nil {
		return nil
	}
	return &jsonFunc{Name: fc.Name, Args: argsToJSON(fc.Args)}
}

func argsToJSON(args []ast.FuncArg) []jsonArg {
	if len(args) == 0 {
		return nil
	}
	out := make([]jsonArg, len(args))
	for i, a := range args {
		switch {
		case a.Field != nil:
			out[i].Field = []string(*a.Field)
		case a.Value != nil:
			out[i].Value = valueToJSON(a.Value)
		case a.Call != nil:
			out[i].Call = funcToJSON(a.Call)
		}
	}
	return out
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
		if n.FieldFunc != nil {
			q.FieldFunc = jsonToFunc(n.FieldFunc)
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
	case "selector":
		base, err := nodeToAST(n.Base)
		if err != nil {
			return nil, err
		}
		s := &ast.SelectorExpr{
			Base:     base,
			Selector: n.Selector,
		}
		if n.Inner != nil {
			inner, err := nodeToAST(n.Inner)
			if err != nil {
				return nil, err
			}
			s.Inner = inner
		}
		return s, nil
	case "funccall":
		return &ast.FuncCallExpr{
			Name: n.Name,
			Args: jsonToArgs(n.Args),
		}, nil
	default:
		return nil, fmt.Errorf("unknown node type %q", n.Type)
	}
}

func jsonToValue(v *jsonVal) (*ast.Value, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	val := &ast.Value{Raw: v.Raw, Wildcard: v.Wildcard, Quoted: v.Quoted}
	switch v.Type {
	case "string":
		val.Type = ast.ValueString
		// Prefer the deserialized Value over Raw — quoted strings keep their
		// quotes in Raw but the underlying Str is the unquoted form.
		if s, ok := v.Value.(string); ok {
			val.Str = s
		} else {
			val.Str = v.Raw
		}
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
		if d, err := time.ParseDuration(v.Raw); err == nil {
			val.Duration = d
		}
	case "function":
		val.Type = ast.ValueFunc
		val.Func = jsonToFunc(v.Func)
	case "arithmetic":
		val.Type = ast.ValueArith
		if v.Arith != nil {
			left, _ := jsonToValue(v.Arith.Left)
			right, _ := jsonToValue(v.Arith.Right)
			val.Arith = &ast.ArithExpr{
				Op:    ast.ArithOp(v.Arith.Op),
				Left:  left,
				Right: right,
			}
		}
	}
	return val, nil
}

func jsonToFunc(f *jsonFunc) *ast.FuncCallExpr {
	if f == nil {
		return nil
	}
	return &ast.FuncCallExpr{
		Name: f.Name,
		Args: jsonToArgs(f.Args),
	}
}

func jsonToArgs(args []jsonArg) []ast.FuncArg {
	if len(args) == 0 {
		return nil
	}
	out := make([]ast.FuncArg, len(args))
	for i, a := range args {
		switch {
		case a.Field != nil:
			fp := ast.FieldPath(a.Field)
			out[i].Field = &fp
		case a.Value != nil:
			v, _ := jsonToValue(a.Value)
			out[i].Value = v
		case a.Call != nil:
			out[i].Call = jsonToFunc(a.Call)
		}
	}
	return out
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
