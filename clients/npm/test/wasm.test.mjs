// End-to-end tests for the query WASM bridge, exercised through the Go binary
// from JavaScript. Run with the project's zero-dependency Node test runner:
//
//   make -C wasm build   # produces clients/npm/query.wasm + src/wasm_exec.js
//   node --test
//
// These prove the JS <-> WASM round-trip for every exported function, including
// the match/eval bridges, not just the Go-side unit tests.

import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const wasmPath = join(here, "..", "query.wasm");
const wasmExecPath = join(here, "..", "src", "wasm_exec.js");

// Load Go's wasm_exec.js shim (defines globalThis.Go) and instantiate the
// module once for the whole suite.
async function loadWasm() {
  await import(wasmExecPath);
  const go = new globalThis.Go();
  const bytes = readFileSync(wasmPath);
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // main uses select{}; never resolves — do not await
  return globalThis;
}

const fields = [
  { Name: "state", Type: 0, AllowedOps: ["=", "!=", "*", "?"] }, // TypeText
  { Name: "total", Type: 2, AllowedOps: ["=", "!=", ">", ">=", "<", "<=", ".."] }, // TypeDecimal
  { Name: "base", Type: 2, AllowedOps: ["=", "!=", ">", ">=", "<", "<=", ".."] },
];

const w = await loadWasm();

test("queryParse returns an AST", () => {
  const { result, error } = w.queryParse("state=draft AND total>50000");
  assert.equal(error, undefined);
  assert.equal(result.type, "binary");
});

test("queryParse reports a parse error", () => {
  const { error } = w.queryParse("=invalid");
  assert.ok(error, "expected a parse error");
});

test("queryParseAndValidate accepts a valid query", () => {
  const { result, error } = w.queryParseAndValidate(
    "state=draft",
    JSON.stringify(fields)
  );
  assert.equal(error, undefined);
  assert.equal(result.type, "qualifier");
});

test("queryValidate flags an unknown field", () => {
  const { result: ast } = w.queryParse("state=draft");
  const v = w.queryValidate(JSON.stringify(ast), JSON.stringify([]));
  assert.equal(v.valid, false);
  assert.ok(v.errors.length > 0);
});

test("queryStringify round-trips a query", () => {
  const { result: ast } = w.queryParse("state=draft AND total>50000");
  const { result: str } = w.queryStringify(JSON.stringify(ast));
  assert.equal(str, "state=draft AND total>50000");
});

test("queryMatch evaluates a boolean predicate", () => {
  const f = JSON.stringify(fields);
  const yes = w.queryMatch(
    "state=draft AND total>50000",
    f,
    JSON.stringify({ state: "draft", total: 60000 })
  );
  assert.equal(yes.error, undefined);
  assert.equal(yes.result, true);

  const no = w.queryMatch(
    "total>100000",
    f,
    JSON.stringify({ state: "draft", total: 60000 })
  );
  assert.equal(no.result, false);
});

test("queryMatch supports cross-field comparison", () => {
  const f = JSON.stringify(fields);
  const r = w.queryMatch(
    "total>[base]",
    f,
    JSON.stringify({ total: 100, base: 50 })
  );
  assert.equal(r.error, undefined);
  assert.equal(r.result, true);
});

test("queryEval computes a value expression", () => {
  const r = w.queryEval(
    "[base]*2",
    JSON.stringify(fields),
    JSON.stringify({ base: 21 })
  );
  assert.equal(r.error, undefined);
  assert.equal(r.result, 42);
});

test("queryEval reports ErrNoValue for a missing field", () => {
  const r = w.queryEval(
    "[base]",
    JSON.stringify(fields),
    JSON.stringify({})
  );
  assert.ok(r.error, "expected an error for a missing field");
});

test("queryMatch surfaces a compile error", () => {
  const r = w.queryMatch(
    "unknown_field=x",
    JSON.stringify(fields),
    JSON.stringify({})
  );
  assert.ok(r.error, "expected a validate/compile error");
});
