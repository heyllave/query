---
title: Completing a query as it is typed
nav_order: 3
parent: Guides
permalink: /completion/
---

# Completing a query as it is typed
{: .no_toc }

A search box that suggests the next thing to write: field names, the operators a field allows, values. The engine works out what belongs at the cursor from the same schema that validates the query, so a suggestion is never something validation would refuse. Values that live in your own data come from you, and the engine writes them the way the grammar needs.

1. TOC
{:toc}

## The call

Completion is one function. It takes the text, the cursor and your fields, and answers from those alone: no network, no state, fast enough to run on every keystroke.

```ts
const c = q.complete(text, input.selectionStart, fields, strict).result;
// c.slot     what belongs here: "field", "operator", "value", ...
// c.items    the suggestions the schema can make
// c.replace  the span of the text an accepted item goes in place of
```

Pass the options you parse with. Under the strict grammar a word after a complete condition can only be `AND` or `OR`; under the default grammar it may also start the next condition, so field names are offered there too.

An unfinished query is not an error. A position the engine cannot place, such as after `name==`, has the slot `none` and no items.

## What it suggests

| The cursor is | `slot` | Items |
|---|---|---|
| At the start, or after `AND`, `OR`, `NOT` or `(` | `field` | Field names, then `NOT`. |
| After a field name and a space | `operator` | The operators the field allows, `IN (`, and a list's selectors. `AND` and `OR` too, when the bare name is a condition by itself. After `name NOT`, only `IN (`. |
| After the `@` of a list field | `selector` | The selectors the list allows. |
| Inside `forks@(` | `field` | The fields of one element of the list. |
| After an operator, or inside an `IN` list | `value` | `true` and `false` for a boolean, and the field's `Values`. Empty when only you know them. |
| After a nested field's dot (`labels.`) | `key` | Empty: the keys are yours to supply. |
| After a complete condition | `connective` | `AND`, `OR`, and `)` inside an open group. |
| At an argument of a function | `argument` | Field names. |

Items come best first: names that start with what was typed, then names that only contain it, each group in the order of your schema.

An item never repeats what the query already has after the cursor. A nested field's `labels.` loses its dot unless it is the last thing in the text, and `@all(` loses its parenthesis before one that is already there.

## Values the schema knows

List a field's values in its config and they are offered after its operators:

```ts
const fields = [
  { Name: "status", Type: "text", Values: ["ready", "not ready", "failed"] },
  { Name: "ready", Type: "boolean" },
];

q.complete("status=", 7, fields).result.items;
// [ { label: "ready",     text: "ready",         kind: "value" },
//   { label: "not ready", text: '"not ready"',   kind: "value" },
//   { label: "failed",    text: "failed",        kind: "value" } ]
```

Show `label` and insert `text`. They differ when the value would not read back as itself: `not ready` has a space, so it is quoted.

`Values` are suggestions. Validation does not hold a query to them, so the list does not have to be complete.

## Values from your own data

Cluster names, label keys and owners are not in a schema. For those the completion says what is being asked for, and you fetch the candidates from wherever you keep them:

```ts
async function suggest(text: string, cursor: number) {
  const c = q.complete(text, cursor, fields, strict).result!;
  if (c.slot === "value" && c.field === "cluster") {
    const names = await api.clusters({ startsWith: c.prefix });
    return { ...c, items: q.offer(c, names).result! };
  }
  if (c.slot === "key" && c.field === "labels") {
    const keys = await api.labelKeys({ startsWith: c.prefix });
    return { ...c, items: q.offer(c, keys).result! };
  }
  return c;
}
```

`offer` writes each candidate the way the grammar needs it at that position:

| Candidate | Where | Written as | Why |
|---|---|---|---|
| `prod` | a text value | `prod` | Reads back as itself. |
| `staging us` | a text value | `"staging us"` | A space ends an unquoted value. |
| `812` | a text value | `"812"` | Bare, it is a number. |
| `812` | an integer field's value | `812` | The field wants a number. |
| `a*b` | a text value | `"a*b"` | Bare, the star is a wildcard. |
| `team` | a key of `labels` | `labels.team` | |
| `my key` | a key of `labels` | `["labels.my key"]` | A name that is not a plain identifier takes the bracket form. |

The candidates are kept in the order you give them: you have already chosen and ranked them. `c.prefix` is what has been typed so far, for you to filter by; for a key it is the key alone, without `labels.`.

Two more members say what kind of value belongs:

- `c.function` is set, and `c.field` is not, when the value is compared with a function's result: `len(name)>`. The engine does not know what a function gives, so `offer` writes a candidate that reads as a number, a date or a duration as it is (`5`, not `"5"`) and quotes only what must be text.
- `c.arithmetic` is true after an arithmetic sign, as in `createdAt>now()-`. Only a number, a date or a duration can stand there, so `offer` leaves out any other candidate.

`c.lists` names the list selectors the cursor is inside, outermost first. Inside `forks@(kind=` the field is `kind` and `lists` is `["forks"]`, so you can tell an element's `kind` from a top-level field of the same name.

## Accepting a suggestion

Put the item's `text` in place of `c.replace`:

```ts
import { applyCompletion } from "@trazo/query";

const next = applyCompletion(text, c, item);
input.value = next.query;
input.setSelectionRange(next.cursor, next.cursor);
```

`replace` covers the whole word under the cursor, not only the part before it, so accepting `status` with the cursor in the middle of `sta|t=x` gives `status=x`. Inside an open quote it starts at the quote, and every `text` is a whole quoted string. For a key it covers the whole name, because the name may need rewriting in the bracket form. A value that is the name of a call is replaced with its arguments.

In the TypeScript client the cursor and `replace` index the string as `selectionStart` does. The Go API counts bytes. The C library counts bytes unless the request says `"utf16": true`.

## When answers arrive out of order

A fetch for `clu` can come back after the person has typed `cluster=p`. Complete again when the answer arrives, and drop it if the question has changed:

```ts
let latest = 0;
input.addEventListener("input", async () => {
  const mine = ++latest;
  const c = await suggest(input.value, input.selectionStart!);
  if (mine === latest) show(c);
});
```

The engine's part is immediate, so show `c.items` at once and add the fetched ones when they come.

## From Go

```go
c := query.Complete(text, cursor, fields, query.WithStrict())

items := c.Items
if c.Slot == parser.SlotValue && c.Field == "cluster" {
    items = c.Offer(clusterNames(c.Prefix)...)
}
```

`complete.ValueText` and `complete.FieldText` are the two writers `Offer` uses, for a host that builds query text some other way: `complete.ValueText("staging us")` is `"staging us"` in quotes, and `complete.FieldText("labels.my key")` is `["labels.my key"]`.

## What it does not do

- It does not suggest function names, or values for a date or a duration beyond the ones you list.
- It does not apply a validator's restrictions (`WithAllowedFields`, `WithAllowedOps`). Pass the fields the caller may use.
- Inside a function's arguments it offers field names for a fresh argument and nothing after one.
- Past the query length limit (256 characters unless you raise it) it offers nothing, since the query would be refused whole. Pass the same limit you parse with.
- Under the strict grammar it offers nothing where only a connective can come and something else was typed, as after `name=x NOT`.
- After text that does not lex, such as a stray `#`, it offers nothing until the text is fixed. [Errors and hints]({{ '/errors/' | relative_url }}) covers what to show then.
