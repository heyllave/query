package eval

import "github.com/heyllave/query/ast"

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
func (p *Program) Match(data map[string]any) bool {
	return p.matcher(func(field string) (any, bool) {
		v, ok := data[field]
		return v, ok
	})
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
