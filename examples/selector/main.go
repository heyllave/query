// Example: @ selector operator — list-field predicates.
//
// Demonstrates @first / @last / @(inner) against realistic shapes:
//   - slices of map[string]any (JSON-ish records)
//   - slices of tagged structs (native Go records)
//
// Run:
//
//	go run ./examples/selector
package main

import (
	"fmt"

	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

// Order is a struct-backed element used inside @(...) predicates.
// Field names inside selector queries resolve via the `query` tag,
// the same contract as StructAccessor / CompileFor.
type Order struct {
	Status string  `query:"status"`
	Qty    int64   `query:"qty"`
	Price  float64 `query:"price"`
	SKU    string  `query:"sku"`
}

// fields declares both the list-valued containers and the element-scoped
// fields. Containers carry TextOps so they satisfy validation; actual
// per-element comparisons use the scalar fields below.
var fields = []validate.FieldConfig{
	// List containers — iterated by @first / @last / @(...).
	{Name: "orders", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},

	// Top-level scalars that compose with selectors.
	{Name: "customer", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},

	// Element-scoped fields resolved inside @(...).
	{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "qty", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "sku", Type: validate.TypeText, AllowedOps: validate.TextOps},
}

func main() {
	fmt.Println("=== @ Selector — map-backed elements ===")
	fmt.Println()

	records := []map[string]any{
		{
			"customer": "acme",
			"total":    750.0,
			"tags":     []any{"priority", "wholesale"},
			"orders": []any{
				map[string]any{"status": "shipped", "qty": int64(2), "price": 120.0},
				map[string]any{"status": "pending", "qty": int64(1), "price": 50.0},
			},
		},
		{
			"customer": "globex",
			"total":    250.0,
			"tags":     []any{},
			"orders": []any{
				map[string]any{"status": "cancelled", "qty": int64(1), "price": 30.0},
			},
		},
		{
			"customer": "initech",
			"total":    900.0,
			"tags":     []any{"priority"},
			"orders":   []any{},
		},
	}

	queries := []string{
		// @first — the list exists and has ≥ 1 element.
		"tags@first",

		// EXISTS semantics.
		"orders@(status=shipped)",

		// Numeric predicate against nested prices.
		"orders@(price>100)",

		// Compound inner condition.
		"orders@(status=shipped AND qty>=2)",

		// Composition with a top-level scalar.
		"orders@(status=shipped) AND total>500",

		// Negation binds outside the selector.
		"NOT orders@(status=cancelled)",

		// Disjunction of selectors.
		"orders@(status=shipped) OR orders@(status=delivered)",
	}

	for _, q := range queries {
		prog, err := eval.Compile(q, fields)
		if err != nil {
			fmt.Printf("  ERROR compiling %q: %v\n", q, err)
			continue
		}
		fmt.Printf("  Query: %s\n", q)
		for _, r := range records {
			fmt.Printf("    %-8s → %v\n", r["customer"], prog.Match(r))
		}
		fmt.Println()
	}

	fmt.Println("=== @ Selector — struct-backed elements ===")
	fmt.Println()

	structRecords := []map[string]any{
		{
			"customer": "acme",
			"total":    1200.0,
			"orders": []Order{
				{Status: "shipped", Qty: 2, Price: 300.0, SKU: "abc-123"},
				{Status: "pending", Qty: 1, Price: 50.0, SKU: "xyz-000"},
			},
		},
		{
			"customer": "globex",
			"total":    75.0,
			"orders": []*Order{
				{Status: "cancelled", Qty: 1, Price: 30.0, SKU: "abc-123"},
			},
		},
	}

	structQueries := []string{
		"orders@(sku=abc-123)",
		"orders@(price>=100)",
		"orders@(status=shipped) AND total>1000",
	}

	for _, q := range structQueries {
		prog, err := eval.Compile(q, fields)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			continue
		}
		fmt.Printf("  Query: %s\n", q)
		for _, r := range structRecords {
			fmt.Printf("    %-8s → %v\n", r["customer"], prog.Match(r))
		}
		fmt.Println()
	}

	fmt.Println("=== Round-trip ===")
	fmt.Println()

	for _, q := range queries {
		prog, err := eval.Compile(q, fields)
		if err != nil {
			continue
		}
		fmt.Printf("  %-55s → %s\n", q, prog.Stringify())
	}
}
