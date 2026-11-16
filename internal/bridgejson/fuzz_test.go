package bridgejson

import (
	"encoding/json"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

// FuzzBridgeRoundTrip checks that every query the parser accepts survives the
// JSON contract: the tree a client receives decodes back to a tree that prints
// exactly as the original did, so stringify after parse never changes a
// query's meaning, and no node type is silently dropped on the way out.
func FuzzBridgeRoundTrip(f *testing.F) {
	for _, s := range []string{
		"a=1", `contains(tags, "x")`, `contains(tags,"x") AND b>1`, "lower(name)=john",
		"x IN (a, b)", "x NOT IN (a)", "n:1..5", "d>now()-7d", "t>=[base]*2", `name="a b"`,
		"NOT items@all(x=1)", "items@first", `a=\(`, `name=*\\`, "f(in)", `A=0 %0\\\\*`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		expr, err := parser.Parse(s, 0)
		if err != nil {
			return
		}
		want := ast.String(expr)
		tree := AstToJSON(expr)
		if tree == nil {
			t.Fatalf("Parse(%q) produced a tree the bridge cannot encode", s)
		}
		data, err := json.Marshal(tree)
		if err != nil {
			t.Fatalf("marshal %q: %v", s, err)
		}
		back, err := JSONToAST(string(data))
		if err != nil {
			t.Fatalf("Parse(%q) encoded as %s, which does not decode: %v", s, data, err)
		}
		if got := ast.String(back); got != want {
			t.Fatalf("Parse(%q) prints %q; through the bridge it prints %q", s, want, got)
		}
	})
}
