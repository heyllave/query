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
  ParseOptions,
  Completion,
  CompleteOptions,
  CompleteResult,
  OfferResult,
} from "./types.js";

export type {
  Expression,
  BinaryExpr,
  UnaryExpr,
  QualifierExpr,
  PresenceExpr,
  CallExpr,
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
  ParseOptions,
  Completion,
  CompletionItem,
  CompletionItemKind,
  CompletionSlot,
  CompleteOptions,
  CompleteResult,
  OfferResult,
} from "./types.js";

export { visit, walk, fields, fieldToString, applyCompletion } from "./types.js";

// Extend the global scope for Go WASM functions.
declare global {
  function queryParse(q: string, options?: number | ParseOptions): ParseResult;
  function queryValidate(astJSON: string, fieldsJSON: string, maxLength?: number): ValidateResult;
  function queryStringify(astJSON: string, maxLength?: number): StringifyResult;
  function queryParseAndValidate(
    q: string,
    fieldsJSON: string,
    options?: number | ParseOptions
  ): ParseResult;
  function queryMatch(
    q: string,
    fieldsJSON: string,
    recordJSON: string,
    options?: number | ParseOptions
  ): MatchResult;
  function queryEval(
    q: string,
    fieldsJSON: string,
    recordJSON: string,
    maxLength?: number
  ): EvalResult;
  function queryComplete(
    q: string,
    fieldsJSON: string,
    options?: number | (ParseOptions & { cursor?: number | null })
  ): CompleteResult;
  function queryOffer(completionJSON: string, candidatesJSON: string): OfferResult;

  // Go's wasm_exec.js provides this constructor.
  class Go {
    importObject: WebAssembly.Imports;
    run(instance: WebAssembly.Instance): Promise<void>;
  }
}

/** Query API returned by loadQuery(). */
export interface QueryAPI {
  /**
   * Parse a query string into an AST. The last argument is the query length
   * limit in characters (default 256; 0 disables it), or a
   * {@link ParseOptions} object that also carries the strict switches.
   */
  parse(q: string, options?: number | ParseOptions): ParseResult;

  /**
   * Validate an AST against field configurations. `maxLength` bounds the
   * text the tree carries as written (string values, field names), as in
   * {@link parse}; pass the limit the tree was parsed under.
   */
  validate(ast: Expression, fields: FieldConfig[], maxLength?: number): ValidateResult;

  /** Convert an AST back to a query string; `maxLength` as in {@link validate}. */
  stringify(ast: Expression, maxLength?: number): StringifyResult;

  /** Parse and validate in one call; `options` as in {@link parse}. */
  parseAndValidate(q: string, fields: FieldConfig[], options?: number | ParseOptions): ParseResult;

  /**
   * Compile a boolean predicate and evaluate it against a record in one call.
   * For repeated evaluation, prefer compiling once on the Go side.
   */
  match(
    q: string,
    fields: FieldConfig[],
    record: QueryRecord,
    options?: number | ParseOptions
  ): MatchResult;

  /**
   * Compile a value expression and evaluate it against a record, returning the
   * computed value (number, string, boolean, or list).
   */
  eval(q: string, fields: FieldConfig[], record: QueryRecord, maxLength?: number): EvalResult;

  /**
   * Report what can be written at `cursor` in a query that is still being
   * typed: field names, the operators a field allows, the selectors of a
   * list, connectives, and the values a field is known to take. `cursor`
   * indexes the string as `selectionStart` does.
   *
   * It answers from the schema alone, so it can run on every keystroke.
   * Where only you know the candidates (the values of a field, the keys of a
   * nested one) the result says what is being asked for; fetch them and
   * pass them to {@link offer}.
   *
   * Pass the options the query will be parsed with, so that what is
   * suggested is what will be accepted. A query that is merely unfinished is
   * never an error: a position nothing can be said about has the slot
   * `none`. An error means the call itself was wrong (a cursor outside the
   * string, an invalid schema).
   */
  complete(
    q: string,
    cursor: number,
    fields: FieldConfig[],
    options?: CompleteOptions
  ): CompleteResult;

  /**
   * Turn candidates you supply into items for a completion, each written the
   * way the grammar needs it at that position: a value is quoted when it
   * would not read back as itself, a key is joined to its field. The
   * candidates are kept in the order given.
   */
  offer(completion: Completion, candidates: string[]): OfferResult;
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
    parse(q: string, options?: number | ParseOptions): ParseResult {
      return queryParse(q, options);
    },

    validate(ast: Expression, fields: FieldConfig[], maxLength?: number): ValidateResult {
      return queryValidate(JSON.stringify(ast), JSON.stringify(fields), maxLength);
    },

    stringify(ast: Expression, maxLength?: number): StringifyResult {
      return queryStringify(JSON.stringify(ast), maxLength);
    },

    parseAndValidate(
      q: string,
      fields: FieldConfig[],
      options?: number | ParseOptions
    ): ParseResult {
      return queryParseAndValidate(q, JSON.stringify(fields), options);
    },

    match(
      q: string,
      fields: FieldConfig[],
      record: QueryRecord,
      options?: number | ParseOptions
    ): MatchResult {
      return queryMatch(q, JSON.stringify(fields), JSON.stringify(record), options);
    },

    eval(q: string, fields: FieldConfig[], record: QueryRecord, maxLength?: number): EvalResult {
      return queryEval(q, JSON.stringify(fields), JSON.stringify(record), maxLength);
    },

    complete(
      q: string,
      cursor: number,
      fields: FieldConfig[],
      options?: CompleteOptions
    ): CompleteResult {
      return queryComplete(q, JSON.stringify(fields), { ...options, cursor });
    },

    offer(completion: Completion, candidates: string[]): OfferResult {
      return queryOffer(JSON.stringify(completion), JSON.stringify(candidates));
    },
  };
}
