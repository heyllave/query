---
title: Filtering a list with SQL
nav_order: 7
permalink: /sql/
---

# Filtering a list with SQL
{: .no_toc }

1. TOC
{:toc}

This is the route for a service that lists resources from a database and lets the caller filter them: `GET /sandboxes?q=labels.team=payments AND status=ready`. The library parses and validates the query. Your code turns the tree into a `WHERE` clause.

The library never produces SQL itself. A complete, runnable translation is in [`examples/sql`](https://github.com/heyllave/query/blob/main/examples/sql/main.go); this page explains the steps.

## 1. Declare what can be queried

```go
var schema = []validate.FieldConfig{
    {Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
    {Name: "status", Type: validate.TypeText, AllowedOps: []validate.Op{validate.OpEq, validate.OpNeq}},
    {Name: "replicas", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
    {Name: "created_at", Type: validate.TypeDatetime, AllowedOps: validate.DateOps},
    {Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
}
```

A field that is not in the list is an error. A field that allows only `=` and `!=` cannot be read with a wildcard, an ordering or a function.

## 2. Validate, and name the functions you can translate

```go
expr, err := query.Parse(q)
if err != nil {
    return err // parser.ErrorList: coded, positioned
}

v := validate.New(schema,
    validate.WithAllowedFunctions("lower", "now", "daysAgo"),
)
if err := v.Validate(expr); err != nil {
    return err // validate.ErrorList: coded, positioned
}
```

Without `WithAllowedFunctions` the validator accepts any function name, because which functions exist is the evaluator's business. A translator should list exactly the functions it has a SQL template for. Called with no names, it refuses every function.

## 3. Translate with a visitor

```go
type sqlVisitor struct {
    params []any
    err    error
}

func (v *sqlVisitor) VisitBinary(e *ast.BinaryExpr) string {
    op := " OR "
    if e.Op == token.And {
        op = " AND "
    }
    return ast.Visit[string](v, e.Left) + op + ast.Visit[string](v, e.Right)
}

func (v *sqlVisitor) VisitQualifier(e *ast.QualifierExpr) string {
    col := v.column(e.Field) // through your own allowlist
    v.params = append(v.params, e.Value.Any())
    return fmt.Sprintf("%s %s $%d", col, ast.SQLOperator(e.Operator, false), len(v.params))
}

// ... VisitUnary, VisitPresence, VisitGroup, VisitSelector, VisitFuncCall

where := ast.Visit[string](&sqlVisitor{}, expr)
```

```
Input:  state=draft AND total>50000
WHERE:  invoices.state = $1 AND invoices.total > $2
Params: [draft 50000]
```

## Rules for a safe translation

- **Bind every literal as a parameter.** Never write a value into the SQL text.
- **Map every field to a column through your own table.** A validated tree is not safe to interpolate. Nested keys such as `labels.example.com/team` are caller-supplied text, so bind the key too (`labels ->> $1`).
- **Map every function through your own table**, with its argument count.
- **Keep the length limit.** The default is 256 bytes.

## Details that are easy to get wrong

| Query | What it means | SQL |
|---|---|---|
| `created_at=2026-09-28` | The whole UTC day. `Value.DayOnly` is set. | `created_at >= $1 AND created_at < $2` |
| `name=Jo*` | Wildcard, ignoring case. `e.IsWildcard()` is true. | `name ILIKE $1`, with `ast.WildcardToLike` for the pattern |
| `name!=Jo*` | The negated wildcard. | `name NOT ILIKE $1` |
| `total!>5` | `NOT total>5`, so it is true when `total` is missing. | `NOT COALESCE(total > $1, FALSE)` |
| `state NOT IN (a, b)` | `state!=a AND state!=b`, false when `state` is missing. | `state <> $1 AND state <> $2` |
| `status` | Presence: the field exists and is not empty. | `status IS NOT NULL AND status <> ''` |
| `orders@(status=shipped)` | At least one element matches. | `EXISTS (SELECT 1 ...)` |

The evaluator in this library is the reference for what a query means. If you want to check your translation against it, [`conformance/sql`](https://github.com/heyllave/query/tree/main/conformance/sql) does exactly that with SQLite: every case runs through the evaluator and through a real database, and the two must agree. You can run your own lowering through the same corpus.

## Returning errors to the caller

Both error lists carry a stable code and a position for every failure.

```go
var perr parser.ErrorList
if errors.As(err, &perr) {
    for _, e := range perr {
        // e.Code, e.Position.Offset, e.Message
    }
}
```

Return the code and the offset in your API response. A client can then underline the right characters and show its own wording.
