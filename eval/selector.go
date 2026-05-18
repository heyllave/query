package eval

import (
	"reflect"

	"github.com/heyllave/query/ast"
)

// compileSelector handles `items@first`, `items@last`, and `items@(inner)`.
//
// Semantics:
//   - items@first       → the base field is a non-empty slice/array
//   - items@last        → same (distinct AST node for codegen, matcher is identical)
//   - items@(inner)     → at least one element of the slice satisfies inner
//
// Elements may be map[string]any or structs with `query:"..."` tags.
func compileSelector(e *ast.SelectorExpr, funcs FuncRegistry) matcher {
	field, ok := selectorBaseField(e.Base)
	if !ok {
		// Unsupported base (e.g., nested selector or group): fall back to
		// evaluating the base alone so we don't silently succeed.
		return compileMatcher(e.Base, funcs)
	}

	// @first / @last: the slice exists and has at least one element.
	if e.Selector == "first" || e.Selector == "last" {
		return func(get func(string) (any, bool)) bool {
			raw, ok := get(field)
			if !ok {
				return false
			}
			elems, ok := toSlice(raw)
			if !ok {
				return false
			}
			return len(elems) > 0
		}
	}

	// @(inner) / @any(inner) / @all(inner) / @none(inner): per-element predicate.
	if e.Inner == nil {
		return func(func(string) (any, bool)) bool { return false }
	}
	inner := compileMatcher(e.Inner, funcs)

	// Selector mode:
	//   "all"  → every element must satisfy
	//   "none" → no element may satisfy (empty list ⇒ true)
	//   ""     → anonymous @(...) (EXISTS, same as @any)
	//   "any"  → explicit existential
	mode := e.Selector
	return func(get func(string) (any, bool)) bool {
		raw, ok := get(field)
		if !ok {
			return mode == "none" // missing field ≡ empty list for `none`
		}
		elems, ok := toSlice(raw)
		if !ok {
			return false
		}
		switch mode {
		case "all":
			// Vacuous truth: an empty list satisfies "all". Matches SQL's
			// ALL semantics and predicate logic.
			for _, elem := range elems {
				if !inner(elementAccessor(elem)) {
					return false
				}
			}
			return true
		case "none":
			for _, elem := range elems {
				if inner(elementAccessor(elem)) {
					return false
				}
			}
			return true
		default: // "" or "any"
			for _, elem := range elems {
				if inner(elementAccessor(elem)) {
					return true
				}
			}
			return false
		}
	}
}

// selectorBaseField extracts the field path from a selector's base.
// The base is typically a PresenceExpr (e.g. `items` in `items@first`).
func selectorBaseField(e ast.Expression) (string, bool) {
	switch b := e.(type) {
	case *ast.PresenceExpr:
		return b.Field.String(), true
	case *ast.QualifierExpr:
		return b.Field.String(), true
	default:
		return "", false
	}
}

// toSlice converts a reflected slice/array value into []any.
// Returns (nil, false) if v is not a slice or array.
func toSlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	if s, ok := v.([]any); ok {
		return s, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

// elementAccessor returns a field accessor for a single slice element.
//
// map[string]any: direct key lookup.
// Struct: resolves fields via `query:"..."` tags (same contract as StructAccessor).
// Other: accessor always returns (nil, false).
func elementAccessor(elem any) func(string) (any, bool) {
	if m, ok := elem.(map[string]any); ok {
		return func(f string) (any, bool) {
			v, ok := m[f]
			return v, ok
		}
	}
	rv := reflect.ValueOf(elem)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return func(string) (any, bool) { return nil, false }
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return func(string) (any, bool) { return nil, false }
	}
	t := rv.Type()
	tagMap := make(map[string]int, t.NumField())
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("query")
		if tag == "" || tag == "-" {
			continue
		}
		name, _ := parseTag(tag)
		tagMap[name] = i
	}
	return func(f string) (any, bool) {
		idx, ok := tagMap[f]
		if !ok {
			return nil, false
		}
		fv := rv.Field(idx)
		if !fv.IsValid() {
			return nil, false
		}
		return fv.Interface(), true
	}
}
