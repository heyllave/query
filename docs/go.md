---
title: Go
nav_order: 4
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

The module has no dependencies outside the standard library. Until the first tagged release, pin a commit.

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

## Parse and validate

```go
expr, err := query.ParseAndValidate("state=draft AND total>50000", fields)
```

`expr` is an `ast.Expression`. A syntax failure is a `parser.ErrorList`; a validation failure is a `validate.ErrorList`. Every entry has a stable `Code` and a position, so you can show your own wording and underline the right characters.

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
go run github.com/heyllave/query/cmd/query explain "status=active AND priority>3"
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

The full API is on [pkg.go.dev](https://pkg.go.dev/github.com/heyllave/query), and runnable programs are in [`examples/`](https://github.com/heyllave/query/tree/main/examples).
