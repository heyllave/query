---
title: Command line
nav_order: 6.5
permalink: /cli/
---

# Command line
{: .no_toc }

1. TOC
{:toc}

`query explain` shows how a query is read: its tree, its tokens, or the errors it produces. Use it to debug a query or to see why one was rejected.

## Run it

```bash
go run github.com/heyllave/query/cmd/query@latest explain "status=active AND priority>3"
```

Or install it once:

```bash
go install github.com/heyllave/query/cmd/query@latest
query explain "status=active AND priority>3"
```

```
AndExpr
├── QualifierExpr (=)
│   ├── Field: status
│   └── Value: active (string)
└── QualifierExpr (>)
    ├── Field: priority
    └── Value: 3 (integer)
```

Quote the query so the shell does not interpret `>`, `*` or parentheses.

## Flags

Flags go before the query. A flag written after the query is ignored.

| Flag | What it prints |
|---|---|
| `--tokens` | The lexer's tokens instead of the tree. |
| `--json` | The tree as JSON. |
| `--positions` | The tree with each node's `[offset:length]` in the query. |
| `--schema <path>` | The tree, after validating the query against a field list. |

### Tokens

```bash
query explain --tokens "status=active AND priority>3"
```

```
IDENT      status
=          =
STRING     active
AND        AND
IDENT      priority
>          >
INTEGER    3
EOF
```

This is the quickest way to see whether a value was read as text or as a number.

### JSON

```bash
query explain --json "status=active"
```

```json
{
  "type": "QualifierExpr",
  "op": "=",
  "field": "status",
  "value": "active",
  "value_type": "string"
}
```

This JSON is a debugging view. It is not the tree the TypeScript client and the C library exchange; that one is described by [`clients/npm/src/types.ts`](https://github.com/heyllave/query/blob/main/clients/npm/src/types.ts).

## Errors

A syntax error prints its code and points at the character. The command exits with status 1.

```bash
query explain "status==active"
```

```
error [unexpectedChar]: unexpected "=" where a value belongs: operators are single (=, !=, >, >=, <, <=); quote the value if it starts with "="
  status==active
         ^
```

## Validate against a field list

`--schema` takes a JSON file that lists the fields a query may use.

```json
{
  "fields": [
    {"name": "status", "type": "text", "allowed_ops": ["=", "!="]},
    {"name": "priority", "type": "integer", "allowed_ops": ["=", ">", ">=", "<", "<="]}
  ]
}
```

```bash
query explain --schema fields.json "status=act* AND nope=1"
```

```
position 0: operator "*" is not allowed for field "status" (type text); position 16: unknown field "nope"
```

- `type` is `text`, `integer`, `decimal`, `boolean`, `date`, `datetime` or `duration`.
- `allowed_ops` holds `=`, `!=`, `>`, `>=`, `<`, `<=`, `..` (range), `*` (wildcard) and `?` (presence).
- A field with no `allowed_ops` allows no operators. This differs from the TypeScript client and the C library, where a field without a list gets the usual operators for its type.
- `nested: true` accepts any key under the field, such as `labels.team`.
