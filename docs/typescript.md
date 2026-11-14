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

`Type` is one of `text`, `integer`, `decimal`, `boolean`, `date`, `datetime` or `duration`. A field gets the usual operators for its type unless you list them in `AllowedOps`.

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

## Show errors to a person

Each failure carries a stable `code` and the span to underline.

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
  visitGroup: (e) => visit(toText, e.expr),
  visitSelector: (e) => `${e.selector}(${e.inner ? visit(toText, e.inner) : ""})`,
};
```

Every node type is defined in [`src/types.ts`](https://github.com/heyllave/query/blob/main/clients/npm/src/types.ts).

## Same answers as Go

The JavaScript client runs the [shared corpus](https://github.com/heyllave/query/blob/main/conformance/corpus.json) on every push, as do the Go API and the C library. A query that matches a record in Go matches it in the browser.
