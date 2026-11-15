// Package bridgejson is the shared JSON contract for the query language's
// foreign-language bridges: WASM for JavaScript and a C shared library for
// everything else. It converts between the AST and a stable JSON
// representation, decodes field configs and implements each bridge call
// (api.go), so every bridge produces and accepts identical shapes and a client
// in one language cannot drift from a client in another.
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
	// FieldFunc is the source form of the function a qualifier compares
	// (lower(name) in lower(name)=john); Field is empty for such a node.
	FieldFunc string `json:"fieldFunc,omitempty"`
	Left      *AST   `json:"left,omitempty"`
	Right     *AST   `json:"right,omitempty"`
	Expr      *AST   `json:"expr,omitempty"`
	Inner     *AST   `json:"inner,omitempty"`
	Base      *AST   `json:"base,omitempty"`
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
		if e.FieldFunc != nil {
			n.FieldFunc = ast.String(e.FieldFunc)
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

// maxASTJSONLength bounds the JSON form of a tree. A tree is hundreds of times
// the size of the query it was read from (a short `f IN (...)` becomes one
// qualifier node per element, and indentation multiplies that again), so the
// bound is far above the query limit: it leaves room for any tree the parser
// builds under [DefaultMaxLength] and still caps what one call can cost.
const maxASTJSONLength = 1 << 20

// tooLong is the parser's length-limit failure for a part of a JSON tree, so a
// client sees the same code whichever way an oversized query arrives.
func tooLong(what string, length, limit int) error {
	return parser.ErrorList{{
		Message: fmt.Sprintf("%s length %d exceeds maximum of %d characters", what, length, limit),
		Code:    parser.CodeQueryTooLong,
		Kind:    parser.ErrQueryTooLong,
	}}
}

// checkLength holds source text carried inside a tree to the query limit: it
// is read back through the parser or printed as part of a query, and neither
// may cost more than a query itself could.
func checkLength(what, text string) error {
	if len(text) > DefaultMaxLength {
		return tooLong(what, len(text), DefaultMaxLength)
	}
	return nil
}

// JSONToAST converts a JSON string back into an [ast.Expression]. The tree is
// untrusted: every part of it that [ast.String] prints as written is held to
// what the parser could have produced, so a tree that validates cannot print
// as a different query.
func JSONToAST(data string) (ast.Expression, error) {
	if len(data) > maxASTJSONLength {
		return nil, tooLong("AST JSON", len(data), maxASTJSONLength)
	}
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
		var op token.Type
		switch n.Op {
		case "AND":
			op = token.And
		case "OR":
			op = token.Or
		default:
			return nil, fmt.Errorf("unknown logical operator %q", n.Op)
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
		op, ok := symbolToToken(n.Op)
		if !ok {
			return nil, fmt.Errorf("unknown operator %q", n.Op)
		}
		if op == token.Range && n.EndValue == nil {
			return nil, fmt.Errorf("operator %q needs an endValue", n.Op)
		}
		if err := checkFieldLength(n.Field); err != nil {
			return nil, err
		}
		val, err := jsonToValue(n.Value)
		if err != nil {
			return nil, err
		}
		q := &ast.QualifierExpr{
			Field:    ast.FieldPath(n.Field),
			Operator: op,
			Value:    *val,
		}
		if n.FieldFunc != "" {
			// The compared function is carried as its source text; read it
			// back through the parser as a standalone call.
			expr, err := parser.Parse(n.FieldFunc, DefaultMaxLength)
			if err != nil {
				return nil, fmt.Errorf("fieldFunc %q: %w", n.FieldFunc, err)
			}
			fc, ok := expr.(*ast.FuncCallExpr)
			if !ok {
				return nil, fmt.Errorf("fieldFunc %q is not a function call", n.FieldFunc)
			}
			q.FieldFunc = fc
			q.Field = nil
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
		if err := checkFieldLength(n.Field); err != nil {
			return nil, err
		}
		return &ast.PresenceExpr{Field: ast.FieldPath(n.Field)}, nil
	case "group":
		inner, err := nodeToAST(n.Expr)
		if err != nil {
			return nil, err
		}
		return &ast.GroupExpr{Expr: inner}, nil
	case "selector":
		// The selector name is printed as written, so it is one of the names
		// the parser reads, in the shape it reads them: @first and @last stand
		// alone, the others wrap an expression.
		switch n.Selector {
		case "first", "last":
			if n.Inner != nil {
				return nil, fmt.Errorf("selector %q takes no inner expression", n.Selector)
			}
		case "", "any", "all", "none":
			if n.Inner == nil {
				return nil, fmt.Errorf("selector %q needs an inner expression", n.Selector)
			}
		default:
			return nil, fmt.Errorf("unknown selector %q", n.Selector)
		}
		// A selector binds to the single term before the '@'. A base of any
		// other shape (a bare AND/OR/NOT, or none) would print as a query that
		// groups differently from this tree, so it is refused rather than
		// printed.
		if n.Base == nil {
			return nil, fmt.Errorf("selector needs a base expression")
		}
		switch n.Base.Type {
		case "qualifier", "presence", "group", "selector":
		default:
			return nil, fmt.Errorf("selector base cannot be a %q node; wrap it in a group", n.Base.Type)
		}
		sel := &ast.SelectorExpr{Selector: n.Selector}
		base, err := nodeToAST(n.Base)
		if err != nil {
			return nil, err
		}
		sel.Base = base
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
	if err := checkLength("value", v.Raw); err != nil {
		return nil, err
	}
	if v.Type == "string" {
		if s, ok := v.Value.(string); ok {
			if err := checkLength("value", s); err != nil {
				return nil, err
			}
		}
		return stringValue(v), nil
	}
	if v.Type == "list" {
		// A list is produced at match time, never by the parser: it has no
		// source text to read back, and its raw form would print unchecked.
		return nil, fmt.Errorf("a list value cannot be written in a query")
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
	// `value` is the decoded content; `raw` may be its escaped source form
	// (a\(b), which is what the bare text must read back to.
	content := v.Raw
	if s, ok := v.Value.(string); ok {
		content = s
	}
	if !v.Quoted {
		if bare := readValue(v.Raw); bare != nil && bare.Type == ast.ValueString &&
			!bare.Quoted && bare.Str == content && bare.Wildcard == v.Wildcard {
			return bare
		}
	}
	// A quoted literal is never a pattern: a `*` inside quotes is a character.
	return &ast.Value{Type: ast.ValueString, Raw: content, Str: content, Quoted: true}
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

// checkFieldLength holds a field path to the query limit.
func checkFieldLength(field []string) error {
	n := 0
	for _, seg := range field {
		n += len(seg) + 1
	}
	if n > DefaultMaxLength {
		return tooLong("field", n, DefaultMaxLength)
	}
	return nil
}

// symbolToToken is the comparison operator a symbol names; ok is false for
// text that is not one.
func symbolToToken(op string) (tok token.Type, ok bool) {
	switch op {
	case "=":
		return token.Eq, true
	case "!=":
		return token.Neq, true
	case ">":
		return token.Gt, true
	case ">=":
		return token.Gte, true
	case "<":
		return token.Lt, true
	case "<=":
		return token.Lte, true
	case "..":
		return token.Range, true
	default:
		return token.Eq, false
	}
}

// ParseFields decodes a field-config JSON array, shared by the bridges so
// every client is held to the same rules. A field's type is its name
// ("decimal") or its number; a field that lists no operators gets the defaults
// for its type. A config that could only misbehave silently — a nameless or
// repeated field, an operator the language does not have — is an error.
func ParseFields(fieldsJSON string) ([]validate.FieldConfig, error) {
	var fields []validate.FieldConfig
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return nil, fmt.Errorf("invalid fields config: %w", err)
	}
	seen := make(map[string]bool, len(fields))
	for i := range fields {
		f := &fields[i]
		if f.Name == "" {
			return nil, fmt.Errorf("invalid fields config: field %d has no name", i)
		}
		if seen[f.Name] {
			return nil, fmt.Errorf("invalid fields config: field %q is declared twice", f.Name)
		}
		seen[f.Name] = true
		if f.AllowedOps == nil {
			f.AllowedOps = validate.DefaultOps(f.Type)
		}
		for _, op := range f.AllowedOps {
			if !knownOp(op) {
				return nil, fmt.Errorf("invalid fields config: field %q lists unknown operator %q", f.Name, op)
			}
		}
	}
	return fields, nil
}

func knownOp(op validate.Op) bool {
	switch op {
	case validate.OpEq, validate.OpNeq, validate.OpGt, validate.OpGte, validate.OpLt,
		validate.OpLte, validate.OpRange, validate.OpWildcard, validate.OpPresence:
		return true
	}
	return false
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
