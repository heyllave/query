// The published entry point: `loadQuery` from the built package, fed each kind
// of source a consumer has — a path, the bytes, a fetch Response — with no
// wasm_exec.js loaded beforehand. Run `npm run build` first.

import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const wasmPath = join(here, "..", "query.wasm");
const { loadQuery } = await import(pathToFileURL(join(here, "..", "dist", "index.js")).href);

const sources = {
  path: () => wasmPath,
  "file URL": () => pathToFileURL(wasmPath),
  bytes: () => readFileSync(wasmPath),
  Response: () =>
    new Response(readFileSync(wasmPath), { headers: { "content-type": "application/wasm" } }),
  "Response without a wasm content type": () => Promise.resolve(new Response(readFileSync(wasmPath))),
};

for (const [name, source] of Object.entries(sources)) {
  test(`loadQuery reads the engine from a ${name}`, async () => {
    const q = await loadQuery(source());
    assert.equal(q.parse("state=draft").result.type, "qualifier");
    assert.equal(q.stringify(q.parse("total:1..5").result).result, "total:1..5");
  });
}

test("loadQuery refuses a failed download", async () => {
  await assert.rejects(loadQuery(new Response("missing", { status: 404 })), /HTTP 404/);
});

// The typed API over the bridge: complete, offer and applyCompletion, used
// the way a search box uses them, with the candidates for a value coming
// from somewhere the schema does not know about.
test("complete, offer and applyCompletion write a query from suggestions", async () => {
  const { applyCompletion } = await import(pathToFileURL(join(here, "..", "dist", "index.js")).href);
  const q = await loadQuery(wasmPath);
  const fields = [
    { Name: "cluster", Type: "text" },
    { Name: "labels", Type: "text", Nested: true },
    { Name: "ready", Type: "boolean" },
  ];
  const strict = { explicitAnd: true, strictValues: true };
  const clusters = ["prod", "staging us", "🚀 lab"];

  let text = "clu";
  let cursor = text.length;
  let c = q.complete(text, cursor, fields, strict).result;
  assert.equal(c.slot, "field");
  ({ query: text, cursor } = applyCompletion(text, c, c.items[0]));
  assert.equal(text, "cluster");

  text += "=st";
  cursor = text.length;
  c = q.complete(text, cursor, fields, strict).result;
  assert.equal(c.slot, "value");
  assert.equal(c.field, "cluster");
  assert.equal(c.prefix, "st");
  assert.deepEqual(c.items, []);
  const offered = q.offer(c, clusters.filter((name) => name.startsWith(c.prefix))).result;
  assert.deepEqual(offered, [{ label: "staging us", text: '"staging us"', kind: "value" }]);
  ({ query: text, cursor } = applyCompletion(text, c, offered[0]));
  assert.equal(text, 'cluster="staging us"');
  assert.equal(cursor, text.length);

  text += " AND labels.";
  cursor = text.length;
  c = q.complete(text, cursor, fields, strict).result;
  assert.equal(c.slot, "key");
  const keys = q.offer(c, ["team", "my key"]).result;
  ({ query: text, cursor } = applyCompletion(text, c, keys[1]));
  assert.equal(text, 'cluster="staging us" AND ["labels.my key"]');

  text += "=x OR cluster=";
  c = q.complete(text, text.length, fields, strict).result;
  ({ query: text, cursor } = applyCompletion(text, c, q.offer(c, clusters).result[2]));
  assert.equal(text, 'cluster="staging us" AND ["labels.my key"]=x OR cluster="🚀 lab"');
  assert.equal(cursor, text.length);

  assert.equal(q.parseAndValidate(text, fields, strict).error, undefined);

  // Replacing a word in the middle keeps what follows it.
  const mid = q.complete("read=true AND cluster=prod", 2, fields, strict).result;
  assert.deepEqual(applyCompletion("read=true AND cluster=prod", mid, mid.items[0]), {
    query: "ready=true AND cluster=prod",
    cursor: 5,
  });

  assert.match(q.complete("cluster=", 99, fields).error, /is not a position in the query/);
  assert.match(q.offer({ slot: "bogus" }, []).error, /unknown slot/);
});
