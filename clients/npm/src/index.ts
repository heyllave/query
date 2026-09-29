/**
 * @trazo/query — Trazo query language for the browser and Node (WASM).
 *
 * This package loads the Go WASM binary and exposes a typed TypeScript API for
 * parsing, validating, stringifying, matching, and evaluating Trazo query
 * expressions.
 *
 * @example
 * ```ts
 * import { loadQuery } from "@trazo/query";
 *
 * const q = await loadQuery();
 * const { result, error } = q.parse("state=draft AND total>50000");
 * if (error) throw new Error(error);
 * console.log(result); // AST object
 * ```
 */

import type {
  Expression,
  FieldConfig,
  ParseResult,
  ValidateResult,
  StringifyResult,
  MatchResult,
  EvalResult,
  QueryRecord,
} from "./types.js";

export type {
  Expression,
  BinaryExpr,
  UnaryExpr,
  QualifierExpr,
  PresenceExpr,
  GroupExpr,
  SelectorExpr,
  Value,
  ValueType,
  Operator,
  FieldConfig,
  ParseError,
  ParseErrorCode,
  ValidationError,
  ValidationErrorCode,
  ParseResult,
  ValidateResult,
  StringifyResult,
  MatchResult,
  EvalResult,
  QueryRecord,
  Visitor,
} from "./types.js";

export { visit, walk, fields, fieldToString } from "./types.js";

// Extend the global scope for Go WASM functions.
declare global {
  function queryParse(q: string, maxLength?: number): ParseResult;
  function queryValidate(astJSON: string, fieldsJSON: string): ValidateResult;
  function queryStringify(astJSON: string): StringifyResult;
  function queryParseAndValidate(
    q: string,
    fieldsJSON: string
  ): ParseResult;
  function queryMatch(
    q: string,
    fieldsJSON: string,
    recordJSON: string
  ): MatchResult;
  function queryEval(
    q: string,
    fieldsJSON: string,
    recordJSON: string
  ): EvalResult;

  // Go's wasm_exec.js provides this constructor.
  class Go {
    importObject: WebAssembly.Imports;
    run(instance: WebAssembly.Instance): Promise<void>;
  }
}

/** Query API returned by loadQuery(). */
export interface QueryAPI {
  /** Parse a query string into an AST. */
  parse(q: string, maxLength?: number): ParseResult;

  /** Validate an AST against field configurations. */
  validate(ast: Expression, fields: FieldConfig[]): ValidateResult;

  /** Convert an AST back to a query string. */
  stringify(ast: Expression): StringifyResult;

  /** Parse and validate in one call. */
  parseAndValidate(q: string, fields: FieldConfig[]): ParseResult;

  /**
   * Compile a boolean predicate and evaluate it against a record in one call.
   * For repeated evaluation, prefer compiling once on the Go side.
   */
  match(q: string, fields: FieldConfig[], record: QueryRecord): MatchResult;

  /**
   * Compile a value expression and evaluate it against a record, returning the
   * computed value (number, string, boolean, or list).
   */
  eval(q: string, fields: FieldConfig[], record: QueryRecord): EvalResult;
}

/**
 * Where the engine's bytes come from: a path or URL to query.wasm (read from
 * disk under Node, fetched elsewhere), the bytes themselves, or a fetch
 * `Response` (or a promise of one), which a browser compiles while it streams.
 */
export type WasmSource =
  | string
  | URL
  | BufferSource
  | Response
  | PromiseLike<Response>;

const isResponseLike = (source: unknown): source is Response | PromiseLike<Response> =>
  (typeof Response !== "undefined" && source instanceof Response) ||
  (typeof source === "object" && source !== null && "then" in source);

async function instantiate(
  source: WasmSource,
  imports: WebAssembly.Imports
): Promise<WebAssembly.Instance> {
  if (source instanceof ArrayBuffer || ArrayBuffer.isView(source)) {
    return (await WebAssembly.instantiate(source, imports)).instance;
  }
  if (isResponseLike(source)) {
    const response = await source;
    if (!response.ok) throw new Error(`query.wasm: HTTP ${response.status}`);
    // A server that does not label the file application/wasm makes streaming
    // compilation throw; the bytes are still good, so fall back to them.
    if (typeof WebAssembly.instantiateStreaming === "function") {
      try {
        return (await WebAssembly.instantiateStreaming(response.clone(), imports)).instance;
      } catch {
        // fall through to the buffered path
      }
    }
    return (await WebAssembly.instantiate(await response.arrayBuffer(), imports)).instance;
  }
  const path = source.toString();
  if (typeof process !== "undefined" && process.versions?.node) {
    const fs = (await nodeModule("node:fs")) as typeof import("fs");
    const url = (await nodeModule("node:url")) as typeof import("url");
    const bytes = fs.readFileSync(path.startsWith("file:") ? url.fileURLToPath(path) : path);
    return (await WebAssembly.instantiate(bytes, imports)).instance;
  }
  return instantiate(fetch(path), imports);
}

// A computed specifier keeps a browser bundler from resolving Node's modules:
// the path branch that needs them only runs under Node.
const nodeModule = (name: string): Promise<unknown> =>
  import(/* @vite-ignore */ /* webpackIgnore: true */ name);

/**
 * Load the WASM module and return the query API.
 *
 * Go's `wasm_exec.js` runtime ships in this package and is loaded by it, so
 * a page or bundle has nothing to include beforehand.
 *
 * @param source - Where to read query.wasm from (see {@link WasmSource}).
 *                 Defaults to "./query.wasm".
 */
export async function loadQuery(source: WasmSource = "./query.wasm"): Promise<QueryAPI> {
  await import("./wasm_exec.js");
  const go = new Go();
  const instance = await instantiate(source, go.importObject);
  // Don't await go.run() — it blocks forever (the Go main uses select{}). The
  // bridge functions are registered before main blocks, so they exist once
  // run() has been called.
  void go.run(instance);

  return {
    parse(q: string, maxLength?: number): ParseResult {
      return queryParse(q, maxLength);
    },

    validate(ast: Expression, fields: FieldConfig[]): ValidateResult {
      return queryValidate(JSON.stringify(ast), JSON.stringify(fields));
    },

    stringify(ast: Expression): StringifyResult {
      return queryStringify(JSON.stringify(ast));
    },

    parseAndValidate(q: string, fields: FieldConfig[]): ParseResult {
      return queryParseAndValidate(q, JSON.stringify(fields));
    },

    match(q: string, fields: FieldConfig[], record: QueryRecord): MatchResult {
      return queryMatch(q, JSON.stringify(fields), JSON.stringify(record));
    },

    eval(q: string, fields: FieldConfig[], record: QueryRecord): EvalResult {
      return queryEval(q, JSON.stringify(fields), JSON.stringify(record));
    },
  };
}
