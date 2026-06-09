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
  | "duration";

/** A typed value in a qualifier expression. */
export interface Value {
  type: ValueType;
  raw: string;
  value: string | number | boolean;
  wildcard?: boolean;
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

/** Union of all expression types. */
export type Expression =
  | BinaryExpr
  | UnaryExpr
  | QualifierExpr
  | PresenceExpr
  | GroupExpr;

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

/** Result from parse operations. */
export interface ParseResult {
  result?: Expression;
  error?: string;
}

/** Result from validate operations. */
export interface ValidateResult {
  valid: boolean;
  errors?: string[];
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
