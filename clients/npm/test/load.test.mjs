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
