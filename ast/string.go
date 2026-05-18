package ast

import (
	"strings"

	"github.com/heyllave/query/token"
)

// String serializes an AST expression back to a query string.
// This is the inverse of parsing — String(Parse(q)) == q for normalized forms.
func String(expr Expression) string {
	if expr == nil {
		return ""
	}
	var buf strings.Builder
	writeExpr(&buf, expr)
	return buf.String()
}

func writeExpr(buf *strings.Builder, expr Expression) {
	switch e := expr.(type) {
	case *BinaryExpr:
		writeExpr(buf, e.Left)
		if e.Op == token.And {
			buf.WriteString(" AND ")
		} else {
			buf.WriteString(" OR ")
		}
		writeExpr(buf, e.Right)
	case *UnaryExpr:
		buf.WriteString("NOT ")
		writeExpr(buf, e.Expr)
	case *QualifierExpr:
		buf.WriteString(e.Field.String())
		if e.EndValue != nil {
			buf.WriteByte(':')
			buf.WriteString(e.Value.Raw)
			buf.WriteString("..")
			buf.WriteString(e.EndValue.Raw)
		} else {
			buf.WriteString(token.OperatorSymbol(e.Operator))
			buf.WriteString(e.Value.Raw)
		}
	case *PresenceExpr:
		buf.WriteString(e.Field.String())
	case *GroupExpr:
		buf.WriteByte('(')
		writeExpr(buf, e.Expr)
		buf.WriteByte(')')
	case *SelectorExpr:
		writeExpr(buf, e.Base)
		buf.WriteByte('@')
		if e.Selector != "" {
			buf.WriteString(e.Selector)
		} else if e.Inner != nil {
			buf.WriteByte('(')
			writeExpr(buf, e.Inner)
			buf.WriteByte(')')
		}
	case *FuncCallExpr:
		writeFuncCall(buf, e)
	}
}

func writeFuncCall(buf *strings.Builder, fc *FuncCallExpr) {
	buf.WriteString(fc.Name)
	buf.WriteByte('(')
	for i, arg := range fc.Args {
		if i > 0 {
			buf.WriteString(", ")
		}
		switch {
		case arg.Field != nil:
			buf.WriteString(arg.Field.String())
		case arg.Value != nil:
			buf.WriteString(arg.Value.Raw)
		case arg.Call != nil:
			writeFuncCall(buf, arg.Call)
		}
	}
	buf.WriteByte(')')
}
