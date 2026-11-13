package token

import "testing"

func TestType_IsKeyword(t *testing.T) {
	for _, tt := range []Type{And, Or, Not, In, Boolean} {
		if !tt.IsKeyword() {
			t.Errorf("%v.IsKeyword() = false", tt)
		}
	}
	for _, tt := range []Type{Ident, String, Integer, Eq, LParen, EOF} {
		if tt.IsKeyword() {
			t.Errorf("%v.IsKeyword() = true", tt)
		}
	}
}

func TestType_IsValueEnd(t *testing.T) {
	for _, tt := range []Type{Integer, Float, Duration, Date, RParen, FieldRef} {
		if !tt.IsValueEnd() {
			t.Errorf("%v.IsValueEnd() = false", tt)
		}
	}
	for _, tt := range []Type{Ident, String, Eq, LParen, Comma, And} {
		if tt.IsValueEnd() {
			t.Errorf("%v.IsValueEnd() = true", tt)
		}
	}
}

func TestToken_StringAndRaw(t *testing.T) {
	tok := Token{Type: String, Value: "a(b", Raw: `a\(b`, Pos: Position{Offset: 2, Length: 4}}
	if got := tok.String(); got != `STRING("a(b")` {
		t.Errorf("Token.String() = %q", got)
	}
	if got := (Token{Type: EOF}).String(); got != "EOF" {
		t.Errorf("EOF token String() = %q", got)
	}
	if got := tok.Pos.String(); got != "offset 2, length 4" {
		t.Errorf("Position.String() = %q", got)
	}
	if got := Type(999).String(); got != "Type(999)" {
		t.Errorf("unknown Type.String() = %q", got)
	}
	for _, op := range []Type{Ngt, Ngte, Nlt, Nlte} {
		if !op.IsNegatedOperator() || NegateOperator(op) == op {
			t.Errorf("%v negated operator handling", op)
		}
	}
	if NegateOperator(Eq) != Eq || Eq.IsNegatedOperator() {
		t.Error("Eq is not negated")
	}
	for _, op := range []Type{Plus, Minus, Mul, Div, Mod} {
		if !op.IsArithmetic() || OperatorSymbol(op) == "" {
			t.Errorf("%v arithmetic handling", op)
		}
	}
	if And.IsArithmetic() || !And.IsLogical() || Eq.IsLogical() {
		t.Error("logical/arithmetic classification")
	}
	if OperatorSymbol(Ident) != "=" {
		t.Error("OperatorSymbol default")
	}
}
