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

// Completion. The cursor and the span it answers with index the JavaScript
// string, in UTF-16 code units, like an input's selectionStart.
const completionFields = JSON.stringify([
  { name: "name", type: "text" },
  { name: "status", type: "text", allowedOps: ["=", "!="], values: ["ready", "not ready", "812"] },
  { name: "replicas", type: "integer" },
  { name: "labels", type: "text", nested: true },
  { name: "forks", type: "list", elements: [{ name: "kind", type: "text", values: ["Rollout"] }] },
]);
const strictGrammar = { explicitAnd: true, strictValues: true };
const texts = (r) => r.result.items.map((it) => it.text);

test("queryComplete walks a query from fields to values", () => {
  let r = w.queryComplete("", completionFields, { cursor: 0 });
  assert.equal(r.error, undefined);
  assert.equal(r.result.slot, "field");
  assert.deepEqual(texts(r), ["name", "status", "replicas", "labels.", "forks", "NOT"]);

  r = w.queryComplete("status ", completionFields, { cursor: 7 });
  assert.equal(r.result.slot, "operator");
  assert.equal(r.result.field, "status");
  assert.deepEqual(texts(r), ["=", "!=", "IN (", "NOT IN ("]);

  r = w.queryComplete("status=", completionFields, { cursor: 7 });
  assert.equal(r.result.slot, "value");
  assert.equal(r.result.type, "text");
  assert.equal(r.result.operator, "=");
  assert.deepEqual(texts(r), ["ready", '"not ready"', '"812"']);
  assert.deepEqual(r.result.items[1], { label: "not ready", text: '"not ready"', kind: "value" });

  r = w.queryComplete("status=ready ", completionFields, { cursor: 13 });
  assert.equal(r.result.slot, "connective");
  assert.deepEqual(texts(r), ["AND", "OR"]);

  r = w.queryComplete("forks@(kind=", completionFields, { cursor: 12 });
  assert.deepEqual(r.result.lists, ["forks"]);
  assert.deepEqual(texts(r), ["Rollout"]);
});

test("queryComplete defaults the cursor to the end of the query", () => {
  for (const r of [
    w.queryComplete("status=re", completionFields),
    w.queryComplete("status=re", completionFields, undefined),
    w.queryComplete("status=re", completionFields, {}),
    w.queryComplete("status=re", completionFields, { cursor: undefined }),
  ]) {
    assert.equal(r.error, undefined);
    assert.deepEqual(r.result.replace, { offset: 7, length: 2 });
    // What starts with the typed text comes first, then what contains it.
    assert.deepEqual(texts(r), ["ready", '"not ready"']);
  }
});

test("queryComplete takes its last argument as every other call does", () => {
  // A number is maxLength, as in queryParse; the cursor travels in the object.
  assert.equal(w.queryComplete("status=re", completionFields, 4).result.slot, "none");
  assert.equal(w.queryComplete("status=re", completionFields, 64).result.slot, "value");
  const r = w.queryComplete("sta=x", completionFields, { cursor: 3 });
  assert.deepEqual(r.result.replace, { offset: 0, length: 3 });
  assert.deepEqual(texts(r), ["status"]);
  // A null cursor is an absent one, as in the C library.
  assert.deepEqual(
    w.queryComplete("status=re", completionFields, { cursor: null }),
    w.queryComplete("status=re", completionFields)
  );
});

test("queryComplete counts in UTF-16 code units", () => {
  const q = 'name="café 🚀" AND st';
  const r = w.queryComplete(q, completionFields, { cursor: q.length });
  assert.equal(r.error, undefined);
  assert.deepEqual(r.result.replace, { offset: q.length - 2, length: 2 });
  assert.equal(q.slice(r.result.replace.offset, r.result.replace.offset + r.result.replace.length), "st");
  assert.deepEqual(texts(r), ["status"]);

  // Between the two halves of the rocket is not a position.
  const inside = q.indexOf("🚀") + 1;
  assert.match(w.queryComplete(q, completionFields, { cursor: inside }).error, /cannot split a character/);
});

test("queryComplete follows the grammar the options name", () => {
  const q = "name=x re";
  assert.deepEqual(texts(w.queryComplete(q, completionFields, { cursor: q.length })), ["replicas"]);
  assert.deepEqual(texts(w.queryComplete(q, completionFields, { cursor: q.length, ...strictGrammar })), []);
  assert.equal(w.queryComplete(q, completionFields, { cursor: q.length, maxLength: 4 }).result.slot, "none");
});

