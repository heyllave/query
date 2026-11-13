package ast

import "github.com/heyllave/query/token"

// Walk traverses the AST depth-first, calling fn for each node.
// If fn returns false, children of that node are not visited.
//
// Function calls are visited wherever they occur: as standalone predicates, as
// a qualifier's field transform (lower(name)=x), in value position
// (created_at>=now()), inside arithmetic (total>[base]*rate()), and as
// arguments of other calls. A walk that counts *FuncCallExpr nodes therefore
// sees every call in the query.
func Walk(expr Expression, fn func(Expression) bool) {
	if expr == nil || !fn(expr) {
		return
	}
	switch e := expr.(type) {
	case *BinaryExpr:
		Walk(e.Left, fn)
		Walk(e.Right, fn)
	case *UnaryExpr:
		Walk(e.Expr, fn)
	case *GroupExpr:
		Walk(e.Expr, fn)
	case *SelectorExpr:
		Walk(e.Base, fn)
		if e.Inner != nil {
			Walk(e.Inner, fn)
		}
	case *FuncCallExpr:
		for _, arg := range e.Args {
			if arg.Call != nil {
				Walk(arg.Call, fn)
			}
			if arg.Value != nil {
				walkValue(arg.Value, fn)
			}
		}
	case *QualifierExpr:
		if e.FieldFunc != nil {
			Walk(e.FieldFunc, fn)
		}
		walkValue(&e.Value, fn)
		walkValue(e.EndValue, fn)
	case *ValueExpr:
		walkValue(&e.Value, fn)
	case *PresenceExpr:
		// leaf
	}
}

// walkValue visits the function calls reachable inside a Value: a
// function-valued operand and the operands of an arithmetic subtree.
func walkValue(v *Value, fn func(Expression) bool) {
	if v == nil {
		return
	}
	switch v.Type { //nolint:exhaustive // only composite value types hold calls
	case ValueFunc:
		if v.Func != nil {
			Walk(v.Func, fn)
		}
	case ValueArith:
		if v.Arith != nil {
			walkValue(v.Arith.Left, fn)
			walkValue(v.Arith.Right, fn)
		}
	}
}

// Fields returns all unique field paths referenced in the expression. This
// includes the comparison/presence field of each node and any fields referenced
// in value position — bracketed field-ref operands ([cpu_limit]) and field
// arguments to functions — so a query like cpu>[cpu_limit] reports both cpu and
// cpu_limit. Walk itself stays leaf-only over Expression nodes because the value
// subtrees are not Expressions; Fields descends into the Value layer explicitly.
func Fields(expr Expression) []FieldPath {
	seen := make(map[string]bool)
	var result []FieldPath
	add := func(fp FieldPath) {
		key := fp.String()
		if !seen[key] {
			seen[key] = true
			result = append(result, fp)
		}
	}
	Walk(expr, func(e Expression) bool {
		switch n := e.(type) {
		case *QualifierExpr:
			// A function-wrapped qualifier has no field of its own; the walk
			// reaches its FieldFunc and value calls as nodes, and
			// collectFuncFields below gathers their field arguments.
			if n.FieldFunc == nil && len(n.Field) > 0 {
				add(n.Field)
			}
			collectValueFields(&n.Value, add)
			collectValueFields(n.EndValue, add)
		case *PresenceExpr:
			add(n.Field)
		case *FuncCallExpr:
			collectFuncFields(n, add)
		}
		return true
	})
	return result
}

// Calls returns every function call in the expression, in walk order,
// wherever it occurs (predicate, field transform, value position, argument).
// A call node the tree holds more than once is returned once: the qualifiers
// that `lower(name) IN (a, b)` lowers to share their left-hand call.
func Calls(expr Expression) []*FuncCallExpr {
	var result []*FuncCallExpr
	seen := make(map[*FuncCallExpr]bool)
	Walk(expr, func(e Expression) bool {
		if fc, ok := e.(*FuncCallExpr); ok && !seen[fc] {
			seen[fc] = true
			result = append(result, fc)
		}
		return true
	})
	return result
}

// collectValueFields surfaces field references reachable inside a Value: a
// bracketed field-ref operand, the operands of an arithmetic subtree, and the
// arguments of a function-valued operand.
func collectValueFields(v *Value, add func(FieldPath)) {
	if v == nil {
		return
	}
	switch v.Type { //nolint:exhaustive // only composite value types carry nested field refs
	case ValueFieldRef:
		add(v.Field)
	case ValueArith:
		if v.Arith != nil {
			collectValueFields(v.Arith.Left, add)
			collectValueFields(v.Arith.Right, add)
		}
	case ValueFunc:
		// Function calls are reached by Walk as nodes; their arguments are
		// collected there.
	}
}

// collectFuncFields surfaces field references in a function call's arguments,
// recursing into nested calls.
func collectFuncFields(fc *FuncCallExpr, add func(FieldPath)) {
	for _, arg := range fc.Args {
		switch {
		case arg.Field != nil:
			add(*arg.Field)
		case arg.Value != nil:
			collectValueFields(arg.Value, add)
		case arg.Call != nil:
			// Nested calls are visited by Walk as nodes of their own.
		}
	}
}

// Qualifiers returns all qualifier expressions in the AST.
func Qualifiers(expr Expression) []*QualifierExpr {
	var result []*QualifierExpr
	Walk(expr, func(e Expression) bool {
		if q, ok := e.(*QualifierExpr); ok {
			result = append(result, q)
		}
		return true
	})
	return result
}

// IsSimple reports whether the expression is a single qualifier or presence
// check with no logical operators.
func IsSimple(expr Expression) bool {
	switch expr.(type) {
	case *QualifierExpr, *PresenceExpr:
		return true
	default:
		return false
	}
}

// Depth returns the nesting depth of the expression tree. A run of the same
// connective counts as one level, so `a AND b AND c AND d` and the OR chain
// an IN list lowers to both have depth 2 regardless of length: depth measures
// how deeply groups, negations, selectors and alternating connectives nest,
// not how many terms a query has.
func Depth(expr Expression) int {
	if expr == nil {
		return 0
	}
	switch e := expr.(type) {
	case *BinaryExpr:
		return chainDepth(e, e.Op) + 1
	case *UnaryExpr:
		return Depth(e.Expr) + 1
	case *GroupExpr:
		return Depth(e.Expr) + 1
	case *SelectorExpr:
		d := Depth(e.Base)
		if e.Inner != nil {
			if id := Depth(e.Inner); id > d {
				d = id
			}
		}
		return d + 1
	default:
		return 1
	}
}

// chainDepth returns the deepest operand of a run of binary nodes sharing op,
// flattening left- and right-nested chains alike.
func chainDepth(e *BinaryExpr, op token.Type) int {
	d := 0
	for _, side := range []Expression{e.Left, e.Right} {
		var sd int
		if b, ok := side.(*BinaryExpr); ok && b.Op == op {
			sd = chainDepth(b, op)
		} else {
			sd = Depth(side)
		}
		if sd > d {
			d = sd
		}
	}
	return d
}
