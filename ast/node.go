package ast

import (
	"github.com/heyllave/query/token"
)

// Node is the common interface for all AST nodes.
type Node interface {
	Pos() token.Position
	node() // marker — restricts implementations to this package
}

// Expression is an AST node that represents a query expression.
type Expression interface {
	Node
	expr() // marker — restricts implementations to this package
}

// BinaryExpr represents a binary logical expression: left AND right, left OR right.
type BinaryExpr struct {
	Op       token.Type // token.And or token.Or
	Left     Expression
	Right    Expression
	Position token.Position
}

// Pos returns the position of the binary expression.
func (e *BinaryExpr) Pos() token.Position { return e.Position }
func (e *BinaryExpr) node()               { _ = e }
func (e *BinaryExpr) expr()               { _ = e }

// UnaryExpr represents a unary expression: NOT expr.
type UnaryExpr struct {
	Op       token.Type // token.Not
	Expr     Expression
	Position token.Position
}

// Pos returns the position of the unary expression.
func (e *UnaryExpr) Pos() token.Position { return e.Position }
func (e *UnaryExpr) node()               { _ = e }
func (e *UnaryExpr) expr()               { _ = e }

// QualifierExpr represents a field comparison: field op value.
// For range expressions (field:start..end), EndValue is non-nil.
type QualifierExpr struct {
	Field     FieldPath     // e.g., ["labels", "dev"]
	FieldFunc *FuncCallExpr // optional: function wrapping the field, e.g., lower(name)
	Operator  token.Type    // comparison operator
	Value     Value         // primary value
	EndValue  *Value        // end value for range expressions
	Position  token.Position
}

// Pos returns the position of the qualifier expression.
func (e *QualifierExpr) Pos() token.Position { return e.Position }
func (e *QualifierExpr) node()               { _ = e }
func (e *QualifierExpr) expr()               { _ = e }

// IsRange reports whether this is a range expression (field:start..end).
func (e *QualifierExpr) IsRange() bool { return e.EndValue != nil }

// IsWildcard reports whether this qualifier uses a wildcard value.
func (e *QualifierExpr) IsWildcard() bool { return e.Value.Wildcard }

// HasFieldFunc reports whether this qualifier has a field transform function.
func (e *QualifierExpr) HasFieldFunc() bool { return e.FieldFunc != nil }

// PresenceExpr represents a field presence check: just the field name with no operator.
type PresenceExpr struct {
	Field    FieldPath
	Position token.Position
}

// Pos returns the position of the presence expression.
func (e *PresenceExpr) Pos() token.Position { return e.Position }
func (e *PresenceExpr) node()               { _ = e }
func (e *PresenceExpr) expr()               { _ = e }

// SelectorExpr represents a selector expression applied to a list-valued field:
//
//	expr @first              — list exists and has ≥ 1 element
//	expr @last               — list exists and has ≥ 1 element (distinct for codegen)
//	expr @(inner)            — EXISTS: at least one element satisfies inner
//	expr @any(inner)         — alias of @(inner)
//	expr @all(inner)         — universal: every element satisfies inner
//	expr @none(inner)        — no element satisfies inner
type SelectorExpr struct {
	Base     Expression
	Selector string     // "first", "last", "any", "all", "none", or "" for @(...)
	Inner    Expression // inner expression for @(...), @any/@all/@none
	Position token.Position
}

// Pos returns the position of the selector expression.
func (e *SelectorExpr) Pos() token.Position { return e.Position }
func (e *SelectorExpr) node()               { _ = e }
func (e *SelectorExpr) expr()               { _ = e }

// GroupExpr represents a parenthesized expression: (expression).
type GroupExpr struct {
	Expr     Expression
	Position token.Position
}

// Pos returns the position of the group expression.
func (e *GroupExpr) Pos() token.Position { return e.Position }
func (e *GroupExpr) node()               { _ = e }
func (e *GroupExpr) expr()               { _ = e }

// FuncCallExpr represents a function call: lower(name), now(), len(description).
//
// Function calls can appear:
//   - As field transforms: lower(name)=john* — wraps a field lookup
//   - As value generators: created_at>=now() — produces a comparison value
//   - As boolean predicates: contains(tags, "urgent") — standalone filter
type FuncCallExpr struct {
	Name     string    // function name
	Args     []FuncArg // arguments
	Position token.Position
}

// Pos returns the position of the function call.
func (e *FuncCallExpr) Pos() token.Position { return e.Position }
func (e *FuncCallExpr) node()               { _ = e }
func (e *FuncCallExpr) expr()               { _ = e }

// ValueExpr is a bare value in expression position: the query computes and
// returns a value rather than evaluating to a boolean. Boolean is one result
// domain of the language; a ValueExpr is a value-domain root.
//
//	now()-7d        // a timestamp
//	(50000+1000)*1.1 // a number
//	upper(name)     // a string
//
// It is produced only by [github.com/heyllave/query/parser.ParseValue] (and the
// eval value-compile entrypoint), never by the boolean grammar — so the
// predicate visitors and matchers never encounter it. Field references in value
// position are only available through function arguments (e.g. upper(name)),
// matching the arithmetic-operand rules.
type ValueExpr struct {
	Value    Value
	Position token.Position
}

// Pos returns the position of the value expression.
func (e *ValueExpr) Pos() token.Position { return e.Position }
func (e *ValueExpr) node()               { _ = e }
func (e *ValueExpr) expr()               { _ = e }

// FuncArg is a function argument: a field reference, a literal value, or a nested call.
type FuncArg struct {
	Field *FieldPath    // field reference: name, labels.dev
	Value *Value        // literal: "urgent", 42, true
	Call  *FuncCallExpr // nested function: year(now())
}

// String returns a debug representation of the argument.
func (a FuncArg) String() string {
	switch {
	case a.Field != nil:
		return a.Field.String()
	case a.Value != nil:
		return a.Value.Raw
	case a.Call != nil:
		return a.Call.Name + "(...)"
	default:
		return "<empty>"
	}
}
