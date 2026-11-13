package eval

import (
	"strings"

	"github.com/heyllave/query/ast"
)

// Program is a compiled query ready for evaluation against data.
// It is safe for concurrent use.
type Program struct {
	source  string
	expr    ast.Expression
	fields  []ast.FieldPath
	funcs   FuncRegistry
	matcher matcher
}

// Match evaluates the query against a map of field values.
//
//	prog.Match(map[string]any{"state": "draft", "total": 60000})
//
// A dotted field such as labels.dev is looked up as the flat key "labels.dev"
// first and then as a path through nested maps (data["labels"]["dev"]), so
// both a flattened record and one decoded from nested JSON match.
func (p *Program) Match(data map[string]any) bool {
	return p.matcher(MapAccessor(data))
}

// MapAccessor returns a field accessor over a map that resolves dotted paths
// through nested maps (map[string]any or map[string]string) when the flat key
// is absent. It is the accessor [Program.Match] and [ValueProgram.Eval] use.
func MapAccessor(data map[string]any) func(string) (any, bool) {
	return func(field string) (any, bool) {
		return lookupPath(data, field)
	}
}

// lookupPath resolves field in data: the flat key, or else the dotted path
// descended segment by segment through nested maps.
func lookupPath(data map[string]any, field string) (any, bool) {
	if v, ok := data[field]; ok {
		return v, true
	}
	if !strings.Contains(field, ".") {
		return nil, false
	}
	var cur any = data
	for _, seg := range strings.Split(field, ".") {
		switch m := cur.(type) {
		case map[string]any:
			v, ok := m[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case map[string]string:
			v, ok := m[seg]
			if !ok {
				return nil, false
			}
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}

// MatchFunc evaluates the query using a custom field accessor.
// The accessor returns the value for a field and whether it exists.
//
//	prog.MatchFunc(func(field string) (any, bool) {
//	    return myRecord.Get(field)
//	})
func (p *Program) MatchFunc(get func(field string) (any, bool)) bool {
	return p.matcher(get)
}

// Fields returns all field paths referenced by the query.
func (p *Program) Fields() []ast.FieldPath {
	return p.fields
}

// String returns the original query string.
func (p *Program) String() string {
	return p.source
}

// AST returns the parsed expression tree.
func (p *Program) AST() ast.Expression {
	return p.expr
}

// Stringify returns the AST serialized back to a query string.
func (p *Program) Stringify() string {
	return ast.String(p.expr)
}
