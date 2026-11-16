---
title: Command line
nav_order: 6.5
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

The language has no object construction beyond `map`, no aggregation across records and no paths that yield several values, so `jq` and `yq` stay in the pipe for those. They compose:

```bash
yq -o json '.events[]' data.yaml | query filter 'state=draft AND total>50000' | jq '{id, total}'
```

| Task | jq | query |
|---|---|---|
| Equality and a number | `select(.state == "draft" and .total > 50000)` | `filter 'state=draft AND total>50000'` |
| Prefix, ignoring case | `select(.name \| ascii_downcase \| startswith("jo"))` | `filter 'name=jo*'` |
| One of several | `select(.state \| IN("draft","issued"))` | `filter 'state IN (draft, issued)'` |
| Date range | `select(.created_at >= "2026-01-01" and .created_at <= "2026-03-31")` | `filter 'created_at:2026-01-01..2026-03-31'` |
| Last seven days | `--arg t "$(date -u -d '7 days ago' +%FT%TZ)" 'select(.created_at >= $t)'` | `filter 'created_at>=now()-7d'` |
| Nested field | `select(.labels.team == "payments")` | `filter 'labels.team=payments'` |
| Present and not empty | `select(.tire_size != null and .tire_size != "")` | `filter 'tire_size'` |
| Two fields | `select(.cpu > .cpu_limit)` | `filter 'cpu>[cpu_limit]'` |
| Any list element | `select(any(.orders[]; .status == "shipped"))` | `filter 'orders@(status=shipped)'` |
| Every list element | `select(all(.orders[]; .price > 0))` | `filter 'orders@all(price>0)'` |
| A value | `.total * 1.1` | `eval '[total]*1.1'` |
| A string, bare | `-r .name` | `eval -r '[name]'` |
| Reshape | `{id, total: (.total * 1.1)}` | `map 'id, total=[total]*1.1'` |
| Group and count | `group_by(.state) \| map(length)` | not in the language |
| Edit in place | `yq -i '.replicas = 3' deploy.yaml` | not in the language |

YAML is read through `yq -o json`: the library has no dependencies, and the standard library has no YAML parser.
