// Command rules shows how to derive rules with the query language: "do X when
// condition A holds, do Y when condition B holds". A rule is a compiled
// predicate ([eval.Program]) paired with a consumer-owned action. The library
// provides the matching engine; the policy — first-match vs collect-all,
// ordering, what an action is — lives in the consumer, where it belongs.
//
// Three patterns are shown:
//   - first-match: the highest-priority matching rule wins (routing, tiering)
//   - collect-all: every matching rule fires (tagging, alerting)
//   - hybrid: a predicate selects the rule, a value query computes its output
//
// Run with:
//
//	go run .
package main

import (
	"fmt"
	"log"

	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

// fields describes the records the rules run against. The same config is shared
// by every rule so predicates validate against one schema.
var fields = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "country", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "items", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
}

// Rule pairs a compiled predicate with an action of any type. The action is
// whatever the consumer needs — here a struct; it could be a string, an enum,
// or a func.
type Rule[A any] struct {
	Name string
	When *eval.Program
	Then A
}

// Routing is the action for the routing ruleset: where an order goes and at
// what priority.
type Routing struct {
	Queue    string
	Priority int
}

// mustRule compiles a predicate or fails fast at startup — rules are authored
// once, not per request.
func mustRule[A any](name, predicate string, action A) Rule[A] {
	prog, err := eval.Compile(predicate, fields)
	if err != nil {
		log.Fatalf("rule %q: %v", name, err)
	}
	return Rule[A]{Name: name, When: prog, Then: action}
}

func main() {
	firstMatch()
	collectAll()
	hybrid()
	nestedIfNote()
}

// firstMatch: rules are tried in order; the first whose predicate matches wins.
// Order encodes priority — most specific first, ending in a catch-all.
func firstMatch() {
	section("first-match routing (priority order)")
	rules := []Rule[Routing]{
		mustRule("vip-eu", `total>10000 AND country IN (de, fr, es)`, Routing{"vip-eu", 100}),
		mustRule("vip", `total>10000`, Routing{"vip", 90}),
		mustRule("bulk", `items>=50`, Routing{"bulk", 50}),
		mustRule("default", `total>=0`, Routing{"standard", 0}), // catch-all
	}

	orders := []map[string]any{
		{"total": 25000.0, "country": "de", "items": 3},
		{"total": 25000.0, "country": "us", "items": 3},
		{"total": 200.0, "country": "us", "items": 80},
		{"total": 49.0, "country": "us", "items": 1},
	}
	for _, o := range orders {
		route := Routing{"none", -1}
		matched := "—"
		for _, r := range rules {
			if r.When.Match(o) {
				route = r.Then
				matched = r.Name
				break // first match wins
			}
		}
		fmt.Printf("  total=%-8v country=%-3v items=%-3v -> %-9s (rule %s, prio %d)\n",
			o["total"], o["country"], o["items"], route.Queue, matched, route.Priority)
	}
}

// collectAll: every matching rule fires. Used for tagging, alerting, or any
// "apply all that apply" policy.
func collectAll() {
	section("collect-all tagging (every match fires)")
	rules := []Rule[string]{
		mustRule("high-value", `total>1000`, "high-value"),
		mustRule("international", `NOT country=us`, "international"),
		mustRule("draft", `state=draft`, "needs-review"),
		mustRule("bulk", `items>=50`, "bulk"),
	}

	order := map[string]any{"total": 5000.0, "country": "fr", "state": "draft", "items": 10}
	var tags []string
	for _, r := range rules {
		if r.When.Match(order) {
			tags = append(tags, r.Then)
		}
	}
	fmt.Printf("  order %v\n  tags: %v\n", order, tags)
}

// hybrid: a predicate selects the rule, and a value query computes the rule's
// output from the record — the discount amount here. Selection uses
// [eval.Program]; the output uses [eval.ValueProgram].
//
// Field references are not bare arithmetic operands (total*0.20 does not
// parse — * after a field is a wildcard, and field-refs-in-arithmetic are out
// of scope), so the rate is applied through a registered discount(total, rate)
// function — the URL-safe way to compute from a field.
func hybrid() {
	section("hybrid: predicate selects, value query computes")
	discountFn := eval.WithFunctions(eval.Func{
		Name: "discount",
		Call: func(args ...any) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("discount() requires 2 arguments, got %d", len(args))
			}
			return toF(args[0]) * toF(args[1]), nil
		},
	})
	type Discount struct {
		Name string
		When *eval.Program
		Amt  *eval.ValueProgram
	}
	mustVal := func(q string) *eval.ValueProgram {
		p, err := eval.CompileValue(q, fields, discountFn)
		if err != nil {
			log.Fatalf("value %q: %v", q, err)
		}
		return p
	}
	rules := []Discount{
		{"vip", mustCompile(`total>10000`), mustVal(`discount(total, 0.20)`)},
		{"loyal", mustCompile(`total>1000`), mustVal(`discount(total, 0.10)`)},
		{"base", mustCompile(`total>=0`), mustVal(`discount(total, 0.05)`)},
	}

	for _, total := range []float64{25000, 5000, 200} {
		rec := map[string]any{"total": total}
		for _, r := range rules {
			if r.When.Match(rec) {
				amount, err := r.Amt.Eval(rec)
				if err != nil {
					log.Fatalf("amount eval: %v", err)
				}
				fmt.Printf("  total=%-8v -> %-6s discount = %v\n", total, r.Name, amount)
				break
			}
		}
	}
}

// toF coerces a discount() argument to float64.
func toF(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func mustCompile(q string) *eval.Program {
	p, err := eval.Compile(q, fields)
	if err != nil {
		log.Fatalf("compile %q: %v", q, err)
	}
	return p
}

// nestedIfNote documents a real constraint: the if() built-in chooses between
// values, but its condition is a boolean-valued argument (a field or function),
// NOT a full comparison. A comparison like state=draft uses the operator
// grammar, which is not available inside a function-argument list — so
// if(state=draft, ...) does not parse. For comparison-driven branching, use a
// ruleset of eval.Program predicates (above); use if() for truthy-field
// branching within a value expression.
func nestedIfNote() {
	section("if() branches on a truthy value, not a comparison")

	// Works: the condition is a boolean-valued field.
	prog, err := eval.CompileValue(`if(active, "on", "off")`, []validate.FieldConfig{
		{Name: "active", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
	})
	if err != nil {
		log.Fatalf("if() compile: %v", err)
	}
	on, _ := prog.Eval(map[string]any{"active": true})
	off, _ := prog.Eval(map[string]any{"active": false})
	fmt.Printf("  if(active, on, off): active=true -> %v, active=false -> %v\n", on, off)

	// Does not parse: a comparison inside the argument list.
	if _, err := eval.CompileValue(`if(state=draft, "review", "ok")`, fields); err != nil {
		fmt.Printf("  if(state=draft, ...) -> rejected: %v\n", firstLine(err))
		fmt.Println("  -> use a ruleset of predicates for comparison-driven branching.")
	}
}

func section(name string) { fmt.Printf("\n# %s\n", name) }

func firstLine(err error) string {
	s := err.Error()
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
