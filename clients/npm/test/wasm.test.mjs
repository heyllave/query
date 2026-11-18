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

// A client renders its own copy of a failure — a Spanish UI underlining the
// token that broke — so the joined English sentence is not enough. These pin
// the structured half of the contract.
test("queryParse reports each failure with a code and a span", () => {
  const { errors } = w.queryParse("items@nope(x=1)");
  assert.ok(Array.isArray(errors), "expected a structured errors array");
  assert.ok(errors.length > 0, "expected at least one structured error");

  const [first] = errors;
  assert.equal(first.code, "expectedSelector");
  assert.equal(first.offset, 6, "offset must point at the offending token");
  assert.equal(first.length, 4, "length must span it, so a caret can underline");
  assert.equal(typeof first.kind, "string");
  assert.ok(first.message.length > 0);
});

test("the strict options refuse the lenient forms and are off by default", () => {
  assert.equal(w.queryParse("a=1 b=2").error, undefined);
  assert.equal(w.queryParse("a=1 b=2", 256).error, undefined);

  const missing = w.queryParse("a=1 b=2", { explicitAnd: true });
  assert.equal(missing.errors[0].code, "missingAnd");
  assert.equal(missing.errors[0].offset, 4);
  assert.ok(missing.errors[0].hint.length > 0);

  assert.equal(w.queryParse("a=1+b=2").error, undefined);
  assert.equal(w.queryParse("a=1+b=2", { strictValues: true }).errors[0].code, "unexpectedChar");

  const strict = { explicitAnd: true, strictValues: true };
  assert.equal(w.queryParse("a=1 AND b=2", strict).error, undefined);

  const fields = JSON.stringify([
    { name: "a", type: "integer" },
    { name: "b", type: "integer" },
  ]);
  assert.equal(w.queryParseAndValidate("a=1 b=2", fields, strict).errors[0].code, "missingAnd");
  assert.equal(w.queryMatch("a=1 b=2", fields, '{"a":1,"b":2}', strict).errors[0].code, "missingAnd");
  assert.equal(w.queryMatch("a=1 AND b=2", fields, '{"a":1,"b":2}', strict).result, true);

  // A switch of the wrong type is an error, never a silent default: a host
  // that passed it believes strict parsing is on.
  for (const bad of [{ explicitAnd: 1 }, { strictValues: "true" }, { explicitAnd: null }, { maxLength: "9" }]) {
    const env = w.queryParse("a=1 b=2", bad);
    assert.match(env.error, /must be a (boolean|number)/, JSON.stringify(bad));
    assert.equal(env.result, undefined);
  }
  assert.match(w.queryMatch("a=1 b=2", fields, '{"a":1,"b":2}', { explicitAnd: "yes" }).error, /explicitAnd must be a boolean/);
  assert.match(w.queryParseAndValidate("a=1 b=2", fields, { strictValues: 0 }).error, /strictValues must be a boolean/);

  // So is an option the engine does not know.
  assert.match(w.queryParse("a=1 b=2", { strict: true }).error, /unknown option "strict"/);
  assert.match(w.queryMatch("a=1", fields, '{"a":1}', { explicitAnd: true, strictvalues: true }).error, /unknown option "strictvalues"/);

  // Two unknown options always name the same one, the one the C library names.
  assert.match(w.queryParse("a=1", { zeta: 1, alpha: 2 }).error, /unknown option "alpha"/);
  // An option set to undefined is an absent one, known or not.
  assert.equal(w.queryParse("a=1", { strict: undefined, explicitAnd: undefined }).error, undefined);

  // An options argument that is neither a number nor an object is refused,
  // not read as the default.
  for (const bad of [true, "strict", [], [1]]) {
    assert.match(w.queryParse("a=1 b=2", bad).error, /options must be a number/, JSON.stringify(bad));
  }
  assert.equal(w.queryParse("a=1 b=2", null).error, undefined);
  assert.equal(w.queryParse("a=1 b=2", undefined).error, undefined);

  // A call with no grammar to be strict about refuses the switches, and
  // still takes its length limit as an object.
  const record = '{"a":1}';
  assert.match(w.queryEval("1+1", fields, record, { explicitAnd: true }).error, /unknown option "explicitAnd"/);
  assert.equal(w.queryEval("1+1", fields, record, { maxLength: 1 }).errors[0].code, "queryTooLong");
  assert.equal(w.queryEval("1+1", fields, record, { maxLength: 9 }).result, 2);
  assert.match(w.queryEval("1+1", fields, record, "9").error, /options must be a number/);
  const ast = JSON.stringify(w.queryParse("a=1").result);
  assert.match(w.queryValidate(ast, fields, { strict: true }).error, /unknown option "strict"/);
  assert.match(w.queryStringify(ast, { strict: true }).error, /unknown option "strict"/);
  assert.equal(w.queryStringify(ast, undefined).result, "a=1");

  // An argument after the options would be ignored, so it is refused.
  assert.match(w.queryMatch("a=1", fields, record, 256, { explicitAnd: true }).error, /too many arguments/);
  assert.equal(w.queryMatch("a=1", fields, record, 256, undefined).result, true);

  // A strict failure's hint is one the strict grammar accepts.
  const keyed = w.queryParse('labels["team"]=x', { explicitAnd: true });
  assert.match(keyed.errors[0].hint, /labels\.team=x/);

  // The object form still carries the length limit.
  assert.equal(w.queryParse("a=1 AND b=2", { maxLength: 3 }).errors[0].code, "queryTooLong");
});

