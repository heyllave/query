// Command query provides a CLI for inspecting and debugging query expressions.
//
// Usage:
//
//	query explain "<expression>" [flags]
//
// Flags:
//
//	--json        emit AST as JSON
//	--tokens      print lexer tokens instead of AST
//	--schema      path to a JSON schema file for validation
//	--positions   include source position spans on each node
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/heyllave/query/output"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// schemaFile represents the JSON schema file format for --schema.
type schemaFile struct {
	Fields []schemaField `json:"fields"`
}

type schemaField struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	AllowedOps []string `json:"allowed_ops"`
	Searchable bool     `json:"searchable"`
	Nested     bool     `json:"nested"`
}

func parseFieldType(s string) (validate.FieldValueType, error) {
	switch s {
	case "text":
		return validate.TypeText, nil
	case "integer":
		return validate.TypeInteger, nil
	case "decimal":
		return validate.TypeDecimal, nil
	case "boolean":
		return validate.TypeBoolean, nil
	case "date":
		return validate.TypeDate, nil
	case "datetime":
		return validate.TypeDatetime, nil
	case "duration":
		return validate.TypeDuration, nil
	default:
		return 0, fmt.Errorf("unknown field type: %q", s)
	}
}

func parseOp(s string) (validate.Op, error) {
	switch s {
	case "=":
		return validate.OpEq, nil
	case "!=":
		return validate.OpNeq, nil
	case ">":
		return validate.OpGt, nil
	case ">=":
		return validate.OpGte, nil
	case "<":
		return validate.OpLt, nil
	case "<=":
		return validate.OpLte, nil
	case "..":
		return validate.OpRange, nil
	case "*":
		return validate.OpWildcard, nil
	case "?":
		return validate.OpPresence, nil
	default:
		return "", fmt.Errorf("unknown operator: %q", s)
	}
}

func loadSchema(path string) ([]validate.FieldConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema file: %w", err)
	}

	var sf schemaFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing schema file: %w", err)
	}

	configs := make([]validate.FieldConfig, 0, len(sf.Fields))
	for _, f := range sf.Fields {
		ft, err := parseFieldType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}

		ops := make([]validate.Op, 0, len(f.AllowedOps))
		for _, opStr := range f.AllowedOps {
			op, err := parseOp(opStr)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", f.Name, err)
			}
			ops = append(ops, op)
		}

		configs = append(configs, validate.FieldConfig{
			Name:       f.Name,
			Type:       ft,
			AllowedOps: ops,
			Searchable: f.Searchable,
			Nested:     f.Nested,
		})
	}

	return configs, nil
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}

	switch args[0] {
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func runExplain(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(stderr)

	jsonFlag := fs.Bool("json", false, "emit AST as JSON")
	tokensFlag := fs.Bool("tokens", false, "print lexer tokens instead of AST")
	schemaPath := fs.String("schema", "", "path to a JSON schema file for validation")
	positions := fs.Bool("positions", false, "include source position spans on each node")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "error: missing query expression")
		fmt.Fprintln(stderr, "usage: query explain \"<expression>\" [--json] [--tokens] [--schema <path>] [--positions]")
		return 1
	}

	query := fs.Arg(0)

	// --tokens mode: lex and print tokens
	if *tokensFlag {
		tokens, err := parser.Lex(query, 0)
		if err != nil {
			printErrors(query, err, stderr)
			return 1
		}
		for _, tok := range tokens {
			if *positions {
				fmt.Fprintf(stdout, "%-10s %-20s [%d:%d]\n", tok.Type, tok.Value, tok.Pos.Offset, tok.Pos.Length)
			} else {
				fmt.Fprintf(stdout, "%-10s %s\n", tok.Type, tok.Value)
			}
		}
		return 0
	}

	// Parse the query
	expr, err := parser.Parse(query, 0)
	if err != nil {
		printErrors(query, err, stderr)
		return 1
	}

	// --schema: validate against schema
	if *schemaPath != "" {
		fields, err := loadSchema(*schemaPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		v := validate.New(fields)
		if err := v.Validate(expr); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	// Build output options
	var opts []output.Option
	if *positions {
		opts = append(opts, output.WithPositions())
	}

	// --json mode
	if *jsonFlag {
		if err := output.Format(stdout, expr, output.JSONOutput, opts...); err != nil {
			fmt.Fprintf(stderr, "error: encoding JSON: %s\n", err)
			return 1
		}
		return 0
	}

	// Default: tree view
	if err := output.Format(stdout, expr, output.TreeOutput, opts...); err != nil {
		fmt.Fprintf(stderr, "error: rendering tree: %s\n", err)
		return 1
	}
	return 0
}

func printErrors(query string, err error, stderr *os.File) {
	errs := parser.Errors(err)
	if len(errs) == 0 {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return
	}

	for _, e := range errs {
		fmt.Fprintf(stderr, "error: %s\n", e.Message)
		fmt.Fprintf(stderr, "  %s\n", query)

		// Build pointer line
		offset := e.Position.Offset
		if offset > len(query) {
			offset = len(query)
		}
		pointer := strings.Repeat(" ", offset) + "^"
		fmt.Fprintf(stderr, "  %s\n", pointer)
	}
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, "Usage: query <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  explain    Parse a query expression and visualize the AST")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'query explain --help' for flag details.")
}
