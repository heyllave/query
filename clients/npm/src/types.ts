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

/**
 * Qualifier expression: field op value.
 *
 * When the left side is a function (`lower(name)=john`), `fieldFunc` carries
 * its source form, `field` is empty and `fields` lists the fields the
 * function reads.
 */
export interface QualifierExpr extends BaseNode {
  type: "qualifier";
  op: Operator;
  field: string[];
  value: Value;
  endValue?: Value;
  fieldFunc?: string;
  fields?: string[][];
}

/** Presence expression: field exists check. */
export interface PresenceExpr extends BaseNode {
  type: "presence";
  field: string[];
}

/**
 * A function used as a predicate: `contains(tags, "x")`. `call` is its source
 * form and `fields` lists the fields it reads. To change the call, parse new
 * text; the bridge reads `call` back through the parser.
 */
export interface CallExpr extends BaseNode {
  type: "call";
  call: string;
  fields?: string[][];
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
  | CallExpr
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
  | "duration"
  | "list";

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

/** The name of a field type, as Go's validate.FieldValueType prints it. */
export type FieldTypeName =
  | "text"
  | "integer"
  | "decimal"
  | "boolean"
  | "date"
  | "datetime"
  | "duration"
  | "list";

/** A selector by name; the empty string is the anonymous form `@(...)`. */
export type SelectorName = "first" | "last" | "" | "any" | "all" | "none";

/** Field configuration for validation. */
export interface FieldConfig {
  Name: string;
  /** The type's name, or its number in the order of {@link FieldTypeName}. */
  Type: number | FieldTypeName;
  /** Omit to get the operators that suit the type. */
  AllowedOps?: string[];
  Searchable?: boolean;
  Nested?: boolean;
  /** For a list: the fields of one element, which a selector's condition may name. */
  Elements?: FieldConfig[];
  /** For a list: the selectors allowed. Omit for first, `@(...)`, all and none. */
  Selectors?: SelectorName[];
  /**
   * Values the field is known to take, which completion offers. They are
   * suggestions: validation does not hold a query to them.
   */
  Values?: string[];
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
  | "nestingTooDeep"
  | "missingAnd";

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
  /**
   * What the writer most likely meant and how to write it, when the parser
   * can tell: "equality is a single =" for `state==draft`. English, absent
   * when there is nothing to suggest, and may be reworded between versions;
   * branch on `code`.
   */
  hint?: string;
}

/** A stable machine identifier for a validation failure, matching Go's ErrorKind.Code. */
export type ValidationErrorCode =
  | "fieldNotFound"
  | "operatorNotAllowed"
  | "typeMismatch"
  | "functionNotAllowed"
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
  /**
   * What the writer most likely meant, when the validator can tell:
   * `did you mean "total"?` for `totl>5`. English, absent when there is
   * nothing to suggest. It names only fields the config would accept.
   */
  hint?: string;
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
  visitCall(expr: CallExpr): T;
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
    case "call":
      return visitor.visitCall(expr);
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
  const add = (field: string[]) => {
    const key = field.join(".");
    if (field.length > 0 && !seen.has(key)) {
      seen.add(key);
      result.push(field);
    }
  };
  walk(expr, (node) => {
    if (node.type === "qualifier" || node.type === "presence") {
      add(node.field);
    }
    if ((node.type === "qualifier" || node.type === "call") && node.fields) {
      node.fields.forEach(add);
    }
    return true;
  });
  return result;
}

/**
 * How a query is parsed. The two switches turn off lenient forms of the
 * default grammar; they only refuse queries, so one accepted with them on
 * means the same with them off.
 */
export interface ParseOptions {
  /** Query length limit in characters. Default 256; 0 disables it. */
  maxLength?: number;
  /**
   * Require AND or OR between conditions. `a=1 b=2` fails with the code
   * `missingAnd`.
   */
  explicitAnd?: boolean;
  /** Refuse `=`, `<` and `>` inside an unquoted value, so `a=1+b=2` fails. */
  strictValues?: boolean;
}

