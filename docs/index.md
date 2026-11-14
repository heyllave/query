---
title: Overview
layout: home
nav_order: 1
---

# query

A small query language for search bars, URL parameters and API filters.
{: .fs-6 .fw-300 }

```
state=draft AND total>50000
labels.team=payments AND created_at>=now()-7d
name=Jo* AND state IN (draft, issued) AND NOT archived
```

[Try it in the playground]({{ '/playground/' | relative_url }}){: .btn .btn-primary .mr-2 }
[Read the syntax]({{ '/syntax/' | relative_url }}){: .btn }

## What it does

The library takes a query string from a person you do not trust and gives you one of three things:

- **A tree.** Parse and validate the query against your field list, then translate the tree yourself: to SQL, to an API filter, to anything.
- **A yes or no.** Compile the query once and match it against records in memory.
- **A value.** Evaluate an expression such as `[base]*1.1` or `now()-7d`.

It is written in Go with no dependencies outside the standard library. The same engine runs everywhere:

| You write | You use | Guide |
|---|---|---|
| Go | the module `github.com/heyllave/query` | [Go]({{ '/go/' | relative_url }}) |
| TypeScript or JavaScript | the npm package, which runs the engine as WebAssembly | [TypeScript]({{ '/typescript/' | relative_url }}) |
| Anything else | `libquery`, a C shared library with a JSON contract | [C library]({{ '/c-library/' | relative_url }}) |

All three run one shared test corpus, so a query means the same thing in each.

## Why this and not a general expression language

The syntax is built for a URL: `?q=state=draft AND total>50000` needs no quotes, no `==` and no `&&`. Each field has a declared type and a list of operators it allows, so you decide what a caller may ask. The language has no loops, no assignment and no user-defined functions, and a query is bounded in length and nesting.

If you need business rules, computed fields or templates, a general expression engine such as [expr-lang/expr](https://github.com/expr-lang/expr) is the better fit.

## Where to go next

- [Syntax]({{ '/syntax/' | relative_url }}): every operator, value and function.
- [Filtering a list with SQL]({{ '/sql/' | relative_url }}): the parse, validate and translate route.
- [Stability]({{ '/stability/' | relative_url }}): what you can rely on, and what enforces it.
