---
title: Command line
nav_order: 4
parent: Clients
permalink: /cli/
---

# Command line
{: .no_toc }

1. TOC
{:toc}

`query explain` shows how a query is read: its tree, its tokens, or the errors it produces. Use it to debug a query or to see why one was rejected. `query filter`, `query eval` and `query map` run a query over JSON records from a pipe, the way `jq` does.

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

An error prints its code, marks the offending text and, when the engine can tell what was meant, adds a hint. The command exits with status 1.

```bash
query explain "status==active"
```

```
error [unexpectedChar]: unexpected "=" where a value belongs
  status==active
         ^
  hint: equality is a single =: status=active
```

```bash
query explain "[base]*1.1 > 3"
```

```
error [unexpected]: unexpected "*" after a complete condition
  [base]*1.1 > 3
        ^
  hint: arithmetic goes on the right of the operator or inside a function: float([base]*1.1)>3
```

[Errors and hints]({{ '/errors/' | relative_url }}) lists what the hints cover.

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
query explain --schema fields.json "statsu=active AND priority>high AND status=act*"
```

```
error [fieldNotFound]: unknown field "statsu"
  statsu=active AND priority>high AND status=act*
  ^~~~~~
  hint: did you mean "status"?
error [typeMismatch]: value "high" (string) does not fit field "priority" (type integer)
  statsu=active AND priority>high AND status=act*
                    ^~~~~~~~
  hint: a number looks like 42, -3 or 1.5
error [operatorNotAllowed]: operator "*" is not allowed for field "status" (type text)
  statsu=active AND priority>high AND status=act*
                                      ^~~~~~
  hint: status cannot be matched with a * pattern; it allows: =, !=
```

- `type` is `text`, `integer`, `decimal`, `boolean`, `date`, `datetime` or `duration`.
- `allowed_ops` holds `=`, `!=`, `>`, `>=`, `<`, `<=`, `..` (range), `*` (wildcard) and `?` (presence).
- A field with no `allowed_ops` allows no operators. This differs from the TypeScript client and the C library, where a field without a list gets the usual operators for its type.
- `nested: true` accepts any key under the field, such as `labels.team`.

## Filter, evaluate and reshape records

`filter`, `eval` and `map` read JSON records from stdin and write one result per record. Records are one object per line, or one array of objects; the output has the same shape as the input.

```bash
query filter 'state=draft AND total>50000' < events.jsonl    # the records that match, unchanged
query eval '[total]*1.1' < events.jsonl                       # one value per record
query map 'id, total=[total]*1.1, team=[labels.team]' < events.jsonl   # one object per record
```

`filter` takes a condition, the same one `match` evaluates in Go, in the browser or in C. `eval` takes a value expression, as `eval` does there. `map` takes a projection: entries separated by commas, each `name=<value expression>`, or a bare field path, which stands for itself. Keys keep the projection's order. A value the record does not have is `null`.

### Field types

Without `--schema`, the fields a query names are typed from the records: a number is decimal, `true` or `false` boolean, a string that reads as a date is a datetime, an object is a nested field, anything else is text. The first records are held until every named field has been seen with a value, then the query is compiled and the records stream. A field no record has is text, so a condition on it matches nothing rather than failing. `--schema` takes the same file `explain` takes and validates the query against it instead.

### Flags

| Flag | Commands | What it does |
|---|---|---|
| `--schema <path>` | all | Validate against a field list instead of typing fields from the records. |
| `--pretty` | all | Indent the output. |
| `--raw`, `-r` | `eval` | Write strings without quotes, like `jq -r`. |

### Next to jq and yq

`query` filters and reshapes one record at a time. It does not group, aggregate or edit files, so `jq` and `yq` stay in the pipe for those, and they compose:

```bash
yq -o json '.events[]' data.yaml | query filter 'state=draft AND total>50000' | jq '{id, total}'
```

YAML is read through `yq -o json`: the library has no dependencies, and the standard library has no YAML parser.

[query and jq]({{ '/jq/' | relative_url }}) puts the two side by side on the same records: where a `query` filter is shorter, where it gives a different and more useful answer, and where `jq` is the tool to use.
