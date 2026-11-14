package ast

import (
	"testing"

	"github.com/heyllave/query/token"
)

// TestString_ConstructedValues drives the serializer with values built by
// hand (as a client building a tree would), covering the quoting and
// grouping decisions that keep String output re-parseable.
func TestString_ConstructedValues(t *testing.T) {
	q := func(field string, op token.Type, v Value) Expression {
		return &QualifierExpr{Field: FieldPath{field}, Operator: op, Value: v}
	}
	str := func(s string) Value { return Value{Type: ValueString, Raw: s, Str: s} }
	num := func(raw string, n int64) Value { return Value{Type: ValueInteger, Raw: raw, Int: n} }
	arith := func(op ArithOp, l, r Value) Value {
		return Value{Type: ValueArith, Arith: &ArithExpr{Op: op, Left: &l, Right: &r}}
	}

	tests := []struct {
		name string
		expr Expression
		want string
	}{
		{"plain string stays bare", q("a", token.Eq, str("draft")), "a=draft"},
		{"empty string is quoted", q("a", token.Eq, str("")), `a=""`},
		{"space is quoted", q("a", token.Eq, str("a b")), `a="a b"`},
		{"operator char is quoted", q("a", token.Eq, str("0>1")), `a="0>1"`},
		{"control char is quoted", q("a", token.Eq, str("a\x01b")), `a="a` + "\x01" + `b"`},
		{"escape pair stays bare", q("a", token.Eq, Value{Type: ValueString, Raw: `a\(b`, Str: "a(b"}), `a=a\(b`},
		{"quoted flag wins", q("a", token.Eq, Value{Type: ValueString, Raw: "x", Str: "x", Quoted: true}), `a="x"`},
		{"wildcard starting with operator gets a space", q("a", token.Gt, Value{Type: ValueString, Raw: "=*", Str: "=*", Wildcard: true}), "a> =*"},
		{"float with trailing dot", q("a", token.Eq, Value{Type: ValueFloat, Raw: "5.", Float: 5}), "a=5.0"},
		{"float with leading dot", q("a", token.Eq, Value{Type: ValueFloat, Raw: ".5", Float: 0.5}), "a=0.5"},
		{"negative float with leading dot", q("a", token.Eq, Value{Type: ValueFloat, Raw: "-.5", Float: -0.5}), "a=-0.5"},
		{"arith over string is grouped and quoted", q("a", token.Eq, arith(ArithMod, num("0", 0), str("0A"))), `a=(0%"0A")`},
		{"arith with four-digit left pads subtraction", q("a", token.Gt, arith(ArithSub, num("2026", 2026), num("1", 1))), "a>2026 - (1)"},
		{"arith with four-digit rightmost leaf pads", q("a", token.Gt, arith(ArithSub, arith(ArithMod, num("0", 0), num("2026", 2026)), num("1", 1))), "a>0%2026 - (1)"},
		{"signed right operand is grouped", q("a", token.Gt, arith(ArithSub, num("5", 5), num("-3", -3))), "a>5-(-3)"},
		{"precedence parens", q("a", token.Gt, arith(ArithMul, arith(ArithAdd, num("1", 1), num("2", 2)), num("3", 3))), "a>(1+2)*3"},
		{"right-assoc subtraction parens", q("a", token.Gt, arith(ArithSub, num("5", 5), arith(ArithSub, num("2", 2), num("1", 1)))), "a>5-(2-1)"},
		{"range touching dots is spaced", &QualifierExpr{Field: FieldPath{"a"}, Operator: token.Range, Value: str(".0."), EndValue: &Value{Type: ValueString, Raw: ".0", Str: ".0"}}, "a:.0. .. .0"},
		{"keyword field is bracketed", q("true", token.Eq, num("1", 1)), `["true"]=1`},
		{"not as field is bracketed", &PresenceExpr{Field: FieldPath{"not"}}, `["not"]`},
		{"slash in field stays bare", &QualifierExpr{Field: FieldPath{"labels", "example", "com/team"}, Operator: token.Eq, Value: str("x")}, "labels.example.com/team=x"},
		{"space in field is bracketed", q("foo bar", token.Eq, num("1", 1)), `["foo bar"]=1`},
		{"function field prints the call", &QualifierExpr{FieldFunc: &FuncCallExpr{Name: "lower", Args: []FuncArg{{Field: &FieldPath{"name"}}}}, Operator: token.Eq, Value: str("john")}, "lower(name)=john"},
		{"function range prints the call", &QualifierExpr{FieldFunc: &FuncCallExpr{Name: "len", Args: []FuncArg{{Field: &FieldPath{"name"}}}}, Operator: token.Range, Value: num("1", 1), EndValue: &Value{Type: ValueInteger, Raw: "5", Int: 5}}, "len(name):1..5"},
		{"arith argument is grouped", &FuncCallExpr{Name: "f", Args: []FuncArg{{Value: &Value{Type: ValueArith, Arith: &ArithExpr{Op: ArithMod, Left: &Value{Type: ValueFunc, Func: &FuncCallExpr{Name: "g"}}, Right: &Value{Type: ValueInteger, Raw: "0"}}}}}}, "f((g()%0))"},
		{"selector after value is spaced", &SelectorExpr{Base: q("email", token.Eq, str("a@b.com")), Inner: &PresenceExpr{Field: FieldPath{"x"}}}, "email=a@b.com @(x)"},
		{"selector after negation is spaced", &SelectorExpr{Base: &UnaryExpr{Op: token.Not, Expr: q("a", token.Gt, num("0", 0))}, Selector: "first"}, "NOT a>0 @first"},
		{"selector after presence is tight", &SelectorExpr{Base: &PresenceExpr{Field: FieldPath{"items"}}, Selector: "last"}, "items@last"},
		{"NOT over binary is grouped", &UnaryExpr{Op: token.Not, Expr: &BinaryExpr{Op: token.Or, Left: q("a", token.Eq, num("1", 1)), Right: q("b", token.Eq, num("2", 2))}}, "NOT (a=1 OR b=2)"},
		{"OR under AND is grouped", &BinaryExpr{Op: token.And, Left: &BinaryExpr{Op: token.Or, Left: q("a", token.Eq, num("1", 1)), Right: q("a", token.Eq, num("2", 2))}, Right: q("b", token.Eq, num("3", 3))}, "(a=1 OR a=2) AND b=3"},
		{"value expr", &ValueExpr{Value: arith(ArithAdd, num("1", 1), num("2", 2))}, "1+2"},
		{"nil is empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.expr); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFieldRefString_Forms(t *testing.T) {
	tests := []struct {
		fp     FieldPath
		quoted bool
		want   string
	}{
		{FieldPath{"base"}, false, "[base]"},
		{FieldPath{"base"}, true, `["base"]`},
		{FieldPath{"labels", "dev"}, false, "[labels.dev]"},
		{FieldPath{"a/b"}, false, "[a/b]"},
		{FieldPath{"line item"}, false, `["line item"]`},
		{FieldPath{"1st"}, false, `["1st"]`},
		{FieldPath{""}, false, `[""]`},
		{FieldPath{}, false, `[""]`},
		{FieldPath{`q"q`}, false, `["q\"q"]`},
	}
	for _, tt := range tests {
		if got := FieldRefString(tt.fp, tt.quoted); got != tt.want {
			t.Errorf("FieldRefString(%v, %v) = %q, want %q", tt.fp, tt.quoted, got, tt.want)
		}
	}
}

func TestUnquotedSafe(t *testing.T) {
	safe := []string{"draft", "550e8400-e29b", "1.2.3-beta", "a@b.com", "a\\(b", "a\\*", "x\\\\y", "ünïcode"}
	unsafe := []string{"", "a b", "(a", "a)", "[a]", `a"b`, "a,b", "a=b", "a<b", "a!b", "a:b", "a+b", "a*", "a/b", "a%b", "\x7f", "a\\"}
	for _, s := range safe {
		if !unquotedSafe(s) {
			t.Errorf("unquotedSafe(%q) = false, want true", s)
		}
	}
	for _, s := range unsafe {
		if unquotedSafe(s) {
			t.Errorf("unquotedSafe(%q) = true, want false", s)
		}
	}
}

func TestSQLOperator_And_WildcardToLike(t *testing.T) {
	if got := SQLOperator(token.Neq, true); got != "NOT LIKE" {
		t.Errorf("SQLOperator(Neq, wildcard) = %q, want NOT LIKE", got)
	}
	if got := SQLOperator(token.Eq, true); got != "LIKE" {
		t.Errorf("SQLOperator(Eq, wildcard) = %q, want LIKE", got)
	}
	for _, op := range []token.Type{token.Eq, token.Neq, token.Gt, token.Gte, token.Lt, token.Lte, token.Range} {
		if SQLOperator(op, false) == "" {
			t.Errorf("SQLOperator(%v) is empty", op)
		}
	}
	tests := map[string]string{
		"John*":    "John%",
		"*yota":    "%yota",
		"*mid*":    "%mid%",
		"50%*":     `50\%%`,
		"a_b*":     `a\_b%`,
		`a\*b*`:    "a*b%",
		`back\\s*`: `back\\s%`,
		`c\\*`:     `c\\%`,
	}
	for in, want := range tests {
		if got := WildcardToLike(in); got != want {
			t.Errorf("WildcardToLike(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDepth_ChainsAndNesting(t *testing.T) {
	leaf := func(n string) Expression { return &PresenceExpr{Field: FieldPath{n}} }
	and := func(l, r Expression) Expression { return &BinaryExpr{Op: token.And, Left: l, Right: r} }
	or := func(l, r Expression) Expression { return &BinaryExpr{Op: token.Or, Left: l, Right: r} }
	tests := []struct {
		name string
		expr Expression
		want int
	}{
		{"nil", nil, 0},
		{"leaf", leaf("a"), 1},
		{"left-nested and chain", and(and(and(leaf("a"), leaf("b")), leaf("c")), leaf("d")), 2},
		{"right-nested and chain", and(leaf("a"), and(leaf("b"), and(leaf("c"), leaf("d")))), 2},
		{"alternating connectives", and(or(leaf("a"), leaf("b")), leaf("c")), 3},
		{"group adds a level", &GroupExpr{Expr: and(leaf("a"), leaf("b"))}, 3},
		{"not adds a level", &UnaryExpr{Op: token.Not, Expr: leaf("a")}, 2},
		{"selector takes the deeper side", &SelectorExpr{Base: leaf("items"), Inner: and(leaf("a"), or(leaf("b"), leaf("c")))}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Depth(tt.expr); got != tt.want {
				t.Errorf("Depth = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWalk_VisitsEveryCall(t *testing.T) {
	inner := &FuncCallExpr{Name: "trim", Args: []FuncArg{{Field: &FieldPath{"name"}}}}
	expr := &BinaryExpr{Op: token.And,
		Left: &QualifierExpr{FieldFunc: &FuncCallExpr{Name: "lower", Args: []FuncArg{{Call: inner}}}, Operator: token.Eq,
			Value: Value{Type: ValueFunc, Func: &FuncCallExpr{Name: "now"}}},
		Right: &QualifierExpr{Field: FieldPath{"n"}, Operator: token.Gt,
			Value: Value{Type: ValueArith, Arith: &ArithExpr{Op: ArithMul,
				Left:  &Value{Type: ValueFunc, Func: &FuncCallExpr{Name: "rate", Args: []FuncArg{{Value: &Value{Type: ValueFunc, Func: &FuncCallExpr{Name: "region"}}}}}},
				Right: &Value{Type: ValueInteger, Raw: "2", Int: 2}}}},
	}
	var names []string
	for _, fc := range Calls(expr) {
		names = append(names, fc.Name)
	}
	want := []string{"lower", "trim", "now", "rate", "region"}
	if len(names) != len(want) {
		t.Fatalf("Calls = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("Calls = %v, want %v", names, want)
		}
	}
	fields := Fields(expr)
	if len(fields) != 2 || fields[0].String() != "name" || fields[1].String() != "n" {
		t.Errorf("Fields = %v, want [name n]", fields)
	}
	// A false return prunes children.
	seen := 0
	Walk(expr, func(Expression) bool { seen++; return false })
	if seen != 1 {
		t.Errorf("pruned walk visited %d nodes, want 1", seen)
	}
}

func TestDecodePattern(t *testing.T) {
	for in, want := range map[string]string{
		"jo*": "jo*", `a\*b*`: "a*b*", `c\\*`: `c\*`, `\\\*`: `\*`, `plain`: "plain", `trail\\`: `trail\`,
	} {
		if got := DecodePattern(in); got != want {
			t.Errorf("DecodePattern(%q) = %q, want %q", in, got, want)
		}
	}
}
