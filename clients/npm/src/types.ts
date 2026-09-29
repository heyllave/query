// AST node types matching the Go ast package.

/** Operator symbols used in qualifier expressions. */
export type Operator = "=" | "!=" | ">" | ">=" | "<" | "<=" | "..";

/** Logical operators for binary expressions. */
export type LogicalOp = "AND" | "OR";

/** Value types matching Go's ast.ValueType. */
export type ValueType =
  | "string"
  | "integer"
  | "float"
  | "boolean"
  | "date"
  | "duration"
  | "function"
  | "arithmetic"
  | "list"
  | "field";

/** A typed value in a qualifier expression. */
export interface Value {
  type: ValueType;
  /**
   * The value's source text; for a string, its content with the quotes and
   * escapes removed.
   */
  raw: string;
  /**
   * The typed value the engine read. A node a client builds may omit it: the
   * bridge reads `raw` back through the parser, so the text is what counts.
   */
  value?: string | number | boolean | null;
  wildcard?: boolean;
  /**
   * True when a string was written as a quoted literal. A built node may omit
   * it: a string whose bare text would not read back as itself is quoted on
   * the way back regardless.
   */
  quoted?: boolean;
}

/** Base interface for all AST nodes. */
export interface BaseNode {
  type: string;
}

/** Binary expression: left AND/OR right. */
export interface BinaryExpr extends BaseNode {
  type: "binary";
  op: LogicalOp;
  left: Expression;
  right: Expression;
}

/** Unary expression: NOT expr. */
export interface UnaryExpr extends BaseNode {
  type: "not";
  expr: Expression;
}

/** Qualifier expression: field op value. */
export interface QualifierExpr extends BaseNode {
  type: "qualifier";
  op: Operator;
  field: string[];
  value: Value;
  endValue?: Value;
}

/** Presence expression: field exists check. */
export interface PresenceExpr extends BaseNode {
  type: "presence";
  field: string[];
}

/** Group expression: (expr). */
export interface GroupExpr extends BaseNode {
  type: "group";
  expr: Expression;
}

/**
 * Selector expression: a base collection narrowed by a selector.
 *
 * `selector` is "first", "last", "any", "all", "none", or "" for the bare
 * `@(...)` form; `inner` carries the expression for the forms that take one.
 */
export interface SelectorExpr extends BaseNode {
  type: "selector";
  selector: string;
  base?: Expression;
  inner?: Expression;
}

/**
 * Union of all expression types the bridge serializes.
 *
 * Two surface forms never reach it, because the parser lowers them: `IN (a, b)`
 * becomes an OR chain of equality qualifiers, and a negated comparison like
 * `!>` becomes `NOT (field > value)`. A client that wants to show either back
 * as it was written has to recognise the lowered shape.
 */
export type Expression =
  | BinaryExpr
  | UnaryExpr
  | QualifierExpr
  | PresenceExpr
  | GroupExpr
  | SelectorExpr;

/** Field value types for validation. */
export type FieldValueType =
  | "text"
  | "integer"
  | "decimal"
  | "boolean"
  | "date"
  | "datetime"
  | "duration";

/** Operator identifiers for field config. */
export type OpId =
  | "="
  | "!="
  | ">"
  | ">="
  | "<"
  | "<="
  | ".."
  | "*"
  | "?";

/** Field configuration for validation. */
export interface FieldConfig {
  Name: string;
  Type: number;
  AllowedOps: string[];
  Searchable?: boolean;
  Nested?: boolean;
}

/**
 * A stable machine identifier for a parse failure, matching Go's parser.Code.
 * Clients render their own wording from it; the identifier never changes once
 * shipped, so a reworded message is not a breaking change.
 */
export type ParseErrorCode =
  | "queryTooLong"
  | "unexpectedChar"
  | "unclosedString"
  | "unclosedParen"
  | "unclosedIn"
  | "emptyInList"
  | "unclosedFieldRef"
  | "emptyFieldRef"
  | "expectedField"
  | "expectedValue"
  | "expectedInList"
  | "expectedSelector"
  | "expectedRange"
  | "unexpected"
  | "invalidWildcard"
  | "invalidDate"
  | "invalidDuration"
  | "invalidInteger"
  | "invalidFloat"
  | "unclosedFuncArgs"
  | "expectedFuncArg";

/**
 * One parse failure, with the span that caused it.
 *
 * `offset` and `length` are byte positions into the query as written, which is
 * what a caret or an underline needs; `message` is English and is there for
 * logs, not for a user-facing surface that has its own copy.
 */
