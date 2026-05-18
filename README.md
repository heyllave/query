# query

[![CI](https://github.com/heyllave/query/actions/workflows/ci.yml/badge.svg)](https://github.com/heyllave/query/actions/workflows/ci.yml)

Pure Go query language library. Handles lexing, parsing, AST construction, validation, and evaluation of a unified query syntax used across all clients (CLI, Web UI, API, VS Code, WASM).

Zero external dependencies. Compiles to WebAssembly.

## Install

```bash
go get github.com/heyllave/query
```

## Packages

| Package | Purpose |
|---------|---------|
| `query` | Top-level API: `Parse()`, `Validate()`, `ParseAndValidate()` |
| `query/token` | Lexical token types and position tracking |
| `query/ast` | AST nodes, `Visitor[T]` pattern, `Walk`, `String` |
| `query/parser` | Lexer and recursive descent parser |
| `query/validate` | Field configuration and AST validation |
| `query/eval` | Compile-and-match engine with functions and struct binding |

## Quick Start

```go
// Parse
expr, err := query.Parse("state=draft AND total>50000")

// Validate
fields := []validate.FieldConfig{
    {Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
    {Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
}
err = query.Validate(expr, fields)

// Or compile and evaluate in one shot
prog, err := eval.Compile("state=draft AND total>50000", fields)
prog.Match(map[string]any{"state": "draft", "total": 60000}) // true
```

## Query Syntax

```
state=draft                                        # equality
state!=cancelled                                   # not equal
year>2020                                          # comparison (>, >=, <, <=)
name=John*                                         # wildcard (prefix, suffix, contains)
tire_size                                          # presence check
state=draft AND customer_id=customer_john-doe      # logical AND
(state=draft OR state=issued) AND total>50000      # grouping with precedence
NOT state=cancelled                                # negation
created_at:2026-01-01..2026-03-31                  # date range
ttl.duration>1d                                    # duration comparison
labels.dev=jane                                    # nested field access
lower(name)=john*                                  # function call as field transform
len(name)>5                                        # function in comparison
contains(tags, category)                           # function as boolean predicate
orders@first                                       # selector: list is non-empty
orders@(status=shipped)                            # selector: any element satisfies
```

## Compile and Evaluate

The `eval` package compiles a query into an executable program:

```go
import "github.com/heyllave/query/eval"

prog, err := eval.Compile("state=draft AND total>50000", fields)

// Match against a map
prog.Match(map[string]any{"state": "draft", "total": 60000}) // true

// Match with a custom accessor
prog.MatchFunc(func(field string) (any, bool) {
    return myRecord.Get(field)
})

// Inspect
prog.Fields()    // []ast.FieldPath{["state"], ["total"]}
prog.Stringify() // "state=draft AND total>50000"
prog.AST()       // ast.Expression
```

## Struct Binding

Compile against Go structs for type-safe evaluation:

```go
type Invoice struct {
    State     string    `query:"state"`
    Total     float64   `query:"total"`
    CreatedAt time.Time `query:"created_at"`
    Internal  string    // no tag = not queryable
}

prog, err := eval.CompileFor[Invoice]("state=draft AND total>50000")
prog.MatchStruct(Invoice{State: "draft", Total: 60000}) // true

// Type mismatches caught at compile time:
_, err = eval.CompileFor[Invoice]("total=notanumber") // error: type mismatch
_, err = eval.CompileFor[Invoice]("Internal=secret")  // error: unknown field
```

## Built-in Functions

Functions can transform fields or act as boolean predicates:

```go
// String functions
lower(name)=john*         // case-insensitive match
upper(name)=JOHN          // uppercase transform
trim(description)=hello   // strip whitespace
len(name)>5               // string length

// String predicates (two field references)
contains(name, tags)      // field value contains other field's value
startsWith(name, prefix)  // prefix check
endsWith(name, suffix)    // suffix check

// Date functions
year(created_at)=2026     // extract year
month(created_at)=3       // extract month
day(created_at)=15        // extract day

// Date generators
// (use in eval context)
now()                     // current timestamp
today()                   // midnight today
daysAgo(7)                // 7 days ago
```

## Custom Functions

Register domain-specific functions:

```go
prog, err := eval.Compile("wordCount(description)>3", fields,
    eval.WithFunctions(eval.Func{
        Name: "wordCount",
        Call: func(args ...any) (any, error) {
            s := strings.TrimSpace(fmt.Sprint(args[0]))
            return int64(len(strings.Fields(s))), nil
        },
    }),
)
```

Disable built-ins if you want full control:

```go
prog, err := eval.Compile(q, fields,
    eval.WithNoBuiltins(),
    eval.WithFunctions(myFunc1, myFunc2),
)
```

## Query Restrictions

Sandbox queries for different user roles or API contexts:

```go
// Public API: only allow specific fields
prog, err := eval.Compile(q, fields,
    eval.WithAllowedFields("state", "total", "year"),
)

// Read-only: only equality checks
prog, err := eval.Compile(q, fields,
    eval.WithAllowedOps(validate.OpEq, validate.OpNeq),
)

// DoS protection: limit nesting depth and query length
prog, err := eval.Compile(q, fields,
    eval.WithMaxDepth(3),
    eval.WithMaxLength(256),
)
```

## Custom Validation Rules

For rules beyond field/type/op checks (per-tenant access, cross-field
constraints, value ranges), implement `validate.AstValidator` and install it
via `eval.WithCustomValidator` (or `validate.WithCustomValidator` if you are
using the `validate` package directly):

```go
type tenantValidator struct {
    tenantID string
    fields   map[string]validate.FieldConfig
    denied   map[string]bool
}

// GetFieldConfig overrides the static config. Returning (_, false) marks
// the field as unknown — even if declared statically. Use this for
// per-tenant field allow/denylists.
func (t *tenantValidator) GetFieldConfig(name string) (validate.FieldConfig, bool) {
    if t.denied[name] {
        return validate.FieldConfig{}, false
    }
    cfg, ok := t.fields[name]
    return cfg, ok
}

// ValidateCustomRules runs once on the root after built-in checks.
// Walk the AST to implement cross-field rules, value ranges, etc.
func (t *tenantValidator) ValidateCustomRules(node ast.Expression) error {
    var start, end *ast.QualifierExpr
    ast.Walk(node, func(e ast.Expression) bool {
        if q, ok := e.(*ast.QualifierExpr); ok {
            switch q.Field.String() {
            case "start_date": start = q
            case "end_date":   end = q
            }
        }
        return true
    })
    if start != nil && end != nil && !start.Value.Date.Before(end.Value.Date) {
        return fmt.Errorf("start_date must be before end_date")
    }
    return nil
}

prog, err := eval.Compile(q, fields, eval.WithCustomValidator(&tenantValidator{...}))
```

Errors returned from `ValidateCustomRules` are merged into the validator's
`ErrorList` alongside built-in errors. Returning a `*validate.Error` or
`validate.ErrorList` preserves positions and kinds; any other error is wrapped
as `ErrCustomRule` anchored at the root position.

See [`examples/customvalidator/`](examples/customvalidator/) for a complete
runnable example covering all three use cases.

## Code Generation via Visitor

Implement `ast.Visitor[T]` to transform the AST into any target:

```go
type sqlVisitor struct{ params []any }

func (v *sqlVisitor) VisitBinary(e *ast.BinaryExpr) string {
    left := ast.Visit[string](v, e.Left)
    right := ast.Visit[string](v, e.Right)
    if e.Op == token.And { return left + " AND " + right }
    return left + " OR " + right
}

func (v *sqlVisitor) VisitQualifier(e *ast.QualifierExpr) string {
    v.params = append(v.params, e.Value.Any())
    return fmt.Sprintf("%s %s $%d", e.Field, ast.SQLOperator(e.Operator, false), len(v.params))
}
// ... implement remaining 5 methods ...

v := &sqlVisitor{}
where := ast.Visit[string](v, expr)
// "state = $1 AND total > $2", params: ["draft", 50000]
```

See [`examples/`](examples/) for complete implementations of SQL, JSON, filter function, and struct binding visitors.

## AST Utilities

```go
ast.Fields(expr)      // []FieldPath — all referenced fields
ast.Qualifiers(expr)  // []*QualifierExpr — all field=value pairs
ast.IsSimple(expr)    // bool — single condition (no AND/OR)?
ast.Depth(expr)       // int — max nesting depth
ast.Walk(expr, fn)    // depth-first traversal
ast.String(expr)      // round-trip back to query string
```

## Selectors (list fields)

Selectors apply a predicate to a list-valued field. Three forms are supported:

```
items@first            # list exists and has ≥ 1 element
items@last             # list exists and has ≥ 1 element (distinct for codegen)
orders@(status=shipped)  # EXISTS: at least one element satisfies the inner
```

Element shapes inside `@(...)`:

- `map[string]any` — inner fields resolve by key: `orders@(status=shipped)` reads `"status"` on each map.
- Struct with `query:"..."` tags — inner fields resolve by tag, same contract as `StructAccessor`.
- Any other type (primitives, untyped slices) — inner field lookups return `(nil, false)` and do not match.

Validation of list fields only requires the field to be declared. `OpPresence` is not required for a field used as a selector base.

Composition works as expected:

```
(orders@(status=shipped) OR orders@(status=delivered)) AND total>500
NOT line_items@(price>100)
```

Codegen via `Visitor[T]` is the consumer's responsibility — the library does not translate selectors into SQL `EXISTS` or JSON path queries. See `ast.VisitSelector` to plug in your target.

## Operators

| Operator | Allowed Types | Description |
|----------|--------------|-------------|
| `=` | all | Equality or wildcard match |
| `!=` | all | Not equal |
| `>` `>=` `<` `<=` | number, date, duration | Comparison |
| `..` | number, date, duration | Inclusive range (`field:start..end`) |
| `NOT` | expression | Boolean negation |
| `AND` | expressions | Logical AND (higher precedence than OR) |
| `OR` | expressions | Logical OR |

## Strengths

- **URL-native syntax** — `state=draft AND total>50000` works directly in `?q=` params. No quotes, no `==`, no `&&`.
- **Zero dependencies** — stdlib only. Compiles to WASM without issues.
- **Compile-time type safety** — `CompileFor[T]` catches field name typos and type mismatches before any data is evaluated.
- **Multi-target code generation** — one AST, many outputs. The `Visitor[T]` pattern makes it trivial to generate SQL, JSON, React components, or filter functions.
- **Query sandboxing** — `WithAllowedFields`, `WithAllowedOps`, `WithMaxDepth` let you expose different query capabilities to different user roles.
- **Built-in + custom functions** — `lower()`, `year()`, `len()` out of the box; register your own with `WithFunctions`.
- **Rich value types** — native support for dates (`2026-01-01`), durations (`1d`, `4h`), wildcards (`John*`), and ranges (`field:start..end`).
- **Struct binding** — `query:"field_name"` tags on Go structs auto-generate field configs.
- **Round-trip fidelity** — `ast.String(ast.Parse(q)) == q` for all normalized queries.
- **TypeScript package** — full type definitions, visitor pattern, and WASM loader for browser/Node.js.

## Limitations

These are known limitations that we plan to address in future versions:

- **No string literals in function args** — function arguments are parsed as field references, not quoted strings. `contains(name, "urgent")` won't work; use wildcard `name=*urgent*` or pass both as field refs: `contains(name, search_term)`.
- **No functions in value position** — `created_at>=now()` is not yet supported. Compute dynamic values before compiling, or use `daysAgo()` as a field transform workaround.
- **No arithmetic** — `total>50000*1.1` is not supported. Register a custom function for computed comparisons.
- **No implicit AND** — `state=draft total>50000` requires explicit `AND`. Some query languages allow space as implicit AND.
- **No quoted strings** — values are unquoted and terminated by whitespace or `)`. Values containing spaces are not directly expressible.
- **No OR shorthand** — `state=(draft,issued)` or `state IN (draft,issued)` is not supported. Use `state=draft OR state=issued`.
- **Case-sensitive keywords** — `AND`, `OR`, `NOT` must be uppercase. `and`, `or`, `not` are treated as identifiers.
- **No negated comparisons** — `NOT total>50000` works, but `total!>50000` does not exist.
- **Closure-based eval** — the eval engine compiles to closure trees, not bytecode. For hot-path evaluation of millions of records, a bytecode compiler would be faster.
- **Reflect in struct binding** — `CompileFor[T]` and `StructAccessor` use reflection. This is fine for compile-time setup but adds overhead if called per-record. Compile once, match many.

## Comparison with expr-lang/expr

| Feature | query | expr-lang/expr |
|---------|-------|---------------|
| **Use case** | Search bars, URL params, API filters | Business rules, computed fields, templates |
| **Syntax** | `state=draft AND total>50000` | `state == "draft" && total > 50000` |
| **URL-safe** | Yes (no quotes, no special chars) | No (requires URL encoding) |
| **Wildcards** | `name=John*` native | Regex or custom function |
| **Ranges** | `created_at:2026-01-01..2026-03-31` | Manual `>=` and `<=` |
| **Presence** | `tire_size` | `tire_size != nil` |
| **Field validation** | Per-field type + operator config | Struct-based type checking |
| **Code generation** | `Visitor[T]` for SQL/JSON/React/etc. | Not designed for this |
| **Dependencies** | Zero (stdlib only) | reflect, unsafe, internal |
| **WASM** | First-class target | Possible but heavy |
| **Functions** | Built-in + custom registry | Rich expression language |
| **Arithmetic** | Not supported | Full arithmetic |
| **String operations** | Via functions (`lower`, `len`) | Native (`+`, `contains`, etc.) |
| **Ternary/nullish** | Not supported | `?:`, `??` |
| **Array operations** | Not supported | `map`, `filter`, `all`, `any` |
| **Maturity** | New | Battle-tested, years of production |

**Choose this library** when you need a search/filter language for end users (search bars, `?q=` params, API filters) with multi-target code generation and query sandboxing.

**Choose expr-lang** when you need a general-purpose expression engine for business rules, computed fields, or template evaluation where the full power of arithmetic, arrays, and ternary operators matters.

## CLI — `query explain`

The `query explain` command parses a query expression and visualizes the AST for debugging:

```bash
go run ./cmd/query explain "status=active AND priority>3"
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

Nested groups and NOT:

```bash
go run ./cmd/query explain "(state=draft OR state=issued) AND NOT cancelled"
```

```
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

JSON output for programmatic use:

```bash
go run ./cmd/query explain --json "status=active"
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

### Flags

| Flag | Description |
|------|-------------|
| `--json` | Emit AST as JSON |
| `--tokens` | Print lexer tokens instead of AST |
| `--schema <path>` | Validate against a JSON schema file |
| `--positions` | Include source position spans on each node |

## Examples

See the [`examples/`](examples/) directory for runnable programs:

```bash
go run ./examples/sql "state=draft AND total>50000"
go run ./examples/json "(state=draft OR state=issued) AND total>50000"
go run ./examples/filter
go run ./examples/functions
go run ./examples/struct
go run ./examples/restrictions
go run ./examples/customvalidator
```

## License

Apache License 2.0