test("queryParse codes the common failures a person types", () => {
  const cases = [
    ["(state=draft", "unclosedParen"],
    ["state=", "expectedValue"],
    ["=draft", "expectedField"],
    ["state IN ()", "emptyInList"],
    ["state=**b**", "invalidWildcard"],
    ['state="draft', "unclosedString"],
  ];
  for (const [query, code] of cases) {
    const { errors } = w.queryParse(query);
    assert.ok(errors?.length, `${query}: expected structured errors`);
    assert.equal(errors[0].code, code, `${query}: wrong code`);
  }
});

test("queryParse carries a zero-length span at end of input", () => {
  // A failure at the end has nowhere to underline. Zero is the honest answer
  // and must survive the wire rather than being dropped as falsy.
  const { errors } = w.queryParse("state=");
  assert.equal(errors[0].length, 0);
  assert.ok(Object.hasOwn(errors[0], "length"), "length must be present, not omitted");
});

test("queryParse omits the structured array when it succeeds", () => {
  const { result, errors, error } = w.queryParse("state=draft");
  assert.ok(result, "expected an AST");
  assert.equal(error, undefined);
  assert.equal(errors, undefined, "a success carries no errors array");
});

test("queryParseAndValidate reports parse failures structurally too", () => {
  const { errors } = w.queryParseAndValidate("(state=draft", JSON.stringify(fields));
  assert.ok(errors?.length, "expected structured errors from the validate entry point");
  assert.equal(errors[0].code, "unclosedParen");
});

test("a validation failure carries no parse errors array", () => {
  // Only a PARSE failure is coded this way; a validation failure is a
  // different envelope and must not pretend otherwise.
  const { errors, error } = w.queryParseAndValidate("color=red", JSON.stringify(fields));
  assert.ok(error, "expected a validation failure");
  assert.equal(errors, undefined);
});

test("a validation failure is coded with its field and operator", () => {
  const { validationErrors } = w.queryParseAndValidate(
    "color=red AND state>draft",
    JSON.stringify(fields)
  );
  assert.deepEqual(
    validationErrors.map(({ code, field, op }) => ({ code, field, op })),
    [
      { code: "fieldNotFound", field: "color", op: "" },
      { code: "operatorNotAllowed", field: "state", op: ">" },
    ]
  );
});

