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
		writeOperand(buf, e.Left, e.Op == token.And)
		if e.Op == token.And {
			buf.WriteString(" AND ")
		} else {
			buf.WriteString(" OR ")
		}
		writeOperand(buf, e.Right, e.Op == token.And)
	case *UnaryExpr:
		buf.WriteString("NOT ")
		// NOT binds tighter than either connective, so a bare binary operand
		// would print as `NOT a=1 OR b=2` — the negation of `a=1` alone.
		_, binary := e.Expr.(*BinaryExpr)
		writeGrouped(buf, e.Expr, binary)
	case *QualifierExpr:
		if e.FieldFunc != nil {
			writeFuncCall(buf, e.FieldFunc)
		} else {
			writeFieldName(buf, e.Field)
		}
		if e.EndValue != nil {
			buf.WriteByte(':')
			writeValue(buf, &e.Value)
			// A boundary that touches a '.' would fuse with the range
			// operator (`.0....0`), so pad it with spaces, which the lexer
			// accepts around `..`.
			if strings.HasSuffix(e.Value.Raw, ".") || strings.HasPrefix(e.EndValue.Raw, ".") {
				buf.WriteString(" .. ")
			} else {
				buf.WriteString("..")
			}
			writeValue(buf, e.EndValue)
		} else {
			buf.WriteString(token.OperatorSymbol(e.Operator))
			writeValue(buf, &e.Value)
		}
	case *PresenceExpr:
		writeFieldName(buf, e.Field)
	case *GroupExpr:
		buf.WriteByte('(')
		writeExpr(buf, e.Expr)
		buf.WriteByte(')')
	case *SelectorExpr:
		writeExpr(buf, e.Base)
		if _, presence := e.Base.(*PresenceExpr); !presence {
			// After a value the '@' needs a space, or an unquoted value such
			// as an e-mail address would absorb it.
			buf.WriteByte(' ')
		}
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

// writeFieldName writes a field in field position: bare when every segment
// is a plain identifier, otherwise in the quoted bracket form the parser
// accepts there (["foo bar"]).
func writeFieldName(buf *strings.Builder, fp FieldPath) {
	if fieldRefNeedsQuote(fp) || isReservedWord(fp.String()) {
		buf.WriteString(FieldRefString(fp, true))
		return
	}
	buf.WriteString(fp.String())
}

// isReservedWord reports whether s would lex as a keyword in field position
// in a context where the parser cannot recover it as a name (a bare `not`
// before another term reads as a negation).
func isReservedWord(s string) bool {
	switch strings.ToUpper(s) {
	case "NOT", "AND", "OR", "IN", "TRUE", "FALSE":
		return true
	default:
		return false
	}
}

// writeOperand writes one side of a connective. An OR under an AND needs its
// parentheses back: the tree only has that shape when the source grouped it —
// or when the parser lowered `f IN (a, b)` to `f=a OR f=b` — and printed bare
// it would read as `x AND f=a OR f=b`, which is a different query.
func writeOperand(buf *strings.Builder, operand Expression, underAnd bool) {
	child, ok := operand.(*BinaryExpr)
	writeGrouped(buf, operand, underAnd && ok && child.Op == token.Or)
}

// writeGrouped writes expr, inside parentheses when grouped is set.
func writeGrouped(buf *strings.Builder, expr Expression, grouped bool) {
	if grouped {
		buf.WriteByte('(')
	}
	writeExpr(buf, expr)
	if grouped {
		buf.WriteByte(')')
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
			if arg.Value.Type == ValueArith {
				// Arithmetic is an argument only as a paren group.
				buf.WriteByte('(')
				writeValue(buf, arg.Value)
				buf.WriteByte(')')
			} else {
				writeValue(buf, arg.Value)
			}
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
		// Arithmetic over a string operand only lexes inside a paren group
		// (the bare form would read as an unquoted string), so wrap it.
		grouped := hasStringOperand(v)
		if grouped {
			buf.WriteByte('(')
		}
		writeArithOperand(buf, v.Arith.Left, v.Arith.Op, true)
		if v.Arith.Op == ArithSub && looksLikeDatePrefix(v.Arith.Left) {
			// `2026-01-01` is a date: keep a subtraction whose left operand
			// is four digits from fusing into one. The right side is grouped
			// so a leading sign in it cannot read as a second operator.
			buf.WriteString(" - (")
			writeArithOperand(buf, v.Arith.Right, ArithAdd, true)
			buf.WriteByte(')')
		} else {
			buf.WriteString(v.Arith.Op.String())
			writeArithOperand(buf, v.Arith.Right, v.Arith.Op, false)
		}
		if grouped {
			buf.WriteByte(')')
		}
		return
	}
	if v.Type == ValueFieldRef {
		buf.WriteString(FieldRefString(v.Field, v.Quoted))
		return
	}
	if v.Type == ValueFloat {
		// A float written as `5.` or `.5` is canonicalized so it re-lexes as
		// a number in every position.
		raw := v.Raw
		if strings.HasSuffix(raw, ".") {
			raw += "0"
		}
		if strings.HasPrefix(raw, ".") || strings.HasPrefix(raw, "-.") || strings.HasPrefix(raw, "+.") {
			raw = strings.Replace(raw, ".", "0.", 1)
		}
		buf.WriteString(raw)
		return
	}
	if v.Type == ValueString && v.Wildcard && len(v.Raw) > 0 && strings.IndexByte("=<>!:.", v.Raw[0]) >= 0 {
		// A pattern that starts with an operator character would fuse with
		// the operator before it (`>` + `=*` reads as `>=`); a space keeps
		// them apart and is skipped by the lexer.
		buf.WriteByte(' ')
	}
	if v.Type == ValueString && (v.Quoted || (!v.Wildcard && !unquotedSafe(v.Raw))) {
		// A string that would not lex back as the same unquoted value (it is
		// empty, or holds whitespace, an operator or a delimiter) is quoted.
		writeQuotedString(buf, v.Str)
		return
	}
	buf.WriteString(v.Raw)
}

