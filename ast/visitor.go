package ast

// Visitor defines typed callbacks for each AST node. Consumers implement this
// interface to transform an AST into any target representation (SQL, JSON,
// filter functions, React components, etc.).
//
// Each method receives the concrete node and returns a result of type T.
// Use [Visit] to dispatch an [Expression] to the correct method.
//
// # Scope
//
// Visitor covers the boolean/predicate domain — the tree [Parse] produces. It
// deliberately does not include [ValueExpr], the value-domain root that
// [github.com/heyllave/query/parser.ParseValue] produces (now()-7d, (1+2)*3);
// evaluate those with the eval package. [Visit] returns the zero T for a
// ValueExpr.
//
// # Growth contract
//
// The [Expression] set is sealed (the unexported node()/expr() markers), so only
// this package adds node types and this interface stays complete. Adding a
// predicate node is therefore a breaking, minor-version change: a new method
// here, a case in [Visit], [Walk] and [String], and an entry in the ast
// dispatch-guard test. Every implementer then gets a compile error until they
// handle the new construct — which is the point, because a SQL or filter
// generator that silently ignored a new operator would be a correctness bug.
type Visitor[T any] interface {
	VisitBinary(expr *BinaryExpr) T
	VisitUnary(expr *UnaryExpr) T
	VisitQualifier(expr *QualifierExpr) T
	VisitPresence(expr *PresenceExpr) T
	VisitGroup(expr *GroupExpr) T
	VisitSelector(expr *SelectorExpr) T
	VisitFuncCall(expr *FuncCallExpr) T
}

// Visit dispatches an expression to the appropriate visitor method.
func Visit[T any](v Visitor[T], expr Expression) T {
	switch e := expr.(type) {
	case *BinaryExpr:
		return v.VisitBinary(e)
	case *UnaryExpr:
		return v.VisitUnary(e)
	case *QualifierExpr:
		return v.VisitQualifier(e)
	case *PresenceExpr:
		return v.VisitPresence(e)
	case *GroupExpr:
		return v.VisitGroup(e)
	case *SelectorExpr:
		return v.VisitSelector(e)
	case *FuncCallExpr:
		return v.VisitFuncCall(e)
	default:
		// A nil expression or a ValueExpr (the value domain, outside the
		// predicate Visitor). A newly added predicate node reaching here would
		// be a bug; the ast dispatch-guard test fails before it can ship.
		var zero T
		return zero
	}
}
