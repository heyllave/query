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
			writeValue(buf, &e.Value)
			buf.WriteString("..")
			writeValue(buf, e.EndValue)
		} else {
			buf.WriteString(token.OperatorSymbol(e.Operator))
			writeValue(buf, &e.Value)
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
		switch {
		case e.Selector != "" && e.Inner != nil:
			// @all(...), @any(...), @none(...): name keeps the inner expression.
			buf.WriteString(e.Selector)
			buf.WriteByte('(')
			writeExpr(buf, e.Inner)
			buf.WriteByte(')')
		case e.Selector != "":
			// @first / @last.
			buf.WriteString(e.Selector)
		case e.Inner != nil:
			// Anonymous @(...) (implicit EXISTS).
			buf.WriteByte('(')
			writeExpr(buf, e.Inner)
			buf.WriteByte(')')
		}
	case *FuncCallExpr:
		writeFuncCall(buf, e)
	case *ValueExpr:
		writeValue(buf, &e.Value)
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
			writeValue(buf, arg.Value)
		case arg.Call != nil:
			writeFuncCall(buf, arg.Call)
		}
	}
	buf.WriteByte(')')
}

// writeValue serializes a Value to the buffer. Function-valued expressions
// (e.g. now() in `created_at>=now()`) re-emit the call source; quoted string
// literals preserve their quoting and escape any embedded special chars;
// arithmetic expressions recursively render their operands.
func writeValue(buf *strings.Builder, v *Value) {
	if v == nil {
		return
	}
	if v.Type == ValueFunc && v.Func != nil {
		writeFuncCall(buf, v.Func)
		return
	}
	if v.Type == ValueArith && v.Arith != nil {
		writeArithOperand(buf, v.Arith.Left, v.Arith.Op, true)
		buf.WriteString(v.Arith.Op.String())
		writeArithOperand(buf, v.Arith.Right, v.Arith.Op, false)
		return
	}
	if v.Type == ValueString && v.Quoted {
		writeQuotedString(buf, v.Str)
		return
	}
	buf.WriteString(v.Raw)
}

// writeArithOperand renders an arithmetic operand, wrapping it in parens when
// its operator binds looser than the parent's (so `(1+2)*3` round-trips
// correctly instead of degenerating to `1+2*3`). The right operand of a
// non-commutative parent (`-`, `/`, `%`) also needs parens when its operator
// has equal precedence — `5-(2-1)` is not `5-2-1`.
func writeArithOperand(buf *strings.Builder, v *Value, parentOp ArithOp, isLeft bool) {
	if v != nil && v.Type == ValueArith && v.Arith != nil {
		childPrec := arithPrecedence(v.Arith.Op)
		parentPrec := arithPrecedence(parentOp)
		needParens := childPrec < parentPrec
		if !needParens && !isLeft && childPrec == parentPrec {
			// Non-commutative ops need right-side parens at equal precedence
			// (`5-(2-1)` is not `5-2-1`). Commutative `+` and `*` don't.
			switch parentOp {
			case ArithSub, ArithDiv, ArithMod:
				needParens = true
			default:
				// ArithAdd, ArithMul — commutative, no parens needed.
			}
		}
		if needParens {
			buf.WriteByte('(')
			writeValue(buf, v)
			buf.WriteByte(')')
			return
		}
	}
	writeValue(buf, v)
}

func arithPrecedence(op ArithOp) int {
	switch op {
	case ArithMul, ArithDiv, ArithMod:
		return 2
	case ArithAdd, ArithSub:
		return 1
	default:
		return 0
	}
}

func writeQuotedString(buf *strings.Builder, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\':
			buf.WriteByte('\\')
			buf.WriteByte(c)
		case '\n':
			buf.WriteString(`\n`)
		case '\t':
			buf.WriteString(`\t`)
		case '\r':
			buf.WriteString(`\r`)
		default:
			buf.WriteByte(c)
		}
	}
	buf.WriteByte('"')
}
