# query CLI

A debugging tool for the query language. Parses query expressions and visualizes
their AST structure, token streams, and validation results.

## Install

```bash
go install github.com/heyllave/query/cmd/query@latest
```

Or run directly:

```bash
go run ./cmd/query explain "<expression>"
```

## Commands

### `explain`

Parse a query expression and display its AST.

```bash
query explain "<expression>" [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `--json` | Emit AST as JSON (for piping / programmatic use) |
| `--tokens` | Print lexer tokens instead of AST (for lexer debugging) |
| `--schema <path>` | Validate against a JSON schema file |
| `--positions` | Include source position spans `[offset:length]` on each node |

## Examples

### Tree view (default)

```bash
$ query explain "status=active AND priority>3"
AndExpr
├── QualifierExpr (=)
│   ├── Field: status
│   └── Value: active (string)
└── QualifierExpr (>)
    ├── Field: priority
    └── Value: 3 (integer)
```

### Nested groups and NOT

```bash
$ query explain "(state=draft OR state=issued) AND NOT cancelled"
AndExpr
├── GroupExpr
│   └── OrExpr
│       ├── QualifierExpr (=)
│       │   ├── Field: state
│       │   └── Value: draft (string)
│       └── QualifierExpr (=)
│           ├── Field: state
│           └── Value: issued (string)
└── NotExpr
    └── PresenceExpr
        └── Field: cancelled
```

### Selectors

```bash
$ query explain "items@first"
SelectorExpr (@first)
└── PresenceExpr
    └── Field: items
```

### Range expressions

```bash
$ query explain "price:10..50"
QualifierExpr (..)
├── Field: price
├── Value: 10 (integer)
└── EndValue: 50 (integer)
```

### JSON output

```bash
$ query explain --json "status=active AND priority>3"
{
  "type": "BinaryExpr",
  "op": "AND",
  "children": [
    {
      "type": "QualifierExpr",
      "op": "=",
      "field": "status",
      "value": "active",
      "value_type": "string"
    },
    {
      "type": "QualifierExpr",
      "op": ">",
      "field": "priority",
      "value": "3",
      "value_type": "integer"
    }
  ]
}
```

### Token stream

```bash
$ query explain --tokens "status=active"
IDENT      status
=          =
STRING     active
EOF
```

### With positions

```bash
$ query explain --positions "status=active"
QualifierExpr (=) [0:6]
├── Field: status [0:6]
└── Value: active (string) [0:6]
```

### Schema validation

Create a schema file (`schema.json`):

```json
{
  "fields": [
    {
      "name": "status",
      "type": "text",
      "allowed_ops": ["=", "!=", "*"],
      "searchable": true
    },
    {
      "name": "priority",
      "type": "integer",
      "allowed_ops": ["=", "!=", ">", ">=", "<", "<="]
    }
  ]
}
```

```bash
# Valid query — prints tree
$ query explain --schema schema.json "status=active"

# Invalid field — prints validation error
$ query explain --schema schema.json "unknown=value"
```

### Error output

Parse errors include source-position pointers:

```bash
$ query explain "status="
error: expected value, got end of query
  status=
         ^

$ query explain "AND foo"
error: expected field name, got AND("AND")
  AND foo
  ^
```
