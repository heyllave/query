---
title: Syntax
nav_order: 3
permalink: /syntax/
---

# Syntax
{: .no_toc }

1. TOC
{:toc}

## Comparisons

```
state=draft                    # equal
state!=cancelled               # not equal
year>2020                      # also >=, <, <=
total!>50000                   # negated comparison, the same as NOT total>50000
created_at:2026-01-01..2026-03-31   # inclusive range
state IN (draft, issued, paid)      # any of
state NOT IN (draft, issued)        # none of
tire_size                      # presence: the field exists and is not empty
```

Text equality and wildcards ignore case: `state=Draft` matches `"draft"`.

`==`, `=>` and `<>` are parse errors. A value that really starts with `=`, `<` or `>` is written in quotes: `note="=draft"`.

## Combining

```
state=draft AND total>1000
state=draft total>1000                         # a space is an implicit AND
(state=draft OR state=issued) AND total>50000
NOT state=cancelled
```

`AND` binds tighter than `OR`. The keywords `AND`, `OR`, `NOT` and `IN` are accepted in any case.

## Values

| Kind | Examples | Notes |
|---|---|---|
| Text | `draft`, `"John Doe"`, `customer_john-doe` | Quote a value that has spaces or special characters. |
| Number | `42`, `-3`, `1.5` | Only ASCII digits make a number. |
| Boolean | `true`, `false` | |
| Date | `2026-01-01` | A date without a time means the whole UTC day. |
| Datetime | `2026-01-01T10:00:00Z` | RFC 3339. |
| Duration | `30m`, `4h`, `1d` | One unit per literal. |
| Wildcard | `Jo*`, `*son`, `*oh*` | `\*` is a literal star. |

An unquoted value that contains letters is always text, so UUIDs, versions, commit hashes and e-mail addresses are one value:

```
id=550e8400-e29b-41d4-a716-446655440000
version=v1.2.3
```

A literal follows the field's declared type. `zip=90210` on a text field compares the text `"90210"`.

## Field names

```
labels.dev=jane                       # nested field
labels.example.com/team=platform      # dots, slashes and hyphens are part of a name
["billing address"]=remote            # bracket-quoted name for anything else
```

A dotted name reads the flat key `"labels.dev"` first, then descends into nested maps.

## Arithmetic and field references

Arithmetic is allowed on the value side. A field used as an operand is written in brackets.

```
total>=(50000+1000)*1.1
total>=[base]*1.1
cpu>[cpu_limit]
created_at>=now()-7d
```

Operators are `+ - * / %`, with the usual precedence and parentheses. A bare word is never a field reference in arithmetic, because it would collide with hyphenated names such as `customer-id`.

## Functions

A function can wrap the field, sit on the value side, or stand alone as a predicate.

```
lower(name)=john*
len(name)>5
created_at>=daysAgo(30)
contains(tags, "urgent")
coalesce(nickname, name)="John"
```

| Family | Functions |
|---|---|
| Text | `lower`, `upper`, `trim`, `len`, `contains`, `startsWith`, `endsWith` |
| Date | `now`, `today`, `daysAgo`, `year`, `month`, `day`, `hour`, `minute`, `second`, `weekday`, `isBusinessDay`, `addDays`, `addBusinessDays` |
| Number | `abs`, `ceil`, `floor`, `round`, `sqrt`, `pow`, `min`, `max` |
| List | `count`, `sum`, `avg`, `first`, `last` |
| Other | `coalesce`, `if`, `int`, `float`, `string` |

An unknown function or a wrong number of arguments is a compile error. The host can register its own functions, remove the built-ins, or allow only a named subset.

## Lists

A selector applies a condition to the elements of a list field.

```
orders@first                     # the list has at least one element
orders@(status=shipped)          # at least one element matches (also @any)
orders@all(price>0)              # every element matches
orders@none(status=cancelled)    # no element matches
```

| Selector | Empty list | Missing field |
|---|---|---|
| `@first`, `@last` | false | false |
| `@(...)`, `@any(...)` | false | false |
| `@all(...)` | true | false |
| `@none(...)` | true | true |

## Missing and unreadable values

- A missing or nil field makes every comparison false, including through a function: `len(name)>3` does not match a record without a name.
- The negated operators are the exception, because `total!>5` is `NOT total>5`. It is true for a record without a total.
- `NOT IN` follows SQL: `state NOT IN (a, b)` is false without a state.
- A value that cannot be read as the literal's type never compares. `n=0`, `n!=0` and `n<1` are all false when the record's `n` is `"abc"`.

## Limits

- A query is at most 256 bytes unless the host raises the limit.
- Groups, negations, selectors and calls nest at most 128 levels deep.
- Day arithmetic accepts at most about 10,000 years in either direction.
