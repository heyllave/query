package ast

import "testing"

// allNodeTypes holds one instance of every concrete [Expression] type. It is
// the single checklist behind the dispatch guards below: when you add an
// Expression to node.go you MUST add it here, which then forces you to wire it
// into Visit (visitor.go), Walk (walk.go) and String (string.go) — the guards
// fail until you do. The Expression interface is sealed (the node()/expr()
// markers), so this list is closed and can be complete.
func allNodeTypes() []Expression {
	leaf := &PresenceExpr{Field: FieldPath{"x"}}
	return []Expression{
		&BinaryExpr{Op: 0, Left: leaf, Right: leaf},
		&UnaryExpr{Expr: leaf},
		&QualifierExpr{Field: FieldPath{"a"}},
		&PresenceExpr{Field: FieldPath{"a"}},
		&GroupExpr{Expr: leaf},
		&SelectorExpr{Base: leaf, Selector: SelectorAll, Inner: leaf},
		&FuncCallExpr{Name: "f"},
		&ValueExpr{},
	}
}

// predicateVisitDispatch maps each predicate node to the method name Visit must
// route it to. ValueExpr is deliberately absent: it is the value-domain root and
// is not part of the predicate Visitor (see the Visitor doc), so Visit returns
// the zero value for it. That exclusion is pinned by
// TestVisit_ValueExprIsOutsideThePredicateVisitor.
func predicateVisitDispatch() map[string]string {
	return map[string]string{
		"*ast.BinaryExpr":    "binary",
		"*ast.UnaryExpr":     "unary",
		"*ast.QualifierExpr": "qualifier",
		"*ast.PresenceExpr":  "presence",
		"*ast.GroupExpr":     "group",
		"*ast.SelectorExpr":  "selector",
		"*ast.FuncCallExpr":  "funcCall",
	}
}

// nameVisitor returns, for each node, the name of the method that handled it.
// An unhandled node reaches Visit's default and yields "".
type nameVisitor struct{}

func (nameVisitor) VisitBinary(*BinaryExpr) string       { return "binary" }
func (nameVisitor) VisitUnary(*UnaryExpr) string         { return "unary" }
func (nameVisitor) VisitQualifier(*QualifierExpr) string { return "qualifier" }
func (nameVisitor) VisitPresence(*PresenceExpr) string   { return "presence" }
func (nameVisitor) VisitGroup(*GroupExpr) string         { return "group" }
func (nameVisitor) VisitSelector(*SelectorExpr) string   { return "selector" }
func (nameVisitor) VisitFuncCall(*FuncCallExpr) string   { return "funcCall" }

func typeName(e Expression) string {
	switch e.(type) {
	case *BinaryExpr:
		return "*ast.BinaryExpr"
	case *UnaryExpr:
		return "*ast.UnaryExpr"
	case *QualifierExpr:
		return "*ast.QualifierExpr"
	case *PresenceExpr:
		return "*ast.PresenceExpr"
	case *GroupExpr:
		return "*ast.GroupExpr"
	case *SelectorExpr:
		return "*ast.SelectorExpr"
	case *FuncCallExpr:
		return "*ast.FuncCallExpr"
	case *ValueExpr:
		return "*ast.ValueExpr"
	default:
		return "<unknown>"
	}
}

// TestVisit_DispatchesEveryPredicateNode fails if a predicate node reaches the
// silent default branch of Visit — the trap a newly added node would fall into
// if its case were forgotten.
func TestVisit_DispatchesEveryPredicateNode(t *testing.T) {
	want := predicateVisitDispatch()
	for _, node := range allNodeTypes() {
		name := typeName(node)
		if _, isPredicate := want[name]; !isPredicate {
			continue // ValueExpr; checked separately
		}
		if got := Visit[string](nameVisitor{}, node); got != want[name] {
			t.Errorf("Visit(%s) = %q, want %q (did a new node miss its case in visitor.go?)", name, got, want[name])
		}
	}
}

// TestVisit_ValueExprIsOutsideThePredicateVisitor pins the one deliberate gap:
// the value-domain root is not part of the predicate Visitor, so Visit yields
// the zero value. If the value domain ever gets its own visitor, this is where
// the decision changes.
func TestVisit_ValueExprIsOutsideThePredicateVisitor(t *testing.T) {
	if got := Visit[string](nameVisitor{}, &ValueExpr{}); got != "" {
		t.Errorf("Visit(*ast.ValueExpr) = %q, want \"\" (the predicate Visitor does not cover the value domain)", got)
	}
}

// TestWalk_VisitsEveryNodeType fails if Walk does not reach a node type —
// including ValueExpr, which Walk does cover even though the predicate Visitor
// does not. A new node added to node.go and allNodeTypes but not to Walk's
// switch is caught here.
func TestWalk_VisitsEveryNodeType(t *testing.T) {
	for _, node := range allNodeTypes() {
		name := typeName(node)
		var seen bool
		Walk(node, func(e Expression) bool {
			if typeName(e) == name {
				seen = true
			}
			return true
		})
		if !seen {
			t.Errorf("Walk never visited %s (missing case in walk.go?)", name)
		}
	}
}

// TestString_HandlesEveryNodeType fails if String panics or returns empty for a
// node type, so the AST-to-text round trip stays total as nodes are added.
func TestString_HandlesEveryNodeType(t *testing.T) {
	for _, node := range allNodeTypes() {
		name := typeName(node)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("String(%s) panicked: %v", name, r)
				}
			}()
			if got := String(node); got == "" {
				t.Errorf("String(%s) = \"\" (missing case in string.go?)", name)
			}
		}()
	}
}
