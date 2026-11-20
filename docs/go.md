---
title: Go
nav_order: 1
parent: Clients
permalink: /go/
---

# Go
{: .no_toc }

1. TOC
{:toc}

## Install

```bash
go get github.com/heyllave/query
```

The module has no dependencies outside the standard library. Pin a tag: `go get github.com/heyllave/query@v0.4.0`.

## Describe your fields

Every query is checked against a list of fields. Each field has a type and the operators it allows.

```go
fields := []validate.FieldConfig{
    {Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
    {Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
    {Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
}
```

`validate.DefaultOps(t)` returns the usual operators for a type. A field marked `Nested` accepts any key under it (`labels.team`, `labels.example.com/owner`).

### List fields

A field of `validate.TypeList` is asked about with a selector. `Elements` declares the fields of one element, which is what the condition inside a selector may name.

```go
fields := []validate.FieldConfig{
    {Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
    {Name: "orders", Type: validate.TypeList, AllowedOps: validate.ListOps, Elements: []validate.FieldConfig{
        {Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps},
        {Name: "price", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
        {Name: "items", Type: validate.TypeList, AllowedOps: validate.ListOps, Elements: []validate.FieldConfig{
            {Name: "sku", Type: validate.TypeText, AllowedOps: validate.TextOps},
        }},
    }},
}
```

With the list declared, the validator checks what it could not check before:

| Query | Result |
|---|---|
| `orders@(status=shipped AND price>10)` | valid |
| `orders@(items@(sku=A1))` | valid: an element field may be a list |
| `orders@(bogus=1)` | `unknown field "bogus" on "orders"` |
| `orders@(name=x)` | `unknown field "name" on "orders"`: only an element's fields are in scope |
| `orders@(price=high)` | a type error, against the element's type |
| `name@first` | `"name" is not a list field` |
| `orders@last` | `selector @last is not allowed for field "orders"` |

A list accepts `@first`, `@(...)`, `@all(...)` and `@none(...)` unless it says otherwise, which is one spelling for each question: `@last` asks what `@first` asks, and `@any(...)` is `@(...)`. To allow them, list the selectors by name:

```go
{Name: "tags", Type: validate.TypeList, Selectors: []string{ast.SelectorFirst, ast.SelectorLast, ast.SelectorAny}}
```

`FieldConfig.SelectorForms()` returns them as written (`@first`, `@(...)`, `@all(...)`), for a host that describes its schema to a client.

Two rules to know:

- **Declaring one list declares them all.** Once a schema has a list field, a selector on any other field is an error. A schema with no list field keeps the older rule: the base only has to exist, and the condition is checked against the record's fields.
- **A list without `Elements`** is a list of plain values or of something you chose not to describe. Its condition is checked against the record's fields.

## Parse and validate

```go
expr, err := query.ParseAndValidate("state=draft AND total>50000", fields)
```

`expr` is an `ast.Expression`. A syntax failure is a `parser.ErrorList`; a validation failure is a `validate.ErrorList`. Every entry has a stable `Code` and a position, so you can show your own wording and underline the right characters, and a `Hint` when the engine can tell what was meant. See [Errors and hints]({{ '/errors/' | relative_url }}).

## Match records in memory

Compile once, match many times. A compiled program is safe to share between goroutines.

```go
prog, err := eval.Compile("state=draft AND total>50000", fields)

prog.Match(map[string]any{"state": "draft", "total": 60000}) // true
```

For a struct, tag the fields and let the library build the field list:

```go
type Invoice struct {
    State string  `query:"state"`
    Total float64 `query:"total"`
}

prog, err := eval.CompileFor[Invoice]("state=draft AND total>50000")
prog.MatchStruct(Invoice{State: "draft", Total: 60000}) // true
```

## Compute a value

```go
prog, _ := eval.CompileValue("[base]*1.1", fields)
v, err := prog.Eval(map[string]any{"base": 100}) // 110.00000000000001
```

The result is `int64`, `float64`, `string`, `bool`, `time.Time`, `time.Duration` or `[]any`. When the expression cannot resolve (a missing field, a division by zero), `Eval` returns `eval.ErrNoValue`.

`Compile` takes a condition and answers yes or no; `CompileValue` takes an expression and returns what it computes. See [Conditions and values]({{ '/conditions-and-values/' | relative_url }}) for the difference and for what happens when one is passed to the other.

## Restrict what a caller may ask

```go
prog, err := eval.Compile(q, fields,
    eval.WithAllowedFields("state", "total"),
    eval.WithAllowedOps(validate.OpEq, validate.OpNeq),
    eval.WithMaxDepth(3),
    eval.WithMaxLength(256),
)
```

A field's own `AllowedOps` bounds every way a query can read it: directly, through a function, and from the value side of a comparison. A field that allows only `=` can be tested for equality and nothing else.

If you translate the tree yourself, also name the functions you can translate:

```go
v := validate.New(fields, validate.WithAllowedFunctions("lower", "now"))
err := v.Validate(expr) // upper(name)=X fails with code functionNotAllowed
```

Called with no names, `WithAllowedFunctions` refuses every function.

## Add your own functions

```go
prog, err := eval.Compile("wordCount(description)>3", fields,
    eval.WithFunctions(eval.Func{
        Name:  "wordCount",
        Arity: eval.Exactly(1),
        Call: func(args ...any) (any, error) {
            if args[0] == nil {
                return nil, nil // a missing field resolves to no value
            }
            return int64(len(strings.Fields(fmt.Sprint(args[0])))), nil
        },
    }),
)
```

A panic inside your function becomes an error from that call. It does not crash the caller.

## Complete a query being typed

```go
c := query.Complete("status=re", 9, fields, query.WithStrict())
c.Slot    // parser.SlotValue
c.Field   // "status"
c.Prefix  // "re"
c.Items   // the values status lists in FieldConfig.Values that fit "re"
```

`query.Complete` says what the grammar expects at a byte offset of a query that is still being written, and lists what the schema can suggest there. It never fails: a position it cannot place has `parser.SlotNone`. Candidates that live outside the schema go through `c.Offer(names...)`, which writes each one the way the grammar needs it. The [completion guide]({{ '/completion/' | relative_url }}) has the slots and a full example.

`parser.CursorAt` is the layer underneath, for a tool that wants the position without a schema: the slot, the word under the cursor, the field it belongs to and the list selectors around it.

## Translate the tree

Implement `ast.Visitor[T]` to turn a tree into SQL, JSON or anything else. See [Filtering a list with SQL]({{ '/sql/' | relative_url }}).

Useful helpers:

```go
ast.Fields(expr)   // every field the query reads
ast.Calls(expr)    // every function call
ast.Depth(expr)    // nesting depth
ast.String(expr)   // the query text; parsing it gives the same tree
```

## Debug a query

```bash
go run github.com/heyllave/query/cmd/query@latest explain "status=active AND priority>3"
```

The [command line]({{ '/cli/' | relative_url }}) page covers its flags: tokens, JSON, positions and validation against a field list.

The full API is on [pkg.go.dev](https://pkg.go.dev/github.com/heyllave/query), and runnable programs are in [`examples/`](https://github.com/heyllave/query/tree/main/examples).
