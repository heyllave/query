---
title: TypeScript
nav_order: 5
permalink: /typescript/
---

# TypeScript and JavaScript
{: .no_toc }

1. TOC
{:toc}

The package in [`clients/npm`](https://github.com/heyllave/query/tree/main/clients/npm) runs the Go engine as WebAssembly, in the browser and in Node.js. It ships full type definitions for the tree.

## Build

```bash
cd clients/npm
npm install
npm run build    # builds query.wasm with Go, then compiles the TypeScript
```

The build needs Go 1.25 or newer. It produces `query.wasm` and `dist/`.

## Load

```ts
import { loadQuery } from "@trazo/query";

const q = await loadQuery(new URL("@trazo/query/query.wasm", import.meta.url));
```

`loadQuery` accepts a URL or path, a `Response` (or a promise of one, such as `fetch(...)`), or the bytes themselves. Load once and reuse the result.

## Describe your fields

```ts
const fields = [
  { Name: "state", Type: "text" },
  { Name: "total", Type: "decimal" },
  { Name: "labels", Type: "text", Nested: true },
];
```

`Type` is one of `text`, `integer`, `decimal`, `boolean`, `date`, `datetime`, `duration` or `list`. A field gets the usual operators for its type unless you list them in `AllowedOps`.

A `list` field declares the fields of one element in `Elements`, and the validator then checks a selector and the condition inside it:

```ts
const fields = [
  { Name: "name", Type: "text" },
  {
    Name: "orders",
    Type: "list",
    Elements: [
      { Name: "status", Type: "text" },
      { Name: "price", Type: "decimal" },
    ],
  },
];

q.parseAndValidate("orders@(status=shipped)", fields); // ok
q.parseAndValidate("orders@(bogus=1)", fields);        // unknown field "bogus" on "orders"
q.parseAndValidate("name@first", fields);              // "name" is not a list field
q.parseAndValidate("orders@last", fields);             // selector @last is not allowed
```

A list accepts `first`, `@(...)`, `all` and `none`. To change that, name the selectors in `Selectors`, with `""` for `@(...)`: `Selectors: ["first", "last", "", "any"]`. [List fields]({{ '/go/' | relative_url }}#list-fields) on the Go page has the rules.

## Parse, validate, match, evaluate

```ts
const parsed = q.parseAndValidate("state=draft AND total>50000", fields);
if (parsed.error) {
  // parsed.errors: syntax failures; parsed.validationErrors: validation failures
} else {
  parsed.result; // the tree
}

q.match("state=draft AND total>50000", fields, { state: "draft", total: 60000 });
// { result: true }

q.eval("[total]*1.1", fields, { total: 100 });
// { result: 110.00000000000001 }

q.stringify(parsed.result!);
// { result: "state=draft AND total>50000" }
```

No call throws on a bad query. Every result is an object with either `result` or `error`.

## Length limit

Every call takes an optional last argument, `maxLength`: the query length limit in characters. It defaults to 256 and `0` disables it. `validate` and `stringify` take it too, because a tree is held to the same limit as the query it came from: pass the limit the tree was parsed under.

`parse`, `parseAndValidate` and `match` accept an object in its place, which also carries the strict switches: `q.parse(text, { maxLength: 512, explicitAnd: true, strictValues: true })`. With `explicitAnd`, `a=1 b=2` fails with the code `missingAnd`. See [Grammar](../grammar/#strict-options).

`validate`, `stringify` and `eval` accept `{ maxLength }` as an object too, and nothing else: they have no condition grammar to be strict about.

What a call does not understand is an error, not a default, so a host cannot believe strict parsing is on when it is not. That covers an option the call does not take (`{ strict: true }`, or `explicitAnd` passed to `eval`), a switch that is not a boolean, an argument that is neither a number nor an object (`q.parse(text, true)`), and an argument after the options.

```ts
const long = q.parse(text, 1000);
q.validate(long.result!, fields, 1000);
q.stringify(long.result!, 1000);
```

## Show errors to a person

Each failure carries a stable `code`, the span to underline and, when the engine can tell what was meant, a `hint` such as `did you mean "total"?`. See [Errors and hints]({{ '/errors/' | relative_url }}).

```ts
for (const e of parsed.errors ?? []) {
  underline(e.offset, e.length);
  show(myMessages[e.code] ?? e.message);
}
```

The codes never change once shipped. The English `message` may be reworded, so use it for logs and as a fallback.

## Walk the tree

```ts
import { visit, walk, fields as fieldsOf, type Visitor } from "@trazo/query";

fieldsOf(parsed.result!); // every field the query reads

const toText: Visitor<string> = {
  visitBinary: (e) => `(${visit(toText, e.left)} ${e.op} ${visit(toText, e.right)})`,
  visitUnary: (e) => `NOT ${visit(toText, e.expr)}`,
  visitQualifier: (e) => `${e.field.join(".")} ${e.op} ?`,
  visitPresence: (e) => `${e.field.join(".")} IS NOT NULL`,
  visitCall: (e) => e.call, // a function used as a predicate, in source form
  visitGroup: (e) => visit(toText, e.expr),
  visitSelector: (e) => `${e.selector}(${e.inner ? visit(toText, e.inner) : ""})`,
};
```

Every node type is defined in [`src/types.ts`](https://github.com/heyllave/query/blob/main/clients/npm/src/types.ts).

## Same answers as Go

The JavaScript client runs the [shared corpus](https://github.com/heyllave/query/blob/main/conformance/corpus.json) on every push, as do the Go API and the C library. A query that matches a record in Go matches it in the browser.
