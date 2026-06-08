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
	{Name: "nickname", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "description", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "year", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "created_at", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	{Name: "tags", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "active", Type: validate.TypeBoolean, AllowedOps: validate.BoolOps},
}

func main() {
	data := map[string]any{
		"name":        "John Doe",
		"nickname":    "",
		"state":       "DRAFT",
		"description": "  urgent repair needed  ",
		"year":        2025,
		"total":       75000.50,
		"created_at":  time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		"tags":        "urgent,high-priority",
		"active":      true,
	}

	fmt.Println("=== Built-in String Functions ===")
	fmt.Println()
	runExample(data, "lower(state)=draft",
		"lower() — case-insensitive match (DRAFT → draft)")
	runExample(data, `upper(name)="JOHN DOE"`,
		"upper() — uppercase transform with quoted string value")
	runExample(data, `trim(description)="urgent repair needed"`,
		"trim() — strip whitespace, compare to quoted string")
	runExample(data, "len(name)>5",
		"len() — string length comparison")
	runExample(data, `contains(tags, "urgent")`,
		`contains(field, "literal") — string literal argument`)
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
		`currency(total)="75000.50 USD"`,
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
	fmt.Println("=== Functions in Value Position ===")
	fmt.Println()
	runExample(data, "created_at>=daysAgo(365)",
		"daysAgo() in value position — match dates within the last year")
	runExample(data, "created_at:daysAgo(365)..now()",
		"Functions on both sides of a date range")
	runExampleWithFuncs(data,
		"total>=threshold()",
		"Custom function returning a comparison value",
		eval.Func{
			Name: "threshold",
			Call: func(...any) (any, error) { return int64(10000), nil },
		},
	)

	fmt.Println()
	fmt.Println("=== Arithmetic in Value Position ===")
	fmt.Println()
	runExample(data, "total>=50000*1.1",
		"Multiplication with precedence (50000*1.1 = 55000)")
	runExample(data, "total>=(50000+1000)*1.1",
		"Parens override precedence ((51000)*1.1 = 56100)")
	runExample(data, "created_at>=now()-7d",
		"Date - duration → time within the last week")

	fmt.Println()
	fmt.Println("=== Ternary / Nullish via Built-in Functions ===")
	fmt.Println()
	runExample(data, `coalesce(name, "unknown")="John Doe"`,
		"coalesce() — first non-null/non-empty arg (SQL COALESCE)")
	runExample(data, `coalesce(nickname, name)="John Doe"`,
		"coalesce() — empty nickname falls through to name")
	runExample(data, `if(active, "on", "off")="on"`,
		"if(cond, a, b) — ternary; active=true picks the first branch")
	runExample(data, `if(active, "on", "off")="off"`,
		"if() — false branch is rejected, so this is false")

	fmt.Println()
	fmt.Println("=== Numeric Functions ===")
	fmt.Println()
	runExample(data, "abs(total)>=75000",
		"abs() — magnitude; preserves int/float kind")
	runExample(data, "ceil(total)>=75001",
		"ceil() — round up to a float64")
	runExample(data, "floor(total)=75000",
		"floor() — round down")
	runExample(data, "round(total)=75001",
		"round() — nearest, half away from zero (75000.50 → 75001)")
	runExample(data, "max(year, 2020)=2025",
		"max() — variadic; larger of a field and a literal")

	fmt.Println()
	fmt.Println("=== Time-Component Extractors ===")
	fmt.Println()
	runExample(data, "hour(created_at)=0",
		"hour() — 0..23 (created_at is midnight)")
	runExample(data, "weekday(created_at)=0",
		"weekday() — Sunday=0..Saturday=6 (2026-03-15 is a Sunday)")
	runExample(data, "year(addDays(created_at, 365))=2027",
		"addDays() — calendar-day arithmetic; a year later is 2027")
	fmt.Println("  [RECURRENCE] A schedule is a predicate over decomposed now():")
	fmt.Println("               weekday(now())>=1 AND weekday(now())<=5 AND hour(now())>=9")
	fmt.Println("               — \"weekdays from 9am\" — no cron string needed.")
	fmt.Println()

	fmt.Println("=== List Aggregations (value position) ===")
	fmt.Println()
	scores := eval.WithFunctions(eval.Func{
		Name: "scores", Call: func(...any) (any, error) { return []float64{80, 90, 100}, nil },
	})
	runValue(data, "count(scores())", "count() — element count → 3", scores)
	runValue(data, "sum(scores())", "sum() — total → 270", scores)
	runValue(data, "avg(scores())", "avg() — mean as float → 90", scores)
	runValue(data, "first(scores())", "first() — leading element → 80", scores)
	runValue(data, "last(scores())", "last() — trailing element → 100", scores)

	fmt.Println("=== Type Coercions (evaluation-time hints, not static casts) ===")
	fmt.Println()
	runValue(data, `int("42")`, "int() — parse string to int64", scores)
	runValue(data, `float("3.5")`, "float() — parse string to float64", scores)
	runValue(data, `string(year(created_at))`, "string() — render a value as text", scores)

	fmt.Println("=== Remaining Limitations ===")
	fmt.Println()
	fmt.Println("  [LIMITATION] No string concatenation — use a custom function (e.g.")
	fmt.Println("               full_name(first, last)) when you need to build strings.")
	fmt.Println("  [LIMITATION] No field refs as bare arithmetic operands — total>=base*1.1")
	fmt.Println("               doesn't work. Register a function: scaled(total)>50000.")
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

// runValue compiles and evaluates a value-returning query (eval.CompileValue),
// printing the computed value rather than a boolean match.
func runValue(data map[string]any, q, desc string, opts ...eval.Option) {
	prog, err := eval.CompileValue(q, fields, opts...)
	if err != nil {
		fmt.Printf("  %-45s  ERROR: %v\n", q, err)
		return
	}
	v, err := prog.Eval(data)
	if err != nil {
		fmt.Printf("  %-45s  ERROR: %v\n", q, err)
		return
	}
	fmt.Printf("  %-45s  → %v\n", q, v)
	fmt.Printf("    %s\n\n", desc)
}