export interface ParseError {
  code: ParseErrorCode;
  kind: string;
  message: string;
  offset: number;
  length: number;
}

/** A stable machine identifier for a validation failure, matching Go's ErrorKind.Code. */
export type ValidationErrorCode =
  | "fieldNotFound"
  | "operatorNotAllowed"
  | "typeMismatch"
  | "customRule";

/**
 * One validation failure: a query that parsed but names a field the config
 * does not declare, an operator the field does not allow, or a value its type
 * cannot hold.
 */
export interface ValidationError {
  code: ValidationErrorCode;
  /** English, for logs. */
  message: string;
  offset: number;
  length: number;
  /** The dotted field path the failure is about; empty when it is about none. */
  field: string;
  /** The refused operator for `operatorNotAllowed` ("?" for a presence check). */
  op: string;
}

/** Result from parse operations. */
export interface ParseResult {
  result?: Expression;
  /** The joined English message. Prefer `errors` for anything user-facing. */
  error?: string;
  /** Each failure, coded and positioned. Absent when the failure is not a parse error. */
  errors?: ParseError[];
  /** From parseAndValidate: each validation failure, coded. Absent on a parse error. */
  validationErrors?: ValidationError[];
}

/** Result from validate operations. */
export interface ValidateResult {
  valid: boolean;
  errors?: string[];
  /** Each failure, coded, when the query is invalid. */
  validationErrors?: ValidationError[];
}

/** Result from stringify operations. */
export interface StringifyResult {
  result?: string;
  error?: string;
}

/** Result from match operations: a boolean predicate evaluated against a record. */
export interface MatchResult {
  result?: boolean;
  error?: string;
}

/** Result from eval operations: a value expression evaluated against a record. */
export interface EvalResult {
  result?: string | number | boolean | unknown[];
  error?: string;
}

/** A record of field values to evaluate a query against. */
export type QueryRecord = { [field: string]: unknown };

// -------------------------------------------------------------------------
// Visitor pattern (mirrors Go's ast.Visitor[T])
// -------------------------------------------------------------------------

/** Visitor interface for transforming an AST into type T. */
export interface Visitor<T> {
  visitBinary(expr: BinaryExpr): T;
  visitUnary(expr: UnaryExpr): T;
  visitQualifier(expr: QualifierExpr): T;
  visitPresence(expr: PresenceExpr): T;
  visitGroup(expr: GroupExpr): T;
  visitSelector(expr: SelectorExpr): T;
}

/** Dispatch an expression to the appropriate visitor method. */
export function visit<T>(visitor: Visitor<T>, expr: Expression): T {
  switch (expr.type) {
    case "binary":
      return visitor.visitBinary(expr);
    case "not":
      return visitor.visitUnary(expr);
    case "qualifier":
      return visitor.visitQualifier(expr);
    case "presence":
      return visitor.visitPresence(expr);
    case "group":
      return visitor.visitGroup(expr);
    case "selector":
      return visitor.visitSelector(expr);
  }
}

// -------------------------------------------------------------------------
// Utility functions
// -------------------------------------------------------------------------

/** Get the dotted string representation of a field path. */
export function fieldToString(field: string[]): string {
  return field.join(".");
}

/** Walk the AST depth-first, calling fn for each node. */
export function walk(
  expr: Expression,
  fn: (node: Expression) => boolean
): void {
  if (!fn(expr)) return;
  switch (expr.type) {
    case "binary":
      walk(expr.left, fn);
      walk(expr.right, fn);
      break;
    case "not":
      walk(expr.expr, fn);
      break;
    case "group":
      walk(expr.expr, fn);
      break;
    case "selector":
      // A selector narrows a base collection; both sides carry qualifiers, and
      // skipping them hides every field inside `@any(...)` from `fields()`.
      if (expr.base) walk(expr.base, fn);
      if (expr.inner) walk(expr.inner, fn);
      break;
  }
}

/** Extract all unique field paths from an expression. */
export function fields(expr: Expression): string[][] {
  const seen = new Set<string>();
  const result: string[][] = [];
  walk(expr, (node) => {
    if (node.type === "qualifier" || node.type === "presence") {
      const key = node.field.join(".");
      if (!seen.has(key)) {
        seen.add(key);
        result.push(node.field);
      }
    }
    return true;
  });
  return result;
}
