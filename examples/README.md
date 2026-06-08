# Examples

Runnable examples demonstrating every capability of the query library.

## Quick Start

```bash
# SQL generation
go run ./examples/sql "state=draft AND total>50000"
go run ./examples/sql "name=John* AND created_at:2026-01-01..2026-03-31"

# JSON AST
go run ./examples/json "(state=draft OR state=issued) AND NOT cluster=demo"

# In-memory filtering
go run ./examples/filter

# Built-in and custom functions
go run ./examples/functions

# Value-returning queries (compute a number, string, time, duration, or list)
go run ./examples/value

# Rule derivation (do X when A, do Y when B): first-match, collect-all, hybrid
go run ./examples/rules

# Struct binding with compile-time type safety
go run ./examples/struct

# Query restrictions and sandboxing
go run ./examples/restrictions

# @ selector operator — list-field predicates
go run ./examples/selector

# Custom validation rules (per-tenant access, cross-field, value-range)
go run ./examples/customvalidator

# AST inspection and debugging
go run ./examples/explain
```

## Examples

| Directory | What it demonstrates |
|-----------|---------------------|
| [`sql/`](sql/) | SQL WHERE clause generation with parameterized queries ($1, $2) |
| [`json/`](json/) | JSON AST serialization for APIs and frontends |
| [`filter/`](filter/) | In-memory filter functions with wildcard matching |
| [`functions/`](functions/) | Built-in function families — string, numeric math (abs/round/min/max), time-component (hour/weekday/addDays), list aggregation (count/sum/avg/first/last), type coercion — and custom function registration |
| [`value/`](value/) | `CompileValue` / `Eval` — value-returning queries: arithmetic, functions over fields, time values, lists, and `ErrNoValue` handling |
| [`rules/`](rules/) | Rule derivation — pair predicates with actions: first-match routing, collect-all tagging, and hybrid predicate-selects-value-computes |
| [`struct/`](struct/) | `CompileFor[T]` struct binding, `MatchStruct`, type safety |
| [`restrictions/`](restrictions/) | `WithAllowedFields`, `WithAllowedOps`, `WithMaxDepth`, `WithMaxLength` |
| [`selector/`](selector/) | `@first`, `@last`, `@(inner)` against map- and struct-backed list fields |
| [`customvalidator/`](customvalidator/) | `AstValidator` hook: per-tenant field denylists, cross-field rules, value ranges |
| [`explain/`](explain/) | AST inspection: tokens, tree rendering, field extraction, error formatting |

## Pattern: Implementing a Visitor

Every code generation target follows the same pattern — implement `ast.Visitor[T]`:

```go
type myVisitor struct{ /* state */ }

func (v *myVisitor) VisitBinary(e *ast.BinaryExpr) T    { /* ... */ }
func (v *myVisitor) VisitUnary(e *ast.UnaryExpr) T      { /* ... */ }
func (v *myVisitor) VisitQualifier(e *ast.QualifierExpr) T { /* ... */ }
func (v *myVisitor) VisitPresence(e *ast.PresenceExpr) T { /* ... */ }
func (v *myVisitor) VisitGroup(e *ast.GroupExpr) T       { /* ... */ }
func (v *myVisitor) VisitSelector(e *ast.SelectorExpr) T { /* ... */ }
func (v *myVisitor) VisitFuncCall(e *ast.FuncCallExpr) T { /* ... */ }

result := ast.Visit[T](&myVisitor{}, expr)
```

See [`sql/main.go`](sql/main.go) for a complete reference implementation.
