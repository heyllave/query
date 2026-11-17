---
title: Grammar
nav_order: 3.2
permalink: /grammar/
---

# Grammar
{: .no_toc }

The whole language in one place: the grammar the parser implements, the
choices it makes where text could be read two ways, and the options that turn
the lenient forms off. [Syntax](../syntax/) explains the same thing by example.

1. TOC
{:toc}

## Grammar

EBNF, with `{ x }` for zero or more, `[ x ]` for optional and `|` for a
choice. Terminals are in quotes. The keywords `AND`, `OR`, `NOT` and `IN` are
accepted in any case.

```ebnf
query      = or_expr ;

or_expr    = and_expr { "OR" and_expr } ;
and_expr   = chain { [ "AND" ] chain } ;       (* see "Implicit AND" below *)

chain      = term { "@" selector } ;
selector   = "first" | "last"
           | [ "any" | "all" | "none" ] "(" or_expr ")" ;

term       = "NOT" chain
           | "(" or_expr ")"
           | condition ;

condition  = subject [ comparison ] ;          (* no comparison: presence, or a predicate call *)
subject    = field | call ;
comparison = operator value
           | ":" value ".." value              (* inclusive range *)
           | [ "NOT" ] "IN" "(" value { "," value } ")" ;
operator   = "=" | "!=" | ">" | ">=" | "<" | "<="
           | "!>" | "!>=" | "!<" | "!<=" ;     (* field !> v  is  NOT field > v *)

field      = name { "." name }                 (* labels.team *)
           | "[" name { "." name } "]"         (* [labels.team], a field used as a value *)
           | "[" string "]"                    (* ["billing address"], one name, dots included *)
           | string ;                          (* "billing address" in field position *)

call       = name "(" [ arg { "," arg } ] ")" ;   (* no space between the name and "(" *)
arg        = call | field | value ;

value      = sum ;
sum        = product { ( "+" | "-" ) product } ;
product    = primary { ( "*" | "/" | "%" ) primary } ;
primary    = "(" sum ")"
           | call
           | "[" name { "." name } "]" | "[" string "]"
           | string
           | literal ;
literal    = integer | float | boolean | date | datetime | duration
           | pattern | text ;
```

Precedence, tightest first: a selector (`@`), `NOT`, `AND`, `OR`. So
`NOT items@(x=1) AND a=1 OR b=2` is `((NOT (items@(x=1))) AND a=1) OR b=2`.

## Words

```ebnf
name       = name_start { name_char } ;
name_start = letter | "_" | non_ascii ;
name_char  = name_start | digit | "-" | "/" ;

string     = '"' { char | '\"' | '\\' | '\n' | '\t' | '\r' } '"' ;

integer    = [ "-" | "+" ] digit { digit } ;
float      = [ "-" | "+" ] digit { digit } "." digit { digit } ;
boolean    = "true" | "false" ;
date       = digit digit digit digit "-" digit digit "-" digit digit ;   (* a whole UTC day *)
datetime   = date "T" time ( "Z" | offset ) ;                            (* RFC 3339 *)
duration   = digit { digit } ( "m" | "h" | "d" | "w" ) ;

text       = value_char { value_char } ;       (* an unquoted value that is none of the above *)
pattern    = "*" text | text "*" | "*" text "*" | "*" ;
```

An unquoted value ends at a space, `)`, `[`, `]`, `..`, and at a `,` inside
an `IN` list or a call. Inside one, `\*`, `\\`, `\(` and `\)` stand for the
literal character. Anything else goes in quotes.

## Where text could be read two ways

A grammar this small still has places where the same characters could mean
two things. Each is decided the same way every time:

