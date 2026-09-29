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
	"errors"
	"fmt"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
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
	// Quoted records that a string was written as a "..." literal. A client
	// building a node may omit it: a string that would not read back as itself
	// unquoted is quoted on the way back regardless.
	Quoted bool `json:"quoted,omitempty"`
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
		Quoted:   v.Quoted,
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
	case "selector":
		sel := &ast.SelectorExpr{Selector: n.Selector}
		if n.Base != nil {
			base, err := nodeToAST(n.Base)
			if err != nil {
				return nil, err
			}
			sel.Base = base
		}
		if n.Inner != nil {
			inner, err := nodeToAST(n.Inner)
			if err != nil {
				return nil, err
			}
			sel.Inner = inner
		}
		return sel, nil
	default:
		return nil, fmt.Errorf("unknown node type %q", n.Type)
	}
}

// jsonToValue rebuilds a value from its JSON form by reading its source text
// back through the parser, so the node carries everything the engine would
// have produced for it (a function call, an arithmetic tree, a field ref) and
// printing it writes text that parses to the same value.
//
// A string is the one type whose text is not its source: `raw` is the
// unescaped content. It stays bare only when the parser reads the bare text
// back as this very string (same content, same wildcard flag); anything else —
// a space, a keyword, a number-looking or boolean-looking word, a `*` that is
// meant literally — is quoted.
func jsonToValue(v *Val) (*ast.Value, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	if v.Type == "string" {
		return stringValue(v), nil
	}
	if v.Type == "list" {
		// A list is produced at match time, never by the parser; there is no
		// source text to read back, so it keeps its raw form.
		return &ast.Value{Type: ast.ValueList, Raw: v.Raw}, nil
	}
	parsed := readValue(v.Raw)
	if parsed == nil || !sameValueType(parsed.Type.String(), v.Type) {
		return nil, fmt.Errorf("value %q does not read back as %s", v.Raw, v.Type)
	}
	return parsed, nil
}

// stringValue is the string node for v, quoted unless its bare text reads back
// as the same string.
func stringValue(v *Val) *ast.Value {
	if !v.Quoted {
		if bare := readValue(v.Raw); bare != nil && bare.Type == ast.ValueString &&
			!bare.Quoted && bare.Str == v.Raw && bare.Wildcard == v.Wildcard {
			return bare
		}
	}
	// A quoted literal is never a pattern: a `*` inside quotes is a character.
	return &ast.Value{Type: ast.ValueString, Raw: v.Raw, Str: v.Raw, Quoted: true}
}

// readValue parses text in value position, or returns nil when it does not
// read as exactly one value.
func readValue(text string) *ast.Value {
	if text == "" {
		return nil
	}
	expr, err := parser.Parse("v="+text, len(text)+2)
	if err != nil {
		return nil
	}
	q, ok := expr.(*ast.QualifierExpr)
	if !ok || q.EndValue != nil {
		return nil
	}
	return &q.Value
}

// sameValueType reports whether the parsed type satisfies the declared one.
// The numeric types are one family: a client that declares 100 a float gets
// the integer the text says.
func sameValueType(parsed, declared string) bool {
	if parsed == declared {
		return true
	}
	numeric := func(t string) bool { return t == "integer" || t == "float" }
	return numeric(parsed) && numeric(declared)
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

// ValidationErrors lists each validation failure inside err as
// {code, message, offset, length, field, op}: the stable code a client renders
// its own wording from, the span to underline, and the field and operator the
// failure is about. It is empty when err carries no validation failure.
func ValidationErrors(err error) []map[string]any {
	var list validate.ErrorList
	if !errors.As(err, &list) {
		var single *validate.Error
		if !errors.As(err, &single) {
			return nil
		}
		list = validate.ErrorList{single}
	}
	out := make([]map[string]any, len(list))
	for i, e := range list {
		out[i] = map[string]any{
			"code":    e.Kind.Code(),
			"message": e.Message,
			"offset":  e.Position.Offset,
			"length":  e.Position.Length,
			"field":   e.Field,
			"op":      e.Op,
		}
	}
	return out
}
