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
  Value,
  FieldConfig,
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
 * Load the WASM module and return the query API.
 *
 * @param wasmPath - Path or URL to the query.wasm file.
 *                   Defaults to "./query.wasm".
 */
export async function loadQuery(wasmPath = "./query.wasm"): Promise<QueryAPI> {
  // Load Go's wasm_exec.js runtime (must be included in the page/bundle).
  const go = new Go();

  let wasmBytes: BufferSource;

  // Node.js
  if (typeof process !== "undefined" && process.versions?.node) {
    const fs = await import("fs");
    wasmBytes = fs.readFileSync(wasmPath);
  } else {
    // Browser / Deno
    const resp = await fetch(wasmPath);
    wasmBytes = await resp.arrayBuffer();
  }

  const result = await WebAssembly.instantiate(wasmBytes, go.importObject);
  // Don't await go.run() — it blocks forever (the Go main uses select{}).
  void go.run(result.instance);

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