// unquotedSafe reports whether raw re-lexes as a single unquoted string
// value: non-empty, and free of whitespace, delimiters, comparison and
// arithmetic operators. Hyphens, dots and slashes are safe; the lexer treats
// a run containing letters as a bareword and leaves it whole.
func unquotedSafe(raw string) bool {
	if raw == "" {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			switch raw[i+1] {
			case '*', '\\', '(', ')':
				i++ // an escape pair the lexer decodes
				continue
			}
		}
		switch raw[i] {
		case ' ', '\t', '\n', '\r', '(', ')', '[', ']', '"', ',', '=', '<', '>', '!', ':', '+', '*', '/', '%':
			return false
		}
		if raw[i] < 0x20 || raw[i] == 0x7f {
			return false
		}
	}
	return true
}

// hasStringOperand reports whether an arithmetic value has a string literal
// anywhere among its operands.
func hasStringOperand(v *Value) bool {
	if v == nil {
		return false
	}
	switch v.Type { //nolint:exhaustive // other types are never string operands
	case ValueString:
		return true
	case ValueArith:
		return v.Arith != nil && (hasStringOperand(v.Arith.Left) || hasStringOperand(v.Arith.Right))
	default:
		return false
	}
}

// writeArithOperand renders an arithmetic operand, wrapping it in parens when
// its operator binds looser than the parent's (so `(1+2)*3` round-trips
// correctly instead of degenerating to `1+2*3`). The right operand of a
// non-commutative parent (`-`, `/`, `%`) also needs parens when its operator
// has equal precedence — `5-(2-1)` is not `5-2-1`.
func writeArithOperand(buf *strings.Builder, v *Value, parentOp ArithOp, isLeft bool) {
	if v != nil && !isLeft && (v.Type == ValueInteger || v.Type == ValueFloat) && len(v.Raw) > 0 && (v.Raw[0] == '-' || v.Raw[0] == '+') {
		// A signed literal on the right (`5 - -3`) is grouped so the sign
		// does not read as a second operator.
		buf.WriteByte('(')
		writeValue(buf, v)
		buf.WriteByte(')')
		return
	}
	if v != nil && v.Type == ValueString {
		// A string operand only lexes inside arithmetic when quoted. A
		// wildcard here is printed as literal text: arithmetic over a
		// pattern never resolves, so nothing is lost.
		writeQuotedString(buf, strings.ReplaceAll(v.Str, `\*`, "*"))
		return
	}
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

// looksLikeDatePrefix reports whether an operand's printed text ends in a
// run of four digits (its rightmost leaf), the start of a date literal.
func looksLikeDatePrefix(v *Value) bool {
	for v != nil && v.Type == ValueArith && v.Arith != nil {
		v = v.Arith.Right
	}
	if v == nil || v.Type != ValueInteger || len(v.Raw) != 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		if v.Raw[i] < '0' || v.Raw[i] > '9' {
			return false
		}
	}
	return true
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

// FieldRefString renders a value-position field reference: the bare form
// [path] when every path segment is a plain identifier, otherwise the quoted
// form ["text"] over the full dotted path. quoted forces the quoted form so a
// source that was written ["base-1"] round-trips quoted even though [base-1]
// would also be legal. Parser and serializer share this so Value.Raw and
// String() output stay in lockstep.
func FieldRefString(fp FieldPath, quoted bool) string {
	var buf strings.Builder
	buf.WriteByte('[')
	if quoted || fieldRefNeedsQuote(fp) {
		writeQuotedString(&buf, fp.String())
	} else {
		buf.WriteString(fp.String())
	}
	buf.WriteByte(']')
	return buf.String()
}

// fieldRefNeedsQuote reports whether a field path must use the quoted bracket
// form because a segment is empty or contains a character outside the bare
// identifier alphabet (ASCII letters, any non-ASCII rune, digits, underscore,
// hyphen, slash; not leading digit).
func fieldRefNeedsQuote(fp FieldPath) bool {
	if len(fp) == 0 {
		return true
	}
	for _, seg := range fp {
		if seg == "" {
			return true
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			isAlpha := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
			isDigit := c >= '0' && c <= '9'
			if i == 0 && !isAlpha {
				return true
			}
			if !isAlpha && !isDigit && c != '-' && c != '/' {
				return true
			}
		}
	}
	return false
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
