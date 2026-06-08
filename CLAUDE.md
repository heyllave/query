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
│   ├── lexer.go        # Lex(), ParseDuration()
│   ├── lexer_test.go
│   ├── parser.go       # Parse()
│   └── parser_test.go
├── validate/           # Field config and AST validation
│   ├── doc.go
│   ├── config.go       # FieldConfig, FieldValueType, Op, operator groups
│   ├── validate.go     # Validator, New(), Validate()
│   └── validate_test.go
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
