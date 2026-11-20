---
title: Grammar
nav_order: 4
parent: Language
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
and_expr   = unary { [ "AND" ] unary } ;       (* see "Implicit AND" below *)

unary      = "NOT" unary | chain ;             (* NOT a@first is NOT (a@first) *)
chain      = term { "@" selector } ;
selector   = "first" | "last"
           | [ "any" | "all" | "none" ] "(" or_expr ")" ;

term       = "(" or_expr ")"
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

time       = digit digit ":" digit digit ":" digit digit [ "." digit { digit } ] ;
offset     = ( "+" | "-" ) digit digit ":" digit digit ;
letter     = "a" … "z" | "A" … "Z" ;
digit      = "0" … "9" ;
non_ascii  = ? any character outside ASCII that is not white space ? ;
char       = ? any character except '"' and '\' ? ;
value_char = ? any character except white space, ")", "[", "]" and '"',
               and except "," inside an IN list or a call ? | escape ;
escape     = '\*' | '\\' | '\(' | '\)' ;
```

An unquoted value ends at a space, `)`, `[`, `]`, `..`, and at a `,` inside
an `IN` list or a call. Inside one, `\*`, `\\`, `\(` and `\)` stand for the
literal character. Anything else goes in quotes.

### Keywords are not reserved

`AND`, `OR`, `NOT` and `IN` are keywords only where a connective can stand.
Everywhere else they are ordinary words:

- **After an operator there are no keywords.** `name=AND` and
  `name IN (and, or, not, in)` compare with text.
- **A keyword is a field name when something names it as one:** an operator,
  a dot, `@` or `IN` directly after it (`not=1`, `not.x=1`, `not@first`), or
  a dot before it (`labels.and=x`).
- **A longer word is not a keyword.** `NOTname=a` is the field `NOTname`.
- **A keyword cannot name a function.** `not(x)` is a negated group.

Under the default grammar a `NOT` with nothing after it is also read as a
word (`NOT` alone is the term `not`). `ExplicitAnd` refuses that form.

### The type of an unquoted value

A value's type comes from how it is written, not from the field. The steps
below are tried in order, and the first that applies decides:

| Step | If the value | It is | Examples |
|---|---|---|---|
| 1 | is a complete date, `YYYY-MM-DD`, alone or followed by `T` and a time | a date | `2026-11-16`, `2026-11-16T10:00:00Z` |
| 2 | holds a word: letters that are not a function call and not one of the units `d` `h` `m` `w` directly after a digit | text, or a pattern if a `*` starts or ends it | `10-x`, `pr-10-3`, `5+3x`, `1e5`, `7days`, `a/b`, `TRUE`, `check*` |
| 3 | holds a number with more than one dot | text | `1.2.3`, `10.0.0.1/24` |
| 4 | has `+ - * / %` with a number, a date, a duration, a call or a group on both sides | arithmetic | `10-3`, `10/2`, `7d-1h`, `2026-11-5`, `now()-7d`, `2026-11-16T10:00:00Z-1h` |
| 5 | is `true` or `false`, in lower case | a boolean | `true` |
| 6 | is digits and one unit | a duration | `7d`, `30m` |
| 7 | is a number, with or without a sign | an integer or a float | `42`, `-5`, `1.5`, `007` |
| 8 | has a `*` at one end or both | a pattern | `10*` |
| 9 | is anything else | text | `5-`, `10%`, `1h30m`, `1.5h` |

Of the values made of nothing but digits and hyphens, a complete date is the
only one that is not a subtraction, and it takes all ten characters:
`2026-11-16` is a date, while `2026-11-5` and `2026-1` are subtractions like
`10-3`. (A hyphen is also plain text inside a word or a version, by steps 2
and 3: `pr-10-3`, `1.2.3-4`.) A date that is written in full but cannot
exist, such as `2026-13-45`, is an error.

A date with a time is one operand, so `2026-11-16T10:00:00Z-1h` is that
instant minus an hour. Its zone offset belongs to it: `2026-11-16T10:00:00-05:00`
is an instant, and `2026-11-16T10:00:00-05:00+30m` adds half an hour to it.

A star makes a pattern only out of text. After arithmetic it is an error,
not a pattern: `name=2026-11*` and `n=10-3*` do not parse.

A date field is the one place a date written short is taken as a date. The
lexer cannot see the field, so it reads `2026-11-5` as a subtraction; the
validator can, and on a date field takes it as the day it names. Every other
field keeps the subtraction and judges it as one:

| Field type | `2026-11-5` | `2026-11` | `10-3` | `"2026-11-5"` in quotes |
|---|---|---|---|---|
| date, datetime | the day 2026-11-05 | refused: a date needs a day | refused: it gives a number | the day 2026-11-05 |
| text | refused: quote it | refused: quote it | refused: quote it | the text `2026-11-5` |
| integer, decimal | refused: write the number itself, `2010` | refused: `2015` | the number 7 | refused: text |
| duration, boolean | refused | refused | refused | refused: text |
| a function's argument, or a value compared with a function | refused | refused | the number 7 | the text `2026-11-5` |

The last row is where no declared type says what the run is:
`contains(name, 2026-11)` and `upper(name)=2026-11-5` are refused, with the
three ways to say what was meant: the date in full, the text in quotes, or
the subtraction with spaces around the minus.

A text field treats `2026-11-5` and `10-3` alike: both are arithmetic, and
both are written in quotes when the characters are meant. A number field
refuses the date-shaped run because nobody writes 2015 as `2026-11`.

A date written short is exactly one unbroken run: four digits, then one or
two groups of a hyphen and one or two digits, the first from 1 to 12 and the
second from 1 to 31. Anything else is an ordinary subtraction on every
field:

- a number that could not be a month or a day: `1500-50` is 1450;

- a space or a parenthesis: `2026 - 11`, `(2026)-11` and `(2026-11)` are
  the number 2015;
- more after it: date arithmetic takes the date in full, `2026-11-05-1d`
  and not `2026-11-5-1d`;
- a time: `2026-11-5T10:00:00Z` is not a date. A date with a time is
  written in full.

A date written short that cannot exist, such as `2026-2-30`, is refused.
Once validated, the value is that date: it prints as `2026-11-05`.

With no schema there is no field to ask, so the run is the subtraction the
grammar reads: `query.Eval("2026-11")` is 2015.

Arithmetic over literals is checked against the field as well. Its result
must be of the field's kind (`ttl>10-3` compares a duration with 7), its
operands must go together (`10-7d`, and `7d-1.5h`, where `1.5h` is text), a
divisor written as zero is refused (`n=10/0`), and a field of whole numbers
refuses a fraction (`n=10/4`). Arithmetic that reads a field or calls a
function is left to the evaluator.

A value that has the shape of a number, a date, a duration or arithmetic and
is meant as text goes in quotes: `name="10-3"`. A field declared as text
refuses arithmetic rather than guess, and takes a lone number, date or
duration as its text.

These characters are text inside an unquoted value: `!` `:` `@` `+` `-` `/`
`%`, and `,` outside an `IN` list or a call. These are not: `=` `<` `>` end
the default grammar's reading in surprising ways and are refused under
`StrictValues`, even after a backslash.

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
| `a=now()x`, `a=[b]c`, `a=2026-11-16[b]` | an error: a value glued to the next term | `a=now() AND x` | a space before the next condition, an operator between two values, or quotes |
| `labels[team]=x` | `labels AND [team]=x` | a keyed lookup | write `labels.team=x`; `ExplicitAnd` refuses this form |

The last two rows are the ones that surprise people. Both come from being
lenient, and both are refused under the strict options.

## Strict options

The default grammar is lenient where someone typing into a search box
benefits. A host that takes queries from scripts, saved links or other
programs can turn the lenient forms off:

| Option | Default | When on |
|---|---|---|
| `ExplicitAnd` | off | A space is no longer an `AND`. `a=1 b=2`, `frontend checkout` and `labels[team]=x` are errors with the code `missingAnd`. Write `a=1 AND b=2`; quote text that has a space. A `NOT` with nothing to negate (`a=1 AND NOT`, `NOT` alone) is an error too, as a dangling `AND` already is; the text is searched for as `"not"`. |
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
and_expr   = unary { "AND" unary } ;
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
