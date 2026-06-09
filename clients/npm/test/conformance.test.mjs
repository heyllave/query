// Runs the shared cross-language corpus (/conformance/corpus.json) against the
// query WASM build. The Go and Dart clients run the same file, so grammar drift
// between implementations becomes a failing build.

import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const wasmPath = join(here, "..", "query.wasm");
const wasmExecPath = join(here, "..", "src", "wasm_exec.js");
const corpusPath = join(here, "..", "..", "..", "conformance", "corpus.json");

const EXPECTED_CASES = 25;

async function loadWasm() {
  await import(wasmExecPath);
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
  void go.run(instance);
  return globalThis;
}

const corpus = JSON.parse(readFileSync(corpusPath, "utf8"));
const w = await loadWasm();

function fieldsFor(tc) {
  return JSON.stringify(tc.fieldSet ? corpus.fieldSets[tc.fieldSet] : tc.fields);
}

test(`corpus has at least ${EXPECTED_CASES} cases`, () => {
  assert.ok(
    corpus.cases.length >= EXPECTED_CASES,
    `corpus has ${corpus.cases.length} cases, expected >= ${EXPECTED_CASES} (stale?)`
  );
});

for (const tc of corpus.cases) {
  test(`${tc.op}: ${tc.id}`, () => {
    const wantErr = tc.expectError === true;
    switch (tc.op) {
      case "parse": {
        const r = w.queryParse(tc.query);
        checkErr(r, wantErr);
        if (!wantErr && tc.expectAst) assert.equal(r.result.type, tc.expectAst);
        break;
      }
      case "parseAndValidate": {
        const r = w.queryParseAndValidate(tc.query, fieldsFor(tc));
        checkErr(r, wantErr);
        if (!wantErr && tc.expectAst) assert.equal(r.result.type, tc.expectAst);
        break;
      }
      case "stringify": {
        const ast = w.queryParse(tc.query).result;
        const r = w.queryStringify(JSON.stringify(ast));
        if (tc.expectString) assert.equal(r.result, tc.expectString);
        break;
      }
      case "match": {
        const r = w.queryMatch(tc.query, fieldsFor(tc), JSON.stringify(tc.record ?? {}));
        if (wantErr) { assert.ok(r.error, "expected error"); break; }
        assert.equal(r.error, undefined, r.error);
        if (tc.expectMatch !== undefined) assert.equal(r.result, tc.expectMatch);
        break;
      }
      case "eval": {
        const r = w.queryEval(tc.query, fieldsFor(tc), JSON.stringify(tc.record ?? {}));
        if (wantErr) { assert.ok(r.error, "expected error"); break; }
        assert.equal(r.error, undefined, r.error);
        if (tc.expectValue !== undefined) {
          const tol = tc.tolerance ?? 1e-9;
          assert.ok(
            Math.abs(Number(r.result) - tc.expectValue) <= tol,
            `value ${r.result}, want ${tc.expectValue} (±${tol})`
          );
        }
        break;
      }
      default:
        throw new Error(`unknown op ${tc.op}`);
    }
  });
}

function checkErr(r, wantErr) {
  if (wantErr) assert.ok(r.error, "expected error");
  else assert.equal(r.error, undefined, r.error);
}