| Text | Read as | Not as | To get the other reading |
|---|---|---|---|
| `a=1 b=2` | `a=1 AND b=2` | an error | turn on `ExplicitAnd` (below) |
| `a=1 OR b=2 AND c=3` | `a=1 OR (b=2 AND c=3)` | left to right | parentheses |
| `NOT a=1 AND b=2` | `(NOT a=1) AND b=2` | `NOT (a=1 AND b=2)` | parentheses |
| `NOT items@(x=1)` | `NOT (items@(x=1))` | `(NOT items)@(x=1)` | none; a selector applies to a field |
| `state=draft` | field `state`, text `draft` | two fields | `state=[draft]` compares two fields |
| `id=550e8400-e29b-41d4` | one text value | subtraction | arithmetic needs numbers: `n=10-3` |
| `name=foo*` | prefix pattern | a literal star | `name=foo\*` or `name="foo*"` |
| `name=a*b*` | an error (`invalidWildcard`) | a pattern | a star may only start or end a value |
| `urgent (a OR b)` | term, then a group | a call | a call has no space: `urgent(a)` |
| `state NOT IN (a, b)` | negated list | `state AND NOT in(...)` | `NOT` binds to `IN` only directly before `IN (` |
| `labels.in=x`, `not=1` | a field named by a keyword | a connective | a keyword is a name when an operator follows it |
| `a=1+b=2` | `a` equals the text `1+b=2` | two conditions | turn on `StrictValues` (below) |
| `labels[team]=x` | `labels AND [team]=x` | a keyed lookup | write `labels.team=x`; `ExplicitAnd` refuses this form |

The last two rows are the ones that surprise people. Both come from being
lenient, and both are refused under the strict options.

## Strict options

The default grammar is lenient where someone typing into a search box
benefits. A host that takes queries from scripts, saved links or other
programs can turn the lenient forms off:

| Option | Default | When on |
|---|---|---|
| `ExplicitAnd` | off | A space is no longer an `AND`. `a=1 b=2`, `frontend checkout` and `labels[team]=x` are errors with the code `missingAnd`. Write `a=1 AND b=2`; quote text that has a space. |
| `StrictValues` | off | `=`, `<` and `>` are refused inside an unquoted value, so `a=1+b=2` and `a=b=c` are errors (`unexpectedChar`). A value that holds one is quoted: `url="https://x/?a=b"`. |

```go
// One at a time, or both.
expr, err := query.Parse(q, query.WithExplicitAnd())
expr, err := query.Parse(q, query.WithStrictValues())
expr, err := query.Parse(q, query.WithStrict())

// The same switches on the parser and the evaluator.
expr, err := parser.ParseWith(q, parser.Config{MaxLength: 512, ExplicitAnd: true, StrictValues: true})
prog, err := eval.Compile(q, fields, eval.WithStrict())
ok, err := query.MatchWith(q, fields, record, query.WithStrict())
```

A strict option only refuses queries. It never changes what an accepted query
means, so a query that parses under `WithStrict` parses to the same tree under
the default grammar, and a host can move from lenient to strict, or back,
without any stored query changing its meaning.

With `ExplicitAnd` the grammar's one optional connective becomes required:

```ebnf
and_expr   = chain { "AND" chain } ;
```

The TypeScript client and the C library take the same switches:

```ts
q.parse("a=1 b=2", { explicitAnd: true, strictValues: true });   // missingAnd
q.parseAndValidate(text, fields, { maxLength: 512, explicitAnd: true });
```

```json
{"query": "a=1 b=2", "explicitAnd": true, "strictValues": true}
```

A switch that is not a boolean, or an option the call does not know, is an
error in both, so a host cannot believe strict parsing is on when it is not.
That includes passing a switch to a call that does not take one.

`parse`, `parseAndValidate` and `match` take them. `validate` and `stringify`
work on a tree that is already parsed, and `eval` takes a value, not a
condition, so the switches do not apply there.

## What is not in the grammar

These are deliberate, and each has a way to say the same thing:

| Not supported | Write instead |
|---|---|
| `+` or `&&` between conditions | `AND` |
| `-field=x` or `!field=x` to negate | `NOT field=x` |
| `labels["team"]=x` | `labels.team=x`, or `["labels.weird key"]=x` for a name a bare word cannot spell |
| A selector after a selector: `forks@(name=a).env@(name=b)` | nest it: `forks@(name=a AND env@(name=b))` |
| A wildcard in a field name: `labels.*=x` | a list of pairs: `labels@(value=x)`, when the host declares the field that way |
| `a*b*`, a star in the middle and the end | `contains()`, `startsWith()` and `endsWith()` joined with `AND` |
| Sorting and paging | they are not conditions; a host takes them as separate parameters |
