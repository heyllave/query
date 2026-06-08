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

// Fields returns all unique field paths referenced in the expression.
func Fields(expr Expression) []FieldPath {
	seen := make(map[string]bool)
	var result []FieldPath
	Walk(expr, func(e Expression) bool {
		var fp FieldPath
		switch n := e.(type) {
		case *QualifierExpr:
			fp = n.Field
		case *PresenceExpr:
			fp = n.Field
		default:
			return true
		}
		key := fp.String()
		if !seen[key] {
			seen[key] = true
			result = append(result, fp)
		}
		return true
	})
	return result
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
