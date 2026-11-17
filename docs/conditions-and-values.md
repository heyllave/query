---
title: Conditions and values
nav_order: 2
parent: Language
permalink: /conditions-and-values/
---

# Conditions and values
{: .no_toc }

1. TOC
{:toc}

A query is one of two things. A **condition** answers yes or no about a record. A **value expression** computes something from a record. They have separate grammars and separate entry points.

| | Condition | Value expression |
|---|---|---|
| Example | `state=draft AND total>50000` | `[base]*1.1`, `now()-7d`, `upper(name)` |
| Go: compile | `eval.Compile` | `eval.CompileValue` |
| Go: run | `prog.Match(record)` | `prog.Eval(record)` |
| Go: one call | `query.Match` | `query.Eval` |
| TypeScript | `q.match(...)` | `q.eval(...)` |
| C library | `QueryMatch` | `QueryEval` |
| Result | `true` or `false` | a number, text, boolean, date, duration or list |
| Cannot resolve (missing field, division by zero) | the comparison is false | an error (`eval.ErrNoValue` in Go) |

Use a condition for filters and search. Use a value expression for computed fields: a derived price, a default date, a formatted label.

## In Go

```go
// A condition: does the record match?
p, err := eval.Compile("total>=[base]*1.1", fields)
p.Match(map[string]any{"total": 120, "base": 100}) // true

// A value expression: what does it compute?
v, err := eval.CompileValue("[base]*1.1", fields)
v.Eval(map[string]any{"base": 100}) // 110.00000000000001
v.Eval(map[string]any{})            // error: expression did not resolve to a value
```

`Eval` returns `int64`, `float64`, `string`, `bool`, `time.Time`, `time.Duration` or `[]any`.

Both take the same options (`WithFunctions`, `WithAllowedFields`, `WithAllowedOps`, `WithMaxLength`, `WithCustomValidator`), and both check field references against your field list.

## What a value expression can hold

```
(50000+1000)*1.1          # arithmetic, with precedence and parentheses
[base]*1.1                # a field, written in brackets
now()-7d                  # dates and durations
upper(name)               # a function over a field
round([price]/3, 2)       # arithmetic inside an argument
"John Doe"                # a literal
```

A value expression can also appear inside a condition, on the right of an operator: `total>=[base]*1.1`.

## Passing one to the other

The two grammars do not accept each other's input, but only one direction is an error.

| Call | What happens |
|---|---|
| `Compile("[base]*1.1")` | Parse error. There is no comparison. |
| `CompileValue("state=draft AND total>5")` | Parse error at `AND`. |
| `CompileValue("state=draft")` | **No error.** The result is the text `state=draft`. |
| `CompileValue("base+1")` | **No error.** The result is the text `base+1`. Write `[base]+1`. |
| `CompileValue("base")` | **No error.** The result is the text `base`. Write `[base]`. |
| `CompileValue("base*1.1")` | Parse error. `base*1.1` is read as a malformed wildcard. Write `[base]*1.1`. |

The rows marked "No error" are the ones to watch. In the value grammar an unquoted word is text and `=` is not an operator, so a simple condition, or a field written without brackets, comes back as a string. If a value expression returns its own source text, either it was meant for `Compile` or a field is missing its brackets. See [when a word is a field and when it is text]({{ '/syntax/' | relative_url }}#when-a-word-is-a-field-and-when-it-is-text).
