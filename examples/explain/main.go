// Example: AST inspection and debugging.
//
// Demonstrates how to parse a query expression and inspect its AST
// using the output package for tree and JSON rendering, plus the
// library's AST utilities for field extraction and depth analysis.
//
// Run:
//
//	go run ./examples/explain
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/output"
	"github.com/heyllave/query/parser"
)

func main() {
	queries := []string{
		"status=active AND priority>3",
		"(state=draft OR state=issued) AND NOT cancelled",
		"items@first",
		"price:10..50",
	}

	for _, q := range queries {
		fmt.Printf("Query: %s\n", q)
		fmt.Println(strings.Repeat("─", 60))

		// 1. Lex — show the token stream
		tokens, err := parser.Lex(q, 0)
		if err != nil {
			fmt.Printf("  Lex error: %v\n\n", err)
			continue
		}
		fmt.Print("  Tokens: ")
		for i, tok := range tokens {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(tok)
		}
		fmt.Println()

		// 2. Parse — build the AST
		expr, err := parser.Parse(q, 0)
		if err != nil {
			fmt.Printf("  Parse error: %v\n\n", err)
			continue
		}

		// 3. AST utilities
		fmt.Printf("  Fields:  %v\n", fieldNames(ast.Fields(expr)))
		fmt.Printf("  Depth:   %d\n", ast.Depth(expr))
		fmt.Printf("  Simple:  %v\n", ast.IsSimple(expr))
		fmt.Printf("  String:  %s\n", ast.String(expr))

		// 4. Tree view — using output.AsTree
		fmt.Println("  Tree:")
		tree, _ := output.AsTree(expr)
		for _, line := range strings.Split(strings.TrimRight(string(tree), "\n"), "\n") {
			fmt.Printf("    %s\n", line)
		}

		// 5. JSON view — using output.AsJSON
		fmt.Println("  JSON:")
		jsonData, _ := output.AsJSON(expr)
		for _, line := range strings.Split(strings.TrimRight(string(jsonData), "\n"), "\n") {
			fmt.Printf("    %s\n", line)
		}

		fmt.Println()
	}

	// Demonstrate custom formatter
	fmt.Println("Custom formatter (node count)")
	fmt.Println(strings.Repeat("─", 60))
	for _, q := range queries {
		expr, _ := parser.Parse(q, 0)
		var count int
		ast.Walk(expr, func(_ ast.Expression) bool {
			count++
			return true
		})
		fmt.Printf("  %-55s → %d nodes\n", q, count)
	}
	fmt.Println()

	// Demonstrate error formatting with source pointers
	fmt.Println("Error formatting")
	fmt.Println(strings.Repeat("─", 60))
	badQueries := []string{"AND foo", "status=", "$invalid"}
	for _, q := range badQueries {
		_, err := parser.Parse(q, 0)
		if err == nil {
			continue
		}
		for _, e := range parser.Errors(err) {
			fmt.Printf("  error: %s\n", e.Message)
			fmt.Printf("    %s\n", q)
			offset := e.Position.Offset
			if offset > len(q) {
				offset = len(q)
			}
			fmt.Printf("    %s^\n", strings.Repeat(" ", offset))
		}
	}

	// Demonstrate output.Format writing to a writer
	fmt.Println()
	fmt.Println("output.Format → os.Stdout")
	fmt.Println(strings.Repeat("─", 60))
	expr, _ := parser.Parse("status=active AND priority>3", 0)
	_ = output.Format(os.Stdout, expr, output.TreeOutput)
}

func fieldNames(fps []ast.FieldPath) []string {
	names := make([]string, len(fps))
	for i, fp := range fps {
		names[i] = fp.String()
	}
	return names
}