test("queryComplete has nothing to say after text that does not read as a query", () => {
  const r = w.queryComplete("name==", completionFields, { cursor: 6 });
  assert.equal(r.error, undefined);
  assert.deepEqual(r.result, { slot: "none", replace: { offset: 6, length: 0 }, prefix: "", items: [] });
});

test("queryComplete refuses a call that is wrong", () => {
  assert.match(w.queryComplete("name=x", completionFields, { cursor: -1 }).error, /is not a position in the query/);
  assert.match(w.queryComplete("name=x", completionFields, { cursor: 7 }).error, /from 0 to 6/);
  assert.equal(w.queryComplete("name=x", completionFields, { cursor: 1.5 }).error, "cursor must be an integer");
  assert.equal(w.queryComplete("name=x", completionFields, { cursor: [] }).error, "cursor must be an integer");
  assert.equal(w.queryComplete("name=x", completionFields, { cursor: "3" }).error, "cursor must be an integer");
  assert.match(w.queryComplete("name=x", completionFields, { cursr: 3 }).error, /unknown option "cursr"/);
  assert.equal(w.queryComplete("name=x", completionFields, { explicitAnd: 1 }).error, "explicitAnd must be a boolean");
  assert.match(w.queryComplete("name=x", completionFields, "3").error, /options must be/);
  assert.match(w.queryComplete("name=x", "{", { cursor: 3 }).error, /invalid fields config/);
  assert.match(w.queryComplete("name=x", '[{"type":"text"}]', { cursor: 3 }).error, /has no name/);
  assert.equal(w.queryComplete("name=x").error, "queryComplete: missing arguments");
  assert.equal(w.queryComplete("name=x", completionFields, 3, { cursor: 4 }).error, "queryComplete: too many arguments");
  assert.equal(w.queryComplete("name=x", [], { cursor: 3 }).error, "queryComplete: arguments must be strings");
});

test("queryOffer writes the host's candidates the way the grammar needs them", () => {
  const value = w.queryComplete("name=", completionFields, { cursor: 5 }).result;
  let r = w.queryOffer(JSON.stringify(value), JSON.stringify(["web", "api server", "812", "a*b"]));
  assert.equal(r.error, undefined);
  assert.deepEqual(r.result.map((it) => it.text), ["web", '"api server"', '"812"', '"a*b"']);
  assert.deepEqual(r.result[1], { label: "api server", text: '"api server"', kind: "value" });

  const number = w.queryComplete("replicas>", completionFields, { cursor: 9 }).result;
  r = w.queryOffer(JSON.stringify(number), JSON.stringify(["3", "many"]));
  assert.deepEqual(r.result.map((it) => it.text), ["3"]);

  const key = w.queryComplete("labels.te", completionFields, { cursor: 9 }).result;
  assert.equal(key.slot, "key");
  assert.equal(key.field, "labels");
  assert.equal(key.prefix, "te");
  assert.deepEqual(key.replace, { offset: 0, length: 9 });
  r = w.queryOffer(JSON.stringify(key), JSON.stringify(["team", "app.kubernetes.io/name", "my key"]));
  assert.deepEqual(r.result.map((it) => it.text), ["labels.team", "labels.app.kubernetes.io/name", '["labels.my key"]']);
  assert.equal(r.result[0].kind, "key");

  const quoted = w.queryComplete('name="we', completionFields, { cursor: 8 }).result;
  assert.equal(quoted.quoted, true);
  r = w.queryOffer(JSON.stringify(quoted), JSON.stringify(["web"]));
  assert.deepEqual(r.result.map((it) => it.text), ['"web"']);
});

test("queryOffer keeps a number a number after a function, and drops text after arithmetic", () => {
  const afterCall = w.queryComplete("len(name)>", completionFields).result;
  assert.equal(afterCall.function, "len");
  assert.equal(afterCall.field, undefined);
  // len gives a number, so a word is left out; lower gives text, so a
  // number is quoted.
  let r = w.queryOffer(JSON.stringify(afterCall), JSON.stringify(["5", "two words"]));
  assert.deepEqual(r.result.map((it) => it.text), ["5"]);
  const afterLower = w.queryComplete("lower(name)=", completionFields).result;
  r = w.queryOffer(JSON.stringify(afterLower), JSON.stringify(["812", "web"]));
  assert.deepEqual(r.result.map((it) => it.text), ['"812"', "web"]);

  const operand = w.queryComplete("replicas>10 - 3", completionFields).result;
  assert.equal(operand.arithmetic, true);
  assert.equal(operand.sign, "-");
  // replicas is a number, so only a number can follow the minus.
  r = w.queryOffer(JSON.stringify(operand), JSON.stringify(["web", "5", "7d"]));
  assert.deepEqual(r.result.map((it) => it.text), ["5"]);
});

