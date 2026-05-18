// Example: Struct binding — compile-time type safety.
//
// Demonstrates CompileFor[T] which infers field names and types from
// Go struct tags, and MatchStruct for type-safe evaluation.
//
// Run:
//
//	go run ./examples/struct
package main

import (
	"fmt"
	"time"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

// Invoice represents a billing document. Fields tagged with `query:"name"`
// become queryable. Untagged or `query:"-"` fields are invisible to queries.
type Invoice struct {
	ID        string    `query:"-"`          // excluded from queries
	State     string    `query:"state"`      // → TypeText, TextOps
	Total     float64   `query:"total"`      // → TypeDecimal, NumericOps
	Year      int       `query:"year"`       // → TypeInteger, NumericOps
	Active    bool      `query:"active"`     // → TypeBoolean, BoolOps
	CreatedAt time.Time `query:"created_at"` // → TypeDate, DateOps
	Notes     string    `query:"notes"`      // → TypeText, TextOps
	internal  string    //nolint:unused       // unexported — not accessible
}

func main() {
	fmt.Println("=== Struct Field Discovery ===")
	fmt.Println()

	fields := eval.FieldsFromStruct(Invoice{})
	for _, f := range fields {
		fmt.Printf("  %-12s type=%-10s ops=%v\n", f.Name, f.Type, opsNames(f.AllowedOps))
	}

	fmt.Println()
	fmt.Println("=== CompileFor[Invoice] ===")
	fmt.Println()

	invoices := []Invoice{
		{ID: "INV-001", State: "draft", Total: 75000, Year: 2026, Active: true,
			CreatedAt: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), Notes: "Urgent repair"},
		{ID: "INV-002", State: "issued", Total: 12000, Year: 2025, Active: true,
			CreatedAt: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC), Notes: "Routine"},
		{ID: "INV-003", State: "cancelled", Total: 50000, Year: 2026, Active: false,
			CreatedAt: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), Notes: "Voided"},
		{ID: "INV-004", State: "draft", Total: 200000, Year: 2026, Active: true,
			CreatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Notes: "Large order"},
	}

	queries := []string{
		"state=draft",
		"state=draft AND total>50000",
		"year=2026 AND active=true",
		"NOT state=cancelled",
		"total>=50000 AND total<=100000",
		"notes=*urgent*",
	}

	for _, q := range queries {
		prog, err := eval.CompileFor[Invoice](q)
		if err != nil {
			fmt.Printf("  ERROR compiling %q: %v\n", q, err)
			continue
		}

		fmt.Printf("  Query: %s\n", q)
		fmt.Printf("  Fields: %v\n", fieldNames(prog.Fields()))
		for _, inv := range invoices {
			if prog.MatchStruct(inv) {
				fmt.Printf("    ✓ %s (state=%s total=%.0f year=%d)\n",
					inv.ID, inv.State, inv.Total, inv.Year)
			}
		}
		fmt.Println()
	}

	fmt.Println("=== Type Safety — Compile-Time Errors ===")
	fmt.Println()

	badQueries := []struct {
		q    string
		desc string
	}{
		{"ID=INV-001", "ID field excluded with query:\"-\""},
		{"internal=secret", "unexported field not accessible"},
		{"nonexistent=value", "field doesn't exist on struct"},
		{"year=notanumber", "type mismatch: string for integer field"},
		{"active=maybe", "type mismatch: string for boolean field"},
	}

	for _, bad := range badQueries {
		_, err := eval.CompileFor[Invoice](bad.q)
		if err != nil {
			fmt.Printf("  ✓ %s\n    → %v\n    (%s)\n\n", bad.q, err, bad.desc)
		} else {
			fmt.Printf("  ✗ %s — expected error (%s)\n\n", bad.q, bad.desc)
		}
	}

	fmt.Println("=== MatchFunc — Custom Accessor ===")
	fmt.Println()

	prog, _ := eval.CompileFor[Invoice]("state=draft")

	// You can also use MatchFunc with a custom accessor for edge cases
	// (e.g., nested maps, computed fields)
	customMatch := prog.MatchFunc(func(field string) (any, bool) {
		switch field {
		case "state":
			return "draft", true
		default:
			return nil, false
		}
	})
	fmt.Printf("  Custom accessor: state=draft → %v\n", customMatch)
}

func opsNames(ops []validate.Op) []string {
	names := make([]string, len(ops))
	for i, op := range ops {
		names[i] = string(op)
	}
	return names
}

func fieldNames(fps []ast.FieldPath) []string {
	names := make([]string, len(fps))
	for i, fp := range fps {
		names[i] = fp.String()
	}
	return names
}
