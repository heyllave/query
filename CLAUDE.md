# Query Library — Claude Code Guidelines

## Overview

Pure Go query language library for Trazo. Handles lexing, parsing, AST construction,
and validation of a unified query syntax. Zero external dependencies (stdlib only).
WASM-compilable for client-side use.

## Tech Stack

| Library | Purpose |
|---------|---------|
| Go 1.25+ | Standard library only — no third-party deps |

## Project Structure

```
query/
├── doc.go              # Package-level documentation
├── query.go            # Public API: Parse(), Validate(), ParseAndValidate()
├── example_test.go     # Godoc examples (SQL, JSON, React, filter function)
├── token/              # Lexical token types
│   ├── doc.go
│   └── token.go        # Position, Type, Token, OperatorSymbol
├── ast/                # Abstract syntax tree
│   ├── doc.go
│   ├── node.go         # Expression interface, all concrete node types
│   ├── value.go        # Value, ValueType, FieldPath
│   ├── visitor.go      # Visitor[T] generic interface, Visit[T], SQLOperator, WildcardToLike
│   ├── walk.go         # Walk, Fields, Qualifiers, IsSimple, Depth
│   ├── string.go       # String() — AST back to query string
│   └── walk_test.go
├── parser/             # Lexer and parser
│   ├── doc.go
│   ├── errors.go       # Error, ErrorKind, ErrorList
│   ├── lexer.go        # lexer state, Lex(), LexValue(), the main loop
│   ├── lex_mode.go     # what comes next (mode) and what each open '(' means; every change of mode
│   ├── lex_field.go    # field position: names, keywords, operators, parens, lists
│   ├── lex_value.go    # after an operator: quoted, bracketed and bare values
│   ├── lex_number.go   # number, date and duration runs; classifyValue
│   ├── lex_arith.go    # arithmetic values and function-call arguments
│   ├── lex_quoted.go   # quoted strings and [field] references
│   ├── literals.go     # ParseDateLiteral(), ParseDuration(), IsDayLiteral()
│   ├── lexer_golden_test.go  # pins the lexer's answers for ~5,700 inputs
│   ├── internal/
│   │   ├── chars/      # one character: classes, spaces, identifiers, quotes
│   │   ├── literal/    # one whole literal: dates, durations, numbers, patterns
│   │   └── shape/      # is an unquoted value arithmetic or text (the lookahead)
│   ├── parser.go       # Parse()
│   ├── cursor.go       # CursorAt(): what the grammar expects at a position
│   └── parser_test.go
├── validate/           # Field config and AST validation
│   ├── doc.go
│   ├── config.go       # FieldConfig, FieldValueType, Op, operator groups
│   ├── validate.go     # Validator, New(), Validate()
│   └── validate_test.go
├── complete/           # Completion for a query being typed
│   └── complete.go     # At(), Completion, Offer(), ValueText(), FieldText()
└── wasm/               # WASM build target
    ├── main.go
    └── Makefile
```

## Architecture Decisions — DO NOT DEVIATE

1. **Zero external dependencies** — stdlib only. No testify, no third-party.
2. **Sub-package structure** — token, ast, parser, validate are separate packages.
3. **WASM-compatible** — no OS-specific code in the library.
4. **Consumers own code generation** — use ast.Visitor[T] to transform AST.

## Code Conventions

### Small functions in the lexer

Functions in `parser/lex*.go` and `parser/internal/` stay at 40 lines or less
(`funlen` in `.golangci.yml` enforces it). A rule that does not need lexer
state goes in `internal/chars`, `internal/literal` or `internal/shape`, pure
on a string and an index, with a table test. A change to what the lexer
produces shows up in `TestLexer_Golden`; regenerate it with
`go test ./parser -run Golden -update` only when the change is meant.

### Import Order

```go
import (
    // 1. Standard library
    "fmt"
    "strings"

    // 2. Internal packages
    "github.com/heyllave/query/ast"
    "github.com/heyllave/query/token"
)
```

### Test Naming

- `Test<Function>_<Scenario>` with `t.Run` subtests.
- AAA pattern: Arrange, Act, Assert.
- Table-driven for all token types, operators, grammar productions.
- Use stdlib `testing` only — no testify.

## Commit Format

```
<type>(<scope>): <description>
```

- Types: `feat`, `fix`, `refactor`, `test`, `chore`, `docs`
- Scopes: `token`, `ast`, `parser`, `validate`, `api`, `wasm`, `ci`
- No `Co-authored-by` for Claude.

## Common Commands

```bash
make build      # compile all packages
make test       # unit tests with -race
make lint       # golangci-lint
make fmt        # gofmt + goimports
make vet        # go vet
make check      # fmt + vet + lint + test
make coverage   # test with coverage report
make wasm       # build WASM target
```