test("accepting an item does not repeat the punctuation that follows it", () => {
  const apply = (q, c, it) => q.slice(0, c.replace.offset) + it.text + q.slice(c.replace.offset + c.replace.length);
  let q = "labels.foo=1";
  let c = w.queryComplete(q, completionFields, { cursor: 6 }).result;
  assert.equal(apply(q, c, c.items.find((it) => it.label === "labels")), "labels.foo=1");
  q = "forks@al(kind=x)";
  c = w.queryComplete(q, completionFields, { cursor: 8 }).result;
  assert.equal(apply(q, c, c.items.find((it) => it.label === "@all(...)")), "forks@all(kind=x)");
  q = '["labels.my"';
  c = w.queryComplete(q, completionFields, { cursor: 11 }).result;
  const key = w.queryOffer(JSON.stringify(c), JSON.stringify(["my key"])).result[0];
  assert.equal(apply(q, c, key), '["labels.my key"]');
  assert.equal(w.queryParse(apply(q, c, key) + "=x", strictGrammar).error, undefined);
});

test("a value is replaced without what follows it, and a date being typed is one word", () => {
  const apply = (q, c, it) => q.slice(0, c.replace.offset) + it.text + q.slice(c.replace.offset + c.replace.length);
  const offer = (q, cursor, cand) => {
    const c = w.queryComplete(q, completionFields, { cursor }).result;
    return apply(q, c, w.queryOffer(JSON.stringify(c), JSON.stringify([cand])).result[0]);
  };
  assert.equal(offer("replicas>4*2 AND name=x", 10, "2"), "replicas>2*2 AND name=x");
  assert.equal(offer("replicas:100..500", 12, "250"), "replicas:250..500");
  const dates = JSON.stringify([{ name: "day", type: "date" }]);
  const q = "day=2026-01-0";
  const c = w.queryComplete(q, dates).result;
  assert.equal(c.prefix, "2026-01-0");
  assert.deepEqual(c.replace, { offset: 4, length: 9 });
  const picked = w.queryOffer(JSON.stringify(c), JSON.stringify(["2026-01-01"])).result[0];
  assert.equal(apply(q, c, picked), "day=2026-01-01");
});

test("a quote opened on a field that is not text is dropped by the value", () => {
  const fields = JSON.stringify([{ name: "ready", type: "boolean" }]);
  const q = 'ready="';
  const c = w.queryComplete(q, fields).result;
  assert.deepEqual(c.items.map((it) => it.text), ["true", "false"]);
  const out = q.slice(0, c.replace.offset) + c.items[0].text;
  assert.equal(out, "ready=true");
  assert.equal(w.queryParseAndValidate(out, fields, strictGrammar).error, undefined);
});

test("queryOffer refuses what it cannot read", () => {
  assert.match(w.queryOffer("{", "[]").error, /invalid completion/);
  assert.match(w.queryOffer('{"slot":"bogus"}', "[]").error, /unknown slot "bogus"/);
  assert.equal(w.queryOffer('{"slot":"value"}', '"x"').error, "candidates must be an array of strings");
  assert.equal(w.queryOffer('{"slot":"value"}', "[1]").error, "candidates must be an array of strings");
  assert.equal(w.queryOffer('{"slot":"value"}').error, "queryOffer: missing arguments");
  assert.equal(w.queryOffer('{"slot":"value"}', "[]", {}).error, "queryOffer: too many arguments");
  assert.equal(w.queryOffer({ slot: "value" }, "[]").error, "queryOffer: arguments must be strings");
});

test("a query built from suggestions parses and validates", () => {
  const second = texts(w.queryComplete("status=", completionFields, { cursor: 7 }))[1];
  const built = `status=${second} AND replicas>3`;
  assert.equal(built, 'status="not ready" AND replicas>3');
  assert.equal(w.queryParseAndValidate(built, completionFields, strictGrammar).error, undefined);
});
