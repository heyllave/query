package eval

import (
	"reflect"
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
// through nested maps (any map with string keys) when the flat key
// is absent. It is the accessor [Program.Match] and [ValueProgram.Eval] use.
func MapAccessor(data map[string]any) func(string) (any, bool) {
	return func(field string) (any, bool) {
		return lookupPath(data, field)
	}
}

// lookupPath resolves field in data: the flat key, or else the dotted path
// descended through nested maps. A key may itself contain dots
// (labels.example.com/team reads data["labels"]["example.com/team"]), so at
// each level the longest key that exists is tried first.
func lookupPath(data map[string]any, field string) (any, bool) {
	if v, ok := data[field]; ok {
		return v, true
	}
	if !strings.Contains(field, ".") {
		return nil, false
	}
	v, ok, _ := descend(data, strings.Split(field, "."), maxPathProbes)
	return v, ok
}

// maxPathProbes bounds the key lookups one dotted path may cost. A path of n
// segments has 2^(n-1) ways to be split into keys; real records resolve within
// a handful of probes, and a path that needs more is reported missing.
const maxPathProbes = 64

// descend resolves segs against the map cur, joining leading segments into
// one key where that key exists. budget is the number of lookups left, and is
// returned so sibling branches share it.
func descend(cur any, segs []string, budget int) (val any, ok bool, left int) {
	get := mapGetter(cur)
	if get == nil {
		return nil, false, budget
	}
	for n := len(segs); n >= 1; n-- {
		if budget <= 0 {
			return nil, false, 0
		}
		budget--
		v, found := get(strings.Join(segs[:n], "."))
		if !found {
			continue
		}
		if n == len(segs) {
			return v, true, budget
		}
		if val, ok, budget = descend(v, segs[n:], budget); ok {
			return val, true, budget
		}
	}
	return nil, false, budget
}

// mapGetter returns a key lookup over v when v is a map with string keys
// (map[string]any, map[string]string, or any other such map — a named type,
// or the map[any]any a YAML decoder produces), and nil otherwise.
func mapGetter(v any) func(string) (any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return func(k string) (any, bool) { e, ok := m[k]; return e, ok }
	case map[string]string:
		return func(k string) (any, bool) { e, ok := m[k]; return e, ok }
	case nil:
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil
	}
	kt := rv.Type().Key()
	if kt.Kind() != reflect.String && kt.Kind() != reflect.Interface {
		return nil
	}
	return func(k string) (any, bool) {
		key := reflect.ValueOf(k)
		if kt.Kind() == reflect.String {
			key = key.Convert(kt)
		} else if !key.Type().AssignableTo(kt) {
			return nil, false
		}
		e := rv.MapIndex(key)
		if !e.IsValid() {
			return nil, false
		}
		return e.Interface(), true
	}
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
