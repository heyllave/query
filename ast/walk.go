package ast

// Walk traverses the AST depth-first, calling fn for each node.
// If fn returns false, children of that node are not visited.
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
		}
	case *QualifierExpr, *PresenceExpr, *ValueExpr:
		// leaf nodes — value-position function/arithmetic subtrees live in the
		// Value layer, not as Expression children.
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
			add(n.Field)
			if n.FieldFunc != nil {
				collectFuncFields(n.FieldFunc, add)
			}
			collectValueFields(&n.Value, add)
			collectValueFields(n.EndValue, add)
		case *PresenceExpr:
			add(n.Field)
		case *ValueExpr:
			collectValueFields(&n.Value, add)
		case *FuncCallExpr:
			collectFuncFields(n, add)
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
		if v.Func != nil {
			collectFuncFields(v.Func, add)
		}
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
			collectFuncFields(arg.Call, add)
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

// Depth returns the maximum nesting depth of the expression tree.
func Depth(expr Expression) int {
	if expr == nil {
		return 0
	}
	switch e := expr.(type) {
	case *BinaryExpr:
		ld := Depth(e.Left)
		rd := Depth(e.Right)
		if ld > rd {
			return ld + 1
		}
		return rd + 1
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
