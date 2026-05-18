package eval

import (
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

// Eval-side tests for batch-2 parity additions: arithmetic-valued comparisons,
// @all / @none selectors, implicit AND, and the coalesce()/if() builtins.

func TestEval_Arith_NumericMul(t *testing.T) {
	prog, err := Compile("total>=50000*1.1", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// 50000 * 1.1 = 55000.
	if !prog.Match(map[string]any{"total": 60000.0}) {
		t.Error("60000 should be >= 55000")
	}
	if prog.Match(map[string]any{"total": 50000.0}) {
		t.Error("50000 should not be >= 55000")
	}
}

func TestEval_Arith_NowMinusDuration(t *testing.T) {
	prog, err := Compile("created_at>=now()-7d", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	recent := time.Now().Add(-3 * 24 * time.Hour)
	old := time.Now().Add(-30 * 24 * time.Hour)
	if !prog.Match(map[string]any{"created_at": recent}) {
		t.Error("3 days ago should be >= now()-7d")
	}
	if prog.Match(map[string]any{"created_at": old}) {
		t.Error("30 days ago should not be >= now()-7d")
	}
}

func TestEval_Arith_ParenPrecedence(t *testing.T) {
	// (50000+1000) * 2 = 102000 (exact integer arithmetic). Plain
	// 50000+1000*2 would be 52000. Picking integers avoids IEEE rounding noise.
	prog, err := Compile("total>=(50000+1000)*2", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"total": 102000.0}) {
		t.Error("102000 should be >= 102000")
	}
	if prog.Match(map[string]any{"total": 52000.0}) {
		t.Error("52000 should not match (proves parens were honored)")
	}
}

func TestEval_Arith_DivisionByZero(t *testing.T) {
	// Division by zero should fall back to false (resolver returns nil).
	prog, err := Compile("total>=10/0", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if prog.Match(map[string]any{"total": 5.0}) {
		t.Error("comparison against div-by-zero should not match")
	}
}

func TestEval_Arith_DurationMultiplyByNumber(t *testing.T) {
	// `now()-7d*2` should resolve to "now minus 14 days".
	prog, err := Compile("created_at>=now()-7d*2", parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tenDays := time.Now().Add(-10 * 24 * time.Hour)
	twentyDays := time.Now().Add(-20 * 24 * time.Hour)
	if !prog.Match(map[string]any{"created_at": tenDays}) {
		t.Error("10 days ago should be >= now()-14d")
	}
	if prog.Match(map[string]any{"created_at": twentyDays}) {
		t.Error("20 days ago should not be >= now()-14d")
	}
}

// --- @all / @none selectors --------------------------------------------------

var selectorParityFields = []validate.FieldConfig{
	{Name: "orders", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
	// Inner field declared at top-level — selectors validate their inner
	// expression against the same registry; element-scoped lookup happens at
	// match time via elementAccessor.
	{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
}

func TestEval_SelectorAll_AllSatisfy(t *testing.T) {
	prog, err := Compile("orders@all(status=shipped)", selectorParityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	allShipped := []map[string]any{
		{"status": "shipped"},
		{"status": "shipped"},
		{"status": "shipped"},
	}
	someShipped := []map[string]any{
		{"status": "shipped"},
		{"status": "pending"},
	}
	if !prog.Match(map[string]any{"orders": allShipped}) {
		t.Error("all-shipped should satisfy @all")
	}
	if prog.Match(map[string]any{"orders": someShipped}) {
		t.Error("partial-shipped should not satisfy @all")
	}
}

func TestEval_SelectorAll_EmptyListVacuous(t *testing.T) {
	prog, err := Compile("orders@all(status=shipped)", selectorParityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// Vacuous truth: an empty list trivially satisfies @all.
	if !prog.Match(map[string]any{"orders": []map[string]any{}}) {
		t.Error("empty list should satisfy @all by vacuous truth")
	}
}

func TestEval_SelectorNone(t *testing.T) {
	prog, err := Compile("orders@none(status=cancelled)", selectorParityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	noneCancelled := []map[string]any{
		{"status": "shipped"},
		{"status": "pending"},
	}
	someCancelled := []map[string]any{
		{"status": "shipped"},
		{"status": "cancelled"},
	}
	if !prog.Match(map[string]any{"orders": noneCancelled}) {
		t.Error("no-cancelled should satisfy @none")
	}
	if prog.Match(map[string]any{"orders": someCancelled}) {
		t.Error("any-cancelled should not satisfy @none")
	}
}

func TestEval_SelectorAny_AliasOfAnonymous(t *testing.T) {
	// @any(p) ≡ @(p).
	for _, q := range []string{"orders@any(status=shipped)", "orders@(status=shipped)"} {
		prog, err := Compile(q, selectorParityFields)
		if err != nil {
			t.Fatalf("%s: compile: %v", q, err)
		}
		data := map[string]any{"orders": []map[string]any{{"status": "shipped"}, {"status": "pending"}}}
		if !prog.Match(data) {
			t.Errorf("%s: expected match", q)
		}
	}
}

// --- Implicit AND ------------------------------------------------------------

func TestEval_ImplicitAnd_Equivalent(t *testing.T) {
	// Implicit AND should evaluate identically to explicit AND.
	q1, err := Compile("state=draft year>2020", parityFields)
	if err != nil {
		t.Fatalf("compile implicit: %v", err)
	}
	q2, err := Compile("state=draft AND year>2020", parityFields)
	if err != nil {
		t.Fatalf("compile explicit: %v", err)
	}
	cases := []map[string]any{
		{"state": "draft", "year": 2025},
		{"state": "draft", "year": 2010},
		{"state": "issued", "year": 2025},
		{},
	}
	for _, data := range cases {
		if q1.Match(data) != q2.Match(data) {
			t.Errorf("data=%v: implicit %v != explicit %v", data, q1.Match(data), q2.Match(data))
		}
	}
}

// --- coalesce / if -----------------------------------------------------------

func TestEval_Coalesce_PicksFirstNonEmpty(t *testing.T) {
	prog, err := Compile(`coalesce(name, description, "fallback")="John"`, parityFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"name": "John"}) {
		t.Error("non-empty name should be picked")
	}
	if !prog.Match(map[string]any{"name": "", "description": "John"}) {
		t.Error("empty name should fall through to description")
	}
	if prog.Match(map[string]any{"name": "", "description": ""}) {
		t.Error(`empty both should fall through to "fallback", not match "John"`)
	}
}

func TestEval_If_TernarySelectsBranch(t *testing.T) {
	// `if(active, "on", "off")` — uses the field `active` (bool) as predicate.
	// Quoted string equality on the result.
	prog, err := Compile(`if(active, "on", "off")="on"`, []validate.FieldConfig{
		{Name: "active", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !prog.Match(map[string]any{"active": true}) {
		t.Error("active=true should pick the 'on' branch")
	}
	if prog.Match(map[string]any{"active": false}) {
		t.Error("active=false should pick the 'off' branch")
	}
}