test("queryValidate codes its failures too", () => {
  const { result: ast } = w.queryParse("state=draft");
  const v = w.queryValidate(JSON.stringify(ast), JSON.stringify([]));
  assert.equal(v.validationErrors[0].code, "fieldNotFound");
  assert.equal(v.validationErrors[0].field, "state");
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

// A client that builds a node (a filter form) has no source text to keep the
// quotes of; the bridge must still print text that reads back as that node.
test("queryStringify quotes a built string that would not read back bare", () => {
  const node = {
    type: "binary",
    op: "AND",
    left: { type: "qualifier", op: "=", field: ["cliente", "nombre"], value: { type: "string", raw: "Ana Pérez" } },
    right: { type: "qualifier", op: "=", field: ["code"], value: { type: "string", raw: "500" } },
  };
  const { result: str, error } = w.queryStringify(JSON.stringify(node));
  assert.equal(error, undefined);
  assert.equal(str, 'cliente.nombre="Ana Pérez" AND code="500"');
  assert.deepEqual(w.queryParse(str).result.left.value.raw, "Ana Pérez");
});

test("queryStringify keeps what the parser read quoted, quoted", () => {
  const { result: ast } = w.queryParse('state="needs review" OR state="42"');
  assert.equal(ast.left.value.quoted, true);
  const { result: str } = w.queryStringify(JSON.stringify(ast));
  assert.equal(str, 'state="needs review" OR state="42"');
});

test("queryStringify prints a selector", () => {
  const { result: ast } = w.queryParse("items@any(total>5) AND items@first");
  const { result: str, error } = w.queryStringify(JSON.stringify(ast));
  assert.equal(error, undefined);
  assert.equal(str, "items@any(total>5) AND items@first");
});

test("queryStringify builds a range from its end value", () => {
  const node = {
    type: "qualifier",
    op: "..",
    field: ["total"],
    value: { type: "integer", raw: "100" },
    endValue: { type: "integer", raw: "500" },
  };
  assert.equal(w.queryStringify(JSON.stringify(node)).result, "total:100..500");
});

test("queryParse carries a predicate function as a call node", () => {
  const r = w.queryParse('contains(tags, "x") AND total>1');
  assert.equal(r.error, undefined);
  assert.equal(r.result.type, "binary");
  assert.deepEqual(r.result.left, {
    type: "call",
    call: 'contains(tags, "x")',
    fields: [["tags"]],
  });
  const alone = w.queryParse('startsWith(state, "dr")');
  assert.equal(alone.result.type, "call");
  const { result: str } = w.queryStringify(JSON.stringify(r.result));
  assert.equal(str, 'contains(tags, "x") AND total>1');
});

test("queryParse lists the fields a fieldFunc reads", () => {
  const r = w.queryParse("len(state)>3");
  assert.equal(r.result.fieldFunc, "len(state)");
  assert.deepEqual(r.result.field, undefined);
  assert.deepEqual(r.result.fields, [["state"]]);
});

test("every entry point takes an optional maxLength in characters", () => {
  const f = JSON.stringify(fields);
  const q = "state=" + "x".repeat(300);
  assert.ok(w.queryParseAndValidate(q, f).error, "default limit applies");
  assert.equal(w.queryParseAndValidate(q, f, 0).error, undefined, "0 disables it");
  assert.ok(w.queryMatch(q, f, "{}").error);
  assert.equal(w.queryMatch(q, f, "{}", 1000).error, undefined);
  assert.ok(w.queryEval("[base]", f, "{}", 3).error);
  // Characters, not bytes: eleven two-byte letters fit under a limit of 13.
  const accented = "state=" + "é".repeat(11);
  assert.equal(w.queryParse(accented, 17).error, undefined);
  assert.ok(w.queryParse(accented, 16).error);
  // A tree is held to the limit it was parsed under, which validate and
  // stringify take too.
  const tree = JSON.stringify(w.queryParse(q, 0).result);
  assert.ok(w.queryValidate(tree, f).error, "default limit applies to a tree");
  assert.equal(w.queryValidate(tree, f, 0).error, undefined);
  assert.equal(w.queryValidate(tree, f, 1000).valid, true);
  assert.ok(w.queryStringify(tree).error);
  assert.equal(w.queryStringify(tree, 0).result, q);
});

test("a failure carries a hint when the engine can tell what was meant", () => {
  const syntax = w.queryParse("state==draft");
  assert.equal(syntax.errors[0].code, "unexpectedChar");
  assert.match(syntax.errors[0].hint, /single =/);

  const arithmetic = w.queryParse("[base]*1.1 > 3");
  assert.match(arithmetic.errors[0].hint, /float\(\[base\]\*1\.1\)>3/);

  const typo = w.queryParseAndValidate("stat=draft", JSON.stringify(fields));
  assert.equal(typo.validationErrors[0].code, "fieldNotFound");
  assert.match(typo.validationErrors[0].hint, /did you mean "state"\?/);
});

test("a doubled character is one error spanning both", () => {
  const { errors } = w.queryParse("a=1 && b=2");
  assert.equal(errors.length, 1);
  assert.equal(errors[0].offset, 4);
  assert.equal(errors[0].length, 2);
  assert.match(errors[0].hint, /AND/);
});

test("a declared list field checks its selectors and its elements", () => {
  const lists = JSON.stringify([
    { Name: "name", Type: "text" },
    {
      Name: "forks",
      Type: "list",
      Elements: [
        { Name: "name", Type: "text" },
        { Name: "replicas", Type: "integer" },
      ],
    },
  ]);
  const ok = w.queryParseAndValidate("forks@(name=a AND replicas>2)", lists);
  assert.equal(ok.error, undefined);
  assert.equal(ok.errors, undefined);
  assert.equal(w.queryMatch("forks@(replicas>2)", lists, '{"forks":[{"name":"a","replicas":3}]}').result, true);

  const text = (r) => JSON.stringify(r);
  assert.match(text(w.queryParseAndValidate("forks@last", lists)), /selector @last is not allowed/);
  assert.match(text(w.queryParseAndValidate("name@first", lists)), /is not a list field/);
  assert.match(text(w.queryParseAndValidate("forks@(bogus=1)", lists)), /unknown field .{1,2}bogus.{1,2} on .{1,2}forks/);
  assert.match(
    text(w.queryParseAndValidate("forks@first", JSON.stringify([{ Name: "forks", Type: "list", Selectors: ["every"] }]))),
    /unknown selector/,
  );
});
