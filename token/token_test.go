package token

import "testing"

func TestPosition_String(t *testing.T) {
	p := Position{Offset: 5, Length: 3}
	want := "offset 5, length 3"
	if p.String() != want {
		t.Errorf("got %q, want %q", p.String(), want)
	}
}

func TestType_String(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{Illegal, "ILLEGAL"},
		{EOF, "EOF"},
		{Ident, "IDENT"},
		{String, "STRING"},
		{Integer, "INTEGER"},
		{Float, "FLOAT"},
		{Date, "DATE"},
		{Duration, "DURATION"},
		{Boolean, "BOOLEAN"},
		{Eq, "="},
		{Neq, "!="},
		{Gt, ">"},
		{Gte, ">="},
		{Lt, "<"},
		{Lte, "<="},
		{Range, ".."},
		{Colon, ":"},
		{And, "AND"},
		{Or, "OR"},
		{Not, "NOT"},
		{LParen, "("},
		{RParen, ")"},
		{At, "@"},
		{Dot, "."},
		{Comma, ","},
		{Wildcard, "*"},
		{Type(9999), "Type(9999)"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("Type(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestType_IsOperator(t *testing.T) {
	operators := []Type{Eq, Neq, Gt, Gte, Lt, Lte, Colon}
	for _, op := range operators {
		if !op.IsOperator() {
			t.Errorf("%v.IsOperator() = false, want true", op)
		}
	}
	nonOperators := []Type{Ident, String, And, Or, Not, LParen, RParen, Dot, Comma, Range}
	for _, op := range nonOperators {
		if op.IsOperator() {
			t.Errorf("%v.IsOperator() = true, want false", op)
		}
	}
}

func TestType_IsLogical(t *testing.T) {
	if !And.IsLogical() {
		t.Error("And.IsLogical() = false, want true")
	}
	if !Or.IsLogical() {
		t.Error("Or.IsLogical() = false, want true")
	}
	if Not.IsLogical() {
		t.Error("Not.IsLogical() = true, want false")
	}
	if Eq.IsLogical() {
		t.Error("Eq.IsLogical() = true, want false")
	}
}

func TestToken_String(t *testing.T) {
	tests := []struct {
		tok  Token
		want string
	}{
		{Token{Type: Ident, Value: "name"}, `IDENT("name")`},
		{Token{Type: EOF}, "EOF"},
		{Token{Type: Eq, Value: "="}, `=("=")`},
	}
	for _, tt := range tests {
		if got := tt.tok.String(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestOperatorSymbol(t *testing.T) {
	tests := []struct {
		op   Type
		want string
	}{
		{Eq, "="},
		{Neq, "!="},
		{Gt, ">"},
		{Gte, ">="},
		{Lt, "<"},
		{Lte, "<="},
		{Range, ".."},
		{Colon, ":"},
		{Ident, "="}, // default
	}
	for _, tt := range tests {
		if got := OperatorSymbol(tt.op); got != tt.want {
			t.Errorf("OperatorSymbol(%v) = %q, want %q", tt.op, got, tt.want)
		}
	}
}

// TestOperatorInfo_TableConsistency guards the single operator table: the three
// operator families are pinned to their exact members, every symbol agrees with
// the type's printed name, and each negated comparison points at a well-formed
// positive form. A malformed or forgotten row for a new operator fails here.
func TestOperatorInfo_TableConsistency(t *testing.T) {
	comparison := map[Type]bool{Eq: true, Neq: true, Gt: true, Gte: true, Lt: true, Lte: true, Ngt: true, Ngte: true, Nlt: true, Nlte: true, Colon: true}
	arithmetic := map[Type]bool{Plus: true, Minus: true, Mul: true, Div: true, Mod: true}
	negated := map[Type]Type{Ngt: Gt, Ngte: Gte, Nlt: Lt, Nlte: Lte}

	for ty := Type(0); int(ty) < len(operatorInfo); ty++ {
		info := operatorInfo[ty]

		if ty.IsOperator() != comparison[ty] {
			t.Errorf("%v: IsOperator()=%v, table comparison=%v, want member set=%v", ty, ty.IsOperator(), info.comparison, comparison[ty])
		}
		if ty.IsArithmetic() != arithmetic[ty] {
			t.Errorf("%v: IsArithmetic()=%v, want %v", ty, ty.IsArithmetic(), arithmetic[ty])
		}
		if ty.IsNegatedOperator() != (negated[ty] != Illegal) {
			t.Errorf("%v: IsNegatedOperator()=%v, want %v", ty, ty.IsNegatedOperator(), negated[ty] != Illegal)
		}
		// A symbol in the table must match the type's printed name.
		if info.symbol != "" && info.symbol != ty.String() {
			t.Errorf("%v: table symbol %q != typeNames %q", ty, info.symbol, ty.String())
		}
		// A negated comparison points at a positive comparison form.
		if base := info.negatedBase; base != Illegal {
			if want := negated[ty]; base != want {
				t.Errorf("%v: negatedBase=%v, want %v", ty, base, want)
			}
			if !base.IsOperator() || base.IsNegatedOperator() {
				t.Errorf("%v: negatedBase %v must be a positive comparison operator", ty, base)
			}
			if got := NegateOperator(ty); got != base {
				t.Errorf("NegateOperator(%v)=%v, want %v", ty, got, base)
			}
			if "!"+base.String() != info.symbol {
				t.Errorf("%v: symbol %q is not \"!\"+%q", ty, info.symbol, base.String())
			}
		}
	}
	// NegateOperator leaves a non-negated operator unchanged.
	for _, op := range []Type{Eq, Gt, Lte, Plus, Ident} {
		if got := NegateOperator(op); got != op {
			t.Errorf("NegateOperator(%v)=%v, want unchanged", op, got)
		}
	}
}
