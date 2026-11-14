package parser

import (
	"testing"

	"github.com/heyllave/query/ast"
)

// FuzzParse checks two properties over arbitrary input: the parser never
// panics, and a query that parses prints (ast.String) to text that parses
// back to the same tree, printing identically again. The seeds are the
// shapes that broke one or the other in the past; `go test` runs them as
// ordinary cases, and `make fuzz` explores from them.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"a=1", "state=draft AND total>50000", "lower(name)=john*", "x:1..2",
		"a@(b=1)", "n>=(1+2)*3", "s IN (a,b)", `a="x\"y"`, "a=[b]", "NOT a",
		"a=1d*2", "items@first", "NOT items@all(x=1)",
		// Escapes and quoting in unquoted values.
		`A=\(`, `A=\0\*\0`, `A=0$\\`, "A>0 +0>0", "A>0 +0 +0 + 00+",
		// Arithmetic over strings, wildcards and signed operands.
		`_<("0"%0)`, "A=(0%(0%A))", "A>0 %(A)", "A>0 *0 *0 *00* ", "A=0 %00*",
		"A>0000--00000%0000%00-0", "A>0000 -0000-0", "A>0000 -00000A0",
		"A((A)%A(2))", "A((A()%0))=0",
		// Dates, durations and the hyphen.
		"A>0%0000 -00 -00", "a-0>00-0m -0m -S", "A=0-(00h-(0%0))",
		"created_at>now()-30m-1h", "d:2026-01-01..2026-02-01",
		// Field names and keywords.
		`"foo bar"=1`, `""=0`, `"0"or"or"`, "true=1", "not=1", "In!<$0",
		"labels.example.com/team=x", "[cpu]>5",
		// IN forms, calls next to groups, Unicode spaces, argument arithmetic.
		"x NOT IN (a, b)", "lower(x) IN (a, b*)", "f(x) NOT IN (1)", "a (b)", "f(a) (b)",
		"a\u00a0b", "x=a\u3000b", "[\"a\u00a0b\"]=1", "ip=10.0.0.1/24", "d>f(d, 1-8)",
		"n>10*-3", "n>10--3", `n>abs(["cpu limit"])`, "f((A))", `f(["true"])`, "f(in)",
		// Selectors after values and negations.
		"A>0 @(A)", "(A!>0 @(A))", "email=a@b.com",
		// Floats with a bare dot and ranges touching dots.
		"A>00000. -000000-000000-0", "A:.0. ...0.", "A:\x94...00", "A>+0000 -0", "IN A(0A)", "A=0+(+0+(0))", "A(0%(A*(0%0)))", "A(0%(A*(0*0)))", "n>1+(2+3)", "IN(\\(,\\(,\\(,\\ )", "IN(*\\ )", "name=*\\", "A=0 %0\\\\\\\\*", "[A.iN]", "labels.in=1", "IN()!<0", "IN(0)!<0", "IN.A!<0", "NOT or.x=1",
		// Values that look like other things.
		"id=550e8400-e29b-41d4-a716-446655440000", "id=version-1.2.3-4", "n=5+3x",
		"A> =\"*", "A>0 %A(A)A<=0",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		expr, err := Parse(s, 0)
		if err != nil {
			return
		}
		out := ast.String(expr)
		again, err := Parse(out, 0)
		if err != nil {
			t.Fatalf("Parse(%q) printed as %q, which does not parse: %v", s, out, err)
		}
		if out2 := ast.String(again); out2 != out {
			t.Fatalf("Parse(%q) printed as %q, reparsed and printed as %q", s, out, out2)
		}
		// The utilities over the tree must not panic either.
		ast.Depth(expr)
		ast.Fields(expr)
		ast.Calls(expr)
		ast.Qualifiers(expr)
	})
}

// FuzzParseValue checks the value grammar the same way.
func FuzzParseValue(f *testing.F) {
	for _, s := range []string{"1+2", "now()-7d", "upper(name)", "(1+2)*3", "1/0", "len(tags)", "[base]*1.1", `"a"%2`, "-5", "+3.5"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		expr, err := ParseValue(s, 0)
		if err != nil {
			return
		}
		out := ast.String(expr)
		again, err := ParseValue(out, 0)
		if err != nil {
			t.Fatalf("ParseValue(%q) printed as %q, which does not parse: %v", s, out, err)
		}
		if out2 := ast.String(again); out2 != out {
			t.Fatalf("ParseValue(%q) printed as %q, reparsed and printed as %q", s, out, out2)
		}
	})
}
