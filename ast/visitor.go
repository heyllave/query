package ast

import "github.com/heyllave/query/token"

// Visitor defines typed callbacks for each AST node. Consumers implement this
// interface to transform an AST into any target representation (SQL, JSON,
// filter functions, React components, etc.).
//
// Each method receives the concrete node and returns a result of type T.
// Use [Visit] to dispatch an [Expression] to the correct method.
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
		var zero T
		return zero
	}
}

// SQLOperator returns the SQL equivalent of a comparison operator token.
// For wildcards, returns "LIKE". For presence, returns "IS NOT NULL".
func SQLOperator(op token.Type, wildcard bool) string {
	if wildcard {
		return "LIKE"
	}
	switch op { //nolint:exhaustive // only comparison operators
	case token.Eq:
		return "="
	case token.Neq:
		return "!="
	case token.Gt:
		return ">"
	case token.Gte:
		return ">="
	case token.Lt:
		return "<"
	case token.Lte:
		return "<="
	default:
		return "="
	}
}

// WildcardToLike converts a query wildcard pattern to a SQL LIKE pattern.
//
//	"John*"    → "John%"
//	"*yota"    → "%yota"
//	"*test*"   → "%test%"
func WildcardToLike(pattern string) string {
	var buf []byte
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			buf = append(buf, '%')
		case '%', '_':
			buf = append(buf, '\\', pattern[i])
		default:
			buf = append(buf, pattern[i])
		}
	}
	return string(buf)
}
