package parser

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/heyllave/query/parser/internal/shape"
)

// readsTheWholeRun reports whether the lexer, reading s as an arithmetic
// value, covers the run that shape judged arithmetic. The two decide a
// value's extent separately: shape looks ahead, lexArithValueExpr reads. A
// reader that stops early and says nothing hands the rest of the run to
// field position, and when the rest starts a term of its own it becomes a
// second condition: now()x read as now() AND x. Stopping early is fine with
// an error, before a mark that cannot start a term (an operator, a
// selector's @), which the parser then judges, or before AND or OR, which
// join two conditions with or without a space.
func readsTheWholeRun(s string) bool {
	if !shape.IsArithmetic(s, 0) {
		return true
	}
	l := &lexer{input: s}
	l.lexArithValueExpr()
	return len(l.errors) > 0 || l.pos >= shape.RunEnd(s, 0) || !beginsTerm(s[l.pos]) || l.atConnective()
}

var agreementSeeds = []string{
	"10-3", "now()-7d", "now()x", "[a]x", "[a]1.5", "7d7d", "2026-11-16[a]", `now()"q"`,
	"(1+2)x", "2026-11-16T10:00:00Z-1h", "1h*2026-11-16f(x)", "f(x)now()", "10%7d2026-11-16",
	"now()-7d AND x", "(1+2", "1+now(", "x2026-11-16now()", "2*[unit-price]", `1+"a"b`,
	"now()AND x", "[a]OR b", "now()ANDx", "now()ORDER",
	"(2026-11-16 7d%*", "(\xa5 0000", // found by the fuzzer: a term after a space inside a group
}

func TestArithmetic_ReadsTheWholeRun(t *testing.T) {
	for _, s := range agreementSeeds {
		if !readsTheWholeRun(s) {
			t.Errorf("%q: the lexer stops inside the run shape judged arithmetic, with no error", s)
		}
	}
	// Random runs of the pieces arithmetic is made of, glued in any order.
	pieces := []string{"1", "10", "2026-11-16", "2026-11-16T10:00:00Z", "7d", "1h", "+", "-", "*", "/", "%",
		"(", ")", "now()", "f(x)", "[a]", `"q"`, " ", ",", "..", "x", "1.5", "1.2.3", "e", "T"}
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 50000; n++ {
		var b strings.Builder
		for k := r.Intn(8) + 1; k > 0; k-- {
			b.WriteString(pieces[r.Intn(len(pieces))])
		}
		if s := b.String(); !readsTheWholeRun(s) {
			t.Fatalf("%q: the lexer stops inside the run shape judged arithmetic, with no error", s)
		}
	}
}

func FuzzArithmeticReadsTheWholeRun(f *testing.F) {
	for _, s := range agreementSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !readsTheWholeRun(s) {
			t.Errorf("%q: the lexer stops inside the run shape judged arithmetic, with no error", s)
		}
	})
}
