package query_test

import (
	"fmt"

	"github.com/heyllave/query"
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/validate"
)

func Example_parse() {
	expr, err := query.Parse("state=draft AND total>50000")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(ast.String(expr))
	// Output: state=draft AND total>50000
}

func Example_parseAndValidate() {
	fields := []validate.FieldConfig{
		{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	}

	expr, err := query.ParseAndValidate("state=draft AND total>50000", fields)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("fields:", len(ast.Fields(expr)))
	fmt.Println("simple:", ast.IsSimple(expr))
	// Output:
	// fields: 2
	// simple: false
}

func Example_fields() {
	expr, _ := query.Parse("(labels.dev=jane OR labels.env=bar) AND NOT cluster=demo")
	for _, fp := range ast.Fields(expr) {
		fmt.Println(fp.String())
	}
	// Output:
	// labels.dev
	// labels.env
	// cluster
}

func Example_roundTrip() {
	queries := []string{
		"state=draft",
		"name=John*",
		"NOT state=cancelled",
		"(state=draft OR state=issued) AND total>50000",
		"created_at:2026-01-01..2026-03-31",
	}
	for _, q := range queries {
		expr, _ := query.Parse(q)
		fmt.Println(ast.String(expr))
	}
	// Output:
	// state=draft
	// name=John*
	// NOT state=cancelled
	// (state=draft OR state=issued) AND total>50000
	// created_at:2026-01-01..2026-03-31
}