// -------------------------------------------------------------------------
// Completion
// -------------------------------------------------------------------------

/**
 * What the grammar expects at a position in a query that is still being
 * typed. `none` means nothing can be suggested there.
 *
 * - `field`: a condition starts here: a field name, or NOT.
 * - `key`: a key of a nested field, after its dot (`labels.<here>`).
 * - `operator`: what follows a field: an operator, a selector, a connective.
 * - `selector`: what follows the `@` after a list field.
 * - `value`: a value, after an operator or inside an IN list.
 * - `connective`: AND, OR, or the `)` that closes a group.
 * - `argument`: an argument of a function call.
 */
export type CompletionSlot =
  | "none"
  | "field"
  | "key"
  | "operator"
  | "selector"
  | "value"
  | "connective"
  | "argument";

/** What a suggestion is, for grouping or decorating the list. */
export type CompletionItemKind = "field" | "key" | "operator" | "selector" | "keyword" | "value";

/** One suggestion. */
export interface CompletionItem {
  /** What to show. */
  label: string;
  /**
   * What to put in place of {@link Completion.replace}. It differs from the
   * label when the grammar needs the text written another way: a value with
   * a space in it is quoted, a nested field ends in its dot.
   */
  text: string;
  kind: CompletionItemKind;
  /** A short note to show beside the label: a field's type, what an operator does. */
  detail?: string;
}

/** What can be written at a position in a query. */
export interface Completion {
  slot: CompletionSlot;
  /**
   * The span of the query an accepted item's text goes in place of: the word
   * under the cursor, and an empty span at the cursor when there is none.
   * Offsets index the JavaScript string, as `selectionStart` does.
   */
  replace: { offset: number; length: number };
  /**
   * What has been typed of the thing being completed, without quotes, and
   * for a key without the field it belongs to. The items are already
   * filtered by it; filter candidates you fetch by it too.
   */
  prefix: string;
  /** The suggestions the schema can make, best first. Empty when only you know the candidates. */
  items: CompletionItem[];
  /**
   * The field the slot is about: the one a value, operator or selector
   * belongs to, and for a key the nested field whose key is being typed.
   */
  field?: string;
  /** The field's type, when the field is declared. */
  type?: FieldTypeName;
  /** For a value: the operator it follows, as written (`=`, `>=`, `:`, `IN`). */
  operator?: string;
  /** For an argument: the function being called. */
  function?: string;
  /** The list selectors the position is inside, outermost first. */
  lists?: string[];
  /** The cursor is inside an open quoted string; every item's text is a whole quoted string. */
  quoted?: boolean;
  /**
   * The value is an operand of arithmetic, as after the minus in
   * `createdAt>now()-`. Only a number, a date or a duration can stand there,
   * so `offer` leaves out any other candidate.
   */
  arithmetic?: boolean;
  /** The arithmetic operator such a value follows: `+`, `-`, `*`, `/` or `%`. */
  sign?: string;
}

/** Options for {@link QueryAPI.complete}: the grammar the query will be parsed under. */
export type CompleteOptions = ParseOptions;

/** Result from complete. A query that is merely unfinished is never an error. */
export interface CompleteResult {
  result?: Completion;
  error?: string;
}

/** Result from offer. */
export interface OfferResult {
  result?: CompletionItem[];
  error?: string;
}

/**
 * Put an accepted item into the query, as an editor would: the item's text
 * replaces the completion's span. Returns the new text and where the cursor
 * belongs after it.
 */
export function applyCompletion(
  query: string,
  completion: Pick<Completion, "replace">,
  item: Pick<CompletionItem, "text">
): { query: string; cursor: number } {
  const { offset, length } = completion.replace;
  return {
    query: query.slice(0, offset) + item.text + query.slice(offset + length),
    cursor: offset + item.text.length,
  };
}
