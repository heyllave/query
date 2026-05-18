// Example: Query restrictions — sandboxing queries for different contexts.
//
// Demonstrates WithAllowedFields, WithAllowedOps, WithMaxDepth, and
// WithMaxLength to restrict what users can query based on their role or context.
//
// Run:
//
//	go run ./examples/restrictions
package main

import (
	"fmt"

	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

var allFields = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "year", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "customer_id", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "internal_notes", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "created_at", Type: validate.TypeDate, AllowedOps: validate.DateOps},
}

func main() {
	fmt.Println("=== Field Restrictions ===")
	fmt.Println("  Scenario: public API only exposes state, total, year")
	fmt.Println()

	publicQueries := []string{
		"state=draft AND total>50000",
		"customer_id=cust_123",
		"internal_notes=*secret*",
	}
	for _, q := range publicQueries {
		_, err := eval.Compile(q, allFields,
			eval.WithAllowedFields("state", "total", "year"))
		status := "OK"
		if err != nil {
			status = fmt.Sprintf("BLOCKED: %v", err)
		}
		fmt.Printf("  %-40s  %s\n", q, status)
	}

	fmt.Println()
	fmt.Println("=== Operator Restrictions ===")
	fmt.Println("  Scenario: read-only search allows only = and !=")
	fmt.Println()

	opQueries := []string{
		"state=draft",
		"state!=cancelled",
		"total>50000",
		"year>=2020",
	}
	for _, q := range opQueries {
		_, err := eval.Compile(q, allFields,
			eval.WithAllowedOps(validate.OpEq, validate.OpNeq))
		status := "OK"
		if err != nil {
			status = fmt.Sprintf("BLOCKED: %v", err)
		}
		fmt.Printf("  %-40s  %s\n", q, status)
	}

	fmt.Println()
	fmt.Println("=== Depth Restrictions ===")
	fmt.Println("  Scenario: prevent deeply nested queries (DoS protection)")
	fmt.Println()

	depthQueries := []string{
		"state=draft",
		"state=draft AND total>50000",
		"(state=draft OR state=issued) AND total>50000",
		"((state=draft OR state=issued) AND total>50000) OR year=2026",
	}
	for _, q := range depthQueries {
		_, err := eval.Compile(q, allFields, eval.WithMaxDepth(3))
		status := "OK"
		if err != nil {
			status = fmt.Sprintf("BLOCKED: %v", err)
		}
		fmt.Printf("  %-65s  %s\n", q, status)
	}

	fmt.Println()
	fmt.Println("=== Length Restrictions ===")
	fmt.Println("  Scenario: limit query string length (default 256, configurable)")
	fmt.Println()

	_, errShort := eval.Compile("state=draft", allFields, eval.WithMaxLength(5))
	fmt.Printf("  %-40s  BLOCKED: %v\n", "state=draft (maxLength=5)", errShort)

	if _, err := eval.Compile("state=draft", allFields, eval.WithMaxLength(100)); err == nil {
		fmt.Printf("  %-40s  OK\n", "state=draft (maxLength=100)")
	}

	if _, err := eval.Compile("state=draft", allFields, eval.WithMaxLength(0)); err == nil {
		fmt.Printf("  %-40s  OK (length check disabled)\n", "state=draft (maxLength=0)")
	}
}
