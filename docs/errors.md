---
title: Errors and hints
nav_order: 3.5
permalink: /errors/
---

# Errors and hints
{: .no_toc }

1. TOC
{:toc}

When a query is rejected, each failure tells you four things.

| Part | What it is | Stable? |
|---|---|---|
| Code | A machine identifier, such as `unexpectedChar` or `fieldNotFound`. | Yes. A code never changes once shipped. |
| Position | The offset and length of the offending text, in bytes. | Yes. |
| Message | What is wrong, in English. | No. It may be reworded. |
| Hint | What you most likely meant and how to write it. Not every error has one. | No. It may be reworded. |

Branch on the code. Show the message and the hint to a person, or write your own wording from the code.

```
error [unexpected]: unexpected "*" after a complete condition
  [base]*1.1 > 3
        ^
  hint: arithmetic goes on the right of the operator or inside a function: float([base]*1.1)>3
```

## Hints that rewrite your query

When the fix is mechanical, the hint shows your own query corrected. A rewrite is only offered if it is itself a valid query.

| You wrote | Hint |
|---|---|
| `status==active` | equality is a single =: `status=active` |
| `total=>5` | "greater than or equal" is written >=: `total>=5` |
| `a<>b` | "not equal" is written !=: `a!=b` |
| `status=active && priority>3` | join conditions with AND: `status=active AND priority>3` |
| `!state=draft` | negate a condition with NOT: `NOT state=draft` |
| `priority BETWEEN 1 AND 5` | a range is written with a colon and two dots: `priority:1..5` |
| `3<priority` | the field goes on the left: `priority>3` |
| `now()-7d<created_at` | the field goes on the left and arithmetic on the right: `created_at>now()-7d` |
| `[base]*1.1 > 3` | arithmetic goes on the right of the operator or inside a function: `float([base]*1.1)>3` |
| `total:1-5` | a range uses two dots: `total:1..5` |
| `name:john` | ":" starts a range such as name:1..5; to compare, write `name=john` |

## Hints from your field list

Validation knows your fields, so its hints can name them.

| You wrote | Hint |
|---|---|
| `totl>5` | did you mean "total"? |
| `lables.team=x` | did you mean "labels.team"? |
| `total>base` | to compare with the field base, write it in brackets: `[base]` |
| `abs(total-base)>3` | names may contain - and /, so this reads as one field; for arithmetic write the fields in brackets: `abs([total]-[base])` |
| `contains(tags, urgent)` | a bare word in a function is a field; to pass text to contains(), put it in quotes: `"urgent"` |
| `state=dr*` (state allows only `=` and `!=`) | state cannot be matched with a * pattern; it allows: =, != |
| `active=yes` | a boolean is true or false |
| `ttl>1h30m` | a duration has one unit: write 90m rather than 1h30m |
| `total>1,000` | write a number without separators: 1000 rather than 1,000 |
| `name IS NULL` | to test for a missing value write NOT field; for a present one write the field alone |
| `uper(name)=X` | unknown function "uper" (did you mean "upper"?) |

A hint only names fields and functions the caller is allowed to use. A field hidden by `WithAllowedFields` or by a custom validator is never suggested, so a restriction cannot be read back out of the error messages.

## Where to find them

**Go.** `parser.Error` and `validate.Error` each have a `Hint` field. `Error()` puts the hint after the message in parentheses.

```go
_, err := query.ParseAndValidate("totl>5", fields)

var invalid validate.ErrorList
if errors.As(err, &invalid) {
    for _, e := range invalid {
        // e.Kind.Code(), e.Position.Offset, e.Message, e.Hint
    }
}
for _, e := range parser.Errors(err) {
    // e.Code, e.Position.Offset, e.Message, e.Hint
}
```

**TypeScript and the C library.** Each entry in `errors` and `validationErrors` has a `hint` when there is one. The key is absent otherwise.

```json
{
  "code": "fieldNotFound",
  "message": "unknown field \"totl\"",
  "offset": 0,
  "length": 4,
  "field": "totl",
  "op": "",
  "hint": "did you mean \"total\"?"
}
```

**Command line.** `query explain` prints the hint under the marker. See [Command line]({{ '/cli/' | relative_url }}).

**Playground.** The [playground]({{ '/playground/' | relative_url }}) shows the hint under each error.

## Limits

- Only the first eight errors of a query get a hint. A hint reads the query around its error, so this keeps a long failing query cheap when the length limit is lifted.
- A run of the same unexpected character is one error that spans the run: `&&` is reported once, not twice.
- An unknown field with no close match gets no hint, unless the field list is short enough to print (eight fields or fewer).

## Mistakes that are not errors

Some mistakes produce a valid query that means something else, so there is no error to attach a hint to. See [when a word is a field and when it is text]({{ '/syntax/' | relative_url }}#when-a-word-is-a-field-and-when-it-is-text).
