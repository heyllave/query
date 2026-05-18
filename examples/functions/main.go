// Example: Built-in and custom functions in queries.
//
// Demonstrates every built-in function and how to register custom ones.
//
// Run:
//
//	go run ./examples/functions
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

var fields = []validate.FieldConfig{
	{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "description", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "year", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "created_at", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
}

func main() {
	data := map[string]any{
		"name":        "John Doe",
		"state":       "DRAFT",
		"description": "  urgent repair needed  ",
		"year":        2025,
		"total":       75000.50,
		"created_at":  time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		"tags":        "urgent,high-priority",
	}

	fmt.Println("=== Built-in String Functions ===")
	fmt.Println()
	runExample(data, "lower(state)=draft",
		"lower() — case-insensitive match (DRAFT → draft)")
	runExample(data, "upper(name)=JOHN DOE",
		"upper() — uppercase transform")
	runExample(data, "trim(description)=urgent repair needed",
		"trim() — strip whitespace")
	runExample(data, "len(name)>5",
		"len() — string length comparison")
	runExample(data, "contains(tags, urgent)",
		"contains(field, field) — check if one field's value contains another's")
	runExample(data, "startsWith(name, tags)",
		"startsWith(field, field) — prefix check (tags='urgent', name='John' → false)")

	fmt.Println()
	fmt.Println("=== Built-in Date Functions ===")
	fmt.Println()
	runExample(data, "year(created_at)=2026",
		"year() — extract year from date")
	runExample(data, "month(created_at)=3",
		"month() — extract month from date")
	runExample(data, "day(created_at)=15",
		"day() — extract day from date")

	fmt.Println()
	fmt.Println("=== Custom Functions ===")
	fmt.Println()
	runExampleWithFuncs(data,
		"wordCount(description)>2",
		"wordCount() — custom function counting words",
		eval.Func{
			Name: "wordCount",
			Call: func(args ...any) (any, error) {
				s := strings.TrimSpace(fmt.Sprint(args[0]))
				return int64(len(strings.Fields(s))), nil
			},
		},
	)

	runExampleWithFuncs(data,
		"currency(total)=75000.50 USD",
		"currency() — custom formatter (returns formatted string)",
		eval.Func{
			Name: "currency",
			Call: func(args ...any) (any, error) {
				return fmt.Sprintf("%.2f USD", args[0]), nil
			},
		},
	)

	runExampleWithFuncs(data,
		"domain(name)=doe",
		"domain() — custom extractor (last word after space)",
		eval.Func{
			Name: "domain",
			Call: func(args ...any) (any, error) {
				parts := strings.Fields(fmt.Sprint(args[0]))
				if len(parts) == 0 {
					return "", nil
				}
				return strings.ToLower(parts[len(parts)-1]), nil
			},
		},
	)

	fmt.Println()
	fmt.Println("=== Combining Functions with Logical Operators ===")
	fmt.Println()
	runExample(data, "lower(state)=draft AND len(name)>5",
		"Functions in compound expressions")
	runExample(data, "lower(state)=draft AND year(created_at)=2026",
		"Multiple functions in one query")
	runExample(data, "NOT lower(state)=published",
		"Function with NOT")

	fmt.Println()
	fmt.Println("=== Edge Cases & Limitations ===")
	fmt.Println()

	// Limitation: function args are field references, not string literals
	fmt.Println("  [LIMITATION] Function args are field references, not string literals.")
	fmt.Println("               contains(name, tags) compares two field values.")
	fmt.Println("               To search for a literal string, use wildcards: name=*urgent*")
	fmt.Println()

	// Limitation: no nested function-as-value yet
	fmt.Println("  [LIMITATION] Functions in value position (created_at>=now()) are not yet")
	fmt.Println("               supported. Use daysAgo() or compute the value before compiling.")
	fmt.Println()

	// Limitation: no arithmetic in functions
	fmt.Println("  [LIMITATION] No arithmetic expressions: total>50000*1.1 is not supported.")
	fmt.Println("               Register a custom function for computed comparisons.")
}

func runExample(data map[string]any, q, desc string) {
	prog, err := eval.Compile(q, fields)
	if err != nil {
		fmt.Printf("  %-45s  ERROR: %v\n", q, err)
		return
	}
	result := prog.Match(data)
	fmt.Printf("  %-45s  → %v\n", q, result)
	fmt.Printf("    %s\n\n", desc)
}

func runExampleWithFuncs(data map[string]any, q, desc string, funcs ...eval.Func) {
	prog, err := eval.Compile(q, fields, eval.WithFunctions(funcs...))
	if err != nil {
		fmt.Printf("  %-45s  ERROR: %v\n", q, err)
		return
	}
	result := prog.Match(data)
	fmt.Printf("  %-45s  → %v\n", q, result)
	fmt.Printf("    %s\n\n", desc)
}
